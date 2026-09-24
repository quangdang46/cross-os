//go:build darwin

// Installed-app enumeration for the "IF App = X" rule picker (bead
// w1-app-enumeration).
//
// The picker needs the machine's applications as {bundle id, display name,
// app mode} rows. On macOS that set belongs to LaunchServices: the running
// applications NSWorkspace reports, plus the bundles registered in the
// standard install locations. LaunchServices publishes no "list every
// application" call — LSCopyAllApplications and its siblings are
// underscore-prefixed and would fail App Store review — so the walk supplies
// candidate bundle paths and LaunchServices is what decides what each one is
// and what to call it.
//
// Neither half belongs on the keydown path: the Settings page asks for the
// list when it opens. The mode on each row comes from the caller's
// ctx.Classifier, the same one the resolver decides with, rather than a table
// kept in parallel here — a picker that filed a terminal under "browser"
// would offer a rule the matcher never fires.
package adapter

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit

#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#import <AppKit/AppKit.h>

// cxapp_row is one application as LaunchServices describes it. Fixed-size C
// strings, not char*: a longer name is truncated by cxapp_str rather than
// run off the end, and Go sees a NUL-terminated string either way.
typedef struct {
    char bundle_id[256];
    char display_name[256];
    char executable[128];
    int  pid;
} cxapp_row;

static void cxapp_str(char *dst, size_t n, NSString *s) {
    dst[0] = '\0';
    if (s == nil) {
        return;
    }
    const char *utf8 = [s UTF8String];
    if (utf8 == NULL) {
        return;
    }
    strncpy(dst, utf8, n - 1);
    dst[n - 1] = '\0';
}

// cxapp_running copies the LaunchServices running-application set. Pass a
// NULL out to count first: the buffer is then exactly the size of the set,
// so no application is dropped by a fixed cap and no second pass is needed
// to tell "full" from "complete".
static int cxapp_running(cxapp_row *out, int max) {
    int n = 0;
    @autoreleasepool {
        for (NSRunningApplication *ra in [[NSWorkspace sharedWorkspace] runningApplications]) {
            NSString *bid = [ra bundleIdentifier];
            // A process with no bundle — a plain executable started from a
            // shell — has no identifier for a rule to be scoped to. It is not
            // a row, and naming it after its process would be the fabricated
            // control this list exists to avoid.
            if (bid == nil || [bid length] == 0) {
                continue;
            }
            if (out != NULL && n < max) {
                cxapp_row *r = &out[n];
                memset(r, 0, sizeof(*r));
                cxapp_str(r->bundle_id, sizeof(r->bundle_id), bid);
                cxapp_str(r->display_name, sizeof(r->display_name), [ra localizedName]);
                cxapp_str(r->executable, sizeof(r->executable), [[ra executableURL] lastPathComponent]);
                r->pid = (int)[ra processIdentifier];
            }
            n++;
        }
    }
    return n;
}

// cxapp_installed describes one bundle on disk, naming it through
// LaunchServices. Returns 0 for a path that is not an application bundle the
// system can identify (a helper directory that happens to end in .app, an
// unreadable bundle), so the caller skips it instead of reporting a row it
// cannot vouch for.
static int cxapp_installed(const char *path, cxapp_row *out) {
    @autoreleasepool {
        NSBundle *bundle = [NSBundle bundleWithPath:[NSString stringWithUTF8String:path]];
        if (bundle == nil) {
            return 0;
        }
        NSString *bid = [bundle bundleIdentifier];
        if (bid == nil || [bid length] == 0) {
            return 0;
        }
        memset(out, 0, sizeof(*out));
        cxapp_str(out->bundle_id, sizeof(out->bundle_id), bid);

        // LaunchServices owns the canonical copy of an application: the one
        // it would actually launch, when one bundle id is installed in more
        // than one place (/Applications and ~/Applications, or a developer
        // build beside the released one). Name and executable come from that
        // copy, and fall back to the bundle we were handed when LS has no
        // registration for the id yet — an app copied in and never launched
        // is still installed, and dropping it would hide something the user
        // can see in Finder.
        NSBundle *named = bundle;
        NSURL *registered = [[NSWorkspace sharedWorkspace] URLForApplicationWithBundleIdentifier:bid];
        if (registered != nil) {
            NSBundle *canonical = [NSBundle bundleWithURL:registered];
            if (canonical != nil) {
                named = canonical;
            }
        }

        // CFBundleDisplayName is the localized name Finder shows, so it wins.
        // CFBundleName is what the bundle calls itself when it ships no
        // display name at all. The directory name is the last resort and is
        // still true: it is what the file on disk is called. (NSBundle's
        // bundleDisplayName getter would answer the first two in one call and
        // is gone from the SDK, so the Info.plist keys are read directly.)
        NSString *name = [named objectForInfoDictionaryKey:@"CFBundleDisplayName"];
        if (name == nil || [name length] == 0) {
            name = [named objectForInfoDictionaryKey:@"CFBundleName"];
        }
        if (name == nil || [name length] == 0) {
            name = [[named bundleURL] lastPathComponent];
        }
        cxapp_str(out->display_name, sizeof(out->display_name), name);

        NSString *exe = [named objectForInfoDictionaryKey:@"CFBundleExecutable"];
        if (exe == nil || [exe length] == 0) {
            exe = [[named bundleURL] lastPathComponent];
        }
        cxapp_str(out->executable, sizeof(out->executable), exe);
        out->pid = 0; // installed is not running; a PID here would be a lie
        return 1;
    }
}
*/
import "C"

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unsafe"

	"crossos/core/pkg/ctx"
)

// appRoots are where macOS installs applications. /System/Applications is
// here because the rules people actually write name Finder and Terminal, not
// only their third-party apps.
var appRoots = []string{
	"/Applications",
	"/System/Applications",
}

// appWalkDepth bounds the walk. The standard layout is two levels deep
// (/Applications/Utilities); the headroom covers a user who groups apps into
// folders of their own. It is also what stops a symlink pointing back up the
// tree from walking forever.
const appWalkDepth = 4

// ListApps returns the applications a rule may be scoped to, one row per
// bundle id, ordered by display name.
//
// The result is never nil: an empty machine yields an empty slice, so the
// picker renders "no applications" rather than a missing value. This never
// fails — the running set is whatever the workspace reports and the installed
// set is whatever is on disk, and an absence in either is an absence in the
// list, not a fault to report.
//
// c is the classifier the caller resolves rules with. A nil c falls back to
// ctx.SeedClassifier, the fail-open table the interface itself ends in,
// because the zero value of AppMode is not a mode and a row carrying one
// would be a row the matcher cannot act on.
func ListApps(c ctx.Classifier) ([]ctx.ApplicationInfo, error) {
	if c == nil {
		c = ctx.SeedClassifier{}
	}
	byID := make(map[string]ctx.ApplicationInfo, 256)

	// merge is order-independent: a later source only fills a field an
	// earlier one left empty and never overwrites a PID, so the running pass
	// and the installed pass can be written in either order and the result
	// will not depend on which ran first. The mode is computed from the
	// merged identity on every call rather than once per source, so no row
	// ends up classified from a half-empty pair.
	merge := func(row C.cxapp_row) {
		id := C.GoString(&row.bundle_id[0])
		if id == "" {
			return
		}
		app, seen := byID[id]
		if !seen {
			app = ctx.ApplicationInfo{BundleID: id}
		}
		if app.PID == 0 {
			app.PID = int(row.pid)
		}
		if app.DisplayName == "" {
			app.DisplayName = C.GoString(&row.display_name[0])
		}
		if app.Executable == "" {
			app.Executable = C.GoString(&row.executable[0])
		}
		app.AppMode, app.Category = c.Classify(app.BundleID, app.Executable)
		byID[id] = app
	}

	// Running applications first: a PID is live evidence that the app is on
	// this machine right now, and the installed pass cannot take it away.
	//
	// The two calls each walk the set, so an app that quits or launches
	// between them leaves the buffer short. The slice is zeroed, and a
	// zeroed row has an empty bundle id that merge drops, so a shrinking set
	// costs a row the user just closed and a growing one costs a row the
	// picker shows on its next poll. Neither is worth a lock the Settings
	// page does not need.
	if n := int(C.cxapp_running(nil, 0)); n > 0 {
		rows := make([]C.cxapp_row, n)
		C.cxapp_running(&rows[0], C.int(n))
		for i := range rows {
			merge(rows[i])
		}
	}

	roots := make([]string, 0, len(appRoots)+1)
	roots = append(roots, appRoots...)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		roots = append(roots, filepath.Join(home, "Applications"))
	}
	for _, root := range roots {
		walkApps(root, 0, func(path string) {
			var row C.cxapp_row
			cpath := C.CString(path)
			ok := C.cxapp_installed(cpath, &row)
			C.free(unsafe.Pointer(cpath))
			if ok == 1 {
				merge(row)
			}
		})
	}

	out := make([]ctx.ApplicationInfo, 0, len(byID))
	for _, app := range byID {
		out = append(out, app)
	}
	// Sorted before they leave (the rule every pagedata.go list follows): the
	// Settings page polls this, and a list that reshuffles per poll reads as
	// the picker losing track. Display name is the picker's reading order;
	// the bundle id breaks ties so two apps sharing a name hold a fixed
	// place across polls.
	sort.Slice(out, func(i, j int) bool {
		li, lj := strings.ToLower(out[i].DisplayName), strings.ToLower(out[j].DisplayName)
		if li != lj {
			return li < lj
		}
		return out[i].BundleID < out[j].BundleID
	})
	return out, nil
}

// walkApps yields the .app bundle paths under root, depth-first, and never
// descends into a bundle: a bundle's Contents ships helper applications the
// user never installed and cannot meaningfully scope a rule to. A root that
// is not there contributes nothing — ~/Applications is absent on a machine
// that has never had a per-user app.
func walkApps(root string, depth int, yield func(string)) {
	if depth >= appWalkDepth {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		// Dot-directories hold hidden and on-disk-only content. /Applications
		// shows the user the same set, and a rule naming a hidden bundle is a
		// rule the picker offered out of nowhere.
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(root, name)
		// Stat, not DirEntry.IsDir: an app on another volume, or one the
		// user relocated, is a symlink in /Applications, and IsDir would
		// report the link as a file and drop the app.
		st, err := os.Stat(path)
		if err != nil || !st.IsDir() {
			continue
		}
		if strings.HasSuffix(name, ".app") {
			yield(path)
			continue
		}
		walkApps(path, depth+1, yield)
	}
}
