// checkconfig.go implements `cc-probeline check-config` subcommand.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"

	"github.com/labzink/cc-probeline/internal/config"
	"golang.org/x/term"
)

// ANSI escape codes used by the text formatter.
const (
	ansiReset      = "\x1b[0m"
	ansiBoldRed    = "\x1b[1;31m"
	ansiBoldYellow = "\x1b[1;33m"
	ansiGreen      = "\x1b[32m"
	ansiRed        = "\x1b[31m"
	ansiBold       = "\x1b[1m"
	ansiDim        = "\x1b[2m"
)

// runCheckConfig is the entry point for `cc-probeline check-config [--verbose|-v] [--json]`.
// Returns exit code: 0 if no SeverityError (warnings allowed), 2 otherwise.
// Returns exit 64 on unknown flag.
func runCheckConfig(args []string) int {
	return runCheckConfigImpl(args, os.Stdout, os.Stderr)
}

// runCheckConfigImpl is the testable core of runCheckConfig, writing to supplied
// writers instead of os.Stdout/os.Stderr.
func runCheckConfigImpl(args []string, stdout, stderr io.Writer) int {
	// Parse flags: simple walk — no third-party flag library needed.
	var verbose, jsonOut bool
	for _, a := range args {
		switch a {
		case "--verbose", "-v":
			verbose = true
		case "--json":
			jsonOut = true
		default:
			fmt.Fprintf(stderr, "unknown flag: %s\nUsage: cc-probeline check-config [--verbose|-v] [--json]\n", a)
			return 64
		}
	}

	// Colors are enabled only when: stdout is a TTY AND NO_COLOR is unset AND --json is not requested.
	colorEnabled := !jsonOut &&
		os.Getenv("NO_COLOR") == "" &&
		isCheckTTY(stdout)

	cw := &ccColorWriter{w: stdout, enabled: colorEnabled}

	cwd, _ := os.Getwd()
	cfg, source, errs := config.LoadCascade(cwd)

	if jsonOut {
		printCheckConfigJSON(stdout, source, cfg, errs)
	} else {
		printCheckConfigText(cw, source, cfg, errs, verbose, cwd)
	}

	for _, e := range errs {
		if e.Severity == config.SeverityError {
			return 2
		}
	}
	return 0
}

// isCheckTTY reports whether w is os.Stdout backed by a terminal.
func isCheckTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// ccColorWriter wraps an io.Writer and applies ANSI codes when colors are enabled.
type ccColorWriter struct {
	w       io.Writer
	enabled bool
}

// colorize wraps s in the given ANSI code + reset when colors are enabled.
func (c *ccColorWriter) colorize(s, code string) string {
	if !c.enabled {
		return s
	}
	return code + s + ansiReset
}

// ─── text formatter ──────────────────────────────────────────────────────────

// printCheckConfigText writes the human-readable check-config output.
func printCheckConfigText(cw *ccColorWriter, source config.Source, cfg *config.Config, errs []config.Error, verbose bool, cwd string) {
	w := cw.w

	// Header.
	fmt.Fprintf(w, "cc-probeline check-config v%s\n", versionString())

	switch source {
	case config.SourceDefaults:
		fmt.Fprintln(w, "Source: defaults (no config file found)")
		fmt.Fprintln(w)
		printLocationsChecked(w, cwd)
		fmt.Fprintln(w)
		fmt.Fprintln(w, cw.colorize("✓", ansiGreen)+" Running with built-in defaults. To customize, create the global config:")
		fmt.Fprintln(w, "  mkdir -p ~/.config/cc-probeline")
		fmt.Fprintln(w, "  # then write config.toml — see https://github.com/labzink/cc-probeline#configuration")
		return

	case config.SourceGlobal:
		gp := checkGlobalConfigPath()
		fmt.Fprintf(w, "Source: global (%s)\n", cw.colorize(gp, ansiDim))

	case config.SourceProject:
		pp := configPathFromErrors(errs)
		if pp == "" {
			pp = checkFindProjectConfig(cwd)
		}
		fmt.Fprintf(w, "Source: project (%s)\n", cw.colorize(pp, ansiDim))

	case config.SourceEnv:
		ep := os.Getenv("CC_PROBELINE_CONFIG")
		fmt.Fprintf(w, "Source: env (%s)\n", cw.colorize(ep, ansiDim))

	default:
		fmt.Fprintf(w, "Source: %s\n", source)
	}

	fmt.Fprintln(w)

	// Print each error/warning entry.
	for _, e := range errs {
		printCheckConfigError(cw, e)
	}

	// Effective config section.
	printEffectiveConfig(w, cfg, verbose)

	// Summary line.
	numErrors, numWarnings := countSeverities(errs)
	if numErrors == 0 && numWarnings == 0 {
		fmt.Fprintln(w, cw.colorize("✓", ansiGreen)+" No errors. Config is valid.")
	} else if numErrors == 0 {
		fmt.Fprintf(w, "%s No errors, %d warning(s). Config is valid.\n",
			cw.colorize("✓", ansiGreen), numWarnings)
	} else {
		errWord := "error"
		if numErrors != 1 {
			errWord = "errors"
		}
		warnWord := "warning"
		if numWarnings != 1 {
			warnWord = "warnings"
		}
		fmt.Fprintf(w, "%s %d %s, %d %s. Run `cc-probeline check-config --verbose` for full schema.\n",
			cw.colorize("✗", ansiRed), numErrors, errWord, numWarnings, warnWord)
	}
}

// printLocationsChecked prints the cascade location check list for the "defaults" case.
func printLocationsChecked(w io.Writer, cwd string) {
	fmt.Fprintln(w, "Locations checked (in order):")

	// 1. CC_PROBELINE_CONFIG env.
	envVal := os.Getenv("CC_PROBELINE_CONFIG")
	if envVal == "" {
		envVal = "not set"
	}
	fmt.Fprintf(w, "  1. CC_PROBELINE_CONFIG env: %s\n", envVal)

	// 2. project-local.
	projPath := checkFindProjectConfig(cwd)
	if projPath != "" {
		fmt.Fprintf(w, "  2. project-local:           found at %s\n", projPath)
	} else {
		fmt.Fprintf(w, "  2. project-local:           not found (searched upward from %s)\n", cwd)
	}

	// 3. global.
	gp := checkGlobalConfigPath()
	if gp == "" {
		fmt.Fprintf(w, "  3. global:                  not found (could not determine home directory)\n")
	} else {
		// Normalise home prefix to ~ for display.
		home := os.Getenv("HOME")
		display := gp
		if home != "" && len(gp) > len(home) && gp[:len(home)] == home {
			display = "~" + gp[len(home):]
		}
		fmt.Fprintf(w, "  3. global:                  not found (%s)\n", display)
	}
}

// printCheckConfigError writes a single error/warning entry in tsc-style format.
func printCheckConfigError(cw *ccColorWriter, e config.Error) {
	w := cw.w

	// Location prefix: file:line:col
	loc := ""
	if e.File != "" {
		loc = e.File
		if e.Line > 0 {
			loc = fmt.Sprintf("%s:%d", loc, e.Line)
			if e.Column > 0 {
				loc = fmt.Sprintf("%s:%d", loc, e.Column)
			}
		}
	}

	// Severity label with color.
	var sevLabel string
	if e.Severity == config.SeverityError {
		sevLabel = cw.colorize("error", ansiBoldRed)
	} else {
		sevLabel = cw.colorize("warning", ansiBoldYellow)
	}

	// Field name (bold).
	field := e.Field
	if field == "" {
		field = "(unknown)"
	}
	fieldStr := cw.colorize(field, ansiBold)

	if loc != "" {
		fmt.Fprintf(w, "%s - %s: %s\n", cw.colorize(loc, ansiDim), sevLabel, fieldStr)
	} else {
		fmt.Fprintf(w, "%s: %s\n", sevLabel, fieldStr)
	}

	fmt.Fprintf(w, "  %s\n", e.Message)
	if e.Hint != "" {
		fmt.Fprintf(w, "  %s\n", e.Hint)
	}
	fmt.Fprintln(w)
}

// printEffectiveConfig prints the effective config section.
// When verbose=true, all fields are printed; otherwise only non-default fields.
func printEffectiveConfig(w io.Writer, cfg *config.Config, verbose bool) {
	def := config.Default()
	fields := collectConfigFields(cfg, def, verbose)
	if len(fields) == 0 {
		return
	}

	fmt.Fprintln(w, "Effective config:")
	for _, f := range fields {
		fmt.Fprintf(w, "  %-36s = %v\n", f.key, f.val)
	}
	fmt.Fprintln(w)
}

type configField struct {
	key string
	val interface{}
}

// collectConfigFields returns a flat list of config field entries.
// When verbose=true all fields are included; otherwise only those differing from defaults.
func collectConfigFields(cfg, def *config.Config, verbose bool) []configField {
	var fields []configField

	add := func(key string, val, defVal interface{}) {
		if verbose || !reflect.DeepEqual(val, defVal) {
			fields = append(fields, configField{key: key, val: formatConfigVal(val)})
		}
	}

	// General.
	add("general.tutorial_hints", cfg.General.TutorialHints, def.General.TutorialHints)
	add("general.no_color", cfg.General.NoColor, def.General.NoColor)
	add("general.nerd_font", cfg.General.NerdFont, def.General.NerdFont)
	add("general.refresh_interval_hint", cfg.General.RefreshIntervalHint, def.General.RefreshIntervalHint)
	add("general.table_rows", cfg.General.TableRows, def.General.TableRows)
	add("general.mode", cfg.General.Mode, def.General.Mode)

	// Widgets.
	add("widgets.model", cfg.Widgets.Model, def.Widgets.Model)
	add("widgets.effort", cfg.Widgets.Effort, def.Widgets.Effort)
	add("widgets.cost", cfg.Widgets.Cost, def.Widgets.Cost)
	add("widgets.project", cfg.Widgets.Project, def.Widgets.Project)
	add("widgets.email", cfg.Widgets.Email, def.Widgets.Email)
	add("widgets.time", cfg.Widgets.Time, def.Widgets.Time)
	add("widgets.ctx", cfg.Widgets.Ctx, def.Widgets.Ctx)
	add("widgets.quota", cfg.Widgets.Quota, def.Widgets.Quota)
	add("widgets.git", cfg.Widgets.Git, def.Widgets.Git)

	// Thresholds.
	add("thresholds.cost_budget_usd", cfg.Thresholds.CostBudgetUSD, def.Thresholds.CostBudgetUSD)
	add("thresholds.ctx_warn_ratio", cfg.Thresholds.CtxWarnRatio, def.Thresholds.CtxWarnRatio)
	add("thresholds.ctx_critical_ratio", cfg.Thresholds.CtxCriticalRatio, def.Thresholds.CtxCriticalRatio)
	add("thresholds.orch_ttl_minutes", cfg.Thresholds.OrchTTLMinutes, def.Thresholds.OrchTTLMinutes)
	add("thresholds.subagent_gap_minutes", cfg.Thresholds.SubagentGapMinutes, def.Thresholds.SubagentGapMinutes)

	// Probes.
	add("probes.email.address", cfg.Probes.Email.Address, def.Probes.Email.Address)

	return fields
}

// formatConfigVal formats a config field value for display.
func formatConfigVal(v interface{}) interface{} {
	switch t := v.(type) {
	case bool:
		if t {
			return "true"
		}
		return "false"
	case string:
		if t == "" {
			return `""`
		}
		return t
	default:
		return v
	}
}

// countSeverities returns (numErrors, numWarnings) from errs.
func countSeverities(errs []config.Error) (int, int) {
	var numErrors, numWarnings int
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			numErrors++
		} else {
			numWarnings++
		}
	}
	return numErrors, numWarnings
}

// configPathFromErrors extracts the first non-empty file path from errs.
func configPathFromErrors(errs []config.Error) string {
	for _, e := range errs {
		if e.File != "" {
			return e.File
		}
	}
	return ""
}

// checkGlobalConfigPath returns the platform-appropriate global config location.
// Delegates to config.GlobalConfigPath to avoid duplication with path.go.
func checkGlobalConfigPath() string { return config.GlobalConfigPath() }

// checkFindProjectConfig walks up from cwd looking for .cc-probeline.toml.
// Delegates to config.FindProjectConfig to avoid duplication with path.go.
func checkFindProjectConfig(cwd string) string { return config.FindProjectConfig(cwd) }

// ─── JSON formatter ──────────────────────────────────────────────────────────

// jsonCheckOut is the stable JSON schema for `check-config --json`.
type jsonCheckOut struct {
	Version int            `json:"version"`
	Source  string         `json:"source"`
	Path    string         `json:"path"`
	Valid   bool           `json:"valid"`
	Config  *config.Config `json:"config"`
	Errors  []jsonCheckErr `json:"errors"`
}

// jsonCheckErr is a single error entry in the JSON output.
type jsonCheckErr struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Column     int    `json:"column"`
	Field      string `json:"field"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
	Severity   string `json:"severity"`
}

// printCheckConfigJSON writes the JSON output to w. No ANSI codes are emitted.
func printCheckConfigJSON(w io.Writer, source config.Source, cfg *config.Config, errs []config.Error) {
	numErrors, _ := countSeverities(errs)

	// Determine path: empty for defaults.
	path := ""
	if source != config.SourceDefaults {
		// Try to get from errors first (most accurate for env/project sources).
		path = configPathFromErrors(errs)
		if path == "" {
			switch source {
			case config.SourceGlobal:
				path = checkGlobalConfigPath()
			case config.SourceEnv:
				path = os.Getenv("CC_PROBELINE_CONFIG")
			}
		}
	}

	jsonErrs := make([]jsonCheckErr, 0, len(errs))
	for _, e := range errs {
		sev := "warning"
		if e.Severity == config.SeverityError {
			sev = "error"
		}
		jsonErrs = append(jsonErrs, jsonCheckErr{
			File:       e.File,
			Line:       e.Line,
			Column:     e.Column,
			Field:      e.Field,
			Message:    e.Message,
			Suggestion: e.Hint,
			Severity:   sev,
		})
	}

	out := jsonCheckOut{
		Version: 1,
		Source:  string(source),
		Path:    path,
		Valid:   numErrors == 0,
		Config:  cfg,
		Errors:  jsonErrs,
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(w, `{"error":"json marshal failed: %s"}`+"\n", err.Error())
		return
	}
	fmt.Fprintf(w, "%s\n", data)
}
