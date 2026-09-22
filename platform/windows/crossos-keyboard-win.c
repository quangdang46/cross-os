// crossos-keyboard-win.dll — thin Windows native helper (bead cross-os-qhp.8).
//
// Plan §12 build commands: CMake builds JUST this thin DLL — hook + Raw
// Input observe + SendInput, nothing more. All decision logic stays in Go,
// reached via C ABI from core/internal/adapter (the frozen surface is
// documented in core/internal/adapter/dll_windows.go; this file implements
// exactly those 5 exports, nothing more).
//
// Proven shape: platform/windows/spike_b (bead cross-os-ssj) proved
// WH_KEYBOARD_LL suppress + SendInput replacement + UIPI probe in-test;
// this DLL is the product seam for that shape (Tier-1 green, Tier-2 green
// on real Windows input routing as of the waitKeyNonModifier fix).
//
// Build (MSVC, verified on BuildTools 18 / 14.51):
//
//   cl /LD crossos-keyboard-win.c user32.lib /Fecrossos-keyboard-win.dll
//
// or via CMake: cd platform/windows && cmake . && cmake --build .
// (CMakeLists.txt in this directory; same single-file build, no vcxproj.)

#include <windows.h>

// --- CrossOS_HookInstall / CrossOS_HookUninstall ---
//
// WH_KEYBOARD_LL hook, thin callback: the real product callback forwards
// to Go via the C-ABI decide entry owned by core/internal/adapter. This
// translation unit keeps a pass-through install/uninstall pair so the
// loader seam (LoadLibrary + GetProcAddress) resolves and the hook
// lifecycle is smoke-testable without Go linked in. Decision logic NEVER
// lives here — Core decides, this DLL executes.

static HHOOK g_hook = NULL;

static LRESULT CALLBACK CrossOS_LLProc(int nCode, WPARAM wParam, LPARAM lParam) {
    if (nCode == HC_ACTION) {
        // Thin path: forward to the next hook. The Go-side decide entry
        // (wired by the adapter at runtime) replaces this forward with
        // classify-then-suppress-or-forward once linked; until then the
        // DLL is observably inert, never silently suppressive.
        return CallNextHookEx(g_hook, nCode, wParam, lParam);
    }
    return CallNextHookEx(g_hook, nCode, wParam, lParam);
}

__declspec(dllexport) HHOOK CrossOS_HookInstall(void) {
    if (g_hook != NULL) {
        return g_hook;
    }
    g_hook = SetWindowsHookExW(WH_KEYBOARD_LL, CrossOS_LLProc, GetModuleHandleW(NULL), 0);
    return g_hook;
}

__declspec(dllexport) void CrossOS_HookUninstall(HHOOK handle) {
    if (handle != NULL) {
        UnhookWindowsHookEx(handle);
    }
    if (handle == g_hook || handle == NULL) {
        g_hook = NULL;
    }
}

// --- CrossOS_SendInput ---
//
// Thin SendInput wrapper: synthesizes one key event (vk + flags) and
// returns the accepted count. UIPI-blocked injections report 0 (+ last
// error via GetLastError for the caller); the CALLER owns the
// passthrough+notice fallback (plan §4.2, spike B fallback contract) —
// this function never decides, never retries, never drops silently.

__declspec(dllexport) UINT CrossOS_SendInput(WORD vk, DWORD flags) {
    INPUT in;
    ZeroMemory(&in, sizeof(in));
    in.type = INPUT_KEYBOARD;
    in.ki.wVk = vk;
    in.ki.dwFlags = flags;
    return SendInput(1, &in, sizeof(in));
}

// --- CrossOS_UIPIStatus ---
//
// Elevation/reachability bits for the spike-B UIPI contract: bit 0 set =
// current process is elevated (CheckTokenMembership against the
// Administrators SID, same probe as spike_b's isElevated); bit 1 reserved.
// Callers record result + error code and fall back to passthrough+notice
// on a block — never a silent drop.

__declspec(dllexport) DWORD CrossOS_UIPIStatus(void) {
    DWORD status = 0;
    HANDLE token = NULL;
    if (OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &token)) {
        TOKEN_ELEVATION elevation;
        DWORD outLen = 0;
        if (GetTokenInformation(token, TokenElevation, &elevation,
                                sizeof(elevation), &outLen) &&
            elevation.TokenIsElevated) {
            status |= 0x1;
        }
        CloseHandle(token);
    }
    return status;
}

// --- CrossOS_DeviceId ---
//
// Raw Input device-identity query (observe-only path — plan §4.2: Raw Input
// is observation/device identity ONLY, never a suppression mechanism).
// Returns the count of attached keyboards; identity enumeration rides the
// same GetRawInputDeviceList call the product adapter uses. Zero devices
// (or a failed enumeration) returns 0 — the caller treats it as
// "unknown", never as an error to act on.

__declspec(dllexport) DWORD CrossOS_DeviceId(void) {
    UINT count = 0;
    if (GetRawInputDeviceList(NULL, &count, sizeof(RAWINPUTDEVICELIST)) != 0) {
        return 0;
    }
    if (count == 0) {
        return 0;
    }
    PRAWINPUTDEVICELIST list =
        (PRAWINPUTDEVICELIST)HeapAlloc(GetProcessHeap(), 0,
                                       count * sizeof(RAWINPUTDEVICELIST));
    if (list == NULL) {
        return 0;
    }
    DWORD keyboards = 0;
    if (GetRawInputDeviceList(list, &count, sizeof(RAWINPUTDEVICELIST)) != (UINT)-1) {
        for (UINT i = 0; i < count; i++) {
            if (list[i].dwType == RIM_TYPEKEYBOARD) {
                keyboards++;
            }
        }
    }
    HeapFree(GetProcessHeap(), 0, list);
    return keyboards;
}
