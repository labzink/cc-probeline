// Package cmd_test exercises the cc-probeline --check subcommand via os/exec.
//
// The binary is compiled once in TestMain and shared across all sub-tests.
// Each test gets its own hermetic HOME directory via t.TempDir() so the real
// ~/.claude is never read or written.
package cmd_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// binaryPath holds the path to the compiled binary built in TestMain.
var binaryPath string

// TestMain builds the cc-probeline binary once, then runs all tests.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "cc-probeline-cmd-test-*")
	if err != nil {
		panic("TestMain: MkdirTemp: " + err.Error())
	}
	defer os.RemoveAll(dir) //nolint:errcheck

	binaryPath = filepath.Join(dir, "cc-probeline")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/cc-probeline/")
	cmd.Dir = projectRoot()
	if out, err := cmd.CombinedOutput(); err != nil {
		panic("TestMain: build failed:\n" + string(out))
	}

	os.Exit(m.Run())
}

// projectRoot walks up from the test directory to find the go.mod root.
func projectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		panic("projectRoot: Getwd: " + err.Error())
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("projectRoot: go.mod not found")
		}
		dir = parent
	}
}

// runCheckCmd executes cc-probeline --check with the given home directory and
// returns stdout, stderr, and the exit code.
func runCheckCmd(t *testing.T, home string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(binaryPath, "--check")
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

// writeSettingsJSON marshals v and writes it to HOME/.claude/settings.json.
func writeSettingsJSON(t *testing.T, home string, v any) {
	t.Helper()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll .claude: %v", err)
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile settings.json: %v", err)
	}
}

// T-C1: settings.json points to an existing executable (the test binary itself)
// that responds to --version with exit 0 → exit 0 + "OK" in stdout.
func TestCheck_ValidInstall(t *testing.T) {
	home := t.TempDir()
	// binaryPath is the compiled cc-probeline binary; it handles --version.
	writeSettingsJSON(t, home, map[string]any{
		"statusLine": map[string]any{
			"type":    "command",
			"command": binaryPath,
		},
	})

	stdout, stderr, code := runCheckCmd(t, home)
	if code != 0 {
		t.Fatalf("exit code = %d; want 0\nstdout=%q\nstderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "OK") {
		t.Fatalf("stdout does not contain 'OK'; got %q", stdout)
	}
}

// T-C2: settings.json does not exist → exit 1 + "settings not found" in stderr.
func TestCheck_MissingSettings(t *testing.T) {
	home := t.TempDir()
	// Do NOT create .claude/settings.json.

	stdout, stderr, code := runCheckCmd(t, home)
	if code != 1 {
		t.Fatalf("exit code = %d; want 1\nstdout=%q\nstderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "settings not found") {
		t.Fatalf("stderr does not contain 'settings not found'; got %q", stderr)
	}
}

// T-C3: settings.json exists but has no statusLine block → exit 1 +
// "statusLine not configured" in stderr.
func TestCheck_NoStatusLine(t *testing.T) {
	home := t.TempDir()
	writeSettingsJSON(t, home, map[string]any{
		"theme": "dark",
	})

	stdout, stderr, code := runCheckCmd(t, home)
	if code != 1 {
		t.Fatalf("exit code = %d; want 1\nstdout=%q\nstderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "statusLine not configured") {
		t.Fatalf("stderr does not contain 'statusLine not configured'; got %q", stderr)
	}
}

// T-C4: statusLine.command points to a non-existent path → exit 1 +
// "binary not found" in stderr.
func TestCheck_BinaryMissing(t *testing.T) {
	home := t.TempDir()
	writeSettingsJSON(t, home, map[string]any{
		"statusLine": map[string]any{
			"type":    "command",
			"command": filepath.Join(home, "no-such-binary"),
		},
	})

	stdout, stderr, code := runCheckCmd(t, home)
	if code != 1 {
		t.Fatalf("exit code = %d; want 1\nstdout=%q\nstderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "binary not found") {
		t.Fatalf("stderr does not contain 'binary not found'; got %q", stderr)
	}
}

// T-C5: after running --check, settings.json is byte-for-byte unchanged
// (read-only guarantee).
func TestCheck_ReadOnly(t *testing.T) {
	home := t.TempDir()
	writeSettingsJSON(t, home, map[string]any{
		"statusLine": map[string]any{
			"type":    "command",
			"command": binaryPath,
		},
	})

	settingsFile := filepath.Join(home, ".claude", "settings.json")
	before, err := os.ReadFile(settingsFile)
	if err != nil {
		t.Fatalf("ReadFile before: %v", err)
	}

	_, _, _ = runCheckCmd(t, home) // ignore exit code — we only care about file mutation

	after, err := os.ReadFile(settingsFile)
	if err != nil {
		t.Fatalf("ReadFile after: %v", err)
	}

	if !bytes.Equal(before, after) {
		t.Fatalf("settings.json was modified by --check (byte diff detected)")
	}
}
