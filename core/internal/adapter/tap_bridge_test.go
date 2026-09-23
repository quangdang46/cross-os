// Tap bridge tests — native tap bridge bead (plan §4).
//
// Install on a sandbox/CI machine without TCC input-monitoring consent
// returns the typed denial (bridge NULL → errTapDenied, dual-surfaced).
// That IS the assertion: the test proves the failure path is explicit,
// never silent success. On hardware with consent the same call returns
// nil and the live tap owns the callback (Tier-2, manual).
package adapter

import (
	"strings"
	"testing"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/pluginapi"
)

func TestTapInstallDenialIsTyped(t *testing.T) {
	log := &memLog{}
	d := &Driver{Log: log}
	tap := NewKeyboardTap(d)
	err := tap.Install()
	if err == nil {
		// Consent granted on this machine: live tap installed — clean up.
		if uerr := tap.Uninstall(); uerr != nil {
			t.Fatalf("Uninstall after live install: %v", uerr)
		}
		t.Log("live tap installed (TCC consent present) — denial path N/A here")
		return
	}
	// Sandbox path: typed denial, dual-surfaced (caller + stage log).
	if !strings.Contains(err.Error(), "tap") {
		t.Fatalf("denial error %q must name the tap", err)
	}
	found := false
	for _, l := range log.lines {
		if strings.HasPrefix(l, "tap:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("stage log missing tap denial line, got %v", log.lines)
	}
}

func TestTapDecideExportRouting(t *testing.T) {
	// The C callback routes through crossosGoDecide → bound Driver.Decide.
	// Exercised in-process (no tap needed): bind, call the export, unbind.
	// The stub Decide suppresses Ctrl+C keydown only (spike A semantics);
	// everything else passes. It matches on the TRANSLATED keycode (0x43),
	// which is the point: the tap receives macOS 0x08 and Decide must see
	// the internal Windows-VK code the rule table is written in.
	d := &Driver{Decide: func(ev event.Event, ctx event.FastContext) event.Outcome {
		if ev.Type == event.EventKeyDown && ev.KeyCode == 0x43 && ev.Modifiers == 1 {
			return event.Outcome{Decision: pluginapi.DecisionReplace}
		}
		return event.Outcome{Decision: pluginapi.DecisionPass}
	}}
	decideExport = d
	defer func() { decideExport = nil }()
	if got := crossosGoDecide(0x08, cgFlagCtrl, 1, nil); got != 1 {
		t.Fatalf("ctrl+c keydown export=%d, want 1 (suppress)", got)
	}
	if got := crossosGoDecide(0x08, 0, 1, nil); got != 0 {
		t.Fatalf("bare c export=%d, want 0 (pass)", got)
	}
	if got := crossosGoDecide(0x08, cgFlagCtrl, 0, nil); got != 0 {
		t.Fatalf("key-up export=%d, want 0 (pass)", got)
	}
	_ = intent.DefaultRegistry
}
