// Package config_test contains unit tests for Load / parseFile.
// Tests T-L1..T-L8 per phase-6-plan-6.b.md §3.1.
//
// NOTE: These tests target the GREEN implementation of errors.go where
// Error.File and Error.Field replace the stub's Error.Path and Error.Key.
// They will compile once the GREEN agent rewrites internal/config/errors.go.
package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/config"
)

// T-L1: Load("") returns Default() with no errors.
func TestLoad_EmptyPath(t *testing.T) {
	cfg, errs := config.Load("")
	if cfg == nil {
		t.Fatal("T-L1: Load(\"\") returned nil cfg")
	}
	def := config.Default()
	if cfg.General.TutorialHints != def.General.TutorialHints {
		t.Errorf("T-L1: TutorialHints: got %v, want %v", cfg.General.TutorialHints, def.General.TutorialHints)
	}
	if cfg.Theme.Name != def.Theme.Name {
		t.Errorf("T-L1: Theme.Name: got %q, want %q", cfg.Theme.Name, def.Theme.Name)
	}
	if cfg.Thresholds.CtxWarnRatio != def.Thresholds.CtxWarnRatio {
		t.Errorf("T-L1: CtxWarnRatio: got %v, want %v", cfg.Thresholds.CtxWarnRatio, def.Thresholds.CtxWarnRatio)
	}
	if len(errs) != 0 {
		t.Errorf("T-L1: expected 0 errors, got %d: %v", len(errs), errs)
	}
}

// T-L2: Load on a non-existent path returns Default() and one Error with
// "not found" in the message and SeverityError.
func TestLoad_NonExistent(t *testing.T) {
	cfg, errs := config.Load("/tmp/cc-probeline-does-not-exist-12345.toml")
	if cfg == nil {
		t.Fatal("T-L2: cfg is nil")
	}
	// cfg should be Default()
	def := config.Default()
	if cfg.Theme.Name != def.Theme.Name {
		t.Errorf("T-L2: Theme.Name: got %q, want %q (should be Default)", cfg.Theme.Name, def.Theme.Name)
	}
	if len(errs) == 0 {
		t.Fatal("T-L2: expected 1+ error, got 0")
	}
	foundErr := false
	for _, e := range errs {
		if e.Severity == config.SeverityError && strings.Contains(strings.ToLower(e.Message), "not found") {
			foundErr = true
		}
	}
	if !foundErr {
		t.Errorf("T-L2: no SeverityError with 'not found' in message; errs=%v", errs)
	}
}

// T-L3: Load on a valid complete TOML returns cfg matching the fixture and
// no errors.
func TestLoad_ValidComplete(t *testing.T) {
	path := filepath.Join("..", "fixtures", "config", "valid-complete.toml")
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("T-L3: filepath.Abs: %v", err)
	}

	cfg, errs := config.Load(abs)
	if cfg == nil {
		t.Fatal("T-L3: cfg is nil")
	}
	// All errors should be nil or only warnings (no parse errors).
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			t.Errorf("T-L3: unexpected SeverityError: %v", e)
		}
	}

	// Assert fixture-specific values (from valid-complete.toml).
	if cfg.Version != 1 {
		t.Errorf("T-L3: Version: got %d, want 1", cfg.Version)
	}
	if cfg.General.TutorialHints != false {
		t.Errorf("T-L3: TutorialHints: got true, want false")
	}
	if cfg.General.NoColor != true {
		t.Errorf("T-L3: NoColor: got false, want true")
	}
	if cfg.General.NerdFont != true {
		t.Errorf("T-L3: NerdFont: got false, want true")
	}
	if cfg.General.RefreshIntervalHint != 10 {
		t.Errorf("T-L3: RefreshIntervalHint: got %d, want 10", cfg.General.RefreshIntervalHint)
	}
	if cfg.Theme.Name != "high-contrast" {
		t.Errorf("T-L3: Theme.Name: got %q, want %q", cfg.Theme.Name, "high-contrast")
	}
	if cfg.Theme.Colors.Cyan != "#00FFFF" {
		t.Errorf("T-L3: Colors.Cyan: got %q, want %q", cfg.Theme.Colors.Cyan, "#00FFFF")
	}
	if cfg.Widgets.Effort != false {
		t.Errorf("T-L3: Widgets.Effort: got true, want false")
	}
	if cfg.Thresholds.CostBudgetUSD != 5.0 {
		t.Errorf("T-L3: CostBudgetUSD: got %v, want 5.0", cfg.Thresholds.CostBudgetUSD)
	}
	if cfg.Thresholds.CtxWarnRatio != 0.65 {
		t.Errorf("T-L3: CtxWarnRatio: got %v, want 0.65", cfg.Thresholds.CtxWarnRatio)
	}
	if cfg.Probes.Email.Address != "test@example.com" {
		t.Errorf("T-L3: Probes.Email.Address: got %q, want %q", cfg.Probes.Email.Address, "test@example.com")
	}
}

// T-L4: Load on a partial TOML (only [general] tutorial_hints=false) returns
// cfg with that field set and all other fields matching Default() values.
func TestLoad_ValidPartial(t *testing.T) {
	path := filepath.Join("..", "fixtures", "config", "valid-partial.toml")
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("T-L4: filepath.Abs: %v", err)
	}

	cfg, errs := config.Load(abs)
	if cfg == nil {
		t.Fatal("T-L4: cfg is nil")
	}
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			t.Errorf("T-L4: unexpected SeverityError: %v", e)
		}
	}

	// The explicit field from the fixture.
	if cfg.General.TutorialHints != false {
		t.Errorf("T-L4: TutorialHints: got true, want false")
	}

	// All other fields must match Default() (partial parse preserves defaults).
	def := config.Default()
	if cfg.Theme.Name != def.Theme.Name {
		t.Errorf("T-L4: Theme.Name: got %q, want %q (default)", cfg.Theme.Name, def.Theme.Name)
	}
	if cfg.Thresholds.CtxWarnRatio != def.Thresholds.CtxWarnRatio {
		t.Errorf("T-L4: CtxWarnRatio: got %v, want %v (default)", cfg.Thresholds.CtxWarnRatio, def.Thresholds.CtxWarnRatio)
	}
	if cfg.Widgets.Model != def.Widgets.Model {
		t.Errorf("T-L4: Widgets.Model: got %v, want %v (default)", cfg.Widgets.Model, def.Widgets.Model)
	}
	if cfg.General.RefreshIntervalHint != def.General.RefreshIntervalHint {
		t.Errorf("T-L4: RefreshIntervalHint: got %d, want %d (default)", cfg.General.RefreshIntervalHint, def.General.RefreshIntervalHint)
	}
}

// T-L5: Load on a malformed TOML file returns Default() and 1+ errors with
// Severity=Error and Line > 0 (line number captured from pelletier).
// Fixture malformed.toml: line 2 has "[broken" (missing "]").
// pelletier reports: row=2, col=8.
func TestLoad_MalformedTOML(t *testing.T) {
	path := filepath.Join("..", "fixtures", "config", "malformed.toml")
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("T-L5: filepath.Abs: %v", err)
	}

	cfg, errs := config.Load(abs)
	if cfg == nil {
		t.Fatal("T-L5: cfg is nil")
	}
	// Must return Default() on fatal parse error.
	def := config.Default()
	if cfg.Theme.Name != def.Theme.Name {
		t.Errorf("T-L5: Theme.Name: got %q, want %q (Default)", cfg.Theme.Name, def.Theme.Name)
	}

	if len(errs) == 0 {
		t.Fatal("T-L5: expected 1+ errors, got 0")
	}
	foundSevError := false
	foundLine := false
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			foundSevError = true
			if e.Line > 0 {
				foundLine = true
			}
		}
	}
	if !foundSevError {
		t.Error("T-L5: no SeverityError found in errs")
	}
	if !foundLine {
		t.Error("T-L5: no Error with Line > 0 found (insurance #1: line number must be captured)")
	}
}

// T-L6: Load on TOML with an unknown field returns the cfg (with known fields
// parsed correctly) and 1 Error with Severity=Warning and Field == the unknown key.
// Fixture unknown-field.toml: [general] tutorial_hints=true, ctx_silly_ratio=0.5.
// pelletier strict mode reports: StrictMissingError with key=[general ctx_silly_ratio].
func TestLoad_UnknownField(t *testing.T) {
	path := filepath.Join("..", "fixtures", "config", "unknown-field.toml")
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("T-L6: filepath.Abs: %v", err)
	}

	cfg, errs := config.Load(abs)
	if cfg == nil {
		t.Fatal("T-L6: cfg is nil")
	}

	// Known field must be parsed correctly.
	if cfg.General.TutorialHints != true {
		t.Errorf("T-L6: TutorialHints: got false, want true")
	}

	// Expect at least 1 warning error about the unknown field.
	if len(errs) == 0 {
		t.Fatal("T-L6: expected 1+ errors, got 0")
	}
	// GREEN renames Key→Field in errors.go. Until then, check Key (current stub field).
	// After GREEN: change e.Key to e.Field here.
	foundWarning := false
	for _, e := range errs {
		if e.Severity == config.SeverityWarning && strings.Contains(e.Key, "ctx_silly_ratio") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Errorf("T-L6: expected SeverityWarning with Key/Field containing \"ctx_silly_ratio\"; errs=%v", errs)
	}
}

// T-L7: Load on TOML with a type mismatch returns 1+ Errors with Severity=Error
// and a message describing the type mismatch.
// Fixture type-mismatch.toml: [general] tutorial_hints="not a bool".
// pelletier reports: DecodeError row=2 col=18.
func TestLoad_TypeMismatch(t *testing.T) {
	path := filepath.Join("..", "fixtures", "config", "type-mismatch.toml")
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("T-L7: filepath.Abs: %v", err)
	}

	_, errs := config.Load(abs)
	if len(errs) == 0 {
		t.Fatal("T-L7: expected 1+ errors, got 0")
	}
	foundSevError := false
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			foundSevError = true
			// The error message should contain something about type / decode.
			msg := strings.ToLower(e.Message)
			if !strings.Contains(msg, "bool") && !strings.Contains(msg, "string") &&
				!strings.Contains(msg, "type") && !strings.Contains(msg, "decode") &&
				!strings.Contains(msg, "cannot") {
				t.Errorf("T-L7: error message %q does not describe type mismatch", e.Message)
			}
		}
	}
	if !foundSevError {
		t.Errorf("T-L7: no SeverityError found; errs=%v", errs)
	}
}

// T-L8: Load on a file with a known-position error must capture exact Line and
// Column from pelletier. Verifies insurance #1 per-test.
//
// Fixture malformed.toml:
//
//	line 1: version = 1
//	line 2: [broken        ← pelletier: row=2, col=8 (expected "]")
//
// Per-test verification: pelletier DecodeError.Position() returns (2, 8).
func TestLoad_LineColumnExtraction(t *testing.T) {
	path := filepath.Join("..", "fixtures", "config", "malformed.toml")
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("T-L8: filepath.Abs: %v", err)
	}

	_, errs := config.Load(abs)
	if len(errs) == 0 {
		t.Fatal("T-L8: expected 1+ errors, got 0")
	}

	// Find the parse error with line/column info.
	const wantLine = 2
	const wantCol = 8
	found := false
	for _, e := range errs {
		if e.Severity == config.SeverityError && e.Line == wantLine && e.Column == wantCol {
			found = true
		}
	}
	if !found {
		t.Errorf("T-L8: no SeverityError with Line=%d Column=%d; errs=%v", wantLine, wantCol, errs)
	}

	// Also verify Path (current stub field; GREEN renames Path→File in errors.go).
	// After GREEN: change e.Path to e.File here.
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			if e.Path == "" {
				t.Errorf("T-L8: Error.Path/File is empty; want absolute path")
			}
		}
	}
}

// writeTempTOML writes content to a temp file and returns its path.
// The file is removed when the test ends.
func writeTempTOML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTempTOML: %v", err)
	}
	return p
}
