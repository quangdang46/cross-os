package record

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/pluginapi"
	"crossos/core/pkg/rule"
)

func mkOutcome(decision pluginapi.Decision, params string) event.Outcome {
	return event.Outcome{
		Event:    event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: 0x43, Modifiers: 0x02},
		Context:  event.FastContext{AppID: "ghostty", AppMode: "terminal"},
		Decision: decision,
		Intent: intent.Intent{ID: "clipboard.write", Version: 1,
			Source:     intent.SourceKeyboard,
			Parameters: json.RawMessage(params)},
		WinnerRule: "copy-rule",
		Losers:     []rule.Candidate{{RuleID: "other-rule"}},
		Traces: []event.StageLog{
			{Stage: "event", Detail: "key=67"},
			{Stage: "context", Detail: "terminal"},
			{Stage: "rule", Detail: "winner=copy-rule"},
			{Stage: "intent", Detail: "clipboard.write"},
			{Stage: "action", Detail: "authorized clipboard.write"},
			{Stage: "result", Detail: "replace"},
		},
		At: time.Now(),
	}
}

func TestDefaultIsMetadataOnly(t *testing.T) {
	r := NewRecorder()
	if r.Mode() != ModeMetadataOnly {
		t.Fatalf("default mode: got %v, want METADATA_ONLY", r.Mode())
	}
	// Zero value is OFF (fail-closed): unconfigured recorders keep nothing.
	var z Recorder
	z.OnOutcome(mkOutcome(pluginapi.DecisionReplace, `{"text":"hi"}`))
	if len(z.Traces()) != 0 {
		t.Fatal("zero-value recorder must record nothing")
	}
}

func TestSixStagesRecorded(t *testing.T) {
	r := NewRecorder()
	r.OnOutcome(mkOutcome(pluginapi.DecisionReplace, `{"text":"hi"}`))
	trs := r.Traces()
	if len(trs) != 1 {
		t.Fatalf("want 1 trace, got %d", len(trs))
	}
	stages := map[string]bool{}
	for _, s := range trs[0].Stages {
		stages[s.Stage] = true
	}
	for _, want := range []string{"event", "context", "rule", "intent", "action", "result"} {
		if !stages[want] {
			t.Fatalf("missing stage %q", want)
		}
	}
	if len(trs[0].Losers) != 1 || trs[0].Losers[0] != "other-rule" {
		t.Fatalf("losers not recorded: %+v", trs[0].Losers)
	}
}

// TestSecretsRedactedStoredAndReplayed is the bead's normative criterion:
// no typed text / clipboard content / password values in stored AND
// replayed traces, verified by grep over a fixture with synthetic secrets.
func TestSecretsRedactedStoredAndReplayed(t *testing.T) {
	r := NewRecorder()
	secrets := map[string]string{
		"text":     "S3CR3T-typed-text",
		"content":  "S3CR3T-file-content",
		"password": "S3CR3T-password",
		"zone":     "left-half", // non-secret control: must survive
	}
	raw, _ := json.Marshal(secrets)
	r.OnOutcome(mkOutcome(pluginapi.DecisionReplace, string(raw)))

	stored, _ := json.Marshal(r.Traces())
	for _, s := range []string{"S3CR3T-typed-text", "S3CR3T-file-content", "S3CR3T-password"} {
		if strings.Contains(string(stored), s) {
			t.Fatalf("stored trace leaks secret %q: %s", s, stored)
		}
	}
	if !strings.Contains(string(stored), "left-half") {
		t.Fatalf("non-secret param wrongly redacted: %s", stored)
	}
	for _, tier := range []ReplayTier{ReplayRaw, ReplaySemantic, ReplayCapability} {
		for _, step := range r.Replay(tier) {
			for _, s := range []string{"S3CR3T-typed-text", "S3CR3T-file-content", "S3CR3T-password"} {
				if strings.Contains(step.Describe, s) {
					t.Fatalf("replay tier %d leaks secret %q: %s", tier, s, step.Describe)
				}
			}
		}
	}
}

func TestFullTraceKeepsSecrets(t *testing.T) {
	// FULL_TRACE is explicit opt-in and keeps values (operator debugging).
	r := NewRecorder()
	r.SetMode(ModeFullTrace)
	r.OnOutcome(mkOutcome(pluginapi.DecisionReplace, `{"text":"visible"}`))
	stored, _ := json.Marshal(r.Traces())
	if !strings.Contains(string(stored), "visible") {
		t.Fatalf("full trace should keep values: %s", stored)
	}
}

func TestOffRecordsNothing(t *testing.T) {
	r := NewRecorder()
	r.SetMode(ModeOff)
	r.OnOutcome(mkOutcome(pluginapi.DecisionReplace, `{"text":"hi"}`))
	if len(r.Traces()) != 0 {
		t.Fatal("OFF must record nothing")
	}
}

func TestReplayDeterministic(t *testing.T) {
	// Same traces → same steps, twice. No live input involved.
	r := NewRecorder()
	r.OnOutcome(mkOutcome(pluginapi.DecisionReplace, `{"zone":"left-half"}`))
	r.OnOutcome(mkOutcome(pluginapi.DecisionPass, ``))
	a, b := r.Replay(ReplaySemantic), r.Replay(ReplaySemantic)
	if len(a) != 2 || len(b) != 2 || a[0] != b[0] || a[1] != b[1] {
		t.Fatalf("replay not deterministic: %+v vs %+v", a, b)
	}
	caps := r.Replay(ReplayCapability)
	if len(caps) != 2 || !strings.Contains(caps[0].Describe, "params=") {
		t.Fatalf("capability tier wrong: %+v", caps)
	}
}

func TestDryRunMarksWouldExecute(t *testing.T) {
	r := NewRecorder()
	r.SetDryRun(true)
	out := mkOutcome(pluginapi.DecisionReplace, `{"zone":"left-half"}`)
	out.Request = &intent.Request{Capability: mustCap(t, "clipboard.copyPath")}
	r.OnOutcome(out)
	trs := r.Traces()
	found := false
	for _, s := range trs[0].Stages {
		if strings.Contains(s.Detail, "Would-execute clipboard.copyPath") {
			found = true
		}
	}
	if !found {
		t.Fatalf("dry-run trace missing Would-execute: %+v", trs[0].Stages)
	}
}

func mustCap(t *testing.T, id string) intent.CapabilityDescriptor {
	t.Helper()
	d, ok := intent.DefaultRegistry().Get(id)
	if !ok {
		t.Fatalf("capability %q missing", id)
	}
	return d
}
