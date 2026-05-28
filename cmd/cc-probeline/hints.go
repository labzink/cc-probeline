package main

import (
	"fmt"
	"io"
	"os"

	"github.com/labzink/cc-probeline/internal/config"
)

// runHints implements `cc-probeline hints on|off`.
// Delegates to runHintsImpl for testability, passing os.Stdout/os.Stderr.
//
// Exit codes:
//
//	0  — success
//	64 — usage error (missing arg, unknown arg)
//	2  — config write failed (IO, broken existing TOML)
func runHints(args []string) int {
	return runHintsImpl(args, os.Stdout, os.Stderr)
}

// runHintsImpl is the testable core of runHints. args is the subcommand-args
// slice (i.e. args[0] is "on" or "off", mirroring the convention used by
// runCheckConfigImpl where main.go passes args[2:]).
func runHintsImpl(args []string, stdout, stderr io.Writer) int {
	// args[0] = "on" or "off" (caller strips binary and "hints").
	if len(args) < 1 {
		fmt.Fprintln(stderr, "Usage: cc-probeline hints on|off")
		return 64
	}

	var value bool
	switch args[0] {
	case "on":
		value = true
	case "off":
		value = false
	default:
		fmt.Fprintln(stderr, "Usage: cc-probeline hints on|off")
		return 64
	}

	path := config.GlobalConfigPath()
	if path == "" {
		fmt.Fprintln(stderr, "cc-probeline: cannot determine config directory (HOME/XDG_CONFIG_HOME/APPDATA unset)")
		return 2
	}

	if err := config.SetTutorialHints(path, value); err != nil {
		fmt.Fprintln(stderr, "cc-probeline:", err)
		return 2
	}

	state := "enabled"
	if !value {
		state = "disabled"
	}
	fmt.Fprintf(stdout, "cc-probeline: tutorial hints %s (config: %s)\n", state, path)
	return 0
}
