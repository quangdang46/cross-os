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
	"crossos/core/pkg/ipc"
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

// TestEveryPageDataMethodIsRegistered pins the ten settings-page data
// sources in the method table. A handler left out of methods() is dead code
// the shell can never reach, and only a direct call would ever see it.
func TestEveryPageDataMethodIsRegistered(t *testing.T) {
	c := testCore(t)
	reg := c.methods()
	for _, name := range []string{
		"config.getMatrix", "config.getOverrides", "config.setOverride",
		"config.getZones", "config.setZones", "core.commands",
		"core.pluginSchemas", "safety.ownershipAudit", "safety.trialState",
		"core.readiness",
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
	for _, tc := range []call{
		{"config.getMatrix", matrix, matrixErr},
		{"config.getOverrides", overrides, overridesErr},
		{"config.getZones", zones, zonesErr},
		{"core.commands", commands, commandsErr},
		{"core.pluginSchemas", schemas, schemasErr},
		{"safety.ownershipAudit", audit, auditErr},
		{"core.readiness", readiness, readinessErr},
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
