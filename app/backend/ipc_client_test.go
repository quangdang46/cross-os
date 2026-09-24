// IPC client tests — bead cross-os-80g bridge criterion, plus the ten page
// sources added in cross-os-72g.
//
// Pass criteria mapping:
//  1. Bridge calls reach Core over IPC → TestIPCClientRoundTrip (in-memory
//     net.Pipe pair speaking the §3.9 framing against a stub server) +
//     TestIPCClientFraming (byte-level request shape matches core/pkg/ipc).
//  2. Bridge/IPC failures in UI logs, never silent → TestIPCClientDialFailure
//     (unreachable transport surfaces a typed error the bridge logs).
//  3. The ten sources cross the wire with the contract's exact key names and
//     RPC names → TestIPCSourcesRoundTrip, TestIPCSourceRPCNames,
//     TestIPCSourceWriteParams, TestIPCSourceKeysMatchContract.
//  4. A null, empty, broken or refused source is never silently "nothing" →
//     TestIPCEmptyCollectionsAreEmptyNotNil, TestIPCNullCollectionIsEmptyNotNil,
//     TestIPCTrialStateNullIsAnError, TestIPCSourceDecodeFailureIsNotEmptiness,
//     TestIPCSourceFailuresPropagate.
package shell

import (
	"encoding/json"
	"net"
	"sort"
	"strings"
	"testing"
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

	default:
		return errResp(-32601, "no such method: "+method)
	}
}

// The literal daemon results for the ten sources, exactly as the frozen
// contract spells them. Keys here are the whole point of this file's new
// half: an empty collection is [], an object keeps every key, and no field is
// renamed on the way.
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
		"core.readiness": ipcEmptyCollections,
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
		"core.readiness": "null",
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
		"safety.trialState", "core.readiness",
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
