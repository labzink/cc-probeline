// Package main — check subcommand.
//
// runCheckImpl performs a read-only dry-run validation of the cc-probeline
// installation by inspecting ~/.claude/settings.json (or an injected path
// for tests).
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/labzink/cc-probeline/internal/settingsfile"
)

// runCheck is the entry point called from main when --check is passed.
// It delegates to runCheckImpl so the logic is unit-testable without touching
// the real ~/.claude directory.
func runCheck() int {
	return runCheckImpl(settingsfile.Path(), os.Stdout, os.Stderr)
}

// runCheckImpl validates the cc-probeline installation.
//
// Checks performed (in order):
//  1. settingsPath exists and can be read.
//  2. statusLine.command is present in the file.
//  3. The command path points to an existing executable file.
//  4. Running the binary with --version exits 0.
//
// Returns 0 and prints "installation OK" when all checks pass.
// Returns 1 and prints a diagnostic line for the first failure found.
// The function is read-only: it never writes or modifies any file.
func runCheckImpl(settingsPath string, stdout, stderr io.Writer) int {
	// Check 1: settings file exists and is readable.
	s, err := settingsfile.Load(settingsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(stderr, "cc-probeline --check: settings not found: %s\n", settingsPath)
			return 1
		}
		fmt.Fprintf(stderr, "cc-probeline --check: cannot read settings: %v\n", err)
		return 1
	}

	// Check 2: statusLine block with a command field exists.
	sl, ok := s["statusLine"].(map[string]any)
	if !ok {
		fmt.Fprintln(stderr, "cc-probeline --check: statusLine not configured in settings.json")
		return 1
	}
	cmdPath, _ := sl["command"].(string)
	if cmdPath == "" {
		fmt.Fprintln(stderr, "cc-probeline --check: statusLine not configured in settings.json")
		return 1
	}

	// Check 3: the binary path exists and is a regular (executable) file.
	info, err := os.Stat(cmdPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(stderr, "cc-probeline --check: binary not found: %s\n", cmdPath)
			return 1
		}
		fmt.Fprintf(stderr, "cc-probeline --check: cannot stat binary: %v\n", err)
		return 1
	}
	if !info.Mode().IsRegular() {
		fmt.Fprintf(stderr, "cc-probeline --check: binary not found: %s\n", cmdPath)
		return 1
	}

	// Check 4: the binary responds to --version with exit 0.
	if err := exec.Command(cmdPath, "--version").Run(); err != nil { //nolint:gosec
		fmt.Fprintf(stderr, "cc-probeline --check: binary --version failed: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "installation OK")
	return 0
}
