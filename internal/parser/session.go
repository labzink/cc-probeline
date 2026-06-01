package parser

import (
	"strings"
	"time"
)

// SessionStats is a single-pass snapshot of a parsed session.
// Populated by Aggregate. Zero value (TurnCount == 0) means empty session.
// PerModel and Turns are nil (not empty containers) when there are no records.
type SessionStats struct {
	// Token aggregates
	Totals   TokenCounts
	PerModel map[string]TokenCounts

	// Time window
	FirstTimestamp time.Time
	LastTimestamp  time.Time

	// Counters
	TurnCount    int
	ToolUseCount int

	// Per-record snapshots for Phase 4.2 box-drawing table. Same length and
	// order as the deduplicated input []Record.
	Turns []Turn

	// Cache invalidation events detected over Turns. Populated by Aggregate
	// via DetectCacheEvents (implemented in Phase 4.4.a).
	CacheEvents []CacheEvent
}

// isUserTextRecord reports whether r is a user record with textual (non-tool-result)
// content. Such records mark prompt boundaries for GroupID assignment (spec §2.3,
// Insurance #1: any user-text record = new group).
//
// A user-text record has Role=="user" and content that is either empty or a plain
// string. Tool-result records have content as a list with type "tool_result".
// We detect this by checking that none of the ContentBlocks has type "tool_result".
func isUserTextRecord(r Record) bool {
	if r.Type != "user" {
		return false
	}
	// If content blocks exist, verify no tool_result is present.
	for _, c := range r.Content {
		if c.Type == "tool_result" {
			return false
		}
	}
	return true
}

// Aggregate computes a SessionStats from a []Record that may contain both user
// and assistant records. User records with textual content are used as prompt
// boundaries for GroupID assignment (spec §2.3). Only assistant records contribute
// to Turns, token totals, and PerModel counts.
//
// Phase 6.8.0 additions:
//   - Turn.UUID is set from Record.UUID.
//   - Turn.GroupID is set to the 1-based index of the most recent user-text boundary
//     for orchestrator turns (IsSidechain=false); sidechain turns get GroupID=0.
//   - Turn.Thinking is true when the record had a thinking-block AND no tool_use block.
//
// Pure function: no I/O, no logging, no mutation of the input slice.
// Nil or empty input returns SessionStats{} (zero value, PerModel == nil).
func Aggregate(records []Record) SessionStats {
	if len(records) == 0 {
		return SessionStats{}
	}

	var s SessionStats
	// Pre-allocate for typical session: Opus orchestrator + Sonnet/Haiku subagent + spare.
	s.PerModel = make(map[string]TokenCounts, 4)
	s.Turns = make([]Turn, 0, len(records))

	// groupID tracks the current 1-based prompt group index.
	// Incremented on every user-text boundary record.
	groupID := 0

	// prevTimestamp is used to compute Turn.Duration. We track it across
	// all assistant records only (not user boundaries).
	var prevTimestamp time.Time
	turnIndex := 0 // 1-based index among assistant records only

	for _, rec := range records {
		// User-text records mark prompt boundaries; they do not produce a Turn.
		if isUserTextRecord(rec) {
			groupID++
			continue
		}

		// Skip all other non-assistant records (e.g. user tool-result records).
		if rec.Type != "assistant" {
			continue
		}

		key := CanonicalModelKey(rec.Model)

		cur := s.PerModel[key]
		cur.Input += rec.Usage.Input
		cur.Output += rec.Usage.Output
		cur.CacheRead += rec.Usage.CacheRead
		cur.CacheCreate += rec.Usage.CacheCreate
		cur.CacheCreate5m += rec.Usage.CacheCreate5m
		cur.CacheCreate1h += rec.Usage.CacheCreate1h
		s.PerModel[key] = cur

		s.Totals.Input += rec.Usage.Input
		s.Totals.Output += rec.Usage.Output
		s.Totals.CacheRead += rec.Usage.CacheRead
		s.Totals.CacheCreate += rec.Usage.CacheCreate
		s.Totals.CacheCreate5m += rec.Usage.CacheCreate5m
		s.Totals.CacheCreate1h += rec.Usage.CacheCreate1h

		s.TurnCount++

		hasThinking := false
		hasToolUse := false
		toolUse := ""
		for _, c := range rec.Content {
			switch c.Type {
			case "thinking":
				hasThinking = true
			case "tool_use":
				s.ToolUseCount++
				hasToolUse = true
				if toolUse == "" {
					toolUse = c.ToolName
				}
			}
		}

		role := "orchestrator"
		if rec.IsSidechain {
			role = "agent"
		}

		// Sidechain turns do not get a GroupID here; it will be assigned
		// during the merge step in 6.8.d based on timestamp.
		turnGroupID := 0
		if !rec.IsSidechain {
			turnGroupID = groupID
		}

		turnIndex++
		var dur time.Duration
		if turnIndex > 1 {
			dur = rec.Timestamp.Sub(prevTimestamp)
		}
		s.Turns = append(s.Turns, Turn{
			Index:       turnIndex,
			Role:        role,
			Model:       key,
			Tokens:      rec.Usage,
			ToolUse:     toolUse,
			Timestamp:   rec.Timestamp,
			Duration:    dur,
			IsSidechain: rec.IsSidechain,
			UUID:        rec.UUID,
			GroupID:     turnGroupID,
			Thinking:    hasThinking && !hasToolUse,
		})
		prevTimestamp = rec.Timestamp

		if s.FirstTimestamp.IsZero() {
			s.FirstTimestamp = rec.Timestamp
		}
		s.LastTimestamp = rec.Timestamp
	}

	if len(s.Turns) > 0 {
		s.CacheEvents = DetectCacheEvents(s.Turns, time.Now())
	}
	return s
}

// CanonicalModelKey returns the short model name used as a PerModel key.
// Examples: "claude-opus-4-7-20250805" -> "opus-4-7", "" -> "unknown".
func CanonicalModelKey(rawModel string) string {
	if rawModel == "" {
		return "unknown"
	}
	const prefix = "claude-"
	if !strings.HasPrefix(rawModel, prefix) {
		return rawModel
	}
	// Strip "claude-" prefix and keep the first 3 dash-separated segments.
	trimmed := rawModel[len(prefix):]
	if trimmed == "" {
		return "unknown"
	}
	parts := strings.SplitN(trimmed, "-", 4)
	if len(parts) <= 3 {
		return trimmed
	}
	return parts[0] + "-" + parts[1] + "-" + parts[2]
}
