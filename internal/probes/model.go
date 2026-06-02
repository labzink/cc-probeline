package probes

import (
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/renderer"
)

// ModelProbe renders the canonical short model name (e.g. "opus-4-7").
// It is a P0 probe: always visible when Model.ID is non-empty.
type ModelProbe struct{}

func (p *ModelProbe) Name() string  { return "model" }
func (p *ModelProbe) Priority() int { return 0 }
func (p *ModelProbe) MinWidth() int { return 8 } // len("opus-4-7")

// Visible returns true when ModelEnabled is set and Stdin.Model.ID is non-empty.
func (p *ModelProbe) Visible(d Data, c Config) bool {
	if !c.ModelEnabled {
		return false
	}
	return d.Stdin.Model.ID != ""
}

// Render returns the canonical short model name, optionally followed by the
// effort icon (e.g. "sonnet-4-6 ◑"). Effort is appended without a separator so
// it reads as one visual unit.
//
// When AnsiEnabled, the model name is wrapped in effortColorMarker(effort.Level)
// instead of the former {{bold}} marker, so the model name shares the same colour
// semantics as the effort glyph (T-14). An empty marker means the name is plain.
// All display levels return the same value.
func (p *ModelProbe) Render(d Data, _ Config, t renderer.Theme, level Level) string {
	id := d.Stdin.Model.ID
	if id == "" {
		return ""
	}
	name := parser.CanonicalModelKey(id)
	var displayName string
	if t.AnsiEnabled {
		marker := effortColorMarker(d.Stdin.Effort.Level)
		if marker != "" {
			displayName = marker + name + "{{reset}}"
		} else {
			// medium or no effort — render plain (no {{bold}}, no colour).
			displayName = name
		}
	} else {
		displayName = name
	}
	if glyph := effortGlyph(d.Stdin.Effort.Level, t.AnsiEnabled); glyph != "" {
		return displayName + " " + glyph
	}
	return displayName
}
