package probes_test

// toggle_test.go — RED tests for Phase 6.g widget toggle wiring.
//
// Each probe must consult its c.XEnabled flag in Visible(d, c) and return false
// when the flag is false. These tests FAIL until GREEN adds the 1-line guard to
// each probe that does not yet have one.
//
// Test IDs: T-WT1..T-WT15 per plan §4.1.

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/config"
	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/statusline"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// ----------------------------------------------------------------------------
// Fixture helpers
// ----------------------------------------------------------------------------

// dataWithModel returns a Data fixture that satisfies ModelProbe.Visible baseline:
// Stdin.Model.ID is non-empty.
func dataWithModel() probes.Data {
	return probes.Data{
		Stdin: stdin.Payload{
			Model: stdin.Model{ID: "claude-sonnet-4-6"},
		},
	}
}

// dataWithEffort returns a Data fixture that satisfies EffortProbe.Visible baseline:
// Stdin.Effort.Level is non-empty and not "off".
func dataWithEffort() probes.Data {
	return probes.Data{
		Stdin: stdin.Payload{
			Effort: stdin.Effort{Level: "high"},
		},
	}
}

// dataWithCost returns a Data fixture. CostProbe.Visible always returns true,
// so any Data works; we use the zero value for clarity.
func dataWithCost() probes.Data {
	return probes.Data{}
}

// dataWithProject returns a Data fixture. ProjectProbe.Visible always returns true.
func dataWithProject() probes.Data {
	return probes.Data{}
}

// dataWithEmail returns a Data fixture. EmailProbe.Visible depends only on Config,
// so Data can be zero; the config fixture sets Email and EmailEnabled.
func dataWithEmail() probes.Data {
	return probes.Data{}
}

// dataWithTime returns a Data fixture. TimeProbe.Visible always returns true.
func dataWithTime() probes.Data {
	return probes.Data{}
}

// dataWithCtx returns a Data fixture that satisfies CtxProbe.Visible baseline:
// Stdin.ContextWindow.Size > 0.
func dataWithCtx() probes.Data {
	return probes.Data{
		Stdin: stdin.Payload{
			ContextWindow: stdin.ContextWindow{
				Size: 200_000,
				CurrentUsage: map[string]int{
					"input_tokens":                10_000,
					"cache_read_input_tokens":     5_000,
					"cache_creation_input_tokens": 2_000,
				},
			},
		},
	}
}

// dataWithQuota returns a Data fixture that satisfies QuotaProbe.Visible:
// QuotaEnabled=true (set via Config) AND RateLimits != nil.
// Updated in Phase 6.5.b4: real rate_limits required for visibility.
func dataWithQuota() probes.Data {
	rl := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{UsedPercentage: 50},
		SevenDay: stdin.RateWindow{UsedPercentage: 50},
	}
	return probes.Data{Stdin: stdin.Payload{RateLimits: rl}}
}

// dataWithGit returns a Data fixture that satisfies GitProbe.Visible baseline:
// Git is non-nil.
func dataWithGit() probes.Data {
	return probes.Data{
		Git: &parser.GitStatus{Branch: "main"},
	}
}

// dataWithSubagent returns a Data fixture that satisfies SubagentProbe.Visible
// baseline: Stdin.Tasks is non-empty.
func dataWithSubagent() probes.Data {
	return probes.Data{
		Stdin: stdin.Payload{
			Tasks: []stdin.Task{
				{
					ID:        "agent-1",
					Name:      "sub-task",
					StartTime: time.Now().Add(-1 * time.Minute),
				},
			},
		},
		Now: time.Now(),
	}
}

// cfgAllOn returns a probes.Config with all XEnabled fields set to true and an
// email address populated (satisfying EmailProbe's non-empty Email requirement).
func cfgAllOn() probes.Config {
	return probes.Config{
		ModelEnabled:    true,
		EffortEnabled:   true,
		CostEnabled:     true,
		ProjectEnabled:  true,
		EmailEnabled:    true,
		TimeEnabled:     true,
		CtxEnabled:      true,
		CacheEnabled:    true,
		QuotaEnabled:    true,
		GitEnabled:      true,
		SubagentEnabled: true,
		Email:           "user@example.com",
	}
}

// ----------------------------------------------------------------------------
// T-WT1: ModelProbe — toggle off
// ----------------------------------------------------------------------------

// TestToggle_ModelOff_NotVisible verifies that ModelProbe.Visible returns false
// when c.ModelEnabled is false, even when the data fixture satisfies the
// data-driven precondition (Model.ID non-empty).
func TestToggle_ModelOff_NotVisible(t *testing.T) {
	p := &probes.ModelProbe{}
	d := dataWithModel()
	c := cfgAllOn()
	c.ModelEnabled = false

	if p.Visible(d, c) {
		t.Errorf("T-WT1: ModelProbe.Visible() = true, want false when ModelEnabled=false")
	}
}

// ----------------------------------------------------------------------------
// T-WT2: ModelProbe — toggle on (baseline preserved)
// ----------------------------------------------------------------------------

// TestToggle_ModelOn_VisibilityPreserved verifies that ModelProbe.Visible
// returns true when ModelEnabled=true and data satisfies the precondition.
func TestToggle_ModelOn_VisibilityPreserved(t *testing.T) {
	p := &probes.ModelProbe{}
	d := dataWithModel()
	c := cfgAllOn()

	if !p.Visible(d, c) {
		t.Errorf("T-WT2: ModelProbe.Visible() = false, want true when ModelEnabled=true and Model.ID non-empty")
	}
}

// ----------------------------------------------------------------------------
// T-WT3..T-WT12: table-driven toggle-off tests for the remaining 10 probes
// ----------------------------------------------------------------------------

// probeToggleCase describes one probe's toggle-off scenario.
type probeToggleCase struct {
	name    string
	probe   probes.Probe
	data    probes.Data
	disable func(*probes.Config) // sets the probe's XEnabled field to false
}

var toggleOffCases = []probeToggleCase{
	// T-WT3
	{
		name:    "T-WT3/TestToggle_EffortOff_NotVisible",
		probe:   &probes.EffortProbe{},
		data:    dataWithEffort(),
		disable: func(c *probes.Config) { c.EffortEnabled = false },
	},
	// T-WT4
	{
		name:    "T-WT4/TestToggle_CostOff_NotVisible",
		probe:   &probes.CostProbe{},
		data:    dataWithCost(),
		disable: func(c *probes.Config) { c.CostEnabled = false },
	},
	// T-WT5
	{
		name:    "T-WT5/TestToggle_ProjectOff_NotVisible",
		probe:   &probes.ProjectProbe{},
		data:    dataWithProject(),
		disable: func(c *probes.Config) { c.ProjectEnabled = false },
	},
	// T-WT6
	{
		name:    "T-WT6/TestToggle_EmailOff_NotVisible",
		probe:   &probes.EmailProbe{},
		data:    dataWithEmail(),
		disable: func(c *probes.Config) { c.EmailEnabled = false },
	},
	// T-WT7
	{
		name:    "T-WT7/TestToggle_TimeOff_NotVisible",
		probe:   &probes.TimeProbe{},
		data:    dataWithTime(),
		disable: func(c *probes.Config) { c.TimeEnabled = false },
	},
	// T-WT8
	{
		name:    "T-WT8/TestToggle_CtxOff_NotVisible",
		probe:   &probes.CtxProbe{},
		data:    dataWithCtx(),
		disable: func(c *probes.Config) { c.CtxEnabled = false },
	},
	// T-WT10
	{
		name:    "T-WT10/TestToggle_QuotaOff_NotVisible",
		probe:   &probes.QuotaProbe{},
		data:    dataWithQuota(),
		disable: func(c *probes.Config) { c.QuotaEnabled = false },
	},
	// T-WT11
	{
		name:    "T-WT11/TestToggle_GitOff_NotVisible",
		probe:   &probes.GitProbe{},
		data:    dataWithGit(),
		disable: func(c *probes.Config) { c.GitEnabled = false },
	},
	// T-WT12
	{
		name:    "T-WT12/TestToggle_SubagentOff_NotVisible",
		probe:   &probes.SubagentProbe{},
		data:    dataWithSubagent(),
		disable: func(c *probes.Config) { c.SubagentEnabled = false },
	},
}

// TestToggle_TableDriven_ToggleOff runs T-WT3..T-WT12 as a table-driven test.
// Each sub-test sets one XEnabled field to false and asserts Visible() == false.
func TestToggle_TableDriven_ToggleOff(t *testing.T) {
	for _, tc := range toggleOffCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			c := cfgAllOn()
			tc.disable(&c)

			if tc.probe.Visible(tc.data, c) {
				t.Errorf("%s: Visible() = true, want false when toggle disabled", tc.name)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// T-WT13: assembler integration smoke (skipped — requires 6.d render integration)
// ----------------------------------------------------------------------------

// TestToggle_AllOff_NoVisibleProbes is a smoke test that verifies assembler.Render
// produces no visible probe output when all widget toggles are false.
func TestToggle_AllOff_NoVisibleProbes(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	// Config with all XEnabled fields false (zero-value probes.Config).
	cfg := probes.Config{} // all toggles false by default

	a := &statusline.Assembler{
		Mode:   mode.SuperCompact,
		Theme:  renderer.Theme{},
		Cols:   80,
		Config: cfg,
	}

	d := probes.Data{
		Stdin: stdin.Payload{
			Model: stdin.Model{ID: "claude-sonnet-4-6"},
		},
		Session:      &parser.SessionStats{Turns: []parser.Turn{{Index: 1, Role: "orch"}}},
		TerminalCols: 80,
		Now:          time.Now(),
	}

	out := a.Render(d)

	// When all probes are off, no probe-specific content should appear.
	// We check for content that only comes from active probes, not from the
	// hint.Widget (which can surface cache/effort strings in alert texts).
	// Specifically: model ID, cost '$', and known probe prefixes (not hint text).
	probeStrings := []string{"claude-sonnet", "$0.", "ctx 1", "quota", "git:"}
	for _, s := range probeStrings {
		if strings.Contains(out, s) {
			t.Errorf("T-WT13: probe content %q present in output despite all toggles off: %q", s, out)
		}
	}
}

// ----------------------------------------------------------------------------
// T-WT14: regression baseline — Default() gives all-on behaviour
// ----------------------------------------------------------------------------

// TestToggle_DefaultsAllOn_RegressionBaseline verifies that ToProbesConfig applied
// to Default() yields a Config with all 11 XEnabled fields set to true, and that
// each probe's Visible() with a populated fixture returns the same value as
// Phase 4-5 behaviour (i.e. the toggle does not suppress a probe that has data).
func TestToggle_DefaultsAllOn_RegressionBaseline(t *testing.T) {
	cfg := config.ToProbesConfig(*config.Default())

	// Assert all 11 XEnabled fields are true.
	if !cfg.ModelEnabled {
		t.Error("T-WT14: ModelEnabled = false in Default(); want true")
	}
	if !cfg.EffortEnabled {
		t.Error("T-WT14: EffortEnabled = false in Default(); want true")
	}
	if !cfg.CostEnabled {
		t.Error("T-WT14: CostEnabled = false in Default(); want true")
	}
	if !cfg.ProjectEnabled {
		t.Error("T-WT14: ProjectEnabled = false in Default(); want true")
	}
	if !cfg.EmailEnabled {
		t.Error("T-WT14: EmailEnabled = false in Default(); want true")
	}
	if !cfg.TimeEnabled {
		t.Error("T-WT14: TimeEnabled = false in Default(); want true")
	}
	if !cfg.CtxEnabled {
		t.Error("T-WT14: CtxEnabled = false in Default(); want true")
	}
	if !cfg.CacheEnabled {
		t.Error("T-WT14: CacheEnabled = false in Default(); want true")
	}
	if !cfg.QuotaEnabled {
		t.Error("T-WT14: QuotaEnabled = false in Default(); want true")
	}
	if !cfg.GitEnabled {
		t.Error("T-WT14: GitEnabled = false in Default(); want true")
	}
	if !cfg.SubagentEnabled {
		t.Error("T-WT14: SubagentEnabled = false in Default(); want true")
	}

	// Regression baseline: probes with data should be visible under Default() config.

	// ModelProbe: visible when Model.ID non-empty.
	if !(&probes.ModelProbe{}).Visible(dataWithModel(), cfg) {
		t.Error("T-WT14: ModelProbe.Visible() = false under Default(); want true")
	}

	// EffortProbe: visible when Effort.Level is valid and non-"off".
	if !(&probes.EffortProbe{}).Visible(dataWithEffort(), cfg) {
		t.Error("T-WT14: EffortProbe.Visible() = false under Default(); want true")
	}

	// CostProbe: always visible (even at $0.00).
	if !(&probes.CostProbe{}).Visible(dataWithCost(), cfg) {
		t.Error("T-WT14: CostProbe.Visible() = false under Default(); want true")
	}

	// ProjectProbe: always visible (falls back to "?").
	if !(&probes.ProjectProbe{}).Visible(dataWithProject(), cfg) {
		t.Error("T-WT14: ProjectProbe.Visible() = false under Default(); want true")
	}

	// EmailProbe: visible only when EmailEnabled=true AND Email non-empty.
	// Default() has Email="" so EmailProbe.Visible returns false even with toggle on.
	// This is expected Phase 4-5 behaviour: email probe is gated on having an address.
	emailCfgWithAddr := cfg
	emailCfgWithAddr.Email = "user@example.com"
	if !(&probes.EmailProbe{}).Visible(dataWithEmail(), emailCfgWithAddr) {
		t.Error("T-WT14: EmailProbe.Visible() = false when EmailEnabled=true and Email non-empty; want true")
	}

	// TimeProbe: always visible.
	if !(&probes.TimeProbe{}).Visible(dataWithTime(), cfg) {
		t.Error("T-WT14: TimeProbe.Visible() = false under Default(); want true")
	}

	// CtxProbe: visible when ContextWindow.Size > 0.
	if !(&probes.CtxProbe{}).Visible(dataWithCtx(), cfg) {
		t.Error("T-WT14: CtxProbe.Visible() = false under Default() with Size>0; want true")
	}

	// QuotaProbe: visible when QuotaEnabled=true (already set in Default()).
	if !(&probes.QuotaProbe{}).Visible(dataWithQuota(), cfg) {
		t.Error("T-WT14: QuotaProbe.Visible() = false under Default(); want true")
	}

	// GitProbe: visible when Git is non-nil.
	if !(&probes.GitProbe{}).Visible(dataWithGit(), cfg) {
		t.Error("T-WT14: GitProbe.Visible() = false under Default() with non-nil Git; want true")
	}

	// SubagentProbe: visible when Stdin.Tasks is non-empty.
	if !(&probes.SubagentProbe{}).Visible(dataWithSubagent(), cfg) {
		t.Error("T-WT14: SubagentProbe.Visible() = false under Default() with non-empty Tasks; want true")
	}
}

// ----------------------------------------------------------------------------
// T-WT15: example file parses via Load (skipped — requires 6.b Load)
// ----------------------------------------------------------------------------

// TestExampleFile_TomlParses verifies that scripts/config.toml.example parses
// successfully via config.Load without SeverityError entries.
func TestExampleFile_TomlParses(t *testing.T) {
	_, errs := config.Load("../../scripts/config.toml.example")
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			t.Errorf("T-WT15: config.toml.example has SeverityError: %v", e)
		}
	}
}
