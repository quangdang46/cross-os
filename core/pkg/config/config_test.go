package config

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func testSchema() Schema {
	return Schema{
		"theme":    {Type: "string"},
		"timeout":  {Type: "int", Required: true},
		"enabled":  {Type: "bool"},
		"advanced": {Type: "object"},
		"excluded": {Type: "array"},
	}
}

func testDefaults() json.RawMessage {
	return json.RawMessage(`{"theme":"dark","timeout":30,"enabled":true}`)
}

func TestPrecedence(t *testing.T) {
	// session > plugin > user > defaults.
	m, err := New(testDefaults(), testSchema())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetUser(json.RawMessage(`{"theme":"light"}`)); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPlugin("p1", json.RawMessage(`{"theme":"blue","timeout":60}`)); err != nil {
		t.Fatal(err)
	}
	if err := m.SetSession(json.RawMessage(`{"timeout":5}`)); err != nil {
		t.Fatal(err)
	}
	got := m.Merged()
	if got["theme"] != "blue" {
		t.Fatalf("theme: got %v, want plugin blue", got["theme"])
	}
	if got["timeout"] != float64(5) {
		t.Fatalf("timeout: got %v, want session 5", got["timeout"])
	}
	if got["enabled"] != true {
		t.Fatalf("enabled: got %v, want default true", got["enabled"])
	}
	// ClearSession drops back to plugin value.
	m.ClearSession()
	if got := m.Merged(); got["timeout"] != float64(60) {
		t.Fatalf("after clear: got %v, want plugin 60", got["timeout"])
	}
}

func TestInvalidRejected(t *testing.T) {
	m, err := New(testDefaults(), testSchema())
	if err != nil {
		t.Fatal(err)
	}
	// Wrong type.
	if err := m.SetUser(json.RawMessage(`{"timeout":"fast"}`)); err == nil {
		t.Fatal("wrong-type user config allowed")
	}
	// Undeclared key.
	if err := m.SetPlugin("p", json.RawMessage(`{"nope":1}`)); err == nil {
		t.Fatal("undeclared plugin key allowed")
	}
	// Non-object layer.
	if err := m.SetSession(json.RawMessage(`[1,2]`)); err == nil {
		t.Fatal("non-object session allowed")
	}
	// Bad defaults fail at construction.
	if _, err := New(json.RawMessage(`{"timeout":"x"}`), testSchema()); err == nil {
		t.Fatal("bad defaults allowed")
	}
	// Startup validation catches a smuggled bad layer.
	m2, _ := New(testDefaults(), testSchema())
	m2.user = json.RawMessage(`{"timeout":"fast"}`)
	if err := m2.ValidateAtStartup(); err == nil {
		t.Fatal("startup validation missed bad user layer")
	}
}

func TestConfigPaths(t *testing.T) {
	// ConfigPath picks the layout by the running OS, so each assertion is
	// made under the OS it describes. The previous version asserted the
	// Windows layout unconditionally, which failed on every non-Windows
	// machine and is why this test was excluded from CI.
	if runtime.GOOS == "darwin" {
		mac := ConfigPath("/Users/u", "/Users/u/Library/Application Support")
		if mac != "/Users/u/Library/Application Support/CrossOS/config.json" {
			t.Fatalf("darwin path = %q, want Application Support/CrossOS/config.json", mac)
		}
	} else {
		win := ConfigPath(`C:\Users\u`, `ignored`)
		if !strings.Contains(win, ".crossos") || !strings.HasSuffix(win, "config.json") {
			t.Fatalf("windows-ish path wrong: %s", win)
		}
	}
	if d := DefaultConfigPath(); !strings.HasSuffix(d, "config.json") {
		t.Fatalf("default path = %q, want a config.json suffix", d)
	}
}

func TestConcurrentSetMerged(t *testing.T) {
	// Session overrides are runtime state: Set-while-read must not panic
	// (concurrent map access is fatal in Go). (review fix: cross-os-c0)
	m, err := New(testDefaults(), testSchema())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan bool)
	go func() {
		for i := 0; i < 200; i++ {
			_ = m.Merged()
			_ = m.ValidateAtStartup()
		}
		done <- true
	}()
	for i := 0; i < 200; i++ {
		_ = m.SetSession(json.RawMessage(`{"timeout":5}`))
		m.ClearSession()
	}
	<-done
}

func TestNestedMerge(t *testing.T) {
	m, err := New(json.RawMessage(`{"timeout":30,"advanced":{"a":1,"b":2}}`), testSchema())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetUser(json.RawMessage(`{"advanced":{"b":3}}`)); err != nil {
		t.Fatal(err)
	}
	got := m.Merged()
	adv, ok := got["advanced"].(map[string]any)
	if !ok || adv["a"] != float64(1) || adv["b"] != float64(3) {
		t.Fatalf("nested merge wrong: %+v", got["advanced"])
	}
}
