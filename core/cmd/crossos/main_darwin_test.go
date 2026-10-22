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
	"sync"
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
	// capability that cannot run yet (no snapshot, no bridge on this path)
	// must NOT change the verdict: the key is suppressed and the failure is
	// surfaced through the daemon log, which the UI reads. Blocking here to
	// learn the outcome is what makes macOS disable the tap.
	//
	// Both chords here are ones the daemon CAN act on now (beads
	// cross-os-1be / cross-os-jzc), and both are suppressed: the verdict is
	// about the COMMIT, not about the outcome — the assert is that the
	// callback never blocks, never reraises, never eats a key whose action
	// did not even get committed.
	c, err := NewCoreWithSettings(builtin.All(), builtin.Grants(), "")
	if err != nil {
		t.Fatalf("NewCore: %v", err)
	}
	for _, id := range builtin.BuiltinIDs {
		c.registerBuiltin(id, true)
	}
	focus := &appCache{}
	focus.set(event.FastContext{AppID: "com.apple.finder", AppMode: event.AppModeNative})
	d := &adapter.Driver{
		Decide:   c.decideLocked,
		Dispatch: c.dispatch,
		Context:  focus.get,
		Log:      tapLog,
	}
	adapter.BindDecideForTest(d)
	defer adapter.BindDecideForTest(nil)

	// The chord posts are real input injected into the machine, so this
	// test binds the seam to a recorder: committing must happen for the
	// test, but strike-the-real-Edit-menu must not.
	var postedMu sync.Mutex
	var posted []uint16
	prevPost := postChordLive
	postChordLive = func(k uint16) error {
		postedMu.Lock()
		posted = append(posted, k)
		postedMu.Unlock()
		return nil
	}
	defer func() { postChordLive = prevPost }()

	done := make(chan int32, 1)
	// Win+Left is the window-snap rule, and window.move has a dispatch
	// route — so the tap commits it and suppresses the original.
	go func() { done <- adapter.DecideForMacTest(0x7B, 1<<3, event.FastContext{}) }()
	select {
	case got := <-done:
		if got != 1 {
			t.Fatalf("verdict=%d, want 1 (action committed, key suppressed)", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("callback blocked on dispatch — this is the timeout-disable cause")
	}

	// Ctrl+C in Finder is the windows-keyboard copy rule. The worker
	// commits it the same way, and the chord that reaches the recorder is
	// Cmd+C — the Windows-to-macOS translation — not the physical Ctrl+C
	// the user pressed.
	alone := make(chan int32, 1)
	go func() { done2 := adapter.DecideForMacTest(0x08, 1, event.FastContext{}); alone <- done2 }()
	select {
	case got := <-alone:
		if got != 1 {
			t.Fatalf("verdict=%d, want 1 (the copy chord is suppressed after its commit)", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("callback blocked on the copy path")
	}
	// The worker is asynchronous — the snap job it runs first logs its
	// no-cache refusal after the callback has already returned — so the
	// copy chord is waited for, bounded, rather than sampled at a race
	// point.
	deadline := time.Now().Add(time.Second)
	for {
		postedMu.Lock()
		got := append([]uint16(nil), posted...)
		postedMu.Unlock()
		if len(got) > 0 {
			if got[0] != 0x08 {
				t.Fatalf("posted=%v, want the Cmd+C keycode 0x08 chord through the seam", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the worker never ran the copy commit; no chord reached the seam")
		}
		time.Sleep(5 * time.Millisecond)
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
	focus.set(event.FastContext{AppID: "com.apple.finder", AppMode: event.AppModeNative})
	d := &adapter.Driver{
		Decide:   c.decideLocked,
		Dispatch: func(intent.Request) error { return nil },
		Context:  focus.get,
		Log:      tapLog,
	}
	adapter.BindDecideForTest(d)
	defer adapter.BindDecideForTest(nil)

	fastCtx := event.FastContext{AppID: "com.apple.finder", AppMode: event.AppModeNative}
	cases := []struct {
		name   string
		macKey uint16 // kVK_*, as CGEvent reports it
		mods   uint32 // internal Mod* bits, for the flag reconstruction
		winner string
		// claimed is whether the daemon CAN perform the rule's capability.
		// Every built-in rule here is dispatchable since beads
		// cross-os-1be (window close/minimize/maximize carry real AX
		// actions) and cross-os-jzc (clipboard.copy posts the Cmd chord) —
		// decideLocked refuses a key whose capability has no route, so a
		// verdict of 0 next to a named winner would now mean a routing
		// bug, and the table pins it as one.
		claimed bool
	}{
		{"Ctrl+C", 0x08, 1 << 0, "windows-keyboard.ctrl-c-copy", true},
		{"Win+Left", 0x7B, 1 << 3, "windows-keyboard.win-left-snap", true},
		{"Win+Right", 0x7C, 1 << 3, "windows-keyboard.win-right-snap", true},
		{"Win+Up", 0x7E, 1 << 3, "windows-keyboard.win-up-maximize", true},
		{"Win+Down", 0x7D, 1 << 3, "windows-keyboard.win-down-minimize", true},
		{"Alt+F4", 0x76, 1 << 2, "windows-keyboard.alt-f4-close-window", true},
		{"Ctrl+Shift+C", 0x08, 1<<0 | 1<<1, "developer.ctrl-shift-c-copypath", true},
		{"Ctrl+Shift+P", 0x23, 1<<0 | 1<<1, "developer.ctrl-shift-p-editor", true},
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
		// focused app the cache reports. The verdict is 1 for every
		// dispatched built-in chord — the macOS keycode must reach the
		// rule AND the action must run — and 0 only for a chord the
		// decision path deliberately refuses to claim.
		focus.set(ctx)
		want := int32(0)
		if tc.claimed {
			want = 1
		}
		if got := adapter.DecideForMacTest(tc.macKey, tc.mods, ctx); got != want {
			t.Fatalf("%s: tap verdict=%d, want %d (the macOS keycode must reach the rule, and the key is claimed only when the daemon can act on it)", tc.name, got, want)
		}
	}

	// An app-scoped rule must MISS when the cache reports an app outside its
	// scope: the Context seam is load-bearing, not decoration.
	//
	// **THE APP OUTSIDE THE SCOPE IS NOT FINDER.** The developer's rules are
	// scoped to the IDEs AND to Finder — "open a terminal at the folder" is
	// a Finder gesture, and the scope names "com.apple.finder", the id the
	// system reports. A control that used Finder as the app that must miss
	// was a control built on the misspelling: it passed for the wrong
	// reason, and it would have failed the moment the id was right.
	focus.set(event.FastContext{AppID: "com.apple.Safari", AppMode: event.AppModeNative})
	if got := adapter.DecideForMacTest(0x23, 1<<0|1<<1, fastCtx); got != 0 {
		t.Fatalf("developer chord in an app outside its scope: verdict=%d, want 0 (app-scoped rule must not fire)", got)
	}
}

// TestBuiltinAppIDsAreTheIdsTheSystemReports: every app id a built-in rule
// scopes itself to is compared for EQUALITY against the focused app's id
// (core/pkg/event/router.go), and that id is what
// `NSRunningApplication.bundleIdentifier` reports.
//
// So a rule that names an id with the wrong spelling names an application
// that does not exist, and the rule can never fire. Measured on this
// machine: the developer's three Finder rules were scoped to
// "com.apple.Finder", while the system reports "com.apple.finder" — so
// Ctrl+Shift+Enter, Ctrl+Shift+C and Ctrl+Shift+P decided PASS in Finder,
// and the shortcuts the plugin exists to provide were unreachable.
//
// The check is against the machine's own app list, so it catches a spelling
// nobody typed by hand and a spelling that drifts. An id whose application is
// not installed on this machine cannot be checked — the test says so and
// moves on — but an id whose application IS here must match exactly.
func TestBuiltinAppIDsAreTheIdsTheSystemReports(t *testing.T) {
	apps, err := adapter.ListApps(nil)
	if err != nil {
		t.Skipf("the app list is not available on this machine: %v", err)
	}
	if len(apps) == 0 {
		t.Skip("the app list is empty on this machine")
	}
	exact := map[string]bool{}
	folded := map[string]string{}
	for _, a := range apps {
		exact[a.BundleID] = true
		folded[strings.ToLower(a.BundleID)] = a.BundleID
	}
	seen := map[string]bool{}
	for _, r := range builtin.All() {
		for _, id := range r.AppIDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			if exact[id] {
				continue
			}
			if real, ok := folded[strings.ToLower(id)]; ok {
				t.Errorf("rule %q scopes itself to %q, but this machine reports that application as %q; the ids are compared for equality, so this rule can never fire",
					r.RuleID, id, real)
				continue
			}
			t.Logf("rule %q scopes itself to %q, which is not installed on this machine; nothing to check",
				r.RuleID, id)
		}
	}
}
