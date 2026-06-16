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
	"time"

	"github.com/gofrs/flock"
	"github.com/labzink/cc-probeline/internal/hint"
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

	// BaselineTurnTime is the timestamp of the newest turn present at the first
	// Reconcile call (observation start). Turns strictly newer than this are the
	// "in-session" pool that shares SessionTotal; turns at or before it predate
	// observation (their cost is folded into BaselineCost) and render "—".
	// Phase 7.45 B2: replaces the incremental PerTurnCost accumulation.
	BaselineTurnTime time.Time `json:"baseline_turn_time"`

	// LastSeenTotal is the last ccTotal value passed to Reconcile. Monotonic
	// high-water mark; used only to baseline prompt groups for LastRequest.
	LastSeenTotal float64

	// PerTurnCost maps turn UUID to its USD cost for the current render. Phase
	// 7.45 B2: recomputed from scratch each Reconcile (SessionTotal × weighted
	// share), no longer an immutable accumulation. Persisted as a transport cache
	// (overwritten every tick).
	PerTurnCost map[string]float64

	// ReconCost maps an orchestrator turn UUID to a reconstructed surcharge for a
	// turn that CC billed but did not write to the JSONL as its own record (Phase
	// 7.46). Detected from the cache chain: when a turn's cache_read exceeds the
	// previous orchestrator turn's read+write by rdChk>1 tokens, a "missing" turn
	// ran between them — it re-read the full context and wrote a small diff. Its
	// cost is added to this turn's displayed per-turn cost (PerTurn) so the table
	// attribution is closer to reality. Derived every Reconcile from the chain,
	// never persisted (json:"-"); the header uses the official ccTotal, not this.
	ReconCost map[string]float64 `json:"-"`

	// PromptCost maps GroupID (1-based) to the ccTotal at the start of that
	// prompt group. Used to compute LastRequest = ccTotal − PromptCost[group].
	PromptCost map[int]float64

	// LastGoodGit is the most recent successfully detected git status for this
	// session. Used as a fallback when DetectGit fails (anti-flicker).
	LastGoodGit *parser.GitStatus

	// HintRotation persists the rotating-hint widget state (which hints have
	// been shown, current index, last switch time). Phase 6.95.b consolidated
	// this here, retiring the separate ~/.cache/cc-probeline/hint-<sid>.json.
	// Disposable: loss only resets the hint rotation, never costs/quota data.
	HintRotation hint.State `json:"hint_rotation"`

	// CommitBadge tracks the transient "✓ N committed" git badge (Phase 6.95.a).
	// Set when the working tree's modified-file count drops from N>0 to 0; shown
	// for exactly one refresh, then cleared. See CommitBadgeTick.
	CommitBadge CommitBadge `json:"commit_badge"`

	// OverageBaseline is the SessionTotal captured the moment the account first
	// crossed a rate-limit window into paid extra usage (Phase 6.95.h). The
	// overage shown is SessionTotal − OverageBaseline. Valid only while
	// OverageActive; reset to 0 when the badge clears. See ExtraUsageTick.
	OverageBaseline float64 `json:"overage_baseline"`

	// OverageActive reports whether the extra-usage badge is currently armed: a
	// rate-limit window is at ≥100% AND ~/.claude.json has hasExtraUsageEnabled.
	OverageActive bool `json:"overage_active"`

	// PrevQuotaPct and PrevQuotaTotal record the previous refresh's binding quota
	// percentage (max of the two windows) and SessionTotal. They let the first
	// 100%-crossing count only the portion of the crossing turn's cost that lies
	// above the 100% line, proportional to how far past 100 the window moved this
	// tick (Phase 7.45 B4). Recorded every tick, independent of the badge state.
	PrevQuotaPct   float64 `json:"prev_quota_pct"`
	PrevQuotaTotal float64 `json:"prev_quota_total"`

	// CostHistory records the official Anthropic ccTotal stepwise — one sample
	// each time ccTotal advances (identical 5-second ticks are not duplicated),
	// alongside our own running estimate and the turn count at that moment. It is
	// a diagnostic trail (Phase 7.46) for reconciling our table-driven estimate
	// against CC's official meter: it reveals when the official sum "appears"
	// relative to turns (lag) and which step a discrepancy enters. Capped length.
	CostHistory []CostSample `json:"cost_history"`
}

// CostSample is one entry in Session.CostHistory: the official ccTotal at a tick
// where it advanced, our running estimate (Σ PerTurnCost) at that tick, the API
// duration timestamp, the turn count, and the newest turn's UUID (to correlate a
// jump in the official sum with the turn that triggered it).
type CostSample struct {
	DurMS      int64   `json:"dur_ms"`
	CCTotal    float64 `json:"cc_total"`
	Estimate   float64 `json:"estimate"`
	Turns      int     `json:"turns"`
	NewestTurn string  `json:"newest_turn"`

	// Phase 7.46 debug instrumentation — populated only when
	// CC_PROBELINE_COST_DEBUG=1, otherwise omitted (release JSON is unchanged).
	// Purpose: accumulate, over real daily use, the data needed to locate any
	// residual cc_total↔estimate gap offline. An ordinary-least-squares fit of
	// cc_total on Tokens across many samples recovers CC's true per-class prices
	// (so a mispriced class shows up directly); EstMain/EstSub split the estimate
	// into orchestrator vs subagent (to catch subagent-accounting gaps); ByModel
	// breaks the token vector down per model (to catch mixed-pool errors).
	EstMain float64             `json:"est_main,omitempty"`
	EstSub  float64             `json:"est_sub,omitempty"`
	Tokens  *TokenVec           `json:"tokens,omitempty"`
	ByModel map[string]TokenVec `json:"by_model,omitempty"`
}

// TokenVec is a per-token-class count vector used by the cost debug
// instrumentation (Phase 7.46): input, cache-read, 5-minute cache-write,
// 1-hour cache-write, and output tokens. The two cache-write classes use the
// same TTL split/inference as the cost estimator (parsed 5m/1h fields, with an
// author-based fallback when CC reports only a lumped cache_creation count).
type TokenVec struct {
	In   int `json:"in,omitempty"`
	Read int `json:"read,omitempty"`
	W5   int `json:"w5,omitempty"`
	W1   int `json:"w1,omitempty"`
	Out  int `json:"out,omitempty"`
}

// CommitBadge is the transient post-commit indicator state. Count is the number
// of files that were just committed; Shown records whether the badge has already
// been rendered (so it appears for a single refresh and then disappears).
type CommitBadge struct {
	Count int  `json:"count"`
	Shown bool `json:"shown"`
}

// CommitBadgeTick advances the commit-badge state for one refresh and returns
// the badge count to display now (0 = render nothing).
//
// Trigger: a prevModified>0 → currModified==0 transition (only when gitOK, i.e.
// the current git status was detected successfully). On trigger the badge is
// armed with Count=prevModified. The badge is shown for exactly one refresh
// (the call that first sees Count>0 && !Shown) and cleared on the following tick.
func (s *Session) CommitBadgeTick(prevModified, currModified int, gitOK bool) int {
	if gitOK && prevModified > 0 && currModified == 0 {
		s.CommitBadge = CommitBadge{Count: prevModified, Shown: false}
	}
	if s.CommitBadge.Count > 0 && !s.CommitBadge.Shown {
		s.CommitBadge.Shown = true
		return s.CommitBadge.Count
	}
	if s.CommitBadge.Shown {
		s.CommitBadge = CommitBadge{}
	}
	return 0
}

// ExtraUsageTick advances the extra-usage (paid overage) state for one refresh
// and returns whether the badge is active and the overage USD to display.
//
// pct is the binding rate-limit percentage (max of the 5h/7d windows); the badge
// arms when pct ≥ 100 AND hasExtra (~/.claude.json hasExtraUsageEnabled).
//
// Phase 7.45 B4 — proportional crossing tail. On the first refresh that crosses
// 100%, the cost added this tick (sessionTotal − prevTotal) is the crossing
// turn's cost; only the fraction of it that lies above the 100% line counts as
// extra:
//
//	tail = (sessionTotal − prevTotal) × (pct − 100) / (pct − prevPct)
//
// so the baseline is sessionTotal − tail (not the full sessionTotal as before,
// which silently dropped the crossing turn's overage). If CC clips pct at 100 the
// fraction is 0 → tail 0 → identical to the old behaviour. The tail is only taken
// when a genuine sub-100 previous reading exists (prevPct in (0,100)); a cold
// start (prevPct == 0) or an already-over window takes no tail.
//
// When the trigger is false the badge clears and the baseline resets to 0 —
// recomputed every refresh, never sticky.
func (s *Session) ExtraUsageTick(sessionTotal, pct float64, hasExtra bool) (active bool, usd float64) {
	prevPct, prevTotal := s.PrevQuotaPct, s.PrevQuotaTotal
	// Record this tick for the next call — always, so the reading immediately
	// before a crossing is available regardless of badge state.
	s.PrevQuotaPct, s.PrevQuotaTotal = pct, sessionTotal

	if pct >= 100 && hasExtra {
		if !s.OverageActive {
			tail := 0.0
			if prevPct > 0 && prevPct < 100 && pct > prevPct {
				if turnCost := sessionTotal - prevTotal; turnCost > 0 {
					tail = turnCost * (pct - 100) / (pct - prevPct)
				}
			}
			s.OverageBaseline = sessionTotal - tail
			s.OverageActive = true
		}
		over := sessionTotal - s.OverageBaseline
		if over < 0 {
			over = 0
		}
		return true, over
	}
	s.OverageActive = false
	s.OverageBaseline = 0
	return false, 0
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
