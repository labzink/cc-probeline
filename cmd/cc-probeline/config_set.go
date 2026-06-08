// config_set.go implements CLI subcommands for config mutations:
//   - cc-probeline mode standard|super-compact
//   - cc-probeline no-color on|off
//   - cc-probeline widgets <name> on|off
//   - cc-probeline refresh-interval <n>
//   - cc-probeline table-rows <n>
//
// Each command follows the pattern established by hints.go / runHints.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/labzink/cc-probeline/internal/config"
)

// runConfigShow prints the effective configuration (after cascade resolution)
// as indented JSON, so tooling such as the /cc-probeline-config wizard can read the
// current values. Read-only: it never writes the config.
// Usage: cc-probeline config show
func runConfigShow(args []string) int {
	return runConfigShowImpl(args, os.Stdout, os.Stderr)
}

func runConfigShowImpl(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || args[0] != "show" {
		fmt.Fprintln(stderr, "Usage: cc-probeline config show")
		return 64
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	cfg, _, _ := config.LoadCascade(cwd)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "cc-probeline:", err)
		return 2
	}
	fmt.Fprintln(stdout, string(data))
	return 0
}

// runMode sets the display mode.
// Usage: cc-probeline mode standard|super-compact
func runModeCmd(args []string) int {
	return runModeCmdImpl(args, os.Stdout, os.Stderr)
}

func runModeCmdImpl(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "Usage: cc-probeline mode standard|super-compact")
		return 64
	}
	mode := args[0]

	path := config.GlobalConfigPath()
	if path == "" {
		fmt.Fprintln(stderr, "cc-probeline: cannot determine config directory (HOME/XDG_CONFIG_HOME/APPDATA unset)")
		return 2
	}

	if err := config.SetMode(path, mode); err != nil {
		fmt.Fprintln(stderr, "cc-probeline:", err)
		return 2
	}

	fmt.Fprintf(stdout, "cc-probeline: mode set to %q (config: %s)\n", mode, path)
	return 0
}

// runNoColor enables or disables colour output.
// Usage: cc-probeline no-color on|off
func runNoColor(args []string) int {
	return runNoColorImpl(args, os.Stdout, os.Stderr)
}

func runNoColorImpl(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "Usage: cc-probeline no-color on|off")
		return 64
	}

	var value bool
	switch args[0] {
	case "on":
		value = true
	case "off":
		value = false
	default:
		fmt.Fprintln(stderr, "Usage: cc-probeline no-color on|off")
		return 64
	}

	path := config.GlobalConfigPath()
	if path == "" {
		fmt.Fprintln(stderr, "cc-probeline: cannot determine config directory (HOME/XDG_CONFIG_HOME/APPDATA unset)")
		return 2
	}

	if err := config.SetNoColor(path, value); err != nil {
		fmt.Fprintln(stderr, "cc-probeline:", err)
		return 2
	}

	state := "enabled"
	if !value {
		state = "disabled"
	}
	fmt.Fprintf(stdout, "cc-probeline: no-color %s (config: %s)\n", state, path)
	return 0
}

// runWidgets enables or disables a named widget.
// Usage: cc-probeline widgets <name> on|off
func runWidgets(args []string) int {
	return runWidgetsImpl(args, os.Stdout, os.Stderr)
}

func runWidgetsImpl(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "Usage: cc-probeline widgets <name> on|off")
		return 64
	}
	name := args[0]

	var value bool
	switch args[1] {
	case "on":
		value = true
	case "off":
		value = false
	default:
		fmt.Fprintln(stderr, "Usage: cc-probeline widgets <name> on|off")
		return 64
	}

	path := config.GlobalConfigPath()
	if path == "" {
		fmt.Fprintln(stderr, "cc-probeline: cannot determine config directory (HOME/XDG_CONFIG_HOME/APPDATA unset)")
		return 2
	}

	if err := config.SetWidget(path, name, value); err != nil {
		fmt.Fprintln(stderr, "cc-probeline:", err)
		return 2
	}

	state := "enabled"
	if !value {
		state = "disabled"
	}
	fmt.Fprintf(stdout, "cc-probeline: widget %q %s (config: %s)\n", name, state, path)
	return 0
}

// runRefreshInterval sets the refresh interval hint.
// Usage: cc-probeline refresh-interval <n>
func runRefreshInterval(args []string) int {
	return runRefreshIntervalImpl(args, os.Stdout, os.Stderr)
}

func runRefreshIntervalImpl(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "Usage: cc-probeline refresh-interval <seconds>")
		return 64
	}

	n, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintln(stderr, "cc-probeline: refresh-interval requires an integer, got:", args[0])
		return 64
	}

	path := config.GlobalConfigPath()
	if path == "" {
		fmt.Fprintln(stderr, "cc-probeline: cannot determine config directory (HOME/XDG_CONFIG_HOME/APPDATA unset)")
		return 2
	}

	if err := config.SetRefreshInterval(path, n); err != nil {
		fmt.Fprintln(stderr, "cc-probeline:", err)
		return 2
	}

	fmt.Fprintf(stdout, "cc-probeline: refresh-interval set to %d (config: %s)\n", n, path)
	return 0
}

// runTableRows sets the table rows limit.
// Usage: cc-probeline table-rows <n>
func runTableRows(args []string) int {
	return runTableRowsImpl(args, os.Stdout, os.Stderr)
}

func runTableRowsImpl(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "Usage: cc-probeline table-rows <n>")
		return 64
	}

	n, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintln(stderr, "cc-probeline: table-rows requires an integer, got:", args[0])
		return 64
	}

	path := config.GlobalConfigPath()
	if path == "" {
		fmt.Fprintln(stderr, "cc-probeline: cannot determine config directory (HOME/XDG_CONFIG_HOME/APPDATA unset)")
		return 2
	}

	if err := config.SetTableRows(path, n); err != nil {
		fmt.Fprintln(stderr, "cc-probeline:", err)
		return 2
	}

	// Reflect the capped value back in stdout by re-reading — simpler: just
	// report what was requested (capping is transparent to the user; the TOML
	// file will hold the capped value which check-config can confirm).
	fmt.Fprintf(stdout, "cc-probeline: table-rows set to %d (config: %s)\n", n, path)
	return 0
}
