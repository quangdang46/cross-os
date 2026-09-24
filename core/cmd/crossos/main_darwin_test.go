//go:build darwin

// macOS-only daemon tests. These drive the real CGEventTap decide entry
// (adapter.BindDecideForTest / DecideForMacTest) and the macOS keycode
// translation (ToWinKeycode), all of which exist only under the darwin build
// constraint. They are the proof that a chord as CGEvent reports it reaches
// the same rule the router reaches with the internal Windows virtual keycode
// — a property no off-darwin stub could assert, so the file is constrained
// rather than emptied. Everything portable stays in main_test.go and runs on
// every GOOS.
package main

import (
	"strings"
	"testing"
	"time"

	"crossos/core/internal/adapter"
	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	builtin "crossos/core/rules"
)

// TestRealDispatchFailurePassesKeyThrough binds the tap's decide entry to
// the daemon's REAL dispatcher and asserts the end-to-end safety property
// (bead cross-os-vx9): with no AX bridge the darwin seam denies, so
// crossosGoDecide must return 0 (let the key through) rather than 1.
// A future change that makes windowDispatch swallow denials and return nil
// would flip this to 1 and fail here.
func TestRealDispatchCommitsAndDoesNotBlock(t *testing.T) {
	// The tap commits the action to the worker and returns immediately, so a
	// capability that cannot run yet (no AX bridge on this path) must NOT
	// change the verdict: the key is suppressed and the failure is surfaced
	// through the daemon log, which the UI reads. Blocking here to learn the
	// outcome is what makes macOS disable the tap.
	c, err := NewCoreWithSettings(builtin.All(), builtin.Grants(), "")
	if err != nil {
		t.Fatalf("NewCore: %v", err)
	}
	for _, id := range builtin.BuiltinIDs {
		c.registerBuiltin(id, true)
	}
	focus := &appCache{}
	focus.set(event.FastContext{AppID: "com.apple.Finder", AppMode: event.AppModeNative})
	d := &adapter.Driver{
		Decide:   c.decideLocked,
		Dispatch: c.dispatch,
		Context:  focus.get,
		Log:      daemonTapLog{},
	}
	adapter.BindDecideForTest(d)
	defer adapter.BindDecideForTest(nil)

	done := make(chan int32, 1)
	go func() { done <- adapter.DecideForMacTest(0x08, 1, event.FastContext{}) }()
	select {
	case got := <-done:
		if got != 1 {
			t.Fatalf("verdict=%d, want 1 (action committed, key suppressed)", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("callback blocked on dispatch — this is the timeout-disable cause")
	}
}

// TestEveryRuleKeycodeIsReachableFromMac guards the trap that made the app
// silently inert (bead cross-os-uok): a rule authored in Windows VK that no
// macOS keycode maps to can never fire from the live tap. Every keycode the
// builtin rules claim must be the image of at least one macOS keycode.
func TestEveryRuleKeycodeIsReachableFromMac(t *testing.T) {
	reachable := map[uint32]uint16{}
	for mac := uint16(0); mac < 128; mac++ {
		if win, ok := adapter.ToWinKeycode(mac); ok {
			reachable[uint32(win)] = mac
		}
	}
	for _, r := range builtin.All() {
		if _, ok := reachable[r.KeyCode]; !ok {
			t.Fatalf("rule %s uses keycode %#x which no macOS keycode maps to — it can never fire from the tap",
				r.RuleID, r.KeyCode)
		}
	}
}

// TestMacKeycodesDriveRules is the end-to-end proof: chords expressed the
// way macOS reports them (kVK_C=0x08 with Control, kVK_LeftArrow=0x7B with
// Command) must reach the SAME rules the router tests reach with Windows
// codes. The negative control keeps this honest.
func TestMacKeycodesDriveRules(t *testing.T) {
	c, err := NewCoreWithSettings(builtin.All(), builtin.Grants(), "")
	if err != nil {
		t.Fatalf("NewCore: %v", err)
	}
	for _, id := range builtin.BuiltinIDs {
		c.registerBuiltin(id, true)
	}
	// The tap reads the focused app from a cached snapshot (cross-os-heu),
	// so the driver gets a Context seam — the same shape startTap wires.
	focus := &appCache{}
	focus.set(event.FastContext{AppID: "com.apple.Finder", AppMode: event.AppModeNative})
	d := &adapter.Driver{
		Decide:   c.decideLocked,
		Dispatch: func(intent.Request) error { return nil },
		Context:  focus.get,
		Log:      daemonTapLog{},
	}
	adapter.BindDecideForTest(d)
	defer adapter.BindDecideForTest(nil)

	fastCtx := event.FastContext{AppID: "com.apple.Finder", AppMode: event.AppModeNative}
	cases := []struct {
		name   string
		macKey uint16 // kVK_*, as CGEvent reports it
		mods   uint32 // internal Mod* bits, for the flag reconstruction
		winner string
	}{
		{"Ctrl+C", 0x08, 1 << 0, "windows-keyboard.ctrl-c-copy"},
		{"Win+Left", 0x7B, 1 << 3, "windows-keyboard.win-left-snap"},
		{"Win+Right", 0x7C, 1 << 3, "windows-keyboard.win-right-snap"},
		{"Win+Up", 0x7E, 1 << 3, "windows-keyboard.win-up-maximize"},
		{"Win+Down", 0x7D, 1 << 3, "windows-keyboard.win-down-minimize"},
		{"Alt+F4", 0x76, 1 << 2, "windows-keyboard.alt-f4-close-window"},
		{"Ctrl+Shift+C", 0x08, 1<<0 | 1<<1, "developer.ctrl-shift-c-copypath"},
		{"Ctrl+Shift+P", 0x23, 1<<0 | 1<<1, "developer.ctrl-shift-p-editor"},
	}
	// The developer rules are scoped to IDE apps, so those chords must be
	// evaluated with an IDE in focus; a Finder context legitimately misses.
	ideCtx := event.FastContext{AppID: "com.microsoft.VSCode", AppMode: event.AppModeNative}
	for _, tc := range cases {
		ctx := fastCtx
		if strings.HasPrefix(tc.winner, "developer.") {
			ctx = ideCtx
		}
		out := c.decideLocked(event.Event{
			Type: event.EventKeyDown, KeyCode: uint32(adapter.MustWinKeycodeForTest(t, tc.macKey)), Modifiers: tc.mods,
		}, ctx)
		if out.WinnerRule != tc.winner {
			t.Fatalf("%s: winner=%q, want %q (internal-VK path)", tc.name, out.WinnerRule, tc.winner)
		}
		// The same chord through the real macOS tap entry point, with the
		// focused app the cache reports.
		focus.set(ctx)
		if got := adapter.DecideForMacTest(tc.macKey, tc.mods, ctx); got != 1 {
			t.Fatalf("%s: tap verdict=%d, want 1 (macOS keycode must reach the rule)", tc.name, got)
		}
	}

	// An app-scoped rule must MISS when the cache reports a different app:
	// the Context seam is load-bearing, not decoration.
	focus.set(event.FastContext{AppID: "com.apple.finder", AppMode: event.AppModeNative})
	if got := adapter.DecideForMacTest(0x23, 1<<0|1<<1, fastCtx); got != 0 {
		t.Fatalf("developer chord with a non-IDE focused app: verdict=%d, want 0 (app-scoped rule must not fire)", got)
	}
}
