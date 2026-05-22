package settingsfile_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/settingsfile"
)

// T-S1: IsOurs returns true when command is an absolute Unix path ending with cc-probeline.
func TestIsOurs_OurAbsolutePath(t *testing.T) {
	s := settingsfile.Settings{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "/Users/x/.local/bin/cc-probeline",
		},
	}
	if !settingsfile.IsOurs(s) {
		t.Fatal("IsOurs = false; want true for absolute path ending with cc-probeline")
	}
}

// T-S2: IsOurs returns true when command is a Windows path ending with cc-probeline.exe.
func TestIsOurs_OurExe(t *testing.T) {
	s := settingsfile.Settings{
		"statusLine": map[string]any{
			"type":    "command",
			"command": `C:\Users\user\AppData\Local\Programs\cc-probeline\cc-probeline.exe`,
		},
	}
	if !settingsfile.IsOurs(s) {
		t.Fatal("IsOurs = false; want true for Windows path ending with cc-probeline.exe")
	}
}

// T-S3: IsOurs returns false for a typosquat binary name (missing trailing 'e').
func TestIsOurs_Typosquat(t *testing.T) {
	s := settingsfile.Settings{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "/usr/local/bin/cc-probelin",
		},
	}
	if settingsfile.IsOurs(s) {
		t.Fatal("IsOurs = true; want false for typosquat cc-probelin")
	}
}

// T-S4: IsOurs returns false for a wrapper shell command (not a plain binary path).
// Per plan §5.1: regex end-anchor means "wrapper.sh && cc-probeline" is treated
// as foreign (safer — avoids cross-plugin overwrite).
func TestIsOurs_Wrapper(t *testing.T) {
	s := settingsfile.Settings{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "wrapper.sh && cc-probeline",
		},
	}
	if settingsfile.IsOurs(s) {
		t.Fatal("IsOurs = true; want false for wrapper shell command (non-plain-path)")
	}
}

// T-S5: IsOurs returns false when Settings has no statusLine key.
func TestIsOurs_NoStatusLine(t *testing.T) {
	s := settingsfile.Settings{
		"theme": "dark",
	}
	if settingsfile.IsOurs(s) {
		t.Fatal("IsOurs = true; want false for Settings without statusLine")
	}
}

// T-S6: RemoveStatusLine removes only "statusLine"; other keys survive intact.
func TestRemoveStatusLine_Preserves(t *testing.T) {
	input := settingsfile.Settings{
		"theme": "dark",
		"permissions": map[string]any{
			"allow": []any{"Bash(go *)"},
		},
		"statusLine": map[string]any{
			"type":    "command",
			"command": "/usr/local/bin/cc-probeline",
		},
	}
	got := settingsfile.RemoveStatusLine(input)

	if _, ok := got["statusLine"]; ok {
		t.Fatal("RemoveStatusLine: statusLine still present in output")
	}
	if got["theme"] != "dark" {
		t.Fatalf("RemoveStatusLine: theme missing or changed, got %v", got["theme"])
	}
	perms, ok := got["permissions"].(map[string]any)
	if !ok {
		t.Fatal("RemoveStatusLine: permissions key missing or wrong type")
	}
	allow, ok := perms["allow"].([]any)
	if !ok || len(allow) != 1 || allow[0] != "Bash(go *)" {
		t.Fatalf("RemoveStatusLine: permissions.allow not preserved, got %v", perms["allow"])
	}

	// Input must not have been mutated.
	if _, ok := input["statusLine"]; !ok {
		t.Fatal("RemoveStatusLine mutated the input map (statusLine was removed from input)")
	}
}

// T-S7: Save is atomic — if the tmp write would fail, the original file is untouched.
// We induce failure by making the destination directory read-only so WriteFile to .tmp fails.
func TestSaveAtomic(t *testing.T) {
	dir := t.TempDir()
	origPath := filepath.Join(dir, "settings.json")

	original := settingsfile.Settings{"theme": "original"}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(origPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Make the directory read-only so that creating .tmp inside it fails.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) }) //nolint:errcheck

	newSettings := settingsfile.Settings{"theme": "new-value-that-should-not-appear"}
	if saveErr := settingsfile.Save(origPath, newSettings); saveErr == nil {
		t.Fatal("Save unexpectedly succeeded in a read-only directory")
	}

	// Original must still be intact.
	got, err := os.ReadFile(origPath)
	if err != nil {
		t.Fatalf("ReadFile after failed Save: %v", err)
	}
	if !strings.Contains(string(got), "original") {
		t.Fatalf("original file was corrupted; content: %s", got)
	}
}

// T-S8: BackupPath formats the timestamp as YYYYMMDD-HHmmss.
func TestBackupPathFormat(t *testing.T) {
	ts := time.Date(2026, 5, 22, 15, 30, 12, 0, time.UTC)
	base := "/home/user/.claude/settings.json"
	got := settingsfile.BackupPath(base, ts)

	want := base + ".cc-probeline.bak.20260522-153012"
	if got != want {
		t.Fatalf("BackupPath = %q; want %q", got, want)
	}
}

// T-S9: Load returns a wrapped error for malformed JSON.
func TestLoad_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(badPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := settingsfile.Load(badPath)
	if err == nil {
		t.Fatal("Load returned nil error for malformed JSON; want non-nil error")
	}
}
