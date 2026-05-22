// Command cc-probeline renders the Claude Code status line.
//
// Modes (selected by parseMode):
//   - render:    default; reads stdin payload, prints one status line.
//   - version:   prints version/commit/build-date and exits.
//   - help:      prints usage to stdout and exits.
//   - install:   writes statusLine block into ~/.claude/settings.json.
//   - uninstall: removes statusLine block from ~/.claude/settings.json.
//   - check:     dry-run validation of the current installation.
//
// Phase 5.0 foundation: only routing + version/help are wired. The other
// subcommands return 0 (stubs) and are implemented in 5.a (render),
// 5.b (uninstall), 5.e (install/--merge-settings).
package main

import (
	"fmt"
	"io"
	"os"
)

type runMode int

const (
	modeRender runMode = iota
	modeVersion
	modeHelp
	modeUninstall
	modeInstall
	modeCheck
)

func main() {
	os.Exit(run(os.Args, os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	switch parseMode(args) {
	case modeVersion:
		fmt.Fprintf(stdout, "cc-probeline %s (commit %s, built %s)\n", version, commit, buildDate)
		return 0
	case modeHelp:
		printUsage(stdout)
		return 0
	case modeUninstall:
		return runUninstall()
	case modeInstall:
		return runInstall()
	case modeCheck:
		return runCheck()
	case modeRender:
		return runRender()
	default:
		fmt.Fprintln(stderr, "cc-probeline: unknown mode")
		return 64
	}
}

func parseMode(args []string) runMode {
	if len(args) < 2 {
		return modeRender
	}
	switch args[1] {
	case "--version", "-V":
		return modeVersion
	case "--help", "-h":
		return modeHelp
	case "uninstall":
		return modeUninstall
	case "install":
		return modeInstall
	case "--check":
		return modeCheck
	default:
		return modeRender
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: cc-probeline [--version|--help|install|uninstall|--check]")
	fmt.Fprintln(w, "  (no args)   render status line from stdin payload")
	fmt.Fprintln(w, "  install     write statusLine block into ~/.claude/settings.json")
	fmt.Fprintln(w, "  uninstall   remove statusLine block from ~/.claude/settings.json")
	fmt.Fprintln(w, "  --check     dry-run validation of the current installation")
	fmt.Fprintln(w, "  --version   print version and exit")
	fmt.Fprintln(w, "  --help      print this help and exit")
}

// Stubs filled in by later subtasks (5.a/5.b/5.e). Returning 0 keeps the
// skeleton harmless when invoked before those phases land.
func runRender() int    { return 0 } // TODO(5.a)
func runUninstall() int { return 0 } // TODO(5.b)
func runInstall() int   { return 0 } // TODO(5.e)
func runCheck() int     { return 0 } // TODO(5.a)
