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

	// DataTS is epoch-ms of the moment the quota DATA last actually changed
	// (any of pct/reset differs from the previously stored snapshot). It is
	// preserved across freshest-by-data tie-break writes that bump TS on
	// identical data every tick (fresher() returns true via the TS tie-break).
	// The staleness "(as of Xm ago)" suffix is computed from DataTS, not TS, so
	// an idle machine whose data never changes correctly ages out. Phase 7.45 B1.
	DataTS int64 `json:"data_ts"`

	// HintStart is the rotating-hint starting index, persisted account-wide so it
	// survives session_id changes and /clear. It rides in this existing global
	// file (no separate store): each new session reads it as its first hint, then
	// BumpHintStart advances it by one (mod hint count). Quota writes preserve it.
	HintStart int `json:"hint_start"`
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

// fresher reports whether incoming snapshot s carries fresher quota data than
// the stored snapshot. Freshness is determined by data quality, not by
// observation timestamp:
//
//  1. Later reset-window wins (s.FiveHourReset > stored.FiveHourReset).
//  2. Same reset-window + higher used% wins (equal window, s.FiveHourPct > stored.FiveHourPct).
//  3. Tie-break by TS when both reset-window and used% are equal (Insurance #3).
//
// The 7-day window is evaluated with the same rule; either dimension being
// fresher is sufficient to accept the incoming snapshot.
func fresher(s, stored Snapshot) bool {
	// 5-hour window comparison.
	if s.FiveHourReset > stored.FiveHourReset {
		return true
	}
	if s.FiveHourReset == stored.FiveHourReset && s.FiveHourPct > stored.FiveHourPct {
		return true
	}
	// 7-day window comparison.
	if s.SevenDayReset > stored.SevenDayReset {
		return true
	}
	if s.SevenDayReset == stored.SevenDayReset && s.SevenDayPct > stored.SevenDayPct {
		return true
	}
	// Window-reset rule: a known reset-window becoming unknown (stored reset > 0,
	// incoming reset == 0) together with a used% drop is a genuine rollover — at the
	// moment a window resets, CC sends used%≈0 with a null resets_at (the next window
	// has not started ticking), so the incoming reset is 0. Accept it so the stale
	// high percentage clears to ~0. Deliberately narrow: it requires the incoming
	// window to be explicitly unknown (== 0), so an idle session carrying an OLD
	// non-zero reset window with a low used% cannot clobber an active snapshot
	// (that path stays governed by the reset-window/used% comparisons above) —
	// preventing the idle-mirage regression that freshest-by-data was built to avoid.
	if stored.FiveHourReset > 0 && s.FiveHourReset == 0 && s.FiveHourPct < stored.FiveHourPct {
		return true
	}
	if stored.SevenDayReset > 0 && s.SevenDayReset == 0 && s.SevenDayPct < stored.SevenDayPct {
		return true
	}
	// Tie-break: newer observation timestamp (Insurance #3 — rollover edge case).
	if s.FiveHourReset == stored.FiveHourReset &&
		s.SevenDayReset == stored.SevenDayReset &&
		s.FiveHourPct == stored.FiveHourPct &&
		s.SevenDayPct == stored.SevenDayPct &&
		s.TS > stored.TS {
		return true
	}
	return false
}

// dataChanged reports whether the quota data of s differs from stored in any
// dimension that matters for staleness: either used-percentage or reset-window.
// Used by Update to decide whether DataTS advances (real change) or is preserved
// (TS tie-break on identical data).
func dataChanged(s, stored Snapshot) bool {
	return s.FiveHourPct != stored.FiveHourPct ||
		s.SevenDayPct != stored.SevenDayPct ||
		s.FiveHourReset != stored.FiveHourReset ||
		s.SevenDayReset != stored.SevenDayReset
}

// Update persists s to disk only if s carries fresher quota data than any
// already-stored snapshot. Freshness is determined by data quality (reset
// window, then used percentage) rather than observation timestamp alone.
// This ensures that an idle session with stale data cannot overwrite a live
// session's fresher snapshot even when the idle observation is more recent.
//
// Write sequence: MkdirAll → lock → read existing → compare → write .tmp → rename → unlock.
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

	// Read existing snapshot to compare freshness.
	existing, err := readFromPath(p)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// Log but continue: on any read/decode error we allow the write.
		slog.Warn("quota.Update: read existing failed; overwriting", "err", err)
	}

	// Freshest-by-data: reject if the incoming snapshot is not fresher.
	if err == nil && !fresher(s, existing) {
		slog.Debug("quota.Update: incoming snapshot not fresher; skipping write",
			"incoming_5h_reset", s.FiveHourReset, "stored_5h_reset", existing.FiveHourReset,
			"incoming_5h_pct", s.FiveHourPct, "stored_5h_pct", existing.FiveHourPct)
		return nil
	}

	// Preserve the hint-rotation offset: it is owned by Bump/HintStart, not by
	// the quota payload, so a quota refresh must not reset it.
	s.HintStart = existing.HintStart

	// DataTS: timestamp of the last actual data change. We only reach this point
	// when fresher() accepted the write — that is either a real data change OR a
	// no-op TS tie-break on identical data (the bug B1 fixes). Preserve DataTS
	// across the tie-break so the staleness age reflects when the numbers last
	// moved, not the write time. err != nil ⇒ no readable prior snapshot ⇒ this
	// observation originates the data.
	switch {
	case err != nil:
		s.DataTS = s.TS
	case dataChanged(s, existing):
		s.DataTS = s.TS
	default:
		s.DataTS = existing.DataTS
		if s.DataTS == 0 {
			s.DataTS = existing.TS // migrate snapshots written before DataTS existed
		}
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

// HintStart returns the persisted rotating-hint starting index (0 when the
// global file is absent or unreadable). Read-only: advancing the offset is the
// job of BumpHintStart.
func HintStart() int {
	p := quotaPath()
	if p == "" {
		return 0
	}
	s, err := readFromPath(p)
	if err != nil {
		return 0
	}
	return s.HintStart
}

// BumpHintStart advances the persisted hint offset by one (mod total) so the
// next session starts on the following hint. It is a no-op when total <= 0 or
// when the global file does not yet exist — the offset rides inside quota.json
// and we deliberately do not create that file just to hold the counter (doing so
// would surface an empty quota block before any rate-limit data arrives).
//
// Read-modify-write under the same flock as Update; quota fields are preserved.
// Fail-soft: errors are logged and swallowed (the offset is disposable).
func BumpHintStart(total int) {
	if total <= 0 {
		return
	}
	p := quotaPath()
	if p == "" {
		return
	}

	fl := flock.New(p + ".lock")
	if err := fl.Lock(); err != nil {
		slog.Warn("quota.BumpHintStart: flock", "err", err)
		return
	}
	defer fl.Unlock() //nolint:errcheck

	existing, err := readFromPath(p)
	if err != nil {
		// Absent file → nothing to ride on yet; skip (see doc above).
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("quota.BumpHintStart: read existing", "err", err)
		}
		return
	}

	existing.HintStart = ((existing.HintStart % total) + 1) % total

	data, err := json.Marshal(existing)
	if err != nil {
		slog.Warn("quota.BumpHintStart: encode", "err", err)
		return
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		slog.Warn("quota.BumpHintStart: write tmp", "err", err)
		return
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		slog.Warn("quota.BumpHintStart: rename", "err", err)
	}
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
