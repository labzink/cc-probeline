// Package cost_test — TestMain changes the working directory to the module root
// so that relative paths like "internal/cost" used in TestNoPricingTable resolve
// correctly (go test sets cwd to the package source directory, not the module root).
package cost_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	// Find the module root by walking up from the current file's directory
	// until we find a go.mod file.
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root without finding go.mod; leave cwd as-is.
			break
		}
		dir = parent
	}
	if err := os.Chdir(dir); err != nil {
		// Non-fatal: tests that rely on cwd will fail with clear messages.
		_ = err
	}
	os.Exit(m.Run())
}
