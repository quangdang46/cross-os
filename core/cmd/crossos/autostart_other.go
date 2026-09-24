//go:build !darwin

// Login-item management is macOS-only (bead cross-os-2aj). Elsewhere the
// subcommands exist but report honestly rather than pretending to install
// something.
package main

import "fmt"

func installAutostart() error {
	return fmt.Errorf("autostart is not implemented on this platform yet")
}

func uninstallAutostart() error {
	return fmt.Errorf("autostart is not implemented on this platform yet")
}

// autostartPath is empty here: installAutostart refuses on this platform, so
// no CrossOS-owned login item can exist and the ownership audit has nothing
// to look for.
func autostartPath() string { return "" }
