package rule

import (
	"fmt"
	"sort"

	"crossos/core/pkg/intent"
	"crossos/core/pkg/pluginapi"
)

// Scope is the narrowing tier a rule claims. Narrower scope outranks wider
// scope at equal specificity — this is what makes an app-specific Ctrl+C beat
// the global Ctrl+C without depending on registration order.
type Scope int

const (
	ScopeGlobal Scope = iota // applies everywhere
	ScopeApp                 // one app / app category
	ScopeWindow              // one window class / role
	ScopeDevice              // one physical device
)

// Priority baselines (plan §3.5b): terminal/VM = 100, app-specific = 80,
// global = 10. Rules declare their own priority; these are the convention.
const (
	PriorityTerminal = 100
	PriorityApp      = 80
	PriorityGlobal   = 10
)

// Candidate is one rule claiming an event, after the filter stage has
// admitted it (enabled, AppMode matches, requires satisfiable). Filter inputs
// live here as facts so Resolve stays pure.
type Candidate struct {
	RuleID      string
	PluginID    string
	Priority    int
	Specificity int // computed: app+window+device+requires match depth
	Scope       Scope
	Intent      intent.Intent
	Enabled     bool
	AppModeOK   bool
	RequiresOK  bool
}

// Eligible reports whether the candidate survived filtering.
func (c Candidate) Eligible() bool {
	return c.Enabled && c.AppModeOK && c.RequiresOK
}

// Resolution is the single winner plus the decision and, for the Recorder,
// the losers in rank order.
type Resolution struct {
	RuleID   string
	PluginID string
	Intent   intent.Intent
	Decision pluginapi.Decision
	Losers   []Candidate // ranked, winner excluded — feeds the Recorder
}

// Resolve picks exactly ONE winner: eligible candidates ranked by
// specificity (desc), then scope narrowness (desc), then priority (desc),
// then RuleID (asc, deterministic seeded tie-break — no map order, no
// registration order). Empty eligible set → DecisionPass with zero Intent.
//
// Decision mapping: a winner that substitutes a native action yields
// DecisionReplace; the spike-level distinction between Consume and Replace
// (whether the winner emits a replacement) is owned by the dispatcher, so
// Resolve reports Replace and the dispatcher narrows it. No winner at all
// yields DecisionPass.
func Resolve(cands []Candidate) Resolution {
	elig := make([]Candidate, 0, len(cands))
	for _, c := range cands {
		if c.Eligible() {
			elig = append(elig, c)
		}
	}
	if len(elig) == 0 {
		return Resolution{Decision: pluginapi.DecisionPass}
	}
	sort.SliceStable(elig, func(i, j int) bool {
		a, b := elig[i], elig[j]
		if a.Specificity != b.Specificity {
			return a.Specificity > b.Specificity
		}
		if a.Scope != b.Scope {
			return a.Scope > b.Scope
		}
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		return a.RuleID < b.RuleID
	})
	w := elig[0]
	return Resolution{
		RuleID:   w.RuleID,
		PluginID: w.PluginID,
		Intent:   w.Intent,
		Decision: pluginapi.DecisionReplace,
		Losers:   append([]Candidate(nil), elig[1:]...),
	}
}

// ValidateCandidates fails on duplicate RuleIDs or negative specificity —
// both are authoring bugs, caught before resolution, not during.
func ValidateCandidates(cands []Candidate) error {
	seen := map[string]struct{}{}
	for _, c := range cands {
		if c.RuleID == "" {
			return fmt.Errorf("rule: candidate with empty RuleID")
		}
		if _, dup := seen[c.RuleID]; dup {
			return fmt.Errorf("rule: duplicate RuleID %q", c.RuleID)
		}
		seen[c.RuleID] = struct{}{}
		if c.Specificity < 0 {
			return fmt.Errorf("rule: negative specificity on %q", c.RuleID)
		}
	}
	return nil
}
