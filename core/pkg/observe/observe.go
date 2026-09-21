package observe

import (
	"encoding/json"
	"fmt"
	"strings"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/pluginapi"
	"crossos/core/pkg/record"
)

// Fixture is one reported keyboard case: physical input + context + the
// expected decision/intent. Fixtures are drawn from the s4i matrix (same
// keycodes/modifiers/AppModes the plugin registers).
type Fixture struct {
	Name     string
	Event    event.Event
	Context  event.FastContext
	Decision pluginapi.Decision
	IntentID string // "" when Decision is Pass
}

// S4IFixtures returns the MVP 0 fixture set: the s4i matrix rows a user
// report would cite ("Ctrl+C doesn't work in Ghostty" and friends).
func S4IFixtures() []Fixture {
	const (
		vkC   = 0x43
		vkF4  = 0x73
		ctrl  = 1 << 0
		alt   = 1 << 2
		meta  = 1 << 3
		right = 0x27
	)
	kb := event.SourceKeyboard
	return []Fixture{
		{"finder-copy", event.Event{Type: event.EventKeyDown, Source: kb, KeyCode: vkC, Modifiers: ctrl},
			event.FastContext{AppID: "Finder", AppMode: event.AppModeNative},
			pluginapi.DecisionReplace, "clipboard.copy"},
		{"terminal-interrupt", event.Event{Type: event.EventKeyDown, Source: kb, KeyCode: vkC, Modifiers: ctrl},
			event.FastContext{AppID: "Terminal", AppMode: event.AppModeTerminal},
			pluginapi.DecisionPass, ""},
		{"finder-close", event.Event{Type: event.EventKeyDown, Source: kb, KeyCode: vkF4, Modifiers: alt},
			event.FastContext{AppID: "Finder", AppMode: event.AppModeNative},
			pluginapi.DecisionReplace, "window.close"},
		{"remote-passthrough", event.Event{Type: event.EventKeyDown, Source: kb, KeyCode: vkC, Modifiers: ctrl},
			event.FastContext{AppID: "RemoteDesktop", AppMode: event.AppModeRemote},
			pluginapi.DecisionPass, ""},
		{"snap-right", event.Event{Type: event.EventKeyDown, Source: kb, KeyCode: right, Modifiers: meta},
			event.FastContext{AppID: "Editor", AppMode: event.AppModeNative},
			pluginapi.DecisionReplace, "window.move"},
	}
}

// Mismatch is a reported-vs-replayed divergence: what the user saw vs what
// the trace says should have happened.
type Mismatch struct {
	FixtureName string
	Reported    string // user report, e.g. "nothing happened"
	Got         pluginapi.Decision
	Want        pluginapi.Decision
	Trace       record.Trace
}

// Reproduce runs fixtures through the router (no live input — the events
// are synthetic structs) and records each outcome. It returns mismatches
// where the replayed decision differs from the fixture expectation: the
// primary AI debug loop (reproduce → fix → replay → verify).
//
// Trace attachment is per-fixture (captured inside the loop), never
// positional over the mismatch list: mismatches are a subset of fixtures,
// one fixture can emit two entries, and rec may hold pre-existing traces.
// (review fix: cross-os-c0 — positional mapping debugged the wrong case.)
// Reproduce appends to rec; it does not own or clear it.
func Reproduce(rt *event.Router, rec *record.Recorder, fixtures []Fixture) []Mismatch {
	var out []Mismatch
	for _, f := range fixtures {
		before := len(rec.Traces())
		got := rt.Decide(f.Event, f.Context)
		rec.OnOutcome(got)
		trs := rec.Traces()
		var tr record.Trace
		if len(trs) > before {
			tr = trs[len(trs)-1]
		}
		if got.Decision != f.Decision {
			out = append(out, Mismatch{
				FixtureName: f.Name,
				Reported:    fmt.Sprintf("expected %v per matrix", f.Decision),
				Got:         got.Decision,
				Want:        f.Decision,
				Trace:       tr,
			})
		}
		if f.IntentID != "" && got.Intent.ID != f.IntentID {
			out = append(out, Mismatch{
				FixtureName: f.Name,
				Reported:    fmt.Sprintf("expected intent %q", f.IntentID),
				Got:         got.Decision,
				Want:        f.Decision,
				Trace:       tr,
			})
		}
	}
	return out
}

// Approval is the observe-mode gate record: a Would-execute entry the user
// explicitly approved (or refused). The dispatcher enforces; this package
// records the decision for audit.
type Approval struct {
	IntentID   string
	Capability string
	Approved   bool
	Reason     string
}

// ObserveLog renders the Would-execute log for one outcome: Physical →
// Context → Rule → Intent → Would-execute, with zero side effects (pure
// string building — calling this executes nothing).
//
// Privacy: takes record.Trace (params ALREADY redacted at record time),
// never event.Outcome (whose Parameters are pre-redaction). Display paths
// must only ever read redacted data — rendering an Outcome directly would
// print raw secrets. (review fix: cross-os-c0)
func ObserveLog(out event.Outcome, tr record.Trace) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Physical: key=%d mods=%d src=%s\n",
		out.Event.KeyCode, out.Event.Modifiers, out.Event.Source)
	fmt.Fprintf(&b, "Context: %s/%s/%s\n",
		out.Context.AppMode, out.Context.AppID, out.Context.WindowID)
	if out.WinnerRule == "" {
		b.WriteString("Rule: (no match)\n")
	} else {
		fmt.Fprintf(&b, "Rule: %s\n", out.WinnerRule)
	}
	if out.Intent.ID == "" {
		b.WriteString("Intent: (none)\n")
	} else {
		fmt.Fprintf(&b, "Intent: %s %s\n", out.Intent.ID, string(tr.Params))
	}
	switch out.Decision {
	case pluginapi.DecisionPass:
		b.WriteString("Would-execute: (pass through — nothing)\n")
	case pluginapi.DecisionConsume:
		b.WriteString("Would-execute: (consume — suppress, no dispatch)\n")
	case pluginapi.DecisionReplace:
		if out.Request != nil {
			fmt.Fprintf(&b, "Would-execute: %s (NOT executed — observe mode)\n",
				out.Request.Capability.ID)
		} else {
			b.WriteString("Would-execute: (replace blocked — fail-closed)\n")
		}
	}
	return b.String()
}

// Approve records an explicit user approval for a Would-execute entry.
// Approved=false is a refusal, never a silent enable (mirrors safety's
// no-auto-approve rule at the observe layer).
func Approve(out event.Outcome, approved bool, reason string) Approval {
	cap := ""
	if out.Request != nil {
		cap = out.Request.Capability.ID
	}
	return Approval{IntentID: out.Intent.ID, Capability: cap,
		Approved: approved, Reason: reason}
}

// SanitizeForDisplay renders traces safe for UI display: parameters pass
// through the recorder's redaction (already applied at record time), and
// this function asserts no raw secret keys survive in the rendered text.
func SanitizeForDisplay(trs []record.Trace) []string {
	var out []string
	for _, tr := range trs {
		raw, _ := json.Marshal(tr.Params)
		out = append(out, fmt.Sprintf("%s %s params=%s",
			tr.Winner, tr.Action, string(raw)))
	}
	return out
}

// SecretLeak scans rendered lines for secret values (test helper + runtime
// assertion for the UI layer: display paths must never show raw secrets).
func SecretLeak(lines []string, secrets []string) string {
	for _, l := range lines {
		for _, s := range secrets {
			if s != "" && strings.Contains(l, s) {
				return s
			}
		}
	}
	return ""
}

// ValidateFixtures fails loudly on fixture typos/renames: every fixture
// IntentID must resolve in the canonical registry. S4IFixtures is a
// hand-copy of the s4i matrix (core must not import plugins), so matrix
// edits silently desync fixtures — whoever edits
// plugins/windows-keyboard/matrix.go must update S4IFixtures, and this
// check makes the drift loud instead of silent. (review: cross-os-c0)
func ValidateFixtures(fixtures []Fixture) error {
	reg := intent.DefaultRegistry()
	for _, f := range fixtures {
		if f.IntentID == "" {
			continue
		}
		if _, ok := reg.Get(f.IntentID); !ok {
			return fmt.Errorf("observe: fixture %q targets unknown intent %q", f.Name, f.IntentID)
		}
	}
	return nil
}
