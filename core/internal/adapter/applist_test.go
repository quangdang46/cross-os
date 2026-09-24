// Build-tag contract for the app-list seam (bead w1-app-enumeration).
//
// ListApps is split across two build-tagged files, which is the shape that
// rots without anyone noticing: a signature change on one side leaves the
// other compiling against something nothing calls, and a stub that returns a
// placeholder row is indistinguishable from a real one until a user scopes a
// rule to it. This file carries no tag of its own, so the stub's clause runs
// wherever the stub compiles; the last test proves both file sets still build
// for the GOOSes this host cannot run.
package adapter

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"crossos/core/pkg/ctx"
)

// recordingClassifier is the injected ctx.Classifier. It answers with a mode
// no real table produces, so a row that did not come from it is caught, and
// it remembers the identity it was asked about, so a row whose mode was
// computed from a different bundle than the one it reports is caught too.
// The second half matters because Classify takes both a bundle id and an
// executable: answering from the wrong one is a mode that looks right.
type recordingClassifier struct {
	mode ctx.AppMode
	seen map[string]string
}

func (r *recordingClassifier) Classify(bundleID, executable string) (ctx.AppMode, ctx.AppCategory) {
	if r.seen == nil {
		r.seen = make(map[string]string)
	}
	r.seen[bundleID] = executable
	return r.mode, ctx.AppUser
}

// The mode on a row is the one the caller's own classifier produces. A second
// table kept beside the resolver would show the user an app filed under a
// heading that never fires when the rule runs.
func TestListAppsClassifiesThroughTheInjectedClassifier(t *testing.T) {
	const sentinel = ctx.AppModeVM
	rec := &recordingClassifier{mode: sentinel}
	apps, err := ListApps(rec)
	if err != nil {
		t.Fatalf("ListApps: %v", err)
	}
	if len(apps) == 0 {
		// Reachable only where enumeration has no cheap equivalent; that
		// platform's contract is TestListAppsIsEmptyWithoutASource.
		t.Skip("no applications enumerated on this platform")
	}
	for _, a := range apps {
		if a.AppMode != sentinel {
			t.Fatalf("%s: mode %q, want the injected classifier's %q — a mode from a second table would not be the one the resolver decides with",
				a.BundleID, a.AppMode, sentinel)
		}
		asked, called := rec.seen[a.BundleID]
		if !called {
			t.Fatalf("%s: no classifier call recorded, so its mode came from somewhere else", a.BundleID)
		}
		if asked != a.Executable {
			t.Fatalf("%s: classified from executable %q but reports %q", a.BundleID, asked, a.Executable)
		}
	}
}

// Every row names an application the user could have pointed at, and the
// list is ordered and unique. The ordering comparison is restated here rather
// than shared with the implementation: a test that calls the function it is
// testing proves nothing about it.
func TestListAppsRowsAreRealAndOrdered(t *testing.T) {
	// A nil classifier must still produce a mode. It falls back to the
	// fail-open seed table, and the zero value of AppMode is not a mode, so a
	// row carrying one would be a row the matcher cannot act on.
	apps, err := ListApps(nil)
	if err != nil {
		t.Fatalf("ListApps: %v", err)
	}
	if apps == nil {
		t.Fatal("nil list: the picker needs [] so an empty machine renders as empty, not missing")
	}
	seen := make(map[string]bool, len(apps))
	for _, a := range apps {
		if a.BundleID == "" {
			t.Fatalf("row with no bundle id, which no rule can be scoped to: %+v", a)
		}
		if a.DisplayName == "" {
			t.Fatalf("%s: no display name", a.BundleID)
		}
		if !knownMode(a.AppMode) {
			t.Fatalf("%s: mode %q is not one the resolver knows", a.BundleID, a.AppMode)
		}
		if seen[a.BundleID] {
			t.Fatalf("%s appears twice", a.BundleID)
		}
		seen[a.BundleID] = true
	}
	for i := 1; i < len(apps); i++ {
		if before(apps[i], apps[i-1]) {
			t.Fatalf("out of order: %q/%q sorted before %q/%q",
				apps[i].DisplayName, apps[i].BundleID,
				apps[i-1].DisplayName, apps[i-1].BundleID)
		}
	}
	// The Settings page polls this list; a second call that returns a
	// different set or a different order reads as the picker losing track.
	again, err := ListApps(nil)
	if err != nil {
		t.Fatalf("second ListApps: %v", err)
	}
	if len(again) != len(apps) {
		t.Fatalf("poll changed the list: %d rows then %d", len(apps), len(again))
	}
	for i := range again {
		if again[i].BundleID != apps[i].BundleID {
			t.Fatalf("poll reordered the list at %d: %s then %s", i, apps[i].BundleID, again[i].BundleID)
		}
	}
}

// The stub's contract, and the reason this file is untagged: the assertion has
// to run wherever applist_other.go compiles. A row here would be a bundle id
// the user never installed — a rule that looks scoped in the editor and never
// fires. A nil slice and an error are both wrong for the same reason: the
// caller would show an error the user cannot act on, or a control that never
// resolves, where the honest answer is an empty picker over typed free text.
func TestListAppsIsEmptyWithoutASource(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin enumerates the real set through LaunchServices")
	}
	rec := &recordingClassifier{mode: ctx.AppModeVM}
	apps, err := ListApps(rec)
	if err != nil {
		t.Fatalf("no source is not a failure, got: %v", err)
	}
	if apps == nil {
		t.Fatal("nil list: the picker needs [] so an empty machine renders as empty, not missing")
	}
	if len(apps) != 0 {
		t.Fatalf("got %d fabricated rows, want none; first: %+v", len(apps), apps[0])
	}
	if len(rec.seen) != 0 {
		t.Fatalf("classifier called for %d applications that do not exist: %v", len(rec.seen), rec.seen)
	}
}

// The compile half of the same contract. A run on one GOOS only ever proves
// that GOOS: the stub can rot on linux while every test here passes on the
// mac it was written on. scripts/dev-verify.sh gates the whole tree the same
// way, but only when someone runs it, and it allows known gaps; this keeps
// the seam's own invariant attached to the seam and fails closed.
func TestListAppsBuildsUnderEveryGoosConstraint(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("no go tool on PATH: %v", err)
	}
	if runtime.GOOS == "darwin" && os.Getenv("CGO_ENABLED") == "0" {
		t.Skip("cgo disabled: the darwin file set cannot be type-checked here")
	}
	// Building FOR darwin needs the macOS SDK and a cgo toolchain, so a
	// linux or windows dev box asserts the targets it can and the mac asserts
	// all three.
	targets := []string{"linux", "windows"}
	if runtime.GOOS == "darwin" {
		targets = append([]string{"darwin"}, targets...)
	}
	bin := filepath.Join(t.TempDir(), "adapter.test")
	for _, goos := range targets {
		checks := [][]string{
			{"build", "."},
			{"vet", "."},
			// test -c, not test: it compiles this file TOGETHER WITH the file
			// set for that GOOS, so the clause above is proven to type-check
			// against applist_other.go instead of sitting in a file the darwin
			// build never reads. The binary is never run — the host cannot.
			{"test", "-c", "-o", bin, "."},
		}
		for _, args := range checks {
			cmd := exec.Command(goBin, args...)
			cmd.Env = append(os.Environ(), "GOOS="+goos)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("GOOS=%s go %s: %v\n%s", goos, strings.Join(args, " "), err, out)
			}
		}
	}
}

// knownMode reports whether m is a mode the resolver has a case for.
func knownMode(m ctx.AppMode) bool {
	switch m {
	case ctx.AppModeNative, ctx.AppModeTerminal, ctx.AppModeRemote,
		ctx.AppModeVM, ctx.AppModeExcluded:
		return true
	}
	return false
}

// before is the ordering the list is sorted into: case-insensitive display
// name, then bundle id.
func before(a, b ctx.ApplicationInfo) bool {
	la, lb := strings.ToLower(a.DisplayName), strings.ToLower(b.DisplayName)
	if la != lb {
		return la < lb
	}
	return a.BundleID < b.BundleID
}
