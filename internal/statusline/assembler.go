// Package statusline composes the multi-line status output from the Phase 4.1
// probes Registry, the Phase 4.2 box-drawing table, and the Phase 4.4 hint
// widget. It lives above probes and renderer to avoid an import cycle:
// probes depends on renderer (Theme), and the assembler depends on both.
package statusline

import (
	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
)

// Assembler joins probes from the Registry into the final multi-line status
// string. Cols is the detected terminal width; Mode gates whether the per-turn
// table and footer are emitted. Phase 4.2.d fills in Render; this is a
// foundation stub.
type Assembler struct {
	Mode   mode.Mode
	Theme  renderer.Theme
	Cols   int
	Config probes.Config
}

// Render produces the full status string with marker tokens (no ANSI escapes).
// Caller is responsible for piping the result through renderer.Apply. Stub
// returns "".
func (a *Assembler) Render(_ probes.Data) string { return "" }
