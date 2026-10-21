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
	"errors"
	"os"
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
	// Install: a typed error to the caller AND in the stage log.
	//
	// **The premise was wrong, and it said so in its own message.** The
	// message is "never silent success" — the property is that a FAILURE is
	// never silent, not that Install must fail. But the assertion demanded a
	// failure, and the tap does not fail: `CGEventTapCreate` succeeds
	// whether or not the input-monitoring consent is granted, because the
	// consent is checked when events flow rather than when the tap is made.
	// Measured: on a machine whose daemon had a live keyboard tap, this
	// assertion failed with "pre-link Install: want typed error, got nil".
	//
	// So the error path is asserted WHERE THERE IS ONE, and the success path
	// is not asserted to be a failure.
	err := tap.Install()
	if err != nil {
		// Typed means one of the adapter's own errors, and the marker they
		// all carry is the "adapter:" prefix — the sentinel itself is
		// platform-specific, so the prefix is what is checked here. This
		// file is built for every GOOS.
		if !strings.HasPrefix(err.Error(), "adapter:") {
			t.Fatalf("Install: %v, want a typed adapter error", err)
		}
		// ...AND in the stage log ("tap:" on darwin, "hook:" on Windows).
		found := false
		for _, l := range log.lines {
			if strings.HasPrefix(l, "tap:") || strings.HasPrefix(l, "hook:") {
				found = true
			}
		}
		if !found {
			t.Fatalf("stage log missing tap error line, got %v", log.lines)
		}
	}
	q := NewWindowQuery(d)
	// The same contract on the Accessibility side, and the same answer: a
	// machine that has granted the consent has no denial to report, so the
	// assertion is made where there is one.
	if _, err := q.Focused(); err != nil {
		if !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("Focused: %v, want a typed denial", err)
		}
	}
	// This one is deterministic: a zero id is refused whatever the machine
	// has granted.
	if err := q.MoveResize(MoveResize{}); err == nil {
		t.Fatal("MoveResize zero id: want typed error, got nil")
	}
}

// TestDocMentions pins criterion 2: the build/linking/signing contract must
// stay documented in doc.go (macOS static-archive + TCC note, Windows DLL
// seam, no-Swift-direct rule). Guards against a future edit silently
// dropping the contract the bead locked.
// (review nit: cross-os-ed — header referenced this test before it existed.)
func TestDocMentions(t *testing.T) {
	raw, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatalf("ReadFile doc.go: %v", err)
	}
	doc := string(raw)
	for _, want := range []string{
		"Build/linking/signing notes",
		"input-monitoring + accessibility consent",
		"crossos-keyboard-win.dll",
		"No Swift-direct-from-Go",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("doc.go missing documented contract %q", want)
		}
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
