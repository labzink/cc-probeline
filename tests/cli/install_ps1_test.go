// Tests for scripts/install.ps1 — Phase 5.d.
//
// T-D1..T-D4 are Windows-only (require pwsh or powershell).
// T-D5..T-D6 are cross-platform static checks (file content / BOM).
//
// On macOS/Linux: T-D1..T-D4 are SKIPPED; T-D5 is expected to FAIL on the
// 5.0 stub (required strings not yet present); T-D6 PASS or FAIL depending
// on whether the stub carries a UTF-8 BOM.
package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// scriptPath returns the absolute path to scripts/install.ps1 relative to
// the test binary's working directory (tests/cli → ../../scripts/install.ps1).
func ps1ScriptPath(t *testing.T) string {
	t.Helper()
	root := projectRoot()
	p := filepath.Join(root, "scripts", "install.ps1")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("install.ps1 not found at %q: %v", p, err)
	}
	return p
}

// pwshBin returns the first available PowerShell executable name.
// Prefers "pwsh" (PowerShell Core), falls back to "powershell" (Windows built-in).
// Must only be called on Windows.
func pwshBin(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("pwsh"); err == nil {
		return "pwsh"
	}
	if _, err := exec.LookPath("powershell"); err == nil {
		return "powershell"
	}
	t.Skip("neither pwsh nor powershell found in PATH")
	return ""
}

// T-D1: PowerShell syntax check via Test-ScriptFile.
// Windows-only: requires pwsh or powershell.
func TestInstallPs1_SyntaxStatic(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	shell := pwshBin(t)
	script := ps1ScriptPath(t)

	// Test-ScriptFile validates syntax without executing.
	cmd := exec.Command(shell, "-NoProfile", "-NonInteractive", "-Command",
		"Test-ScriptFile -Path '"+script+"'")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Test-ScriptFile failed:\n%s", string(out))
	}
}

// T-D2: Help flag prints usage text.
// Windows-only: runs install.ps1 -? and expects usage-like output.
func TestInstallPs1_HelpFlag(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	shell := pwshBin(t)
	script := ps1ScriptPath(t)

	cmd := exec.Command(shell, "-NoProfile", "-NonInteractive",
		"-File", script, "-?")
	out, err := cmd.CombinedOutput()
	// -? may exit non-zero; that is acceptable — we only check content.
	_ = err
	combined := string(out)
	if combined == "" {
		t.Fatal("help output is empty; expected usage text")
	}
}

// T-D3: Basic install copies binary to LOCALAPPDATA-derived path.
// Windows-only: sets USERPROFILE and LOCALAPPDATA to tempDir, places a
// dummy .exe as cc-probeline.exe next to the script, and verifies copy.
func TestInstallPs1_BasicInstall(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	shell := pwshBin(t)
	scriptDir := filepath.Dir(ps1ScriptPath(t))
	root := projectRoot()

	// Create temp env directories.
	tmpHome := t.TempDir()
	localApp := filepath.Join(tmpHome, "AppData", "Local")
	if err := os.MkdirAll(localApp, 0o755); err != nil {
		t.Fatalf("MkdirAll localApp: %v", err)
	}

	// Place a minimal dummy binary next to the script (or at project root).
	dummyExe := filepath.Join(root, "cc-probeline.exe")
	if _, err := os.Stat(dummyExe); os.IsNotExist(err) {
		// Write a tiny fake .exe (PE headers are irrelevant for copy test).
		if err := os.WriteFile(dummyExe, []byte("MZ"), 0o755); err != nil {
			t.Fatalf("write dummy exe: %v", err)
		}
		defer os.Remove(dummyExe)
	}

	destExe := filepath.Join(localApp, "Programs", "cc-probeline", "cc-probeline.exe")
	destDir := filepath.Dir(destExe)

	cmd := exec.Command(shell, "-NoProfile", "-NonInteractive",
		"-File", filepath.Join(scriptDir, "install.ps1"),
		"-Dest", destExe,
		"-NoSettings")
	cmd.Env = append(os.Environ(),
		"USERPROFILE="+tmpHome,
		"LOCALAPPDATA="+localApp,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.ps1 -Dest ... -NoSettings failed:\n%s", string(out))
	}

	if _, err := os.Stat(filepath.Join(destDir, "cc-probeline.exe")); err != nil {
		t.Fatalf("binary not found at dest %q after install: %v", destDir, err)
	}
}

// T-D4: Idempotency — run install.ps1 twice, settings.json md5 must be equal.
// Windows-only.
func TestInstallPs1_Idempotent(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	shell := pwshBin(t)
	scriptDir := filepath.Dir(ps1ScriptPath(t))
	root := projectRoot()

	tmpHome := t.TempDir()
	localApp := filepath.Join(tmpHome, "AppData", "Local")
	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll .claude: %v", err)
	}

	dummyExe := filepath.Join(root, "cc-probeline-idempotent-test.exe")
	if err := os.WriteFile(dummyExe, []byte("MZ"), 0o755); err != nil {
		t.Fatalf("write dummy exe: %v", err)
	}
	defer os.Remove(dummyExe)

	destExe := filepath.Join(localApp, "Programs", "cc-probeline", "cc-probeline.exe")
	runInstallPs1 := func() {
		t.Helper()
		cmd := exec.Command(shell, "-NoProfile", "-NonInteractive",
			"-File", filepath.Join(scriptDir, "install.ps1"),
			"-Dest", destExe)
		cmd.Env = append(os.Environ(),
			"USERPROFILE="+tmpHome,
			"LOCALAPPDATA="+localApp,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("install.ps1 run failed:\n%s", string(out))
		}
	}

	runInstallPs1()
	settingsFile := filepath.Join(claudeDir, "settings.json")
	data1, err := os.ReadFile(settingsFile)
	if err != nil {
		t.Fatalf("ReadFile settings after first run: %v", err)
	}
	hash1 := md5Hex(data1)

	runInstallPs1()
	data2, err := os.ReadFile(settingsFile)
	if err != nil {
		t.Fatalf("ReadFile settings after second run: %v", err)
	}
	hash2 := md5Hex(data2)

	if hash1 != hash2 {
		t.Fatalf("install.ps1 not idempotent: md5 changed from %s to %s", hash1, hash2)
	}
}

// T-D5: Static grep — install.ps1 must contain required strings.
// Cross-platform: runs on macOS/Linux as well.
// Expected to FAIL on 5.0 stub (required strings not yet present).
func TestInstallPs1_Contains(t *testing.T) {
	script := ps1ScriptPath(t)
	data, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("ReadFile install.ps1: %v", err)
	}

	needles := []string{
		"#requires -version 5.1",
		"ConvertFrom-Json",
		"--merge-settings",
		"LOCALAPPDATA",
	}
	for _, n := range needles {
		if !bytes.Contains(data, []byte(n)) {
			t.Errorf("install.ps1 must contain %q (not present in current stub)", n)
		}
	}
}

// T-D6: BOM check — install.ps1 must NOT start with UTF-8 BOM (EF BB BF).
// Cross-platform: runs on macOS/Linux as well.
// PowerShell 5.1 quirk: scripts with BOM may cause issues in some edge cases.
func TestInstallPs1_NoBom(t *testing.T) {
	script := ps1ScriptPath(t)
	data, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("ReadFile install.ps1: %v", err)
	}
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		t.Fatalf("install.ps1 must not have UTF-8 BOM (PS 5.1 quirk): first bytes are EF BB BF")
	}
}
