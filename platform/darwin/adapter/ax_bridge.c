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
#import <stdlib.h>

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

// Window action tags Go sends to cxax_window_action. Go declares the same
// numbers (ax_cgo.go), so a renumbering here without there breaks the call
// loudly in reviewable diffs rather than silently in behavior.
#define CXAX_WIN_CLOSE 1
#define CXAX_WIN_MINIMIZE 2
#define CXAX_WIN_MAXIMIZE 3

static void cxax_copy_string(const char *src, char *dst, size_t n) {
    if (src == NULL) {
        dst[0] = '\0';
        return;
    }
    strncpy(dst, src, n - 1);
    dst[n - 1] = '\0';
}

// cxax_cf_string_utf8 copies a CF value into a C buffer when the value IS a
// string, and leaves the buffer empty when it is not.
//
// **The blind cast is the hazard, and it took the daemon down.** An AX
// attribute is not promised to be a string: kAXTitleAttribute answers an
// NSAttributedString for an app that gives its windows rich titles, and an
// app is free to answer with a number, a boolean, or an array. Casting that
// to CFStringRef and handing it to CFStringGetCStringPtr does not produce a
// wrong answer — it reads the object's memory as a string's internal layout,
// which is a wild pointer. Measured: SIGBUS, "signal arrived during cgo
// execution", in cxax_app_titles, on a machine where a window belonged to an
// app that answered with something that is not a string. The whole daemon
// went down with it.
//
// So the type is CHECKED, and the converting call is used rather than the fast
// one. CFStringGetCStringPtr answers NULL for any string its fast path cannot
// represent — an emoji, a combining mark, a CJK title — and NULL is
// indistinguishable from "this is not a title", so a title the fast path
// declines would be dropped from the list. CFStringGetCString converts those.
//
// Every string the bridge reads goes through here.
static void cxax_cf_string_utf8(CFTypeRef value, char *dst, size_t n) {
    if (n == 0) {
        return;
    }
    if (value == NULL || CFGetTypeID(value) != CFStringGetTypeID()) {
        dst[0] = '\0';
        return;
    }
    CFStringRef s = (CFStringRef)value;
    const char *fast = CFStringGetCStringPtr(s, kCFStringEncodingUTF8);
    if (fast != NULL) {
        cxax_copy_string(fast, dst, n);
        return;
    }
    if (CFStringGetCString(s, dst, (CFIndex)n, kCFStringEncodingUTF8)) {
        dst[n - 1] = '\0';
        return;
    }
    dst[0] = '\0';
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
// The AX↔wid bridge is one-directional: the AX plane has no wid→element
// routine, so an element is acquired by reading an app's windows once and
// keying what comes back by the window's FRAME, then cached. The per-app
// acquisition count is exposed rather than assumed, because "one batched read
// per app" is a claim that rots silently the moment a loop moves the read
// inside it.
//
// **The frame is the key because the private API that used to be is a fault
// waiting to happen.** `_AXUIElementGetWindow()` is a private HIServices
// function that assumes the element answers the "_AXUIElementGetWindow"
// attribute and dereferences the result without checking it. Measured on this
// macOS: it faulted with SIGBUS — "signal arrived during cgo execution" — on
// element after element, and every fault took the whole daemon down with it.
// The public attribute read is no help: it answers
// kAXErrorAttributeUnavailable (-25205) for every element, ordinary
// application windows included.
//
// The frame needs no private anything. The Accessibility plane reports a
// window's frame in the same coordinates CGWindowList reports its bounds, and
// measured on this machine they are the same four numbers to the point: an AX
// window at (962, 579) sized 930x728, and the physical row wid=38 at x=962
// y=579 w=930 h=728. The two planes are joined by a frame.
//
// A window list is worth less than a daemon, and a public frame is worth more
// than either.

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
            cxax_cf_string_utf8(CFDictionaryGetValue(info, kCGWindowName),
                                 row.title, sizeof(row.title));
            row.minimized = 0; // decoded in Go, where the table test lives
            yield(&row, ctx);
        }
        CFRelease(list);
    }
}

// CXAX_FRAME_TOLERANCE is how far a frame may differ and still be the same
// window, summed across the four numbers.
//
// A point each, which is the width of a rounding error and not a tolerance
// that could match two different windows: the measured AX frame and the
// measured CG bounds are the same four numbers to the point.
#define CXAX_FRAME_TOLERANCE 4.0

typedef struct {
    pid_t pid;
    const char *title;
    // The frame to match, when the title cannot be. See `cxax_window_id`.
    double x, y, w, h;
    unsigned int found;
    unsigned int fallback;
} cxax_id_ctx;

static void cxax_id_yield(const cxax_wrow *row, void *vctx) {
    cxax_id_ctx *c = (cxax_id_ctx *)vctx;
    if ((pid_t)row->pid != c->pid) {
        return;
    }
    // The FRAME is matched first, because it is the more specific of the two
    // and it is the one that always exists on both sides: without Screen
    // Recording, kCGWindowName comes back empty for every window, and a
    // title match against an empty string matches nothing at all.
    if (c->w > 0 && c->h > 0) {
        double score = fabs(row->x - c->x) + fabs(row->y - c->y) +
                       fabs(row->w - c->w) + fabs(row->h - c->h);
        if (score <= CXAX_FRAME_TOLERANCE) {
            c->found = row->window_id;
            return;
        }
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
//
// `x`, `y`, `w`, `h` are the window's frame, and they are matched when the
// title cannot be. See `cxax_id_yield`.
static unsigned int cxax_window_id(pid_t pid, const char *title, double x,
                                   double y, double w, double h) {
    cxax_id_ctx c;
    c.pid = pid;
    c.title = title;
    c.x = x;
    c.y = y;
    c.w = w;
    c.h = h;
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
    // The window ids already yielded, so the second walk can add only the
    // rows the first one did not carry.
    unsigned int *seen;
    int seenCount;
    int seenCap;
} cxax_fill_ctx;

// cxax_seen_add remembers a window id, growing the set when it is full.
static void cxax_seen_add(cxax_fill_ctx *c, unsigned int wid) {
    if (c->seenCount >= c->seenCap) {
        int cap = c->seenCap == 0 ? 256 : c->seenCap * 2;
        unsigned int *grown =
            (unsigned int *)realloc(c->seen, (size_t)cap * sizeof(unsigned int));
        if (grown == NULL) {
            // The set stays as it is. A window id that is not remembered is
            // a window that may be yielded twice, which is a duplicate row —
            // far better than a walk that cannot finish.
            return;
        }
        c->seen = grown;
        c->seenCap = cap;
    }
    c->seen[c->seenCount++] = wid;
}

// cxax_seen_has reports whether a window id was already yielded.
static int cxax_seen_has(cxax_fill_ctx *c, unsigned int wid) {
    for (int i = 0; i < c->seenCount; i++) {
        if (c->seen[i] == wid) {
            return 1;
        }
    }
    return 0;
}

static void cxax_fill_yield(const cxax_wrow *row, void *vctx) {
    cxax_fill_ctx *c = (cxax_fill_ctx *)vctx;
    if (c->out != NULL && c->n < c->max) {
        c->out[c->n] = *row;
    }
    c->n++;
    cxax_seen_add(c, row->window_id);
}

// cxax_fill_new_yield yields a row the set has not seen: the windows the
// on-screen list does not carry.
static void cxax_fill_new_yield(const cxax_wrow *row, void *vctx) {
    cxax_fill_ctx *c = (cxax_fill_ctx *)vctx;
    if (cxax_seen_has(c, row->window_id)) {
        return;
    }
    cxax_fill_yield(row, vctx);
}

// cxax_list_windows walks the full window set — not just the on-screen one, so
// a minimized window is a row rather than a gap. Pass a NULL out to count, the
// same two-pass shape the app list uses, so no window is dropped by a cap and
// no second call is needed to tell "full" from "complete".
//
// **THE ON-SCREEN LIST COMES FIRST, because it is the only list that carries
// a focus order.** The WindowServer answers it front to back; the ALL list
// does not, and a row read from it answers "where is this window" but not
// "what is in front of it".
//
// Measured on this machine, with the system saying Finder was frontmost: the
// on-screen list answers 38 (Finder) then 1759 (Docker) then 97 (WezTerm) —
// it agrees with the system. The ALL list answers 1759 first and puts
// Finder twenty-two rows down, among the cursor views and the menu strips,
// and it does not even carry the on-screen windows in a stable order.
//
// That order is the switcher's focus order: the row's position in this walk
// is its MRU rank, and the rank is what the switcher sorts by and what the
// "current window" is read from. So the walk order has to come from the
// list that has one.
//
// The ALL list follows, for the windows the on-screen list does not carry —
// which is where a minimized window lives.
int cxax_list_windows(cxax_wrow *out, int max) {
    cxax_fill_ctx c;
    c.out = out;
    c.max = max;
    c.n = 0;
    c.seen = NULL;
    c.seenCount = 0;
    c.seenCap = 0;
    cxax_walk_windows(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements,
                      cxax_fill_yield, &c);
    cxax_walk_windows(kCGWindowListOptionAll | kCGWindowListExcludeDesktopElements,
                      cxax_fill_new_yield, &c);
    free(c.seen);
    return c.n;
}

// cxax_onscreen_window_ids writes the ids of the on-screen normal-layer
// windows into `out`, in the WindowServer's own front-to-back order, and
// returns how many there are.
//
// **This is the order the whole focus-order design rests on, read on its
// own.** The on-screen list is the only one the WindowServer answers front
// to back; the ALL list interleaves the windows of every space, the cursor
// views and the menu strips, and answers "where is this window" but not
// "what is in front of it".
//
// It is a separate read on purpose: the merged walk that builds the rows
// and this one must be able to disagree, or the order cannot be checked
// against anything.
int cxax_onscreen_window_ids(unsigned int *out, int max) {
    @autoreleasepool {
        CFArrayRef list = CGWindowListCopyWindowInfo(
            kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements,
            kCGNullWindowID);
        if (list == NULL) {
            return 0;
        }
        CFIndex n = CFArrayGetCount(list);
        int count = 0;
        for (CFIndex i = 0; i < n; i++) {
            CFDictionaryRef info = (CFDictionaryRef)CFArrayGetValueAtIndex(list, i);
            if (info == NULL || CFGetTypeID(info) != CFDictionaryGetTypeID()) {
                continue;
            }
            if (cxax_cf_int(info, kCGWindowLayer, -1) != 0) {
                continue;
            }
            if (out != NULL && count < max) {
                out[count] = (unsigned int)cxax_cf_int(info, kCGWindowNumber, 0);
            }
            count++;
        }
        CFRelease(list);
        return count;
    }
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
// The number of windows the last read was answering for, per app. A change
// is what says the app's window set moved.
static int cxax_read_windows[ CXAX_READ_SLOTS ];
static int cxax_read_slots = 0;
static int cxax_read_total = 0;

static void cxax_record_windows_read(pid_t pid, int windows) {
    cxax_read_total++;
    for (int i = 0; i < cxax_read_slots; i++) {
        if (cxax_read_pid[i] == (int)pid) {
            cxax_read_count[i]++;
            // A caller that does not know the count (a focus, which names one
            // window) leaves the recorded count alone.
            if (windows >= 0) {
                cxax_read_windows[i] = windows;
            }
            return;
        }
    }
    if (cxax_read_slots < CXAX_READ_SLOTS) {
        cxax_read_pid[cxax_read_slots] = (int)pid;
        cxax_read_count[cxax_read_slots] = 1;
        cxax_read_windows[cxax_read_slots] = windows;
        cxax_read_slots++;
    }
}

// cxax_record_read_failed marks an app's window set as UNKNOWN.
//
// **A READ THAT DID NOT ANSWER IS NOT A COUNT.**
//
// The count recorded by a read is what says the app's window set has not
// moved, and that is what makes a repeated poll free. But a read that FAILED
// answered nothing: recording the count that was ASKED ABOUT would make the
// next poll believe the set has not moved, and the app's windows would never
// be named again.
//
// Measured on this machine: an app whose Accessibility server did not answer
// within the messaging bound left the daemon's window list empty for the
// life of the process — the Switcher page said there were no windows, with
// nothing wrong to see — and only a restart brought them back. The list was
// not empty because the machine had no windows; it was empty because one
// app was asked once and never asked again.
//
// So a failed read records -1, which no count can equal, and the next poll
// asks the app again.
static void cxax_record_read_failed(pid_t pid) {
    for (int i = 0; i < cxax_read_slots; i++) {
        if (cxax_read_pid[i] == (int)pid) {
            cxax_read_windows[i] = -1;
            return;
        }
    }
    if (cxax_read_slots < CXAX_READ_SLOTS) {
        cxax_read_pid[cxax_read_slots] = (int)pid;
        cxax_read_count[cxax_read_slots] = 0;
        cxax_read_windows[cxax_read_slots] = -1;
        cxax_read_slots++;
    }
}

// cxax_window_set_changed reports whether the number of windows the app is
// being asked about differs from the number the last read answered for.
//
// **This is what makes a repeat poll free, and it is a fact rather than a
// timer.** The physical plane lists surfaces — a 64x64 cursor view, a 1080x30
// strip — and the Accessibility plane has no element for any of them, so a
// request for one can never be satisfied. Re-reading the app to look for an
// element that was never going to be there would make every poll cost what
// the first one did.
//
// What says the app's window set has really moved is the COUNT changing: a
// window opened or closed, and the count of windows we are asking about goes
// with it. So a repeat poll over an unchanged set is free, and a new window
// is named on the next poll rather than on the next timer.
static int cxax_window_set_changed(pid_t pid, int windows) {
    for (int i = 0; i < cxax_read_slots; i++) {
        if (cxax_read_pid[i] == (int)pid) {
            return cxax_read_windows[i] != windows;
        }
    }
    return 1;
}

// cxax_elem_slot is one cached AX window element, keyed by the frame it was
// reported at and the process that owns it.
typedef struct {
    double x, y, w, h;
    pid_t pid;
    AXUIElementRef el;
} cxax_elem_slot;

static cxax_elem_slot cxax_elems[ CXAX_ELEM_SLOTS ];
static int cxax_elem_next = 0;

// cxax_elem_find returns the cached AX window element whose frame matches the
// given one, or NULL.
//
// **The frame IS the identity, and that is what replaced a private API.**
//
// The Accessibility plane reports a window's frame in the SAME coordinates
// CGWindowList reports its bounds. Measured on this machine: an AX window at
// (962, 579) sized 930x728, and the physical row wid=38 at x=962 y=579
// w=930 h=728 — the same four numbers. So the two planes are joined by a
// frame, with nothing private involved.
//
// The private API it replaces is the reason this file has a scar.
// "_AXUIElementGetWindow" is not exposed through the public attribute API —
// the read answers kAXErrorAttributeUnavailable (-25205) for every element,
// including ordinary application windows — and the private function that
// reads it dereferences a result it never checks. On this macOS it faulted
// with SIGBUS, "signal arrived during cgo execution", on element after
// element, and every fault took the whole daemon down with it. The window
// list was empty and the Switcher page said "Could not list windows."
//
// A window list is worth less than a daemon, and a public frame is worth more
// than either.
//
// The pid is part of the key: a window that appears under a different pid is
// a different window, and acting on the cached one would raise a window in an
// app the caller never named.
static AXUIElementRef cxax_elem_find(pid_t pid, double x, double y, double w,
                                      double h) {
    AXUIElementRef best = NULL;
    double bestScore = 0;
    for (int i = 0; i < CXAX_ELEM_SLOTS; i++) {
        if (cxax_elems[i].el == NULL || cxax_elems[i].pid != pid) {
            continue;
        }
        double score = fabs(cxax_elems[i].x - x) + fabs(cxax_elems[i].y - y) +
                       fabs(cxax_elems[i].w - w) + fabs(cxax_elems[i].h - h);
        if (best == NULL || score < bestScore) {
            best = cxax_elems[i].el;
            bestScore = score;
        }
    }
    if (best == NULL || bestScore > CXAX_FRAME_TOLERANCE) {
        return NULL;
    }
    return best;
}

static void cxax_elem_store(pid_t pid, double x, double y, double w, double h,
                             AXUIElementRef el) {
    int slot = -1;
    for (int i = 0; i < CXAX_ELEM_SLOTS; i++) {
        if (cxax_elems[i].el != NULL && cxax_elems[i].pid == pid &&
            cxax_elems[i].x == x && cxax_elems[i].y == y &&
            cxax_elems[i].w == w && cxax_elems[i].h == h) {
            // **The same object, released and retained in that order, is a
            // use-after-free.** Releasing first drops the count to zero and
            // deallocates the element; retaining it afterwards touches memory
            // that is gone. The AX server hands back the same element for a
            // window it has already given us, so "the same object" is the
            // ordinary case and not a rare one.
            //
            // The retain happens FIRST, so the object is alive across the
            // release, and the release is skipped entirely when it was the
            // same object — there is nothing to trade.
            if (cxax_elems[i].el == el) {
                return;
            }
            AXUIElementRef fresh = (AXUIElementRef)CFRetain(el);
            CFRelease(cxax_elems[i].el);
            cxax_elems[i].el = fresh;
            return;
        }
        if (cxax_elems[i].el == NULL && slot < 0) {
            slot = i;
        }
    }
    if (slot < 0) {
        slot = cxax_elem_next;
        cxax_elem_next = (cxax_elem_next + 1) % CXAX_ELEM_SLOTS;
    }
    // Retain BEFORE the release, for the same reason the matching branch
    // does: the new element may be the one being evicted, and releasing
    // first would deallocate it.
    AXUIElementRef fresh = (AXUIElementRef)CFRetain(el);
    if (cxax_elems[slot].el != NULL) {
        CFRelease(cxax_elems[slot].el);
    }
    cxax_elems[slot].x = x;
    cxax_elems[slot].y = y;
    cxax_elems[slot].w = w;
    cxax_elems[slot].h = h;
    cxax_elems[slot].pid = pid;
    cxax_elems[slot].el = fresh;
}

// cxax_frame_of reads an AX window element's frame. Declared before its
// definition because `cxax_acquire_app` uses it and the definition reads
// better next to the function that gives the frame its meaning.
static int cxax_frame_of(AXUIElementRef el, double *x, double *y, double *w,
                         double *h);

// cxax_acquire_app reads one app's window list and caches every element it
// returns, keyed by the frame the element reports.
// This is the ONE call into the app; everything after it is a lookup. Returns
// the number of elements cached, or 0 with *err set.
// `bound` is the messaging timeout the read is given, in seconds. It is a
// parameter because the two callers want different things: the list read
// asks every app once and must not be held up (0.15), while a FOCUS names
// one window and has nothing to show until that window is in hand (0.5).
static int cxax_acquire_app(pid_t pid, int windows, double bound, int *err) {
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
        cxax_record_windows_read(pid, windows);
        // **The call into the app is BOUNDED.**
        //
        // An AX attribute read is a message INTO the application that owns
        // the window, and an application that is slow to answer — or that
        // has stopped answering — holds the whole list up. Measured: listing
        // the windows on this machine took 6.26 SECONDS, and the shell's
        // client has one connection, so every page that asked for anything
        // after the Switcher page waited for it. The Activity page's table
        // was never drawn because its load had not landed when the picture
        // was taken.
        //
        // `AXUIElementSetMessagingTimeout` is the system's own bound, and it
        // is set on the ELEMENT so it covers the read that follows.
        AXUIElementSetMessagingTimeout(appEl, bound);
        CFTypeRef ref = NULL;
        AXError e = AXUIElementCopyAttributeValue(appEl, kAXWindowsAttribute, &ref);
        if (e != kAXErrorSuccess || ref == NULL) {
            // **A READ THAT FAILED IS NOT A COUNT.** The count this call
            // recorded says the app's window set has not moved, and the next
            // poll would believe it — so the app's windows would never be
            // named again, for the life of the process. The failure undoes
            // the count, so the next poll asks again.
            cxax_record_read_failed(pid);
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
            // **THE WINDOW ELEMENT IS BOUNDED TOO.**
            //
            // The timeout above covers the batched read on the application
            // element. Every call after it is made on the WINDOW element —
            // the frame read below, the title read, and the raise that a
            // focus performs — and a timeout is per element, so an
            // unbounded window element leaves those three unbounded.
            //
            // Measured: a focus that stalled on a busy application took
            // 3.24 SECONDS, and the switcher is on screen while it happens.
            // The bound is the same one the application element gets, and
            // the cost of hitting it is the same one the design already
            // pays: an application that cannot answer in time has its
            // windows left unnamed, and an unnamed window is not listed.
            AXUIElementSetMessagingTimeout(w, bound);
            // The element's FRAME is what identifies it, and it is read
            // publicly. The window id it used to be keyed by came from a
            // private function that faults; see `cxax_elem_find`.
            double fx, fy, fw, fh;
            if (!cxax_frame_of(w, &fx, &fy, &fw, &fh)) {
                continue;
            }
            // **Every element is cached, whatever its role.** The role was
            // used to keep only the windows, and it is not a reliable
            // filter: `kAXWindowsAttribute` answers with the CONTENT
            // element for an application whose window is a scroll area, and
            // the role came back "AXScrollArea" for windows that were
            // plainly there. Measured: filtering by role emptied the window
            // list — five titled windows became none.
            //
            // What keeps the cache honest is the FRAME: the match is exact,
            // so an element only answers for a physical row that has the
            // same frame, and the window it belongs to is the one with that
            // frame.
            cxax_elem_store(pid, fx, fy, fw, fh, w);
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

// cxax_frame_of reads an AX window element's frame, in the same coordinates
// CGWindowList reports its bounds.
//
// The type of each value is CHECKED before it is read, for the reason the
// string reads check theirs: an AX attribute is free to answer with something
// that is not an AXValue, and reading a foreign object as one is a wild call
// rather than a wrong answer.
static int cxax_frame_of(AXUIElementRef el, double *x, double *y, double *w,
                         double *h) {
    CGPoint pos = CGPointZero;
    CGSize size = CGSizeZero;
    CFTypeRef posRef = NULL;
    CFTypeRef sizeRef = NULL;
    if (AXUIElementCopyAttributeValue(el, kAXPositionAttribute, &posRef) !=
            kAXErrorSuccess ||
        posRef == NULL) {
        if (posRef != NULL) {
            CFRelease(posRef);
        }
        return 0;
    }
    if (AXUIElementCopyAttributeValue(el, kAXSizeAttribute, &sizeRef) !=
            kAXErrorSuccess ||
        sizeRef == NULL) {
        CFRelease(posRef);
        if (sizeRef != NULL) {
            CFRelease(sizeRef);
        }
        return 0;
    }
    if (CFGetTypeID(posRef) == AXValueGetTypeID()) {
        AXValueGetValue((AXValueRef)posRef, kAXValueCGPointType, &pos);
    }
    if (CFGetTypeID(sizeRef) == AXValueGetTypeID()) {
        AXValueGetValue((AXValueRef)sizeRef, kAXValueCGSizeType, &size);
    }
    CFRelease(posRef);
    CFRelease(sizeRef);
    *x = pos.x;
    *y = pos.y;
    *w = size.width;
    *h = size.height;
    // A window with no size is a surface, not a window, and it cannot be
    // matched to a physical row anyway.
    return size.width > 0 && size.height > 0;
}

// cxax_app_titles resolves the titles for up to max window ids, taking them
// off the elements the single batched read already returned. The title read
// is on an element in hand, not a fresh query into a busy app, which is the
// difference between this and one attribute read per window.
//
// `bounds` carries each window's frame, four doubles per window, in the same
// order as `wids` — the frame is how an AX element is matched to the
// physical row it belongs to. See `cxax_elem_find`.
int cxax_app_titles(pid_t pid, const unsigned int *wids, const double *bounds,
                    char *titles, int title_cap, int max, int *err) {
    if (err != NULL) {
        *err = -1;
    }
    if (max <= 0) {
        if (err != NULL) {
            *err = 0;
        }
        return 0;
    }
    // The app is read only for the windows we do not already hold an element
    // for. An app whose windows have not moved is therefore read ONCE, and a
    // second poll of the same list costs nothing — which is the budget the
    // design claims and the one the read counter measures.
    int missing = 0;
    for (int i = 0; i < max; i++) {
        if (cxax_elem_find(pid, bounds[i * 4], bounds[i * 4 + 1],
                           bounds[i * 4 + 2], bounds[i * 4 + 3]) == NULL) {
            missing = 1;
            break;
        }
    }
    // A failed acquisition is reported, not absorbed. The caller asked for
    // these windows' titles and could not get them; returning an empty set
    // would drop every window this app owns from the list while looking
    // exactly like an app that has none.
    //
    // The app is read only when it has not been read lately. A window that
    // missed is usually a surface with no Accessibility element at all, and
    // re-reading the app to look for one would make every poll cost what
    // the first one did.
    if (missing && cxax_window_set_changed(pid, max) &&
        cxax_acquire_app(pid, max, 0.15, err) == 0) {
        return 0;
    }
    @autoreleasepool {
        for (int i = 0; i < max; i++) {
            AXUIElementRef el = cxax_elem_find(pid, bounds[i * 4], bounds[i * 4 + 1],
                                               bounds[i * 4 + 2], bounds[i * 4 + 3]);
            char t[512];
            t[0] = '\0';
            if (el != NULL) {
                CFTypeRef tr = NULL;
                if (AXUIElementCopyAttributeValue(el, kAXTitleAttribute, &tr) ==
                        kAXErrorSuccess &&
                    tr != NULL) {
                    cxax_cf_string_utf8(tr, t, sizeof(t));
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

// cxax_wait_front waits for an application to become the frontmost one, for
// at most `budget` seconds, and reports whether it did.
//
// The wait is a RUN LOOP and not a sleep: the activation lands on the
// WindowServer's own turn, and the run loop is what lets that turn happen
// while this thread waits for it.
static int cxax_wait_front(pid_t pid, double budget) {
    NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:budget];
    while ([deadline timeIntervalSinceNow] > 0) {
        NSRunningApplication *front = [[NSWorkspace sharedWorkspace] frontmostApplication];
        if (front != nil && [front processIdentifier] == pid) {
            return 1;
        }
        [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.01]];
    }
    return 0;
}

// cxax_focus_window raises the window with the given frame and brings its
// application forward. The element comes from the cache when it is there and
// from the one batched read when it is not, so a repeated focus on the same
// window costs no call into the app at all.
//
// The frame is the identity, for the reason `cxax_elem_find` gives.
int cxax_focus_window(pid_t pid, double x, double y, double w, double h,
                      int *err) {
    if (err != NULL) {
        *err = -1;
    }
    AXUIElementRef el = cxax_elem_find(pid, x, y, w, h);
    if (el == NULL) {
        // **THE APP IS READ ONCE WITH THE LIST'S BOUND, AND AGAIN WITH MORE
        // ROOM IF IT DID NOT ANSWER.**
        //
        // The list bound is 0.15 seconds, and an application that is busy
        // does not answer inside it: the read comes back with
        // kAXErrorCannotComplete and the app's windows are not named. A
        // focus has nothing to show until that window is in hand, and the
        // user is waiting on it — so the second read is given the room the
        // list does not need.
        //
        // Measured on this machine: focusing a System Settings window
        // failed with "AXError -25205" (the element was never acquired)
        // because the read timed out at 0.15 seconds. The same window's
        // Accessibility read answered in well under that when the machine
        // was idle.
        int ignored = 0;
        if (cxax_acquire_app(pid, -1, 0.15, &ignored) == 0) {
            if (cxax_acquire_app(pid, -1, 0.5, &ignored) == 0) {
                if (err != NULL) {
                    *err = kAXErrorNoValue;
                }
                return 0;
            }
        }
        el = cxax_elem_find(pid, x, y, w, h);
    }
    if (el == NULL) {
        if (err != NULL) {
            *err = kAXErrorNoValue;
        }
        return 0;
    }
    @autoreleasepool {
        // **Raise, then activate, then wait for the activation to land.**
        //
        // The order was measured, not chosen. Raising a window of a
        // BACKGROUND application and then activating that application is
        // what worked; the other order — activate, wait, raise — left the
        // front window where it was on a machine where the raise reported
        // success. The raise is the action that moves the window inside the
        // application, and the activation is what brings the application
        // forward to show it.
        AXError raised = AXUIElementPerformAction(el, kAXRaiseAction);
        if (raised != kAXErrorSuccess) {
            // **A RAISE THAT HIT THE BOUND IS ASKED AGAIN, ONCE, WITH MORE
            // ROOM.**
            //
            // The bound is there so that one busy application cannot hold
            // the switcher's focus hostage — measured, a focus that waited
            // on a busy application took 3.24 SECONDS. But the bound is
            // also what turns a slow ANSWER into a failure: the AX server
            // answers a call that outruns the client's own messaging
            // timeout with kAXErrorCannotComplete (-25204), which is what
            // "we stopped waiting" looks like from this side.
            //
            // Measured on this machine: raising a background application's
            // window came back with -25204 about one time in fifteen, and
            // it came back in the bound's own time — the whole test that
            // failed took 0.20 SECONDS, which is the bound and not much
            // else. The application was busy, not refusing.
            //
            // So the raise is asked again, on a freshly read element, with
            // twice the room. A window that answers at all answers the
            // second time, and the price when it does not is bounded and
            // small: this is the last attempt, and it is 0.5 seconds.
            int ignored = 0;
            if (cxax_acquire_app(pid, -1, 0.5, &ignored) > 0) {
                AXUIElementRef retry = cxax_elem_find(pid, x, y, w, h);
                if (retry != NULL) {
                    AXUIElementSetMessagingTimeout(retry, 0.5);
                    AXError again = AXUIElementPerformAction(retry, kAXRaiseAction);
                    if (again == kAXErrorSuccess) {
                        // The window that answered is the window the rest of
                        // this function has to work with: the key window is
                        // set on it below.
                        el = retry;
                        raised = again;
                    }
                }
            }
        }
        // **AN APPLICATION THAT DOES NOT SUPPORT THE RAISE IS NOT A FAILED
        // FOCUS.**
        //
        // Measured on this machine: System Settings' window answers the raise
        // with kAXErrorAttributeUnsupported (-25205) — the action is not
        // supported by that element at all. The element is a real AXWindow
        // with the right title and the right frame; the application simply
        // does not take a raise.
        //
        // The focus is still possible. The app's key window is set to the
        // one that was asked for, and the app is activated, so the app comes
        // forward showing the window the user picked. Failing here would
        // leave the switcher unable to focus one application on the
        // machine — measured here: focusing the System Settings window
        // failed with "AXError -25205" every time.
        //
        // The raise's failure is still not swallowed: if the activation does
        // not land either, the focus did not happen, and the caller is told
        // so.
        int raiseUnsupported =
            (raised == kAXErrorAttributeUnsupported ||
             raised == kAXErrorActionUnsupported);
        if (raised != kAXErrorSuccess && !raiseUnsupported) {
            if (err != NULL) {
                *err = (int)raised;
            }
            return 0;
        }
        // **THE APP'S KEY WINDOW IS SET TO THE ONE THAT WAS ASKED FOR.**
        //
        // The raise orders an application's windows among themselves. It
        // does not say which of them the application brings forward when it
        // is activated, and that is a different window: an application with
        // several windows has a key window, and the activation puts THAT
        // one in front.
        //
        // Measured on this machine: Tailscale's settings window was raised
        // — the raise reported success — and the window that came forward
        // was not it. The window the user picked has to be the app's
        // focused window for the activation to bring it forward, so it is
        // set to be.
        {
            AXUIElementRef appEl = AXUIElementCreateApplication(pid);
            if (appEl != NULL) {
                // The same bound as every other call into an application.
                AXUIElementSetMessagingTimeout(appEl, 0.15);
                AXUIElementSetAttributeValue(appEl, kAXFocusedAttribute, el);
                CFRelease(appEl);
            }
        }
        NSRunningApplication *app =
            [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
        if (app != nil) {
            // No options: the ignore-other-apps flag was deprecated in macOS 14
            // and does nothing there, while an unconditional activation would
            // steal the front from an app the user pinned on top.
            [app activateWithOptions:0];
            // The activation is ASYNCHRONOUS: the app is asked to come
            // forward and the WindowServer does it on its own turn, so a
            // front window read straight afterwards reads the OLD front. The
            // wait is for the app to become front, and never longer than the
            // budget — the switcher is on screen while this happens.
            int landed = cxax_wait_front(pid, 0.4);
            if (!landed) {
                // **A REFUSED ACTIVATION IS ASKED AGAIN.**
                //
                // Measured on this machine: the activation is sometimes
                // swallowed. The window still comes to the front — the
                // raise above is what does that, and it happens before the
                // activation is asked for — but the application does not
                // become the frontmost one, so the keyboard stays where it
                // was. Asking again lands it.
                //
                // The budget is split rather than doubled, so the whole
                // wait is what it was: the user is waiting on the switcher
                // dismissing, and the window they asked for is already in
                // front.
                [app activateWithOptions:0];
                landed = cxax_wait_front(pid, 0.4);
            }
            if (!landed) {
                // **A REFUSAL IS REPORTED, NOT ABSORBED.**
                //
                // The application was asked to come forward and did not: the
                // keyboard stayed where it was. Saying "focused" would be a
                // lie the user finds out from their next keystroke going
                // somewhere else — and the caller cannot tell a refusal from
                // a success by the return value alone.
                //
                // The window IS in front — the raise did that — so this is
                // a partial answer, and it is the honest one: the keyboard
                // did not come over.
                //
                // The return value is 2, and not 0, so the caller can tell
                // this from an AX failure: the raise worked, the platform
                // did not hand the activation over.
                //
                // **UNLESS THE RAISE NEVER HAPPENED.** An application that
                // does not support the raise was never brought forward by it,
                // so saying "the window was raised" would be a lie. That case
                // is a failure, and it carries the AX error that said so.
                if (raiseUnsupported) {
                    if (err != NULL) {
                        *err = (int)raised;
                    }
                    return 0;
                }
                if (err != NULL) {
                    *err = 0;
                }
                return 2;
            }
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

// **The screen's lock state is NOT read from the session dictionary.**
//
// `CGSessionCopyCurrentDictionary()` is the public way to ask, and it was
// tried: the daemon hung on it. Measured — two threads stuck in it for the
// life of the process, the stack ending in
// `CGSLookupServerRootPort → bootstrap_look_up → mach_msg`, an XPC lookup
// that nothing was answering. The screen was locked at the time, and a
// locked screen is exactly when the answer would be wanted.
//
// A daemon that blocks on a system call to answer a question it can answer
// for free is worse than a daemon that never answers it, and the question
// has a free answer: the AX plane itself. A locked screen answers the
// focused-window query with the APPLICATION element — role AXApplication, a
// 0x0 frame — and an application element is never a window. That is read
// from the element already in hand, costs nothing, and cannot hang.

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

// cxax_windows_unanswered counts the applications whose last read did not
// answer, and whose windows are therefore owed another read.
//
// It is the counterpart of the read counter: a repeated poll costs one read
// for each of these and nothing for the rest, so the budget a test measures
// has to allow exactly this many — the reads that were OWED, not the reads
// that were wasted.
int cxax_windows_unanswered(void) {
    int n = 0;
    for (int i = 0; i < cxax_read_slots; i++) {
        if (cxax_read_windows[i] < 0) {
            n++;
        }
    }
    return n;
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
            cxax_cf_string_utf8(titleRef, out->title, sizeof(out->title));
            CFRelease(titleRef);
        }
        CFTypeRef roleRef = NULL;
        if (AXUIElementCopyAttributeValue(win, kAXRoleAttribute, &roleRef) ==
                kAXErrorSuccess &&
            roleRef != NULL) {
            cxax_cf_string_utf8(roleRef, out->role, sizeof(out->role));
            CFRelease(roleRef);
        }

        CGPoint pos = CGPointZero;
        CFTypeRef posRef = NULL;
        if (AXUIElementCopyAttributeValue(win, kAXPositionAttribute, &posRef) ==
                kAXErrorSuccess &&
            posRef != NULL) {
            // The type is checked for the same reason the string's is: an AX
            // attribute is free to answer with something that is not an AXValue,
            // and AXValueGetValue on a foreign object is a wild call rather than
            // a wrong answer.
            if (CFGetTypeID(posRef) == AXValueGetTypeID()) {
                AXValueGetValue((AXValueRef)posRef, kAXValueCGPointType, &pos);
            }
            CFRelease(posRef);
        }
        CGSize size = CGSizeZero;
        CFTypeRef sizeRef = NULL;
        if (AXUIElementCopyAttributeValue(win, kAXSizeAttribute, &sizeRef) ==
                kAXErrorSuccess &&
            sizeRef != NULL) {
            if (CFGetTypeID(sizeRef) == AXValueGetTypeID()) {
                AXValueGetValue((AXValueRef)sizeRef, kAXValueCGSizeType, &size);
            }
            CFRelease(sizeRef);
        }
        out->x = pos.x;
        out->y = pos.y;
        out->w = size.width;
        out->h = size.height;

        out->window_id = cxax_window_id((pid_t)out->pid, out->title, out->x,
                                        out->y, out->w, out->h);

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

// cxax_window_action performs the named AX action on the frontmost app's
// focused window. Action values are the same numbers Go sends:
// CXAX_WIN_CLOSE, CXAX_WIN_MINIMIZE, CXAX_WIN_MAXIMIZE.
//
// Close and minimize are presses on the window's own button attributes —
// the same events the user's mouse delivers to the red and yellow buttons,
// so each application's close/minimize behavior (sheets, confirmation
// prompts, dock animation) is whatever it already is. Maximize is a
// geometry set to the screen's visible frame, not the zoom button: the
// green button toggles full-screen on apps that answer the press with the
// fullscreen transition, and a shortcut named "maximize" must not enter
// it. Returns 1 on success; on failure *err carries the AXError, the same
// shape cxax_move_resize answers with so the two share Go's error mapping.
int cxax_window_action(int action_tag, int *err) {
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

        AXError e = kAXErrorSuccess;

        if (action_tag == CXAX_WIN_MINIMIZE) {
            // The AX button press carries the window's own minimize
            // rule (dock animation and integration), which is what a
            // user pressing the yellow button gets.
            CFTypeRef btnRef = NULL;
            AXError be = AXUIElementCopyAttributeValue(
                win, kAXMinimizeButtonAttribute, &btnRef);
            if (be != kAXErrorSuccess || btnRef == NULL ||
                CFGetTypeID(btnRef) != AXUIElementGetTypeID()) {
                e = (be != kAXErrorSuccess) ? be : kAXErrorNoValue;
            } else {
                e = AXUIElementPerformAction((AXUIElementRef)btnRef,
                                             kAXPressAction);
                CFRelease(btnRef);
            }
        } else if (action_tag == CXAX_WIN_MAXIMIZE) {
            // Maximize/zoom: the window's own geometry, set to the frame of
            // the screen it is on. Not the zoom button: the green button
            // toggles full-screen on apps that answer the press with the
            // fullscreen transition, and a shortcut named "maximize" must
            // not enter it. Same shape as window.move, so the seam has one
            // maximize mechanism.
            //
            // Which screen: the one that contains the window's center. The
            // daemon has no windows of its own, so NSScreen.mainScreen is
            // the daemon's screen and not the window's — measured on a
            // two-display desk, the daemon's fill target and the window's
            // screen were different screens, and the window teleported
            // rather than maximizing.
            CGPoint pos = CGPointZero;
            CGSize size = CGSizeZero;
            CFTypeRef fpos = NULL;
            CFTypeRef fsize = NULL;
            int sized = 0;
            if (AXUIElementCopyAttributeValue(win, kAXPositionAttribute,
                                              &fpos) == kAXErrorSuccess &&
                fpos != NULL) {
                if (CFGetTypeID(fpos) == AXValueGetTypeID()) {
                    AXValueGetValue((AXValueRef)fpos, kAXValueCGPointType,
                                    &pos);
                }
                CFRelease(fpos);
            }
            if (AXUIElementCopyAttributeValue(win, kAXSizeAttribute,
                                              &fsize) == kAXErrorSuccess &&
                fsize != NULL) {
                if (CFGetTypeID(fsize) == AXValueGetTypeID()) {
                    AXValueGetValue((AXValueRef)fsize, kAXValueCGSizeType,
                                    &size);
                }
                CFRelease(fsize);
            }
            NSScreen *screen = nil;
            if (size.width > 0 && size.height > 0) {
                CGPoint center = CGPointMake(pos.x + size.width / 2,
                                             pos.y + size.height / 2);
                for (NSScreen *s in [NSScreen screens]) {
                    if (CGRectContainsPoint(s.frame, center)) {
                        screen = s;
                        break;
                    }
                }
            }
            if (screen == nil) {
                screen = [NSScreen screens].firstObject;
            }
            if (screen == nil) {
                e = kAXErrorCannotComplete;
            } else {
                NSRect visible = [screen visibleFrame];
                CGPoint target = CGPointMake(visible.origin.x,
                                             visible.origin.y);
                CGSize tsize = CGSizeMake(visible.size.width,
                                          visible.size.height);
                AXValueRef posV = AXValueCreate(kAXValueCGPointType, &target);
                AXValueRef sizeV =
                    AXValueCreate(kAXValueCGSizeType, &tsize);
                if (posV == NULL || sizeV == NULL) {
                    e = kAXErrorFailure;
                } else {
                    AXError pe = AXUIElementSetAttributeValue(
                        win, kAXPositionAttribute, posV);
                    if (pe == kAXErrorSuccess) {
                        pe = AXUIElementSetAttributeValue(
                            win, kAXSizeAttribute, sizeV);
                    }
                    e = pe;
                }
                if (posV != NULL) {
                    CFRelease(posV);
                }
                if (sizeV != NULL) {
                    CFRelease(sizeV);
                }
            }
        } else if (action_tag == CXAX_WIN_CLOSE) {
            CFTypeRef btnRef = NULL;
            AXError be = AXUIElementCopyAttributeValue(
                win, kAXCloseButtonAttribute, &btnRef);
            if (be != kAXErrorSuccess || btnRef == NULL ||
                CFGetTypeID(btnRef) != AXUIElementGetTypeID()) {
                e = (be != kAXErrorSuccess) ? be : kAXErrorNoValue;
            } else {
                e = AXUIElementPerformAction((AXUIElementRef)btnRef,
                                             kAXPressAction);
                CFRelease(btnRef);
            }
        } else {
            e = kAXErrorActionUnsupported;
        }

        CFRelease(win);
        CFRelease(appEl);
        if (e != kAXErrorSuccess) {
            if (err != NULL) {
                *err = (int)e;
            }
            return 0;
        }
        if (err != NULL) {
            *err = 0;
        }
        return 1;
    }
}
