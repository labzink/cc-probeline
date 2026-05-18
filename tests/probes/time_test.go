// Package probes_test — black-box tests for TimeProbe.
// Covers Full/Compact/Minimal rendering and zero-duration behaviour.
package probes_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
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
		name              string
		totalAPIDurationMS int64
		want              string
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
			got := p.Render(makeTimeData(tc.totalAPIDurationMS), th, probes.LevelFull)
			if got != tc.want {
				t.Errorf("Render(Full, %dms): want %q, got %q",
					tc.totalAPIDurationMS, tc.want, got)
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
		name              string
		totalAPIDurationMS int64
		want              string
	}{
		{"49m58s", 2998000, "49:58"},
		{"1m00s", 60000, "01:00"},
		{"59m59s", 3599000, "59:59"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := p.Render(makeTimeData(tc.totalAPIDurationMS), th, probes.LevelCompact)
			if got != tc.want {
				t.Errorf("Render(Compact, %dms): want %q, got %q",
					tc.totalAPIDurationMS, tc.want, got)
			}
		})
	}
}

// TestTime_Render_Minimal verifies that LevelMinimal returns an empty string
// (the entire time block is dropped; renderer will remove the separator).
// Visible() remains true — the probe is present but renders nothing at Minimal.
func TestTime_Render_Minimal(t *testing.T) {
	p := &probes.TimeProbe{}
	th := renderer.Theme{}
	cfg := probes.Config{}

	tests := []struct {
		name              string
		totalAPIDurationMS int64
	}{
		{"typical", 2998000},
		{"one minute", 60000},
		{"large", 3600000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := makeTimeData(tc.totalAPIDurationMS)
			got := p.Render(d, th, probes.LevelMinimal)
			if got != "" {
				t.Errorf("Render(Minimal, %dms): want %q (empty), got %q",
					tc.totalAPIDurationMS, "", got)
			}
			// Visible must still be true so the renderer knows the probe exists
			// (it just chooses to hide the block at Minimal).
			if vis := p.Visible(d, cfg); !vis {
				t.Errorf("Visible(Minimal, %dms): want true, got false",
					tc.totalAPIDurationMS)
			}
		})
	}
}

// TestTime_Render_Zero verifies rendering when TotalAPIDurationMS is 0.
// Per §4.1.a concept: zero duration → Full/Compact render "time: 00:00"/"00:00";
// Minimal always returns "".
// Visible() is true even at zero (cost block is always shown).
func TestTime_Render_Zero(t *testing.T) {
	p := &probes.TimeProbe{}
	th := renderer.Theme{}
	cfg := probes.Config{}
	d := makeTimeData(0)

	if got := p.Render(d, th, probes.LevelFull); got != "time: 00:00" {
		t.Errorf("Render(Full, 0ms): want %q, got %q", "time: 00:00", got)
	}
	if got := p.Render(d, th, probes.LevelCompact); got != "00:00" {
		t.Errorf("Render(Compact, 0ms): want %q, got %q", "00:00", got)
	}
	if got := p.Render(d, th, probes.LevelMinimal); got != "" {
		t.Errorf("Render(Minimal, 0ms): want %q (empty), got %q", "", got)
	}
	if vis := p.Visible(d, cfg); !vis {
		t.Errorf("Visible(0ms): want true, got false")
	}
}
