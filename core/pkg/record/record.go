package record

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"crossos/core/pkg/event"
	"crossos/core/pkg/pluginapi"
)

// Mode is the recorder privacy mode. METADATA_ONLY is the default —
// recorders must be constructed with NewRecorder, which defaults
// correctly; the zero value is deliberately OFF (fail-closed: an
// unconfigured recorder records nothing, never everything).
type Mode int

const (
	ModeOff          Mode = iota // record nothing
	ModeMetadataOnly             // default: redact text/clipboard/passwords
	ModeDebug                    // + structural params, still redacted secrets
	ModeFullTrace                // explicit opt-in: everything (never default)
)

// redacted is the placeholder written over secrets.
const redacted = "[REDACTED]"

// Trace is one recorded event: the router Outcome plus recorder metadata.
// Parameters arrive here as the router logged them; ApplyMode redacts at
// record time so secrets never reach storage.
type Trace struct {
	At       time.Time
	Event    event.Event
	Context  event.FastContext
	Decision pluginapi.Decision
	IntentID string
	Action   string // capability ID dispatched ("" on pass/consume)
	Winner   string
	Losers   []string
	Stages   []event.StageLog
	Params   json.RawMessage // redacted per mode at record time
}

// Recorder taps the Bus as an Observer (slow path only) and stores traces.
type Recorder struct {
	mu     sync.Mutex
	mode   Mode
	traces []Trace
	dryRun bool // observe mode: log Would-execute, execute nothing
}

// NewRecorder returns a Recorder in the default mode (METADATA_ONLY).
func NewRecorder() *Recorder {
	return &Recorder{mode: ModeMetadataOnly}
}

// SetMode changes the privacy mode. ModeFullTrace requires explicit opt-in
// at the call site — there is no path that selects it by default.
func (r *Recorder) SetMode(m Mode) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mode = m
}

// Mode returns the current privacy mode.
func (r *Recorder) Mode() Mode {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mode
}

// SetDryRun enables/disables observe mode.
func (r *Recorder) SetDryRun(dry bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dryRun = dry
}

// DryRun reports observe mode.
func (r *Recorder) DryRun() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dryRun
}

// Name satisfies event.Observer.
func (r *Recorder) Name() string { return "recorder" }

// OnOutcome implements event.Observer: called by Bus.Drain on the slow
// path, never on the callback path.
func (r *Recorder) OnOutcome(out event.Outcome) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.mode == ModeOff {
		return
	}
	tr := Trace{
		At:       out.At,
		Event:    out.Event,
		Context:  out.Context,
		Decision: out.Decision,
		Winner:   out.WinnerRule,
		Stages:   append([]event.StageLog(nil), out.Traces...),
	}
	for _, l := range out.Losers {
		tr.Losers = append(tr.Losers, l.RuleID)
	}
	if out.Request != nil {
		tr.IntentID = out.Intent.ID
		tr.Action = out.Request.Capability.ID
		tr.Params = redactParams(out.Intent.Parameters, r.mode)
	} else {
		tr.IntentID = out.Intent.ID
		tr.Params = redactParams(out.Intent.Parameters, r.mode)
	}
	if r.dryRun && tr.Action != "" {
		tr.Stages = append(tr.Stages, event.StageLog{
			Stage:  "result",
			Detail: "dry-run: Would-execute " + tr.Action + " (not executed)",
		})
	}
	r.traces = append(r.traces, tr)
}

// Traces returns a copy of stored traces.
func (r *Recorder) Traces() []Trace {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Trace(nil), r.traces...)
}

// Clear drops stored traces.
func (r *Recorder) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.traces = nil
}

// secretKeys are parameter names whose values are secrets at
// METADATA_ONLY/DEBUG. FULL_TRACE keeps them (explicit opt-in only).
var secretKeys = map[string]bool{
	"text": true, "content": true, "template": true,
	"password": true, "secret": true, "clipboard": true,
}

// redactParams redacts secret parameter values per mode. Top-level keys
// only (safe TODAY: all canonical v1 InputSchemas are flat primitives).
// TODO(recorder): go recursive when schemas gain nesting — a nested
// {"config":{"password":"x"}} currently passes through.
// TODO(secrets): exact-match list covers every v1 schema key; switch to
// substring matching (token/apiKey/passwd/...) when the capability set
// grows. Unparseable params drop to redacted-empty, never stored raw.
// (review follow-ups: cross-os-c0)
func redactParams(raw json.RawMessage, m Mode) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	if m == ModeFullTrace {
		return raw
	}
	var params map[string]json.RawMessage
	if err := json.Unmarshal(raw, &params); err != nil {
		return json.RawMessage(`{}`)
	}
	out := make(map[string]json.RawMessage, len(params))
	for k, v := range params {
		lk := strings.ToLower(k)
		if secretKeys[lk] && (m == ModeMetadataOnly || m == ModeDebug) {
			out[k] = json.RawMessage(`"` + redacted + `"`)
			continue
		}
		// At METADATA_ONLY, free-form string values that are NOT secrets
		// still pass (zone names, paths in non-secret keys). At DEBUG the
		// same rule holds — DEBUG adds structural detail elsewhere, not
		// secret exfiltration.
		out[k] = v
	}
	enc, err := json.Marshal(out)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return enc
}

// ReplayTier selects the replay granularity.
type ReplayTier int

const (
	ReplayRaw        ReplayTier = 1 // key down/up sequences
	ReplaySemantic   ReplayTier = 2 // intents
	ReplayCapability ReplayTier = 3 // capability invocations
)

// ReplayedStep is one deterministic replay step (no live input, no secrets).
type ReplayedStep struct {
	Tier     ReplayTier
	Describe string
}

// Replay replays stored traces deterministically without live input. It
// replays semantic/capability tiers (the primary debug loop); raw secrets
// never appear because they were redacted at record time.
func (r *Recorder) Replay(tier ReplayTier) []ReplayedStep {
	r.mu.Lock()
	defer r.mu.Unlock()
	var steps []ReplayedStep
	for _, tr := range r.traces {
		switch tier {
		case ReplayRaw:
			steps = append(steps, ReplayedStep{tier, fmt.Sprintf(
				"key=%d mods=%d decision=%d", tr.Event.KeyCode,
				tr.Event.Modifiers, tr.Decision)})
		case ReplaySemantic:
			steps = append(steps, ReplayedStep{tier, fmt.Sprintf(
				"intent=%s winner=%s", tr.IntentID, tr.Winner)})
		case ReplayCapability:
			act := tr.Action
			if act == "" {
				act = "(no dispatch)"
			}
			steps = append(steps, ReplayedStep{tier, fmt.Sprintf(
				"capability=%s params=%s", act, string(tr.Params))})
		}
	}
	return steps
}
