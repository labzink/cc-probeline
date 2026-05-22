package settingsfile_test

import (
	"errors"
	"testing"

	"github.com/labzink/cc-probeline/internal/settingsfile"
)

// defaultOpts returns a minimal valid InsertOpts for tests that do not need
// to customise specific fields.
func defaultOpts() settingsfile.InsertOpts {
	return settingsfile.InsertOpts{
		BinaryPath:      "/usr/local/bin/cc-probeline",
		RefreshInterval: 5,
		Padding:         0,
		Force:           false,
	}
}

// statusLineBlock is a helper that extracts the statusLine sub-map from s and
// calls t.Fatalf if it is absent or the wrong type.
func statusLineBlock(t *testing.T, s settingsfile.Settings) map[string]any {
	t.Helper()
	raw, ok := s["statusLine"]
	if !ok {
		t.Fatal("statusLine key absent in result Settings")
	}
	block, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("statusLine is not map[string]any; got %T", raw)
	}
	return block
}

// T-I1: empty Settings → result contains statusLine block with the supplied command.
// Concept §5.2.1.
func TestInsert_EmptySettings(t *testing.T) {
	opts := defaultOpts()
	got, err := settingsfile.InsertStatusLine(settingsfile.Settings{}, opts)
	if err != nil {
		t.Fatalf("InsertStatusLine error = %v; want nil", err)
	}
	block := statusLineBlock(t, got)
	if block["command"] != opts.BinaryPath {
		t.Fatalf("block.command = %q; want %q", block["command"], opts.BinaryPath)
	}
	if block["type"] != "command" {
		t.Fatalf("block.type = %q; want %q", block["type"], "command")
	}
}

// T-I2: input with unrelated keys (theme, permissions) → all three keys present,
// theme and permissions values untouched.
// Concept §5.2.2, §7.7.
func TestInsert_PreservesOtherKeys(t *testing.T) {
	input := settingsfile.Settings{
		"theme": "dark",
		"permissions": map[string]any{
			"allow": []any{"Bash(go *)"},
		},
	}
	opts := defaultOpts()
	got, err := settingsfile.InsertStatusLine(input, opts)
	if err != nil {
		t.Fatalf("InsertStatusLine error = %v; want nil", err)
	}

	// theme must be preserved.
	if got["theme"] != "dark" {
		t.Fatalf("theme = %q; want %q", got["theme"], "dark")
	}

	// permissions must be preserved.
	perms, ok := got["permissions"].(map[string]any)
	if !ok {
		t.Fatal("permissions key missing or wrong type")
	}
	allow, ok := perms["allow"].([]any)
	if !ok || len(allow) != 1 || allow[0] != "Bash(go *)" {
		t.Fatalf("permissions.allow not preserved; got %v", perms["allow"])
	}

	// statusLine must be present.
	_ = statusLineBlock(t, got)

	// Input must not have been mutated.
	if _, ok := input["statusLine"]; ok {
		t.Fatal("InsertStatusLine mutated the input map")
	}
}

// T-I3: Settings already contains our statusLine block with the same command →
// result is semantically identical to input (idempotency, no-op).
// Concept §5.2.3.
func TestInsert_IdempotentOurBlock(t *testing.T) {
	const binPath = "/usr/local/bin/cc-probeline"
	input := settingsfile.Settings{
		"statusLine": map[string]any{
			"type":            "command",
			"command":         binPath,
			"padding":         float64(0),
			"refreshInterval": float64(5),
		},
	}
	opts := settingsfile.InsertOpts{
		BinaryPath:      binPath,
		RefreshInterval: 5,
		Padding:         0,
		Force:           false,
	}
	got, err := settingsfile.InsertStatusLine(input, opts)
	if err != nil {
		t.Fatalf("InsertStatusLine error = %v; want nil", err)
	}
	block := statusLineBlock(t, got)

	// The returned block must carry the same values as input.
	if block["command"] != binPath {
		t.Fatalf("idempotent: command changed; got %q; want %q", block["command"], binPath)
	}
	if block["refreshInterval"] != float64(5) {
		t.Fatalf("idempotent: refreshInterval changed; got %v", block["refreshInterval"])
	}
}

// T-I4: Settings contains our block but with a different binary path →
// result block carries the new path.
// Concept §5.2.3 (update on path change).
func TestInsert_UpdatesOurBlock(t *testing.T) {
	oldPath := "/old/path/cc-probeline"
	newPath := "/new/path/cc-probeline"

	input := settingsfile.Settings{
		"statusLine": map[string]any{
			"type":            "command",
			"command":         oldPath,
			"padding":         float64(0),
			"refreshInterval": float64(5),
		},
	}
	opts := settingsfile.InsertOpts{
		BinaryPath:      newPath,
		RefreshInterval: 5,
		Padding:         0,
		Force:           false,
	}
	got, err := settingsfile.InsertStatusLine(input, opts)
	if err != nil {
		t.Fatalf("InsertStatusLine error = %v; want nil", err)
	}
	block := statusLineBlock(t, got)
	if block["command"] != newPath {
		t.Fatalf("block.command = %q; want %q", block["command"], newPath)
	}
}

// T-I5: Settings contains a foreign statusLine, Force=false →
// error is ErrForeignStatusLine, returned Settings is zero-value.
// Concept §5.2.4, §7.8.
func TestInsert_RefusesForeign_NoForce(t *testing.T) {
	input := settingsfile.Settings{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "/usr/local/bin/other-plugin",
		},
	}
	opts := settingsfile.InsertOpts{
		BinaryPath: "/usr/local/bin/cc-probeline",
		Force:      false,
	}
	_, err := settingsfile.InsertStatusLine(input, opts)
	if !errors.Is(err, settingsfile.ErrForeignStatusLine) {
		t.Fatalf("error = %v; want ErrForeignStatusLine", err)
	}
}

// T-I6: Settings contains a foreign statusLine, Force=true →
// no error, result block carries our command.
// Concept §5.2.4, §7.9.
func TestInsert_ForceOverwritesForeign(t *testing.T) {
	input := settingsfile.Settings{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "/usr/local/bin/other-plugin",
		},
	}
	opts := settingsfile.InsertOpts{
		BinaryPath: "/usr/local/bin/cc-probeline",
		Force:      true,
	}
	got, err := settingsfile.InsertStatusLine(input, opts)
	if err != nil {
		t.Fatalf("InsertStatusLine with Force=true error = %v; want nil", err)
	}
	block := statusLineBlock(t, got)
	if block["command"] != opts.BinaryPath {
		t.Fatalf("block.command = %q; want %q", block["command"], opts.BinaryPath)
	}
}

// T-I7: opts.RefreshInterval=0 → final block uses the default value 5.
// Concept §5.1 (default refreshInterval=5).
func TestInsert_DefaultRefreshInterval(t *testing.T) {
	opts := settingsfile.InsertOpts{
		BinaryPath:      "/usr/local/bin/cc-probeline",
		RefreshInterval: 0, // trigger default
		Padding:         0,
	}
	got, err := settingsfile.InsertStatusLine(settingsfile.Settings{}, opts)
	if err != nil {
		t.Fatalf("InsertStatusLine error = %v; want nil", err)
	}
	block := statusLineBlock(t, got)
	// refreshInterval may be stored as int or float64 depending on implementation.
	ri := block["refreshInterval"]
	switch v := ri.(type) {
	case int:
		if v != 5 {
			t.Fatalf("refreshInterval = %d; want 5", v)
		}
	case float64:
		if v != 5 {
			t.Fatalf("refreshInterval = %v; want 5", v)
		}
	default:
		t.Fatalf("refreshInterval type unexpected: %T = %v", ri, ri)
	}
}

// T-I8: opts.RefreshInterval=10 → final block uses 10.
// Concept §5.1.
func TestInsert_CustomRefreshInterval(t *testing.T) {
	opts := settingsfile.InsertOpts{
		BinaryPath:      "/usr/local/bin/cc-probeline",
		RefreshInterval: 10,
		Padding:         0,
	}
	got, err := settingsfile.InsertStatusLine(settingsfile.Settings{}, opts)
	if err != nil {
		t.Fatalf("InsertStatusLine error = %v; want nil", err)
	}
	block := statusLineBlock(t, got)
	ri := block["refreshInterval"]
	switch v := ri.(type) {
	case int:
		if v != 10 {
			t.Fatalf("refreshInterval = %d; want 10", v)
		}
	case float64:
		if v != 10 {
			t.Fatalf("refreshInterval = %v; want 10", v)
		}
	default:
		t.Fatalf("refreshInterval type unexpected: %T = %v", ri, ri)
	}
}

// T-I9: resulting block.padding == 0.
// Concept §5.1.
func TestInsert_PaddingZero(t *testing.T) {
	opts := defaultOpts()
	got, err := settingsfile.InsertStatusLine(settingsfile.Settings{}, opts)
	if err != nil {
		t.Fatalf("InsertStatusLine error = %v; want nil", err)
	}
	block := statusLineBlock(t, got)
	pad := block["padding"]
	switch v := pad.(type) {
	case int:
		if v != 0 {
			t.Fatalf("padding = %d; want 0", v)
		}
	case float64:
		if v != 0 {
			t.Fatalf("padding = %v; want 0", v)
		}
	default:
		t.Fatalf("padding type unexpected: %T = %v", pad, pad)
	}
}
