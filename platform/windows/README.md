# Windows native helper (thin DLL) + device-identity note
#
# Plan §12 build commands. CMake builds JUST the thin
# crossos-keyboard-win.dll — hook + Raw Input observe + SendInput, nothing
# more. All decision logic stays in Go, reached via C ABI from
# core/internal/adapter (see dll_windows.go for the frozen surface).
#
# Spike B (platform/windows/spike_b, bead cross-os-ssj) proved the hook
# shape in-test: WH_KEYBOARD_LL suppress + SendInput replacement + UIPI
# probe. This directory hosts the product CMake step when a Windows
# machine runs it:
#
#   cd platform/windows && cmake . && make   # outputs crossos-keyboard-win.dll
#
# No CMakeLists.txt in tree yet: authoring it needs a Windows MSVC
# verification pass (same policy as the Xcode project — no hand-written
# build files without the toolchain that consumes them). CI builds the Go
# side cross-platform (GOOS=windows); the DLL links at runtime via
# LoadLibrary so its absence is a typed Install-time error, never a
# link-time surprise.
