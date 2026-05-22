// Tests for the "uninstall" subcommand of cc-probeline.
//
// These tests are intentionally RED until Phase 5.b GREEN lands:
//   - runUninstall() is a stub returning 0 (Phase 5.0 foundation).
//   - No backup, no settings.json modification occurs yet.
//
// TestMain and binaryPath are defined in render_test.go (same package cli_test).
package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// homeDir creates a minimal XDG-style home directory under t.TempDir()
// and sets HOME so that cc-probeline reads/writes settings from there.
// Returns the home path and a cleanup function that restores the original HOME.
func homeDir(t *testing.T) (string, func()) {
	t.Helper()
	home := t.TempDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("Setenv HOME: %v", err)
	}
	return home, func() { os.Setenv("HOME", origHome) } //nolint:errcheck
}

// claudeDir returns (and creates) the ~/.claude directory under home.
func claudeDir(t *testing.T, home string) string {
	t.Helper()
	d := filepath.Join(home, ".claude")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatalf("MkdirAll .claude: %v", err)
	}
	return d
}

// settingsPath returns ~/.claude/settings.json path under home.
func settingsPath(home string) string {
	return filepath.Join(home, ".claude", "settings.json")
}

// writeSettings marshals v and writes it to ~/.claude/settings.json.
func writeSettings(t *testing.T, home string, v any) {
	t.Helper()
	claudeDir(t, home)
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(settingsPath(home), data, 0o644); err != nil {
		t.Fatalf("WriteFile settings: %v", err)
	}
}

// readSettings reads and JSON-decodes ~/.claude/settings.json.
func readSettings(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatalf("ReadFile settings: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("json.Unmarshal settings: %v", err)
	}
	return out
}

// runUninstallCmd executes the binary with "uninstall" plus any extra args
// and returns stdout, stderr, and exit code.
func runUninstallCmd(t *testing.T, home string, extra ...string) (stdout, stderr string, code int) {
	t.Helper()
	args := append([]string{"uninstall"}, extra...)
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(), "HOME="+home)

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("cmd.Run: %v", err)
		}
	}
	return
}

// T-B1: No settings.json present → exit 0, message "nothing to uninstall".
func TestUninstall_NoSettings(t *testing.T) {
	home, cleanup := homeDir(t)
	defer cleanup()

	// Do NOT create .claude/settings.json — file must be absent.
	stdout, _, code := runUninstallCmd(t, home)

	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}
	if !strings.Contains(stdout, "nothing to uninstall") {
		t.Fatalf("stdout does not contain 'nothing to uninstall'; got: %q", stdout)
	}
}

// T-B2: settings.json contains our statusLine → deleted, backup created, exit 0.
func TestUninstall_OurBlock(t *testing.T) {
	home, cleanup := homeDir(t)
	defer cleanup()

	writeSettings(t, home, map[string]any{
		"theme": "dark",
		"statusLine": map[string]any{
			"type":            "command",
			"command":         filepath.Join(home, ".local", "bin", "cc-probeline"),
			"padding":         0,
			"refreshInterval": 5,
		},
	})

	_, _, code := runUninstallCmd(t, home)

	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}

	// statusLine must have been removed.
	got := readSettings(t, home)
	if _, ok := got["statusLine"]; ok {
		t.Fatal("statusLine still present after uninstall")
	}

	// Other keys must be preserved.
	if got["theme"] != "dark" {
		t.Fatalf("theme key not preserved; got %v", got["theme"])
	}

	// A backup file must exist.
	entries, err := os.ReadDir(filepath.Join(home, ".claude"))
	if err != nil {
		t.Fatalf("ReadDir .claude: %v", err)
	}
	hasBak := false
	for _, e := range entries {
		if strings.Contains(e.Name(), ".cc-probeline.bak.") {
			hasBak = true
			break
		}
	}
	if !hasBak {
		t.Fatal("no backup file found after uninstall")
	}
}

// T-B3: settings.json has a foreign statusLine → block is left intact, exit 0,
// message "leaving it alone".
func TestUninstall_ForeignBlock(t *testing.T) {
	home, cleanup := homeDir(t)
	defer cleanup()

	writeSettings(t, home, map[string]any{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "/usr/bin/other-plugin",
		},
	})

	// Snapshot content before uninstall.
	before, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatalf("ReadFile before: %v", err)
	}

	stdout, _, code := runUninstallCmd(t, home)

	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}
	if !strings.Contains(stdout, "leaving it alone") {
		t.Fatalf("stdout does not contain 'leaving it alone'; got: %q", stdout)
	}

	// File must be unchanged.
	after, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatalf("ReadFile after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("settings.json was modified for a foreign block")
	}
}

// T-B4: Round-trip — install (manual JSON write) then uninstall restores the file
// to its pre-install state (modulo whitespace-normalisation).
func TestUninstall_RoundTrip(t *testing.T) {
	home, cleanup := homeDir(t)
	defer cleanup()

	// Initial settings without statusLine.
	initial := map[string]any{
		"theme": "light",
		"permissions": map[string]any{
			"allow": []any{"Bash(go *)"},
		},
	}
	writeSettings(t, home, initial)

	// Simulate install: add our statusLine block manually.
	installed := map[string]any{
		"theme": "light",
		"permissions": map[string]any{
			"allow": []any{"Bash(go *)"},
		},
		"statusLine": map[string]any{
			"type":            "command",
			"command":         filepath.Join(home, ".local", "bin", "cc-probeline"),
			"padding":         0,
			"refreshInterval": 5,
		},
	}
	writeSettings(t, home, installed)

	// Uninstall.
	_, _, code := runUninstallCmd(t, home)
	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}

	// Read back and compare with initial (key-by-key, ignoring statusLine absence).
	got := readSettings(t, home)
	if _, ok := got["statusLine"]; ok {
		t.Fatal("statusLine present after round-trip uninstall")
	}
	if got["theme"] != "light" {
		t.Fatalf("theme not preserved; got %v", got["theme"])
	}
	perms, ok := got["permissions"].(map[string]any)
	if !ok {
		t.Fatal("permissions key missing after round-trip")
	}
	allow, ok := perms["allow"].([]any)
	if !ok || len(allow) != 1 || allow[0] != "Bash(go *)" {
		t.Fatalf("permissions.allow not preserved; got %v", perms["allow"])
	}
}

// T-B5: --dry-run flag → settings.json unchanged, exit 0.
func TestUninstall_DryRun(t *testing.T) {
	home, cleanup := homeDir(t)
	defer cleanup()

	writeSettings(t, home, map[string]any{
		"statusLine": map[string]any{
			"type":    "command",
			"command": filepath.Join(home, ".local", "bin", "cc-probeline"),
		},
	})

	before, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatalf("ReadFile before: %v", err)
	}

	_, _, code := runUninstallCmd(t, home, "--dry-run")

	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}

	after, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatalf("ReadFile after: %v", err)
	}

	if string(before) != string(after) {
		t.Fatal("--dry-run modified settings.json; it must not")
	}
}

// T-B6: After successful uninstall a .cc-probeline.bak.* file exists and is
// valid JSON.
func TestUninstall_BackupCreated(t *testing.T) {
	home, cleanup := homeDir(t)
	defer cleanup()

	writeSettings(t, home, map[string]any{
		"statusLine": map[string]any{
			"type":    "command",
			"command": filepath.Join(home, ".local", "bin", "cc-probeline"),
		},
	})

	_, _, code := runUninstallCmd(t, home)
	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}

	// Find backup file.
	entries, err := os.ReadDir(filepath.Join(home, ".claude"))
	if err != nil {
		t.Fatalf("ReadDir .claude: %v", err)
	}
	var bakPath string
	for _, e := range entries {
		if strings.Contains(e.Name(), ".cc-probeline.bak.") {
			bakPath = filepath.Join(home, ".claude", e.Name())
			break
		}
	}
	if bakPath == "" {
		t.Fatal("no backup file found after uninstall")
	}

	// Backup must be valid JSON.
	data, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatalf("ReadFile backup: %v", err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("backup file is not valid JSON: %v", err)
	}
}
