//go:build windows

// DLL loader tests — bead cross-os-qhp.8.
//
// loadDLL resolves crossos-keyboard-win.dll (built by platform/windows via
// CMake, MSVC-verified) through the injectable dllLoad seam: the real DLL
// loads when present; a missing DLL stays a typed error on BOTH surfaces
// (caller + stage log), never a silent success.
package adapter

import (
	"strings"
	"syscall"
	"testing"
)

// TestLoadDLLMissingFailsClosed: with no DLL loadable, loadDLL returns the
// typed missing-DLL error AND logs the dll stage (never silent).
func TestLoadDLLMissingFailsClosed(t *testing.T) {
	oldLoad, oldHandle := dllLoad, dllHandle
	defer func() { dllLoad, dllHandle = oldLoad, oldHandle }()
	dllHandle = 0
	dllLoad = func(string) (*syscall.DLL, error) {
		return nil, syscall.ERROR_MOD_NOT_FOUND
	}
	log := &memLog{}
	d := &Driver{Log: log}
	if err := loadDLL(d); err == nil {
		t.Fatal("missing DLL: want typed error, got nil (never silent success)")
	} else if !strings.Contains(err.Error(), "crossos-keyboard-win.dll") {
		t.Fatalf("missing DLL error=%q, want the DLL named", err)
	}
	found := false
	for _, l := range log.lines {
		if strings.HasPrefix(l, "dll:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("stage log missing dll error line, got %v", log.lines)
	}
}

// TestLoadDLLAlreadyLoaded: a loaded handle short-circuits without calling
// the loader again (idempotent across repeated Install paths).
func TestLoadDLLAlreadyLoaded(t *testing.T) {
	oldLoad, oldHandle := dllLoad, dllHandle
	defer func() { dllLoad, dllHandle = oldLoad, oldHandle }()
	dllHandle = syscall.Handle(12345)
	called := false
	dllLoad = func(string) (*syscall.DLL, error) {
		called = true
		return nil, syscall.ERROR_MOD_NOT_FOUND
	}
	if err := loadDLL(&Driver{}); err != nil {
		t.Fatalf("loaded handle: want nil, got %v", err)
	}
	if called {
		t.Fatal("loaded handle must short-circuit the loader")
	}
}
