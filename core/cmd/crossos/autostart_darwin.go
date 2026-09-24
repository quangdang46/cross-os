//go:build darwin

// Login-item (launchd) management (bead cross-os-2aj).
//
// The daemon plist used to ship only as a repository file, so RunAtLoad
// never fired on a real install: after a reboot the shell opened onto a
// socket nobody was serving. `crossos install-autostart` writes the agent
// into ~/Library/LaunchAgents with the binary path resolved, so there is no
// placeholder for the user to edit by hand.
//
// The plist is embedded from dev.crossos.daemon.plist in this directory —
// that file is the single source. A copy of the template in Go would drift
// from it, which is the failure this bead exists to remove.
//
// Installing is opt-in and never happens implicitly (§8.4): the daemon
// only installs the agent when asked, and run.sh only points at the command.

package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed dev.crossos.daemon.plist
var autostartTemplate string

const autostartLabel = "dev.crossos.daemon"

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

func autostartPlist() string {
	return filepath.Join(homeDir(), "Library", "LaunchAgents", autostartLabel+".plist")
}

func autostartStateDir() string {
	return filepath.Join(homeDir(), "Library", "Application Support", "CrossOS")
}

// renderAutostartPlist substitutes the two placeholders in the embedded
// template. Pure, so the patch logic is testable without launchd.
func renderAutostartPlist(bin, state string) string {
	out := strings.ReplaceAll(autostartTemplate, "__CROSSOS_BIN__", bin)
	return strings.ReplaceAll(out, "__CROSSOS_STATE__", state)
}

// launchctl wrappers take the gui domain so the agent is a per-user login
// item rather than a system daemon.
func launchctlDomain() string { return "gui/" + fmt.Sprint(os.Getuid()) }

func agentLoaded() bool {
	cmd := exec.Command("launchctl", "print",
		launchctlDomain()+"/"+autostartLabel)
	return cmd.Run() == nil
}

// bootstrapAgent loads the agent, retrying once: an immediate
// bootout-then-bootstrap can fail while launchd is still releasing the
// previous job, and without the retry a second install would leave the
// user with no running login item at all.
func bootstrapAgent(plist string) error {
	domain := launchctlDomain()
	_ = exec.Command("launchctl", "bootout", domain+"/"+autostartLabel).Run()
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		if err := exec.Command("launchctl", "bootstrap", domain, plist).Run(); err == nil {
			_ = exec.Command("launchctl", "enable", domain+"/"+autostartLabel).Run()
			return nil
		} else {
			lastErr = err
		}
	}
	return lastErr
}

// installAutostart copies the running binary somewhere stable, writes the
// agent, and loads it. Safe to run twice.
func installAutostart() error {
	if homeDir() == "" {
		return fmt.Errorf("cannot resolve the home directory")
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate this binary: %w", err)
	}
	if resolved, rerr := filepath.EvalSymlinks(self); rerr == nil {
		self = resolved
	}

	state := autostartStateDir()
	if err := os.MkdirAll(state, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(autostartPlist()), 0o755); err != nil {
		return err
	}

	// Stable copy: a build under .crossos/ or /tmp would be a stale path by
	// the next login, and launchd would fail to exec it silently.
	installed := filepath.Join(state, "crossos")
	data, err := os.ReadFile(self)
	if err != nil {
		return fmt.Errorf("read the running binary: %w", err)
	}
	if err := os.WriteFile(installed, data, 0o755); err != nil {
		return fmt.Errorf("install the daemon to %s: %w", installed, err)
	}

	if err := os.WriteFile(autostartPlist(),
		[]byte(renderAutostartPlist(installed, state)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", autostartPlist(), err)
	}

	if err := bootstrapAgent(autostartPlist()); err != nil {
		// The plist is what a reboot reads, so the install still holds even
		// when this session cannot talk to launchd.
		fmt.Printf("crossos: wrote %s\n", autostartPlist())
		fmt.Printf("crossos: could not load it now (%v) — it starts on next login\n", err)
		return nil
	}
	fmt.Printf("crossos: login item installed and running\n")
	fmt.Printf("crossos:   daemon: %s\n", installed)
	fmt.Printf("crossos:   agent:  %s\n", autostartPlist())
	fmt.Printf("crossos: remove with: crossos uninstall-autostart\n")
	return nil
}

// uninstallAutostart stops and removes the agent. Already-absent is success:
// the postcondition is "not installed", not "something was there".
func uninstallAutostart() error {
	_ = exec.Command("launchctl", "bootout",
		launchctlDomain()+"/"+autostartLabel).Run()
	if err := os.Remove(autostartPlist()); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Println("crossos: login item removed")
	return nil
}

// autostartPath is the login item the ownership audit looks for, or "" where
// this platform has no login item to own. The audit stats the file itself and
// dates the record from its mtime: the serving daemon did not write this plist
// (a separate `crossos install-autostart` run did, possibly a login ago), so
// "now" would be a creation time that never happened.
func autostartPath() string { return autostartPlist() }
