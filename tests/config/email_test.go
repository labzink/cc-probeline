// Package config_test contains tests for config.ResolveEmail (T-10..T-13).
// These are RED tests: config.ResolveEmail does not exist yet.
// Tests will fail to compile until internal/config/email.go is implemented.
package config_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/labzink/cc-probeline/internal/config"
)

// writeClaudeJSON writes a minimal ~/.claude.json with the given emailAddress
// into dir. Pass "" to write a JSON object with no emailAddress key.
func writeClaudeJSON(t *testing.T, dir, email string) {
	t.Helper()
	data := map[string]interface{}{}
	if email != "" {
		data["emailAddress"] = email
	}
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("writeClaudeJSON: marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), b, 0o644); err != nil {
		t.Fatalf("writeClaudeJSON: write: %v", err)
	}
}

// initGitRepo creates a minimal git repo at dir and sets user.email to addr.
// No commit is required — git config is readable without one.
func initGitRepo(t *testing.T, dir, addr string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("initGitRepo: %v: %s", err, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", addr)
}

// T-10: TOML override wins over .claude.json.
func TestResolveEmail_TOMLOverride(t *testing.T) {
	tmpDir := t.TempDir()
	// Write a .claude.json with a different email to prove override takes priority.
	writeClaudeJSON(t, tmpDir, "claude@example.com")
	t.Setenv("HOME", tmpDir)

	cfg := &config.Config{}
	cfg.Probes.Email.Address = "override@example.com"

	got := config.ResolveEmail(cfg, tmpDir)
	if got != "override@example.com" {
		t.Errorf("T-10: got %q, want %q", got, "override@example.com")
	}
}

// T-11: No TOML override — email resolved from ~/.claude.json.
func TestResolveEmail_ClaudeJSON(t *testing.T) {
	tmpDir := t.TempDir()
	writeClaudeJSON(t, tmpDir, "claude@example.com")
	t.Setenv("HOME", tmpDir)

	cfg := &config.Config{}
	// cfg.Probes.Email.Address is intentionally empty.

	got := config.ResolveEmail(cfg, tmpDir)
	if got != "claude@example.com" {
		t.Errorf("T-11: got %q, want %q", got, "claude@example.com")
	}
}

// T-12: No TOML override, no .claude.json — email resolved from git config.
func TestResolveEmail_GitConfig(t *testing.T) {
	homeDir := t.TempDir()
	// Write an empty .claude.json (no emailAddress key) so the JSON source yields "".
	writeClaudeJSON(t, homeDir, "")
	t.Setenv("HOME", homeDir)

	repoDir := t.TempDir()
	initGitRepo(t, repoDir, "git@example.com")

	cfg := &config.Config{}

	got := config.ResolveEmail(cfg, repoDir)
	if got != "git@example.com" {
		t.Errorf("T-12: got %q, want %q", got, "git@example.com")
	}
}

// T-13: All sources empty — ResolveEmail returns "" without panic.
func TestResolveEmail_AllEmpty(t *testing.T) {
	homeDir := t.TempDir()
	// No .claude.json in homeDir at all.
	t.Setenv("HOME", homeDir)

	cwdDir := t.TempDir()
	// cwdDir is not a git repo — git config will fail gracefully.

	cfg := &config.Config{}

	got := config.ResolveEmail(cfg, cwdDir)
	if got != "" {
		t.Errorf("T-13: got %q, want %q", got, "")
	}
}

// TestResolveEmail_ClaudeJSON_TypeMismatch (I5) verifies that when ~/.claude.json
// contains {"emailAddress": 42} (integer instead of string), ResolveEmail returns
// "" without panicking. This exercises the claudeJSONEmail type-mismatch path.
func TestResolveEmail_ClaudeJSON_TypeMismatch(t *testing.T) {
	homeDir := t.TempDir()
	// Write .claude.json with emailAddress as integer (invalid type).
	content := []byte(`{"emailAddress": 42}`)
	if err := os.WriteFile(filepath.Join(homeDir, ".claude.json"), content, 0o644); err != nil {
		t.Fatalf("writeClaudeJSON type-mismatch: write: %v", err)
	}
	t.Setenv("HOME", homeDir)

	cwdDir := t.TempDir()
	// cwdDir is not a git repo so git config also yields "".

	cfg := &config.Config{}

	// Must not panic; must return "".
	got := config.ResolveEmail(cfg, cwdDir)
	if got != "" {
		t.Errorf("TypeMismatch: got %q, want \"\"", got)
	}
}
