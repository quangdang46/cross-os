// Adapter tests — bead cross-os-ab4.
//
// Pass criteria mapping:
//  1. Suppress-one-synthetic-key round trip → TestSuppressRoundTrip (Driver
//     maps Core's CONSUME/REPLACE to suppress/inject through the seam).
//  2. Build/linking/signing documented → doc.go (asserted by TestDocMentions).
//  3. No Swift-direct-from-Go, no logic in adapters → TestNoPolicyInBridge
//     (bridge maps only; policy lives in Core's Decide).
//  4. Errors to caller AND stage logs, never silent → TestErrorsSurfaced.
package adapter

import (
	"strings"
	"testing"

	"crossos/core/pkg/event"
	"crossos/core/pkg/pluginapi"
)

type memLog struct{ lines []string }

func (m *memLog) Log(stage, msg string) { m.lines = append(m.lines, stage+":"+msg) }

func outcome(d pluginapi.Decision) event.Outcome {
	return event.Outcome{Decision: d}
}

func TestSuppressRoundTrip(t *testing.T) {
	// A synthetic key suppressed by Core must cross the seam as suppress.
	d := &Driver{Decide: func(ev event.Event, ctx event.FastContext) event.Outcome {
		return outcome(pluginapi.DecisionConsume)
	}}
	tap := NewKeyboardTap(d)
	if err := tap.Uninstall(); err != nil {
		t.Fatalf("Uninstall clean: %v", err)
	}
	got := tap.DecideOne(event.Event{Type: event.EventKeyDown}, event.FastContext{})
	if got != KeySuppress {
		t.Fatalf("CONSUME mapped to %v, want KeySuppress", got)
	}
	// REPLACE → inject path; PASS → untouched.
	d.Decide = func(ev event.Event, ctx event.FastContext) event.Outcome {
		return outcome(pluginapi.DecisionReplace)
	}
	if got := tap.DecideOne(event.Event{}, event.FastContext{}); got != KeySuppressInject {
		t.Fatalf("REPLACE mapped to %v, want KeySuppressInject", got)
	}
	d.Decide = func(ev event.Event, ctx event.FastContext) event.Outcome {
		return outcome(pluginapi.DecisionPass)
	}
	if got := tap.DecideOne(event.Event{}, event.FastContext{}); got != KeyPass {
		t.Fatalf("PASS mapped to %v, want KeyPass", got)
	}
	// Inert without a router: pass-through, never suppress.
	bare := &Driver{}
	if got := NewKeyboardTap(bare).DecideOne(event.Event{}, event.FastContext{}); got != KeyPass {
		t.Fatalf("nil router mapped to %v, want KeyPass (inert)", got)
	}
}

func TestFromDecision(t *testing.T) {
	if FromDecision(pluginapi.DecisionPass) != KeyPass {
		t.Fatal("PASS → KeyPass")
	}
	if FromDecision(pluginapi.DecisionConsume) != KeySuppress {
		t.Fatal("CONSUME → KeySuppress")
	}
	if FromDecision(pluginapi.DecisionReplace) != KeySuppressInject {
		t.Fatal("REPLACE → KeySuppressInject")
	}
}

func TestErrorsSurfaced(t *testing.T) {
	log := &memLog{}
	d := &Driver{Log: log}
	tap := NewKeyboardTap(d)
	// Install before the native bridge links: typed error to the caller...
	err := tap.Install()
	if err == nil {
		t.Fatal("pre-link Install: want typed error, got nil (never silent success)")
	}
	// ...AND in the stage log.
	found := false
	for _, l := range log.lines {
		if strings.HasPrefix(l, "tap:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("stage log missing tap error line, got %v", log.lines)
	}
	q := NewWindowQuery(d)
	if _, err := q.Focused(); err == nil {
		t.Fatal("Focused pre-consent: want typed error, got nil")
	}
	if err := q.MoveResize(MoveResize{}); err == nil {
		t.Fatal("MoveResize zero id: want typed error, got nil")
	}
}

func TestNoPolicyInBridge(t *testing.T) {
	// The bridge must not suppress on its own: with a PASS router every
	// input passes regardless of key code. Policy lives in Core's Decide.
	d := &Driver{Decide: func(ev event.Event, ctx event.FastContext) event.Outcome {
		return outcome(pluginapi.DecisionPass)
	}}
	tap := NewKeyboardTap(d)
	ctrlC := event.Event{Type: event.EventKeyDown, KeyCode: 0x43, Modifiers: 0x01}
	if got := tap.DecideOne(ctrlC, event.FastContext{}); got != KeyPass {
		t.Fatalf("bridge suppressed without Core verdict: %v", got)
	}
}
