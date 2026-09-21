// Spike B — WH_KEYBOARD_LL suppress + SendInput replacement + UIPI probe.
//
// Bead: cross-os-ssj. Plan: COMPREHENSIVE_PLAN.md Phase 0 Spike B.
//
// Test strategy (two tiers):
//
//	Tier 1 (always runs on Windows): hook-observation tests. They install the
//	LL hook, synthesize input via SendInput, and assert the hook callback
//	itself observed the event and returned "suppress". This proves the
//	intercept half without depending on window focus.
//	Tier 2 (key receipt at a real window): gated behind -short skip because
//	the sandboxed session does not route synthesized keys to test windows
//	(SendInput succeeds, foreground matches, yet WM_KEYDOWN never arrives).
//	Run `go test` (not -short) on a real interactive desktop for these.
//
// Pass criteria (from the bead):
//  1. Ctrl+C suppressed via WH_KEYBOARD_LL (hook observes + returns 1;
//     Tier 2 additionally asserts the original never reached a window).
//  2. SendInput replacement received by non-elevated target (Tier 2).
//  3. UIPI elevated-target case recorded (SendInput result/error code +
//     fallback = passthrough + user notice, never silent drop).
//  4. Raw Input is observe-only — documented, no test needed.
package main

import (
	"runtime"
	"testing"
	"time"
)

// TestHookObservesCtrlC (Tier 1): the LL hook fires for a real injected
// Ctrl+C and the spike harness records the observation. Combined with the
// callback's `return 1` path (unit-asserted via classifyKey below), this is
// the sandbox-runnable proof of suppression.
func TestHookObservesCtrlC(t *testing.T) {
	if !isWindows() {
		t.Skip("windows-only")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hookSawCtrlC.Store(false)
	installTestHook(t, observeCtrlC)
	defer uninstallTestHook()

	if err := sendCtrlC(); err != nil {
		t.Fatalf("sendCtrlC: %v", err)
	}
	// Pump THIS thread's queue while waiting: LL callbacks arrive on the
	// installing thread and only fire while it pumps. Sleeping without
	// pumping starves delivery. (review P0: cross-os-c0)
	if !pumpUntil(5*time.Second, hookSawCtrlC.Load) {
		t.Fatal("hook never observed injected Ctrl+C")
	}
}

// TestClassifySuppress (Tier 1, pure unit): the decision function returns
// suppress for Ctrl+C and pass for everything else — no Windows input needed
// beyond the function itself.
func TestClassifySuppress(t *testing.T) {
	if got := classifyKey(vkC, true, ctrlCOnly); got != decisionSuppress {
		t.Fatalf("Ctrl+C with suppress mode: got %v, want suppress", got)
	}
	if got := classifyKey(vkC, true, ctrlCToF9); got != decisionSuppressInject {
		t.Fatalf("Ctrl+C with replace mode: got %v, want suppress+inject", got)
	}
	if got := classifyKey(vkC, true, modePass); got != decisionPass {
		t.Fatalf("Ctrl+C with pass mode: got %v, want pass", got)
	}
	if got := classifyKey(vkC, false, ctrlCOnly); got != decisionPass {
		t.Fatalf("bare C (no ctrl): got %v, want pass", got)
	}
	if got := classifyKey(0x41, true, ctrlCOnly); got != decisionPass {
		t.Fatalf("Ctrl+A: got %v, want pass", got)
	}
}

// TestSuppressCtrlC (Tier 2): with the hook armed, the original Ctrl+C never
// reaches a focusable window. Skipped under -short (sandbox has no key
// routing); run on a real interactive desktop.
func TestSuppressCtrlC(t *testing.T) {
	if !isWindows() {
		t.Skip("windows-only")
	}
	if testing.Short() {
		t.Skip("needs interactive desktop key routing")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	target := newTestWindow(t, "spike-b-suppress")
	defer target.close()

	// Sanity: with the hook disarmed the window MUST receive keys, proving
	// the window is focusable. Without this, a suppress assertion below would
	// pass vacuously (a window that can never receive input "receives nothing").
	installTestHook(t, modePass)
	if err := sendF9(); err != nil {
		t.Fatalf("sendF9 sanity: %v", err)
	}
	if got := target.waitKey(2 * time.Second); got != vkF9 {
		t.Fatalf("sanity failed: focusable window got vk=%#x, want F9", got)
	}

	installTestHook(t, ctrlCOnly)
	defer uninstallTestHook()

	if err := sendCtrlC(); err != nil {
		t.Fatalf("sendCtrlC: %v", err)
	}
	if got := target.waitKey(500 * time.Millisecond); got != 0 {
		t.Fatalf("original Ctrl+C leaked to target: vk=%#x", got)
	}
}

// TestSendInputReplacement (Tier 2): the replacement key emitted after
// suppression arrives at a non-elevated target. Same -short gate as above.
func TestSendInputReplacement(t *testing.T) {
	if !isWindows() {
		t.Skip("windows-only")
	}
	if testing.Short() {
		t.Skip("needs interactive desktop key routing")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	target := newTestWindow(t, "spike-b-replace")
	defer target.close()

	installTestHook(t, ctrlCToF9)
	defer uninstallTestHook()

	if err := sendCtrlC(); err != nil {
		t.Fatalf("sendCtrlC: %v", err)
	}
	if got := target.waitKey(2 * time.Second); got != vkF9 {
		t.Fatalf("replacement not received: got vk=%#x, want F9", got)
	}
}

// TestUIPIProbe records the SendInput result against the current integrity
// level and asserts the fallback contract (criterion 3): when injection is
// blocked, the spike must fall back to passthrough + user notice, never a
// silent drop.
func TestUIPIProbe(t *testing.T) {
	if !isWindows() {
		t.Skip("windows-only")
	}
	res := probeUIPI(t)
	t.Logf("UIPI probe: elevated=%v sendInputResult=%d lastErr=%v fallback=%s",
		res.elevated, res.sent, res.lastErr, res.fallback)
	// Honest framing (review P1: cross-os-c0): this probe measures same-IL
	// injectability, NOT the low→high UIPI block — no elevated target window
	// exists in this harness. An elevated process injecting successfully is
	// the EXPECTED outcome (UIPI blocks low→high, never high→low). The true
	// low→high case needs a real desktop with an elevated target and is
	// deferred to bead cross-os-ab4. What the spike locks: the result/errcode
	// are recorded and the fallback constant is passthrough+notice.
	if res.fallback != fallbackPassThroughNotice {
		t.Fatalf("fallback contract violated: got %q, want passthrough+notice",
			res.fallback)
	}
}
