package frames_test

// frames_test.go — Phase 7 Wave 4 (R-0) README asset emitter.
//
// ISOLATED COPY (deliberate): this package owns its OWN scenario definitions,
// scenarioData builder and helpers, fully decoupled from the golden snapshot
// harness (tests/statusline/golden_test.go). README frames are tuned by editing
// THIS file only; the golden regression guard is never touched. The two used to
// share scenarios() / scenarioData(), which meant any frame tweak moved the
// goldens — that coupling is gone.
//
// The emitter renders each scenario with the production 16-colour ANSI palette
// (renderer.DefaultPalette + renderer.Apply, mirroring cmd/cc-probeline main.go)
// so a screenshot in a dark + SF Mono terminal captures real dim/colour/glyphs.
//
// It is NOT an assertion test — gated behind CC_PROBELINE_EMIT_DIR, skipped by
// default so `go test ./...` stays hermetic and asset-free. When the env var
// points at a directory it writes one <scenario>.ansi file per scenario.
//
// Emit all frames:
//
//	CC_PROBELINE_EMIT_DIR=<dir> go test ./tests/frames/ -run EmitANSIFrames -v
//
// Frame → scenario map (readme-brainstorm.md §5/§7, composed by build-frames.sh):
//
//	frame 1 (full dashboard + hero framing) → s1-rich-baseline        (cols 120)
//	frame 2 (table close-up)                → s1-rich-baseline        (crop table)
//	frame 3 (extra-usage red)               → s4-quota-100-extra-commit
//	frame 4 (quota warning)                 → s3-quota-split-ctx-warn
//	frame 5 (cache lifetime / TTL)          → s4 (OrchTTL) + table

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/config"
	"github.com/labzink/cc-probeline/internal/cost"
	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/state"
	"github.com/labzink/cc-probeline/internal/statusline"
	"github.com/labzink/cc-probeline/internal/stdin"
	"github.com/labzink/cc-probeline/tests/testutil"
)

// ─── Master fixture (own copy of the path constants) ─────────────────────────

const (
	fxMaster    = "tests/fixtures/integration/golden-master.jsonl"
	fxMasterDir = "tests/fixtures/integration/golden-master"
)

// frameNow pins the observation moment, independent of the golden harness. Same
// value (08:47 → full TTL mix: live orchestrator ~58m, live subagent ~1m,
// expired early subagents, frozen "⏱ 0m" groups) but owned here.
var frameNow = time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

// ─── Scenario definition (own copy) ──────────────────────────────────────────

type scenario struct {
	name    string
	fixture string
	subDir  string

	mode mode.Mode
	cols int
	cfg  probes.Config

	ccTotalUSD float64
	durMS      int64

	git         *parser.GitStatus
	rl          *stdin.RateLimits
	ctx         stdin.ContextWindow
	extraActive bool
	extraUSD    float64
	commitBadge int
	events      []parser.CacheEvent

	model  stdin.Model
	effort stdin.Effort
}

// scenarios returns ONLY the scenarios the README frames are built from. Edit
// these freely to tune the frames — nothing here feeds the golden snapshots.
func scenarios() []scenario {
	rich := richProbesCfg()
	gitDirty := &parser.GitStatus{Branch: "agent/dev/phase-7", ModifiedCount: 3}
	gitClean := &parser.GitStatus{Branch: "main", ModifiedCount: 0}
	opus := stdin.Model{ID: "claude-opus-4-7", Name: "Opus 4.7"}
	ctxNormal := ctxWindow(200_000, 60_000)
	ctxWarn := ctxWindow(200_000, 156_000)

	master := func(s scenario) scenario {
		s.fixture, s.subDir = fxMaster, fxMasterDir
		s.model = opus
		if s.mode == "" {
			s.mode = mode.Standard
		}
		if s.ccTotalUSD == 0 {
			s.ccTotalUSD, s.durMS = 11.39, 1_217_000
		}
		return s
	}

	return []scenario{
		// frame 1 + frame 2: full dashboard at full width.
		master(scenario{
			name: "s1-rich-baseline", cols: 120, cfg: rich,
			git: gitDirty, ctx: ctxNormal, effort: stdin.Effort{Level: "high"},
			rl: rl(35, 15, 3*time.Hour, 5*24*time.Hour),
		}),
		// frame 4: two quota zones (5h red+near-reset, 7d orange), ctx warn, alert.
		master(scenario{
			name: "s3-quota-split-ctx-warn", cols: 80, cfg: rich,
			git: gitClean, ctx: ctxWarn,
			rl:     rl(98, 72, 8*time.Minute, 30*time.Hour),
			events: []parser.CacheEvent{{Type: parser.SubagentCacheExpired, Timestamp: frameNow, Detail: "conceptualist"}},
		}),
		// frame 3 + frame 5: both windows 100% (extra-usage + commit badge + alert).
		master(scenario{
			name: "s4-quota-100-extra-commit", cols: 80, cfg: rich,
			git: gitClean, ctx: ctxNormal,
			rl:          rl(100, 100, 2*time.Hour, 5*24*time.Hour),
			extraActive: true, extraUSD: 0.80, commitBadge: 2,
			events: []parser.CacheEvent{{Type: parser.OrchTTL, Timestamp: frameNow}},
		}),
	}
}

// ─── Emitter ─────────────────────────────────────────────────────────────────

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

// ─── scenarioData (own copy, mirrors cmd/cc-probeline runRender, hermetic) ────

func scenarioData(t *testing.T, sc scenario) (probes.Data, probes.Config) {
	t.Helper()
	root := testutil.ProjectRoot(t)
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir()) // empty ⇒ quota reads payload RateLimits

	records := mustParse(t, filepath.Join(root, sc.fixture))
	session := parser.Aggregate(parser.Dedup(records))

	var subagents []parser.SubagentStats
	if sc.subDir != "" {
		subs, err := parser.CollectSubagents(context.Background(), filepath.Join(root, sc.subDir))
		if err != nil {
			t.Fatalf("CollectSubagents: %v", err)
		}
		subagents = subs
	}

	// Two-phase Reconcile: seed a $0 baseline, then reconcile the real ccTotal so
	// the full amount becomes this session's delta, distributed across turns.
	st := &state.Session{}
	allTurns := make([]parser.Turn, len(session.Turns))
	copy(allTurns, session.Turns)
	for i := range subagents {
		allTurns = append(allTurns, subagents[i].Turns...)
	}
	cost.Reconcile(st, 0, 0, nil)
	cost.Reconcile(st, sc.ccTotalUSD, sc.durMS, allTurns)

	payload := stdin.Payload{
		Model:         sc.model,
		Effort:        sc.effort,
		SessionID:     "frame-" + sc.name,
		Cwd:           "/Users/dev/Projects/cc-probeline",
		ContextWindow: sc.ctx,
		Cost:          stdin.Cost{TotalCostUSD: sc.ccTotalUSD, TotalAPIDurationMS: sc.durMS},
		RateLimits:    sc.rl,
	}

	d := probes.Data{
		Stdin:            payload,
		Session:          &session,
		Subagents:        subagents,
		Git:              sc.git,
		Now:              frameNow,
		SessionID:        payload.SessionID,
		ExtraCacheEvents: sc.events,
		State:            st,
		CommitBadgeCount: sc.commitBadge,
		ExtraActive:      sc.extraActive,
		ExtraUSD:         sc.extraUSD,
	}
	d.SessionTotal = cost.SessionTotal(st, sc.ccTotalUSD)
	d.SessionDurMS = cost.SessionDuration(st, sc.durMS)
	curGroupID := 0
	if len(session.Turns) > 0 {
		curGroupID = session.Turns[len(session.Turns)-1].GroupID
	}
	d.LastRequestCost = cost.LastRequest(st, sc.ccTotalUSD, curGroupID)
	captured := st
	d.PerTurnCostFn = func(uuid string) (float64, bool) { return cost.PerTurn(captured, uuid) }

	cfg := sc.cfg
	if cfg.EmailEnabled {
		cfg.Email = "me@example.com"
	}
	return d, cfg
}

// ─── Helpers (own copies) ────────────────────────────────────────────────────

// richProbesCfg is the all-widgets-on config with a roomy table so the master's
// request groups (and their subagent ↳ rows) surface in the frames.
func richProbesCfg() probes.Config {
	c := config.ToProbesConfig(*config.Default())
	c.TableRows = 20
	return c
}

func mustParse(t *testing.T, path string) []parser.Record {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	records, _, scanErr := parser.ParseLines(f)
	if scanErr != nil {
		t.Fatalf("ParseLines %s: %v", path, scanErr)
	}
	return records
}

func rl(pct5h, pct7d float64, reset5h, reset7d time.Duration) *stdin.RateLimits {
	return &stdin.RateLimits{
		FiveHour: stdin.RateWindow{UsedPercentage: pct5h, ResetsAt: jsonTime(frameNow.Add(reset5h))},
		SevenDay: stdin.RateWindow{UsedPercentage: pct7d, ResetsAt: jsonTime(frameNow.Add(reset7d))},
	}
}

func jsonTime(t time.Time) json.RawMessage {
	b, _ := json.Marshal(t.UTC().Format(time.RFC3339))
	return b
}

func ctxWindow(size, used int) stdin.ContextWindow {
	return stdin.ContextWindow{
		Size:         size,
		CurrentUsage: map[string]int{"cache_read_input_tokens": used},
	}
}
