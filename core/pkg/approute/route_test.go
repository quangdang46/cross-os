package approute

import (
	"testing"

	"crossos/core/pkg/ctx"
)

func TestConfigOverridesSeed(t *testing.T) {
	// Seed knows ghostty→terminal; config can move it anywhere with no
	// code change (bead criterion: config-only behavior change).
	r := New(Lists{Excluded: []string{"ghostty"}})
	mode, _, cause := r.Route("com.mitchellh.ghostty", "ghostty")
	if mode != ctx.AppModeExcluded {
		t.Fatalf("want excluded via config, got %q (%s)", mode, cause)
	}
	// Empty config → seed decides.
	r2 := New(Lists{})
	mode, cat, cause := r2.Route("com.mitchellh.ghostty", "ghostty")
	if mode != ctx.AppModeTerminal || cat != ctx.AppTerminal || cause != "seed" {
		t.Fatalf("want seed terminal, got %q/%q (%s)", mode, cat, cause)
	}
}

func TestCategoryLists(t *testing.T) {
	r := New(Lists{
		Terminals: []string{"myterm"},
		Remotes:   []string{"myrdp"},
		VMs:       []string{"myvm"},
		Browsers:  []string{"mybrowser"},
	})
	cases := []struct {
		exe  string
		mode ctx.AppMode
	}{
		{"myterm", ctx.AppModeTerminal},
		{"myrdp-client", ctx.AppModeRemote},
		{"myvm-player", ctx.AppModeVM},
		{"mybrowser", ctx.AppModeNative},
		{"unknown-thing", ctx.AppModeNative},
	}
	for _, tc := range cases {
		mode, _, _ := r.Route("", tc.exe)
		if mode != tc.mode {
			t.Fatalf("%q: got %q, want %q", tc.exe, mode, tc.mode)
		}
	}
}

func TestPrecedenceExcludedFirst(t *testing.T) {
	// An app in two lists resolves excluded > vm > remote > terminal.
	r := New(Lists{
		Excluded:  []string{"both"},
		VMs:       []string{"both"},
		Remotes:   []string{"both"},
		Terminals: []string{"both"},
	})
	mode, _, cause := r.Route("", "both")
	if mode != ctx.AppModeExcluded {
		t.Fatalf("want excluded-first, got %q (%s)", mode, cause)
	}
}

func TestDeviceFiltering(t *testing.T) {
	// Two keyboards: excluded ID ignored while the other still triggers
	// (config-only change, no code change).
	r := New(Lists{Devices: map[string]bool{"kbd-bad": true}})
	if !r.DeviceExcluded("kbd-bad") {
		t.Fatal("excluded device not filtered")
	}
	if r.DeviceExcluded("kbd-good") {
		t.Fatal("good device wrongly filtered")
	}
}

func TestPassthroughModes(t *testing.T) {
	for _, m := range []ctx.AppMode{ctx.AppModeRemote, ctx.AppModeVM, ctx.AppModeExcluded} {
		if !Passthrough(m) {
			t.Fatalf("%q should passthrough", m)
		}
	}
	for _, m := range []ctx.AppMode{ctx.AppModeNative, ctx.AppModeTerminal} {
		if Passthrough(m) {
			t.Fatalf("%q should not passthrough", m)
		}
	}
}

func TestCauseAnnotation(t *testing.T) {
	// Every decision carries its cause for Recorder trace annotation.
	r := New(Lists{Terminals: []string{"t"}})
	_, _, cause := r.Route("", "t-app")
	if cause == "" {
		t.Fatal("missing cause")
	}
	_, _, cause = New(Lists{}).Route("", "nope-unknown")
	if cause != "seed" {
		t.Fatalf("want seed cause, got %q", cause)
	}
}
