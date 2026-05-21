// Package mode_test verifies the persistence layer for the user's display
// preference (super-compact vs standard).
//
// §4.2 Mode toggle — global storage, atomic write, XDG-aware Path resolution.
//
// RED phase: all tests fail because the stub in internal/mode/mode.go:
//   - Path()   always returns ""
//   - Load()   always returns Default (Standard)
//   - Save()   is a no-op
//   - Toggle() returns (Default, nil) without touching the filesystem
package mode_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/tests/testutil"
)

// ---------------------------------------------------------------------------
// Path resolution
// ---------------------------------------------------------------------------

// TestPath_XDG verifies that when XDG_CONFIG_HOME is set, Path() returns
// $XDG_CONFIG_HOME/cc-probeline/mode.
// §4.2 Mode toggle — XDG_CONFIG_HOME resolution (C-1).
func TestPath_XDG(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	want := filepath.Join(tmpDir, "cc-probeline", "mode")
	got := mode.Path()
	if got != want {
		t.Errorf("Path() with XDG_CONFIG_HOME=%q: got %q, want %q", tmpDir, got, want)
	}
}

// TestPath_NoXDG verifies that when XDG_CONFIG_HOME is empty, Path() falls
// back to $HOME/.config/cc-probeline/mode.
// §4.2 Mode toggle — HOME fallback (C-1).
func TestPath_NoXDG(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", tmpDir)

	want := filepath.Join(tmpDir, ".config", "cc-probeline", "mode")
	got := mode.Path()
	if got != want {
		t.Errorf("Path() with HOME=%q and no XDG: got %q, want %q", tmpDir, got, want)
	}
}

// ---------------------------------------------------------------------------
// Load
// ---------------------------------------------------------------------------

// TestLoad_Default_NoFile verifies that Load() returns Standard when the
// storage file does not exist.
// §4.2 Mode toggle — Default=Standard on missing file (C-3).
func TestLoad_Default_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	// Do NOT create the mode file — it must not exist.

	got := mode.Load()
	if got != mode.Standard {
		t.Errorf("Load() with no file: got %q, want %q", got, mode.Standard)
	}
}

// TestLoad_Default_Corrupt verifies that Load() returns Standard when the
// storage file contains an unrecognised value.
// §4.2 Mode toggle — Default=Standard on corrupt content (C-3).
func TestLoad_Default_Corrupt(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Write an invalid value to the mode file.
	dir := filepath.Join(tmpDir, "cc-probeline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	p := filepath.Join(dir, "mode")
	if err := os.WriteFile(p, []byte("garbage\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := mode.Load()
	if got != mode.Standard {
		t.Errorf("Load() with corrupt file: got %q, want %q", got, mode.Standard)
	}
}

// TestLoad_Valid_SuperCompact verifies that Load() returns SuperCompact when
// the storage file contains "super-compact".
// §4.2 Mode toggle — valid super-compact value (C-3).
func TestLoad_Valid_SuperCompact(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	dir := filepath.Join(tmpDir, "cc-probeline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	p := filepath.Join(dir, "mode")
	if err := os.WriteFile(p, []byte("super-compact"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := mode.Load()
	if got != mode.SuperCompact {
		t.Errorf("Load() with super-compact file: got %q, want %q", got, mode.SuperCompact)
	}
}

// TestLoad_Valid_Standard verifies that Load() returns Standard when the
// storage file contains "standard" (with optional trailing newline — trim).
// §4.2 Mode toggle — valid standard value with whitespace trimming.
func TestLoad_Valid_Standard(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	dir := filepath.Join(tmpDir, "cc-probeline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	p := filepath.Join(dir, "mode")
	// Include trailing newline to verify TrimSpace behaviour.
	if err := os.WriteFile(p, []byte("standard\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := mode.Load()
	if got != mode.Standard {
		t.Errorf("Load() with 'standard\\n' file: got %q, want %q", got, mode.Standard)
	}
}

// ---------------------------------------------------------------------------
// Save
// ---------------------------------------------------------------------------

// TestSave_Atomic verifies that Save() writes via a .tmp file then renames it,
// leaving no .tmp artifact and writing the correct content.
// §4.2 Mode toggle — atomic write: .tmp + rename (C-2).
func TestSave_Atomic(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Pre-create the parent directory so we can observe the .tmp file.
	dir := filepath.Join(tmpDir, "cc-probeline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := mode.Save(mode.SuperCompact); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	p := filepath.Join(dir, "mode")
	tmpPath := p + ".tmp"

	// After Save the .tmp file must NOT exist (it was renamed away).
	if _, err := os.Stat(tmpPath); err == nil {
		t.Errorf("Save() left behind .tmp file at %q", tmpPath)
	}

	// The main file must exist and contain exactly the mode value.
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile after Save: %v", err)
	}
	got := strings.TrimSpace(string(raw))
	want := string(mode.SuperCompact)
	if got != want {
		t.Errorf("Save() wrote %q, want %q", got, want)
	}
}

// TestSave_MkdirP verifies that Save() creates all necessary parent directories
// (with permissions 0755) when they do not exist yet.
// §4.2 Mode toggle — MkdirAll(dirname, 0o755) before write (C-2).
func TestSave_MkdirP(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Do NOT pre-create cc-probeline/ — Save must create it.
	dir := filepath.Join(tmpDir, "cc-probeline")
	if _, err := os.Stat(dir); err == nil {
		t.Fatalf("pre-condition failed: dir %q already exists", dir)
	}

	if err := mode.Save(mode.Standard); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	// Parent directory must now exist.
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("parent dir not created by Save(): %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected a directory at %q", dir)
	}

	// The mode file itself must be readable and contain the correct value.
	p := filepath.Join(dir, "mode")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("mode file not created: %v", err)
	}
	got := strings.TrimSpace(string(raw))
	if got != string(mode.Standard) {
		t.Errorf("Save() wrote %q, want %q", got, string(mode.Standard))
	}
}

// ---------------------------------------------------------------------------
// Toggle
// ---------------------------------------------------------------------------

// TestToggle_StandardToSuperCompact verifies that Toggle() when the current
// mode is Standard writes SuperCompact to disk and returns SuperCompact.
// §4.2 Mode toggle — Toggle flips and persists (C-2).
func TestToggle_StandardToSuperCompact(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Seed with Standard.
	dir := filepath.Join(tmpDir, "cc-probeline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mode"), []byte("standard"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := mode.Toggle()
	if err != nil {
		t.Fatalf("Toggle() returned error: %v", err)
	}
	if got != mode.SuperCompact {
		t.Errorf("Toggle() return value: got %q, want %q", got, mode.SuperCompact)
	}

	// Verify persisted value on disk.
	raw, err := os.ReadFile(filepath.Join(dir, "mode"))
	if err != nil {
		t.Fatalf("ReadFile after Toggle: %v", err)
	}
	persisted := mode.Mode(strings.TrimSpace(string(raw)))
	if persisted != mode.SuperCompact {
		t.Errorf("Toggle() persisted %q, want %q", persisted, mode.SuperCompact)
	}
}

// TestToggle_SuperCompactToStandard verifies that Toggle() when the current
// mode is SuperCompact writes Standard to disk and returns Standard.
// §4.2 Mode toggle — Toggle flips and persists (C-2).
func TestToggle_SuperCompactToStandard(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Seed with SuperCompact.
	dir := filepath.Join(tmpDir, "cc-probeline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mode"), []byte("super-compact"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := mode.Toggle()
	if err != nil {
		t.Fatalf("Toggle() returned error: %v", err)
	}
	if got != mode.Standard {
		t.Errorf("Toggle() return value: got %q, want %q", got, mode.Standard)
	}

	// Verify persisted value on disk.
	raw, err := os.ReadFile(filepath.Join(dir, "mode"))
	if err != nil {
		t.Fatalf("ReadFile after Toggle: %v", err)
	}
	persisted := mode.Mode(strings.TrimSpace(string(raw)))
	if persisted != mode.Standard {
		t.Errorf("Toggle() persisted %q, want %q", persisted, mode.Standard)
	}
}

// TestToggle_Twice_RestoresOriginal verifies that two sequential Toggle() calls
// return the original mode value (round-trip property).
// §4.2 Mode toggle — double toggle restores original value.
func TestToggle_Twice_RestoresOriginal(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Seed with Standard as the baseline.
	dir := filepath.Join(tmpDir, "cc-probeline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mode"), []byte("standard"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	first, err := mode.Toggle()
	if err != nil {
		t.Fatalf("first Toggle() error: %v", err)
	}
	if first != mode.SuperCompact {
		t.Errorf("first Toggle(): got %q, want %q", first, mode.SuperCompact)
	}

	second, err := mode.Toggle()
	if err != nil {
		t.Fatalf("second Toggle() error: %v", err)
	}
	if second != mode.Standard {
		t.Errorf("second Toggle() (restore): got %q, want %q", second, mode.Standard)
	}

	// Confirm disk state matches the returned value.
	raw, err := os.ReadFile(filepath.Join(dir, "mode"))
	if err != nil {
		t.Fatalf("ReadFile after two Toggles: %v", err)
	}
	persisted := mode.Mode(strings.TrimSpace(string(raw)))
	if persisted != mode.Standard {
		t.Errorf("disk after two Toggles: got %q, want %q", persisted, mode.Standard)
	}
}

// TestPath_EmptyHomeFallback verifies that Path() returns "" when both
// XDG_CONFIG_HOME and HOME are empty, and that Save() returns an error
// in that case without creating any file.
// §4.2 sec — HOME validation.
func TestPath_EmptyHomeFallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")

	got := mode.Path()
	if got != "" {
		t.Errorf("Path() with empty HOME: got %q, want \"\"", got)
	}

	err := mode.Save(mode.Standard)
	if err == nil {
		t.Error("Save() with empty HOME: expected error, got nil")
	}
}

// TestSave_RejectsUnknownMode verifies that Save() returns an error when
// called with an unrecognised Mode value and does not create any file.
// §4.2 sec — Save unknown mode validation.
func TestSave_RejectsUnknownMode(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	err := mode.Save(mode.Mode("garbage"))
	if err == nil {
		t.Fatal("Save(garbage): expected error, got nil")
	}

	// The mode file must not have been created.
	p := filepath.Join(tmpDir, "cc-probeline", "mode")
	if _, statErr := os.Stat(p); statErr == nil {
		t.Errorf("Save(garbage): file should not exist at %q", p)
	}
}

// TestToggle_SaveError_ReturnsCurrent verifies that Toggle() returns the
// current mode (not Default) when Save fails.
// §4.2 code-I3 — Toggle returns current on Save error.
func TestToggle_SaveError_ReturnsCurrent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Seed with SuperCompact so current != Default (Standard).
	dir := filepath.Join(tmpDir, "cc-probeline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mode"), []byte("super-compact"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Make the directory read-only so Save cannot write.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer os.Chmod(dir, 0o755) //nolint:errcheck

	got, err := mode.Toggle()
	if err == nil {
		t.Fatal("Toggle() with read-only dir: expected error, got nil")
	}
	// Must return current (SuperCompact), not Default (Standard).
	if got != mode.SuperCompact {
		t.Errorf("Toggle() on Save error: got %q, want %q (current)", got, mode.SuperCompact)
	}
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

// TestConcurrent_TwoToggles verifies that two goroutines calling Toggle()
// simultaneously do not produce a corrupt mode file. After both complete,
// the file must contain exactly one of the two valid Mode values AND the
// collected return values must each be one of the two valid modes (no stub
// default leaking for a non-standard seed).
//
// Run with -race to detect data races.
// §4.2 Mode toggle — atomic .tmp+rename+flock prevents concurrent corruption (C-2).
func TestConcurrent_TwoToggles(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Seed with SuperCompact so the first real Toggle must produce Standard —
	// giving us a concrete assertion that is NOT the stub default (Standard).
	// With the stub, Toggle always returns (Standard, nil) regardless of disk;
	// so collecting results and verifying consistency exposes the stub's failure.
	dir := filepath.Join(tmpDir, "cc-probeline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mode"), []byte("super-compact"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var wg sync.WaitGroup
	results := make([]mode.Mode, 2)
	errs := make([]error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		results[0], errs[0] = mode.Toggle()
	}()
	go func() {
		defer wg.Done()
		results[1], errs[1] = mode.Toggle()
	}()
	wg.Wait()

	// Neither goroutine must have returned an error.
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d Toggle() error: %v", i, err)
		}
	}

	// Each return value must be a known mode.
	for i, r := range results {
		if r != mode.SuperCompact && r != mode.Standard {
			t.Errorf("goroutine %d returned corrupt mode %q", i, r)
		}
	}

	// The file must exist and contain exactly one of the two valid values.
	raw, err := os.ReadFile(filepath.Join(dir, "mode"))
	if err != nil {
		t.Fatalf("ReadFile after concurrent Toggles: %v", err)
	}
	persisted := mode.Mode(strings.TrimSpace(string(raw)))
	if persisted != mode.SuperCompact && persisted != mode.Standard {
		t.Errorf("concurrent Toggles left corrupt file: got %q", persisted)
	}

	// The disk value must match one of the returned values: the last writer wins,
	// so the on-disk mode must equal one of the two Toggle() return values.
	// This ensures the stub cannot satisfy the test by leaving the file unchanged
	// (stub returns Standard but disk has super-compact, which is a mismatch).
	if persisted != results[0] && persisted != results[1] {
		t.Errorf("disk value %q does not match either Toggle() result (%q, %q): stub did not persist",
			persisted, results[0], results[1])
	}
}

// ---------------------------------------------------------------------------
// spec-I4: slog.Warn on non-ErrNotExist errors in Load
// ---------------------------------------------------------------------------

// setDefaultLogger installs h as the default slog logger for the duration of
// the test and restores the previous logger via t.Cleanup.
func setDefaultLogger(t *testing.T, h slog.Handler) {
	t.Helper()
	old := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(old) })
}

// TestLoad_FileNotExist_NoWarn verifies that Load() returns Default when the
// file is missing and does NOT emit a slog.Warn (ErrNotExist is expected).
// spec-I4: only non-ErrNotExist errors produce a warning.
func TestLoad_FileNotExist_NoWarn(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	// Do NOT create the file.

	h := testutil.NewCaptureHandler()
	setDefaultLogger(t, h)

	got := mode.Load()
	if got != mode.Standard {
		t.Errorf("Load() with no file: got %q, want %q", got, mode.Standard)
	}
	if h.HasWarnContaining("mode.Load") {
		t.Error("Load() with missing file: unexpected slog.Warn emitted")
	}
}

// TestLoad_PermissionDenied_WarnEmitted verifies that Load() returns Default
// and emits slog.Warn("mode.Load: read failed") when the file is unreadable.
// spec-I4: permission-denied is a non-ErrNotExist error that must warn.
func TestLoad_PermissionDenied_WarnEmitted(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses file permissions — skipping")
	}
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	dir := filepath.Join(tmpDir, "cc-probeline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	p := filepath.Join(dir, "mode")
	if err := os.WriteFile(p, []byte("standard"), 0o000); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	defer os.Chmod(p, 0o644) //nolint:errcheck

	h := testutil.NewCaptureHandler()
	setDefaultLogger(t, h)

	got := mode.Load()
	if got != mode.Standard {
		t.Errorf("Load() with unreadable file: got %q, want %q", got, mode.Standard)
	}
	if !h.HasWarnContaining("mode.Load: read failed") {
		t.Errorf("Load() with unreadable file: expected slog.Warn with 'mode.Load: read failed'; got records: %v", h.Records)
	}
}

// TestLoad_UnknownValue_WarnEmitted verifies that Load() returns Default and
// emits slog.Warn("mode.Load: unknown mode value") when the file has garbage.
// spec-I4: unknown value triggers warn in addition to returning Default.
func TestLoad_UnknownValue_WarnEmitted(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	dir := filepath.Join(tmpDir, "cc-probeline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mode"), []byte("garbage"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	h := testutil.NewCaptureHandler()
	setDefaultLogger(t, h)

	got := mode.Load()
	if got != mode.Standard {
		t.Errorf("Load() with unknown value: got %q, want %q", got, mode.Standard)
	}
	if !h.HasWarnContaining("mode.Load: unknown mode value") {
		t.Errorf("Load() with garbage file: expected slog.Warn with 'mode.Load: unknown mode value'; got records: %v", h.Records)
	}
}

// TestLoad_ValidStandard_NoWarn verifies that Load() returns Standard with no
// warning when the file is valid.
// spec-I4: clean path must not produce any slog output.
func TestLoad_ValidStandard_NoWarn(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	dir := filepath.Join(tmpDir, "cc-probeline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mode"), []byte("standard"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	h := testutil.NewCaptureHandler()
	setDefaultLogger(t, h)

	got := mode.Load()
	if got != mode.Standard {
		t.Errorf("Load() with valid file: got %q, want %q", got, mode.Standard)
	}
	if h.HasWarnContaining("mode.Load") {
		t.Error("Load() with valid file: unexpected slog.Warn emitted")
	}
}
