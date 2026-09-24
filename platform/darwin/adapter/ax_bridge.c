// CrossOS macOS AX bridge — focused-window query + move/resize over the
// Accessibility API (bead cross-os-ax-bridge-window, plan §4.1 window
// management; shape adapted from platform/darwin/spike_c).
//
// Authority: this file EXECUTES. Core decides (intent → capability) and
// calls in; nothing here decides policy, and nothing here can suppress a
// key — the tap's Dispatch seam owns that.
//
// Shape (spike_c query_darwin.go, Rectangle AXExtension.swift reference):
//   NSWorkspace.frontmostApplication → bundle id + pid
//   AXUIElementCreateApplication(pid) → kAXFocusedWindowAttribute
//   kAXPositionAttribute / kAXSizeAttribute via AXValue (CGPoint/CGSize)
//   set with AXUIElementSetAttributeValue
//
// Failure is a typed AX error code (never a hang, never a silent success):
// kAXErrorAPIDisabled / kAXErrorNotTrusted map to permission-denied in Go.
// CGWindowList answers the physical plane — existence, geometry, ordered-in,
// minimized, and a CGWindowID for logs/UI — for every window in one batched
// call, so enumeration runs there and the AX plane is touched only when an
// action needs an element. Move/resize still targets the AX focused window;
// a CGWindowID is not an AX handle, and the bridge never pretends otherwise.

#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#import <CoreFoundation/CoreFoundation.h>
#import <string.h>

typedef struct {
    int ok;            // 1 on success
    int err;           // AXError (or -1 for a non-AX failure)
    pid_t pid;
    unsigned int window_id;
    double x, y, w, h; // screen points
    char bundle[256];
    char title[512];
    char role[64];
} cxax_window;

static void cxax_copy_string(const char *src, char *dst, size_t n) {
    if (src == NULL) {
        dst[0] = '\0';
        return;
    }
    strncpy(dst, src, n - 1);
    dst[n - 1] = '\0';
}

// ---------------------------------------------------------------------------
// The physical plane.
//
// Existence, geometry, ordered-in and the minimized bit are all facts the
// WindowServer owns, and CGWindowList answers every one of them for every
// window in a single batched call. That is the whole reason enumeration runs
// here rather than in AX: an AX attribute is a call INTO the owning app, so it
// costs that app as much as it is busy and cannot be bounded from outside. The
// split is deliberate — the physical plane stays available when an app is
// wedged; the semantic plane (which window inside a process is key) is read
// lazily, and only when an action actually needs an element.
//
// The AX↔wid bridge is one-directional: _AXUIElementGetWindow goes element→wid
// and there is no wid→element routine, so an element is acquired by reading an
// app's windows once and keying what comes back by the wid, then cached. The
// per-app acquisition count is exposed rather than assumed, because "one
// batched read per app" is a claim that rots silently the moment a loop moves
// the read inside it.

// _AXUIElementGetWindow is the element→wid direction. Declared here because it
// is not in any public header; the absence of the reverse direction is the
// reason acquisition is an enumerate-and-cache rather than a lookup.
extern unsigned int _AXUIElementGetWindow(AXUIElementRef element);

// cxax_wrow is one window as the WindowServer describes it. Fixed-size C
// strings, so a longer title is truncated rather than run off the end.
typedef struct {
    int minimized;        // always 0 here; the Go side decodes it, see below
    int on_screen;
    int pid;
    unsigned int window_id;
    double x, y, w, h;    // screen points
    char bundle[256];
    char title[512];
} cxax_wrow;

typedef void (*cxax_wrow_fn)(const cxax_wrow *row, void *ctx);

// cxax_cf_int reads a CFNumber field as an int, falling back when the key is
// absent or holds something else. Every CGWindowList field is optional and its
// type is not promised, so a missing key must read as "unknown", never as a
// cast of a null pointer.
static int cxax_cf_int(CFDictionaryRef d, const void *key, int fallback) {
    CFTypeRef v = CFDictionaryGetValue(d, key);
    if (v == NULL || CFGetTypeID(v) != CFNumberGetTypeID()) {
        return fallback;
    }
    int out = fallback;
    if (!CFNumberGetValue((CFNumberRef)v, kCFNumberIntType, &out)) {
        return fallback;
    }
    return out;
}

static double cxax_cf_double(CFDictionaryRef d, const void *key) {
    CFTypeRef v = CFDictionaryGetValue(d, key);
    if (v == NULL || CFGetTypeID(v) != CFNumberGetTypeID()) {
        return 0;
    }
    CGFloat out = 0;
    if (!CFNumberGetValue((CFNumberRef)v, kCFNumberCGFloatType, &out)) {
        return 0;
    }
    return (double)out;
}

static CGRect cxax_cf_rect(CFDictionaryRef d) {
    CFDictionaryRef b = (CFDictionaryRef)CFDictionaryGetValue(d, kCGWindowBounds);
    if (b == NULL || CFGetTypeID(b) != CFDictionaryGetTypeID()) {
        return CGRectZero;
    }
    return CGRectMake(cxax_cf_double(b, CFSTR("X")), cxax_cf_double(b, CFSTR("Y")),
                      cxax_cf_double(b, CFSTR("Width")), cxax_cf_double(b, CFSTR("Height")));
}

// cxax_walk_windows hands every normal-layer window of one CGWindowList
// option set to yield, in the order the WindowServer returns them. Non-normal
// layers are dropped here so no caller can forget: the menu bar, the shadow
// behind a window, a panel and a sheet's parent all have a wid, and a row
// carrying one of those is a row the user cannot point at.
static void cxax_walk_windows(int options, cxax_wrow_fn yield, void *ctx) {
    @autoreleasepool {
        CFArrayRef list = CGWindowListCopyWindowInfo(options, kCGNullWindowID);
        if (list == NULL) {
            return;
        }
        CFIndex n = CFArrayGetCount(list);
        for (CFIndex i = 0; i < n; i++) {
            CFDictionaryRef info = CFArrayGetValueAtIndex(list, i);
            if (info == NULL || CFGetTypeID(info) != CFDictionaryGetTypeID()) {
                continue;
            }
            if (cxax_cf_int(info, kCGWindowLayer, -1) != 0) {
                continue;
            }
            cxax_wrow row;
            memset(&row, 0, sizeof(row));
            row.window_id = (unsigned int)cxax_cf_int(info, kCGWindowNumber, 0);
            row.pid = cxax_cf_int(info, kCGWindowOwnerPID, 0);
            row.on_screen = cxax_cf_int(info, kCGWindowIsOnscreen, 1);
            CGRect b = cxax_cf_rect(info);
            row.x = b.origin.x;
            row.y = b.origin.y;
            row.w = b.size.width;
            row.h = b.size.height;
            // The bundle id comes from the workspace rather than the row: the
            // row's own name field is what Screen Recording withholds, so a
            // window would lose the one identifier a rule can be scoped to at
            // exactly the moment the user needs it.
            NSRunningApplication *app = [NSRunningApplication
                runningApplicationWithProcessIdentifier:(pid_t)row.pid];
            if (app != nil) {
                cxax_copy_string([[app bundleIdentifier] UTF8String], row.bundle,
                                 sizeof(row.bundle));
            }
            CFStringRef name = (CFStringRef)CFDictionaryGetValue(info, kCGWindowName);
            cxax_copy_string(name == NULL
                                 ? NULL
                                 : CFStringGetCStringPtr(name, kCFStringEncodingUTF8),
                             row.title, sizeof(row.title));
            row.minimized = 0; // decoded in Go, where the table test lives
            yield(&row, ctx);
        }
        CFRelease(list);
    }
}

typedef struct {
    pid_t pid;
    const char *title;
    unsigned int found;
    unsigned int fallback;
} cxax_id_ctx;

static void cxax_id_yield(const cxax_wrow *row, void *vctx) {
    cxax_id_ctx *c = (cxax_id_ctx *)vctx;
    if ((pid_t)row->pid != c->pid) {
        return;
    }
    if (c->title != NULL && c->title[0] != '\0' && strcmp(row->title, c->title) == 0) {
        c->found = row->window_id;
        return;
    }
    if (c->fallback == 0) {
        c->fallback = row->window_id;
    }
}

// cxax_window_id reports the CGWindowID of the frontmost app's on-screen
// window whose title matches, falling back to its first normal-layer
// window. Purely an identifier for logs/UI — never the move target. It reads
// the same walk enumeration does, so the two can never disagree about which
// windows exist.
static unsigned int cxax_window_id(pid_t pid, const char *title) {
    cxax_id_ctx c;
    c.pid = pid;
    c.title = title;
    c.found = 0;
    c.fallback = 0;
    cxax_walk_windows(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements,
                      cxax_id_yield, &c);
    return c.found != 0 ? c.found : c.fallback;
}

typedef struct {
    cxax_wrow *out;
    int max;
    int n;
} cxax_fill_ctx;

static void cxax_fill_yield(const cxax_wrow *row, void *vctx) {
    cxax_fill_ctx *c = (cxax_fill_ctx *)vctx;
    if (c->out != NULL && c->n < c->max) {
        c->out[c->n] = *row;
    }
    c->n++;
}

// cxax_list_windows walks the full window set — not just the on-screen one, so
// a minimized window is a row rather than a gap. Pass a NULL out to count, the
// same two-pass shape the app list uses, so no window is dropped by a cap and
// no second call is needed to tell "full" from "complete".
int cxax_list_windows(cxax_wrow *out, int max) {
    cxax_fill_ctx c;
    c.out = out;
    c.max = max;
    c.n = 0;
    cxax_walk_windows(kCGWindowListOptionAll | kCGWindowListExcludeDesktopElements,
                      cxax_fill_yield, &c);
    return c.n;
}

// cxax_front_wid reports the window the user is looking at when the frontmost
// application owns exactly one normal-layer on-screen window, and 0 when it
// owns more. With several, which of them holds the attention is an order fact
// the physical plane does not carry, so this declines rather than guessing —
// the caller falls through to the AX plane, which does know.
int cxax_front_wid(void) {
    @autoreleasepool {
        NSRunningApplication *front = [[NSWorkspace sharedWorkspace] frontmostApplication];
        if (front == nil) {
            return 0;
        }
        pid_t pid = [front processIdentifier];
        CFArrayRef list = CGWindowListCopyWindowInfo(
            kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements,
            kCGNullWindowID);
        if (list == NULL) {
            return 0;
        }
        unsigned int found = 0;
        int count = 0;
        CFIndex n = CFArrayGetCount(list);
        for (CFIndex i = 0; i < n; i++) {
            CFDictionaryRef info = CFArrayGetValueAtIndex(list, i);
            if (info == NULL || CFGetTypeID(info) != CFDictionaryGetTypeID()) {
                continue;
            }
            if (cxax_cf_int(info, kCGWindowLayer, -1) != 0) {
                continue;
            }
            if (cxax_cf_int(info, kCGWindowOwnerPID, -1) != (int)pid) {
                continue;
            }
            found = (unsigned int)cxax_cf_int(info, kCGWindowNumber, 0);
            count++;
        }
        CFRelease(list);
        return count == 1 ? found : 0;
    }
}

// --- The semantic plane, acquired lazily and counted -----------------------

#define CXAX_READ_SLOTS 256
#define CXAX_ELEM_SLOTS 128

static int cxax_read_pid[ CXAX_READ_SLOTS ];
static int cxax_read_count[ CXAX_READ_SLOTS ];
static int cxax_read_slots = 0;
static int cxax_read_total = 0;

static void cxax_record_windows_read(pid_t pid) {
    cxax_read_total++;
    for (int i = 0; i < cxax_read_slots; i++) {
        if (cxax_read_pid[i] == (int)pid) {
            cxax_read_count[i]++;
            return;
        }
    }
    if (cxax_read_slots < CXAX_READ_SLOTS) {
        cxax_read_pid[cxax_read_slots] = (int)pid;
        cxax_read_count[cxax_read_slots] = 1;
        cxax_read_slots++;
    }
}

typedef struct {
    unsigned int wid;
    pid_t pid;
    AXUIElementRef el;
} cxax_elem_slot;

static cxax_elem_slot cxax_elems[ CXAX_ELEM_SLOTS ];
static int cxax_elem_next = 0;

// cxax_elem_lookup returns a borrowed reference, or NULL. The pid is part of
// the key: a wid that reappears under a different pid is a different window,
// and acting on the cached one would raise a window in an app the caller
// never named.
static AXUIElementRef cxax_elem_lookup(unsigned int wid, pid_t pid) {
    for (int i = 0; i < CXAX_ELEM_SLOTS; i++) {
        if (cxax_elems[i].el != NULL && cxax_elems[i].wid == wid &&
            cxax_elems[i].pid == pid) {
            return cxax_elems[i].el;
        }
    }
    return NULL;
}

static void cxax_elem_store(unsigned int wid, pid_t pid, AXUIElementRef el) {
    int slot = -1;
    for (int i = 0; i < CXAX_ELEM_SLOTS; i++) {
        if (cxax_elems[i].el != NULL && cxax_elems[i].wid == wid &&
            cxax_elems[i].pid == pid) {
            CFRelease(cxax_elems[i].el);
            cxax_elems[i].el = (AXUIElementRef)CFRetain(el);
            return;
        }
        if (cxax_elems[i].el == NULL && slot < 0) {
            slot = i;
        }
    }
    if (slot < 0) {
        slot = cxax_elem_next;
        cxax_elem_next = (cxax_elem_next + 1) % CXAX_ELEM_SLOTS;
        if (cxax_elems[slot].el != NULL) {
            CFRelease(cxax_elems[slot].el);
        }
    }
    cxax_elems[slot].wid = wid;
    cxax_elems[slot].pid = pid;
    cxax_elems[slot].el = (AXUIElementRef)CFRetain(el);
}

// cxax_acquire_app reads one app's window list and caches every element it
// returns, keyed by the wid that comes back from the element→wid direction.
// This is the ONE call into the app; everything after it is a lookup. Returns
// the number of elements cached, or 0 with *err set.
static int cxax_acquire_app(pid_t pid, int *err) {
    if (err != NULL) {
        *err = -1;
    }
    @autoreleasepool {
        AXUIElementRef appEl = AXUIElementCreateApplication(pid);
        if (appEl == NULL) {
            if (err != NULL) {
                *err = kAXErrorCannotComplete;
            }
            return 0;
        }
        cxax_record_windows_read(pid);
        CFTypeRef ref = NULL;
        AXError e = AXUIElementCopyAttributeValue(appEl, kAXWindowsAttribute, &ref);
        if (e != kAXErrorSuccess || ref == NULL) {
            if (err != NULL) {
                *err = (int)e;
            }
            CFRelease(appEl);
            return 0;
        }
        CFArrayRef arr = (CFArrayRef)ref;
        CFIndex n = CFArrayGetCount(arr);
        int cached = 0;
        for (CFIndex i = 0; i < n; i++) {
            AXUIElementRef w = (AXUIElementRef)CFArrayGetValueAtIndex(arr, i);
            if (w == NULL) {
                continue;
            }
            unsigned int wid = _AXUIElementGetWindow(w);
            if (wid == 0) {
                continue;
            }
            cxax_elem_store(wid, pid, w);
            cached++;
        }
        CFRelease(arr);
        CFRelease(appEl);
        if (err != NULL) {
            *err = 0;
        }
        return cached;
    }
}

// cxax_app_titles resolves the titles for up to max window ids, taking them
// off the elements the single batched read already returned. The title read
// is on an element in hand, not a fresh query into a busy app, which is the
// difference between this and one attribute read per window.
int cxax_app_titles(pid_t pid, const unsigned int *wids, char *titles, int title_cap,
                    int max, int *err) {
    if (err != NULL) {
        *err = -1;
    }
    // A failed acquisition is reported, not absorbed. The caller asked for
    // these windows' titles and could not get them; returning an empty set
    // would drop every window this app owns from the list while looking
    // exactly like an app that has none.
    if (max > 0 && cxax_acquire_app(pid, err) == 0) {
        return 0;
    }
    @autoreleasepool {
        for (int i = 0; i < max; i++) {
            AXUIElementRef el = cxax_elem_lookup(wids[i], pid);
            char t[512];
            t[0] = '\0';
            if (el != NULL) {
                CFTypeRef tr = NULL;
                if (AXUIElementCopyAttributeValue(el, kAXTitleAttribute, &tr) ==
                        kAXErrorSuccess &&
                    tr != NULL) {
                    cxax_copy_string(
                        CFStringGetCStringPtr((CFStringRef)tr, kCFStringEncodingUTF8), t,
                        sizeof(t));
                    CFRelease(tr);
                }
            }
            cxax_copy_string(t, titles + (size_t)i * (size_t)title_cap, (size_t)title_cap);
        }
        if (err != NULL) {
            *err = 0;
        }
    }
    return max;
}

// cxax_focus_window raises the window named by wid and brings its application
// forward. The element comes from the cache when it is there and from the one
// batched read when it is not, so a repeated focus on the same window costs no
// call into the app at all.
int cxax_focus_window(unsigned int wid, pid_t pid, int *err) {
    if (err != NULL) {
        *err = -1;
    }
    AXUIElementRef el = cxax_elem_lookup(wid, pid);
    if (el == NULL) {
        int ignored = 0;
        if (cxax_acquire_app(pid, &ignored) == 0) {
            if (err != NULL) {
                *err = kAXErrorNoValue;
            }
            return 0;
        }
        el = cxax_elem_lookup(wid, pid);
    }
    if (el == NULL) {
        if (err != NULL) {
            *err = kAXErrorNoValue;
        }
        return 0;
    }
    @autoreleasepool {
        AXError raised = AXUIElementPerformAction(el, kAXRaiseAction);
        if (raised != kAXErrorSuccess) {
            if (err != NULL) {
                *err = (int)raised;
            }
            return 0;
        }
        NSRunningApplication *app =
            [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
        if (app != nil) {
            // No options: the ignore-other-apps flag was deprecated in macOS 14
            // and does nothing there, while an unconditional activation would
            // steal the front from an app the user pinned on top.
            [app activateWithOptions:0];
        }
        if (err != NULL) {
            *err = 0;
        }
        return 1;
    }
}

// The grant, as one boolean. Read before any call into another app, so a
// machine without accessibility consent pays nothing and learns immediately.
int cxax_ax_trusted(void) { return AXIsProcessTrusted() ? 1 : 0; }

// Per-app and total batched-acquisition counts, with a reset. These are the
// measurement the window seam is held to: a minimized window and an on-screen
// window cost the same, and a second call costs none of either.
int cxax_windows_reads_total(void) { return cxax_read_total; }

int cxax_windows_reads_for_pid(int pid) {
    for (int i = 0; i < cxax_read_slots; i++) {
        if (cxax_read_pid[i] == pid) {
            return cxax_read_count[i];
        }
    }
    return 0;
}

void cxax_windows_reads_reset(void) {
    cxax_read_total = 0;
    cxax_read_slots = 0;
}

// cxax_focused fills *out with the frontmost app + its focused window.
// Returns 1 on success, 0 on failure (*out->err carries the AXError).
static int cxax_focused(cxax_window *out) {
    memset(out, 0, sizeof(*out));
    out->err = -1;

    @autoreleasepool {
        NSRunningApplication *app = [[NSWorkspace sharedWorkspace]
            frontmostApplication];
        if (app == nil) {
            out->err = kAXErrorNoValue;
            return 0;
        }
        out->pid = [app processIdentifier];
        cxax_copy_string([[app bundleIdentifier] UTF8String], out->bundle,
                         sizeof(out->bundle));

        AXUIElementRef appEl =
            AXUIElementCreateApplication((pid_t)out->pid);
        if (appEl == NULL) {
            out->err = kAXErrorCannotComplete;
            return 0;
        }

        CFTypeRef focusedRef = NULL;
        AXError err = AXUIElementCopyAttributeValue(
            appEl, kAXFocusedWindowAttribute, &focusedRef);
        if (err != kAXErrorSuccess || focusedRef == NULL) {
            out->err = (int)err;
            CFRelease(appEl);
            return 0;
        }
        AXUIElementRef win = (AXUIElementRef)focusedRef;

        CFTypeRef titleRef = NULL;
        if (AXUIElementCopyAttributeValue(win, kAXTitleAttribute, &titleRef) ==
                kAXErrorSuccess &&
            titleRef != NULL) {
            cxax_copy_string(CFStringGetCStringPtr((CFStringRef)titleRef,
                                                   kCFStringEncodingUTF8),
                             out->title, sizeof(out->title));
            CFRelease(titleRef);
        }
        CFTypeRef roleRef = NULL;
        if (AXUIElementCopyAttributeValue(win, kAXRoleAttribute, &roleRef) ==
                kAXErrorSuccess &&
            roleRef != NULL) {
            cxax_copy_string(CFStringGetCStringPtr((CFStringRef)roleRef,
                                                   kCFStringEncodingUTF8),
                             out->role, sizeof(out->role));
            CFRelease(roleRef);
        }

        CGPoint pos = CGPointZero;
        CFTypeRef posRef = NULL;
        if (AXUIElementCopyAttributeValue(win, kAXPositionAttribute, &posRef) ==
                kAXErrorSuccess &&
            posRef != NULL) {
            AXValueGetValue((AXValueRef)posRef, kAXValueCGPointType, &pos);
            CFRelease(posRef);
        }
        CGSize size = CGSizeZero;
        CFTypeRef sizeRef = NULL;
        if (AXUIElementCopyAttributeValue(win, kAXSizeAttribute, &sizeRef) ==
                kAXErrorSuccess &&
            sizeRef != NULL) {
            AXValueGetValue((AXValueRef)sizeRef, kAXValueCGSizeType, &size);
            CFRelease(sizeRef);
        }
        out->x = pos.x;
        out->y = pos.y;
        out->w = size.width;
        out->h = size.height;

        out->window_id = cxax_window_id((pid_t)out->pid, out->title);

        CFRelease(win);
        CFRelease(appEl);
        out->ok = 1;
        out->err = 0;
        return 1;
    }
}

// cxax_move_resize sets kAXPositionAttribute / kAXSizeAttribute on the
// frontmost app's focused window. Returns 1 on success; on failure
// *err carries the AXError so Go can map consent denial precisely.
static int cxax_move_resize(double x, double y, double w, double h, int doMove,
                            int doResize, int *err) {
    if (err != NULL) {
        *err = -1;
    }
    @autoreleasepool {
        NSRunningApplication *app = [[NSWorkspace sharedWorkspace]
            frontmostApplication];
        if (app == nil) {
            if (err != NULL) {
                *err = kAXErrorNoValue;
            }
            return 0;
        }
        AXUIElementRef appEl =
            AXUIElementCreateApplication([app processIdentifier]);
        if (appEl == NULL) {
            if (err != NULL) {
                *err = kAXErrorCannotComplete;
            }
            return 0;
        }
        CFTypeRef focusedRef = NULL;
        AXError e0 = AXUIElementCopyAttributeValue(
            appEl, kAXFocusedWindowAttribute, &focusedRef);
        if (e0 != kAXErrorSuccess || focusedRef == NULL) {
            if (err != NULL) {
                *err = (int)e0;
            }
            CFRelease(appEl);
            return 0;
        }
        AXUIElementRef win = (AXUIElementRef)focusedRef;

        int failed = 0;
        if (doMove) {
            CGPoint pos = CGPointMake(x, y);
            AXValueRef v = AXValueCreate(kAXValueCGPointType, &pos);
            if (v == NULL) {
                failed = 1;
            } else {
                AXError e = AXUIElementSetAttributeValue(
                    win, kAXPositionAttribute, v);
                CFRelease(v);
                if (e != kAXErrorSuccess) {
                    failed = 1;
                    if (err != NULL) {
                        *err = (int)e;
                    }
                }
            }
        }
        if (doResize && !failed) {
            CGSize size = CGSizeMake(w, h);
            AXValueRef v = AXValueCreate(kAXValueCGSizeType, &size);
            if (v == NULL) {
                failed = 1;
            } else {
                AXError e =
                    AXUIElementSetAttributeValue(win, kAXSizeAttribute, v);
                CFRelease(v);
                if (e != kAXErrorSuccess) {
                    failed = 1;
                    if (err != NULL) {
                        *err = (int)e;
                    }
                }
            }
        }
        if (!failed && err != NULL) {
            *err = 0;
        }

        CFRelease(win);
        CFRelease(appEl);
        return failed ? 0 : 1;
    }
}
