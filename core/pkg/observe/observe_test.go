package observe

import (
	"encoding/json"
	"strings"
	"testing"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/pluginapi"
	"crossos/core/pkg/record"
	"crossos/core/pkg/rule"
)

// testRouter mirrors the s4i matrix (copy/close/snap rules) over the
// canonical registry — the same shape the plugin registers, without
// importing the plugin module (core must not depend on plugins).
func testRouter() *event.Router {
	reg := intent.DefaultRegistry()
	mk := func(id, params string) intent.Intent {
		return intent.Intent{ID: id, Version: 1, Source: intent.SourceKeyboard,
			Parameters: json.RawMessage(params)}
	}
	rules := []event.CompiledRule{
		{KeyCode: 0x73, Modifiers: 1 << 2, AppModes: []event.AppMode{"native", "terminal"},
			RuleID: "kb.alt-f4", PluginID: "windows-keyboard",
			Priority: rule.PriorityGlobal, Specificity: 1, Scope: rule.ScopeGlobal,
			Intent: mk("window.close", ""), Emit: true},
		{KeyCode: 0x43, Modifiers: 1 << 0, AppModes: []event.AppMode{"native"},
			RuleID: "kb.ctrl-c", PluginID: "windows-keyboard",
			Priority: rule.PriorityApp, Specificity: 1, Scope: rule.ScopeApp,
			Intent: mk("clipboard.copy", ""), Emit: true},
		{KeyCode: 0x27, Modifiers: 1 << 3, AppModes: []event.AppMode{"native", "terminal"},
			RuleID: "kb.snap-r", PluginID: "windows-keyboard",
			Priority: rule.PriorityGlobal, Specificity: 1, Scope: rule.ScopeGlobal,
			Intent: mk("window.move", `{"zone":"right-half"}`), Emit: true},
	}
	grants := map[string][]intent.Permission{
		"windows-keyboard": {intent.PermInputIntercept, intent.PermAccessControl},
	}
	return event.Compile(rules, reg, grants, nil)
}

func TestReproduceNoMismatches(t *testing.T) {
	// Fixtures drawn from the s4i matrix replay deterministically with zero
	// mismatches against the same-shape router.
	rt := testRouter()
	rec := record.NewRecorder()
	mm := Reproduce(rt, rec, S4IFixtures())
	if len(mm) != 0 {
		t.Fatalf("mismatches on clean matrix: %+v", mm)
	}
}

func TestReproduceFindsMismatch(t *testing.T) {
	// A broken router (grants stripped → everything fail-closed Consume)
	// reproduces the reported mismatch deterministically.
	reg := intent.DefaultRegistry()
	broken := event.Compile([]event.CompiledRule{
		{KeyCode: 0x43, Modifiers: 1 << 0, AppModes: []event.AppMode{"native"},
			RuleID: "kb.ctrl-c", PluginID: "windows-keyboard",
			Priority: rule.PriorityApp, Specificity: 1, Scope: rule.ScopeApp,
			Intent: intent.Intent{ID: "clipboard.copy", Version: 1, Source: intent.SourceKeyboard},
			Emit:   true},
	}, reg, map[string][]intent.Permission{}, nil) // no grants → fail-closed
	rec := record.NewRecorder()
	mm := Reproduce(broken, rec, S4IFixtures()[:1])
	if len(mm) == 0 {
		t.Fatal("broken router produced no mismatch")
	}
	if mm[0].FixtureName != "finder-copy" {
		t.Fatalf("wrong fixture: %+v", mm[0])
	}
}

func TestObserveLogHasZeroSideEffects(t *testing.T) {
	// ObserveLog is pure string building: calling it dispatches nothing.
	// Takes the redacted record.Trace, never the raw Outcome (review fix:
	// cross-os-c0 — rendering Outcome.Parameters printed raw secrets).
	rt := testRouter()
	rec := record.NewRecorder()
	out := rt.Decide(
		event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: 0x43, Modifiers: 1 << 0},
		event.FastContext{AppID: "Finder", AppMode: event.AppModeNative})
	rec.OnOutcome(out)
	tr := rec.Traces()[0]
	log := ObserveLog(out, tr)
	for _, want := range []string{"Physical:", "Context:", "Rule:", "Intent:", "Would-execute:"} {
		if !strings.Contains(log, want) {
			t.Fatalf("observe log missing %q:\n%s", want, log)
		}
	}
	if !strings.Contains(log, "clipboard.copy") {
		t.Fatalf("observe log missing intent:\n%s", log)
	}
	// Pass case renders the nothing-branch.
	out2 := rt.Decide(
		event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: 0x41, Modifiers: 1 << 0},
		event.FastContext{AppID: "x", AppMode: event.AppModeNative})
	rec.OnOutcome(out2)
	tr2 := rec.Traces()[1]
	if !strings.Contains(ObserveLog(out2, tr2), "pass through") {
		t.Fatalf("pass observe wrong:\n%s", ObserveLog(out2, tr2))
	}
}

func TestObserveLogRedactsSecrets(t *testing.T) {
	// Regression (review P1: cross-os-c0): ObserveLog must never print raw
	// secret-bearing intent params — it reads the redacted Trace.
	rec := record.NewRecorder()
	out := event.Outcome{
		Intent: intent.Intent{ID: "clipboard.write", Version: 1,
			Source:     intent.SourceKeyboard,
			Parameters: json.RawMessage(`{"text":"S3CR3T-via-observe"}`)},
		Decision: pluginapi.DecisionReplace,
	}
	rec.OnOutcome(out)
	log := ObserveLog(out, rec.Traces()[0])
	if strings.Contains(log, "S3CR3T-via-observe") {
		t.Fatalf("observe log leaks secret:\n%s", log)
	}
}

func TestMismatchTraceBelongsToFixture(t *testing.T) {
	// Regression (review P0: cross-os-c0): mismatching NON-first fixture
	// must carry its own trace, not fixture 0's. Old code mapped
	// positionally and debugged the wrong case.
	reg := intent.DefaultRegistry()
	// Router that only breaks remote-passthrough (fixture index 3): same
	// shape as testRouter (copy/close/snap all present) PLUS a bogus
	// remote-matching rule that steals the remote fixture.
	partial := event.Compile([]event.CompiledRule{
		{KeyCode: 0x73, Modifiers: 1 << 2, AppModes: []event.AppMode{"native", "terminal"},
			RuleID: "kb.alt-f4", PluginID: "windows-keyboard",
			Priority: rule.PriorityGlobal, Specificity: 1, Scope: rule.ScopeGlobal,
			Intent: intent.Intent{ID: "window.close", Version: 1, Source: intent.SourceKeyboard},
			Emit:   true},
		{KeyCode: 0x43, Modifiers: 1 << 0, AppModes: []event.AppMode{"native"},
			RuleID: "kb.ctrl-c", PluginID: "windows-keyboard",
			Priority: rule.PriorityApp, Specificity: 1, Scope: rule.ScopeApp,
			Intent: intent.Intent{ID: "clipboard.copy", Version: 1, Source: intent.SourceKeyboard},
			Emit:   true},
		{KeyCode: 0x27, Modifiers: 1 << 3, AppModes: []event.AppMode{"native", "terminal"},
			RuleID: "kb.snap-r", PluginID: "windows-keyboard",
			Priority: rule.PriorityGlobal, Specificity: 1, Scope: rule.ScopeGlobal,
			Intent: intent.Intent{ID: "window.move", Version: 1, Source: intent.SourceKeyboard,
				Parameters: json.RawMessage(`{"zone":"right-half"}`)},
			Emit: true},
		{KeyCode: 0x43, Modifiers: 1 << 0, AppModes: []event.AppMode{"remote"},
			RuleID: "kb.wrong-remote", PluginID: "windows-keyboard",
			Priority: rule.PriorityGlobal, Specificity: 0, Scope: rule.ScopeGlobal,
			Intent: intent.Intent{ID: "clipboard.copy", Version: 1, Source: intent.SourceKeyboard},
			Emit:   true},
	}, reg, map[string][]intent.Permission{
		"windows-keyboard": {intent.PermInputIntercept, intent.PermAccessControl},
	}, nil)
	rec := record.NewRecorder()
	// Pre-existing traces must not shift attachment either.
	rec.OnOutcome(event.Outcome{Decision: pluginapi.DecisionPass})
	mm := Reproduce(partial, rec, S4IFixtures())
	if len(mm) == 0 {
		t.Fatal("expected remote-passthrough mismatch")
	}
	for _, m := range mm {
		if m.FixtureName != "remote-passthrough" {
			t.Fatalf("unexpected mismatch: %+v", m)
		}
		if m.Trace.Context.AppMode != event.AppModeRemote {
			t.Fatalf("trace belongs to wrong fixture: %+v", m.Trace.Context)
		}
	}
}

func TestValidateFixtures(t *testing.T) {
	if err := ValidateFixtures(S4IFixtures()); err != nil {
		t.Fatalf("bundled fixtures invalid: %v", err)
	}
	if err := ValidateFixtures([]Fixture{{Name: "bad", IntentID: "nope.not-real"}}); err == nil {
		t.Fatal("unknown intent accepted")
	}
}

func TestApproveRefusal(t *testing.T) {
	// Refusal is explicit, never silent enable.
	rt := testRouter()
	out := rt.Decide(
		event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: 0x43, Modifiers: 1 << 0},
		event.FastContext{AppID: "Finder", AppMode: event.AppModeNative})
	a := Approve(out, false, "not yet")
	if a.Approved || a.Capability != "clipboard.copy" {
		t.Fatalf("bad approval record: %+v", a)
	}
}

func TestNoSecretsInDisplay(t *testing.T) {
	// Stored + displayed traces carry no raw secrets (bead criterion).
	rec := record.NewRecorder()
	rt := testRouter()
	out := rt.Decide(
		event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: 0x43, Modifiers: 1 << 0},
		event.FastContext{AppID: "Finder", AppMode: event.AppModeNative})
	rec.OnOutcome(out)
	lines := SanitizeForDisplay(rec.Traces())
	if leak := SecretLeak(lines, []string{"S3CR3T"}); leak != "" {
		t.Fatalf("display leak: %q", leak)
	}
	// And a trace with a secret-bearing intent stays redacted end to end.
	rec2 := record.NewRecorder()
	rec2.OnOutcome(event.Outcome{
		Intent: intent.Intent{ID: "clipboard.write", Version: 1,
			Source:     intent.SourceKeyboard,
			Parameters: json.RawMessage(`{"text":"S3CR3T-payload"}`)},
		Decision: pluginapi.DecisionReplace,
	})
	lines2 := SanitizeForDisplay(rec2.Traces())
	if leak := SecretLeak(lines2, []string{"S3CR3T-payload"}); leak != "" {
		t.Fatalf("secret survived to display: %q in %v", leak, lines2)
	}
}
