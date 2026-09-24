// IPC client tests — bead cross-os-80g bridge criterion, the ten page sources
// added in cross-os-72g, and the Wave 3 set added in w3-shell-bridge.
//
// Pass criteria mapping:
//  1. Bridge calls reach Core over IPC → TestIPCClientRoundTrip (in-memory
//     net.Pipe pair speaking the §3.9 framing against a stub server) +
//     TestIPCClientFraming (byte-level request shape matches core/pkg/ipc).
//  2. Bridge/IPC failures in UI logs, never silent → TestIPCClientDialFailure
//     (unreachable transport surfaces a typed error the bridge logs).
//  3. The sources cross the wire with the contract's exact key names and RPC
//     names → TestIPCSourcesRoundTrip, TestIPCSourceRPCNames,
//     TestIPCSourceWriteParams, TestIPCSourceKeysMatchContract, and for the
//     Wave 3 rows TestIPCWave3SourcesRoundTrip, TestIPCWave3RPCNames,
//     TestIPCWave3WriteParams.
//  4. A null, empty, broken or refused source is never silently "nothing" →
//     TestIPCEmptyCollectionsAreEmptyNotNil, TestIPCNullCollectionIsEmptyNotNil,
//     TestIPCTrialStateNullIsAnError, TestIPCSourceDecodeFailureIsNotEmptiness,
//     TestIPCSourceFailuresPropagate. The first two iterate sourceReaders(), so
//     the Wave 3 lists are covered by the same two rules as the ten.
package shell

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// pipeTransport dials one end of an in-memory net.Pipe whose far end is
// served by the test's stub handler.
type pipeTransport struct {
	serve func(conn net.Conn)
}

func (p pipeTransport) Dial() (net.Conn, error) {
	a, b := net.Pipe()
	go p.serve(b)
	return a, nil
}

// recordedCall is one request the stub daemon received. Write params are
// asserted from this rather than inferred, because a snake_case slip in
// config.setOverride / config.setZones is invisible until the daemon rejects
// the call in production.
type recordedCall struct {
	Method string
	Params json.RawMessage
}

// serveJSONRPC is the §3.9 framing loop: one newline-terminated request per
// connection, one response line back, until the peer closes.
func serveJSONRPC(conn net.Conn, answer func(req *ipcRequest) string) {
	defer conn.Close()
	buf := make([]byte, 1)
	var line []byte
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			line = append(line, buf[:n]...)
			if buf[0] != '\n' {
				continue
			}
			var req ipcRequest
			var resp string
			if jerr := json.Unmarshal(line, &req); jerr != nil {
				resp = `{"jsonrpc":"2.0","error":{"code":-32700,"message":"parse error"},"id":null}` + "\n"
			} else {
				resp = answer(&req)
			}
			line = line[:0]
			if _, werr := conn.Write([]byte(resp)); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// stubServer answers the pre-existing methods with canned payloads.
func stubServer(conn net.Conn) { stubServerWith(nil)(conn) }

// stubServerWith is stubServer plus an optional request recorder.
func stubServerWith(recorder *[]recordedCall) func(net.Conn) {
	return func(conn net.Conn) {
		serveJSONRPC(conn, func(req *ipcRequest) string {
			if recorder != nil {
				*recorder = append(*recorder, recordedCall{Method: req.Method, Params: req.Params})
			}
			return routeStub(req.Method, req.ID, req.Params)
		})
	}
}

func routeStub(method string, id any, params json.RawMessage) string {
	enc := func(v any) string {
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "result": v, "id": id})
		return string(raw) + "\n"
	}
	errResp := func(code int, msg string) string {
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "error": map[string]any{"code": code, "message": msg}, "id": id})
		return string(raw) + "\n"
	}
	switch method {
	case "core.status":
		return enc(map[string]any{"running": true, "safe_mode": false, "killed": false})
	case "plugin.list":
		return enc([]PluginState{{ID: "win-kb", Enabled: true, Healthy: "healthy"}})
	case "plugin.setEnabled":
		return enc(true)
	case "core.reset":
		return enc([]string{"remove login item", "disable extension", "clean owned state"})
	case "core.eventLogs":
		return enc([]string{"e1"})

	// The ten page sources (bead cross-os-72g). These literals are written as
	// JSON TEXT, not marshalled from the shell's own structs, so the test
	// pins the daemon's spelling instead of agreeing with itself: renaming a
	// json tag in uisources.go fails here.
	case "config.getMatrix":
		return encRaw(ipcMatrixResult, id)
	case "config.getOverrides":
		return encRaw(ipcOverridesResult, id)
	case "config.getZones":
		return encRaw(ipcZonesResult, id)
	case "core.commands":
		return encRaw(ipcCommandsResult, id)
	case "core.pluginSchemas":
		return encRaw(ipcSchemasResult, id)
	case "safety.ownershipAudit":
		return encRaw(ipcAuditResult, id)
	case "core.readiness":
		return encRaw(ipcReadinessResult, id)
	case "safety.trialState":
		return encRaw(ipcTrialResult, id)
	case "config.setOverride":
		// Answer with the daemon's REAL reply, spelled as text like the nine
		// cases above: config.setOverride returns {app, rule_id, enabled} and
		// nothing else (pagedata.go handleSetOverride). Decoding the params
		// into the shell's own OverrideRow and marshalling it back would make
		// a json tag rename on that struct invisible here AND invent the
		// action/keys the handler never sends.
		var p struct {
			App     string `json:"app"`
			RuleID  string `json:"rule_id"`
			Enabled bool   `json:"enabled"`
		}
		if jerr := json.Unmarshal(params, &p); jerr != nil {
			return errResp(-32602, "setOverride params: "+jerr.Error())
		}
		return encRaw(`{"app":`+mustJSON(p.App)+`,"rule_id":`+mustJSON(p.RuleID)+`,"enabled":`+mustJSON(p.Enabled)+`}`, id)
	case "config.setZones":
		// Decoding into the shell's own ZoneRow means a wrong zone key yields
		// zero zones and a count mismatch — the contract catches itself.
		var p struct {
			Zones []ZoneRow `json:"zones"`
		}
		if jerr := json.Unmarshal(params, &p); jerr != nil {
			return errResp(-32602, "setZones params: "+jerr.Error())
		}
		return enc(map[string]any{"count": len(p.Zones)})

	// The Wave 3 sources (bead w3-shell-bridge), spelled as JSON text for the
	// same reason the ten above are: the literals pin the daemon's keys, so a
	// tag renamed on either side fails here instead of agreeing with itself.
	case "core.profiles":
		return encRaw(ipcProfilesResult, id)
	case "core.profileApply":
		// The reply repeats the id it applied with, the way handleProfileApply
		// echoes the bundle it resolved — so a card that rendered "applied" for
		// a different profile than it asked for cannot pass here.
		var p struct {
			Profile string `json:"profile"`
		}
		if jerr := json.Unmarshal(params, &p); jerr != nil {
			return errResp(-32602, "profileApply params: "+jerr.Error())
		}
		return encRaw(`{"profile":`+mustJSON(p.Profile)+`,"rules":21,"plugins":2}`, id)
	case "core.traces":
		return encRaw(ipcTracesResult, id)
	case "core.pluginMeta":
		return encRaw(ipcPluginMetaResult, id)
	case "core.apps":
		// The Go field names, because ctx.ApplicationInfo declares no tags.
		return encRaw(ipcAppsResult, id)
	case "config.getUserRules":
		return encRaw(ipcUserRulesResult, id)
	case "config.setUserRule":
		// Decoded into the write's own keys rather than the shell's row: the
		// display half is not on this call, and the echoed id is DERIVED from the
		// dimensions the way userrules.DeriveID derives it. A payload whose key,
		// modifier or scope never arrived therefore answers with a different id,
		// and the test notices.
		var p struct {
			ID         string         `json:"id"`
			Key        string         `json:"key"`
			Modifiers  []string       `json:"modifiers"`
			AppModes   []string       `json:"app_modes"`
			AppIDs     []string       `json:"app_ids"`
			DeviceID   string         `json:"device_id"`
			Capability string         `json:"capability"`
			Parameters map[string]any `json:"parameters"`
			Emit       bool           `json:"emit"`
		}
		if jerr := json.Unmarshal(params, &p); jerr != nil {
			return errResp(-32602, "setUserRule params: "+jerr.Error())
		}
		return encRaw(`{"id":`+mustJSON(deriveStubUserRuleID(p.Key, p.Modifiers, p.AppModes, p.AppIDs, p.DeviceID))+`}`, id)
	case "config.deleteUserRule":
		// Answers the table as it now stands, so an editor can update from one
		// response. Deleting the only rule leaves the empty collection.
		var p struct {
			ID string `json:"id"`
		}
		if jerr := json.Unmarshal(params, &p); jerr != nil {
			return errResp(-32602, "deleteUserRule params: "+jerr.Error())
		}
		return encRaw(ipcEmptyCollections, id)

	// The switcher (bead ws-4). The wait and the focus decode their own params
	// into the daemon's keys rather than into the shell's rows, so a key the
	// bridge spelled differently comes back as bad-params here instead of
	// agreeing with itself.
	case "core.windows":
		return encRaw(ipcWindowsResult, id)
	case "core.switcherWait":
		var p struct {
			TimeoutMs int `json:"timeout_ms"`
		}
		if jerr := json.Unmarshal(params, &p); jerr != nil {
			return errResp(-32602, "switcherWait params: "+jerr.Error())
		}
		if p.TimeoutMs <= 0 {
			return errResp(-32602, "switcherWait needs a positive timeout_ms")
		}
		return encRaw(`{"triggered":true,"action":"summon"}`, id)
	case "core.switcherFocus":
		var p struct {
			WindowID string `json:"window_id"`
		}
		if jerr := json.Unmarshal(params, &p); jerr != nil {
			return errResp(-32602, "switcherFocus params: "+jerr.Error())
		}
		if p.WindowID == "" {
			return errResp(-32602, "switcherFocus needs a window_id")
		}
		return encRaw(`{"window_id":`+mustJSON(p.WindowID)+`,"focused":true}`, id)

	default:
		return errResp(-32601, "no such method: "+method)
	}
}

// deriveStubUserRuleID reproduces userrules.DeriveID's shape — the lowercased
// chord, then the scope the rule constrains — so the setUserRule stub answers
// with the name the real store would derive. It is a test fixture, not a second
// implementation: what it pins is that the shell's payload carried the
// dimensions, because every part of the answer is built from them.
func deriveStubUserRuleID(key string, modifiers, appModes, appIDs []string, deviceID string) string {
	chord := make([]string, 0, len(modifiers)+1)
	for _, m := range modifiers {
		chord = append(chord, strings.ToLower(m))
	}
	chord = append(chord, strings.ToLower(key))
	scope := make([]string, 0, len(appModes)+len(appIDs)+1)
	if deviceID != "" {
		scope = append(scope, deviceID)
	}
	scope = append(scope, appModes...)
	scope = append(scope, appIDs...)
	if len(scope) == 0 {
		scope = []string{"any"}
	}
	return "user." + strings.Join(chord, "+") + "@" + strings.Join(scope, ",")
}

// The literal daemon results for the ten sources, exactly as the frozen
// contract spells them. Keys here are the whole point of this file's new
// half: an empty collection is [], an object keeps every key, and no field is
// renamed on the way. The Wave 3 literals below follow the same rule, and the
// two that are easy to get wrong are called out where they are: core.apps is
// PascalCase because the row it serves declares no json tags, and the user
// rule carries its picked dimensions before the derived display fields, in the
// order the daemon's row embeds them.
const (
	ipcMatrixResult     = `[{"rule_id":"windows-keyboard.ctrl-c-copy","plugin":"win-kb","action":"copy","keys":"Ctrl+C","contexts":["any","finder"],"enabled":true}]`
	ipcOverridesResult  = `[{"app":"Finder","rule_id":"mac-finder.copy","action":"copy","keys":"Cmd+C","enabled":false}]`
	ipcZonesResult      = `[{"id":"left","name":"Left half","x":0,"y":0,"w":0.5,"h":1}]`
	ipcCommandsResult   = `[{"id":"win-kb.reopen","title":"Reopen window","plugin":"win-kb"}]`
	ipcSchemasResult    = `[{"plugin":"win-wm","title":"Window Manager","schema":{"type":"object","widget":"checkbox"}}]`
	ipcAuditResult      = `[{"resource":"login-item","id":"com.crossos.helper","owner":"crossos","created_at":"2026-09-01T10:00:00Z"}]`
	ipcTrialResult      = `{"plugin":"win-wm","state":"trial","remaining_ms":12000,"timeout_ms":30000}`
	ipcReadinessResult  = `[{"id":"keyboard","label":"Keyboard","ready":true,"detail":""},{"id":"windows","label":"Window management","ready":false,"detail":"grant Accessibility"}]`
	ipcEmptyCollections = `[]`

	// A live capability beside an unavailable one: the gap keeps its reason
	// rather than being dropped, and rule_ids mixes a matrix rule with a window
	// zone name, which is what the profile bundle really declares.
	ipcProfilesResult = `[{"id":"windows-11","label":"Windows 11 Experience",` +
		`"description":"Win/Alt shortcuts, Snap Layouts and Explorer muscle memory","active":true,` +
		`"capabilities":[{"id":"window-layouts","label":"Snap Layouts","plugin":"win-wm",` +
		`"available":true,"rule_ids":["windows.window.left","left"],"enabled":1,"total":1,"live":true},` +
		`{"id":"win-keyboard","label":"Win/Alt keyboard","plugin":"win-kb",` +
		`"available":false,"reason":"Win/Alt swapping is not implemented yet",` +
		`"rule_ids":[],"enabled":0,"total":0,"live":false}]}]`

	ipcTracesResult = `[{"at":"2026-09-24T21:00:00Z","decision":"handled",` +
		`"event":{"keys":"Win+Left","source":"tap","key_code":25},` +
		`"context":{"app_id":"com.apple.Finder","app_mode":"native"},` +
		`"winner":"windows.window.move.left","losers":[],"intent":"window.move",` +
		`"action":"Left Half","stages":[{"stage":"classify","detail":"Finder → native"}],` +
		`"params":"zone=left"}]`

	// Name and version empty with the reason present: the builtin plugins are
	// compiled from the rule table and no manifest is loaded.
	ipcPluginMetaResult = `[{"id":"win-kb","name":"","version":"",` +
		`"permissions":["keyboard","accessibility"],"loaded":false,` +
		`"reason":"no manifest is loaded — the builtin plugins are compiled from the rule table"}]`

	ipcAppsResult = `[{"BundleID":"com.apple.Finder","Executable":"Finder","PID":301,` +
		`"DisplayName":"Finder","AppMode":"native","Category":"system"}]`

	ipcUserRulesResult = `[{"id":"user.win+left@com.apple.Finder","key":"Left","modifiers":["Win"],` +
		`"app_modes":[],"app_ids":["com.apple.Finder"],"device_id":"","capability":"window.move",` +
		`"parameters":{"zone":"left"},"emit":false,"chord":"Win+Left","action":"Left Half",` +
		`"priority":80,"specificity":3,"scope":"app"}]`

	// The switcher tiles, in the daemon's MRU order with the highlight on the
	// first one and a skippable row behind it — index, selected and skippable
	// are the daemon's answers about its own session, which is why they travel
	// beside the row instead of being derived by the shell.
	ipcWindowsResult = `[{"window_id":"412","app_id":"com.apple.finder","title":"Downloads",` +
		`"index":0,"selected":true,"skippable":false},` +
		`{"window_id":"887","app_id":"com.microsoft.VSCode","title":"crossos — bridge.go",` +
		`"index":1,"selected":false,"skippable":true}]`
)

// encRaw wraps a literal JSON result in the JSON-RPC envelope, so the payload
// above stays readable as the contract instead of being buried in a map.
func encRaw(result string, id any) string {
	return `{"jsonrpc":"2.0","result":` + result + `,"id":` + mustJSON(id) + "}\n"
}

func mustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func TestIPCClientRoundTrip(t *testing.T) {
	// Bridge calls reach Core over IPC: App bound to IPCCore over the pipe
	// serves Dashboard payloads end to end.
	app := NewApp(NewIPCCore(pipeTransport{stubServer}))
	st := app.GetStatus()
	if !st.Running || len(st.Plugins) != 1 || st.Plugins[0].ID != "win-kb" {
		t.Fatalf("GetStatus over IPC=%+v, want running + win-kb", st)
	}
	if err := app.TogglePlugin("win-kb", false); err != nil {
		t.Fatalf("TogglePlugin over IPC: %v", err)
	}
	if got := app.GetEventLogs(); len(got) != 1 || got[0] != "e1" {
		t.Fatalf("GetEventLogs over IPC=%v, want [e1]", got)
	}
	if got := app.ResetEverything(); len(got) != 3 {
		t.Fatalf("ResetEverything over IPC=%v, want 3 steps", got)
	}
}

func TestIPCClientFraming(t *testing.T) {
	// Byte-level contract: the request line carries jsonrpc/method/id and a
	// trailing newline (same framing core/pkg/ipc serves).
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	go func() {
		buf := make([]byte, 1)
		var line []byte
		for {
			n, err := b.Read(buf)
			if n > 0 {
				line = append(line, buf[:n]...)
				if buf[0] == '\n' {
					break
				}
			}
			if err != nil {
				return
			}
		}
		var req struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			ID      any    `json:"id"`
		}
		if jerr := json.Unmarshal(line, &req); jerr != nil {
			return
		}
		if req.JSONRPC != "2.0" || req.Method != "core.status" || req.ID == nil {
			return
		}
		b.Write([]byte(`{"jsonrpc":"2.0","result":{"running":true},"id":1}` + "\n"))
	}()
	c := NewIPCCore(dialFunc(func() (net.Conn, error) { return a, nil }))
	if !c.IsRunning() {
		t.Fatal("framing round trip failed")
	}
}

type dialFunc func() (net.Conn, error)

func (f dialFunc) Dial() (net.Conn, error) { return f() }

func TestIPCClientDialFailure(t *testing.T) {
	// Unreachable daemon: typed error for the bridge to log (never a hang,
	// never a silent blank page).
	bad := NewIPCCore(dialFunc(func() (net.Conn, error) {
		return nil, errDialRefused
	}))
	app := NewApp(bad)
	if err := app.TogglePlugin("win-kb", false); err == nil {
		t.Fatal("want bridge error on dead transport, got nil")
	}
	// Unknown plugin IDs still fail locally even when transport is live.
	live := NewApp(NewIPCCore(pipeTransport{stubServer}))
	if err := live.TogglePlugin("ghost-plugin", true); err == nil {
		t.Fatal("want unknown-plugin error, got nil")
	}
	if len(live.UILogs()) == 0 {
		t.Fatal("unknown-plugin failure must land in UI logs")
	}
	// GetStatus degrades (not running) rather than panicking on dead transport.
	dead := NewApp(bad)
	if st := dead.GetStatus(); st.Running {
		t.Fatal("dead transport must not report running")
	}
	if got := dead.GetEventLogs(); len(got) != 0 {
		t.Fatalf("dead transport logs=%v, want empty (with UI note)", got)
	}
	if len(dead.UILogs()) == 0 {
		t.Fatal("dead-transport degradation must note UI logs")
	}
}

// errDialRefused stands in for a refused socket/pipe dial.
var errDialRefused = errDialRefusedType{}

type errDialRefusedType struct{}

func (errDialRefusedType) Error() string { return "dial refused (daemon unreachable)" }

func TestIPCClientUnknownMethod(t *testing.T) {
	// RPC-level errors propagate as typed errors (not silent nils).
	c := NewIPCCore(pipeTransport{stubServer})
	if _, err := c.call("core.nope", nil); err == nil {
		t.Fatal("want RPC error, got nil")
	} else if !strings.Contains(err.Error(), "-32601") {
		t.Fatalf("want -32601 code, got %v", err)
	}
}

// IPC source tests — bead cross-os-72g.
//
// The daemon and the shell are separate modules, so nothing at compile time
// holds their declarations together. These tests close that gap from both
// sides: the canned results are JSON text (so a renamed json tag fails), and
// the decoded rows are re-marshalled (so a row cannot spell its keys
// differently on the way out).

func TestIPCSourcesRoundTrip(t *testing.T) {
	app := NewApp(NewIPCCore(pipeTransport{stubServer}))

	matrix, err := app.GetMatrix()
	if err != nil {
		t.Fatalf("GetMatrix: %v", err)
	}
	if len(matrix) != 1 || matrix[0].RuleID != "windows-keyboard.ctrl-c-copy" ||
		matrix[0].Plugin != "win-kb" || matrix[0].Action != "copy" ||
		matrix[0].Keys != "Ctrl+C" || !matrix[0].Enabled ||
		strings.Join(matrix[0].Contexts, ",") != "any,finder" {
		t.Fatalf("GetMatrix=%+v, want the windows-keyboard.ctrl-c-copy row", matrix)
	}

	overs, err := app.GetOverrides()
	if err != nil {
		t.Fatalf("GetOverrides: %v", err)
	}
	if len(overs) != 1 || overs[0].App != "Finder" || overs[0].RuleID != "mac-finder.copy" || overs[0].Enabled {
		t.Fatalf("GetOverrides=%+v, want one disabled Finder override", overs)
	}

	zones, err := app.GetZones()
	if err != nil {
		t.Fatalf("GetZones: %v", err)
	}
	if len(zones) != 1 || zones[0].ID != "left" || zones[0].Name != "Left half" || zones[0].W != 0.5 || zones[0].H != 1 {
		t.Fatalf("GetZones=%+v, want the left half rectangle", zones)
	}

	cmds, err := app.Commands()
	if err != nil {
		t.Fatalf("Commands: %v", err)
	}
	if len(cmds) != 1 || cmds[0].ID != "win-kb.reopen" || cmds[0].Title != "Reopen window" || cmds[0].Plugin != "win-kb" {
		t.Fatalf("Commands=%+v, want win-kb.reopen", cmds)
	}

	schemas, err := app.PluginSchemas()
	if err != nil {
		t.Fatalf("PluginSchemas: %v", err)
	}
	if len(schemas) != 1 || schemas[0].Plugin != "win-wm" || schemas[0].Title != "Window Manager" ||
		schemas[0].Schema["widget"] != "checkbox" {
		t.Fatalf("PluginSchemas=%+v, want a decoded win-wm schema object", schemas)
	}

	audit, err := app.OwnershipAudit()
	if err != nil {
		t.Fatalf("OwnershipAudit: %v", err)
	}
	if len(audit) != 1 || audit[0].Resource != "login-item" || audit[0].ID != "com.crossos.helper" ||
		audit[0].Owner != "crossos" || audit[0].CreatedAt != "2026-09-01T10:00:00Z" {
		t.Fatalf("OwnershipAudit=%+v, want the helper login item", audit)
	}

	st, err := app.TrialState()
	if err != nil {
		t.Fatalf("TrialState: %v", err)
	}
	if st != (TrialState{Plugin: "win-wm", State: "trial", RemainingMS: 12000, TimeoutMS: 30000}) {
		t.Fatalf("TrialState=%+v, want 12000ms remaining of 30000ms", st)
	}

	ready, err := app.Readiness()
	if err != nil {
		t.Fatalf("Readiness: %v", err)
	}
	if len(ready) != 2 || !ready[0].Ready || ready[1].ID != "windows" || ready[1].Detail != "grant Accessibility" {
		t.Fatalf("Readiness=%+v, want keyboard ready and windows blocked with a hint", ready)
	}
	if logs := app.UILogs(); len(logs) != 0 {
		t.Fatalf("a healthy IPC read must not be logged as a failure: %v", logs)
	}
}

// TestIPCSourceRPCNames pins the ten RPC names: a rename on either side of
// this bridge is a "no such method" at runtime, which is exactly how the
// daemon-side table and the shell's Core quietly stop agreeing.
func TestIPCSourceRPCNames(t *testing.T) {
	var seen []recordedCall
	app := NewApp(NewIPCCore(pipeTransport{stubServerWith(&seen)}))
	if _, err := app.GetMatrix(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.GetOverrides(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.GetZones(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Commands(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.PluginSchemas(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.OwnershipAudit(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.TrialState(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Readiness(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SetOverride("Finder", "mac-finder.copy", true); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SetZones([]ZoneRow{{ID: "left", Name: "Left half", W: 0.5, H: 1}}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"config.getMatrix", "config.getOverrides", "config.getZones", "core.commands",
		"core.pluginSchemas", "safety.ownershipAudit", "safety.trialState", "core.readiness",
		"config.setOverride", "config.setZones",
	}
	var got []string
	for _, c := range seen {
		got = append(got, c.Method)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("IPC methods=%v, want %v", got, want)
	}
}

// The Wave 3 source tests (bead w3-shell-bridge) read their payloads from the
// same JSON-text stub the ten do, so the new rows are pinned against the
// daemon's spelling rather than against the shell's own structs.

// TestIPCWave3SourcesRoundTrip is the "one honest row per source" pass over the
// new payloads: every field a page renders is read back off the wire, and a
// healthy read stays out of the error log.
func TestIPCWave3SourcesRoundTrip(t *testing.T) {
	app := NewApp(NewIPCCore(pipeTransport{stubServer}))

	profiles, err := app.Profiles()
	if err != nil {
		t.Fatalf("Profiles: %v", err)
	}
	if len(profiles) != 1 || profiles[0].ID != "windows-11" || !profiles[0].Active {
		t.Fatalf("Profiles=%+v, want the active windows-11 card", profiles)
	}
	// An unavailable capability keeps its reason and is not ticked, and its
	// rollup counts only the behavior-matrix rules — the declared list mixes a
	// rule id with a window zone, so "0 of 0" is the honest count here.
	caps := profiles[0].Capabilities
	if len(caps) != 2 {
		t.Fatalf("Capabilities=%+v, want the live and the declared-but-unavailable one", caps)
	}
	if caps[0].ID != "window-layouts" || !caps[0].Live || caps[0].Enabled != 1 || caps[0].Total != 1 {
		t.Fatalf("live capability=%+v, want 1 of 1 rules live", caps[0])
	}
	if len(caps[0].RuleIDs) != 2 || caps[0].RuleIDs[1] != "left" {
		t.Fatalf("live capability rule_ids=%v, want the declared pair including the window zone", caps[0].RuleIDs)
	}
	if caps[1].Available || caps[1].Live || caps[1].Reason == "" {
		t.Fatalf("unavailable capability=%+v, want the reason kept and no tick", caps[1])
	}

	traces, err := app.Traces()
	if err != nil {
		t.Fatalf("Traces: %v", err)
	}
	if len(traces) != 1 {
		t.Fatalf("Traces=%+v, want the one recorded decision", traces)
	}
	tr := traces[0]
	if tr.At != "2026-09-24T21:00:00Z" || tr.Decision != "handled" || tr.Winner != "windows.window.move.left" ||
		tr.Intent != "window.move" || tr.Action != "Left Half" || tr.Params != "zone=left" {
		t.Fatalf("Traces[0]=%+v, want the handled window.move decision", tr)
	}
	if tr.Event.Keys != "Win+Left" || tr.Event.Source != "tap" || tr.Event.KeyCode != 25 || tr.Event.Device != "" {
		t.Fatalf("Traces[0].Event=%+v, want the physical chord with its virtual-key code", tr.Event)
	}
	if tr.Context.AppID != "com.apple.Finder" || tr.Context.AppMode != "native" {
		t.Fatalf("Traces[0].Context=%+v, want the cached Finder context", tr.Context)
	}
	if len(tr.Stages) != 1 || tr.Stages[0].Stage != "classify" || tr.Stages[0].Detail == "" {
		t.Fatalf("Traces[0].Stages=%+v, want the classify step with its detail", tr.Stages)
	}
	if len(tr.Losers) != 0 {
		t.Fatalf("Traces[0].Losers=%v, want none — the daemon sends [] for an uncontested chord", tr.Losers)
	}

	meta, err := app.PluginMeta()
	if err != nil {
		t.Fatalf("PluginMeta: %v", err)
	}
	if len(meta) != 1 || meta[0].ID != "win-kb" || meta[0].Loaded || meta[0].Name != "" || meta[0].Reason == "" {
		t.Fatalf("PluginMeta=%+v, want win-kb unloaded with the reason kept", meta)
	}
	if strings.Join(meta[0].Permissions, ",") != "keyboard,accessibility" {
		t.Fatalf("PluginMeta[0].Permissions=%v, want the two grants in order", meta[0].Permissions)
	}

	apps, err := app.Apps()
	if err != nil {
		t.Fatalf("Apps: %v", err)
	}
	if len(apps) != 1 || apps[0].BundleID != "com.apple.Finder" || apps[0].DisplayName != "Finder" ||
		apps[0].PID != 301 || apps[0].AppMode != "native" || apps[0].Category != "system" {
		t.Fatalf("Apps=%+v, want the Finder row under the daemon's Go field names", apps)
	}

	rules, err := app.UserRules()
	if err != nil {
		t.Fatalf("UserRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("UserRules=%+v, want the one stored rule", rules)
	}
	rule := rules[0]
	if rule.ID != "user.win+left@com.apple.Finder" || rule.Key != "Left" || rule.Capability != "window.move" ||
		rule.Chord != "Win+Left" || rule.Action != "Left Half" || rule.Scope != "app" ||
		rule.Priority != 80 || rule.Specificity != 3 {
		t.Fatalf("UserRules[0]=%+v, want the rule with its derived fields", rule)
	}
	if strings.Join(rule.Modifiers, ",") != "Win" || strings.Join(rule.AppIDs, ",") != "com.apple.Finder" ||
		len(rule.AppModes) != 0 || rule.Emit {
		t.Fatalf("UserRules[0] scope=%+v, want a Win+Left rule scoped to one app, not emitted", rule)
	}
	// Parameters is an object on the wire, decoded as one — the page renders
	// the capability's arguments instead of re-parsing a JSON string.
	if rule.Parameters["zone"] != "left" {
		t.Fatalf("UserRules[0].Parameters=%v, want the decoded {zone:left}", rule.Parameters)
	}
	if logs := app.UILogs(); len(logs) != 0 {
		t.Fatalf("a healthy Wave 3 read must not be logged as a failure: %v", logs)
	}
}

// TestIPCWave3RPCNames pins the Wave 3 method names in the order the bridge
// calls them. A rename on either side of this boundary is a "no such method" at
// runtime, which is how the daemon's method table and the shell quietly stop
// agreeing.
func TestIPCWave3RPCNames(t *testing.T) {
	var seen []recordedCall
	app := NewApp(NewIPCCore(pipeTransport{stubServerWith(&seen)}))
	if _, err := app.Profiles(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ApplyProfile("windows-11"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Traces(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.PluginMeta(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Apps(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.UserRules(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SetUserRule(UserRuleRow{Key: "C", Modifiers: []string{"Ctrl"}, Capability: "clipboard.copy"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.DeleteUserRule("user.win+left@com.apple.Finder"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"core.profiles", "core.profileApply", "core.traces", "core.pluginMeta", "core.apps",
		"config.getUserRules", "config.setUserRule", "config.deleteUserRule",
	}
	var got []string
	for _, c := range seen {
		got = append(got, c.Method)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Wave 3 IPC methods=%v, want %v", got, want)
	}
}

// TestIPCWave3WriteParams is the write half: the profile the card names, the
// rule dimensions the editor picked, and the reply each is read back through.
//
// The rule payload is the one worth asserting byte-for-byte. config.setUserRule
// decodes into userrules.Rule, which knows nothing of chord, action, priority,
// specificity or scope — so a payload carrying them is a page setting a derived
// rank, and a payload missing one of the picked keys is a rule that compiles to
// something the user did not ask for.
func TestIPCWave3WriteParams(t *testing.T) {
	var seen []recordedCall
	app := NewApp(NewIPCCore(pipeTransport{stubServerWith(&seen)}))

	res, err := app.ApplyProfile("windows-11")
	if err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	// The reply is the daemon's own counts, so the card shows what landed.
	if res["profile"] != "windows-11" || res["rules"] != float64(21) || res["plugins"] != float64(2) {
		t.Fatalf("core.profileApply reply=%v, want the applied profile with its counts", res)
	}
	if got := strings.Join(jsonKeySet(t, seen[0].Params), ","); got != "profile" {
		t.Fatalf("core.profileApply params keys=%s, want profile", got)
	}

	id, err := app.SetUserRule(UserRuleRow{
		Key: "Left", Modifiers: []string{"Win"}, AppIDs: []string{"com.apple.Finder"},
		Capability: "window.move", Parameters: map[string]any{"zone": "left"},
		Chord: "Win+Left", Action: "Left Half", Priority: 999, Specificity: 7, Scope: "window",
	})
	if err != nil {
		t.Fatalf("SetUserRule: %v", err)
	}
	// The id is derived server-side from the dimensions that arrived: a payload
	// whose key, modifier or app scope went missing would derive a different
	// name. The chord is lowercased and the scope is not — a bundle id is
	// already spelled the one way macOS knows it.
	if id != "user.win+left@com.apple.Finder" {
		t.Fatalf("config.setUserRule id=%q, want the id derived from the picked dimensions", id)
	}
	if got := strings.Join(jsonKeySet(t, seen[1].Params), ","); got != "app_ids,app_modes,capability,device_id,emit,id,key,modifiers,parameters" {
		t.Fatalf("config.setUserRule params keys=%s, want the nine keys userrules.Rule declares", got)
	}
	var wp struct {
		Chord      string         `json:"chord"`
		Action     string         `json:"action"`
		Params     map[string]any `json:"parameters"`
		AppIDs     []string       `json:"app_ids"`
		Capability string         `json:"capability"`
	}
	if jerr := json.Unmarshal(seen[1].Params, &wp); jerr != nil {
		t.Fatalf("config.setUserRule params: %v", jerr)
	}
	// The display half is absent from the payload, so the fields the page
	// renders read empty here — which is the point: the daemon derives them.
	if wp.Chord != "" || wp.Action != "" {
		t.Fatalf("config.setUserRule params carried the display half: %s", seen[1].Params)
	}
	if len(wp.AppIDs) != 1 || wp.AppIDs[0] != "com.apple.Finder" || wp.Params["zone"] != "left" ||
		wp.Capability != "window.move" {
		t.Fatalf("config.setUserRule params=%s, want the app id, capability and arguments", seen[1].Params)
	}

	rows, err := app.DeleteUserRule("user.win+left@com.apple.Finder")
	if err != nil {
		t.Fatalf("DeleteUserRule: %v", err)
	}
	// The reply is the table as it now stands, so the editor updates from one
	// response; an empty table is [] and not a null.
	if rows == nil || len(rows) != 0 {
		t.Fatalf("config.deleteUserRule rows=%v, want the emptied table", rows)
	}
	if got := strings.Join(jsonKeySet(t, seen[2].Params), ","); got != "id" {
		t.Fatalf("config.deleteUserRule params keys=%s, want id", got)
	}
	if logs := app.UILogs(); len(logs) != 0 {
		t.Fatalf("a healthy Wave 3 write must not be logged as a failure: %v", logs)
	}
}

// TestIPCSourceWriteParams asserts the two write payloads byte-for-byte. The
// params are snake_case per the frozen contract — the older calls in
// ipc_client.go use the daemon's camelCase pluginId/ruleId, and "normalizing"
// these into that style would make the daemon drop the edit.
func TestIPCSourceWriteParams(t *testing.T) {
	var seen []recordedCall
	app := NewApp(NewIPCCore(pipeTransport{stubServerWith(&seen)}))

	row, err := app.SetOverride("Finder", "mac-finder.copy", true)
	if err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	// The echo carries the three fields the daemon sends and NOTHING else:
	// handleSetOverride answers {app, rule_id, enabled} (pagedata.go), leaving
	// action/keys zero. The page merges this partial row onto the one it
	// already holds (OverridesControl), so an invented field here would hide a
	// daemon that stopped echoing the truth.
	if row.App != "Finder" || row.RuleID != "mac-finder.copy" || !row.Enabled {
		t.Fatalf("SetOverride echo=%+v, want the daemon's stored Finder row", row)
	}
	if row.Keys != "" || row.Action != "" {
		t.Fatalf("SetOverride echo=%+v, want no action/keys: the wire contract omits them", row)
	}

	n, err := app.SetZones([]ZoneRow{{ID: "left", Name: "Left half", W: 0.5, H: 1}, {ID: "top", Name: "Top", W: 1, H: 0.25}})
	if err != nil || n != 2 {
		t.Fatalf("SetZones=%d,%v, want 2 accepted", n, err)
	}
	if len(seen) != 2 {
		t.Fatalf("recorded %d calls, want 2", len(seen))
	}
	if got := strings.Join(jsonKeySet(t, seen[0].Params), ","); got != "app,enabled,rule_id" {
		t.Fatalf("config.setOverride params keys=%s, want app,enabled,rule_id", got)
	}
	if got := strings.Join(jsonKeySet(t, seen[1].Params), ","); got != "zones" {
		t.Fatalf("config.setZones params keys=%s, want zones (a bare array is not the params object)", got)
	}
	var zp struct {
		Zones []ZoneRow `json:"zones"`
	}
	if jerr := json.Unmarshal(seen[1].Params, &zp); jerr != nil {
		t.Fatalf("config.setZones params: %v", jerr)
	}
	if len(zp.Zones) != 2 || zp.Zones[0].ID != "left" || zp.Zones[1].W != 1 {
		t.Fatalf("config.setZones zones=%+v, want both rectangles in order", zp.Zones)
	}
}

// TestIPCSourceKeysMatchContract is the other drift direction: whatever the
// shell decodes must re-marshal to the daemon's key names, or the frontend's
// generated models and the wire disagree even though the decode "worked".
func TestIPCSourceKeysMatchContract(t *testing.T) {
	cases := []struct {
		name string
		row  any
		want string
	}{
		{"MatrixRow", MatrixRow{RuleID: "r", Contexts: []string{}}, "action,contexts,enabled,keys,plugin,rule_id"},
		{"OverrideRow", OverrideRow{App: "a", RuleID: "r"}, "action,app,enabled,keys,rule_id"},
		{"ZoneRow", ZoneRow{ID: "left"}, "h,id,name,w,x,y"},
		{"CommandRow", CommandRow{ID: "c"}, "id,plugin,title"},
		{"SchemaRow", SchemaRow{Plugin: "p"}, "plugin,schema,title"},
		{"AuditRow", AuditRow{Resource: "login-item"}, "created_at,id,owner,resource"},
		{"TrialState", TrialState{Plugin: "p", State: "none"}, "plugin,remaining_ms,state,timeout_ms"},
		{"ReadinessRow", ReadinessRow{ID: "keyboard"}, "detail,id,label,ready"},
		{"ProfileRow", ProfileRow{ID: "p"}, "active,capabilities,description,id,label"},
		// Reason and the two optional trace/event keys are filled here because
		// jsonKeySet lists what is ON the wire, and omitempty means an unset
		// field is not on it.
		{"ProfileCapabilityRow", ProfileCapabilityRow{ID: "c", Reason: "gap"}, "available,enabled,id,label,live,plugin,reason,rule_ids,total"},
		{"TraceRow", TraceRow{At: "t"}, "action,at,context,decision,event,intent,losers,params,stages,winner"},
		{"TraceEvent", TraceEvent{Keys: "Ctrl+C", Device: "kbd0"}, "device,key_code,keys,source"},
		{"TraceContext", TraceContext{AppID: "a", WindowID: "7", WinClass: "AXWindow"}, "app_id,app_mode,win_class,window_id"},
		{"StageRow", StageRow{Stage: "classify"}, "detail,stage"},
		{"PluginMetaRow", PluginMetaRow{ID: "win-kb", Reason: "no manifest"}, "id,loaded,name,permissions,reason,version"},
		// The one untagged row: its keys ARE the Go field names, because the
		// daemon serves ctx.ApplicationInfo, which declares no tags. This is the
		// spell TestAppRowMatchesTheDaemonsRow holds to the daemon's struct.
		{"AppRow", AppRow{BundleID: "com.apple.Finder"}, "AppMode,BundleID,Category,DisplayName,Executable,PID"},
		{"UserRuleRow", UserRuleRow{ID: "user.ctrl+c", Parameters: map[string]any{"zone": "left"}}, "action,app_ids,app_modes,capability,chord,device_id,emit,id,key,modifiers,parameters,priority,scope,specificity"},
	}
	for _, c := range cases {
		if got := strings.Join(jsonKeySet(t, c.row), ","); got != c.want {
			t.Errorf("%s marshals keys %s, want %s", c.name, got, c.want)
		}
	}
}

// jsonKeySet marshals v and returns its top-level JSON keys, sorted.
func jsonKeySet(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("%T did not marshal to a JSON object: %s", v, raw)
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// fixed answers each listed method with its own raw result and reports an
// unknown method for everything else, so a test pointed at the wrong RPC name
// fails on its assertion instead of hanging on a read.
type fixed map[string]string

func (f fixed) serve(conn net.Conn) {
	serveJSONRPC(conn, func(req *ipcRequest) string {
		res, ok := f[req.Method]
		if !ok {
			return `{"jsonrpc":"2.0","error":{"code":-32601,"message":"no such method: ` + req.Method + `"},"id":` + mustJSON(req.ID) + "}\n"
		}
		return encRaw(res, req.ID)
	})
}

func TestIPCEmptyCollectionsAreEmptyNotNil(t *testing.T) {
	// The contract's normal empty answer is []. It must reach the page as an
	// empty, non-nil slice with nothing in the error log.
	empty := fixed{
		"config.getMatrix": ipcEmptyCollections, "config.getOverrides": ipcEmptyCollections,
		"config.getZones": ipcEmptyCollections, "core.commands": ipcEmptyCollections,
		"core.pluginSchemas": ipcEmptyCollections, "safety.ownershipAudit": ipcEmptyCollections,
		"core.readiness": ipcEmptyCollections, "core.profiles": ipcEmptyCollections,
		"core.traces": ipcEmptyCollections, "core.pluginMeta": ipcEmptyCollections,
		"core.apps": ipcEmptyCollections, "config.getUserRules": ipcEmptyCollections,
		"config.deleteUserRule": ipcEmptyCollections, "core.windows": ipcEmptyCollections,
	}
	app := NewApp(NewIPCCore(pipeTransport{serve: empty.serve}))
	for _, c := range sourceReaders() {
		if err := c.read(app); err != nil {
			t.Fatalf("%s: an empty daemon answer is not an error, got %v", c.name, err)
		}
	}
	if logs := app.UILogs(); len(logs) != 0 {
		t.Fatalf("[] is a valid answer and must not be logged as an anomaly: %v", logs)
	}
}

func TestIPCNullCollectionIsEmptyNotNil(t *testing.T) {
	// null breaks the contract. The page still gets a list it can render and
	// the UI log says the daemon is the one at fault — never a silent nothing.
	nulls := fixed{
		"config.getMatrix": "null", "config.getOverrides": "null", "config.getZones": "null",
		"core.commands": "null", "core.pluginSchemas": "null", "safety.ownershipAudit": "null",
		"core.readiness": "null", "core.profiles": "null", "core.traces": "null",
		"core.pluginMeta": "null", "core.apps": "null", "config.getUserRules": "null",
		"config.deleteUserRule": "null", "core.windows": "null",
	}
	app := NewApp(NewIPCCore(pipeTransport{serve: nulls.serve}))
	for _, c := range sourceReaders() {
		if err := c.read(app); err != nil {
			t.Fatalf("%s: a null list must degrade to empty, not to an error page: %v", c.name, err)
		}
	}
	if logs := app.UILogs(); len(logs) != len(sourceReaders()) {
		t.Fatalf("each null source must leave one UI-log note, got %v", logs)
	}
	rows, err := app.GetMatrix()
	if err != nil {
		t.Fatal(err)
	}
	if rows == nil || len(rows) != 0 {
		t.Fatalf("GetMatrix over a null result=%v, want an empty non-nil slice", rows)
	}
}

func TestIPCTrialStateNullIsAnError(t *testing.T) {
	// safety.trialState owes an object even with no trial in flight. A null
	// would otherwise render as a countdown with no owner and no timeout —
	// indistinguishable from "idle" — so it is a bridge error, not a value.
	app := NewApp(NewIPCCore(pipeTransport{serve: fixed{"safety.trialState": "null"}.serve}))
	if _, err := app.TrialState(); err == nil {
		t.Fatal("null trial state must not pass as an idle trial")
	}
	if logs := app.UILogs(); len(logs) != 1 || !strings.Contains(logs[0], "TrialState") {
		t.Fatalf("null trial state must reach the UI log, got %v", logs)
	}
	// The honest idle answer is a real object, and it passes clean.
	ok := NewApp(NewIPCCore(pipeTransport{serve: fixed{
		"safety.trialState": `{"plugin":"","state":"none","remaining_ms":0,"timeout_ms":30000}`,
	}.serve}))
	st, err := ok.TrialState()
	if err != nil {
		t.Fatalf("idle trial state: %v", err)
	}
	if st != (TrialState{Plugin: "", State: "none", RemainingMS: 0, TimeoutMS: 30000}) {
		t.Fatalf("idle trial state=%+v, want state none with a 30000ms timeout", st)
	}
	if logs := ok.UILogs(); len(logs) != 0 {
		t.Fatalf("an idle trial is a value, not an incident: %v", logs)
	}
}

func TestIPCSourceFailuresPropagate(t *testing.T) {
	// An RPC error on any source reaches the page as an error, never as an
	// empty table: "no rules configured" and "the daemon said no" are
	// different product states and the UI has to be able to say which.
	methods := []string{
		"config.getMatrix", "config.getOverrides", "config.setOverride", "config.getZones",
		"config.setZones", "core.commands", "core.pluginSchemas", "safety.ownershipAudit",
		"safety.trialState", "core.readiness", "core.profiles", "core.profileApply",
		"core.traces", "core.pluginMeta", "core.apps", "config.getUserRules",
		"config.setUserRule", "config.deleteUserRule", "core.windows",
		"core.switcherWait", "core.switcherFocus",
	}
	for _, m := range methods {
		app := NewApp(NewIPCCore(pipeTransport{serve: erroring{method: m}.serve}))
		err := callSource(t, app, m)
		if err == nil {
			t.Fatalf("%s: a failed RPC must surface as an error", m)
		}
		if !strings.Contains(err.Error(), m) {
			t.Fatalf("%s: error %q does not name the source that failed", m, err)
		}
		if logs := app.UILogs(); len(logs) != 1 || !strings.Contains(logs[0], m) {
			t.Fatalf("%s: failure must reach the UI log, got %v", m, logs)
		}
	}
}

// callSource invokes the bridge method that speaks the given RPC name.
func callSource(t *testing.T, a *App, method string) error {
	t.Helper()
	switch method {
	case "config.getMatrix":
		_, err := a.GetMatrix()
		return err
	case "config.getOverrides":
		_, err := a.GetOverrides()
		return err
	case "config.setOverride":
		_, err := a.SetOverride("Finder", "mac-finder.copy", true)
		return err
	case "config.getZones":
		_, err := a.GetZones()
		return err
	case "config.setZones":
		_, err := a.SetZones([]ZoneRow{{ID: "left", W: 1, H: 1}})
		return err
	case "core.commands":
		_, err := a.Commands()
		return err
	case "core.pluginSchemas":
		_, err := a.PluginSchemas()
		return err
	case "safety.ownershipAudit":
		_, err := a.OwnershipAudit()
		return err
	case "safety.trialState":
		_, err := a.TrialState()
		return err
	case "core.readiness":
		_, err := a.Readiness()
		return err
	case "core.profiles":
		_, err := a.Profiles()
		return err
	case "core.profileApply":
		_, err := a.ApplyProfile("windows-11")
		return err
	case "core.traces":
		_, err := a.Traces()
		return err
	case "core.pluginMeta":
		_, err := a.PluginMeta()
		return err
	case "core.apps":
		_, err := a.Apps()
		return err
	case "config.getUserRules":
		_, err := a.UserRules()
		return err
	case "config.setUserRule":
		_, err := a.SetUserRule(UserRuleRow{Key: "C", Modifiers: []string{"Ctrl"}, Capability: "clipboard.copy"})
		return err
	case "config.deleteUserRule":
		_, err := a.DeleteUserRule("user.win+left@com.apple.Finder")
		return err
	case "core.windows":
		_, err := a.Windows()
		return err
	case "core.switcherWait":
		_, err := a.SwitcherWait(2000)
		return err
	case "core.switcherFocus":
		return a.SwitcherFocus("412")
	}
	t.Fatalf("no bridge method speaks %s", method)
	return nil
}

// erroring answers one method with a JSON-RPC error, the shape a rejected
// write (unknown rule, invalid zone) takes in production.
type erroring struct {
	method string
}

func (e erroring) serve(conn net.Conn) {
	serveJSONRPC(conn, func(req *ipcRequest) string {
		if req.Method != e.method {
			return `{"jsonrpc":"2.0","error":{"code":-32601,"message":"no such method"},"id":` + mustJSON(req.ID) + "}\n"
		}
		return `{"jsonrpc":"2.0","error":{"code":-32000,"message":"unknown ` + req.Method + `"},"id":` + mustJSON(req.ID) + "}\n"
	})
}

func TestIPCSourceDecodeFailureIsNotEmptiness(t *testing.T) {
	// A source that decodes to the wrong shape is a broken bridge. Turning it
	// into an empty table would be the one failure mode that looks like
	// success, so it stays an error.
	broken := fixed{
		"config.getMatrix":  `{"rule_id":"r"}`,                   // object where a list is promised
		"core.readiness":    `[{"id":"keyboard","ready":"yes"}]`, // wrong field type
		"safety.trialState": `{"plugin":"win-wm","remaining_ms":"soon"}`,
	}
	app := NewApp(NewIPCCore(pipeTransport{serve: broken.serve}))
	if _, err := app.GetMatrix(); err == nil {
		t.Fatal("an object where a list is promised must not read as an empty matrix")
	}
	if _, err := app.Readiness(); err == nil {
		t.Fatal("a mistyped readiness row must not read as an empty checklist")
	}
	if _, err := app.TrialState(); err == nil {
		t.Fatal("a mistyped trial state must not read as an idle trial")
	}
}

// The switcher over IPC (bead ws-4): the tiles decode field for field, the two
// calls carry the daemon's own keys, and the wait is the one call with a
// deadline — set past the daemon's cap, because the cap is the longest answer
// that can come back however briefly the shell asked.

func TestIPCSwitcherRoundTrip(t *testing.T) {
	app := NewApp(NewIPCCore(pipeTransport{stubServer}))

	rows, err := app.Windows()
	if err != nil {
		t.Fatalf("Windows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("Windows=%+v, want the daemon's two tiles", rows)
	}
	front := rows[0]
	if front.WindowID != "412" || front.AppID != "com.apple.finder" || front.Title != "Downloads" ||
		front.Index != 0 || !front.Selected || front.Skippable {
		t.Fatalf("Windows[0]=%+v, want the highlighted front tile", front)
	}
	if rows[1].WindowID != "887" || rows[1].Index != 1 || rows[1].Selected || !rows[1].Skippable {
		t.Fatalf("Windows[1]=%+v, want the skippable tile behind it", rows[1])
	}

	trig, err := app.SwitcherWait(2000)
	if err != nil {
		t.Fatalf("SwitcherWait: %v", err)
	}
	if !trig.Triggered || trig.Action != "summon" {
		t.Fatalf("SwitcherWait=%+v, want the summon the daemon queued", trig)
	}
	if err := app.SwitcherFocus("412"); err != nil {
		t.Fatalf("SwitcherFocus: %v", err)
	}
	if logs := app.UILogs(); len(logs) != 0 {
		t.Fatalf("a healthy switcher call must not be logged as a failure: %v", logs)
	}
}

func TestIPCSwitcherRPCNamesAndParams(t *testing.T) {
	var seen []recordedCall
	app := NewApp(NewIPCCore(pipeTransport{stubServerWith(&seen)}))
	if _, err := app.Windows(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SwitcherWait(1500); err != nil {
		t.Fatal(err)
	}
	if err := app.SwitcherFocus("887"); err != nil {
		t.Fatal(err)
	}
	want := []string{"core.windows", "core.switcherWait", "core.switcherFocus"}
	var got []string
	for _, c := range seen {
		got = append(got, c.Method)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("switcher IPC methods=%v, want %v", got, want)
	}
	// The caller's budget travels whole. A payload that dropped or renamed the
	// key would leave the daemon waiting on a timeout it never received, which
	// reads as a switcher that opens on someone else's schedule.
	if got := string(seen[1].Params); got != `{"timeout_ms":1500}` {
		t.Fatalf("core.switcherWait params=%s, want the caller's budget under timeout_ms", got)
	}
	if got := string(seen[2].Params); got != `{"window_id":"887"}` {
		t.Fatalf("core.switcherFocus params=%s, want the window it was pointed at", got)
	}
}

func TestIPCSwitcherWaitNullIsAnError(t *testing.T) {
	// The wait owes an object even when nothing happened: {triggered:false} is
	// the expired answer, and a null would decode into the same zero value —
	// indistinguishable from a poll that simply ran out of budget.
	app := NewApp(NewIPCCore(pipeTransport{serve: fixed{"core.switcherWait": "null"}.serve}))
	if _, err := app.SwitcherWait(1000); err == nil {
		t.Fatal("a null wait answer must not read as an expired poll")
	}
	if logs := app.UILogs(); len(logs) != 1 || !strings.Contains(logs[0], "SwitcherWait") {
		t.Fatalf("a null wait answer must reach the UI log, got %v", logs)
	}
	// The honest expired answer is a real object, and it passes clean.
	idle := NewApp(NewIPCCore(pipeTransport{serve: fixed{
		"core.switcherWait": `{"triggered":false}`,
	}.serve}))
	trig, err := idle.SwitcherWait(1000)
	if err != nil {
		t.Fatalf("an expired wait: %v", err)
	}
	if trig.Triggered || trig.Action != "" {
		t.Fatalf("an expired wait=%+v, want nothing triggered", trig)
	}
	if logs := idle.UILogs(); len(logs) != 0 {
		t.Fatalf("an expired poll is an answer, not an incident: %v", logs)
	}
}

// deadlineConn records the read deadline the client sets, so the wait's budget
// can be held to the number the daemon is allowed to take rather than to
// whatever this test happens to assume.
type deadlineConn struct {
	net.Conn
	readDeadlines []time.Time
}

func (d *deadlineConn) SetReadDeadline(t time.Time) error {
	d.readDeadlines = append(d.readDeadlines, t)
	return d.Conn.SetReadDeadline(t)
}

// recordingTransport is pipeTransport plus a handle on the connection the
// client dialed, wrapped so the deadline it sets is visible here.
type recordingTransport struct {
	serve func(net.Conn)
	dials []*deadlineConn
}

func (r *recordingTransport) Dial() (net.Conn, error) {
	a, b := net.Pipe()
	go r.serve(b)
	d := &deadlineConn{Conn: a}
	r.dials = append(r.dials, d)
	return d, nil
}

// daemonConstInt reads one integer constant out of a daemon source file. The
// wait's read deadline has to clear the daemon's OWN cap, and a literal copied
// into this test is exactly the number that rots when the cap moves.
func daemonConstInt(t *testing.T, path, name string) int {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, d := range f.Decls {
		gen, ok := d.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, n := range vs.Names {
				if n.Name != name {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok {
					t.Fatalf("%s: const %s is not a literal, which this reader does not name", path, name)
				}
				v, err := strconv.Atoi(lit.Value)
				if err != nil {
					t.Fatalf("%s: const %s=%s: %v", path, name, lit.Value, err)
				}
				return v
			}
		}
	}
	t.Fatalf("%s declares no const %s", path, name)
	return 0
}

func TestIPCSwitcherWaitDeadlineClearsTheDaemonsCap(t *testing.T) {
	// The daemon caps one wait, so that cap — not the caller's ask — is the
	// longest answer that can come back. A deadline at or below it races a
	// reply the daemon is still entitled to send, and the user sees a switcher
	// that opens a beat too late to be worth pressing.
	path := filepath.Join("..", "..", "core", "cmd", "crossos", "switcher.go")
	serverMax := time.Duration(daemonConstInt(t, path, "switcherWaitMaxMs")) * time.Millisecond
	rec := &recordingTransport{serve: stubServer}
	c := NewIPCCore(rec)
	start := time.Now()
	// A deliberately short ask: the deadline is measured from the cap, so a
	// poll for 200ms is not the thing that decides how long we will read.
	if _, err := c.SwitcherWait(200); err != nil {
		t.Fatalf("SwitcherWait: %v", err)
	}
	if len(rec.dials) != 1 {
		t.Fatalf("the wait dialled %d connections, want 1", len(rec.dials))
	}
	got := rec.dials[0].readDeadlines
	if len(got) != 1 {
		t.Fatalf("the wait set %d read deadlines, want exactly one", len(got))
	}
	if margin := got[0].Sub(start); margin <= serverMax {
		t.Fatalf("the wait's read deadline is %v from the start of the call, want strictly more than "+
			"the daemon's %v cap", margin, serverMax)
	}
}

func TestIPCCallSetsNoDeadline(t *testing.T) {
	// Every other call keeps what it had: no deadline of its own, so the
	// switcher's budget cannot quietly start cutting reads short everywhere
	// else in the bridge.
	rec := &recordingTransport{serve: stubServer}
	c := NewIPCCore(rec)
	if _, err := c.call("core.status", nil); err != nil {
		t.Fatalf("core.status: %v", err)
	}
	if got := rec.dials[0].readDeadlines; len(got) != 0 {
		t.Fatalf("a plain call set read deadlines %v, want none", got)
	}
}

func TestIPCCallWithinStopsReadingAtItsBudget(t *testing.T) {
	// The budget is a real one: a daemon that never answers ends as a typed
	// error instead of a poll the UI thread is parked inside forever.
	silent := func(conn net.Conn) {
		serveJSONRPC(conn, func(*ipcRequest) string { return "" })
	}
	c := NewIPCCore(pipeTransport{serve: silent})
	start := time.Now()
	_, err := c.callWithin("core.switcherWait", map[string]any{"timeout_ms": 100}, 20*time.Millisecond)
	if err == nil {
		t.Fatal("a daemon that never answers must end the read, not hold it")
	}
	if !strings.Contains(err.Error(), "core.switcherWait") {
		t.Fatalf("the expired read must name the call it cut off, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("the read ran for %v, want the budget to end it", elapsed)
	}
}
