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
// When AnsiEnabled, the model name is wrapped in {{bold}}…{{reset}} markers.
// All display levels return the same value.
func (p *ModelProbe) Render(d Data, _ Config, t renderer.Theme, level Level) string {
	id := d.Stdin.Model.ID
	if id == "" {
		return ""
	}
	name := parser.CanonicalModelKey(id)
	var displayName string
	if t.AnsiEnabled {
		displayName = "{{bold}}" + name + "{{reset}}"
	} else {
		displayName = name
	}
	if icon := effortIcon[d.Stdin.Effort.Level]; icon != "" {
		return displayName + " " + icon
	}
	return displayName
}
