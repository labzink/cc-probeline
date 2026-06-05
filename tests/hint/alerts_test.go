// Package hint_test verifies BuildAlert newest-wins selection and template
// formatting for the Phase 6.95.d alert types.
//
// Removed types: Compact, SendMessageGap, SlowInternal (all merged into
// SubagentCacheExpired or dropped in Phase 6.95.d).
// New type: SubagentCacheExpired (inter-turn gap ≥ 5 min).
// Newest-wins: among live transient events, the one with the largest Timestamp
// is displayed; ConfigError is a persistent fallback.
package hint_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/hint"
	"github.com/labzink/cc-probeline/internal/parser"
)

var alertBase = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// TestBuildAlert_NoEvents_ReturnsEmpty verifies that BuildAlert(nil) returns "".
func TestBuildAlert_NoEvents_ReturnsEmpty(t *testing.T) {
	got := hint.BuildAlert(nil)
	if got != "" {
		t.Errorf("BuildAlert(nil) = %q; want empty string", got)
	}
}

// TestBuildAlert_OrchTTL verifies the plain (no %s) OrchTTL template.
func TestBuildAlert_OrchTTL(t *testing.T) {
	events := []parser.CacheEvent{{Type: parser.OrchTTL, Timestamp: alertBase}}
	got := hint.BuildAlert(events)
	want := "⚠ Cache rebuilt · 60-min idle TTL passed"
	if got != want {
		t.Errorf("BuildAlert(OrchTTL) = %q; want %q", got, want)
	}
}

// TestBuildAlert_ModelSwitched_WithDetail verifies that the ModelSwitched
// template interpolates Detail into the %s slot.
func TestBuildAlert_ModelSwitched_WithDetail(t *testing.T) {
	events := []parser.CacheEvent{
		{Type: parser.ModelSwitched, Detail: "opus-4-7 → sonnet-4-6", Timestamp: alertBase},
	}
	got := hint.BuildAlert(events)
	want := "⚠ Cache rebuilt · model switched (opus-4-7 → sonnet-4-6)"
	if got != want {
		t.Errorf("BuildAlert(ModelSwitched) = %q; want %q", got, want)
	}
}

// TestBuildAlert_SubagentCacheExpired_WithDetail verifies the SubagentCacheExpired
// template interpolates Detail ("<role>:<name>") into the %s slot.
func TestBuildAlert_SubagentCacheExpired_WithDetail(t *testing.T) {
	events := []parser.CacheEvent{
		{Type: parser.SubagentCacheExpired, Detail: "test-writer:RED-6-9c", Timestamp: alertBase},
	}
	got := hint.BuildAlert(events)
	want := "⚠ Subagent test-writer:RED-6-9c cache expired · 5-min gap"
	if got != want {
		t.Errorf("BuildAlert(SubagentCacheExpired) = %q; want %q", got, want)
	}
}

// TestBuildAlert_CompactHeuristic_Plain verifies the plain CompactHeuristic template.
func TestBuildAlert_CompactHeuristic_Plain(t *testing.T) {
	events := []parser.CacheEvent{{Type: parser.CompactHeuristic, Timestamp: alertBase}}
	got := hint.BuildAlert(events)
	want := "⟳ Context compacted · cache rebuilt"
	if got != want {
		t.Errorf("BuildAlert(CompactHeuristic) = %q; want %q", got, want)
	}
}

// TestBuildAlert_NewestWins verifies that among two live transient events,
// the one with the larger Timestamp is displayed (newest-wins).
func TestBuildAlert_NewestWins(t *testing.T) {
	older := alertBase
	newer := alertBase.Add(30 * time.Second)
	events := []parser.CacheEvent{
		{Type: parser.OrchTTL, Timestamp: older},
		{Type: parser.ModelSwitched, Detail: "opus → sonnet", Timestamp: newer},
	}
	got := hint.BuildAlert(events)
	// ModelSwitched has newer timestamp → must win.
	if !strings.Contains(got, "model switched") {
		t.Errorf("BuildAlert(NewestWins): expected ModelSwitched alert; got %q", got)
	}
}

// TestBuildAlert_NewestWins_SameType verifies that among two events of the same
// type, the one with the larger Timestamp wins.
func TestBuildAlert_NewestWins_SameType(t *testing.T) {
	events := []parser.CacheEvent{
		{Type: parser.ModelSwitched, Detail: "a→b", Timestamp: alertBase},
		{Type: parser.ModelSwitched, Detail: "c→d", Timestamp: alertBase.Add(time.Second)},
	}
	got := hint.BuildAlert(events)
	want := "⚠ Cache rebuilt · model switched (c→d)"
	if got != want {
		t.Errorf("BuildAlert(SameType/newest): got %q; want %q", got, want)
	}
}

// TestBuildAlert_ConfigError_Fallback verifies that ConfigError is returned when
// no transient events are present.
func TestBuildAlert_ConfigError_Fallback(t *testing.T) {
	events := []parser.CacheEvent{{Type: parser.ConfigError}}
	got := hint.BuildAlert(events)
	want := "⚠ Config error · run cc-probeline check-config"
	if got != want {
		t.Errorf("BuildAlert(ConfigError) = %q; want %q", got, want)
	}
}

// TestBuildAlert_TransientBeatsConfigError verifies that a live transient event
// takes priority over ConfigError (transient wins, ConfigError is fallback).
func TestBuildAlert_TransientBeatsConfigError(t *testing.T) {
	events := []parser.CacheEvent{
		{Type: parser.ConfigError},
		{Type: parser.OrchTTL, Timestamp: alertBase},
	}
	got := hint.BuildAlert(events)
	// Transient (OrchTTL) must win over ConfigError.
	if !strings.Contains(got, "60-min idle TTL") {
		t.Errorf("BuildAlert(OrchTTL+ConfigError): expected OrchTTL alert; got %q", got)
	}
}

// TestBuildAlert_CompactHeuristic_WithDetail_NoArtefact verifies that
// CompactHeuristic (no %s in template) returns the exact template text even
// when CacheEvent.Detail is non-empty (no fmt artefacts).
func TestBuildAlert_CompactHeuristic_WithDetail_NoArtefact(t *testing.T) {
	events := []parser.CacheEvent{
		{Type: parser.CompactHeuristic, Detail: "ignored-since-no-verb", Timestamp: alertBase},
	}
	got := hint.BuildAlert(events)
	want := hint.AlertTexts[parser.CompactHeuristic]
	if got != want {
		t.Errorf("BuildAlert(CompactHeuristic+Detail) mismatch:\n  got:  %q\n  want: %q", got, want)
	}
	if strings.Contains(got, "%!") {
		t.Errorf("BuildAlert(CompactHeuristic+Detail) produced fmt artefact: %q", got)
	}
}

// TestBuildAlert_SubagentCacheExpired_AgentIDFallback verifies that when Detail
// is just an AgentID (no "<role>:<name>" enrichment), it is interpolated correctly.
func TestBuildAlert_SubagentCacheExpired_AgentIDFallback(t *testing.T) {
	events := []parser.CacheEvent{
		{Type: parser.SubagentCacheExpired, Detail: "agent-abc123", Timestamp: alertBase},
	}
	got := hint.BuildAlert(events)
	if !strings.Contains(got, "agent-abc123") {
		t.Errorf("BuildAlert(SubagentCacheExpired): Detail not interpolated; got %q", got)
	}
	if strings.Contains(got, "%!") {
		t.Errorf("BuildAlert(SubagentCacheExpired) produced fmt artefact: %q", got)
	}
	if strings.Contains(got, "%s") {
		t.Errorf("BuildAlert(SubagentCacheExpired): raw %%s in output; got %q", got)
	}
}
