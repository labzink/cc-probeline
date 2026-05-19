// Package renderer — truncation pipeline for status-line probes.
//
// FitLine progressively downgrades probe render levels (Full → Compact →
// Minimal) until the assembled string fits in cols. P0 probes (Priority 0)
// are never downgraded. When sep == " | " (line 2) and cols < 50, the result
// is soft-wrapped onto multiple lines; otherwise overflow is accepted because
// P0 content is inviolable (§4.3).
//
// Cycle note (Pre-step clarification, see plans/tasks/phase-4-step3-plan.md):
// putting truncate.go in internal/renderer with `import probes` would form
// renderer → probes → renderer. We break the cycle with an adapter type
// ProbeEntry; the assembler (internal/statusline) builds ProbeEntry values
// from real probes in 4.3.d.
//
// Real algorithm lands in 4.3.c. This stub keeps the public surface compiling.
package renderer

// ProbeEntry is the adapter type FitLine consumes. Render returns the probe's
// output at the chosen level (0=Full, 1=Compact, 2=Minimal). An empty string
// means the probe is invisible or fully dropped at this level.
type ProbeEntry struct {
	Priority int
	Render   func(level int) string
}

// FitLine assembles entries into a single status line that fits in cols,
// downgrading probe levels by Priority group when needed. Soft-wraps with
// sep when cols < 50 and sep is line-2 style (" | ").
//
// Stub: returns "". Real implementation in 4.3.c.
func FitLine(entries []ProbeEntry, cols int, sep string) string {
	_ = entries
	_ = cols
	_ = sep
	return ""
}
