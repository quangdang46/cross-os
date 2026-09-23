// CrossOS macOS tap bridge — thin C surface over CGEventTap (native tap
// bridge bead; plan §4 + COMPREHENSIVE_PLAN.md:671 input-path box).
//
// Adapted shape (not forked — written from the CGEvent.h API + the spike A
// contract, mirroring the Karabiner event_tap_monitor.hpp pattern named in
// platform/darwin/spike_a/doc.go): tap at kCGHIDEventTap, head-insert,
// timeout-disable recovery owned by the Go caller.
//
// Authority: this file EXECUTES (tap create/enable/post). Core DECIDES —
// the callback classifies via the Go decide entry and returns suppress or
// pass; no key policy lives here. Injection uses CGEventCreateKeyboardEvent
// + CGEventPost, async handoff only (never inside the callback — spike A/B
// reentrancy rule).
//
// Build: compiled by cgo via core/internal/adapter/tap_cgo.go (#cgo
// CFLAGS/LDFLAGS in that file). TCC input-monitoring consent is RUNTIME —
// CGEventTapCreate returns NULL without it; the bridge reports NULL, never
// a fake handle.

#include <CoreGraphics/CoreGraphics.h>
#include <stdlib.h>

// cxtap_cb_t: Go decide entry. Returns 1 = suppress (return NULL),
// 0 = pass (return event). Matches adapter.KeyAction semantics for the two
// paths the tap owns (inject travels the async post path, never the return).
typedef int (*cxtap_cb_t)(uint16_t keycode, uint64_t flags, int keydown,
                          void *ctx);

// Tap record: owned by Go (created in tap_create, freed in tap_destroy).
typedef struct {
    CFMachPortRef tap;
    CFRunLoopSourceRef source;
    CFRunLoopRef loop; // owning run loop, so stop() from another thread works
    cxtap_cb_t cb;
    void *ctx;
} cxtap_t;

// Forward: the CGEventTap callback. Thin by contract: classify via Go,
// return the verdict. Timeout-disable arrives as a separate event type
// the Go driver handles via its Recovery loop (spike A recovery.go).
static CGEventRef cxtap_proc(CGEventTapProxy proxy, CGEventType type,
                             CGEventRef event, void *refcon) {
    (void)proxy;
    cxtap_t *t = (cxtap_t *)refcon;
    if (type == kCGEventTapDisabledByTimeout) {
        // The callback overran the system budget, so the tap is now OFF.
        // Hand recovery to Go (bounded re-enable loop + health escalation);
        // returning here keeps this callback short, which is the whole
        // point — a slow recovery callback is what caused the disable.
        extern void crossosGoTapDisabled(void);
        crossosGoTapDisabled();
        return event;
    }
    if (type != kCGEventKeyDown && type != kCGEventKeyUp) {
        return event;
    }
    if (event == NULL) {
        return event;
    }
    uint16_t keycode = (uint16_t)CGEventGetIntegerValueField(
        event, kCGKeyboardEventKeycode);
    uint64_t flags = (uint64_t)CGEventGetFlags(event);
    int keydown = (type == kCGEventKeyDown) ? 1 : 0;
    // Go decide entry (exported from tap_cgo.go via //export).
    // Declared here; resolved at link time by cgo.
    extern int crossosGoDecide(uint16_t keycode, uint64_t flags, int keydown,
                               void *ctx);
    int suppress = crossosGoDecide(keycode, flags, keydown, t->ctx);
    if (suppress) {
        return NULL;
    }
    return event;
}

// cxtap_create installs the tap at kCGHIDEventTap, head-insert.
// Returns NULL on failure (no TCC consent, or tap refused) — Go maps NULL
// to the typed not-wired/denied error, never success.
static cxtap_t *cxtap_create(cxtap_cb_t cb, void *ctx) {
    cxtap_t *t = (cxtap_t *)calloc(1, sizeof(cxtap_t));
    if (t == NULL) {
        return NULL;
    }
    t->cb = cb;
    t->ctx = ctx;
    t->tap = CGEventTapCreate(kCGHIDEventTap, kCGHeadInsertEventTap,
                              kCGEventTapOptionDefault,
                              CGEventMaskBit(kCGEventKeyDown) |
                                  CGEventMaskBit(kCGEventKeyUp),
                              cxtap_proc, t);
    if (t->tap == NULL) {
        free(t);
        return NULL;
    }
    t->source = CFMachPortCreateRunLoopSource(NULL, t->tap, 0);
    if (t->source == NULL) {
        CFMachPortInvalidate(t->tap);
        CFRelease(t->tap);
        free(t);
        return NULL;
    }
    CFRunLoopAddSource(CFRunLoopGetCurrent(), t->source,
                       kCFRunLoopCommonModes);
    t->loop = CFRunLoopGetCurrent();
    CGEventTapEnable(t->tap, true);
    return t;
}

// cxtap_run pumps the tap's run loop until cxtap_stop. CGEventTap delivers
// callbacks only while its run loop runs, so the Go owner pins this to a
// dedicated locked OS thread and calls it there — the thread-local run loop
// the source was attached to in cxtap_create.
static void cxtap_run(cxtap_t *t) {
    (void)t;
    CFRunLoopRun();
}

// cxtap_stop wakes cxtap_run from any thread via the stored loop ref
// (CFRunLoopGetCurrent would be the wrong loop off-thread).
static void cxtap_stop(cxtap_t *t) {
    if (t != NULL && t->loop != NULL) {
        CFRunLoopStop(t->loop);
    }
}

// cxtap_enable re-enables after kCGEventTapDisabledByTimeout
// (Go Recovery loop owns the backoff + escalation).
static void cxtap_enable(cxtap_t *t) {
    if (t != NULL && t->tap != NULL) {
        CGEventTapEnable(t->tap, true);
    }
}

// cxtap_destroy removes the tap. NULL-safe.
static void cxtap_destroy(cxtap_t *t) {
    if (t == NULL) {
        return;
    }
    if (t->source != NULL) {
        CFRunLoopRemoveSource(CFRunLoopGetCurrent(), t->source,
                              kCFRunLoopCommonModes);
        CFRelease(t->source);
    }
    if (t->tap != NULL) {
        CGEventTapEnable(t->tap, false);
        CFMachPortInvalidate(t->tap);
        CFRelease(t->tap);
    }
    free(t);
}

// cxpost_f9 posts a synthetic F9 keydown+keyup pair (the spike A/B
// replacement target — never Ctrl+C, so no self-loop; injectTag reserved
// for the wge self-ignore convention). Async handoff only: Go calls this
// off-callback after a suppress+replace verdict.
static void cxpost_f9(void) {
    CGEventSourceRef src = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
    if (src == NULL) {
        return;
    }
    CGEventRef down = CGEventCreateKeyboardEvent(src, (CGKeyCode)0x65, true);
    CGEventRef up = CGEventCreateKeyboardEvent(src, (CGKeyCode)0x65, false);
    if (down != NULL) {
        CGEventPost(kCGHIDEventTap, down);
        CFRelease(down);
    }
    if (up != NULL) {
        CGEventPost(kCGHIDEventTap, up);
        CFRelease(up);
    }
    CFRelease(src);
}
