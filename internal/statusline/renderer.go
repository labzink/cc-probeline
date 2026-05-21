package statusline

import "github.com/labzink/cc-probeline/internal/probes"

// Renderer produces the final multi-line status string with marker tokens
// (no ANSI escapes). The caller pipes the result through renderer.Apply to
// expand marker tokens into terminal escape sequences.
//
// Phase 4.4 spec-S6: exported for swappable rendering in integration tests
// and the CLI entrypoint (Phase 5).
type Renderer interface {
	Render(d probes.Data) string
}

// Compile-time check: Assembler must implement Renderer.
var _ Renderer = (*Assembler)(nil)
