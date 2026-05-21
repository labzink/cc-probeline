// Package hint_test verifies the per-session hint State persistence layer:
// path resolution, AllShown / Advance logic, Save/Load round-trip, and
// concurrent-write safety.
//
// §4.4.b Hint widget + State — RED phase.
// All tests fail because internal/hint/state.go is a stub.
package hint_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/hint"
)

// ---------------------------------------------------------------------------
// AllShown
// ---------------------------------------------------------------------------

// TestState_AllShown_ZeroOfTotal verifies that a zero State (no shown indices)
// returns false for AllShown(8).
func TestState_AllShown_ZeroOfTotal(t *testing.T) {
	s := hint.State{}
	if s.AllShown(8) {
		t.Error("AllShown(8) on empty state = true; want false")
	}
}

// TestState_AllShown_AllOfTotal verifies that when all 8 indices are shown,
// AllShown(8) returns true.
func TestState_AllShown_AllOfTotal(t *testing.T) {
	s := hint.State{ShownIndices: []int{0, 1, 2, 3, 4, 5, 6, 7}}
	if !s.AllShown(8) {
		t.Error("AllShown(8) on full shown = false; want true")
	}
}

// TestState_AllShown_PartialOfTotal verifies that when only 2 of 8 are shown,
// AllShown(8) returns false.
func TestState_AllShown_PartialOfTotal(t *testing.T) {
	s := hint.State{ShownIndices: []int{0, 1}}
	if s.AllShown(8) {
		t.Error("AllShown(8) on partial shown = true; want false")
	}
}

// ---------------------------------------------------------------------------
// Advance
// ---------------------------------------------------------------------------

// TestState_Advance_FromZero verifies that Advance on a zero State (CurrentIndex=0,
// no shown indices) picks index 1 (next from default 0) and stamps LastSwitch.
func TestState_Advance_FromZero(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := hint.State{}
	s.Advance(8, now)
	if s.CurrentIndex != 1 {
		t.Errorf("Advance(8) from zero: CurrentIndex = %d; want 1", s.CurrentIndex)
	}
	if len(s.ShownIndices) != 1 || s.ShownIndices[0] != 1 {
		t.Errorf("Advance(8) from zero: ShownIndices = %v; want [1]", s.ShownIndices)
	}
	if !s.LastSwitch.Equal(now) {
		t.Errorf("Advance(8): LastSwitch = %v; want %v", s.LastSwitch, now)
	}
}

// TestState_Advance_SkipsAlreadyShown verifies that Advance skips indices already
// in ShownIndices and moves to the next unseen one.
func TestState_Advance_SkipsAlreadyShown(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	// Shown: 0 and 2; current is 2 — next unseen should be 3.
	s := hint.State{
		ShownIndices: []int{0, 2},
		CurrentIndex: 2,
	}
	s.Advance(8, now)
	if s.CurrentIndex != 3 {
		t.Errorf("Advance: CurrentIndex = %d; want 3", s.CurrentIndex)
	}
}

// TestState_Advance_WrapsAround verifies that when the shown set is {3,4,5,6,7}
// and CurrentIndex=7, Advance wraps around and picks index 0.
func TestState_Advance_WrapsAround(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := hint.State{
		ShownIndices: []int{3, 4, 5, 6, 7},
		CurrentIndex: 7,
	}
	s.Advance(8, now)
	if s.CurrentIndex != 0 {
		t.Errorf("Advance(wrap): CurrentIndex = %d; want 0", s.CurrentIndex)
	}
}

// ---------------------------------------------------------------------------
// StatePath
// ---------------------------------------------------------------------------

// TestStatePath_EmptySessionID verifies that StatePath("") returns "".
func TestStatePath_EmptySessionID(t *testing.T) {
	got := hint.StatePath("")
	if got != "" {
		t.Errorf("StatePath(%q) = %q; want empty string", "", got)
	}
}

// TestStatePath_FromXDG verifies that when XDG_CACHE_HOME is set, StatePath
// returns $XDG_CACHE_HOME/cc-probeline/hint-abc.json.
func TestStatePath_FromXDG(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg")
	got := hint.StatePath("abc")
	want := filepath.Join("/tmp/xdg", "cc-probeline", "hint-abc.json")
	if got != want {
		t.Errorf("StatePath(XDG) = %q; want %q", got, want)
	}
}

// TestStatePath_FromHome verifies that when XDG_CACHE_HOME is unset but HOME
// is set, StatePath returns $HOME/.cache/cc-probeline/hint-abc.json.
func TestStatePath_FromHome(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "/tmp/home")
	got := hint.StatePath("abc")
	want := filepath.Join("/tmp/home", ".cache", "cc-probeline", "hint-abc.json")
	if got != want {
		t.Errorf("StatePath(HOME) = %q; want %q", got, want)
	}
}

// TestStatePath_HomeUnset_ReturnsEmpty verifies that when both XDG_CACHE_HOME
// and HOME are unset, StatePath returns "" (memory-only mode).
func TestStatePath_HomeUnset_ReturnsEmpty(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")
	got := hint.StatePath("abc")
	if got != "" {
		t.Errorf("StatePath(noEnv) = %q; want empty string", got)
	}
}

// ---------------------------------------------------------------------------
// Save / Load
// ---------------------------------------------------------------------------

// TestSaveLoad_RoundTrip verifies that Save followed by Load returns the
// exact same State (deep equality of all fields).
func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	original := hint.State{
		ShownIndices: []int{0, 2, 5},
		CurrentIndex: 5,
		LastSwitch:   now,
	}

	if err := hint.Save("sess-rt", original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := hint.Load("sess-rt")

	if got.CurrentIndex != original.CurrentIndex {
		t.Errorf("Load.CurrentIndex = %d; want %d", got.CurrentIndex, original.CurrentIndex)
	}
	if len(got.ShownIndices) != len(original.ShownIndices) {
		t.Errorf("Load.ShownIndices len = %d; want %d", len(got.ShownIndices), len(original.ShownIndices))
	} else {
		for i, v := range original.ShownIndices {
			if got.ShownIndices[i] != v {
				t.Errorf("Load.ShownIndices[%d] = %d; want %d", i, got.ShownIndices[i], v)
			}
		}
	}
	// Compare LastSwitch with second precision — JSON round-trip may lose sub-second.
	if !got.LastSwitch.Equal(original.LastSwitch) {
		t.Errorf("Load.LastSwitch = %v; want %v", got.LastSwitch, original.LastSwitch)
	}
}

// TestLoad_FileMissing_ReturnsEmpty verifies that Load returns an empty State
// when no file exists for the given sessionID.
func TestLoad_FileMissing_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	got := hint.Load("nonexistent-session")
	empty := hint.State{}
	if got.CurrentIndex != empty.CurrentIndex || len(got.ShownIndices) != 0 {
		t.Errorf("Load(missing) = %+v; want zero State", got)
	}
}

// TestLoad_CorruptJson_ReturnsEmpty verifies that when the state file contains
// invalid JSON, Load returns an empty State without panicking.
func TestLoad_CorruptJson_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	// Write corrupt content directly to where StatePath would look.
	stateDir := filepath.Join(dir, "cc-probeline")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	corruptPath := filepath.Join(stateDir, "hint-corrupt-sess.json")
	if err := os.WriteFile(corruptPath, []byte("not valid JSON {{{{"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := hint.Load("corrupt-sess")
	if got.CurrentIndex != 0 || len(got.ShownIndices) != 0 {
		t.Errorf("Load(corrupt) = %+v; want zero State", got)
	}
}

// ---------------------------------------------------------------------------
// Save — error paths
// ---------------------------------------------------------------------------

// TestSave_WriteFileFails_ReadOnlyDir verifies that Save returns an error when
// the cache directory is read-only (os.WriteFile cannot create the .tmp file).
func TestSave_WriteFileFails_ReadOnlyDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	// Create the cc-probeline subdir first so MkdirAll succeeds, then make it
	// read-only so os.WriteFile fails when trying to create the .tmp file.
	ccDir := filepath.Join(dir, "cc-probeline")
	if err := os.MkdirAll(ccDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ccDir, 0o500); err != nil {
		t.Fatal(err)
	}
	// Restore write permission in cleanup so t.TempDir() can clean up.
	t.Cleanup(func() { _ = os.Chmod(ccDir, 0o755) })

	s := hint.State{ShownIndices: []int{0}, CurrentIndex: 0}
	err := hint.Save("readonly-sess", s)
	if err == nil {
		t.Error("Save to read-only dir: expected error, got nil")
	}
}

// TestSave_MkdirAllFails_ParentIsFile verifies that Save returns an error when
// the path where MkdirAll would create the cc-probeline directory is already
// occupied by a plain file (not a directory).
func TestSave_MkdirAllFails_ParentIsFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	// Place a regular file where MkdirAll expects to create a directory.
	conflict := filepath.Join(dir, "cc-probeline")
	if err := os.WriteFile(conflict, []byte("conflict"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := hint.State{ShownIndices: []int{0}, CurrentIndex: 0}
	err := hint.Save("conflict-sess", s)
	if err == nil {
		t.Error("Save with file blocking dir creation: expected error, got nil")
	}
}

// TestSave_ConcurrentCallsSafe verifies that 5 goroutines calling Save
// simultaneously do not cause a data race and leave a valid JSON file on disk.
func TestSave_ConcurrentCallsSafe(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	const goroutines = 5
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			s := hint.State{
				ShownIndices: []int{i},
				CurrentIndex: i,
			}
			_ = hint.Save("concurrent-sess", s)
		}()
	}
	wg.Wait()

	// The file must exist and contain valid JSON after concurrent writes.
	p := hint.StatePath("concurrent-sess")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("state file missing after concurrent Save: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Errorf("state file is not valid JSON after concurrent Save: %v\ncontent: %s", err, b)
	}
}
