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
//	frame 5 (cache rebuild close-up)        → s5-cache-rebuild (truncated fable)

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

// Frames own a private copy of the master fixture (tests/frames/testdata/master)
// so token/timestamp tuning for README screenshots never touches the golden
// snapshot fixture (tests/fixtures/integration/golden-master).
const (
	fxMaster    = "tests/frames/testdata/master/master.jsonl"
	fxMasterDir = "tests/frames/testdata/master/sidecar"
	// fable variant: same master, orchestrator model = claude-fable-5 (frame 5
	// shows the cache-rebuild cost at Fable 5 prices). Kept separate so frames
	// 1/3 stay on Opus 4.8.
	fxFable    = "tests/frames/testdata/master-fable/master.jsonl"
	fxFableDir = "tests/frames/testdata/master-fable/sidecar"
	// fable-rebuild variant: the fable log TRUNCATED at the 240K rebuild turn so
	// that turn (#8) becomes the freshest request — rendered bright (no dim), at
	// the top of its own short table with a real top border. The cache-rebuild
	// close-up (frame 5) reads as "the rebuild just happened on the current turn".
	// No subagents (orchestrator-only) — instead cache_read ramps #1→#7 up to 240K
	// so the table itself shows WHY #8 rebuilds: the cache had grown to 240K (read
	// on #7), the idle TTL passed, so #8 rewrites all 240K (read drops to 6K).
	fxFableRebuild = "tests/frames/testdata/master-fable-rebuild/master.jsonl"
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

	model     stdin.Model
	effort    stdin.Effort
	hintStart int // hint.DefaultHints index this frame opens on (0 = Reasoning legend)
}

// scenarios returns ONLY the scenarios the README frames are built from. Edit
// these freely to tune the frames — nothing here feeds the golden snapshots.
func scenarios() []scenario {
	rich := richProbesCfg()
	// frame 5 shows only the newest 8 rows (#11–#18): a narrow cache-rebuild window
	// with the first 10 turns scrolled off the top, so #11 already carries a large
	// cache_read built up off-screen.
	richWin8 := richProbesCfg()
	richWin8.TableRows = 8
	gitDirty := &parser.GitStatus{Branch: "agent/dev/phase-7", ModifiedCount: 3}
	gitClean := &parser.GitStatus{Branch: "main", ModifiedCount: 0}
	opus := stdin.Model{ID: "claude-opus-4-8", Name: "Opus 4.8"}
	sonnet46 := stdin.Model{ID: "claude-sonnet-4-6", Name: "Sonnet 4.6"}
	fable5 := stdin.Model{ID: "claude-fable-5", Name: "Fable 5"}
	ctxNormal := ctxWindow(200_000, 60_000)
	ctxWarn := ctxWindow(200_000, 156_000)
	// Opus 4.8 advertises a 1M context window; the master session has grown to
	// ~323K read (the newest turn), so the header bar reads 323K/1000K, matching
	// the largest cache-read row in the table.
	ctxOpus1M := ctxWindow(1_000_000, 323_000)

	master := func(s scenario) scenario {
		if s.fixture == "" {
			s.fixture, s.subDir = fxMaster, fxMasterDir
		}
		if s.model.ID == "" {
			s.model = opus
		}
		if s.mode == "" {
			s.mode = mode.Standard
		}
		if s.ccTotalUSD == 0 {
			// Honest bottom-up total of the master fixture computed from its own
			// token data at the current cost weights (Phase 7.45 B3-1 refreshed
			// opus → Opus 4.8): 18 opus orch turns = $3.54 + 3 sonnet subagents
			// (13 turns) = $0.64 ⇒ $4.18. Reconcile distributes it so each per-turn
			// $ equals that turn's real token price (ccTotal == Σ weights ⇒ identity).
			s.ccTotalUSD, s.durMS = 4.18, 1_217_000
		}
		return s
	}

	return []scenario{
		// frame 1: full dashboard at full width. Hint = Reasoning legend (#0).
		master(scenario{
			name: "s1-rich-baseline", cols: 120, cfg: rich,
			git: gitDirty, ctx: ctxOpus1M, effort: stdin.Effort{Level: "high"},
			rl: rl(30, 75, 3*time.Hour, 2*24*time.Hour), // 5h 30% (round) · 7d 75% ⇒ orange, ~2d
		}),
		// frame 2: same dashboard, cropped to the table close-up. Distinct hint —
		// the config tip (#5: ⚙ /cc-probeline-config) so it doesn't repeat the
		// Reasoning legend frame 1 already shows.
		master(scenario{
			name: "s2-table-config-hint", cols: 120, cfg: rich,
			git: gitDirty, ctx: ctxOpus1M, effort: stdin.Effort{Level: "high"},
			rl:        rl(30, 75, 3*time.Hour, 2*24*time.Hour),
			hintStart: 5,
		}),
		// frame 4: quota warning header — rendered WIDE (full bars). Model = Sonnet 4.6.
		master(scenario{
			name: "s3-quota-split-ctx-warn", cols: 130, cfg: rich,
			git: gitClean, ctx: ctxWarn, model: sonnet46,
			rl:     rl(98, 72, 8*time.Minute, 30*time.Hour),
			events: []parser.CacheEvent{{Type: parser.SubagentCacheExpired, Timestamp: frameNow, Detail: "conceptualist"}},
		}),
		// frame 3: extra-usage header — WIDE. Overage +$3.80, session $48.27, time 52:17.
		master(scenario{
			name: "s4-quota-100-extra-commit", cols: 130, cfg: rich,
			git: gitClean, ctx: ctxNormal,
			rl:          rl(100, 100, 2*time.Hour, 24*time.Hour),
			extraActive: true, extraUSD: 3.80, commitBadge: 2,
			ccTotalUSD: 48.27, durMS: 3_137_000, // header cost $48.27 · time 52:17
			events: []parser.CacheEvent{{Type: parser.OrchTTL, Timestamp: frameNow}},
		}),
		// frame 5: cache-rebuild close-up — a scrolled window showing the newest 8
		// turns (#11–#18); the first 10 are off the top (TableRows=8). The cache was
		// built up off-screen, so #11 already reads 182K and the visible dim turns
		// #11→#17 only inch up (uneven ~8–12K cache_creation each) to 240K. Then the
		// idle TTL passes and #18 (freshest → bright) rewrites the whole 240K
		// (6K read / 240K write ≈ $3.02). Token flow is consistent end-to-end:
		// read[N+1] = read[N] + write[N]. ccTotal ≈ Σ all 18 weights so the split
		// keeps #18 at ~$3.02 (the rebuild's intrinsic Fable write price).
		master(scenario{
			name: "s5-cache-rebuild", cols: 130, cfg: richWin8,
			fixture: fxFableRebuild, model: fable5,
			git: gitClean, ctx: ctxNormal,
			rl:         rl(100, 100, 2*time.Hour, 5*24*time.Hour),
			ccTotalUSD: 8.71, durMS: 1_217_000,
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
		HintStart:        sc.hintStart,
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
	// 18 orch turns + 3 subagents = 21 rows; cap 18 shows the newest 18 = turns
	// #4–#18 (15) + the 3 subagents, dropping the 3 oldest (#1–#3) so the table
	// reads as a sliding window (3 turns scrolled off the top, numbering starts
	// at #4, not #1).
	c.TableRows = 18
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
