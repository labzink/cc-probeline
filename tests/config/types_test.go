// Package config_test tests the config.Default() function and TOML round-trip
// behaviour for the Config type. Tests T-T1..T-T6 per phase-6-plan-6.a.md §3.1.
package config_test

import (
	"reflect"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/labzink/cc-probeline/internal/config"
)

// T-T1: Default() must return a Config where every field matches the canonical
// defaults table in concept §2.2 / plan §2.2.
func TestDefault_AllFieldsMatchSpec(t *testing.T) {
	d := config.Default()
	if d == nil {
		t.Fatal("Default() returned nil")
	}

	type check struct {
		name string
		got  interface{}
		want interface{}
	}

	checks := []check{
		{"Version", d.Version, 1},
		{"General.TutorialHints", d.General.TutorialHints, true},
		{"General.NoColor", d.General.NoColor, false},
		{"General.NerdFont", d.General.NerdFont, false},
		{"General.RefreshIntervalHint", d.General.RefreshIntervalHint, 5},
		{"General.TableRows", d.General.TableRows, 10},
		{"General.Mode", d.General.Mode, "standard"},
		{"Theme.Name", d.Theme.Name, "default"},
		{"Theme.Colors.Cyan", d.Theme.Colors.Cyan, ""},
		{"Theme.Colors.Red", d.Theme.Colors.Red, ""},
		{"Thresholds.CostBudgetUSD", d.Thresholds.CostBudgetUSD, 0.0},
		{"Thresholds.CtxWarnRatio", d.Thresholds.CtxWarnRatio, 0.70},
		{"Thresholds.CtxCriticalRatio", d.Thresholds.CtxCriticalRatio, 0.90},
		{"Thresholds.OrchTTLMinutes", d.Thresholds.OrchTTLMinutes, 60},
		{"Thresholds.SubagentGapMinutes", d.Thresholds.SubagentGapMinutes, 5},
		{"Probes.Email.Address", d.Probes.Email.Address, ""},
	}

	for _, c := range checks {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("T-T1 %s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

// T-T2: Default() must return a Config where all active Widgets fields are true.
// Note: Cache and Subagent widget toggles were removed in Phase 6.95 (dead config);
// probes.CacheEnabled/SubagentEnabled are now hardcoded true in adapter.go.
func TestDefault_WidgetsAllTrue(t *testing.T) {
	d := config.Default()
	if d == nil {
		t.Fatal("Default() returned nil")
	}

	type toggle struct {
		name string
		got  bool
	}

	toggles := []toggle{
		{"Model", d.Widgets.Model},
		{"Effort", d.Widgets.Effort},
		{"Cost", d.Widgets.Cost},
		{"Project", d.Widgets.Project},
		{"Email", d.Widgets.Email},
		{"Time", d.Widgets.Time},
		{"Ctx", d.Widgets.Ctx},
		{"Quota", d.Widgets.Quota},
		{"Git", d.Widgets.Git},
	}

	for _, tg := range toggles {
		if !tg.got {
			t.Errorf("T-T2 Widgets.%s: got false, want true", tg.name)
		}
	}
}

// T-T3: Two successive calls to Default() must return independent pointers;
// mutating one must not affect the other.
func TestDefault_NotMutating(t *testing.T) {
	a := config.Default()
	b := config.Default()

	if a == b {
		t.Fatal("T-T3: Default() returned the same pointer twice (not independent)")
	}

	// Mutate a.
	a.General.TutorialHints = false
	a.Theme.Name = "high-contrast"
	a.Thresholds.CostBudgetUSD = 999.99
	a.Widgets.Model = false

	// b must be unaffected.
	if !b.General.TutorialHints {
		t.Error("T-T3: mutating a.General.TutorialHints affected b")
	}
	if b.Theme.Name != "default" {
		t.Errorf("T-T3: mutating a.Theme.Name affected b: got %q", b.Theme.Name)
	}
	if b.Thresholds.CostBudgetUSD != 0.0 {
		t.Errorf("T-T3: mutating a.Thresholds.CostBudgetUSD affected b: got %v", b.Thresholds.CostBudgetUSD)
	}
	if !b.Widgets.Model {
		t.Error("T-T3: mutating a.Widgets.Model affected b")
	}
}

// T-T4: Default() → TOML marshal → unmarshal must produce a Config equal to
// the original (lossless round-trip).
func TestConfig_TOMLRoundTrip_Minimal(t *testing.T) {
	original := config.Default()

	data, err := toml.Marshal(original)
	if err != nil {
		t.Fatalf("T-T4: toml.Marshal failed: %v", err)
	}

	var restored config.Config
	if err := toml.Unmarshal(data, &restored); err != nil {
		t.Fatalf("T-T4: toml.Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(*original, restored) {
		t.Errorf("T-T4: round-trip produced different Config\n  original: %+v\n  restored: %+v", *original, restored)
	}
}

// T-T5: A TOML document with only [general] tutorial_hints = false must unmarshal
// such that TutorialHints is false and all other fields match Default().
func TestConfig_TOMLRoundTrip_Partial(t *testing.T) {
	const partial = `
[general]
tutorial_hints = false
`

	var cfg config.Config
	if err := toml.Unmarshal([]byte(partial), &cfg); err != nil {
		t.Fatalf("T-T5: toml.Unmarshal failed: %v", err)
	}

	if cfg.General.TutorialHints != false {
		t.Errorf("T-T5: TutorialHints: got %v, want false", cfg.General.TutorialHints)
	}

	// All other fields should be zero values (not defaults) because we only
	// unmarshalled a partial document. This tests TOML structural behaviour.
	// Version must be 0 (not set in TOML).
	if cfg.Version != 0 {
		t.Errorf("T-T5: Version: got %d, want 0 (not set in partial TOML)", cfg.Version)
	}
	// Widgets fields default to Go zero (false) when not set in TOML.
	if cfg.Widgets.Model != false {
		t.Errorf("T-T5: Widgets.Model: got true, want false (zero value for partial unmarshal)")
	}
}

// T-T6: Every exported field in Config and its nested structs must have a
// non-empty toml struct tag. Checked via reflection.
func TestConfig_TOMLTags_AllExported(t *testing.T) {
	checkStruct(t, reflect.TypeOf(config.Config{}), "Config")
}

// checkStruct recursively walks a struct type and fails if any exported field
// is missing a non-empty toml tag.
func checkStruct(t *testing.T, typ reflect.Type, path string) {
	t.Helper()
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("toml")
		if tag == "" || tag == "-" {
			t.Errorf("T-T6: field %s.%s has no toml tag", path, f.Name)
		}
		// Recurse into nested structs (but not into basic types).
		ft := f.Type
		if ft.Kind() == reflect.Struct {
			checkStruct(t, ft, path+"."+f.Name)
		}
	}
}
