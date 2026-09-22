package launcher

import (
	"runtime"
	"testing"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/pluginapi"
)

// TestHotkeyOpens: Ctrl+Space resolves through the standard rule path
// (CONSUME — palette opens, conflict-visible like any rule). Uses
// runtime.GOOS so the test proves whatever platform it runs on (review:
// cross-os-ed — a hardcoded macOS keycode used to silently listen on the
// wrong key on Windows; TestSpaceVKPerPlatform below pins both branches).
func TestHotkeyOpens(t *testing.T) {
	reg := intent.DefaultRegistry()
	rt := event.Compile([]event.CompiledRule{HotkeyRule()}, reg,
		map[string][]intent.Permission{pluginID: Grants()}, nil)
	out := rt.Decide(
		event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: spaceVK(runtime.GOOS), Modifiers: modCtrl},
		event.FastContext{AppID: "Finder", AppMode: event.AppModeNative})
	if out.Decision != pluginapi.DecisionConsume {
		t.Fatalf("hotkey decision=%v, want CONSUME (palette opens)", out.Decision)
	}
	if out.WinnerRule != pluginID+".ctrl-space-launcher" {
		t.Fatalf("winner=%q", out.WinnerRule)
	}
	// launcher.open is a registered shell-owned capability: validating the
	// hotkey intent must pass (no deferred crash when palette wiring calls
	// Validate — review: cross-os-8d).
	if err := HotkeyRule().Intent.Validate(reg); err != nil {
		t.Fatalf("hotkey intent invalid: %v", err)
	}
}

// TestSpaceVKPerPlatform pins bead cross-os-jpr.7: Windows gets VK_SPACE
// (0x20), every other GOOS gets kVK_Space (0x31) — no silent wrong-key
// registration on either OS.
func TestSpaceVKPerPlatform(t *testing.T) {
	if got := spaceVK("windows"); got != 0x20 {
		t.Fatalf("windows vk=%#x, want 0x20 (VK_SPACE)", got)
	}
	for _, goos := range []string{"darwin", "linux", "unknown"} {
		if got := spaceVK(goos); got != 0x31 {
			t.Fatalf("%s vk=%#x, want 0x31 (kVK_Space)", goos, got)
		}
	}
}

// TestSearchRanking: exact > prefix > substring > fuzzy > none.
func TestSearchRanking(t *testing.T) {
	apps := []App{
		{Name: "Finder Contrast", BundleID: "com.other.contrast"},
		{Name: "Finder", BundleID: "com.apple.Finder"},
		{Name: "Refinder Pro", BundleID: "com.other.refinder"},
		{Name: "Terminal", BundleID: "com.apple.Terminal"},
	}
	// Fuzzy probe (subsequence, not substring): "fdr" fits f-in-d-e-r order
	// inside "finder"/"refinder" but matches nothing else.
	fuzzyApps := []App{{Name: "Finder", BundleID: "com.apple.Finder"}, {Name: "Terminal", BundleID: "com.apple.Terminal"}}
	if got := Search("fdr", fuzzyApps); len(got) != 1 || got[0].Name != "Finder" {
		t.Fatalf("fuzzy fdr=%v, want [Finder]", got)
	}
	got := Search("Finder", apps)
	// Terminal excluded (no match); "Fd" excluded too — fuzzy is query-chars-
	// in-order within the candidate, and "finder" cannot fit inside "fd".
	if len(got) != 3 {
		t.Fatalf("results=%d, want 3 (Terminal + Fd excluded)", len(got))
	}
	if got[0].Name != "Finder" {
		t.Fatalf("first=%q, want exact match", got[0].Name)
	}
	if got[1].Name != "Finder Contrast" {
		t.Fatalf("second=%q, want prefix", got[1].Name)
	}
	if Rank("xyz", apps[0]) != 0 {
		t.Fatal("non-match must score 0")
	}
	// Bundle ID matching.
	if Rank("com.apple.finder", apps[1]) != 4 {
		t.Fatal("bundle-id exact must score 4")
	}
	// Multi-byte names rank rune-wise ("café" ⊂ "Café Note", not byte-garbled).
	cafe := []App{{Name: "Café Note", BundleID: "com.other.cafe"}, {Name: "Terminal", BundleID: "com.apple.Terminal"}}
	if got := Search("café", cafe); len(got) != 1 || got[0].Name != "Café Note" {
		t.Fatalf("unicode café=%v, want [Café Note]", got)
	}
}

// TestLaunchViaCapability: selection builds an app.launch request with the
// bundle ID (Adapter executes — no direct launch from the plugin).
func TestLaunchViaCapability(t *testing.T) {
	a := App{Name: "Finder", BundleID: "com.apple.Finder"}
	in := LaunchIntent(a)
	if in.ID != "app.launch" {
		t.Fatalf("intent=%q, want app.launch", in.ID)
	}
	reg := intent.DefaultRegistry()
	if err := in.Validate(reg); err != nil {
		t.Fatalf("launch intent invalid: %v", err)
	}
}

// TestGrantsMatchManifest mirrors the sibling plugins' invariant.
func TestGrantsMatchManifest(t *testing.T) {
	m := Manifest()
	grantSet := map[string]bool{}
	for _, g := range Grants() {
		grantSet[string(g)] = true
	}
	for _, p := range m.Permissions {
		if !grantSet[p] {
			t.Fatalf("manifest declares %q but Grants() omits it", p)
		}
	}
}
