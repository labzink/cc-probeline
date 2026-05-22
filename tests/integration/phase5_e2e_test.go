//go:build integration

package integration_test

// phase5_e2e_test.go — end-to-end lifecycle tests for Phase 5 (install → render → uninstall).
//
// Run:
//
//	go test -tags=integration ./tests/integration/ -run TestPhase5 -v -count=1
//
// All tests use isolated HOME directories (t.TempDir()) and never touch the
// real ~/.claude/settings.json.
//
// Binary: built once per test package via phase5BinPath() which uses a
// package-level sync.Once. No TestMain conflict with render_integration_test.go
// (that file has no TestMain either).

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ─── Binary build (once per test run) ────────────────────────────────────────

var (
	phase5BinOnce sync.Once
	phase5Bin     string
	phase5BinErr  error
)

// phase5BinPath builds the cc-probeline binary once and returns its path.
// Subsequent calls return the cached result.
func phase5BinPath(t testing.TB) string {
	t.Helper()
	phase5BinOnce.Do(func() {
		root, err := findProjectRoot()
		if err != nil {
			phase5BinErr = fmt.Errorf("phase5BinPath: findProjectRoot: %w", err)
			return
		}
		dir, err := os.MkdirTemp("", "phase5-bin-*")
		if err != nil {
			phase5BinErr = fmt.Errorf("phase5BinPath: MkdirTemp: %w", err)
			return
		}
		bin := filepath.Join(dir, "cc-probeline")
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/cc-probeline/")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			phase5BinErr = fmt.Errorf("phase5BinPath: build failed: %v\n%s", err, out)
			return
		}
		phase5Bin = bin
	})
	if phase5BinErr != nil {
		t.Fatalf("phase5BinPath: %v", phase5BinErr)
	}
	return phase5Bin
}

// ─── Setup helpers ────────────────────────────────────────────────────────────

// phase5Setup creates an isolated proj dir that mirrors the layout install.sh
// expects: proj/cc-probeline (binary) and proj/scripts/install.sh.
// Returns (home, projDir, scriptPath).
func phase5Setup(t *testing.T) (home, projDir, scriptPath string) {
	t.Helper()

	root, err := findProjectRoot()
	if err != nil {
		t.Fatalf("phase5Setup: findProjectRoot: %v", err)
	}

	home = t.TempDir()
	projDir = t.TempDir()

	// Copy binary into projDir.
	srcBin := phase5BinPath(t)
	binData, err := os.ReadFile(srcBin)
	if err != nil {
		t.Fatalf("phase5Setup: ReadFile binary: %v", err)
	}
	dstBin := filepath.Join(projDir, "cc-probeline")
	if err := os.WriteFile(dstBin, binData, 0o755); err != nil {
		t.Fatalf("phase5Setup: WriteFile binary: %v", err)
	}

	// Copy scripts/install.sh into projDir/scripts/install.sh.
	scriptsDir := filepath.Join(projDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("phase5Setup: MkdirAll scripts: %v", err)
	}
	shData, err := os.ReadFile(filepath.Join(root, "scripts", "install.sh"))
	if err != nil {
		t.Fatalf("phase5Setup: ReadFile install.sh: %v", err)
	}
	scriptPath = filepath.Join(scriptsDir, "install.sh")
	if err := os.WriteFile(scriptPath, shData, 0o755); err != nil {
		t.Fatalf("phase5Setup: WriteFile install.sh: %v", err)
	}

	return home, projDir, scriptPath
}

// phase5Env returns an os.Environ() copy with HOME overridden to home.
func phase5Env(home string) []string {
	return append(os.Environ(), "HOME="+home)
}

// phase5DestBin returns the default install destination for a given home dir.
func phase5DestBin(home string) string {
	return filepath.Join(home, ".local", "bin", "cc-probeline")
}

// phase5SettingsPath returns the path to settings.json under home.
func phase5SettingsPath(home string) string {
	return filepath.Join(home, ".claude", "settings.json")
}

// phase5WriteSettings marshals v and writes it to home/.claude/settings.json.
func phase5WriteSettings(t *testing.T, home string, v any) {
	t.Helper()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("phase5WriteSettings: MkdirAll: %v", err)
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("phase5WriteSettings: Marshal: %v", err)
	}
	if err := os.WriteFile(phase5SettingsPath(home), data, 0o644); err != nil {
		t.Fatalf("phase5WriteSettings: WriteFile: %v", err)
	}
}

// phase5ReadSettings reads and JSON-decodes home/.claude/settings.json.
func phase5ReadSettings(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(phase5SettingsPath(home))
	if err != nil {
		t.Fatalf("phase5ReadSettings: ReadFile: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("phase5ReadSettings: Unmarshal: %v", err)
	}
	return out
}

// phase5MD5 returns the hex-encoded MD5 of the file at path.
func phase5MD5(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("phase5MD5: ReadFile %q: %v", path, err)
	}
	return fmt.Sprintf("%x", md5.Sum(data))
}

// loadPhase5Fixture returns the contents of the phase5 stdin fixture as string.
func loadPhase5Fixture(t *testing.T) string {
	t.Helper()
	root, err := findProjectRoot()
	if err != nil {
		t.Fatalf("loadPhase5Fixture: findProjectRoot: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "tests/fixtures/integration/phase5/short-cli.json"))
	if err != nil {
		t.Fatalf("loadPhase5Fixture: ReadFile: %v", err)
	}
	return string(data)
}

// runInstallSh runs bash <scriptPath> with default dest and HOME override.
// Extra args are appended after the script path.
func runInstallSh(t *testing.T, home, scriptPath string, extraArgs ...string) (stdout, stderr string, code int) {
	t.Helper()
	dest := phase5DestBin(home)
	args := append([]string{scriptPath, "--dest", dest}, extraArgs...)
	cmd := exec.Command("bash", args...)
	cmd.Env = phase5Env(home)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("runInstallSh: unexpected error type: %v", err)
		}
	}
	return
}

// runBin runs destBin with the given args; stdin is stdinData (may be nil).
func runBin(t *testing.T, home, destBin string, stdinData []byte, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(destBin, args...)
	cmd.Env = phase5Env(home)
	if stdinData != nil {
		cmd.Stdin = bytes.NewReader(stdinData)
	}
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("runBin: unexpected error type: %v", err)
		}
	}
	return
}

// ─── T-F1: full lifecycle ─────────────────────────────────────────────────────

// TestPhase5_LifecycleE2E verifies the complete install → render → uninstall
// lifecycle. §2.1 T-F1.
//
// Stages:
//  1. install via install.sh --dest <tmp>
//  2. check settings.json created
//  3. render with fixture stdin → exit 0, stdout non-empty, no "parse error"
//  4. uninstall via installed binary
//  5. check statusLine key removed from settings.json
func TestPhase5_LifecycleE2E(t *testing.T) {
	home, _, scriptPath := phase5Setup(t)
	dest := phase5DestBin(home)

	// Stage 1: install.
	stdout, stderr, code := runInstallSh(t, home, scriptPath)
	if code != 0 {
		t.Fatalf("T-F1 install: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// Stage 2: settings.json must exist.
	if _, err := os.Stat(phase5SettingsPath(home)); err != nil {
		t.Fatalf("T-F1: settings.json not created: %v", err)
	}

	// Stage 3: render.
	fixture := loadPhase5Fixture(t)
	rOut, rErr, rCode := runBin(t, home, dest, []byte(fixture))
	if rCode != 0 {
		t.Fatalf("T-F1 render: exit %d\nstdout: %s\nstderr: %s", rCode, rOut, rErr)
	}
	if strings.TrimSpace(rOut) == "" {
		t.Fatalf("T-F1 render: stdout is empty")
	}
	if strings.Contains(rOut, "parse error") {
		t.Fatalf("T-F1 render: stdout contains 'parse error': %q", rOut)
	}

	// Stage 4: uninstall.
	uOut, uErr, uCode := runBin(t, home, dest, nil, "uninstall")
	if uCode != 0 {
		t.Fatalf("T-F1 uninstall: exit %d\nstdout: %s\nstderr: %s", uCode, uOut, uErr)
	}

	// Stage 5: statusLine must be removed from settings.json.
	settings := phase5ReadSettings(t, home)
	if _, ok := settings["statusLine"]; ok {
		t.Fatalf("T-F1: statusLine still present after uninstall; settings: %v", settings)
	}
}

// ─── T-F2: install preserves user settings ────────────────────────────────────

// TestPhase5_InstallPreservesUserSettings verifies that a pre-seeded
// {"theme":"dark"} key survives the install cycle. §2.1 T-F2.
func TestPhase5_InstallPreservesUserSettings(t *testing.T) {
	home, _, scriptPath := phase5Setup(t)

	// Preseed settings.json with a user key.
	phase5WriteSettings(t, home, map[string]any{"theme": "dark"})

	stdout, stderr, code := runInstallSh(t, home, scriptPath)
	if code != 0 {
		t.Fatalf("T-F2 install: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	settings := phase5ReadSettings(t, home)

	// User key must survive.
	if settings["theme"] != "dark" {
		t.Fatalf("T-F2: theme key not preserved; got %v", settings["theme"])
	}

	// statusLine must be ours.
	block, ok := settings["statusLine"].(map[string]any)
	if !ok {
		t.Fatalf("T-F2: statusLine absent or wrong type; settings: %v", settings)
	}
	cmd, _ := block["command"].(string)
	if !strings.HasSuffix(cmd, "cc-probeline") {
		t.Fatalf("T-F2: statusLine.command does not end with cc-probeline: %q", cmd)
	}

	// Render must still work after install.
	dest := phase5DestBin(home)
	fixture := loadPhase5Fixture(t)
	rOut, rErr, rCode := runBin(t, home, dest, []byte(fixture))
	if rCode != 0 {
		t.Fatalf("T-F2 render: exit %d\nstdout: %s\nstderr: %s", rCode, rOut, rErr)
	}
}

// ─── T-F3: reinstall is idempotent ───────────────────────────────────────────

// TestPhase5_ReinstallNoDuplication verifies that running install.sh three
// times yields identical settings.json between the second and third runs
// (idempotency). §2.1 T-F3.
func TestPhase5_ReinstallNoDuplication(t *testing.T) {
	home, _, scriptPath := phase5Setup(t)
	settPath := phase5SettingsPath(home)

	// First install.
	if _, _, code := runInstallSh(t, home, scriptPath); code != 0 {
		t.Fatalf("T-F3: first install exit %d", code)
	}

	// Second install.
	if _, _, code := runInstallSh(t, home, scriptPath); code != 0 {
		t.Fatalf("T-F3: second install exit %d", code)
	}
	hash2 := phase5MD5(t, settPath)

	// Third install.
	if _, _, code := runInstallSh(t, home, scriptPath); code != 0 {
		t.Fatalf("T-F3: third install exit %d", code)
	}
	hash3 := phase5MD5(t, settPath)

	if hash2 != hash3 {
		t.Fatalf("T-F3: settings.json changed between 2nd and 3rd install (not idempotent): %q -> %q",
			hash2, hash3)
	}
}

// ─── T-F5: uninstall with no settings file ───────────────────────────────────

// TestPhase5_UninstallNoSettingsFile verifies that running uninstall in a
// clean HOME (no settings.json) exits 0 with a "nothing to uninstall" message.
// §2.1 T-F5.
func TestPhase5_UninstallNoSettingsFile(t *testing.T) {
	// Do NOT install — use the pre-built binary directly.
	bin := phase5BinPath(t)
	home := t.TempDir()

	// HOME has no .claude/settings.json.
	stdout, stderr, code := runBin(t, home, bin, nil, "uninstall")
	if code != 0 {
		t.Fatalf("T-F5 uninstall: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "nothing to uninstall") {
		t.Fatalf("T-F5: output does not contain 'nothing to uninstall'; got: %q", combined)
	}
}

// ─── T-F6: --force overwrites foreign statusLine ─────────────────────────────

// TestPhase5_ForceOverwritesForeign verifies that install.sh --force:
//  1. exits 0 even when a foreign statusLine is present
//  2. creates a backup file
//  3. writes our statusLine block
//
// §2.1 T-F6.
func TestPhase5_ForceOverwritesForeign(t *testing.T) {
	home, _, scriptPath := phase5Setup(t)

	// Preseed a foreign statusLine.
	phase5WriteSettings(t, home, map[string]any{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "/usr/local/bin/other-plugin",
		},
	})

	stdout, stderr, code := runInstallSh(t, home, scriptPath, "--force")
	if code != 0 {
		t.Fatalf("T-F6 install --force: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// A backup file must exist in ~/.claude.
	claudeDir := filepath.Join(home, ".claude")
	entries, err := os.ReadDir(claudeDir)
	if err != nil {
		t.Fatalf("T-F6: ReadDir %s: %v", claudeDir, err)
	}
	hasBak := false
	for _, e := range entries {
		if strings.Contains(e.Name(), ".cc-probeline.bak.") {
			hasBak = true
			break
		}
	}
	if !hasBak {
		t.Fatalf("T-F6: no backup file found in %s after --force install", claudeDir)
	}

	// statusLine must be ours.
	settings := phase5ReadSettings(t, home)
	block, ok := settings["statusLine"].(map[string]any)
	if !ok {
		t.Fatalf("T-F6: statusLine absent or wrong type after --force; settings: %v", settings)
	}
	cmd, _ := block["command"].(string)
	if !strings.HasSuffix(cmd, "cc-probeline") {
		t.Fatalf("T-F6: statusLine.command does not end with cc-probeline after --force: %q", cmd)
	}
}

// ─── T-F7: PATH warning when dest is not in $PATH ────────────────────────────

// TestPhase5_PathWarningOnNonPath verifies that install.sh emits a "not in PATH"
// warning when the destination directory is absent from $PATH. §2.1 T-F7.
func TestPhase5_PathWarningOnNonPath(t *testing.T) {
	home, _, scriptPath := phase5Setup(t)

	// Use a dest path that is guaranteed to be outside PATH.
	// Override PATH to a minimal set that lets bash find basic utilities,
	// explicitly excluding home/.local/bin.
	dest := phase5DestBin(home)
	args := []string{scriptPath, "--dest", dest, "--no-settings"}
	cmd := exec.Command("bash", args...)
	cmd.Env = append(
		phase5Env(home),
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin", // minimal PATH, home/.local/bin absent
	)
	var outBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf // combine for warning check
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("T-F7: unexpected error: %v", err)
		}
	}
	if code != 0 {
		t.Fatalf("T-F7: exit %d (PATH warning must not abort install): output: %s", code, outBuf.String())
	}

	out := strings.ToLower(outBuf.String())
	if !strings.Contains(out, "not in path") && !strings.Contains(out, "not in $path") {
		t.Fatalf("T-F7: output does not contain 'not in PATH' warning; got: %q", outBuf.String())
	}
}
