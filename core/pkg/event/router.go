package event

import (
	"strconv"
	"time"

	"crossos/core/pkg/intent"
	"crossos/core/pkg/pluginapi"
	"crossos/core/pkg/rule"
)

// CompiledRule is a Core-compiled matcher entry: declarative/builtin rules
// only, plus Level B plugin rules compiled in ahead of time. External
// plugins NEVER run on this path — their rules are data here, not code.
type CompiledRule struct {
	// Match keys (exact-match lookup, no allocation-heavy evaluation).
	KeyCode   uint32
	Modifiers uint32    // required modifier mask (event.Modifiers & mask == mask)
	AppModes  []AppMode // empty = any mode
	AppIDs    []string  // empty = any app
	DeviceIDs []string  // empty = any device

	// EventTypes are the phases of the key this rule acts on; an empty list
	// means key-down only (see matches). The phase is part of the match
	// because a chord and its release are two intentions made with the same
	// fingers: Alt+Tab down opens the window switcher, Alt+Tab up commits it.
	// A matcher that saw only keycode+modifiers could not tell those rules
	// apart, so the pair collapsed into one arbitrary winner and the other
	// was reported as a conflict it never was.
	EventTypes []EventType

	// Claim (feeds rule.Candidate).
	RuleID      string
	PluginID    string
	Priority    int
	Specificity int
	Scope       rule.Scope
	Intent      intent.Intent

	// Emit selects Consume vs Replace: Emit=true dispatches the Intent
	// through the Capability API (REPLACE); Emit=false absorbs the event
	// with no dispatch (CONSUME). Declared at registration, never guessed.
	Emit bool
}

// matches reports whether the rule claims the event under the context.
// Pure, branch-light, no syscalls — safe for the callback path.
func (r CompiledRule) matches(ev Event, ctx FastContext) bool {
	// A rule claims the key-down phase unless it says otherwise. Defaulting
	// to key-down rather than "any phase" is what keeps every rule written
	// before phases existed behaving exactly as it did: the daemon used to
	// drop every non-key-down event before matching, so a key-down rule must
	// still not match a key-up or releasing the chord fires the action
	// twice. A rule that acts on release declares EventKeyUp.
	if len(r.EventTypes) == 0 {
		if ev.Type != EventKeyDown {
			return false
		}
	} else {
		claimed := false
		for _, t := range r.EventTypes {
			if t == ev.Type {
				claimed = true
				break
			}
		}
		if !claimed {
			return false
		}
	}
	if r.KeyCode != ev.KeyCode {
		return false
	}
	if ev.Modifiers&r.Modifiers != r.Modifiers {
		return false
	}
	if len(r.AppModes) > 0 {
		ok := false
		for _, m := range r.AppModes {
			if m == ctx.AppMode {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(r.AppIDs) > 0 {
		ok := false
		for _, a := range r.AppIDs {
			if a == ctx.AppID {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(r.DeviceIDs) > 0 {
		// Canonical source: ctx.DeviceID (decision input). ev.DeviceID is
		// transport redundancy for callbacks that only carry the event;
		// the adapter bead must wire one canonical string through both.
		// (review: cross-os-c0)
		dev := ctx.DeviceID
		if dev == "" {
			dev = ev.DeviceID
		}
		ok := false
		for _, d := range r.DeviceIDs {
			if d == dev {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// Outcome is one decided event: the decision plus the full stage trace
// (event → context → rule → intent → action → result) for the Recorder.
// The trace is DATA appended by the router; publishing it to observers
// happens after the callback returns (see Bus.Publish).
type Outcome struct {
	Event    Event
	Context  FastContext
	Decision pluginapi.Decision
	Intent   intent.Intent // set on Replace
	Request  *intent.Request
	// Trace stages (winner + losers, permission check, kill-switch state).
	WinnerRule string
	Losers     []rule.Candidate
	Traces     []StageLog
	At         time.Time
}

// StageLog is one pipeline stage record.
type StageLog struct {
	Stage  string // "event","context","rule","intent","action","result"
	Detail string
}

// Router is the deterministic synchronous decision path. It is built once
// (Compile) and thereafter read-only — safe to call Decide from the native
// callback with no locks. Slow-path work (IPC, logging, enrichment) MUST
// NOT enter Decide; observers receive the Outcome via Bus after return.
type Router struct {
	rules    []CompiledRule
	registry *intent.Registry
	grants   map[string][]intent.Permission // pluginID → granted permissions
	killed   func() bool                    // kill-switch probe (2ha binding)
}

// Compile binds the matcher to the capability registry and the permission
// grants. Grants are snapshotted at compile time; re-Compile on change.
func Compile(rules []CompiledRule, reg *intent.Registry,
	grants map[string][]intent.Permission, killed func() bool) *Router {
	if killed == nil {
		killed = func() bool { return false }
	}
	return &Router{rules: rules, registry: reg, grants: grants, killed: killed}
}

// Decide resolves one event synchronously. Contract bindings (bead criterion):
//   - 2ha: kill-switch short-circuits the path (PASS: interception disabled,
//     OS handles keys normally — see inline note at the check).
//   - 4lm: Decision enum is the only vocabulary.
//   - 9tb: Intent/Capability is the action shape; Authorize fail-closed.
//   - xtk: rule.Resolve picks the winner.
//
// No IPC, no logging I/O, no enrichment inside Decide — trace records are
// appended to Outcome.Traces (plain strings) for post-return publishing.
func (rt *Router) Decide(ev Event, ctx FastContext) Outcome {
	out := Outcome{Event: ev, Context: ctx, At: time.Now()}
	out.Traces = append(out.Traces, StageLog{"event", describeEvent(ev)})
	out.Traces = append(out.Traces, StageLog{"context",
		string(ctx.AppMode) + "/" + ctx.AppID + "/" + ctx.WindowID})

	// 2ha binding: kill switch = interception disabled. Disabled means the
	// hook is gone and the OS handles keys normally — i.e. PASS. Consuming
	// here would swallow every keystroke with nothing dispatched (bricked
	// keyboard, not a safe machine). Passthrough IS the safe state: it
	// equals disabled/uninstalled behavior. (review correction: cross-os-c0)
	if rt.killed() {
		out.Decision = pluginapi.DecisionPass
		out.Traces = append(out.Traces, StageLog{"result", "kill-switch active: pass-through"})
		return out
	}

	// Synthetic self-guard: CrossOS-injected events (tagged via dwExtraInfo
	// injectTag upstream, checked by the native callback) must never
	// re-enter matching — otherwise inject→match→inject loops. Belt and
	// suspenders: the callback filters first, Decide refuses second.
	// (review: cross-os-c0; owner bead cross-os-wge)
	if ev.Source == SourceSynthetic {
		out.Decision = pluginapi.DecisionPass
		out.Traces = append(out.Traces, StageLog{"result", "synthetic event: pass-through"})
		return out
	}

	// Fast matcher → candidates.
	var cands []rule.Candidate
	for i := range rt.rules {
		r := &rt.rules[i]
		if !r.matches(ev, ctx) {
			continue
		}
		cands = append(cands, rule.Candidate{
			RuleID: r.RuleID, PluginID: r.PluginID,
			Priority: r.Priority, Specificity: r.Specificity, Scope: r.Scope,
			Intent:  r.Intent,
			Enabled: true, AppModeOK: true, RequiresOK: true,
		})
	}
	if len(cands) == 0 {
		out.Decision = pluginapi.DecisionPass
		out.Traces = append(out.Traces, StageLog{"result", "no rule matched: pass"})
		return out
	}

	// xtk binding: exactly one winner.
	res := rule.Resolve(cands)
	out.WinnerRule = res.RuleID
	out.Losers = res.Losers
	out.Traces = append(out.Traces, StageLog{"rule",
		"winner=" + res.RuleID + " losers=" + itoa(len(res.Losers))})

	// Find the winning compiled rule for Emit + permission binding.
	var win *CompiledRule
	for i := range rt.rules {
		if rt.rules[i].RuleID == res.RuleID {
			win = &rt.rules[i]
			break
		}
	}
	if win == nil {
		out.Decision = pluginapi.DecisionPass
		out.Traces = append(out.Traces, StageLog{"result", "winner rule vanished: pass"})
		return out
	}
	out.Traces = append(out.Traces, StageLog{"intent", win.Intent.ID})

	// Consume-vs-Replace predicate (closes TODO(dispatcher) from rule):
	// Emit=false absorbs with no dispatch.
	if !win.Emit {
		out.Decision = pluginapi.DecisionConsume
		out.Traces = append(out.Traces, StageLog{"result", "consumed (Emit=false)"})
		return out
	}

	// 9tb binding: Authorize fail-closed against registry + grants.
	req, err := intent.Authorize(rt.registry, win.PluginID, win.Intent,
		rt.grants[win.PluginID])
	if err != nil {
		out.Decision = pluginapi.DecisionConsume
		out.Traces = append(out.Traces, StageLog{"action", "authorize failed: " + err.Error()})
		out.Traces = append(out.Traces, StageLog{"result", "consumed (fail-closed)"})
		return out
	}
	out.Request = req
	out.Intent = win.Intent
	out.Decision = pluginapi.DecisionReplace
	out.Traces = append(out.Traces, StageLog{"action", "authorized " + req.Capability.ID})
	out.Traces = append(out.Traces, StageLog{"result", "replace: dispatch " + req.Capability.ID})
	return out
}

func describeEvent(ev Event) string {
	return "key=" + strconv.Itoa(int(ev.KeyCode)) + " mods=" + strconv.Itoa(int(ev.Modifiers)) +
		" src=" + string(ev.Source)
}

func itoa(n int) string { return strconv.Itoa(n) }
