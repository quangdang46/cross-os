// Page data source tests (bead cross-os-jzj).
//
// The shell's ten settings pages resolve their `source` strings through the
// methods registered here, so the assertions are mostly about the contract
// the shell codes against: snake_case wire names, [] for an empty source, a
// stable order across polls, and a write that either lands whole or changes
// nothing. The wire* decode structs are declared independently of the handler
// types on purpose — decoding into the handler's own struct would pass even
// with every json tag wrong, and the tags are the contract.

package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/ipc"
	"crossos/core/pkg/profiles"
	"crossos/core/pkg/rule"
	"crossos/core/pkg/safety"
	builtin "crossos/core/rules"
)

type wireMatrixRow struct {
	RuleID   string   `json:"rule_id"`
	Plugin   string   `json:"plugin"`
	Action   string   `json:"action"`
	Keys     string   `json:"keys"`
	Contexts []string `json:"contexts"`
	Enabled  bool     `json:"enabled"`
}

type wireZoneRow struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
}

type wireTrialState struct {
	Plugin      string `json:"plugin"`
	State       string `json:"state"`
	RemainingMS int64  `json:"remaining_ms"`
	TimeoutMS   int64  `json:"timeout_ms"`
}

type wireCapabilityRow struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Plugin    string   `json:"plugin"`
	Available bool     `json:"available"`
	Reason    string   `json:"reason"`
	RuleIDs   []string `json:"rule_ids"`
	Enabled   int      `json:"enabled"`
	Total     int      `json:"total"`
	Live      bool     `json:"live"`
}

type wireProfileRow struct {
	ID           string              `json:"id"`
	Label        string              `json:"label"`
	Description  string              `json:"description"`
	Active       bool                `json:"active"`
	Capabilities []wireCapabilityRow `json:"capabilities"`
}

type wireConflictRow struct {
	Keys   string   `json:"keys"`
	Winner string   `json:"winner"`
	Losers []string `json:"losers"`
	Rules  []struct {
		RuleID string `json:"rule_id"`
		Plugin string `json:"plugin"`
		Action string `json:"action"`
	} `json:"rules"`
}

type wireTraceStage struct {
	Stage  string `json:"stage"`
	Detail string `json:"detail"`
}

type wireTraceRow struct {
	At       string           `json:"at"`
	Decision string           `json:"decision"`
	Winner   string           `json:"winner"`
	Losers   []string         `json:"losers"`
	Intent   string           `json:"intent"`
	Action   string           `json:"action"`
	Params   string           `json:"params"`
	Stages   []wireTraceStage `json:"stages"`
	Event    struct {
		Keys    string `json:"keys"`
		Source  string `json:"source"`
		KeyCode uint32 `json:"key_code"`
	} `json:"event"`
	Context struct {
		AppID   string `json:"app_id"`
		AppMode string `json:"app_mode"`
	} `json:"context"`
}

type wirePluginMetaRow struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Permissions []string `json:"permissions"`
	Loaded      bool     `json:"loaded"`
	Reason      string   `json:"reason"`
}

type wireOnboardingStep struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Done   bool   `json:"done"`
	Detail string `json:"detail"`
}

type wireOnboardingRow struct {
	Completed   bool                 `json:"completed"`
	CurrentStep string               `json:"current_step"`
	Steps       []wireOnboardingStep `json:"steps"`
	Ready       int                  `json:"ready"`
	Total       int                  `json:"total"`
	Readiness   []readinessRow       `json:"readiness"`
}

// decode runs a handler's result through JSON and decodes it into the wire
// shape the shell sees.
func decode[T any](t *testing.T, res any, rerr *ipc.RPCError) T {
	t.Helper()
	if rerr != nil {
		t.Fatalf("handler failed: %d %s", rerr.Code, rerr.Message)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return out
}

// decodeRows decodes a list result and refuses a null: an empty source must
// reach the shell as [] so the page can say "nothing here" instead of
// tripping over a missing value.
func decodeRows[T any](t *testing.T, res any, rerr *ipc.RPCError) []T {
	t.Helper()
	if rerr != nil {
		t.Fatalf("handler failed: %d %s", rerr.Code, rerr.Message)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if !strings.HasPrefix(string(raw), "[") {
		t.Fatalf("list result is %s; an empty collection must be []", raw)
	}
	var out []T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return out
}

func testCore(t *testing.T) *Core {
	t.Helper()
	c, err := NewCore(builtin.All(), builtin.Grants())
	if err != nil {
		t.Fatalf("NewCore: %v", err)
	}
	return c
}

// TestEveryPageDataMethodIsRegistered pins the settings-page data sources in
// the method table. A handler left out of methods() is dead code the shell can
// never reach, and only a direct call would ever see it.
func TestEveryPageDataMethodIsRegistered(t *testing.T) {
	c := testCore(t)
	reg := c.methods()
	for _, name := range []string{
		"config.getMatrix", "config.getOverrides", "config.setOverride",
		"config.getZones", "config.setZones", "core.commands",
		"core.pluginSchemas", "safety.ownershipAudit", "safety.trialState",
		"core.readiness",
		"core.profiles", "core.profileApply", "core.conflicts",
		"core.traces", "core.pluginMeta", "core.onboardingState",
	} {
		if _, ok := reg[name]; !ok {
			t.Errorf("page data source %q is not in the method table", name)
		}
	}
}

// TestEmptyPageListsAreArrays: every list method answers [] when it has
// nothing, including on a daemon that has never been configured.
func TestEmptyPageListsAreArrays(t *testing.T) {
	c := testCore(t)
	type call struct {
		name string
		res  any
		err  *ipc.RPCError
	}
	matrix, matrixErr := c.handleGetMatrix(nil)
	overrides, overridesErr := c.handleGetOverrides(nil)
	zones, zonesErr := c.handleGetZones(nil)
	commands, commandsErr := c.handleCoreCommands(nil)
	schemas, schemasErr := c.handlePluginSchemas(nil)
	audit, auditErr := c.handleOwnershipAudit(nil)
	readiness, readinessErr := c.handleReadiness(nil)
	profilesRes, profilesErr := c.handleCoreProfiles(nil)
	conflicts, conflictsErr := c.handleConflicts(nil)
	traces, tracesErr := c.handleTraces(nil)
	meta, metaErr := c.handlePluginMeta(nil)
	for _, tc := range []call{
		{"config.getMatrix", matrix, matrixErr},
		{"config.getOverrides", overrides, overridesErr},
		{"config.getZones", zones, zonesErr},
		{"core.commands", commands, commandsErr},
		{"core.pluginSchemas", schemas, schemasErr},
		{"safety.ownershipAudit", audit, auditErr},
		{"core.readiness", readiness, readinessErr},
		{"core.profiles", profilesRes, profilesErr},
		{"core.conflicts", conflicts, conflictsErr},
		{"core.traces", traces, tracesErr},
		{"core.pluginMeta", meta, metaErr},
	} {
		if tc.err != nil {
			t.Errorf("%s: unexpected rpc error %d %s", tc.name, tc.err.Code, tc.err.Message)
			continue
		}
		raw, merr := json.Marshal(tc.res)
		if merr != nil {
			t.Fatalf("%s: marshal: %v", tc.name, merr)
		}
		if !strings.HasPrefix(string(raw), "[") {
			t.Errorf("%s returned %s; an empty collection must be []", tc.name, raw)
		}
	}
}

// TestMatrixEnumeratesTheRuleTable: the matrix is the rule table the router
// compiles, in a fixed order, with the user toggle reflected — and the
// keyboard chords and action names resolve.
func TestMatrixEnumeratesTheRuleTable(t *testing.T) {
	c := testCore(t)
	res, rerr := c.handleGetMatrix(nil)
	rows := decodeRows[wireMatrixRow](t, res, rerr)
	if len(rows) != len(builtin.All()) {
		t.Fatalf("matrix has %d rows, rule table has %d", len(rows), len(builtin.All()))
	}
	// Plugin, then action, then rule ID — asserted as an invariant so a new
	// rule cannot silently land in a different place.
	for i := 1; i < len(rows); i++ {
		a, b := rows[i-1], rows[i]
		if a.Plugin > b.Plugin ||
			(a.Plugin == b.Plugin && a.Action > b.Action) ||
			(a.Plugin == b.Plugin && a.Action == b.Action && a.RuleID > b.RuleID) {
			t.Fatalf("rows %d/%d are out of order: %+v then %+v", i-1, i, a, b)
		}
	}
	byID := map[string]wireMatrixRow{}
	for _, r := range rows {
		if r.Keys == "" {
			t.Errorf("%s: empty keys column", r.RuleID)
		}
		byID[r.RuleID] = r
	}
	// A window.move intent names its zone through winlayout instead of
	// repeating "window.move" on every row.
	if got := byID["windows-keyboard.win-left-snap"]; got.Action != "Left Half" || got.Keys != "Win+Left" {
		t.Errorf("win-left-snap: action=%q keys=%q, want \"Left Half\"/\"Win+Left\"", got.Action, got.Keys)
	}
	// A dedicated window capability resolves through the same vocabulary.
	if got := byID["windows-keyboard.win-up-maximize"]; got.Action != "Maximize" {
		t.Errorf("win-up-maximize: action=%q, want Maximize", got.Action)
	}
	// An intent winlayout does not own keeps its ID — that is its real name.
	if got := byID["windows-keyboard.alt-f4-close-window"]; got.Action != "window.close" || got.Keys != "Alt+F4" {
		t.Errorf("alt-f4: action=%q keys=%q, want window.close/Alt+F4", got.Action, got.Keys)
	}
	if got := byID["developer.ctrl-shift-c-copypath"]; got.Keys != "Ctrl+Shift+C" {
		t.Errorf("ctrl-shift-c: keys=%q, want Ctrl+Shift+C", got.Keys)
	}
	// Contexts are the app modes, never null.
	if got := byID["windows-keyboard.win-left-snap"]; len(got.Contexts) != 2 ||
		got.Contexts[0] != "native" || got.Contexts[1] != "terminal" {
		t.Errorf("win-left-snap contexts=%v, want [native terminal]", got.Contexts)
	}
	if got := byID["windows-keyboard.ctrl-c-copy"]; len(got.Contexts) != 1 || got.Contexts[0] != "native" {
		t.Errorf("ctrl-c-copy contexts=%v, want [native]", got.Contexts)
	}
	// The row is the complement of the disabled set config.setRuleEnabled
	// writes, so a toggle the user just made is visible immediately.
	if _, serr := c.handleSetRuleEnabled(
		json.RawMessage(`{"ruleId":"windows-keyboard.win-left-snap","enabled":false}`)); serr != nil {
		t.Fatalf("setRuleEnabled: %v", serr)
	}
	afterRes, afterErr := c.handleGetMatrix(nil)
	for _, r := range decodeRows[wireMatrixRow](t, afterRes, afterErr) {
		want := r.RuleID != "windows-keyboard.win-left-snap"
		if r.Enabled != want {
			t.Errorf("%s enabled=%v after disabling it", r.RuleID, r.Enabled)
		}
	}
}

// TestOverrideRoundTripAndFailClosed: an override is stored per app, echoed
// back, and an unknown rule or an empty app is refused WITHOUT being stored.
func TestOverrideRoundTripAndFailClosed(t *testing.T) {
	c := testCore(t)
	res, rerr := c.handleGetOverrides(nil)
	if rows := decodeRows[overrideRow](t, res, rerr); len(rows) != 0 {
		t.Fatalf("a fresh daemon has %d overrides, want 0", len(rows))
	}
	if _, oerr := c.handleSetOverride(
		json.RawMessage(`{"app":"com.apple.Finder","rule_id":"nope","enabled":true}`)); oerr == nil {
		t.Fatal("an unknown rule id must fail closed")
	}
	if _, oerr := c.handleSetOverride(
		json.RawMessage(`{"rule_id":"windows-keyboard.ctrl-c-copy","enabled":true}`)); oerr == nil {
		t.Fatal("an override with no app must be rejected")
	}
	res, rerr = c.handleGetOverrides(nil)
	if rows := decodeRows[overrideRow](t, res, rerr); len(rows) != 0 {
		t.Fatalf("rejected overrides were stored: %+v", rows)
	}

	// Inserted out of order on purpose: the list comes back sorted by app so
	// the editor does not reshuffle between polls.
	for _, app := range []string{"com.microsoft.VSCode", "com.apple.Finder"} {
		if _, oerr := c.handleSetOverride(json.RawMessage(
			`{"app":"` + app + `","rule_id":"windows-keyboard.ctrl-c-copy","enabled":false}`)); oerr != nil {
			t.Fatalf("setOverride %s: %v", app, oerr)
		}
	}
	res, rerr = c.handleGetOverrides(nil)
	rows := decodeRows[overrideRow](t, res, rerr)
	if len(rows) != 2 {
		t.Fatalf("overrides=%+v, want 2 rows", rows)
	}
	if rows[0].App != "com.apple.Finder" || rows[1].App != "com.microsoft.VSCode" {
		t.Fatalf("overrides are not sorted by app: %+v", rows)
	}
	// action/keys come from the rule table, so the editor renders a row from
	// one response instead of joining two.
	if rows[0].Action != "clipboard.copy" || rows[0].Keys != "Ctrl+C" || rows[0].Enabled {
		t.Errorf("override row=%+v, want clipboard.copy/Ctrl+C/enabled=false", rows[0])
	}
	// The reply echoes the stored verdict so the editor never assumes its own
	// write landed.
	echoRes, echoErr := c.handleSetOverride(json.RawMessage(
		`{"app":"com.apple.Finder","rule_id":"windows-keyboard.ctrl-c-copy","enabled":true}`))
	echo := decode[map[string]any](t, echoRes, echoErr)
	if echo["app"] != "com.apple.Finder" || echo["rule_id"] != "windows-keyboard.ctrl-c-copy" || echo["enabled"] != true {
		t.Fatalf("setOverride reply=%v", echo)
	}
}

// TestStoredOverrideGatesTheRule: a per-app override is not config.json
// furniture. config.setOverride writes a verdict the Keyboard page reports as
// live ("Edits take effect immediately"), so the decision path has to READ it —
// otherwise the page confirms the change, the file gains the row, and the rule
// fires in that app anyway. An override of true also shadows a global "off",
// which is why the store keeps both verdicts instead of only the disabled ones.
func TestStoredOverrideGatesTheRule(t *testing.T) {
	c, err := NewCoreWithSettings(builtin.All(), builtin.Grants(), "")
	if err != nil {
		t.Fatalf("NewCore: %v", err)
	}
	for _, id := range builtin.BuiltinIDs {
		c.registerBuiltin(id, true)
	}
	const rule = "windows-keyboard.win-left-snap"
	chord := event.Event{Type: event.EventKeyDown, KeyCode: 0x25, Modifiers: 1 << 3}
	finder := event.FastContext{AppID: "com.apple.Finder", AppMode: event.AppModeNative}
	safari := event.FastContext{AppID: "com.apple.Safari", AppMode: event.AppModeNative}

	if out := c.decideLocked(chord, finder); out.WinnerRule != rule {
		t.Fatalf("baseline winner=%q, want %q", out.WinnerRule, rule)
	}
	if _, oerr := c.handleSetOverride(
		json.RawMessage(`{"app":"com.apple.Finder","rule_id":"` + rule + `","enabled":false}`)); oerr != nil {
		t.Fatalf("setOverride: %v", oerr)
	}
	if out := c.decideLocked(chord, finder); out.WinnerRule != "" {
		t.Fatalf("the rule fired in Finder after being overridden off there: winner=%q", out.WinnerRule)
	}
	if out := c.decideLocked(chord, safari); out.WinnerRule != rule {
		t.Fatalf("the override leaked to another app: winner=%q", out.WinnerRule)
	}
	// Shadowing the other way: the global toggle is off and the app's own
	// verdict is on, so the app's verdict decides.
	if _, rerr := c.handleSetRuleEnabled(
		json.RawMessage(`{"ruleId":"` + rule + `","enabled":false}`)); rerr != nil {
		t.Fatalf("setRuleEnabled: %v", rerr)
	}
	if out := c.decideLocked(chord, safari); out.WinnerRule != "" {
		t.Fatalf("a globally disabled rule fired: winner=%q", out.WinnerRule)
	}
	if _, oerr := c.handleSetOverride(
		json.RawMessage(`{"app":"com.apple.Safari","rule_id":"` + rule + `","enabled":true}`)); oerr != nil {
		t.Fatalf("setOverride: %v", oerr)
	}
	if out := c.decideLocked(chord, safari); out.WinnerRule != rule {
		t.Fatalf(`a per-app "on" must shadow the global "off": winner=%q`, out.WinnerRule)
	}
	// Plugin enablement is NOT shadowable: an override is a verdict about a
	// rule, not a permission to run it.
	if _, perr := c.handlePluginSetEnabled(
		json.RawMessage(`{"id":"windows-keyboard","enabled":false}`)); perr != nil {
		t.Fatalf("plugin.setEnabled: %v", perr)
	}
	if out := c.decideLocked(chord, safari); out.WinnerRule != "" {
		t.Fatalf("a disabled plugin's rule fired: winner=%q", out.WinnerRule)
	}
}

// TestZonesRoundTripAndRejectsBadEdit: zones persist as sent, a duplicate id
// is refused without disturbing the running list, and a malformed payload is
// a typed error rather than a half-applied edit.
func TestZonesRoundTripAndRejectsBadEdit(t *testing.T) {
	c := testCore(t)
	res, rerr := c.handleGetZones(nil)
	if rows := decodeRows[wireZoneRow](t, res, rerr); len(rows) != 0 {
		t.Fatalf("zones start at %d rows, want 0 (no display geometry to seed from)", len(rows))
	}
	res, rerr = c.handleSetZones(json.RawMessage(
		`{"zones":[{"id":"left","name":"Left third","x":0,"y":0,"w":480,"h":1080}]}`))
	if rerr != nil {
		t.Fatalf("setZones: %v", rerr)
	}
	if got := decode[map[string]any](t, res, rerr)["count"]; got != float64(1) {
		t.Fatalf("setZones count=%v, want 1", got)
	}
	res, rerr = c.handleGetZones(nil)
	rows := decodeRows[wireZoneRow](t, res, rerr)
	if len(rows) != 1 || rows[0].ID != "left" || rows[0].W != 480 {
		t.Fatalf("zones=%+v, want the row just written", rows)
	}
	// One bad row rejects the whole edit; the editor keeps showing what works.
	if _, zerr := c.handleSetZones(json.RawMessage(
		`{"zones":[{"id":"left","name":"L","x":0,"y":0,"w":10,"h":10},{"id":"left","name":"L2","x":0,"y":0,"w":10,"h":10}]}`)); zerr == nil {
		t.Fatal("a duplicate zone id must be rejected")
	}
	res, rerr = c.handleGetZones(nil)
	if got := decodeRows[wireZoneRow](t, res, rerr); len(got) != 1 || got[0].Name != "Left third" {
		t.Fatalf("a rejected edit changed the running set: %+v", got)
	}
	if _, zerr := c.handleSetZones(json.RawMessage(`{"zones":"nope"}`)); zerr == nil {
		t.Fatal("a malformed payload must be a typed error")
	}
	if _, zerr := c.handleSetZones(json.RawMessage(
		`{"zones":[{"id":"flat","name":"F","x":0,"y":0,"w":0,"h":10}]}`)); zerr == nil {
		t.Fatal("a zone with no area must be rejected")
	}
	res, rerr = c.handleSetZones(json.RawMessage(`{"zones":[]}`))
	if rerr != nil {
		t.Fatalf("clearing the list: %v", rerr)
	}
	if got := decode[map[string]any](t, res, rerr)["count"]; got != float64(0) {
		t.Fatalf("clear count=%v, want 0", got)
	}
	// A page that clears the editor must not get the seed list back.
	res, rerr = c.handleGetZones(nil)
	if got := decodeRows[wireZoneRow](t, res, rerr); len(got) != 0 {
		t.Fatalf("zones=%+v after clearing, want none", got)
	}
	if z := c.set.Zones(); z == nil {
		t.Fatal("Zones() returned nil; the wire would encode null")
	}
}

// TestTrialStateUsesTheSafetyTimeout: the countdown is timed from
// safety.TRIALTimeout, never a local literal, and the idle state still
// reports the window the page will offer.
func TestTrialStateUsesTheSafetyTimeout(t *testing.T) {
	c := testCore(t)
	c.registerBuiltin("plug", false)
	res, rerr := c.handleTrialState(nil)
	idle := decode[wireTrialState](t, res, rerr)
	if idle.State != "none" || idle.Plugin != "" || idle.RemainingMS != 0 {
		t.Fatalf("idle trial state=%+v, want none/empty/0", idle)
	}
	if want := safety.TRIALTimeout.Milliseconds(); idle.TimeoutMS != want {
		t.Fatalf("timeout_ms=%d, want safety.TRIALTimeout (%d ms)", idle.TimeoutMS, want)
	}
	if _, berr := c.handleBeginTrial(json.RawMessage(`{"pluginId":"plug"}`)); berr != nil {
		t.Fatalf("beginTrial: %v", berr)
	}
	res, rerr = c.handleTrialState(nil)
	live := decode[wireTrialState](t, res, rerr)
	if live.Plugin != "plug" || live.State != "trial" {
		t.Fatalf("live trial state=%+v, want plug/trial", live)
	}
	if live.RemainingMS <= 0 || live.RemainingMS > safety.TRIALTimeout.Milliseconds() {
		t.Fatalf("remaining_ms=%d, want 0 < remaining <= %d", live.RemainingMS, safety.TRIALTimeout.Milliseconds())
	}
	if live.TimeoutMS != idle.TimeoutMS {
		t.Fatalf("timeout changed mid-trial: %d then %d", idle.TimeoutMS, live.TimeoutMS)
	}
	// The oldest in-flight trial is the one the single countdown shows.
	c.registerBuiltin("plug2", false)
	if _, berr := c.handleBeginTrial(json.RawMessage(`{"pluginId":"plug2"}`)); berr != nil {
		t.Fatalf("beginTrial plug2: %v", berr)
	}
	res, rerr = c.handleTrialState(nil)
	if both := decode[wireTrialState](t, res, rerr); both.Plugin != "plug" {
		t.Fatalf("with two trials in flight the countdown shows %q, want the oldest (plug)", both.Plugin)
	}
}

// TestOwnershipAuditReportsOnlyRealFiles: a path the daemon manages is not an
// owned resource until it exists; once the file is written the audit names it
// with a creation time the shell can parse.
func TestOwnershipAuditReportsOnlyRealFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c, err := NewCoreWithSettings(builtin.All(), builtin.Grants(), path)
	if err != nil {
		t.Fatalf("NewCoreWithSettings: %v", err)
	}
	res, rerr := c.handleOwnershipAudit(nil)
	for _, r := range decodeRows[auditRow](t, res, rerr) {
		if r.Resource == string(safety.IntegrationConfig) {
			t.Fatalf("audit lists the config file before anything wrote it: %+v", r)
		}
		if r.ID == "" || r.Owner == "" {
			t.Errorf("audit row is missing its identity: %+v", r)
		}
		if _, perr := time.Parse(time.RFC3339, r.CreatedAt); perr != nil {
			t.Errorf("created_at %q is not RFC3339: %v", r.CreatedAt, perr)
		}
	}
	// A user edit writes the file, and the audit picks it up on the next read.
	if _, serr := c.handleSetRuleEnabled(
		json.RawMessage(`{"ruleId":"windows-keyboard.ctrl-c-copy","enabled":false}`)); serr != nil {
		t.Fatalf("setRuleEnabled: %v", serr)
	}
	res, rerr = c.handleOwnershipAudit(nil)
	rows := decodeRows[auditRow](t, res, rerr)
	found := false
	for i, r := range rows {
		if r.Resource == string(safety.IntegrationConfig) {
			found = true
			if r.ID != path || r.Owner != "core" {
				t.Errorf("config row=%+v, want id %q owned by core", r, path)
			}
		}
		if i > 0 {
			prev := rows[i-1]
			if prev.Resource > r.Resource || (prev.Resource == r.Resource && prev.ID > r.ID) {
				t.Errorf("audit is not sorted: %+v then %+v", prev, r)
			}
		}
	}
	if !found {
		t.Fatalf("the settings file was written but the audit does not list it: %+v", rows)
	}
}

// TestReadinessReportsLiveState: every row carries the reason it is not
// ready, and the kill switch reaches every row at once.
func TestReadinessReportsLiveState(t *testing.T) {
	c := testCore(t)
	c.registerBuiltin("windows-keyboard", true)
	c.registerBuiltin("launcher", false)
	res, rerr := c.handleReadiness(nil)
	byID := map[string]readinessRow{}
	for _, r := range decodeRows[readinessRow](t, res, rerr) {
		byID[r.ID] = r
	}
	if got := byID["daemon"]; !got.Ready {
		t.Errorf("daemon row=%+v, want ready on a running daemon", got)
	}
	if got := byID["keyboard"]; got.Ready || got.Detail == "" {
		t.Errorf("keyboard row=%+v, want not ready with the reason stated", got)
	}
	if got := byID["windows-keyboard"]; !got.Ready {
		t.Errorf("enabled plugin row=%+v, want ready", got)
	}
	if got := byID["launcher"]; got.Ready || !strings.Contains(got.Detail, "disabled") {
		t.Errorf("disabled plugin row=%+v, want not ready and named as disabled", got)
	}

	if _, perr := c.handlePanicStop(nil); perr != nil {
		t.Fatalf("panicStop: %v", perr)
	}
	res, rerr = c.handleReadiness(nil)
	for _, r := range decodeRows[readinessRow](t, res, rerr) {
		if r.Ready {
			t.Errorf("%s reports ready after PANIC STOP", r.ID)
		}
		if r.ID != "keyboard" && !strings.Contains(r.Detail, "PANIC STOP") {
			t.Errorf("%s does not name the kill switch: %q", r.ID, r.Detail)
		}
	}
}

// TestCommandAndSchemaSourcesAreHonestAboutBeingEmpty: the two sources with
// no runtime source behind them answer [], not an invented row. This pins the
// emptiness so a placeholder command cannot be committed by accident.
func TestCommandAndSchemaSourcesAreHonestAboutBeingEmpty(t *testing.T) {
	c := testCore(t)
	res, rerr := c.handleCoreCommands(nil)
	if rows := decodeRows[commandRow](t, res, rerr); len(rows) != 0 {
		t.Fatalf("core.commands invented %d commands: %+v", len(rows), rows)
	}
	res, rerr = c.handlePluginSchemas(nil)
	if rows := decodeRows[schemaRow](t, res, rerr); len(rows) != 0 {
		t.Fatalf("core.pluginSchemas invented %d schemas: %+v", len(rows), rows)
	}
}

// TestChordRenderingIsTotal: an unknown keycode still renders, so a new
// matrix row can never show an empty chord.
func TestChordRenderingIsTotal(t *testing.T) {
	cases := map[uint32]string{
		0x43: "C", 0x0D: "Return", 0x73: "F4", 0x87: "F24",
		0x60: "Num0", 0x69: "Num9", 0x2E: "Delete", 0xFFFF: "VK 0xFFFF",
	}
	for vk, want := range cases {
		if got := keyName(vk); got != want {
			t.Errorf("keyName(0x%X)=%q, want %q", vk, got, want)
		}
	}
}

// TestMatrixRowsNameTheirAppConditions: a rule scoped to specific bundle IDs
// says so. The Context column used to render a developer rule's six app IDs as
// a plain "native" — which is exactly what a rule with no app restriction looks
// like — so the editor could not answer "where does this fire", and the rule ID
// is a toggle handle rather than a scope report.
func TestMatrixRowsNameTheirAppConditions(t *testing.T) {
	c := testCore(t)
	res, rerr := c.handleGetMatrix(nil)
	rows := decodeRows[wireMatrixRow](t, res, rerr)
	byID := map[string]wireMatrixRow{}
	for _, r := range rows {
		byID[r.RuleID] = r
	}
	// Every condition a rule declares appears in its row: the app modes, then
	// the app IDs in the order the rule table lists them. Derived from the
	// table, so a new rule is covered without a line here.
	for _, r := range builtin.All() {
		want := make([]string, 0, len(r.AppModes)+len(r.AppIDs))
		for _, m := range r.AppModes {
			want = append(want, string(m))
		}
		want = append(want, r.AppIDs...)
		if got := byID[r.RuleID].Contexts; strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%s contexts=%v, want %v", r.RuleID, got, want)
		}
	}
	// The spec's own example row, readable as itself: Ctrl+Shift+C names Finder
	// among the apps it is scoped to.
	if got := byID["developer.ctrl-shift-c-copypath"].Contexts; len(got) == 0 ||
		!strings.Contains(strings.Join(got, "|"), "com.apple.Finder") {
		t.Errorf("ctrl-shift-c contexts=%v, want com.apple.Finder among them", got)
	}
}

// TestProfilesServeTheBundleAndItsRollup: the card is the profile's own data
// plus a rollup of what is live, and an unavailable capability keeps its reason
// instead of being dropped or ticked.
func TestProfilesServeTheBundleAndItsRollup(t *testing.T) {
	c := testCore(t)
	for _, id := range builtin.BuiltinIDs {
		c.registerBuiltin(id, true)
	}
	res, rerr := c.handleCoreProfiles(nil)
	rows := decodeRows[wireProfileRow](t, res, rerr)
	if len(rows) != len(profiles.All()) {
		t.Fatalf("core.profiles has %d cards, the bundle declares %d", len(rows), len(profiles.All()))
	}
	declared := profiles.All()[0]
	card := rows[0]
	if card.ID != declared.ID || card.Label != declared.Label || card.Description != declared.Description {
		t.Fatalf("card=%+v, want the bundle's %q verbatim", card, declared.ID)
	}
	if card.Active {
		t.Error("a daemon that has applied nothing reports a profile active")
	}
	byID := map[string]wireCapabilityRow{}
	for _, row := range card.Capabilities {
		byID[row.ID] = row
	}
	if len(card.Capabilities) != len(declared.Capabilities) {
		t.Fatalf("card has %d capabilities, the bundle declares %d",
			len(card.Capabilities), len(declared.Capabilities))
	}
	for _, want := range declared.Capabilities {
		row, ok := byID[want.ID]
		if !ok {
			t.Fatalf("capability %q is missing from the card", want.ID)
		}
		if row.Label != want.Label || row.Plugin != want.Plugin || row.Available != want.Available {
			t.Errorf("%s row=%+v, want label=%q plugin=%q available=%v",
				want.ID, row, want.Label, want.Plugin, want.Available)
		}
		if len(row.RuleIDs) != len(want.RuleIDs) || row.RuleIDs == nil {
			t.Errorf("%s rule_ids=%v, want the declared %v as a list", want.ID, row.RuleIDs, want.RuleIDs)
		}
		// The rollup counts the matrix rules a capability can toggle and no
		// more: a window zone is not a rule, and counting it would report
		// "0 of 21" for a capability that ships no shortcuts at all.
		toggleable := 0
		for _, id := range want.RuleIDs {
			if _, isRule := builtinRule(id); isRule {
				toggleable++
			}
		}
		if row.Total != toggleable {
			t.Errorf("%s total=%d, want %d togglable rules", want.ID, row.Total, toggleable)
		}
		if row.Enabled != toggleable {
			t.Errorf("%s enabled=%d, want %d on a daemon that disabled nothing", want.ID, row.Enabled, toggleable)
		}
		// A capability is live when its plugin is on and its rules are, and an
		// unavailable one is never live — with the reason it is missing.
		if want.Available && !row.Live {
			t.Errorf("%s is not live although it is available and its plugin is on", want.ID)
		}
		if !want.Available {
			if row.Live {
				t.Errorf("unavailable capability %s reports live", want.ID)
			}
			if row.Reason == "" {
				t.Errorf("unavailable capability %s dropped the reason it is not delivered", want.ID)
			}
		}
	}
}

// TestProfileApplyIsValidatedAndWhole: an unknown profile is refused before
// anything moves, and applying the declared one turns on exactly the rules and
// plugins its capabilities name — in the store AND in the daemon's own plugin
// map, which is the map decideLocked compiles the running router from.
func TestProfileApplyIsValidatedAndWhole(t *testing.T) {
	c, err := NewCoreWithSettings(builtin.All(), builtin.Grants(),
		filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("NewCoreWithSettings: %v", err)
	}
	for _, id := range builtin.BuiltinIDs {
		c.registerBuiltin(id, true)
	}
	if _, perr := c.handleProfileApply(json.RawMessage(`{"profile":"nope"}`)); perr == nil ||
		perr.Code != ipc.ErrInvalid {
		t.Fatalf("an unknown profile must be refused as invalid, got %v", perr)
	}
	if got := c.set.ActiveProfile(); got != "" {
		t.Fatalf("a refused apply recorded profile %q", got)
	}
	for _, bad := range []string{`{`, `{"profile":""}`, `{"profile":"   "}`} {
		if _, perr := c.handleProfileApply(json.RawMessage(bad)); perr == nil ||
			perr.Code != ipc.ErrBadParams {
			t.Errorf("payload %s must be a bad-params error, got %v", bad, perr)
		}
	}

	// The user switched the Windows plugin off and one of its shortcuts off
	// before pressing the card.
	if _, perr := c.handlePluginSetEnabled(
		json.RawMessage(`{"id":"windows-keyboard","enabled":false}`)); perr != nil {
		t.Fatalf("plugin.setEnabled: %v", perr)
	}
	const off = "windows-keyboard.win-left-snap"
	if _, rerr := c.handleSetRuleEnabled(
		json.RawMessage(`{"ruleId":"` + off + `","enabled":false}`)); rerr != nil {
		t.Fatalf("setRuleEnabled: %v", rerr)
	}

	res, rerr := c.handleProfileApply(json.RawMessage(`{"profile":"windows-11-experience"}`))
	reply := decode[map[string]any](t, res, rerr)
	if reply["profile"] != "windows-11-experience" {
		t.Fatalf("apply reply=%v, want the applied profile echoed", reply)
	}
	// The reply counts the bundle's distinct ids, so three capabilities sharing
	// one plugin read as one edit.
	wantRules := map[string]bool{}
	for _, p := range profiles.All() {
		for _, cap := range p.Capabilities {
			if !cap.Available {
				continue
			}
			for _, id := range cap.RuleIDs {
				if _, isRule := builtinRule(id); isRule {
					wantRules[id] = true
				}
			}
		}
	}
	if got := int(reply["rules"].(float64)); got != len(wantRules) {
		t.Errorf("apply turned on %d rules, the bundle names %d", got, len(wantRules))
	}
	if got := int(reply["plugins"].(float64)); got != 1 {
		t.Errorf("apply toggled %d plugins, want 1 (windows-keyboard)", got)
	}
	if !c.set.IsRuleEnabled(off) {
		t.Errorf("%s is still off after applying the profile that names it", off)
	}
	if got := c.set.ActiveProfile(); got != "windows-11-experience" {
		t.Errorf("stored active profile=%q after apply", got)
	}
	// The running plugin map moved too — a profile that saved to disk and left
	// the daemon alone is the half-activated outcome the batch exists to stop.
	list, lerr := c.handlePluginList(nil)
	for _, row := range decodeRows[map[string]any](t, list, lerr) {
		if row["id"] == "windows-keyboard" && row["enabled"] != true {
			t.Errorf("windows-keyboard is %v in the running set after apply", row["enabled"])
		}
	}
	// And the card reports itself active with its capabilities live.
	cardRes, cardErr := c.handleCoreProfiles(nil)
	card := decodeRows[wireProfileRow](t, cardRes, cardErr)[0]
	if !card.Active {
		t.Error("the card does not report the profile it just applied as active")
	}
	for _, row := range card.Capabilities {
		if row.Available && !row.Live {
			t.Errorf("capability %s is still not live after applying its profile", row.ID)
		}
	}
}

// TestOnboardingStateResolvesEveryChecklistRow: the wizard derives its steps
// from live state, and every id the first-run checklist declares resolves to a
// real readiness row — an id the daemon never reports renders as "Not checked"
// with no reason, which on the first-run page is the difference between fixing
// a permission and believing you fixed one.
func TestOnboardingStateResolvesEveryChecklistRow(t *testing.T) {
	c := testCore(t)
	res, rerr := c.handleOnboardingState(nil)
	state := decode[wireOnboardingRow](t, res, rerr)
	if state.Completed {
		t.Error("a fresh daemon reports the first run as finished")
	}
	if state.CurrentStep != "welcome" {
		t.Errorf("current step=%q on a fresh daemon, want welcome", state.CurrentStep)
	}
	if len(state.Steps) != len(onboardingSteps) {
		t.Fatalf("wizard has %d steps, the flow declares %d", len(state.Steps), len(onboardingSteps))
	}
	byID := map[string]readinessRow{}
	for _, row := range state.Readiness {
		byID[row.ID] = row
	}
	for _, id := range []string{"keyboard", "windows", "finder"} {
		if _, ok := byID[id]; !ok {
			t.Errorf("checklist item %q has no readiness row to resolve to", id)
		}
	}
	// The rollup counts the rows it reports, so the header and the list cannot
	// describe different states.
	ready := 0
	for _, row := range state.Readiness {
		if row.Ready {
			ready++
		}
	}
	if state.Ready != ready || state.Total != len(state.Readiness) {
		t.Errorf("rollup=%d/%d over %d rows", state.Ready, state.Total, len(state.Readiness))
	}
	steps := map[string]wireOnboardingStep{}
	for _, s := range state.Steps {
		if s.Done {
			t.Errorf("step %q is done on a daemon that has done nothing", s.ID)
		}
		steps[s.ID] = s
	}
	// A step that is waiting says what to do, not just that it is waiting.
	for _, id := range []string{"enable", "settings", "verify"} {
		if steps[id].Detail == "" {
			t.Errorf("step %q is not done and does not say why", id)
		}
	}

	// A plugin switched on moves the wizard past the enable step, and the
	// System Settings step now names the permission it is waiting on.
	for _, id := range builtin.BuiltinIDs {
		c.registerBuiltin(id, true)
	}
	res, rerr = c.handleOnboardingState(nil)
	state = decode[wireOnboardingRow](t, res, rerr)
	steps = map[string]wireOnboardingStep{}
	for _, s := range state.Steps {
		steps[s.ID] = s
	}
	if state.CurrentStep != "settings" {
		t.Errorf("current step=%q with every plugin on and no tap, want settings", state.CurrentStep)
	}
	if !steps["enable"].Done {
		t.Error("the enable step is not done with plugins switched on")
	}
	if steps["settings"].Done || steps["settings"].Detail == "" {
		t.Errorf("settings step=%+v, want waiting with the reason stated", steps["settings"])
	}

	// The persisted flag ends the wizard: the user saying "I am finished" is
	// the one thing here the daemon cannot re-derive, so it wins.
	if cerr := c.set.SetOnboardingComplete(true); cerr != nil {
		t.Fatalf("SetOnboardingComplete: %v", cerr)
	}
	res, rerr = c.handleOnboardingState(nil)
	state = decode[wireOnboardingRow](t, res, rerr)
	if !state.Completed || state.CurrentStep != "done" {
		t.Errorf("completed run reports %v at step %q, want done", state.Completed, state.CurrentStep)
	}
	for _, s := range state.Steps {
		if !s.Done {
			t.Errorf("step %q is not done on a completed first run", s.ID)
		}
	}
}

// TestTracesExposeTheRecordedDetail: core.traces serves the columns
// observe.SanitizeForDisplay flattens away — the stages, the physical event,
// the focused app and the rules that lost — from the same recorded trace
// core.eventLogs serves as a sentence.
func TestTracesExposeTheRecordedDetail(t *testing.T) {
	c := testCore(t)
	for _, id := range builtin.BuiltinIDs {
		c.registerBuiltin(id, true)
	}
	res, rerr := c.handleTraces(nil)
	if rows := decodeRows[wireTraceRow](t, res, rerr); len(rows) != 0 {
		t.Fatalf("a daemon that has decided nothing reports %d traces: %+v", len(rows), rows)
	}
	if _, kerr := c.handleKeyEvent(json.RawMessage(
		`{"keyCode":67,"modifiers":1,"appId":"com.apple.Finder","appMode":"native"}`)); kerr != nil {
		t.Fatalf("keyEvent: %v", kerr)
	}
	res, rerr = c.handleTraces(nil)
	rows := decodeRows[wireTraceRow](t, res, rerr)
	if len(rows) != 1 {
		t.Fatalf("traces=%+v, want the one decision just made", rows)
	}
	row := rows[0]
	const winner = "windows-keyboard.ctrl-c-copy"
	if row.Winner != winner || row.Intent != "clipboard.copy" {
		t.Fatalf("trace=%+v, want winner %q / clipboard.copy", row, winner)
	}
	// The physical event renders the way the matrix renders a rule's binding, so
	// a recorded chord and the rule that claims it read alike.
	if row.Event.Keys != "Ctrl+C" || row.Event.Source != "keyboard" || row.Event.KeyCode != 0x43 {
		t.Errorf("event=%+v, want Ctrl+C from the keyboard", row.Event)
	}
	if row.Context.AppID != "com.apple.Finder" || row.Context.AppMode != "native" {
		t.Errorf("context=%+v, want the focused app the decision was made in", row.Context)
	}
	if row.Decision != "REPLACE" {
		t.Errorf("decision=%q, want the PASS/CONSUME/REPLACE vocabulary core.keyEvent serves", row.Decision)
	}
	// Losers is a list even when the decision was uncontested: null would be a
	// missing value the page has to special-case.
	if row.Losers == nil {
		t.Error("losers encoded as null")
	}
	if _, perr := time.Parse(time.RFC3339, row.At); perr != nil {
		t.Errorf("at=%q is not RFC3339: %v", row.At, perr)
	}
	stages := map[string]string{}
	for _, s := range row.Stages {
		stages[s.Stage] = s.Detail
	}
	for _, want := range []string{"event", "context", "rule", "intent", "result"} {
		if stages[want] == "" {
			t.Errorf("trace has no %q stage: %+v", want, row.Stages)
		}
	}
	// Both views describe the same recorded trace, so the sentence and the
	// columns cannot tell the user different stories.
	logs, lerr := c.handleEventLogs(nil)
	if lerr != nil {
		t.Fatalf("eventLogs: %v", lerr)
	}
	lines, ok := logs.([]string)
	if !ok || len(lines) != 1 || !strings.Contains(lines[0], winner) {
		t.Errorf("core.eventLogs=%v, want the flattened %q", logs, winner)
	}
}

// TestPluginMetaIsHonestWithoutAManifest: no manifest is loaded at runtime, so
// there is no display name or version to report — the row says so instead of
// the daemon prettifying a plugin id and stamping a version. The permissions are
// not a guess: they are the grants the router authorizes.
func TestPluginMetaIsHonestWithoutAManifest(t *testing.T) {
	c := testCore(t)
	for _, id := range builtin.BuiltinIDs {
		c.registerBuiltin(id, true)
	}
	res, rerr := c.handlePluginMeta(nil)
	rows := decodeRows[wirePluginMetaRow](t, res, rerr)
	if len(rows) != len(builtin.BuiltinIDs) {
		t.Fatalf("plugin meta has %d rows, %d plugins are registered", len(rows), len(builtin.BuiltinIDs))
	}
	grants := builtin.Grants()
	for i, row := range rows {
		if row.ID != builtin.BuiltinIDs[i] {
			t.Errorf("row %d is %q, want registration order (%q)", i, row.ID, builtin.BuiltinIDs[i])
		}
		if row.Name != "" || row.Version != "" || row.Loaded {
			t.Errorf("%s reports name=%q version=%q loaded=%v with no manifest behind it",
				row.ID, row.Name, row.Version, row.Loaded)
		}
		if row.Reason == "" {
			t.Errorf("%s says nothing about the missing manifest", row.ID)
		}
		want := make([]string, 0, len(grants[row.ID]))
		for _, p := range grants[row.ID] {
			want = append(want, string(p))
		}
		if strings.Join(row.Permissions, ",") != strings.Join(want, ",") {
			t.Errorf("%s permissions=%v, want the grants the router authorizes: %v", row.ID, row.Permissions, want)
		}
	}
}

// TestConflictsRankThroughRuleResolve: the builtin table binds one rule per
// chord, so the honest answer today is an empty list; and a collision is ranked
// by the decision path's own call, with the winner first and the shadowed rule
// named.
func TestConflictsRankThroughRuleResolve(t *testing.T) {
	c := testCore(t)
	for _, id := range builtin.BuiltinIDs {
		c.registerBuiltin(id, true)
	}
	res, rerr := c.handleConflicts(nil)
	if rows := decodeRows[wireConflictRow](t, res, rerr); len(rows) != 0 {
		t.Fatalf("core.conflicts invented %d conflicts: %+v", len(rows), rows)
	}

	// Two rules on Ctrl+Shift+C, plus one on a chord of its own.
	mk := func(key uint32, id, plugin string, spec int, scope rule.Scope, prio int) event.CompiledRule {
		return event.CompiledRule{
			KeyCode: key, Modifiers: 1<<0 | 1<<1, RuleID: id, PluginID: plugin,
			Specificity: spec, Scope: scope, Priority: prio,
			Intent: intent.Intent{ID: "clipboard.copy", Version: 1},
		}
	}
	rows := decodeRows[wireConflictRow](t, chordConflicts([]event.CompiledRule{
		mk(0x43, "global.copy", "windows-keyboard", 1, rule.ScopeGlobal, rule.PriorityGlobal),
		mk(0x43, "app.copy", "developer", 2, rule.ScopeApp, rule.PriorityApp),
		mk(0x50, "solo.copy", "developer", 9, rule.ScopeApp, rule.PriorityApp),
	}), nil)
	if len(rows) != 1 {
		t.Fatalf("one chord has two claimants, got %d conflict rows: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.Keys != "Ctrl+Shift+C" {
		t.Errorf("keys=%q, want the contested chord the page highlights", row.Keys)
	}
	// The more specific rule wins, and the global one is named as shadowed —
	// rule.Resolve is the router's own ranking, so the dialog and the keystroke
	// cannot disagree.
	if row.Winner != "app.copy" {
		t.Errorf("winner=%q, want app.copy (the more specific rule)", row.Winner)
	}
	if len(row.Losers) != 1 || row.Losers[0] != "global.copy" {
		t.Errorf("losers=%v, want [global.copy]", row.Losers)
	}
	if len(row.Rules) != 2 || row.Rules[0].RuleID != "app.copy" || row.Rules[1].RuleID != "global.copy" {
		t.Errorf("claims=%+v, want the winner listed first", row.Rules)
	}
	if row.Rules[0].Plugin != "developer" || row.Rules[0].Action != "clipboard.copy" {
		t.Errorf("winning claim=%+v, want the plugin and action it resolves to", row.Rules[0])
	}
	// A rule alone on its chord is not in any row.
	for _, r := range row.Rules {
		if r.RuleID == "solo.copy" {
			t.Error("a rule with no competitor is reported as a conflict")
		}
	}
}
