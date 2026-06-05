package testutil

// golden.go — shared helpers for golden-snapshot tests (Phase 6.95.i).
//
// A golden test renders a status line through the REAL pipeline
// (Assembler.Render → renderer.Apply) for a fixed input scenario and compares
// the result against a committed snapshot under testdata/golden/. The snapshot
// is approved once by a human; afterwards it acts as a CI regression guard.
//
// Re-generate snapshots after an intentional output change:
//
//	go test ./tests/statusline/ -run Golden -update
//
// Two snapshot forms (caller decides which to pass):
//   - plain  : output AFTER renderer.Apply with AnsiEnabled=false (no markers,
//     human-readable). Used for layout / no-color scenarios.
//   - marker : RAW assembler output BEFORE Apply ({{color:NAME}} tokens intact,
//     so a colour diff stays readable). Used for colour scenarios.

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// UpdateGolden rewrites snapshots instead of comparing when -update is passed.
// Defined once here so every package importing testutil shares the flag.
var UpdateGolden = flag.Bool("update", false, "rewrite golden snapshots instead of comparing")

// ProjectRoot walks up from the current working directory until it finds the
// directory containing go.mod, and returns that absolute path.
func ProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found walking up from working directory")
		}
		dir = parent
	}
}

// CompareGolden compares got against the snapshot at goldenPath. When -update is
// set it (re)writes the snapshot and returns without comparing. On mismatch it
// fails the test with a want/got diff. Parent directories are created on update.
func CompareGolden(t *testing.T, goldenPath, got string) {
	t.Helper()
	if *UpdateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
		t.Logf("updated golden %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s (run with -update to create): %v", goldenPath, err)
	}
	if got != string(want) {
		t.Errorf("snapshot mismatch (%s)\n--- want ---\n%s\n--- got ---\n%s",
			filepath.Base(goldenPath), string(want), got)
	}
}
