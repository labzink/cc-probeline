// Package probes_test — black-box tests for TimeProbe.
// Covers Full/Compact/Minimal rendering and zero-duration behaviour.
package probes_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/state"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// makeTimeData is a helper that constructs Data with TotalAPIDurationMS set.
func makeTimeData(totalAPIDurationMS int64) probes.Data {
	return probes.Data{
		Stdin: stdin.Payload{
			Cost: stdin.Cost{TotalAPIDurationMS: totalAPIDurationMS},
		},
	}
}

// TestTime_Render_Full verifies that LevelFull produces "time: MM:SS".
func TestTime_Render_Full(t *testing.T) {
	p := &probes.TimeProbe{}
	th := renderer.Theme{}

	tests := []struct {
		name               string
		totalAPIDurationMS int64
		want               string
	}{
		// 2998000 ms = 2998 s = 49 min 58 s
		{"49m58s", 2998000, "time: 49:58"},
		// 60000 ms = 60 s = 1 min 0 s
		{"1m00s", 60000, "time: 01:00"},
		// 3599000 ms = 3599 s = 59 min 59 s
		{"59m59s", 3599000, "time: 59:59"},
		// 61000 ms = 61 s = 1 min 1 s
		{"1m01s", 61000, "time: 01:01"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := probes.Config{}
			got := p.Render(makeTimeData(tc.totalAPIDurationMS), cfg, th, probes.LevelFull)
			if got != tc.want {
				t.Errorf("Render(Full, %dms): want %q, got %q",
					tc.totalAPIDurationMS, tc.want, got)
			}
		})
	}
}

// TestTime_Render_ResetOnClear verifies that when state IS loaded (d.State != nil)
// a zero SessionDurMS renders as "00:00" instead of falling back to the raw
// cumulative TotalAPIDurationMS. This is the /clear reset: cost shows $0.00 and
// time must show 00:00, not the stale pre-clear total. Also checks that a positive
// SessionDurMS wins over the raw total, and that a negative delta clamps to 00:00.
func TestTime_Render_ResetOnClear(t *testing.T) {
	p := &probes.TimeProbe{}
	th := renderer.Theme{}
	cfg := probes.Config{}

	tests := []struct {
		name         string
		sessionDurMS int64
		rawTotalMS   int64
		want         string
	}{
		// /clear: state loaded, delta 0, stale raw total must NOT resurface.
		{"reset-zero-ignores-raw", 0, 14297000, "time: 00:00"},
		// state loaded, real delta wins over raw total.
		{"delta-wins-over-raw", 60000, 14297000, "time: 01:00"},
		// transient negative delta clamps to zero.
		{"negative-clamps", -5000, 14297000, "time: 00:00"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := probes.Data{
				Stdin:        stdin.Payload{Cost: stdin.Cost{TotalAPIDurationMS: tc.rawTotalMS}},
				State:        &state.Session{},
				SessionDurMS: tc.sessionDurMS,
			}
			got := p.Render(d, cfg, th, probes.LevelFull)
			if got != tc.want {
				t.Errorf("Render(Full, dur=%d raw=%d): want %q, got %q",
					tc.sessionDurMS, tc.rawTotalMS, tc.want, got)
			}
		})
	}
}

// TestTime_Render_Compact verifies that LevelCompact produces "MM:SS"
// (label "time: " is dropped).
func TestTime_Render_Compact(t *testing.T) {
	p := &probes.TimeProbe{}
	th := renderer.Theme{}

	tests := []struct {
		name               string
		totalAPIDurationMS int64
		want               string
	}{
		{"49m58s", 2998000, "49:58"},
		{"1m00s", 60000, "01:00"},
		{"59m59s", 3599000, "59:59"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := probes.Config{}
			got := p.Render(makeTimeData(tc.totalAPIDurationMS), cfg, th, probes.LevelCompact)
			if got != tc.want {
				t.Errorf("Render(Compact, %dms): want %q, got %q",
					tc.totalAPIDurationMS, tc.want, got)
			}
		})
	}
}

// TestTime_Render_Minimal verifies that LevelMinimal returns "MM:SS" (non-empty).
// Phase 6.6: Minimal is no longer dropped; time is rendered as "MM:SS" at all levels.
func TestTime_Render_Minimal(t *testing.T) {
	p := &probes.TimeProbe{}
	th := renderer.Theme{}
	cfg := cfgAllOn()

	tests := []struct {
		name               string
		totalAPIDurationMS int64
		want               string
	}{
		{"typical", 2998000, "49:58"},
		{"one minute", 60000, "01:00"},
		{"large", 3600000, "60:00"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := makeTimeData(tc.totalAPIDurationMS)
			got := p.Render(d, cfg, th, probes.LevelMinimal)
			// Phase 6.6: Minimal returns MM:SS, not empty string.
			if got != tc.want {
				t.Errorf("Render(Minimal, %dms): want %q, got %q",
					tc.totalAPIDurationMS, tc.want, got)
			}
			if vis := p.Visible(d, cfg); !vis {
				t.Errorf("Visible(Minimal, %dms): want true, got false",
					tc.totalAPIDurationMS)
			}
		})
	}
}

// TestTime_Render_Zero verifies rendering when TotalAPIDurationMS is 0.
// Phase 6.6: zero duration → Full "time: 00:00", Compact "00:00", Minimal "00:00".
// Visible() is true even at zero (cost block is always shown).
func TestTime_Render_Zero(t *testing.T) {
	p := &probes.TimeProbe{}
	th := renderer.Theme{}
	cfg := cfgAllOn()
	d := makeTimeData(0)

	zeroCfg := cfgAllOn()
	if got := p.Render(d, zeroCfg, th, probes.LevelFull); got != "time: 00:00" {
		t.Errorf("Render(Full, 0ms): want %q, got %q", "time: 00:00", got)
	}
	if got := p.Render(d, zeroCfg, th, probes.LevelCompact); got != "00:00" {
		t.Errorf("Render(Compact, 0ms): want %q, got %q", "00:00", got)
	}
	// Phase 6.6: Minimal returns MM:SS, not empty.
	if got := p.Render(d, zeroCfg, th, probes.LevelMinimal); got != "00:00" {
		t.Errorf("Render(Minimal, 0ms): want %q, got %q", "00:00", got)
	}
	if vis := p.Visible(d, cfg); !vis {
		t.Errorf("Visible(0ms): want true, got false")
	}
}
