package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestConfigShow_ReflectsEffectiveConfig verifies that `cc-probeline config show`
// prints the effective config (cascade-resolved) as JSON, picking up an explicit
// config file via CC_PROBELINE_CONFIG and keeping defaults for omitted keys.
func TestConfigShow_ReflectsEffectiveConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	toml := `version = 1
[general]
table_rows = 7
no_color = true
mode = "super-compact"
[widgets]
git = false
`
	if err := os.WriteFile(cfgPath, []byte(toml), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout, stderr, code := run(t, []string{"CC_PROBELINE_CONFIG=" + cfgPath}, nil, "config", "show")
	if code != 0 {
		t.Fatalf("config show exit=%d, stderr=%s", code, stderr)
	}

	var got struct {
		General struct {
			TableRows int    `json:"table_rows"`
			NoColor   bool   `json:"no_color"`
			Mode      string `json:"mode"`
		} `json:"general"`
		Widgets struct {
			Git   bool `json:"git"`
			Model bool `json:"model"`
		} `json:"widgets"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("config show output is not valid JSON: %v\noutput:\n%s", err, stdout)
	}

	if got.General.TableRows != 7 {
		t.Errorf("table_rows: got %d, want 7", got.General.TableRows)
	}
	if !got.General.NoColor {
		t.Errorf("no_color: got %v, want true", got.General.NoColor)
	}
	if got.General.Mode != "super-compact" {
		t.Errorf("mode: got %q, want super-compact", got.General.Mode)
	}
	if got.Widgets.Git {
		t.Errorf("widgets.git: got %v, want false (explicitly set)", got.Widgets.Git)
	}
	if !got.Widgets.Model {
		t.Errorf("widgets.model: got %v, want true (default for omitted key)", got.Widgets.Model)
	}
}

// TestConfigShow_MissingShowArg returns a usage error (exit 64).
func TestConfigShow_MissingShowArg(t *testing.T) {
	_, _, code := run(t, nil, nil, "config")
	if code != 64 {
		t.Errorf("`config` without `show`: exit=%d, want 64", code)
	}
}
