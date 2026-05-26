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
// Phase 5.a: render pipeline wired (runRender, parseMode unknown-flag, setupLogger).
// Phase 5.0 foundation: version/help routing.
// Remaining stubs (5.e): runInstall.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/labzink/cc-probeline/internal/config"
	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/statusline"
	"github.com/labzink/cc-probeline/internal/stdin"
)

type runMode int

const (
	modeRender runMode = iota
	modeVersion
	modeHelp
	modeUninstall
	modeInstall
	modeCheck
	modeCheckConfig // Phase 6.e: cc-probeline check-config
	modeHints       // Phase 6.f: cc-probeline hints on/off
	modeBad         // unknown flag: exit 64
)

func main() {
	os.Exit(run(os.Args))
}

func run(args []string) int {
	m, strict, _ := parseMode(args)
	switch m {
	case modeVersion:
		fmt.Fprintf(os.Stdout, "%s\n", versionString())
		return 0
	case modeHelp:
		printUsage(os.Stdout)
		return 0
	case modeUninstall:
		return runUninstall()
	case modeInstall:
		return runInstall()
	case modeCheck:
		return runCheck()
	case modeCheckConfig:
		return runCheckConfig(args[2:])
	case modeHints:
		return runHints(args)
	case modeBad:
		return 64
	default: // modeRender
		return runRender(strict)
	}
}

// parseMode inspects args to determine the operating mode.
// Unknown flags (start with "-" but not a known flag) print an error to stderr
// and return modeBad (exit 64). Unknown positional args fall through to modeRender.
// Returns the mode, whether --strict-stdin was set, and the bad flag string (if any).
func parseMode(args []string) (mode runMode, strict bool, badFlag string) {
	if len(args) < 2 {
		return modeRender, false, ""
	}
	a := args[1]
	switch a {
	case "--version", "-V":
		return modeVersion, false, ""
	case "--help", "-h":
		return modeHelp, false, ""
	case "uninstall":
		return modeUninstall, false, ""
	case "install":
		return modeInstall, false, ""
	case "--check":
		return modeCheck, false, ""
	case "check-config":
		return modeCheckConfig, false, ""
	case "hints":
		return modeHints, false, ""
	case "--strict-stdin":
		return modeRender, true, ""
	}
	if strings.HasPrefix(a, "-") {
		fmt.Fprintln(os.Stderr, "unknown flag: "+a)
		return modeBad, false, a
	}
	// Unrecognised positional arg: fall through to render mode (CC invocation pattern).
	return modeRender, false, ""
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: cc-probeline [--version|--help|install|uninstall|--check]")
	fmt.Fprintln(w, "  (no args)        render status line from stdin payload")
	fmt.Fprintln(w, "  install          write statusLine block into ~/.claude/settings.json")
	fmt.Fprintln(w, "  uninstall        remove statusLine block from ~/.claude/settings.json")
	fmt.Fprintln(w, "  --check          dry-run validation of the current installation")
	fmt.Fprintln(w, "  --strict-stdin   reject unknown JSON fields in stdin payload")
	fmt.Fprintln(w, "  --version        print version and exit")
	fmt.Fprintln(w, "  --help           print this help and exit")
}

// setupLogger configures the default slog handler.
// When logPath is non-empty, log output is written to that file (append mode).
// When debug is true, the log level is set to Debug; otherwise Warn.
// When logPath is empty or cannot be opened, output is discarded (io.Discard).
func setupLogger(logPath string, debug bool) {
	w := io.Discard
	if logPath != "" {
		if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			w = f
		}
	}
	level := slog.LevelWarn
	if debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})))
}

// runRender implements the default render mode: reads a JSON payload from
// stdin, runs it through the parser/assembler pipeline, and prints the
// resulting status line to stdout.
//
// strict enables rejection of unknown JSON fields in stdin payload.
//
// Exit codes:
//   - 0: success
//   - 1: stdin decode error (error sentinel printed to stdout)
func runRender(strict bool) int {
	setupLogger(os.Getenv("CC_PROBELINE_LOG"), os.Getenv("CC_PROBELINE_DEBUG") == "1")

	strict = os.Getenv("CC_PROBELINE_STRICT_STDIN") == "1" || strict
	payload, err := stdin.Decode(os.Stdin, strict)
	if err != nil {
		fmt.Fprintln(os.Stdout, "cc-probeline · stdin parse error")
		slog.Error("stdin decode", "err", err)
		return 1
	}

	now := time.Now()

	// Load JSONL transcript: fail-soft on any I/O error.
	var records []parser.Record
	if payload.TranscriptPath != "" {
		if f, openErr := os.Open(payload.TranscriptPath); openErr == nil {
			recs, _, _ := parser.ParseLines(f)
			f.Close()
			records = recs
		}
	}
	session := parser.Aggregate(records)

	// Collect subagent stats: fail-soft when sessionDir cannot be determined.
	var subagents []parser.SubagentStats
	if payload.SessionID != "" && payload.Cwd != "" {
		if slug, slugErr := parser.ProjectSlug(payload.Cwd); slugErr == nil {
			home, _ := os.UserHomeDir()
			base := home + "/.claude"
			if cd := os.Getenv("CLAUDE_CONFIG_DIR"); cd != "" {
				base = cd
			}
			sessionDir := base + "/projects/" + slug + "/" + payload.SessionID
			subs, _ := parser.CollectSubagents(context.Background(), sessionDir)
			subagents = subs
		}
	}

	// Load config cascade (Phase 6). Always lenient: errors become alerts, not crashes.
	ccfg, source, configErrs := config.LoadCascade(payload.Cwd)
	// Apply NO_COLOR from config only when env NO_COLOR is not already set.
	// ENV NO_COLOR > config.no_color > auto-detect (concept §7.3).
	baseTheme := renderer.Theme{AnsiEnabled: renderer.DetectAnsi(os.Stdout)}
	if ccfg.General.NoColor && os.Getenv("NO_COLOR") == "" {
		baseTheme.AnsiEnabled = false
	}
	// Sanitise invalid numeric ranges; ignore changed-field list (lenient render).
	_ = config.ApplyRangeFix(ccfg)
	pcfg := config.ToProbesConfig(*ccfg)
	theme := config.ToTheme(*ccfg, baseTheme)
	configAlerts := config.ToCacheEvents(configErrs)
	slog.Debug("config loaded", "source", source, "errors", len(configErrs))

	modeVal := mode.Load()

	d := probes.Data{
		Stdin:            payload,
		Session:          &session,
		Subagents:        subagents,
		Now:              now,
		SessionID:        payload.SessionID,
		ExtraCacheEvents: configAlerts,
	}

	cols := renderer.DetectCols()
	a := statusline.Assembler{Mode: modeVal, Theme: theme, Cols: cols, Config: pcfg}
	raw := a.Render(d)
	final := renderer.Apply(raw, theme)
	fmt.Fprintln(os.Stdout, final)
	return 0
}

// Stubs filled in by later subtasks.
func runUninstall() int { return runUninstallImpl() } // delegated to uninstall.go (5.b)
func runInstall() int   { return runInstallImpl() }   // delegated to install.go (5.e)
func runCheck() int     { return 0 }                  // TODO(5.a/check)
