// Package state persists per-session data across plugin invocations.
// Each session is stored as a JSON file named "<sessionID>.json" under
// the state directory.
//
// Path resolution (in priority order):
//  1. CC_PROBELINE_STATE_DIR env var (used by tests for isolation).
//  2. XDG_DATA_HOME/cc-probeline/state/ (XDG standard).
//  3. ~/.local/share/cc-probeline/state/ (fallback).
//
// Write durability: Save uses atomic .tmp+rename guarded by a flock on
// <path>.lock, matching the pattern from internal/mode/mode.go.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
	"github.com/labzink/cc-probeline/internal/parser"
)

// Session is the persisted per-session state keyed by session_id.
// Zero value (Initialized=false) is valid and means "not yet seen".
type Session struct {
	// Initialized is set to true on the first Reconcile call for this session.
	// When false, BaselineCost has not been captured yet.
	Initialized bool

	// BaselineCost is the ccTotal snapshot captured on the first Reconcile
	// call. Delta cost = ccTotal − BaselineCost. Resets when session_id changes
	// (/clear creates a new session_id, so a new file is created).
	BaselineCost float64

	// BaselineDurMS is the session duration (ms) captured alongside BaselineCost.
	BaselineDurMS int64

	// LastSeenTotal is the last ccTotal value passed to Reconcile. Used to
	// compute the delta for per-turn cost distribution.
	LastSeenTotal float64

	// PerTurnCost maps turn UUID to its finalized USD cost. Once a turn is
	// recorded it is never re-computed (immutable by design).
	PerTurnCost map[string]float64

	// PromptCost maps GroupID (1-based) to the ccTotal at the start of that
	// prompt group. Used to compute LastRequest = ccTotal − PromptCost[group].
	PromptCost map[int]float64

	// LastGoodGit is the most recent successfully detected git status for this
	// session. Used as a fallback when DetectGit fails (anti-flicker).
	LastGoodGit *parser.GitStatus
}

// stateDir resolves the directory used to store state files.
// Priority: CC_PROBELINE_STATE_DIR → XDG_DATA_HOME → ~/.local/share.
func stateDir() string {
	if dir := os.Getenv("CC_PROBELINE_STATE_DIR"); dir != "" {
		return dir
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "cc-probeline", "state")
	}
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share", "cc-probeline", "state")
}

// statePath returns the full path for the JSON file of the given sessionID.
// Returns "" when the state directory cannot be determined.
func statePath(sessionID string) string {
	dir := stateDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, sessionID+".json")
}

// Load reads the persisted Session for the given sessionID from disk.
// Returns a non-nil zero Session when the file does not exist or cannot be read.
// Any I/O or JSON decode error is logged and treated as a fresh session.
func Load(sessionID string) *Session {
	slog.Debug("state.Load start", "sessionID", sessionID)

	p := statePath(sessionID)
	if p == "" {
		slog.Warn("state.Load: state dir unavailable", "sessionID", sessionID)
		return &Session{}
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("state.Load: read failed", "path", p, "err", err)
		}
		return &Session{}
	}

	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		slog.Error("state.Load: decode failed", "path", p, "err", err)
		return &Session{}
	}

	slog.Debug("state.Load complete", "sessionID", sessionID, "initialized", s.Initialized)
	return &s
}

// Save atomically persists s as the state for the given sessionID.
// Write sequence: MkdirAll → encode to <path>.tmp → rename to <path>.
// The write is guarded by a flock on <path>.lock to prevent concurrent writes.
func Save(sessionID string, s *Session) error {
	slog.Debug("state.Save start", "sessionID", sessionID)

	p := statePath(sessionID)
	if p == "" {
		return fmt.Errorf("state.Save: state dir unavailable (HOME not set?)")
	}

	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("state.Save: mkdir %q: %w", dir, err)
	}

	// Acquire exclusive lock on a separate .lock file (stable inode, never removed).
	fl := flock.New(p + ".lock")
	if err := fl.Lock(); err != nil {
		return fmt.Errorf("state.Save: flock: %w", err)
	}
	defer fl.Unlock() //nolint:errcheck

	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("state.Save: encode: %w", err)
	}

	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("state.Save: write tmp: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("state.Save: rename: %w", err)
	}

	slog.Debug("state.Save complete", "sessionID", sessionID, "path", p)
	return nil
}
