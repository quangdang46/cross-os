// User-rule IPC handler tests (bead w2-userrules).
//
// The wire decode structs are declared independently of the handler's own
// types, for the reason pagedata_test.go gives: decoding into the handler's
// struct would pass with every json tag wrong, and the tags are the contract.
//
// Two of these tests are cross-checks rather than handler tests. The key
// vocabulary and the matrix chord renderer are two directions of one table —
// name→code for the builder, code→name for the display — and the only thing
// keeping them agreeing is that a test reads both.

package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"crossos/core/pkg/event"
	"crossos/core/pkg/ipc"
	"crossos/core/pkg/keyboard"
	"crossos/core/pkg/userrules"
	"crossos/core/pkg/winlayout"
)

type wireUserRule struct {
	ID          string          `json:"id"`
	Key         string          `json:"key"`
	Modifiers   []string        `json:"modifiers"`
	AppModes    []string        `json:"app_modes"`
	AppIDs      []string        `json:"app_ids"`
	DeviceID    string          `json:"device_id"`
	Capability  string          `json:"capability"`
	Parameters  json.RawMessage `json:"parameters"`
	Emit        bool            `json:"emit"`
	Chord       string          `json:"chord"`
	Action      string          `json:"action"`
	Priority    int             `json:"priority"`
	Specificity int             `json:"specificity"`
	Scope       string          `json:"scope"`
}

type wireVocabulary struct {
	AppModes      []userrules.Option     `json:"app_modes"`
	AppCategories []userrules.Option     `json:"app_categories"`
	Modifiers     []userrules.Option     `json:"modifiers"`
	Keys          []userrules.Key        `json:"keys"`
	Capabilities  []userrules.Capability `json:"capabilities"`
	Actions       []userrules.Action     `json:"actions"`
}

func testUserRuleService(t *testing.T) *userRuleService {
	t.Helper()
	store, err := userrules.New("")
	if err != nil {
		t.Fatalf("userrules.New: %v", err)
	}
	return newUserRuleService(store)
}

// create writes one rule through the handler and fails on the RPC error, so a
// fixture is never half-built.
func create(t *testing.T, s *userRuleService, payload string) string {
	t.Helper()
	res, rerr := s.setUserRule(json.RawMessage(payload))
	if rerr != nil {
		t.Fatalf("setUserRule(%s): %d %s", payload, rerr.Code, rerr.Message)
	}
	out := decode[map[string]string](t, res, rerr)
	return out["id"]
}

func TestUserRuleCreateReadUpdateDelete(t *testing.T) {
	svc := testUserRuleService(t)

	// An empty table reaches the shell as [], not null: a page that trips
	// over a missing value shows a fault where it should show "no rules yet".
	rows := userRuleRows(t, svc)
	if len(rows) != 0 {
		t.Fatalf("a fresh table = %+v, want empty", rows)
	}

	id := create(t, svc, `{"key":"C","modifiers":["Ctrl"],"app_modes":["native"],"capability":"clipboard.copy","emit":true}`)
	if id != "user.ctrl+c@native" {
		t.Fatalf("id = %q, want the derived one", id)
	}

	// The row is the stored rule plus the display fields, and the derived
	// three are there because the builder cannot work them out itself: they
	// are what rule.Resolve ranks on.
	rows = userRuleRows(t, svc)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.ID != id || row.Key != "C" || row.Capability != "clipboard.copy" || !row.Emit {
		t.Fatalf("row = %+v, want the stored rule", row)
	}
	if row.Chord != "Ctrl+C" {
		t.Fatalf("chord = %q, want Ctrl+C", row.Chord)
	}
	if row.Action != "clipboard.copy" {
		t.Fatalf("action = %q, want the intent ID for a non-window capability", row.Action)
	}
	if row.Scope != "app" || row.Priority != 80 || row.Specificity != 2 {
		t.Fatalf("derived rank = %d/%d/%s, want 80/2/app", row.Priority, row.Specificity, row.Scope)
	}

	// An edit carries the id, so the store replaces rather than adds.
	edited := create(t, svc, `{"id":"`+id+`","key":"C","modifiers":["Ctrl"],"app_modes":["native"],"capability":"clipboard.copyPath","emit":true}`)
	if edited != id {
		t.Fatalf("edit renamed the rule to %q, want %q", edited, id)
	}
	rows = userRuleRows(t, svc)
	if len(rows) != 1 || rows[0].Capability != "clipboard.copyPath" {
		t.Fatalf("rows = %+v, want the one edited rule", rows)
	}

	// Delete answers with the table as it now stands, so the editor updates
	// from the write it already made.
	got, rerr := svc.deleteUserRule(json.RawMessage(`{"id":"` + id + `"}`))
	if rerr != nil {
		t.Fatalf("deleteUserRule: %d %s", rerr.Code, rerr.Message)
	}
	if after := decodeRows[wireUserRule](t, got, rerr); len(after) != 0 {
		t.Fatalf("rows after delete = %+v, want none", after)
	}
}

// TestUserRuleRejectionsAreTyped: a bad edit is a typed RPCError carrying the
// store's reason, and it changes nothing. The reason matters as much as the
// refusal — the editor shows it beside the field that caused it, so "invalid
// request" would tell the user nothing they can act on.
func TestUserRuleRejectionsAreTyped(t *testing.T) {
	svc := testUserRuleService(t)
	id := create(t, svc, `{"key":"C","modifiers":["Ctrl"],"app_modes":["native"],"capability":"clipboard.copy","emit":true}`)

	for _, tc := range []struct {
		name    string
		payload string
		want    []string
	}{
		{
			name:    "unknown app mode",
			payload: `{"key":"C","modifiers":["Ctrl"],"app_modes":["nautilus"],"capability":"clipboard.copy"}`,
			want:    []string{"nautilus", "app mode"},
		},
		{
			name:    "unknown capability",
			payload: `{"key":"C","modifiers":["Ctrl"],"capability":"window.explode"}`,
			want:    []string{"window.explode", "capability"},
		},
		{
			name:    "unknown key",
			payload: `{"key":"Kkk","modifiers":["Ctrl"],"capability":"clipboard.copy"}`,
			want:    []string{"Kkk", "key"},
		},
		{
			name:    "unknown modifier",
			payload: `{"key":"C","modifiers":["Hyper"],"capability":"clipboard.copy"}`,
			want:    []string{"Hyper", "modifier"},
		},
		{
			name:    "a second copy of a rule that exists",
			payload: `{"key":"C","modifiers":["Ctrl"],"app_modes":["native"],"capability":"clipboard.copy"}`,
			want:    []string{"duplicate"},
		},
		{
			name:    "an edit of a rule that is gone",
			payload: `{"id":"user.zzz@any","key":"C","modifiers":["Ctrl"],"capability":"clipboard.copy"}`,
			want:    []string{"no rule"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, rerr := svc.setUserRule(json.RawMessage(tc.payload))
			if rerr == nil {
				t.Fatal("the edit must be refused")
			}
			if rerr.Code != ipc.ErrInvalid {
				t.Fatalf("code = %d, want ErrInvalid (%d)", rerr.Code, ipc.ErrInvalid)
			}
			for _, want := range tc.want {
				if !strings.Contains(rerr.Message, want) {
					t.Fatalf("message %q does not say %q", rerr.Message, want)
				}
			}
			if rows := userRuleRows(t, svc); len(rows) != 1 || rows[0].ID != id {
				t.Fatalf("a refused edit changed the table: %+v", rows)
			}
		})
	}

	// A malformed payload and a nameless delete are both bad params, which is
	// a different code from a refused rule.
	if _, rerr := svc.setUserRule(json.RawMessage(`not json`)); rerr == nil || rerr.Code != ipc.ErrBadParams {
		t.Fatalf("malformed payload = %v, want ErrBadParams", rerr)
	}
	if _, rerr := svc.deleteUserRule(json.RawMessage(`{}`)); rerr == nil || rerr.Code != ipc.ErrBadParams {
		t.Fatalf("nameless delete = %v, want ErrBadParams", rerr)
	}
	if _, rerr := svc.deleteUserRule(json.RawMessage(`{"id":"user.zzz@any"}`)); rerr == nil || rerr.Code != ipc.ErrInvalid {
		t.Fatalf("deleting a rule that is gone = %v, want ErrInvalid", rerr)
	}
}

// TestVocabularyServesEveryPicker: the builder's four questions are answered
// in one reply, and each list is non-empty and comes from the package that
// owns it — a picker served an empty list renders a control with nothing to
// pick, which is worse than not offering the picker at all.
func TestVocabularyServesEveryPicker(t *testing.T) {
	svc := testUserRuleService(t)
	v := userRuleVocabularyRows(t, svc)
	if len(v.AppModes) == 0 || len(v.AppCategories) == 0 {
		t.Fatalf("app vocabularies are empty: %+v", v)
	}
	if len(v.Modifiers) != 4 {
		t.Fatalf("modifiers = %d, want the four bits keyboard.State keeps", len(v.Modifiers))
	}
	if len(v.Keys) == 0 || len(v.Capabilities) == 0 || len(v.Actions) == 0 {
		t.Fatalf("a picker list is empty: keys=%d capabilities=%d actions=%d",
			len(v.Keys), len(v.Capabilities), len(v.Actions))
	}
	if len(v.Actions) != len(winlayout.AllActions) {
		t.Fatalf("actions = %d, want the %d winlayout rows", len(v.Actions), len(winlayout.AllActions))
	}
	// Every list the page polls is non-nil on the wire, so an empty one is []
	// and not a missing value.
	vocab, rerr := svc.getUserRuleVocabulary(nil)
	if rerr != nil {
		t.Fatalf("getUserRuleVocabulary: %d %s", rerr.Code, rerr.Message)
	}
	raw, err := json.Marshal(vocab)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"app_modes"`, `"app_categories"`, `"modifiers"`, `"keys"`, `"capabilities"`, `"actions"`} {
		if !strings.Contains(string(raw), key+":[") {
			t.Fatalf("%s is not an array on the wire: %s", key, raw)
		}
	}
}

// TestKeyVocabularyAgreesWithTheMatrixRenderer is the cross-check that matters
// most here. userrules.Keys() is the builder's name→code direction and
// keyName (pagedata.go) is the display's code→name direction; they are two
// halves of one table and the only thing keeping them agreeing is that a test
// reads both. If they drift, a user picks "Left" in the builder and the matrix
// shows a different key for the same rule.
func TestKeyVocabularyAgreesWithTheMatrixRenderer(t *testing.T) {
	for _, k := range userrules.Keys() {
		if got := keyName(k.Code); got != k.Value {
			t.Errorf("key %#x is %q in the picker and %q in the matrix renderer", k.Code, k.Value, got)
		}
	}
}

// TestModifierVocabularyAgreesWithTheMatrixRenderer: the same two-way
// problem for the modifier bits. A user rule's chord has to render in the
// matrix with the same names the builder offered, or Ctrl+Win in one page is
// Win+Ctrl in the next.
func TestModifierVocabularyAgreesWithTheMatrixRenderer(t *testing.T) {
	bits := map[string]uint32{
		"Ctrl":  keyboard.ModCtrl,
		"Shift": keyboard.ModShift,
		"Alt":   keyboard.ModAlt,
		"Win":   keyboard.ModMeta,
	}
	for _, m := range userrules.Modifiers() {
		bit, ok := bits[m.Value]
		if !ok {
			t.Errorf("picker offers modifier %q, which the matrix renderer does not name", m.Value)
			continue
		}
		if got := chord(event.CompiledRule{KeyCode: 0x43, Modifiers: bit}); got != m.Value+"+C" {
			t.Errorf("the matrix renders %s as %q, want %q+C", m.Value, got, m.Value)
		}
		delete(bits, m.Value)
	}
	for name := range bits {
		t.Errorf("the matrix renders %q, which the picker does not offer", name)
	}
}

// TestUserRulesPathSitsBesideTheSettings: the table is a sibling of the
// settings document, and a memory-only daemon (the "" a test runs with) gets
// no path at all — writing to "./user-rules.json" from a test would put a file
// in the working directory.
func TestUserRulesPathSitsBesideTheSettings(t *testing.T) {
	if got := userRulesPath(""); got != "" {
		t.Fatalf("userRulesPath(\"\") = %q, want no path", got)
	}
	dir := t.TempDir()
	got := userRulesPath(filepath.Join(dir, "config.json"))
	if got != filepath.Join(dir, "user-rules.json") {
		t.Fatalf("userRulesPath = %q, want a sibling of the settings file in %q", got, dir)
	}
}

// userRuleRows reads the table through the handler and decodes it. Almost
// every assertion here is about the rows, and going through the handler each
// time is what keeps the wire names honest.
func userRuleRows(t *testing.T, svc *userRuleService) []wireUserRule {
	t.Helper()
	res, rerr := svc.getUserRules(nil)
	if rerr != nil {
		t.Fatalf("getUserRules: %d %s", rerr.Code, rerr.Message)
	}
	return decodeRows[wireUserRule](t, res, rerr)
}

// userRuleVocabularyRows reads the picker lists through the handler.
func userRuleVocabularyRows(t *testing.T, svc *userRuleService) wireVocabulary {
	t.Helper()
	res, rerr := svc.getUserRuleVocabulary(nil)
	if rerr != nil {
		t.Fatalf("getUserRuleVocabulary: %d %s", rerr.Code, rerr.Message)
	}
	return decode[wireVocabulary](t, res, rerr)
}
