// Tests for scripts/install.sh (T-C1..T-C9).
//
// These tests are intentionally RED until Phase 5.c GREEN lands:
//   - scripts/install.sh is a 5.0 stub that does nothing except print a message.
//   - No binary copy, no settings.json write, no --help flag.
//
// Test setup: each test builds the cc-probeline binary once via TestMain
// (binaryPath is shared, defined in render_test.go). The script is run via
// "bash <path-to-install.sh>" with CC_PROBELINE_INSTALL_DEST and HOME
// pointing at t.TempDir() trees so no real filesystem is touched.
//
// T-C8 is a static grep check (no binary execution needed).
package cli_test

import (
	"crypto/md5"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// installShPath returns the absolute path to scripts/install.sh relative to
// the project root (two levels above the tests/cli/ package directory).
func installShPath(t *testing.T) string {
	t.Helper()
	root := projectRoot()
	p := filepath.Join(root, "scripts", "install.sh")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("installShPath: cannot stat %s: %v", p, err)
	}
	return p
}

// setupInstallSh copies scripts/install.sh into proj/scripts/install.sh and
// returns the paths to use in test commands.
// home  = isolated HOME directory (t.TempDir())
// proj  = directory holding scripts/ and the cc-probeline binary (t.TempDir())
// script = absolute path to the copied install.sh
// destBin = the expected install destination for the binary
func setupInstallSh(t *testing.T) (home, proj, script, destBin string) {
	t.Helper()

	home = t.TempDir()
	proj = t.TempDir()

	// Copy scripts/install.sh into proj/scripts/install.sh.
	scriptsDir := filepath.Join(proj, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatalf("setupInstallSh: MkdirAll scripts: %v", err)
	}
	src, err := os.ReadFile(installShPath(t))
	if err != nil {
		t.Fatalf("setupInstallSh: ReadFile install.sh: %v", err)
	}
	script = filepath.Join(scriptsDir, "install.sh")
	if err := os.WriteFile(script, src, 0755); err != nil {
		t.Fatalf("setupInstallSh: WriteFile install.sh: %v", err)
	}

	// Copy the pre-built binary (binaryPath from render_test.go TestMain) into proj.
	// install.sh locates the binary by looking in its parent dir (proj).
	binSrc, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("setupInstallSh: ReadFile binary: %v", err)
	}
	binDst := filepath.Join(proj, "cc-probeline")
	if err := os.WriteFile(binDst, binSrc, 0755); err != nil {
		t.Fatalf("setupInstallSh: WriteFile binary copy: %v", err)
	}

	destBin = filepath.Join(home, ".local", "bin", "cc-probeline")
	return home, proj, script, destBin
}

// runInstallSh executes "bash <script>" with the given extra args and env
// overrides. Returns combined output and the exit code.
func runInstallSh(t *testing.T, home, script, destBin string, extraEnv []string, args ...string) (out string, code int) {
	t.Helper()

	cmdArgs := append([]string{script}, args...)
	cmd := exec.Command("bash", cmdArgs...)

	// Base env: inherit current process env so that PATH etc. are available,
	// then override HOME and CC_PROBELINE_INSTALL_DEST.
	env := append(os.Environ(),
		"HOME="+home,
		"CC_PROBELINE_INSTALL_DEST="+destBin,
	)
	env = append(env, extraEnv...)
	cmd.Env = env

	combined, err := cmd.CombinedOutput()
	out = string(combined)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("runInstallSh: unexpected error type: %v", err)
		}
	}
	return out, code
}

// md5Hex returns the hex-encoded MD5 digest of data.
// Also consumed by install_ps1_test.go in the same package.
func md5Hex(data []byte) string {
	sum := md5.Sum(data)
	return fmt.Sprintf("%x", sum)
}

// T-C1: install.sh --help exits 0 and prints usage.
// Concept §3.1.1 (--help flag).
func TestInstallSh_HelpFlag(t *testing.T) {
	home, proj, script, destBin := setupInstallSh(t)
	_ = proj

	out, code := runInstallSh(t, home, script, destBin, nil, "--help")
	if code != 0 {
		t.Fatalf("T-C1: exit code = %d; want 0 (--help)\noutput: %s", code, out)
	}
	// GREEN must print a usage block; the stub does not have --help support.
	if !strings.Contains(strings.ToLower(out), "usage") {
		t.Fatalf("T-C1: stdout/stderr does not contain 'usage'; got: %q", out)
	}
}

// T-C2: basic install copies the binary and exits 0.
// Concept §3.1.1 steps 4-6.
func TestInstallSh_BasicInstall(t *testing.T) {
	home, _, script, destBin := setupInstallSh(t)

	out, code := runInstallSh(t, home, script, destBin, nil, "--no-settings")
	if code != 0 {
		t.Fatalf("T-C2: exit code = %d; want 0\noutput: %s", code, out)
	}

	info, err := os.Stat(destBin)
	if err != nil {
		t.Fatalf("T-C2: dest binary not found at %s: %v", destBin, err)
	}
	if info.Mode()&0111 == 0 {
		t.Fatalf("T-C2: dest binary is not executable, mode = %v", info.Mode())
	}
}

// T-C3: idempotency — running install.sh twice yields the same settings.json.
// Concept §3.1.3, §7.6.
func TestInstallSh_Idempotent(t *testing.T) {
	home, _, script, destBin := setupInstallSh(t)

	if _, code := runInstallSh(t, home, script, destBin, nil); code != 0 {
		t.Fatalf("T-C3: first run exit code = %d; want 0", code)
	}

	settPath := filepath.Join(home, ".claude", "settings.json")
	data1, err := os.ReadFile(settPath)
	if err != nil {
		t.Fatalf("T-C3: ReadFile after first run: %v", err)
	}
	hash1 := md5Hex(data1)

	if _, code := runInstallSh(t, home, script, destBin, nil); code != 0 {
		t.Fatalf("T-C3: second run exit code = %d; want 0", code)
	}

	data2, err := os.ReadFile(settPath)
	if err != nil {
		t.Fatalf("T-C3: ReadFile after second run: %v", err)
	}
	hash2 := md5Hex(data2)

	if hash1 != hash2 {
		t.Fatalf("T-C3: settings.json changed on second run (not idempotent): md5 %q -> %q", hash1, hash2)
	}
}

// T-C4: install preserves pre-existing keys (theme, etc.) and adds statusLine.
// Concept §7.7.
func TestInstallSh_PreservesOtherKeys(t *testing.T) {
	home, _, script, destBin := setupInstallSh(t)

	// Preseed settings.json with an unrelated key.
	writeSettings(t, home, map[string]any{
		"theme": "dark",
	})

	out, code := runInstallSh(t, home, script, destBin, nil)
	if code != 0 {
		t.Fatalf("T-C4: exit code = %d; want 0\noutput: %s", code, out)
	}

	got := readSettings(t, home)
	if got["theme"] != "dark" {
		t.Fatalf("T-C4: theme key not preserved; got %v", got["theme"])
	}
	block, ok := got["statusLine"].(map[string]any)
	if !ok {
		t.Fatalf("T-C4: statusLine absent or wrong type after install; settings: %v", got)
	}
	cmd, _ := block["command"].(string)
	if !strings.HasSuffix(cmd, "cc-probeline") {
		t.Fatalf("T-C4: statusLine.command does not end with cc-probeline; got: %q", cmd)
	}
}

// T-C5: install refuses a foreign statusLine and exits non-zero; file unchanged.
// Concept §5.2.4, §7.8.
func TestInstallSh_RefusesForeign(t *testing.T) {
	home, _, script, destBin := setupInstallSh(t)

	foreign := map[string]any{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "/usr/local/bin/other-plugin",
		},
	}
	writeSettings(t, home, foreign)

	before, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatalf("T-C5: ReadFile before: %v", err)
	}

	_, code := runInstallSh(t, home, script, destBin, nil)
	if code == 0 {
		t.Fatalf("T-C5: exit code = 0; want non-zero (should refuse foreign statusLine)")
	}

	after, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatalf("T-C5: ReadFile after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("T-C5: settings.json was modified when refusing foreign statusLine")
	}
}

// T-C6: --force with foreign statusLine exits 0, creates backup, writes our block.
// Concept §5.2.4, §7.9.
func TestInstallSh_ForceWithBackup(t *testing.T) {
	home, _, script, destBin := setupInstallSh(t)

	writeSettings(t, home, map[string]any{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "/usr/local/bin/other-plugin",
		},
	})

	out, code := runInstallSh(t, home, script, destBin, nil, "--force")
	if code != 0 {
		t.Fatalf("T-C6: exit code = %d; want 0 (--force)\noutput: %s", code, out)
	}

	// A backup file must exist in ~/.claude.
	entries, err := os.ReadDir(filepath.Join(home, ".claude"))
	if err != nil {
		t.Fatalf("T-C6: ReadDir .claude: %v", err)
	}
	hasBak := false
	for _, e := range entries {
		if strings.Contains(e.Name(), ".cc-probeline.bak.") {
			hasBak = true
			break
		}
	}
	if !hasBak {
		t.Fatalf("T-C6: no backup file found after --force install in %s/.claude",
			home)
	}

	// statusLine must now be ours.
	got := readSettings(t, home)
	block, ok := got["statusLine"].(map[string]any)
	if !ok {
		t.Fatalf("T-C6: statusLine absent or wrong type after --force install; settings: %v", got)
	}
	cmd, _ := block["command"].(string)
	if !strings.HasSuffix(cmd, "cc-probeline") {
		t.Fatalf("T-C6: statusLine.command does not end with cc-probeline after --force; got: %q", cmd)
	}
}

// T-C7: --no-settings copies binary but does not create settings.json.
// Concept §3.1.1, flag --no-settings.
func TestInstallSh_NoSettingsFlag(t *testing.T) {
	home, _, script, destBin := setupInstallSh(t)

	out, code := runInstallSh(t, home, script, destBin, nil, "--no-settings")
	if code != 0 {
		t.Fatalf("T-C7: exit code = %d; want 0\noutput: %s", code, out)
	}

	// Binary must exist.
	if _, err := os.Stat(destBin); err != nil {
		t.Fatalf("T-C7: dest binary not found at %s: %v", destBin, err)
	}

	// settings.json must NOT exist (we did not create it).
	sPath := settingsPath(home)
	if _, err := os.Stat(sPath); err == nil {
		t.Fatalf("T-C7: settings.json was created even with --no-settings; path: %s", sPath)
	}
}

// T-C8: static check that scripts/install.sh contains the "Unsupported:" string
// required by the OS detection block. Uses grep on the real source file.
// Concept §3.1.1 step 1.
func TestInstallSh_UnsupportedOSGrep(t *testing.T) {
	shPath := installShPath(t)
	data, err := os.ReadFile(shPath)
	if err != nil {
		t.Fatalf("T-C8: ReadFile install.sh: %v", err)
	}
	if !strings.Contains(string(data), "Unsupported:") {
		t.Fatalf("T-C8: scripts/install.sh does not contain 'Unsupported:' string; "+
			"OS detection block is missing or malformed (path: %s)", shPath)
	}
}

// T-C9: when dest dir is not in PATH, stdout contains a "not in PATH" warning.
// Concept §3.1.1 step 7.
func TestInstallSh_PathWarning(t *testing.T) {
	home, _, script, destBin := setupInstallSh(t)

	// Override PATH so that the dest dir is definitely absent.
	// Use a minimal PATH that still lets bash find basic utilities.
	safeEnv := []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"}

	out, code := runInstallSh(t, home, script, destBin, safeEnv, "--no-settings")
	if code != 0 {
		t.Fatalf("T-C9: exit code = %d; want 0 (PATH warning must not abort)\noutput: %s", code, out)
	}

	lc := strings.ToLower(out)
	if !strings.Contains(lc, "not in path") && !strings.Contains(lc, "not in $path") {
		t.Fatalf("T-C9: stdout/stderr does not contain 'not in PATH' warning; got: %q", out)
	}
}
