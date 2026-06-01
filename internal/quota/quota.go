// Package quota stores and retrieves the global account-wide quota snapshot.
//
// The snapshot is persisted as a single JSON file ("quota.json") in:
//  1. CC_PROBELINE_QUOTA_DIR   (env var; used by tests for isolation)
//  2. XDG_DATA_HOME/cc-probeline
//  3. ~/.local/share/cc-probeline
//
// Write durability: Update uses atomic .tmp+rename guarded by a flock on
// <path>.lock, matching the pattern from internal/mode/mode.go.
//
// The "freshest-wins" rule: Update writes only when the incoming Snapshot.TS
// is strictly greater than the TS of the stored snapshot. This ensures that
// an idle session cannot overwrite a fresher snapshot from an active session.
package quota

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// Snapshot is the global account-wide quota state captured at a point in time.
// TS is epoch-milliseconds of the observation; zero means "unknown/uninitialised".
type Snapshot struct {
	TS            int64   // epoch-ms of the observation
	FiveHourPct   float64 // 5-hour rate-limit window, used percentage (0–100)
	SevenDayPct   float64 // 7-day rate-limit window, used percentage (0–100)
	FiveHourReset int64   // unix seconds when the 5-hour window resets; 0 = unknown
	SevenDayReset int64   // unix seconds when the 7-day window resets; 0 = unknown
}

// quotaDir resolves the directory used for the global quota file.
// Priority: CC_PROBELINE_QUOTA_DIR → XDG_DATA_HOME/cc-probeline → ~/.local/share/cc-probeline.
func quotaDir() string {
	if dir := os.Getenv("CC_PROBELINE_QUOTA_DIR"); dir != "" {
		return dir
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "cc-probeline")
	}
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share", "cc-probeline")
}

// quotaPath returns the full path of the quota JSON file.
// Returns "" when the directory cannot be determined.
func quotaPath() string {
	dir := quotaDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "quota.json")
}

// Update persists s to disk only if s.TS is strictly greater than the TS of
// any already-stored snapshot. This "freshest-wins" rule ensures that an idle
// session cannot overwrite a live session's fresher data.
//
// Write sequence: MkdirAll → lock → read existing TS → compare → write .tmp → rename → unlock.
func Update(s Snapshot) error {
	slog.Debug("quota.Update start", "ts", s.TS)

	p := quotaPath()
	if p == "" {
		return fmt.Errorf("quota.Update: quota dir unavailable (HOME not set?)")
	}

	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("quota.Update: mkdir %q: %w", dir, err)
	}

	// Acquire exclusive lock before reading and writing to avoid a race
	// between concurrent cc-probeline invocations.
	fl := flock.New(p + ".lock")
	if err := fl.Lock(); err != nil {
		return fmt.Errorf("quota.Update: flock: %w", err)
	}
	defer fl.Unlock() //nolint:errcheck

	// Read existing snapshot to compare TS.
	existing, err := readFromPath(p)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// Log but continue: on any read/decode error we allow the write.
		slog.Warn("quota.Update: read existing failed; overwriting", "err", err)
	}

	// Freshest-wins: reject if the incoming TS is not strictly newer.
	if err == nil && s.TS <= existing.TS {
		slog.Debug("quota.Update: incoming TS not newer; skipping write",
			"incoming_ts", s.TS, "stored_ts", existing.TS)
		return nil
	}

	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("quota.Update: encode: %w", err)
	}

	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("quota.Update: write tmp: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("quota.Update: rename: %w", err)
	}

	slog.Debug("quota.Update complete", "ts", s.TS, "path", p)
	return nil
}

// Freshest reads the stored Snapshot from disk.
// Returns (Snapshot, true) when the file exists and is valid JSON.
// Returns (zero Snapshot, false) when the file does not exist or cannot be decoded.
func Freshest() (Snapshot, bool) {
	slog.Debug("quota.Freshest called")

	p := quotaPath()
	if p == "" {
		slog.Warn("quota.Freshest: quota dir unavailable")
		return Snapshot{}, false
	}

	s, err := readFromPath(p)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("quota.Freshest: read failed", "path", p, "err", err)
		}
		return Snapshot{}, false
	}

	slog.Debug("quota.Freshest complete", "ts", s.TS)
	return s, true
}

// readFromPath reads and decodes a Snapshot from the given file path.
// Returns os.ErrNotExist when the file is absent.
func readFromPath(p string) (Snapshot, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return Snapshot{}, err
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return Snapshot{}, fmt.Errorf("quota: decode %q: %w", p, err)
	}
	return s, nil
}
