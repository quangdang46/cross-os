package intent

import (
	"encoding/json"
	"testing"
)

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	return DefaultRegistry()
}

func TestCanonicalV1Count(t *testing.T) {
	r := testRegistry(t)
	// §3.12 lists 19 capabilities + clipboard.copy (added for the s4i
	// Ctrl+C→COPY mapping; review: cross-os-c0 — copyPath is file-paths
	// copy, not selection copy).
	if got := len(r.IDs()); got != 20 {
		t.Fatalf("canonical v1: got %d IDs, want 20: %v", got, r.IDs())
	}
}

func TestCanonicalV1IDs(t *testing.T) {
	r := testRegistry(t)
	for _, id := range []string{
		"input.observe", "input.intercept",
		"window.read", "window.move", "window.close", "window.minimize", "window.maximize",
		"clipboard.read", "clipboard.write", "clipboard.copy", "clipboard.copyPath",
		"filesystem.read", "filesystem.write", "filesystem.createFile",
		"filesystem.createFolder", "file.moveToTrash",
		"app.launch", "app.open", "terminal.openAt",
		"finder.menu",
	} {
		if _, ok := r.Get(id); !ok {
			t.Fatalf("canonical v1 missing %q", id)
		}
	}
}

func TestBannedNamesRejected(t *testing.T) {
	r := testRegistry(t)
	for id, hint := range invalidNames {
		if err := ValidateCapabilityID(r, id); err == nil {
			t.Fatalf("banned name %q accepted (hint: %s)", id, hint)
		}
	}
	// Banned IDs can never enter a registry either.
	_, err := NewRegistry([]CapabilityDescriptor{{ID: "window.manage"}})
	if err == nil {
		t.Fatal("registry accepted banned ID window.manage")
	}
}

func TestUnknownCapabilityRejected(t *testing.T) {
	r := testRegistry(t)
	if err := ValidateCapabilityID(r, "window.teleport"); err == nil {
		t.Fatal("unknown capability accepted")
	}
}

func TestIntentValidation(t *testing.T) {
	r := testRegistry(t)
	good := Intent{ID: "window.close", Version: 1, Source: SourceKeyboard}
	if err := good.Validate(r); err != nil {
		t.Fatalf("valid intent rejected: %v", err)
	}
	for _, bad := range []Intent{
		{ID: "window.teleport", Version: 1, Source: SourceKeyboard},
		{ID: "keyboard.intercept", Version: 1, Source: SourceKeyboard},
		{ID: "window.close", Version: 0, Source: SourceKeyboard},
		{ID: "window.close", Version: 1, Source: "telepathy"},
	} {
		if err := bad.Validate(r); err == nil {
			t.Fatalf("invalid intent accepted: %+v", bad)
		}
	}
}

func TestIntentParamsAgainstSchema(t *testing.T) {
	r := testRegistry(t)
	mk := func(id, params string) Intent {
		return Intent{ID: id, Version: 1, Source: SourceKeyboard,
			Parameters: json.RawMessage(params)}
	}
	// window.move requires zone:string.
	if err := mk("window.move", `{"zone":"left-half"}`).ValidateAgainst(r); err != nil {
		t.Fatalf("valid params rejected: %v", err)
	}
	if err := mk("window.move", `{}`).ValidateAgainst(r); err == nil {
		t.Fatal("missing required zone accepted")
	}
	if err := mk("window.move", `{"zone":42}`).ValidateAgainst(r); err == nil {
		t.Fatal("wrong-type zone accepted")
	}
	if err := mk("window.move", `{"zone":"left","extra":1}`).ValidateAgainst(r); err == nil {
		t.Fatal("undeclared param accepted")
	}
	// window.close takes no params — empty is fine.
	if err := mk("window.close", ``).ValidateAgainst(r); err != nil {
		t.Fatalf("paramless capability rejected: %v", err)
	}
	// Non-object params rejected.
	if err := mk("window.move", `[1,2]`).ValidateAgainst(r); err == nil {
		t.Fatal("array params accepted for object schema")
	}
}

func TestAuthorizeChecksPermission(t *testing.T) {
	r := testRegistry(t)
	in := Intent{ID: "window.close", Version: 1, Source: SourceHotkey}
	// Granted → authorized, bound to the canonical descriptor.
	req, err := Authorize(r, "myplugin", in, []Permission{PermAccessControl})
	if err != nil {
		t.Fatalf("authorized request rejected: %v", err)
	}
	if req.Capability.ID != "window.close" || req.PluginID != "myplugin" {
		t.Fatalf("bad binding: %+v", req)
	}
	// Missing grant → fail closed.
	if _, err := Authorize(r, "myplugin", in, []Permission{PermFilesystem}); err == nil {
		t.Fatal("unauthorized request allowed")
	}
	// Invalid intent → fail closed even with grants.
	bad := Intent{ID: "window.teleport", Version: 1, Source: SourceHotkey}
	if _, err := Authorize(r, "myplugin", bad, []Permission{PermAccessControl}); err == nil {
		t.Fatal("unknown capability authorized")
	}
}

func TestWindowCloseDescriptor(t *testing.T) {
	// Pins the plan's worked example: window.close needs
	// accessibility.control, runs on both platforms, not reversible.
	r := testRegistry(t)
	d, ok := r.Get("window.close")
	if !ok {
		t.Fatal("window.close missing")
	}
	if d.Permission != PermAccessControl {
		t.Fatalf("permission: got %q", d.Permission)
	}
	if len(d.Platforms) != 2 {
		t.Fatalf("platforms: got %v", d.Platforms)
	}
	if d.Reversible {
		t.Fatal("window.close must not be reversible")
	}
}
