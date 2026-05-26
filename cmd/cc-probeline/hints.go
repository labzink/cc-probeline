package main

import (
	"fmt"
	"os"

	"github.com/labzink/cc-probeline/internal/config"
)

// runHints implements `cc-probeline hints on|off`.
//
// Exit codes:
//
//	0  — success
//	64 — usage error (missing arg, unknown arg)
//	2  — config write failed (IO, broken existing TOML)
func runHints(args []string) int {
	// args[0] = binary path, args[1] = "hints", args[2] = "on" or "off".
	// We need at least args[2] present; fewer means no on/off provided.
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: cc-probeline hints on|off")
		return 64
	}

	var value bool
	switch args[2] {
	case "on":
		value = true
	case "off":
		value = false
	default:
		fmt.Fprintln(os.Stderr, "Usage: cc-probeline hints on|off")
		return 64
	}

	path := config.GlobalConfigPath()
	if path == "" {
		fmt.Fprintln(os.Stderr, "cc-probeline: cannot determine config directory (HOME/XDG_CONFIG_HOME/APPDATA unset)")
		return 2
	}

	if err := config.SetTutorialHints(path, value); err != nil {
		fmt.Fprintln(os.Stderr, "cc-probeline:", err)
		return 2
	}

	state := "enabled"
	if !value {
		state = "disabled"
	}
	fmt.Printf("cc-probeline: tutorial hints %s (config: %s)\n", state, path)
	return 0
}
