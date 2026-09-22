# crossos-keyboard-win.dll — thin Windows native helper (bead cross-os-qhp.8).
#
# Plan §12: "cd platform/windows && cmake . && make → crossos-keyboard-win.dll".
# Builds JUST the thin DLL (hook + Raw Input observe + SendInput, nothing
# more); all decision logic stays in Go via C ABI from core/internal/adapter
# (frozen surface in core/internal/adapter/dll_windows.go).
#
# Single-file build, no vcxproj: verified with MSVC BuildTools 18 (14.51)
# and MinGW fallback. Signing the DLL/MSI stays cert-gated in qhp.6 —
# this file only builds + links, never signs.
cmake_minimum_required(VERSION 3.20)

project(crossos-keyboard-win C)

add_library(crossos-keyboard-win SHARED crossos-keyboard-win.c)

# user32: SetWindowsHookEx/CallNextHookEx/SendInput; advapi32: token APIs.
target_link_libraries(crossos-keyboard-win PRIVATE user32 advapi32)

set_target_properties(crossos-keyboard-win PROPERTIES
    OUTPUT_NAME "crossos-keyboard-win"
    C_STANDARD 11
)
