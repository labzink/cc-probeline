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
// Phase 5.e: runInstall delegated to install.go (fully implemented).
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/labzink/cc-probeline/internal/claudejson"
	"github.com/labzink/cc-probeline/internal/config"
	"github.com/labzink/cc-probeline/internal/cost"
	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/quota"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/state"
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
	modeCheckConfig  // Phase 6.e: cc-probeline check-config
	modeHints        // Phase 6.f: cc-probeline hints on/off
	modeCfgMode      // Phase 6.95.cfg: cc-probeline mode standard|super-compact
	modeCfgNoColor   // Phase 6.95.cfg: cc-probeline no-color on|off
	modeCfgWidgets   // Phase 6.95.cfg: cc-probeline widgets <name> on|off
	modeCfgRefresh   // Phase 6.95.cfg: cc-probeline refresh-interval <n>
	modeCfgTableRows // Phase 6.95.cfg: cc-probeline table-rows <n>
	modeCfgShow      // Phase 6.95.f3: cc-probeline config show
	modeBad          // unknown flag: exit 64
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
		return runHints(args[2:])
	case modeCfgMode:
		return runModeCmd(args[2:])
	case modeCfgNoColor:
		return runNoColor(args[2:])
	case modeCfgWidgets:
		return runWidgets(args[2:])
	case modeCfgRefresh:
		return runRefreshInterval(args[2:])
	case modeCfgTableRows:
		return runTableRows(args[2:])
	case modeCfgShow:
		return runConfigShow(args[2:])
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
	case "mode":
		return modeCfgMode, false, ""
	case "no-color":
		return modeCfgNoColor, false, ""
	case "widgets":
		return modeCfgWidgets, false, ""
	case "refresh-interval":
		return modeCfgRefresh, false, ""
	case "table-rows":
		return modeCfgTableRows, false, ""
	case "config":
		return modeCfgShow, false, ""
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

	// Collect subagent stats first: their turns live in separate agent-<id>.jsonl
	// files and are NOT part of session.Turns, but must enter cost reconciliation
	// (F4). Fail-soft when sessionDir cannot be determined.
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

	// Load per-session state, reconcile delta cost, and persist (Phase 6.8.a).
	// The reconciliation pool is session.Turns PLUS every subagent turn, so each
	// subagent turn receives a weighted PerTurnCost share; otherwise SubagentTotal
	// sums over UUIDs absent from PerTurnCost and renders Σ $0.00 (F4).
	// Fail-soft: errors log and render continues without state.
	ccTotal := payload.Cost.TotalCostUSD
	durMS := payload.Cost.TotalAPIDurationMS
	var st *state.Session
	if payload.SessionID != "" {
		st = state.Load(payload.SessionID)
		allTurns := make([]parser.Turn, len(session.Turns))
		copy(allTurns, session.Turns)
		for i := range subagents {
			allTurns = append(allTurns, subagents[i].Turns...)
		}
		cost.Reconcile(st, ccTotal, durMS, allTurns)
		if saveErr := state.Save(payload.SessionID, st); saveErr != nil {
			slog.Warn("state.Save failed", "err", saveErr)
		}
	}

	// Update global quota snapshot when rate_limits data is available (Phase 6.8.b).
	// Fail-soft: errors are logged; render continues with whatever Freshest() has.
	if payload.RateLimits != nil {
		rl := payload.RateLimits
		var reset5h, reset7d int64
		if t5h, ok := stdin.ParseResetsAt(rl.FiveHour.ResetsAt); ok {
			reset5h = t5h.Unix()
		}
		if t7d, ok := stdin.ParseResetsAt(rl.SevenDay.ResetsAt); ok {
			reset7d = t7d.Unix()
		}
		snap := quota.Snapshot{
			TS:            now.UnixMilli(),
			FiveHourPct:   rl.FiveHour.UsedPercentage,
			SevenDayPct:   rl.SevenDay.UsedPercentage,
			FiveHourReset: reset5h,
			SevenDayReset: reset7d,
		}
		if updateErr := quota.Update(snap); updateErr != nil {
			slog.Warn("quota.Update failed", "err", updateErr)
		}
	}

	// Load config cascade (Phase 6). Always lenient: errors become alerts, not crashes.
	ccfg, source, configErrs := config.LoadCascade(payload.Cwd)
	// Apply NO_COLOR from config only when env NO_COLOR is not already set.
	// ENV NO_COLOR > config.no_color > auto-detect (concept §7.3).
	baseTheme := renderer.Theme{AnsiEnabled: renderer.DetectAnsi(os.Stdout)}
	baseTheme.Colors = renderer.DefaultPalette()
	if ccfg.General.NoColor && os.Getenv("NO_COLOR") == "" {
		baseTheme.AnsiEnabled = false
	}
	// Sanitise invalid numeric ranges; ignore changed-field list (lenient render).
	_ = config.ApplyRangeFix(ccfg)
	pcfg := config.ToProbesConfig(*ccfg)
	pcfg.Email = config.ResolveEmail(ccfg, payload.Cwd)
	theme := config.ToTheme(*ccfg, baseTheme)
	configAlerts := config.ToCacheEvents(configErrs)
	slog.Debug("config loaded", "source", source, "errors", len(configErrs))

	// Phase 6.95.b: mode is read from the TOML config ([general].mode), not the
	// legacy per-session mode file. mode.Parse falls back to Standard on empty
	// or invalid values. The mode-file storage is no longer consulted at render.
	modeVal := mode.Parse(ccfg.General.Mode)

	d := probes.Data{
		Stdin:            payload,
		Session:          &session,
		Subagents:        subagents,
		Now:              now,
		SessionID:        payload.SessionID,
		ExtraCacheEvents: configAlerts,
	}

	// Populate delta-cost fields from reconciled state (Phase 6.8.a / 6.9.a).
	if st != nil {
		d.SessionTotal = cost.SessionTotal(st, ccTotal)
		d.SessionDurMS = cost.SessionDuration(st, durMS)
		// I3: compute curGroupID from the last turn's GroupID (not hardcoded 0).
		// GroupID is assigned by Aggregate; 0 means no groups yet (safe default).
		curGroupID := 0
		if len(session.Turns) > 0 {
			curGroupID = session.Turns[len(session.Turns)-1].GroupID
		}
		d.LastRequestCost = cost.LastRequest(st, ccTotal, curGroupID)
		// Capture st for per-turn lookup (closure over current st snapshot).
		captured := st
		d.PerTurnCostFn = func(uuid string) (float64, bool) {
			return cost.PerTurn(captured, uuid)
		}
		// C1: pass state to assembler for RenderUnified per-turn cost column.
		d.State = captured

		// Phase 6.95.h: extra-usage (paid overage). Trigger when a rate-limit
		// window is at ≥100% AND ~/.claude.json has hasExtraUsageEnabled. On the
		// first crossing ExtraUsageTick snapshots SessionTotal as the baseline;
		// the overage shown is SessionTotal − baseline. Both windows below 100%
		// clears the badge and resets the baseline (recomputed every refresh).
		at100 := payload.RateLimits != nil &&
			(payload.RateLimits.FiveHour.UsedPercentage >= 100 ||
				payload.RateLimits.SevenDay.UsedPercentage >= 100)
		d.ExtraActive, d.ExtraUSD = st.ExtraUsageTick(d.SessionTotal, at100, claudejson.HasExtraUsageEnabled())
	}

	// Detect git info for the current working directory.
	// Anti-flicker: on error use state.Session.LastGoodGit; on success update it.
	const gitTimeout = 150 * time.Millisecond
	if payload.Cwd != "" {
		gitCtx, gitCancel := context.WithTimeout(context.Background(), gitTimeout)
		freshGit, gitErr := parser.DetectGit(gitCtx, payload.Cwd)
		gitCancel()

		var lastGoodGit *parser.GitStatus
		if st != nil {
			lastGoodGit = st.LastGoodGit
		}
		resolved := parser.ResolveGitStatus(freshGit, gitErr, lastGoodGit)
		d.Git = resolved

		// Commit-badge (Phase 6.95.a): detect the modified-count N>0 → 0 transition
		// between the previous good status and this refresh, and decide whether to
		// render "✓ N committed" now. lastGoodGit is the prev snapshot (captured
		// before the overwrite below); freshGit is curr (valid only when gitErr==nil).
		if st != nil {
			prevN := 0
			if lastGoodGit != nil {
				prevN = lastGoodGit.ModifiedCount
			}
			currN := 0
			if freshGit != nil {
				currN = freshGit.ModifiedCount
			}
			d.CommitBadgeCount = st.CommitBadgeTick(prevN, currN, gitErr == nil && freshGit != nil)
		}

		// Persist the new successful result so the next invocation can use it.
		if gitErr == nil && st != nil && payload.SessionID != "" {
			st.LastGoodGit = freshGit
			if saveErr := state.Save(payload.SessionID, st); saveErr != nil {
				slog.Warn("state.Save after git update failed", "err", saveErr)
			}
		}
	}

	cols := renderer.DetectCols()
	a := statusline.Assembler{Mode: modeVal, Theme: theme, Cols: cols, Config: pcfg}
	raw := a.Render(d)

	// Phase 6.95.b: hint rotation now lives in state.Session (st.HintRotation),
	// mutated during Render via d.State. Persist after Render so the rotation
	// advance survives to the next invocation. Fail-soft: degrade to memory-only.
	if st != nil && payload.SessionID != "" {
		if saveErr := state.Save(payload.SessionID, st); saveErr != nil {
			slog.Warn("state.Save after render failed", "err", saveErr)
		}
	}

	final := renderer.Apply(raw, theme)
	fmt.Fprintln(os.Stdout, final)
	return 0
}

// Stubs filled in by later subtasks.
func runUninstall() int { return runUninstallImpl() } // delegated to uninstall.go (5.b)
func runInstall() int   { return runInstallImpl() }   // delegated to install.go (5.e)
func runCheck() int     { return 0 }                  // BL-14: --check subcommand not yet implemented
