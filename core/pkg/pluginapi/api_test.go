package pluginapi

import (
	"encoding/json"
	"testing"
)

// Canonical core registry subset (§3.12) used across tests.
var testCoreIDs = []string{
	"input.observe", "input.intercept",
	"window.read", "window.move", "window.close", "window.minimize", "window.maximize",
	"clipboard.read", "clipboard.write", "clipboard.copyPath",
	"filesystem.read", "filesystem.write", "filesystem.createFile", "filesystem.createFolder", "file.moveToTrash",
	"app.launch", "app.open", "terminal.openAt",
	"finder.menu",
}

func TestRegisterCapabilityRejectsCoreShadow(t *testing.T) {
	r := NewRegistry(testCoreIDs)
	err := r.RegisterCapability(Capability{
		Name:        "window.close",
		Owner:       CapabilityOwnerPlugin,
		Description: "malicious shadow attempt",
	})
	if err == nil {
		t.Fatal("expected rejection of plugin claiming core ID window.close")
	}
}

func TestRegisterCapabilityRejectsCoreOwner(t *testing.T) {
	r := NewRegistry(testCoreIDs)
	err := r.RegisterCapability(Capability{
		Name:        "window.close",
		Owner:       CapabilityOwnerCore,
		Description: "core owner from plugin",
	})
	if err == nil {
		t.Fatal("expected rejection of CapabilityOwnerCore from plugin registration")
	}
}

func TestRegisterCapabilityAcceptsNamespaced(t *testing.T) {
	r := NewRegistry(testCoreIDs)
	err := r.RegisterCapability(Capability{
		Name:        "myplugin.doSomething",
		Owner:       CapabilityOwnerPlugin,
		Description: "legit plugin capability",
	})
	if err != nil {
		t.Fatalf("unexpected rejection: %v", err)
	}
	if len(r.Capabilities) != 1 {
		t.Fatalf("want 1 capability, got %d", len(r.Capabilities))
	}
}

func TestRegisterUIRejectsCustomView(t *testing.T) {
	r := NewRegistry(testCoreIDs)
	err := r.RegisterUI(UIContribution{
		ID:       "myplugin.dashboard",
		Location: UILocationCustomView,
		Title:    "Dashboard",
	})
	if err == nil {
		t.Fatal("expected rejection of custom-view in MVP registry")
	}
}

func TestRegisterUIAcceptsDeclarative(t *testing.T) {
	r := NewRegistry(testCoreIDs)
	for _, loc := range []UILocation{
		UILocationSettingsPage, UILocationSettingsSection, UILocationSidebar,
		UILocationContextMenu, UILocationCommandPalette, UILocationStatus,
	} {
		err := r.RegisterUI(UIContribution{
			ID:       "myplugin.c1",
			Location: loc,
			Title:    "t",
		})
		if err != nil {
			t.Fatalf("location %s: unexpected rejection: %v", loc, err)
		}
	}
	if len(r.UIContributions) != 6 {
		t.Fatalf("want 6 contributions, got %d", len(r.UIContributions))
	}
}

func TestDecisionEnumOrder(t *testing.T) {
	// Pinned: Pass=0, Consume=1, Replace=2. The resolver and fast path both
	// depend on this vocabulary; changing it breaks the contract.
	if DecisionPass != 0 || DecisionConsume != 1 || DecisionReplace != 2 {
		t.Fatalf("decision enum changed: %d %d %d",
			DecisionPass, DecisionConsume, DecisionReplace)
	}
}

func TestManifestJSONRoundTrip(t *testing.T) {
	m := Manifest{
		ID:      "windows-keyboard",
		Name:    "Windows Keyboard",
		Version: "1.0.0",
		Entry:   "builtin",
		Type:    PluginBuiltin,
		APIVersion: APIVersion{
			ManifestVersion:      "1",
			PluginAPIVersion:     "1",
			CapabilityAPIVersion: "1",
			MinCoreVersion:       "0.1.0",
		},
		Safety: PluginSafety{SafeModeDurationSec: 30, AutoRollback: true},
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Manifest
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.APIVersion.MinCoreVersion != "0.1.0" {
		t.Fatalf("api version lost: %+v", back.APIVersion)
	}
	if back.Safety.SafeModeDurationSec != 30 {
		t.Fatalf("safety lost: %+v", back.Safety)
	}
}
