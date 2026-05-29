package config

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ResolveEmail returns the email address to display in the Email probe.
// It walks the cascade and returns the first non-empty value:
//  1. cfg.Probes.Email.Address — explicit TOML override (highest priority).
//  2. claudeJSONEmail()       — "emailAddress" from ~/.claude.json.
//  3. gitConfigEmail(cwd)     — git config user.email in the working directory.
//
// All sources are fail-soft: any I/O or parse error yields "" and the cascade
// continues. The function never panics.
func ResolveEmail(cfg *Config, cwd string) string {
	// Source 1: explicit TOML override.
	if cfg != nil && cfg.Probes.Email.Address != "" {
		slog.Debug("config.ResolveEmail: using TOML override")
		return cfg.Probes.Email.Address
	}

	// Source 2: ~/.claude.json emailAddress field.
	if email := claudeJSONEmail(); email != "" {
		slog.Debug("config.ResolveEmail: using claude.json email")
		return email
	}

	// Source 3: git config user.email in cwd.
	if email := gitConfigEmail(cwd); email != "" {
		slog.Debug("config.ResolveEmail: using git config email")
		return email
	}

	slog.Debug("config.ResolveEmail: all sources empty")
	return ""
}

// claudeJSONEmail reads ~/.claude.json and returns the top-level "emailAddress"
// string field. Returns "" on any error (missing file, parse error, missing key).
func claudeJSONEmail() string {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Debug("config.claudeJSONEmail: UserHomeDir failed", "err", err)
		return ""
	}

	path := home + "/.claude.json"
	data, err := os.ReadFile(path)
	if err != nil {
		// ENOENT is expected in CI / test environments — not an error worth logging.
		if !os.IsNotExist(err) {
			slog.Warn("config.claudeJSONEmail: read failed", "path", path, "err", err)
		}
		return ""
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		slog.Warn("config.claudeJSONEmail: parse failed", "path", path, "err", err)
		return ""
	}

	raw, ok := obj["emailAddress"]
	if !ok {
		return ""
	}

	var addr string
	if err := json.Unmarshal(raw, &addr); err != nil {
		slog.Warn("config.claudeJSONEmail: emailAddress type mismatch", "err", err)
		return ""
	}

	return addr
}

// gitConfigEmail runs "git config user.email" in cwd and returns the trimmed
// result. Returns "" on any exec error, non-zero exit, or empty output.
// GIT_OPTIONAL_LOCKS=0 is set to avoid lock-file creation during the query.
// A 150ms hard timeout prevents unbounded blocking on slow git operations.
func gitConfigEmail(cwd string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "config", "user.email")
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")

	out, err := cmd.Output()
	if err != nil {
		// Non-zero exit is normal when git config key is not set — Debug only.
		slog.Debug("config.gitConfigEmail: git config failed", "cwd", cwd, "err", err)
		return ""
	}

	return strings.TrimSpace(string(out))
}
