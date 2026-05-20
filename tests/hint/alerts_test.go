// Package hint_test verifies the BuildAlert priority selection and template
// formatting for all six parser.CacheEventType values.
//
// §4.4.b Hint widget + State — RED phase.
// All tests fail because internal/hint/alerts.go is a stub (BuildAlert always
// returns "").
package hint_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/hint"
	"github.com/labzink/cc-probeline/internal/parser"
)

// TestBuildAlert_NoEvents_ReturnsEmpty verifies that BuildAlert(nil) returns "".
func TestBuildAlert_NoEvents_ReturnsEmpty(t *testing.T) {
	got := hint.BuildAlert(nil)
	if got != "" {
		t.Errorf("BuildAlert(nil) = %q; want empty string", got)
	}
}

// TestBuildAlert_OrchTTL verifies the plain (no %s) OrchTTL template.
func TestBuildAlert_OrchTTL(t *testing.T) {
	events := []parser.CacheEvent{{Type: parser.OrchTTL}}
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
		{Type: parser.ModelSwitched, Detail: "opus-4-7 → sonnet-4-6"},
	}
	got := hint.BuildAlert(events)
	want := "⚠ Cache rebuilt · model switched (opus-4-7 → sonnet-4-6)"
	if got != want {
		t.Errorf("BuildAlert(ModelSwitched) = %q; want %q", got, want)
	}
}

// TestBuildAlert_CompactNormal verifies the plain Compact template text.
func TestBuildAlert_CompactNormal(t *testing.T) {
	events := []parser.CacheEvent{{Type: parser.Compact}}
	got := hint.BuildAlert(events)
	want := "Cache rebuilt by /compact (normal)"
	if got != want {
		t.Errorf("BuildAlert(Compact) = %q; want %q", got, want)
	}
}

// TestBuildAlert_MultipleEvents_HigherCriticalWins verifies that when both
// Compact and OrchTTL events are present, OrchTTL (higher in criticalTypes)
// wins.
func TestBuildAlert_MultipleEvents_HigherCriticalWins(t *testing.T) {
	events := []parser.CacheEvent{
		{Type: parser.Compact},
		{Type: parser.OrchTTL},
	}
	got := hint.BuildAlert(events)
	want := "⚠ Cache rebuilt · 60-min idle TTL passed"
	if got != want {
		t.Errorf("BuildAlert(Compact+OrchTTL) = %q; want %q", got, want)
	}
}

// TestBuildAlert_SameTypeMultiple_LastWins verifies that when two ModelSwitched
// events exist, the last one in slice order wins (most recent).
func TestBuildAlert_SameTypeMultiple_LastWins(t *testing.T) {
	events := []parser.CacheEvent{
		{Type: parser.ModelSwitched, Detail: "a→b"},
		{Type: parser.ModelSwitched, Detail: "c→d"},
	}
	got := hint.BuildAlert(events)
	want := "⚠ Cache rebuilt · model switched (c→d)"
	if got != want {
		t.Errorf("BuildAlert(SameType/last) = %q; want %q", got, want)
	}
}

// TestBuildAlert_SubagentEvent_Format verifies that a SendMessageGap event
// interpolates Detail (subagent ID) into the %s slot.
func TestBuildAlert_SubagentEvent_Format(t *testing.T) {
	events := []parser.CacheEvent{
		{Type: parser.SendMessageGap, Detail: "3"},
	}
	got := hint.BuildAlert(events)
	want := "⚠ Subagent#3 cache lost · 5-min SendMessage gap"
	if got != want {
		t.Errorf("BuildAlert(SendMessageGap) = %q; want %q", got, want)
	}
}
