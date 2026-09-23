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
