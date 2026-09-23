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
// CGWindowList is used only to report a CGWindowID for the focused window
// (window identity for logs/UI); move/resize targets the AX focused window,
// which is the same window — CGWindowID is not an AX handle.

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

// cxax_window_id reports the CGWindowID of the frontmost app's on-screen
// window whose title matches, falling back to its first normal-layer
// window. Purely an identifier for logs/UI — never the move target.
static unsigned int cxax_window_id(pid_t pid, const char *title) {
    CFArrayRef list = CGWindowListCopyWindowInfo(
        kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements,
        kCGNullWindowID);
    if (list == NULL) {
        return 0;
    }
    unsigned int found = 0;
    CFIndex n = CFArrayGetCount(list);
    for (CFIndex i = 0; i < n; i++) {
        CFDictionaryRef info = CFArrayGetValueAtIndex(list, i);
        if (info == NULL || CFGetTypeID(info) != CFDictionaryGetTypeID()) {
            continue;
        }
        CFNumberRef owner = CFDictionaryGetValue(info, kCGWindowOwnerPID);
        int opid = 0;
        if (owner != NULL) {
            CFNumberGetValue(owner, kCFNumberIntType, &opid);
        }
        if (opid != (int)pid) {
            continue;
        }
        // kCGWindowLayer is a CFStringRef KEY; its value is a CFNumberRef.
        CFTypeRef layer = CFDictionaryGetValue(info, kCGWindowLayer);
        int l = 0;
        if (layer != NULL && CFGetTypeID(layer) == CFNumberGetTypeID()) {
            CFNumberGetValue((CFNumberRef)layer, kCFNumberIntType, &l);
        }
        if (l != 0) { // normal windows only (skipmenus, shadows, …)
            continue;
        }
        CFStringRef name = CFDictionaryGetValue(info, kCGWindowName);
        char cname[512];
        cxax_copy_string(name == NULL ? NULL
                                   : CFStringGetCStringPtr(name, kCFStringEncodingUTF8),
                         cname, sizeof(cname));
        if (title != NULL && title[0] != '\0' && strcmp(cname, title) == 0) {
            CFNumberRef wid = CFDictionaryGetValue(info, kCGWindowNumber);
            int64_t id = 0;
            if (wid != NULL) {
                CFNumberGetValue(wid, kCFNumberSInt64Type, &id);
            }
            found = (unsigned int)id;
            break;
        }
        if (found == 0) {
            CFNumberRef wid = CFDictionaryGetValue(info, kCGWindowNumber);
            int64_t id = 0;
            if (wid != NULL && CFNumberGetValue(wid, kCFNumberSInt64Type, &id)) {
                found = (unsigned int)id;
            }
        }
    }
    CFRelease(list);
    return found;
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
