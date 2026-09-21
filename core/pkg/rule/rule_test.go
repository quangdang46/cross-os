package rule

import (
	"encoding/json"
	"testing"

	"crossos/core/pkg/intent"
	"crossos/core/pkg/pluginapi"
)

func mkIntent(id string) intent.Intent {
	return intent.Intent{ID: id, Version: 1, Source: intent.SourceKeyboard}
}

// overlapFixture is the plan's canonical conflict: global Ctrl+C (COPY) vs
// app-specific Ctrl+C (COPY) vs terminal Ctrl+C (INTERRUPT).
func overlapFixture() []Candidate {
	return []Candidate{
		{
			RuleID: "global-copy", PluginID: "windows-keyboard",
			Priority: PriorityGlobal, Specificity: 0, Scope: ScopeGlobal,
			Intent:  mkIntent("clipboard.copyPath"),
			Enabled: true, AppModeOK: true, RequiresOK: true,
		},
		{
			RuleID: "finder-copy", PluginID: "windows-keyboard",
			Priority: PriorityApp, Specificity: 2, Scope: ScopeApp,
			Intent:  mkIntent("clipboard.copyPath"),
			Enabled: true, AppModeOK: true, RequiresOK: true,
		},
		{
			RuleID: "terminal-interrupt", PluginID: "terminal-ux",
			Priority: PriorityTerminal, Specificity: 3, Scope: ScopeApp,
			Intent: intent.Intent{ID: "terminal.openAt", Version: 1,
				Source:     intent.SourceKeyboard,
				Parameters: json.RawMessage(`{"path":"/tmp"}`)},
			Enabled: true, AppModeOK: true, RequiresOK: true,
		},
	}
}

func TestMostSpecificWins(t *testing.T) {
	res := Resolve(overlapFixture())
	if res.RuleID != "terminal-interrupt" {
		t.Fatalf("want terminal-interrupt, got %q", res.RuleID)
	}
	if res.Decision != pluginapi.DecisionReplace {
		t.Fatalf("want Replace, got %v", res.Decision)
	}
	if len(res.Losers) != 2 {
		t.Fatalf("want 2 losers for recorder, got %d", len(res.Losers))
	}
	// Losers ranked: app-specific before global.
	if res.Losers[0].RuleID != "finder-copy" || res.Losers[1].RuleID != "global-copy" {
		t.Fatalf("loser order wrong: %+v", res.Losers)
	}
}

func TestFilterExcludesIneligible(t *testing.T) {
	cands := overlapFixture()
	cands[2].Enabled = false // terminal rule off → app-specific wins
	res := Resolve(cands)
	if res.RuleID != "finder-copy" {
		t.Fatalf("want finder-copy, got %q", res.RuleID)
	}
	cands[1].AppModeOK = false // app rule wrong mode → global wins
	res = Resolve(cands)
	if res.RuleID != "global-copy" {
		t.Fatalf("want global-copy, got %q", res.RuleID)
	}
}

func TestEmptyYieldsPass(t *testing.T) {
	res := Resolve(nil)
	if res.Decision != pluginapi.DecisionPass {
		t.Fatalf("want Pass on empty, got %v", res.Decision)
	}
	// All ineligible → same as empty.
	cands := overlapFixture()
	for i := range cands {
		cands[i].RequiresOK = false
	}
	res = Resolve(cands)
	if res.Decision != pluginapi.DecisionPass {
		t.Fatalf("want Pass on all-ineligible, got %v", res.Decision)
	}
}

func TestScopeNarrownessBreaksSpecificityTie(t *testing.T) {
	cands := []Candidate{
		{RuleID: "b", PluginID: "p", Priority: 10, Specificity: 2, Scope: ScopeGlobal,
			Intent: mkIntent("window.close"), Enabled: true, AppModeOK: true, RequiresOK: true},
		{RuleID: "a", PluginID: "p", Priority: 10, Specificity: 2, Scope: ScopeWindow,
			Intent: mkIntent("window.close"), Enabled: true, AppModeOK: true, RequiresOK: true},
	}
	if res := Resolve(cands); res.RuleID != "a" {
		t.Fatalf("want narrower scope a, got %q", res.RuleID)
	}
}

func TestPriorityBreaksScopeTie(t *testing.T) {
	cands := []Candidate{
		{RuleID: "a", PluginID: "p", Priority: 10, Specificity: 1, Scope: ScopeApp,
			Intent: mkIntent("window.close"), Enabled: true, AppModeOK: true, RequiresOK: true},
		{RuleID: "b", PluginID: "p", Priority: 80, Specificity: 1, Scope: ScopeApp,
			Intent: mkIntent("window.close"), Enabled: true, AppModeOK: true, RequiresOK: true},
	}
	if res := Resolve(cands); res.RuleID != "b" {
		t.Fatalf("want higher priority b, got %q", res.RuleID)
	}
}

func TestRuleIDSeededTieBreak(t *testing.T) {
	// Fully tied candidates resolve by RuleID ascending — deterministic,
	// independent of input order. The test pins this by shuffling input.
	mk := func(id string) Candidate {
		return Candidate{RuleID: id, PluginID: "p", Priority: 10,
			Specificity: 1, Scope: ScopeGlobal, Intent: mkIntent("window.close"),
			Enabled: true, AppModeOK: true, RequiresOK: true}
	}
	r1 := Resolve([]Candidate{mk("zzz"), mk("aaa")})
	r2 := Resolve([]Candidate{mk("aaa"), mk("zzz")})
	if r1.RuleID != "aaa" || r2.RuleID != "aaa" {
		t.Fatalf("tie-break not deterministic: %q vs %q", r1.RuleID, r2.RuleID)
	}
}

func TestValidateCandidates(t *testing.T) {
	if err := ValidateCandidates(overlapFixture()); err != nil {
		t.Fatalf("valid fixture rejected: %v", err)
	}
	dup := append(overlapFixture(), overlapFixture()[0])
	if err := ValidateCandidates(dup); err == nil {
		t.Fatal("duplicate RuleID accepted")
	}
	neg := overlapFixture()
	neg[0].Specificity = -1
	if err := ValidateCandidates(neg); err == nil {
		t.Fatal("negative specificity accepted")
	}
}
