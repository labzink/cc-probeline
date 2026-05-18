// Package probes_test — black-box tests for QuotaProbe.
// Covers visibility (QuotaEnabled toggle) and rendering across all three
// display levels with Phase-4.1 hardcoded stubs.
//
// In Phase 4.1 there is no real quota API. QuotaProbe returns hardcoded stub
// values: 5-hour usage at 23% and 7-day usage at 41%. Tests assert on these
// exact stub strings; they will be updated in Phase 6 when real API values
// are plumbed through Config.
package probes_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestQuota_Visible_Disabled verifies that QuotaProbe.Visible returns false
// when Config.QuotaEnabled is false, regardless of other fields.
func TestQuota_Visible_Disabled(t *testing.T) {
	p := &probes.QuotaProbe{}
	d := probes.Data{Stdin: stdin.Payload{}}
	cfg := probes.Config{QuotaEnabled: false}

	got := p.Visible(d, cfg)
	if got != false {
		t.Errorf("Visible(QuotaEnabled=false): want false, got true")
	}
}

// TestQuota_Visible_Enabled verifies that QuotaProbe.Visible returns true
// when Config.QuotaEnabled is true.
func TestQuota_Visible_Enabled(t *testing.T) {
	p := &probes.QuotaProbe{}
	d := probes.Data{Stdin: stdin.Payload{}}
	cfg := probes.Config{QuotaEnabled: true}

	got := p.Visible(d, cfg)
	if got != true {
		t.Errorf("Visible(QuotaEnabled=true): want true, got false")
	}
}

// TestQuota_Render_Full verifies QuotaProbe.Render at LevelFull.
//
// Phase-4.1 stub values: 5h=23%, 7d=41%.
// Expected: "5h: █▒░░░ ↻2h13m · 7d: ██░░░ ↻3d12h"
func TestQuota_Render_Full(t *testing.T) {
	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	th := renderer.Theme{}
	d := probes.Data{Stdin: stdin.Payload{}}

	got := p.Render(d, cfg, th, probes.LevelFull)
	want := "5h: █▒░░░ ↻2h13m · 7d: ██░░░ ↻3d12h"
	if got != want {
		t.Errorf("Render(Full): want %q, got %q", want, got)
	}
}

// TestQuota_Render_Compact verifies QuotaProbe.Render at LevelCompact.
// Labels "5h: " and "7d: " are dropped per §A4 P1.
//
// Expected: "█▒░░░ ↻2h13m · ██░░░ ↻3d12h"
func TestQuota_Render_Compact(t *testing.T) {
	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	th := renderer.Theme{}
	d := probes.Data{Stdin: stdin.Payload{}}

	got := p.Render(d, cfg, th, probes.LevelCompact)
	want := "█▒░░░ ↻2h13m · ██░░░ ↻3d12h"
	if got != want {
		t.Errorf("Render(Compact): want %q, got %q", want, got)
	}
}

// TestQuota_Render_Minimal verifies QuotaProbe.Render at LevelMinimal.
// Progress bars and time-to-reset are dropped; only percent values remain.
//
// Expected: "23% · 41%"
func TestQuota_Render_Minimal(t *testing.T) {
	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	th := renderer.Theme{}
	d := probes.Data{Stdin: stdin.Payload{}}

	got := p.Render(d, cfg, th, probes.LevelMinimal)
	want := "23% · 41%"
	if got != want {
		t.Errorf("Render(Minimal): want %q, got %q", want, got)
	}
}
