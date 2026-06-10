package statusline_test

// svgframes_test.go — Phase 7 Wave 4 (R-0) README asset emitter.
//
// The golden snapshots (golden_test.go) capture colour scenarios as the RAW
// {{marker}} stream for human-auditable diffs. README screenshots need the
// SAME scenarios rendered with the REAL 16-colour ANSI palette (the production
// renderer.DefaultPalette + renderer.Apply path, mirroring cmd/cc-probeline
// main.go:382-402) so a terminal-screenshot tool (e.g. charmbracelet/freeze)
// can turn them into coloured SVG frames.
//
// This is NOT an assertion test — it is gated behind CC_PROBELINE_EMIT_DIR and
// skipped by default (so `go test ./...` stays hermetic and asset-free). When
// the env var points at a directory, it writes one <scenario>.ansi file per
// scenario containing real ESC sequences.
//
// Emit all frames:
//
//	CC_PROBELINE_EMIT_DIR=assets/frames go test ./tests/statusline/ -run EmitANSIFrames -v
//
// Frame → scenario map (readme-brainstorm.md §5):
//
//	frame 1 (full dashboard)          → s1-rich-baseline        (cols 120)
//	frame 2 (table close-up)          → s1-rich-baseline        (crop the table region)
//	frame 3 (extra-usage red)         → s4-quota-100-extra-commit
//	frame 4 (quota warning)           → s3-quota-split-ctx-warn
//	frame 5 (cache lifetime / TTL)    → s4 (OrchTTL) + s3 (SubagentCacheExpired) events

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/statusline"
	"github.com/labzink/cc-probeline/tests/testutil"
)

// realTheme is the production palette: 16-colour ANSI with real ESC sequences,
// the same theme cmd/cc-probeline builds via renderer.DefaultPalette().
func realTheme() renderer.Theme {
	return renderer.Theme{AnsiEnabled: true, Colors: renderer.DefaultPalette()}
}

func TestEmitANSIFrames(t *testing.T) {
	dir := os.Getenv("CC_PROBELINE_EMIT_DIR")
	if dir == "" {
		t.Skip("set CC_PROBELINE_EMIT_DIR to emit real-ANSI frames for README assets")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(testutil.ProjectRoot(t), dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}

	th := realTheme()
	for _, sc := range scenarios() {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			d, cfg := scenarioData(t, sc)
			a := statusline.Assembler{Mode: sc.mode, Theme: th, Cols: sc.cols, Config: cfg}
			out := renderer.Apply(a.Render(d), th)
			path := filepath.Join(dir, sc.name+".ansi")
			if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}
			t.Logf("wrote %s (%d bytes)", path, len(out))
		})
	}
}
