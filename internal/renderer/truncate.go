// Package renderer — truncation pipeline for status-line probes.
//
// FitLine progressively downgrades probe render levels (Full -> Compact ->
// Minimal) until the assembled string fits in cols. P0 probes (Priority 0)
// are never downgraded. When sep == " | " (line 2) and cols < 50, the result
// is soft-wrapped onto multiple lines; otherwise overflow is accepted because
// P0 content is inviolable (section 4.3).
//
// Cycle note (Pre-step clarification, see plans/tasks/phase-4-step3-plan.md):
// putting truncate.go in internal/renderer with import probes would form
// renderer -> probes -> renderer. We break the cycle with an adapter type
// ProbeEntry; the assembler (internal/statusline) builds ProbeEntry values
// from real probes in 4.3.d.
package renderer

import (
	"strings"

	"github.com/labzink/cc-probeline/internal/format"
)

// ProbeEntry is the adapter type FitLine consumes. Render returns the probe's
// output at the chosen level (0=Full, 1=Compact, 2=Minimal). An empty string
// means the probe is invisible or fully dropped at this level.
type ProbeEntry struct {
	Priority int
	Render   func(level int) string
}

// priorityGroup clamps Priority to a bucket: 0, 1, 2, or 3+.
func priorityGroup(p int) int {
	switch {
	case p <= 0:
		return 0
	case p == 1:
		return 1
	case p == 2:
		return 2
	default:
		return 3
	}
}

// levelForPass returns the render level (0=Full, 1=Compact, 2=Minimal) for a
// given priority and pass number. Table from plan section 4.3.c:
//
//	pass | P0 | P1 | P2 | P3+
//	   0 |  0 |  0 |  0 |  0
//	   1 |  0 |  0 |  1 |  1
//	   2 |  0 |  1 |  1 |  2
//	   3 |  0 |  1 |  2 |  2
//	   4 |  0 |  2 |  2 |  2
func levelForPass(priority, pass int) int {
	g := priorityGroup(priority)
	switch pass {
	case 0:
		return 0
	case 1:
		if g <= 1 {
			return 0
		}
		return 1
	case 2:
		if g == 0 {
			return 0
		}
		if g <= 2 {
			return 1
		}
		return 2
	case 3:
		if g == 0 {
			return 0
		}
		if g == 1 {
			return 1
		}
		return 2
	case 4:
		if g == 0 {
			return 0
		}
		return 2
	}
	// Fallback: Minimal for all non-P0.
	if g == 0 {
		return 0
	}
	return 2
}

// assemble renders all entries at the level determined by levelForPass(priority, pass),
// drops empty results, and joins with sep.
func assemble(entries []ProbeEntry, pass int, sep string) string {
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		lvl := levelForPass(e.Priority, pass)
		s := e.Render(lvl)
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, sep)
}

// wrapMarker is the continuation prefix for wrapped lines (U+21AA + space,
// runewidth = 2). Visible to the user as a curved arrow, making line
// continuations easy to spot.
const wrapMarker = "↪ "

// softWrap splits s by sep and accumulates chunks into lines. A new line is
// started whenever adding the next chunk (with sep) would exceed cols
// (measured via format.VisualLen). Chunks within a line are joined by sep;
// lines are joined by "\n". Continuation lines are prefixed with wrapMarker
// so the user can distinguish line2 wraps from line0/line1 bullets.
// A single chunk is returned as-is.
func softWrap(s, sep string, cols int) string {
	chunks := strings.Split(s, sep)
	if len(chunks) <= 1 {
		return s
	}

	var lines []string
	current := chunks[0]

	for _, chunk := range chunks[1:] {
		candidate := current + sep + chunk
		if format.VisualLen(candidate) <= cols {
			current = candidate
		} else {
			lines = append(lines, current)
			current = wrapMarker + chunk
		}
	}
	lines = append(lines, current)

	return strings.Join(lines, "\n")
}

// FitLine assembles entries into a single status line that fits in cols,
// downgrading probe render levels by Priority group when needed.
//
// Algorithm:
//  1. Try passes 0..4; return assembled string on first pass that fits.
//  2. If no pass fits and sep==" | " and cols<50, apply softWrap to the
//     pass-4 result.
//  3. Otherwise return pass-4 result (P0 inviolable, overflow accepted).
func FitLine(entries []ProbeEntry, cols int, sep string) string {
	if len(entries) == 0 {
		return ""
	}

	var last string
	for pass := 0; pass <= 4; pass++ {
		assembled := assemble(entries, pass, sep)
		last = assembled
		if format.VisualLen(assembled) <= cols {
			return assembled
		}
	}

	// All passes overflow.
	if cols < 50 && sep == " | " {
		return softWrap(last, sep, cols)
	}
	return last
}
