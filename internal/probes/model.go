package probes

import (
	"strings"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// ModelProbe renders the canonical short model name (e.g. "opus-4-7").
// It is a P0 probe: always visible when Model.ID is non-empty.
//
// Note: Render uses the 3-param signature (d, t, level) to match the Phase 4.1.a
// test contract. The 4-param Config form will be added in Phase 4.1.a GREEN.
type ModelProbe struct{}

func (p *ModelProbe) Name() string  { return "model" }
func (p *ModelProbe) Priority() int { return 0 }
func (p *ModelProbe) MinWidth() int { return 8 } // len("opus-4-7")

// Visible returns true when Stdin.Model.ID is non-empty.
func (p *ModelProbe) Visible(d Data, c Config) bool {
	return d.Stdin.Model.ID != ""
}

// Render returns the canonical short model name by stripping the "claude-" prefix
// and truncating to the first three dash-separated segments.
// All display levels return the same value (model is never abbreviated).
func (p *ModelProbe) Render(d Data, t renderer.Theme, level Level) string {
	id := d.Stdin.Model.ID
	if id == "" {
		return ""
	}
	const prefix = "claude-"
	if !strings.HasPrefix(id, prefix) {
		return id
	}
	trimmed := id[len(prefix):]
	parts := strings.SplitN(trimmed, "-", 4)
	if len(parts) <= 3 {
		return trimmed
	}
	return parts[0] + "-" + parts[1] + "-" + parts[2]
}
