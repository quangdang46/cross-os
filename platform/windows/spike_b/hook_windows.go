//go:build windows

// Spike B harness: WH_KEYBOARD_LL hook, SendInput injection, UIPI probe,
// and a hidden test window that records the virtual keys it receives.
//
// Design notes (from the plan, §4.2):
//   - The hook callback stays thin: classify the event, decide
//     suppress/inject synchronously, return. No IPC, no allocation-heavy
//     work on the callback path.
//   - A package-level hookMode selects the test behavior; the zero value is
//     pass-through so the harness is inert unless a test arms it.
//   - SendInput failures (UIPI) are surfaced as (sent, lastErr); callers own
//     the fallback decision — the hook never silently drops.
package main

import (
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Virtual keys used by the spike.
const (
	vkControl = 0x11
	vkC       = 0x43
	vkF9      = 0x78
)

const (
	whKeyboardLL   = 13
	hcAction       = 0
	wmKeydown      = 0x0100
	wmSyskeydown   = 0x0104
	inputKeyboard  = 1
	keyeventfKeyup = 0x0002
	pmRemove       = 1
)

// fallbackPassThroughNotice is the normative UIPI fallback for the spike:
// pass the original through and surface a user notice — never drop silently.
const fallbackPassThroughNotice = "passthrough+notice"

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procSetHook            = user32.NewProc("SetWindowsHookExW")
	procUnhook             = user32.NewProc("UnhookWindowsHookEx")
	procCallNext           = user32.NewProc("CallNextHookEx")
	procSendInput          = user32.NewProc("SendInput")
	procGetMessage         = user32.NewProc("GetMessageW")
	procPeekMessage        = user32.NewProc("PeekMessageW")
	procGetForeground      = user32.NewProc("GetForegroundWindow")
	procRegisterClass      = user32.NewProc("RegisterClassExW")
	procCreateWindow       = user32.NewProc("CreateWindowExW")
	procDefWindowProc      = user32.NewProc("DefWindowProcW")
	procDestroyWindow      = user32.NewProc("DestroyWindow")
	procGetAsyncKeyState   = user32.NewProc("GetAsyncKeyState")
	procSetForeground      = user32.NewProc("SetForegroundWindow")
	procShowWindow         = user32.NewProc("ShowWindow")
	procSetFocus           = user32.NewProc("SetFocus")
	procAttachInput        = user32.NewProc("AttachThreadInput")
	procGetWindowThread    = user32.NewProc("GetWindowThreadProcessId")
	procGetCurrentThreadID = kernel32.NewProc("GetCurrentThreadId")
	procTranslateMsg       = user32.NewProc("TranslateMessage")
	procDispatchMsg        = user32.NewProc("DispatchMessageW")
)

// msg mirrors Win32 MSG (48 bytes on amd64).
type msg struct {
	hwnd    uintptr
	message uint32
	_pad    uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
	lPriv   uint32
}

type kbdllhookstruct struct {
	vkCode      uint32
	scanCode    uint32
	flags       uint32
	time        uint32
	dwExtraInfo uintptr
}

// keyboardInput mirrors Win32 INPUT with a KEYBDINPUT payload. Verified
// empirically (spike B, 2026-09-21): the layout SendInput accepts on amd64 is
// 40 bytes — DWORD type + 4 pad + 28-byte payload slot + 8 tail. A 24/32-byte
// packing fails with ERROR_INVALID_PARAMETER even though sizeof(INPUT) in C
// headers suggests otherwise; Go's field alignment differs from the MSVC
// union layout, so the explicit pads below reproduce the accepted shape.
type keyboardInput struct {
	typ     uint32
	_pad0   uint32
	vk      uint16
	scan    uint16
	flags   uint32
	time    uint32
	dwExtra uintptr
	_pad1   [8]byte
}

// hookMode selects what the installed hook does with Ctrl+C.
type hookMode int

const (
	modePass     hookMode = iota // inert: forward everything
	ctrlCOnly                    // suppress Ctrl+C, no replacement
	ctrlCToF9                    // suppress Ctrl+C, inject F9
	observeCtrlC                 // suppress Ctrl+C and record the observation (Tier 1 test tap)
)

// keyDecision is the pure, unit-testable core of the hook callback: given a
// key event, what should the callback do? The actual callback adds only the
// OS plumbing (nCode/wParam checks, CallNextHookEx, async injection).
type keyDecision int

const (
	decisionPass keyDecision = iota
	decisionSuppress
	decisionSuppressInject
)

// classifyKey is the decision function — keep it free of syscalls so the
// Tier 1 unit test covers the suppression logic exactly.
func classifyKey(vk uint32, ctrl bool, m hookMode) keyDecision {
	if m != modePass && vk == vkC && ctrl {
		if m == ctrlCToF9 {
			return decisionSuppressInject
		}
		return decisionSuppress // covers ctrlCOnly AND observeCtrlC
	}
	return decisionPass
}

var (
	// hookMode_ is read on every keystroke inside the LL callback, which has
	// a system timeout (LowLevelHooksTimeout, typically ~300ms) after which
	// the hook is silently removed. It MUST be lock-free on the hot path —
	// hence atomic, never a mutex (review: cross-os-c0).
	hookMode_ atomic.Int32
	hookMu    sync.Mutex // serializes install/uninstall only, never touched in-callback
	hookH     uintptr
	// hookSawCtrlC is the Tier 1 observation tap: set when the callback
	// suppresses a Ctrl+C in observeCtrlC mode. Lives here (not in the
	// _test file) so the non-test build compiles.
	hookSawCtrlC atomic.Bool
)

// injectTag is the dwExtraInfo magic reserved for CrossOS-synthesized input.
// The spike emits F9 (never Ctrl+C) so no loop is possible, but dependent
// bead cross-os-wge (synthetic self-ignore) must tag every injected event
// with this value and PASS anything carrying it. Recorded here so the
// convention is not rediscovered later. (review: cross-os-c0)
// TODO(wge): stamp injectTag into pressKey's dwExtraInfo and add the
// PASS-on-tag branch in the product callback. Currently reserved only.
const injectTag = 0xC20505

type hookProc func(nCode int32, wParam, lParam uintptr) uintptr

// installTestHook arms the LL hook in the requested mode. It fails the test
// immediately if SetWindowsHookExW returns NULL — a failed install must never
// masquerade as "hook armed, nothing observed". (review: cross-os-c0)
func installTestHook(t interface{ Fatalf(string, ...any) }, m hookMode) {
	hookMu.Lock()
	defer hookMu.Unlock()
	hookMode_.Store(int32(m))
	if hookH != 0 {
		return
	}
	cb := syscall.NewCallback(lowLevelProc)
	h, _, err := procSetHook.Call(whKeyboardLL, cb, 0, 0)
	if h == 0 {
		t.Fatalf("SetWindowsHookExW failed: %v", err)
	}
	hookH = h
}

func uninstallTestHook() {
	hookMu.Lock()
	defer hookMu.Unlock()
	if hookH != 0 {
		procUnhook.Call(hookH)
		hookH = 0
	}
	hookMode_.Store(int32(modePass))
}

// lowLevelProc is the hook callback. It classifies synchronously and returns:
// nonzero suppresses the event, CallNextHookEx forwards it.
//
// NOTE on `go vet unsafeptr` (bead cross-os-qhp.9): lParam here is the
// WH_KEYBOARD_LL-mandated KBDLLHOOKSTRUCT pointer (documented Win32 ABI, not
// a Go pointer escape) — the conversion below is the only correct read, and
// vet's unsafeptr check fires on the pattern regardless. There is no
// //vet:ignore mechanism; the CI step for spike_b runs vet with the check
// disabled (see .github/workflows/ci.yml), which is the sanctioned
// suppression (cmd/vet: -unsafeptr=false runs all checks except unsafeptr).
func lowLevelProc(nCode int32, wParam, lParam uintptr) uintptr {
	if nCode != hcAction {
		r, _, _ := procCallNext.Call(0, uintptr(nCode), wParam, lParam)
		return r
	}
	if wParam != wmKeydown && wParam != wmSyskeydown {
		r, _, _ := procCallNext.Call(0, uintptr(nCode), wParam, lParam)
		return r
	}
	kb := (*kbdllhookstruct)(unsafe.Pointer(lParam))

	// Lock-free hot path (see hookMode_ decl). No queue/allocation here; the
	// inject case hands off to a goroutine — TECH DEBT for bead cross-os-ab4:
	// the product adapter must drain injection through a queue owned
	// off-callback instead of spawning from inside syscall.NewCallback.
	// (review: cross-os-c0)
	m := hookMode(hookMode_.Load())
	switch classifyKey(kb.vkCode, ctrlDown(), m) {
	case decisionSuppressInject:
		// Inject asynchronously: SendInput from the callback risks
		// reentrancy, so hand it to a goroutine. The suppression
		// decision itself is already made synchronously here.
		go pressKey(vkF9)
		return 1 // suppress
	case decisionSuppress:
		if m == observeCtrlC {
			hookSawCtrlC.Store(true)
		}
		return 1 // suppress
	}
	r, _, _ := procCallNext.Call(0, uintptr(nCode), wParam, lParam)
	return r
}

func ctrlDown() bool {
	r, _, _ := procGetAsyncKeyState.Call(vkControl)
	return r&0x8000 != 0
}

// pressKey synthesizes a down+up pair for vk. It returns how many INPUTs were
// accepted and the trailing error, so callers (and the UIPI probe) can tell
// injection from a UIPI block.
func pressKey(vk uint16) (uint32, error) {
	ins := [2]keyboardInput{
		{typ: inputKeyboard, vk: vk},
		{typ: inputKeyboard, vk: vk, flags: keyeventfKeyup},
	}
	r, _, err := procSendInput.Call(2, uintptr(unsafe.Pointer(&ins[0])), unsafe.Sizeof(ins[0]))
	accepted := uint32(r)
	var lastErr error
	if accepted != 2 {
		lastErr = err
	}
	return accepted, lastErr
}

// sendF9 synthesizes a bare F9 (used for the focus sanity check).
func sendF9() error {
	if _, err := pressKey(vkF9); err != nil {
		return err
	}
	return nil
}

// sendCtrlC synthesizes a real Ctrl+C through SendInput (down/down/up/up).
func sendCtrlC() error {
	seq := [4]keyboardInput{
		{typ: inputKeyboard, vk: vkControl},
		{typ: inputKeyboard, vk: vkC},
		{typ: inputKeyboard, vk: vkC, flags: keyeventfKeyup},
		{typ: inputKeyboard, vk: vkControl, flags: keyeventfKeyup},
	}
	r, _, err := procSendInput.Call(4, uintptr(unsafe.Pointer(&seq[0])), unsafe.Sizeof(seq[0]))
	if uint32(r) != 4 {
		return err
	}
	return nil
}

// --- test window ---

type wndclassex struct {
	size      uint32
	style     uint32
	wndProc   uintptr
	clsExtra  int32
	wndExtra  int32
	instance  windows.Handle
	icon      windows.Handle
	cursor    windows.Handle
	brush     windows.Handle
	menuName  *uint16
	className *uint16
	iconSm    windows.Handle
}

type testWindow struct {
	hwnd windows.HWND
	mu   sync.Mutex
	keys []uint32
	got  chan uint32
}

var testWndClass = sync.OnceValue(func() *uint16 {
	name, _ := windows.UTF16PtrFromString("CrossOSSpikeB")
	classProc := syscall.NewCallback(testWndProc)
	wc := wndclassex{
		wndProc:   classProc,
		className: name,
	}
	wc.size = uint32(unsafe.Sizeof(wc))
	procRegisterClass.Call(uintptr(unsafe.Pointer(&wc)))
	return name
})

var testWindows sync.Map // hwnd -> *testWindow

// testWndProc (registered via syscall.NewCallback): record WM_KEYDOWN virtual keys.
// Limitation: WM_SYSKEYDOWN (Alt-modified keys) is NOT recorded — Alt-based
// replacements would be missed. Fine for Ctrl+C→F9; revisit if a spike needs
// Alt-modified injection. (review: cross-os-c0)
func testWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	const wmKeydownMsg = 0x0100
	const wmDestroy = 0x0002
	if msg == wmKeydownMsg {
		if v, ok := testWindows.Load(windows.HWND(hwnd)); ok {
			tw := v.(*testWindow)
			tw.mu.Lock()
			tw.keys = append(tw.keys, uint32(wParam))
			tw.mu.Unlock()
			select {
			case tw.got <- uint32(wParam):
			default:
			}
		}
	}
	if msg == wmDestroy {
		testWindows.Delete(windows.HWND(hwnd))
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// newTestWindow creates a hidden top-level window with a message pump so it
// can receive synthesized keys.
func newTestWindow(t interface{ Fatalf(string, ...any) }, title string) *testWindow {
	name := testWndClass()
	wtitle, _ := windows.UTF16PtrFromString(title)
	hwnd, _, err := procCreateWindow.Call(0,
		uintptr(unsafe.Pointer(name)),
		uintptr(unsafe.Pointer(wtitle)),
		0x10CF0000, // WS_OVERLAPPEDWINDOW (visible-capable; shown below)
		100, 100, 300, 200, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatalf("CreateWindowExW: %v", err)
	}
	tw := &testWindow{hwnd: windows.HWND(hwnd), got: make(chan uint32, 16)}
	testWindows.Store(tw.hwnd, tw)
	go pumpMessages()
	// Show + focus so synthesized keys route here. A hidden (zero-size or
	// never-shown) window never receives keyboard input — TestSuppressCtrlC
	// passed vacuously for that reason before this fix. Foreground rights are
	// also stolen: the test binary runs in background (launched from a shell
	// without focus rights), so AttachThreadInput borrows the foreground
	// thread's input state before SetForegroundWindow.
	procShowWindow.Call(uintptr(tw.hwnd), 5) // SW_SHOW
	stealFocus(tw.hwnd)
	return tw
}

// stealFocus attaches our thread input to the foreground thread, then takes
// foreground + focus. Best-effort: logs nothing, callers verify via key
// receipt (the sanity check), not via focus APIs.
func stealFocus(hwnd windows.HWND) {
	// NOTE: must run on an OS thread — test calls runtime.LockOSThread first.
	fg, _, _ := procGetForeground.Call()
	var fgPID uint32
	fgTID, _, _ := procGetWindowThread.Call(fg, uintptr(unsafe.Pointer(&fgPID)))
	selfTID, _, _ := procGetCurrentThreadID.Call()
	if fgTID != 0 && fgTID != selfTID {
		procAttachInput.Call(selfTID, fgTID, 1)
		procSetForeground.Call(uintptr(hwnd))
		time.Sleep(100 * time.Millisecond)
		procAttachInput.Call(selfTID, fgTID, 0)
	} else {
		procSetForeground.Call(uintptr(hwnd))
	}
	time.Sleep(100 * time.Millisecond)
	procSetFocus.Call(uintptr(hwnd))
	time.Sleep(100 * time.Millisecond)
}

func pumpMessages() {
	var m msg
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if r == 0 {
			return
		}
		procTranslateMsg.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMsg.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// pumpOnce drains one pending message for the calling thread (PeekMessage +
// Translate + Dispatch). WH_KEYBOARD_LL callbacks are delivered on the thread
// that installed the hook, and only while that thread pumps — so every test
// that installs the hook MUST pump on the installing (LockedOSThread) thread.
// Polling/sleeping without pumping starves delivery and makes observations
// flaky-to-dead. (review P0: cross-os-c0)
func pumpOnce() {
	var m msg
	r, _, _ := procPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
	if r != 0 {
		procTranslateMsg.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMsg.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// pumpUntil pumps the calling thread's queue until cond is true or timeout
// elapses. Returns the final value of cond.
func pumpUntil(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return cond()
		}
		pumpOnce()
		time.Sleep(5 * time.Millisecond)
	}
}

func (tw *testWindow) waitKey(d time.Duration) uint32 {
	// Pump-while-waiting: the hook lives on this (LockedOSThread) thread, so
	// a bare channel block would starve suppress/replace callbacks exactly
	// the way Tier 1 starved before pumpUntil. (review follow-up: cross-os-c0)
	deadline := time.Now().Add(d)
	for {
		select {
		case k := <-tw.got:
			return k
		default:
		}
		if time.Now().After(deadline) {
			return 0
		}
		pumpOnce()
		time.Sleep(5 * time.Millisecond)
	}
}

func (tw *testWindow) close() {
	procDestroyWindow.Call(uintptr(tw.hwnd))
}

// --- UIPI probe ---

type uipiResult struct {
	elevated bool
	sent     uint32
	lastErr  error
	fallback string
}

// probeUIPI tries one injection and reports the outcome plus the normative
// fallback. On an elevated process the SendInput is expected to be neutered
// for lower-IL targets; the spike records that instead of hiding it.
func probeUIPI(t interface{ Logf(string, ...any) }) uipiResult {
	elevated := isElevated()
	sent, err := pressKey(vkF9)
	t.Logf("probe: elevated=%v accepted=%d err=%v", elevated, sent, err)
	return uipiResult{
		elevated: elevated,
		sent:     sent,
		lastErr:  err,
		fallback: fallbackPassThroughNotice,
	}
}

// isElevated reports whether the current process runs elevated (admin).
// Implemented via CheckTokenMembership against the Administrators SID.
func isElevated() bool {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()
	var elevation uint32
	var outLen uint32
	err := windows.GetTokenInformation(token, windows.TokenElevation,
		(*byte)(unsafe.Pointer(&elevation)), uint32(unsafe.Sizeof(elevation)), &outLen)
	return err == nil && elevation != 0
}

func isWindows() bool { return true }

func main() {}
