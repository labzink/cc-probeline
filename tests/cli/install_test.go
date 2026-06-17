// Tests for the "install --merge-settings" subcommand of cc-probeline.
//
// These tests are intentionally RED until Phase 5.e GREEN lands:
//   - runInstall() does not yet exist.
//   - InsertStatusLine is a stub returning an error.
//
// TestMain and binaryPath are defined in render_test.go (same package cli_test).
package cli_test

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runInstallCmd executes the binary with "install" plus any extra args and
// returns stdout, stderr, and the exit code.
func runInstallCmd(t *testing.T, home string, extra ...string) (stdout, stderr string, code int) {
	t.Helper()
	args := append([]string{"install"}, extra...)
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"))

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

// md5File returns the hex-encoded MD5 digest of the file at path.
func md5File(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("md5File ReadFile %q: %v", path, err)
	}
	return fmt.Sprintf("%x", md5.Sum(data))
}

// T-E1: empty HOME/.claude → install creates settings.json with our statusLine block,
// exit 0.
// Concept §5.2.1.
func TestInstall_NoSettingsFile(t *testing.T) {
	home := homeDir(t)

	// Do NOT create .claude/settings.json.

	_, _, code := runInstallCmd(t, home, "--merge-settings", "--binary-path", binaryPath)
	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}

	got := readSettings(t, home)
	block, ok := got["statusLine"].(map[string]any)
	if !ok {
		t.Fatal("statusLine absent or wrong type after install")
	}
	if block["type"] != "command" {
		t.Fatalf("block.type = %q; want %q", block["type"], "command")
	}
}

// T-E2: running install twice produces the same settings.json (idempotency).
// Second run must not change the file content.
// Concept §7.6, §5.2.3.
func TestInstall_Idempotent(t *testing.T) {
	home := homeDir(t)

	args := []string{"--merge-settings", "--binary-path", binaryPath}

	_, _, code := runInstallCmd(t, home, args...)
	if code != 0 {
		t.Fatalf("first install exit code = %d; want 0", code)
	}

	path := settingsPath(home)
	hashAfterFirst := md5File(t, path)

	_, _, code = runInstallCmd(t, home, args...)
	if code != 0 {
		t.Fatalf("second install exit code = %d; want 0", code)
	}

	hashAfterSecond := md5File(t, path)
	if hashAfterFirst != hashAfterSecond {
		t.Fatalf("settings.json changed on second install (not idempotent): md5 %q -> %q",
			hashAfterFirst, hashAfterSecond)
	}
}

// T-E3: settings.json pre-seeded with {theme, permissions} → after install:
// theme and permissions intact, statusLine is ours.
// Concept §7.7, §5.2.2.
func TestInstall_PreservesOtherKeys(t *testing.T) {
	home := homeDir(t)

	writeSettings(t, home, map[string]any{
		"theme": "dark",
		"permissions": map[string]any{
			"allow": []any{"Bash(go *)"},
		},
	})

	_, _, code := runInstallCmd(t, home, "--merge-settings", "--binary-path", binaryPath)
	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}

	got := readSettings(t, home)

	if got["theme"] != "dark" {
		t.Fatalf("theme = %q; want %q", got["theme"], "dark")
	}

	perms, ok := got["permissions"].(map[string]any)
	if !ok {
		t.Fatal("permissions key missing or wrong type")
	}
	allow, _ := perms["allow"].([]any)
	if len(allow) != 1 || allow[0] != "Bash(go *)" {
		t.Fatalf("permissions.allow not preserved; got %v", perms["allow"])
	}

	block, ok := got["statusLine"].(map[string]any)
	if !ok {
		t.Fatal("statusLine absent or wrong type")
	}
	cmd, _ := block["command"].(string)
	if !strings.HasSuffix(cmd, "cc-probeline") {
		t.Fatalf("statusLine.command does not end with cc-probeline: %q", cmd)
	}
}

// T-E4: settings.json pre-seeded with foreign statusLine → exit 2,
// stderr contains "non-cc-probeline", settings.json unchanged.
// Concept §7.8, §5.2.4.
func TestInstall_RefusesForeign(t *testing.T) {
	home := homeDir(t)

	initial := map[string]any{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "/usr/local/bin/other-plugin",
		},
	}
	writeSettings(t, home, initial)

	before, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatalf("ReadFile before: %v", err)
	}

	_, stderr, code := runInstallCmd(t, home, "--merge-settings", "--binary-path", binaryPath)

	if code != 2 {
		t.Fatalf("exit code = %d; want 2", code)
	}
	if !strings.Contains(stderr, "non-cc-probeline") {
		t.Fatalf("stderr does not contain 'non-cc-probeline'; got: %q", stderr)
	}

	after, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatalf("ReadFile after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("settings.json was modified when refusing foreign statusLine")
	}

	// No backup file must be created when refusing a foreign statusLine (F3).
	baks, err := filepath.Glob(filepath.Join(home, ".claude", ".cc-probeline.bak.*"))
	if err != nil {
		t.Fatalf("Glob backup: %v", err)
	}
	if len(baks) != 0 {
		t.Fatalf("backup file created on foreign refuse (want 0): %v", baks)
	}
}

// T-E5: settings.json pre-seeded with foreign statusLine + --force →
// exit 0, backup file exists, new statusLine is ours.
// Concept §7.9, §5.2.4.
func TestInstall_ForceWithBackup(t *testing.T) {
	home := homeDir(t)

	writeSettings(t, home, map[string]any{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "/usr/local/bin/other-plugin",
		},
	})

	_, _, code := runInstallCmd(t, home,
		"--merge-settings", "--binary-path", binaryPath, "--force")
	if code != 0 {
		t.Fatalf("exit code = %d; want 0 with --force", code)
	}

	// A .cc-probeline.bak.* file must exist.
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
		t.Fatal("no backup file found after --force install")
	}

	// statusLine must now be ours.
	got := readSettings(t, home)
	block, ok := got["statusLine"].(map[string]any)
	if !ok {
		t.Fatal("statusLine absent or wrong type after --force install")
	}
	cmd, _ := block["command"].(string)
	if !strings.HasSuffix(cmd, "cc-probeline") {
		t.Fatalf("statusLine.command does not end with cc-probeline after --force: %q", cmd)
	}
}

// T-E6: --refresh-interval 7 → final block has refreshInterval=7.
// Concept §5.1.
func TestInstall_RefreshIntervalFlag(t *testing.T) {
	home := homeDir(t)

	_, _, code := runInstallCmd(t, home,
		"--merge-settings", "--binary-path", binaryPath, "--refresh-interval", "7")
	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}

	// Read back and decode refreshInterval; it arrives as float64 from JSON.
	data, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatalf("ReadFile settings: %v", err)
	}
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	block, ok := s["statusLine"].(map[string]any)
	if !ok {
		t.Fatal("statusLine absent or wrong type")
	}
	ri, _ := block["refreshInterval"].(float64)
	if ri != 7 {
		t.Fatalf("refreshInterval = %v; want 7", ri)
	}
}

// T-E7: --binary-path /custom/path → final block command=/custom/path.
// Concept §5.1.
func TestInstall_BinaryPathFlag(t *testing.T) {
	home := homeDir(t)

	customPath := "/custom/path/cc-probeline"
	_, _, code := runInstallCmd(t, home, "--merge-settings", "--binary-path", customPath)
	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}

	got := readSettings(t, home)
	block, ok := got["statusLine"].(map[string]any)
	if !ok {
		t.Fatal("statusLine absent or wrong type")
	}
	if block["command"] != customPath {
		t.Fatalf("block.command = %q; want %q", block["command"], customPath)
	}
}

// T-E8: no --binary-path flag → command is the absolute path of the binary
// (os.Executable semantics: must end with "cc-probeline").
// Concept §5.1, §3.e plan §3.
func TestInstall_DefaultBinaryPath(t *testing.T) {
	home := homeDir(t)

	// Run without --binary-path; the binary resolves itself via os.Executable.
	_, _, code := runInstallCmd(t, home, "--merge-settings")
	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}

	got := readSettings(t, home)
	block, ok := got["statusLine"].(map[string]any)
	if !ok {
		t.Fatal("statusLine absent or wrong type")
	}
	cmd, _ := block["command"].(string)
	if !strings.HasSuffix(cmd, "cc-probeline") {
		t.Fatalf("block.command does not end with cc-probeline: %q", cmd)
	}
	if !filepath.IsAbs(cmd) {
		t.Fatalf("block.command is not an absolute path: %q", cmd)
	}
}

// T-E10: foreign statusLine + --force → install-state.json is written under
// XDG_CONFIG_HOME with the path of the backup that captured the user's
// previous statusLine. This file drives the restore step in uninstall.
func TestInstall_ForeignWithForce_WritesInstallState(t *testing.T) {
	home := homeDir(t)

	writeSettings(t, home, map[string]any{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "/usr/local/bin/other-plugin",
		},
	})

	_, _, code := runInstallCmd(t, home,
		"--merge-settings", "--binary-path", binaryPath, "--force")
	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}

	statePath := filepath.Join(home, ".config", "cc-probeline", "install-state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("install-state.json not created at %s: %v", statePath, err)
	}
	var st map[string]any
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("install-state.json not valid JSON: %v", err)
	}
	bak, _ := st["pre_install_backup"].(string)
	if bak == "" {
		t.Fatalf("pre_install_backup missing from state: %v", st)
	}
	if _, err := os.Stat(bak); err != nil {
		t.Fatalf("recorded backup file does not exist: %s: %v", bak, err)
	}
}

// T-E11: clean install (no prior statusLine) does NOT write install-state.json
// — there is nothing to restore on uninstall.
func TestInstall_CleanInstall_NoStateFile(t *testing.T) {
	home := homeDir(t)

	_, _, code := runInstallCmd(t, home, "--merge-settings", "--binary-path", binaryPath)
	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}

	statePath := filepath.Join(home, ".config", "cc-probeline", "install-state.json")
	if _, err := os.Stat(statePath); err == nil {
		t.Fatalf("install-state.json must not be created on clean install: %s exists", statePath)
	}
}

// T-E9: "cc-probeline install" without --merge-settings → exit 2,
// stdout/stderr contains "Phase 7" hint.
// Concept §2.1.1, plan §3.
func TestInstall_NoMergeSettingsFlag(t *testing.T) {
	home := homeDir(t)

	stdout, stderr, code := runInstallCmd(t, home /* no --merge-settings */)
	if code != 2 {
		t.Fatalf("exit code = %d; want 2", code)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "Phase 7") && !strings.Contains(combined, "phase 7") {
		t.Fatalf("output does not contain Phase 7 hint; stdout=%q stderr=%q", stdout, stderr)
	}
}

// --if-absent, empty settings → wire normally, exit 0. (v0.1.3 universal
// channel wiring: the brew postflight / scoop post_install path.)
func TestInstall_IfAbsent_NoStatusLine_Wires(t *testing.T) {
	home := homeDir(t)

	_, _, code := runInstallCmd(t, home, "--merge-settings", "--binary-path", binaryPath, "--if-absent")
	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}
	got := readSettings(t, home)
	block, ok := got["statusLine"].(map[string]any)
	if !ok {
		t.Fatal("statusLine absent after --if-absent install on empty settings")
	}
	if cmd, _ := block["command"].(string); !strings.HasSuffix(cmd, "cc-probeline") {
		t.Fatalf("statusLine.command does not end with cc-probeline: %q", cmd)
	}
}

// --if-absent with an existing FOREIGN statusLine → exit 0 (does not fail the
// package install), settings.json unchanged, hint printed. This is the safety
// guarantee that a `brew install` never clobbers the user's own status line.
func TestInstall_IfAbsent_ExistingForeign_LeftUntouched(t *testing.T) {
	home := homeDir(t)

	writeSettings(t, home, map[string]any{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "/usr/local/bin/other-plugin",
		},
	})
	before, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatalf("ReadFile before: %v", err)
	}

	stdout, _, code := runInstallCmd(t, home, "--merge-settings", "--binary-path", binaryPath, "--if-absent")
	if code != 0 {
		t.Fatalf("exit code = %d; want 0 (package install must not fail)", code)
	}
	after, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatalf("ReadFile after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("settings.json was modified by --if-absent when a statusLine already existed")
	}
	if !strings.Contains(stdout, "already configured") {
		t.Fatalf("stdout does not contain the 'already configured' hint; got: %q", stdout)
	}
}
