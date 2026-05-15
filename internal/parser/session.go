package parser

import (
	"strings"
	"time"
)

// SessionStats is a single-pass snapshot of a parsed session.
// Populated by Aggregate. Zero value (TurnCount == 0) means empty session.
// PerModel is nil (not an empty map) when there are no records.
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
}

// Aggregate computes a SessionStats from a deduplicated, sorted []Record
// (the output of ParseLines). Pure function: no I/O, no logging, no mutation
// of the input slice.
//
// Upstream contract: records are sorted ASC by Timestamp and deduplicated;
// only assistant records with non-empty usage are present.
// Nil or empty input returns SessionStats{} (zero value, PerModel == nil).
func Aggregate(records []Record) SessionStats {
	if len(records) == 0 {
		return SessionStats{}
	}

	var s SessionStats
	s.PerModel = make(map[string]TokenCounts, 4)
	s.FirstTimestamp = records[0].Timestamp

	for _, rec := range records {
		key := canonicalModelKey(rec.Model)

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

		for _, c := range rec.Content {
			if c.Type == "tool_use" {
				s.ToolUseCount++
			}
		}
	}

	s.LastTimestamp = records[len(records)-1].Timestamp
	return s
}

// canonicalModelKey returns the short model name used as a PerModel key.
// Examples: "claude-opus-4-7-20250805" -> "opus-4-7", "" -> "unknown".
func canonicalModelKey(rawModel string) string {
	if rawModel == "" {
		return "unknown"
	}
	const prefix = "claude-"
	if !strings.HasPrefix(rawModel, prefix) {
		return rawModel
	}
	// Strip "claude-" prefix and keep the first 3 dash-separated segments.
	trimmed := rawModel[len(prefix):]
	parts := strings.SplitN(trimmed, "-", 4)
	if len(parts) <= 3 {
		return trimmed
	}
	return parts[0] + "-" + parts[1] + "-" + parts[2]
}
