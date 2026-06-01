package renderer

import (
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/state"
)

// RenderUnified is a stub for Phase 6.8.d.
// It will be replaced by the real implementation (dev GREEN phase).
// Signature: takes a pre-merged, timestamp-sorted []Turn and a session state,
// returns the redesigned table string (interleaved, group-separated, legend footer).
// Stub returns "" so all T-T1..T-T6 tests remain RED.
func (b *Builder) RenderUnified(turns []parser.Turn, st *state.Session) string {
	return ""
}
