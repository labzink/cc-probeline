package statusline_test

// golden_test.go — Phase 6.95.i rich golden-snapshot harness (golden-map.md).
//
// Design (per golden-map.md):
//   - ONE maximally rich MASTER fixture (779cae28: 4 subagents, multi-group,
//     TTL mix) is the basis; every Standard scenario is the SAME master observed
//     at a pinned Now, only the width / quota / mode / config differs.
//   - Colour scenarios capture the RAW marker stream ({{dim}}/{{color:X}}/
//     {{reset}}) BEFORE renderer.Apply, so the table vocabulary of §2.1 stays
//     visible in the snapshot: dim old-group rows, dim-red frozen "⏱ 0m",
//     coloured notches, coloured quota bars. A token ColorScheme turns the few
//     direct theme writes (quota Reset, progress-bar colours) into markers too,
//     so the snapshot is one clean marker vocabulary with no raw ESC bytes.
//   - The nocolor twin and the gating scenario use the PLAIN form (post-Apply,
//     NO_COLOR) to guard layout without markers.
//
// Hermetic inputs: real captured fixtures for Session/Subagents; quota driven
// from the payload RateLimits (empty CC_PROBELINE_QUOTA_DIR ⇒ no snapshot);
// git / extra-usage / commit badge injected into probes.Data; cost computed by
// cost.Reconcile on a fresh in-memory state; Now pinned for deterministic TTL.
//
// Run / regenerate:
//
//	go test ./tests/statusline/ -run Golden -v
//	go test ./tests/statusline/ -run Golden -update   # rewrite snapshots

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/config"
	"github.com/labzink/cc-probeline/internal/cost"
	"github.com/labzink/cc-probeline/internal/format"
	"github.com/labzink/cc-probeline/internal/hint"
	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/state"
	"github.com/labzink/cc-probeline/internal/statusline"
	"github.com/labzink/cc-probeline/internal/stdin"
	"github.com/labzink/cc-probeline/tests/testutil"
)

// ─── Master fixture (golden-map §6) ──────────────────────────────────────────

const (
	fxMaster    = "tests/fixtures/integration/golden-master.jsonl"
	fxMasterDir = "tests/fixtures/integration/golden-master"
)

// goldenNow pins the observation moment. The master spans 2026-06-05 03:48 →
// 09:13; the orchestrator's live burst is ~08:44–08:45 and the conceptualist
// subagent's last turn is 08:43. Pinning Now at 08:47 yields the full TTL mix:
//   - live orchestrator   (08:45 turn, ~2 min ago → ~58 min of the 60-min TTL)
//   - live subagent ↳     (conceptualist 08:43, ~4 min ago → ~1 min of 5-min TTL)
//   - expired subagents   (early researchers 04:19–04:51 → hours ago)
//   - frozen "⏱ 0m"       (early groups 03:48–04:xx, >60 min idle → dim-red)
var goldenNow = time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

// ─── Scenario definition ─────────────────────────────────────────────────────

type scenario struct {
	name    string
	fixture string
	subDir  string

	mode   mode.Mode
	cols   int
	colour bool          // true ⇒ raw marker snapshot; false ⇒ plain (post-Apply)
	cfg    probes.Config // probes.Config{} ⇒ all widgets gated off

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

func scenarios() []scenario {
	rich := richProbesCfg() // all widgets on + a roomy table to surface groups
	gitDirty := &parser.GitStatus{Branch: "agent/dev/phase-7", ModifiedCount: 3}
	gitClean := &parser.GitStatus{Branch: "main", ModifiedCount: 0}
	opus := stdin.Model{ID: "claude-opus-4-7", Name: "Opus 4.7"}
	ctxNormal := ctxWindow(200_000, 60_000)
	ctxWarn := ctxWindow(200_000, 156_000)

	// All Standard scenarios share the master fixture + sidecar; only the
	// width / quota / mode / config axis changes (golden-map §1.3 ladder).
	master := func(s scenario) scenario {
		s.fixture, s.subDir = fxMaster, fxMasterDir
		s.model = opus
		if s.mode == "" {
			s.mode = mode.Standard // scenarios that set SuperCompact keep it
		}
		if s.ccTotalUSD == 0 {
			s.ccTotalUSD, s.durMS = 11.39, 1_217_000
		}
		return s
	}

	return []scenario{
		// S1 — MASTER, colour: every probe + group/dim/notch/TTL-mix vocabulary.
		// The richest snapshot leads the sequence and renders at the FULL width
		// (120): every probe at its Full level (10-bar quota, full ctx bar, full
		// git branch) and the wide table tool column ("<name>: <tool>" on ↳ rows).
		master(scenario{
			name: "s1-rich-baseline", cols: 120, colour: true, cfg: rich,
			git: gitDirty, ctx: ctxNormal, effort: stdin.Effort{Level: "high"},
			rl: rl(35, 15, 3*time.Hour, 5*24*time.Hour),
		}),
		// S2 — same master, PLAIN twin (no-color layout guard).
		master(scenario{
			name: "s2-rich-baseline-nocolor", cols: 120, colour: false, cfg: rich,
			git: gitDirty, ctx: ctxNormal, effort: stdin.Effort{Level: "high"},
			rl: rl(35, 15, 3*time.Hour, 5*24*time.Hour),
		}),
		// S3 — two quota zones (5h red+near-reset, 7d orange), ctx warn, alert.
		master(scenario{
			name: "s3-quota-split-ctx-warn", cols: 80, colour: true, cfg: rich,
			git: gitClean, ctx: ctxWarn,
			rl:     rl(98, 72, 8*time.Minute, 30*time.Hour),
			events: []parser.CacheEvent{{Type: parser.SubagentCacheExpired, Timestamp: goldenNow, Detail: "conceptualist"}},
		}),
		// S4 — both windows at 100%: extra-usage block + commit badge + alert.
		master(scenario{
			name: "s4-quota-100-extra-commit", cols: 80, colour: true, cfg: rich,
			git: gitClean, ctx: ctxNormal,
			rl:          rl(100, 100, 2*time.Hour, 5*24*time.Hour),
			extraActive: true, extraUSD: 0.80, commitBadge: 2,
			events: []parser.CacheEvent{{Type: parser.OrchTTL, Timestamp: goldenNow}},
		}),
		// S5 — SuperCompact: master data folded to one line + legend (no table).
		master(scenario{
			name: "s5-supercompact", cols: 80, colour: true, cfg: rich,
			git: gitClean, ctx: ctxNormal,
			rl:   rl(35, 15, 3*time.Hour, 5*24*time.Hour),
			mode: mode.SuperCompact,
		}),
		// S6 — width 60: quota Full→Compact, header probes squeezed (same master).
		master(scenario{
			name: "s6-narrow-60", cols: 60, colour: true, cfg: rich,
			git: gitDirty, ctx: ctxNormal,
			rl: rl(35, 15, 3*time.Hour, 5*24*time.Hour),
		}),
		// S7 — width 40: Minimal levels (quota %-only, header collapsed).
		master(scenario{
			name: "s7-narrow-40", cols: 40, colour: true, cfg: rich,
			git: gitDirty, ctx: ctxNormal,
			rl: rl(92, 60, 3*time.Hour, 5*24*time.Hour),
		}),
		// S8 — all probes gated off (colour): only table + hint survive. Colour
		// stays on to show the gated line in full colour; S2 carries the plain
		// layout guard (no-marker / width-fit invariant).
		master(scenario{
			name: "s8-allprobes-off", cols: 80, colour: true, cfg: probes.Config{},
		}),
	}
}

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestGoldenScenarios(t *testing.T) {
	for _, sc := range scenarios() {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			got := renderScenario(t, sc)
			testutil.CompareGolden(t, goldenPath(t, sc.name), got)
		})
	}
}

// TestGoldenBottomGallery captures every rotating hint (0–N) and every alert
// type in one snapshot (golden-map §4 Tier B): the bottom slot is a single XOR
// slot, so exhaustive coverage is collected here via the real hint.DefaultHints
// and hint.BuildAlert rather than across a dozen full renders. Marker form keeps
// the per-hint feature colours and the ℹ tip marker visible.
func TestGoldenBottomGallery(t *testing.T) {
	var b strings.Builder
	b.WriteString("── hints (rotation 0–" + itoa(len(hint.DefaultHints)-1) + ", marker form) ──\n")
	for _, h := range hint.DefaultHints {
		b.WriteString(h.Text)
		b.WriteByte('\n')
	}
	b.WriteString("\n── alerts ──\n")
	for _, ev := range []parser.CacheEvent{
		{Type: parser.OrchTTL},
		{Type: parser.SubagentCacheExpired, Detail: "conceptualist"},
		{Type: parser.CompactHeuristic},
		{Type: parser.ConfigError},
	} {
		b.WriteString(hint.BuildAlert([]parser.CacheEvent{ev}))
		b.WriteByte('\n')
	}
	testutil.CompareGolden(t, goldenPath(t, "s9-bottom-gallery"), b.String())
}

// TestGoldenPlainInvariant asserts the PLAIN goldens contain no leftover
// {{marker}} tokens and (at cols ≥ 80, where the engine promises a fit) that
// every header line fits its width. Marker goldens are exempt: they carry
// {{...}} by design, and the unified table renders at its own fixed width with
// the live-TTL suffix hanging in the right margin (guarded by the snapshot).
func TestGoldenPlainInvariant(t *testing.T) {
	for _, sc := range scenarios() {
		if sc.colour {
			continue
		}
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			b, err := os.ReadFile(goldenPath(t, sc.name))
			if err != nil {
				t.Fatalf("read golden (run -update first): %v", err)
			}
			content := string(b)
			if strings.Contains(content, "{{") {
				t.Errorf("%s: plain golden must not contain marker token '{{...}}'", sc.name)
			}
			if sc.cols < 80 {
				return
			}
			for i, line := range strings.Split(content, "\n") {
				if strings.ContainsAny(line, "│┌┐└┘├┤┬┴┼─") {
					continue // fixed-width table / margin TTL: guarded by the snapshot
				}
				if vl := format.VisualLen(line); vl > sc.cols {
					t.Errorf("%s:%d visual length %d > %d cols: %q", sc.name, i+1, vl, sc.cols, line)
				}
			}
		})
	}
}

// ─── Render helper (mirrors cmd/cc-probeline runRender, hermetic) ────────────

func renderScenario(t *testing.T, sc scenario) string {
	t.Helper()
	d, cfg := scenarioData(t, sc)

	// Colour scenarios: render with a token theme (markers stay as {{...}}) and
	// DO NOT Apply — the raw marker stream is the snapshot. Plain scenarios:
	// render with the zero theme then Apply to strip everything to text.
	if sc.colour {
		a := statusline.Assembler{Mode: sc.mode, Theme: markerTheme(), Cols: sc.cols, Config: cfg}
		return a.Render(d)
	}
	a := statusline.Assembler{Mode: sc.mode, Theme: renderer.Theme{}, Cols: sc.cols, Config: cfg}
	return renderer.Apply(a.Render(d), renderer.Theme{})
}

// scenarioData builds the hermetic probes.Data + resolved probes.Config for a
// scenario, mirroring cmd/cc-probeline runRender. Shared by renderScenario
// (golden snapshots, marker/plain themes) and the SVG-frame emitter
// (svgframes_test.go, real ANSI palette) so both observe the same pipeline.
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

	// Two-phase Reconcile: seed a $0 baseline (session started empty), then
	// reconcile the real ccTotal so the full amount becomes this session's delta
	// and is distributed across turns — otherwise a fresh session renders $0.00.
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
		SessionID:     "golden-" + sc.name,
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
		Now:              goldenNow,
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

// ─── Helpers ─────────────────────────────────────────────────────────────────

// markerTheme returns an AnsiEnabled theme whose ColorScheme fields are marker
// tokens rather than ESC sequences. Probes emit literal {{color:X}}/{{dim}}/
// {{reset}} markers; the few that read theme colours directly (quota Reset,
// progress-bar Green/Orange/Red/Yellow) then also emit markers, so the raw
// pre-Apply output is one consistent, ESC-free marker vocabulary.
func markerTheme() renderer.Theme {
	return renderer.Theme{
		AnsiEnabled: true,
		Colors: renderer.ColorScheme{
			Reset:      "{{reset}}",
			Dim:        "{{dim}}",
			DimGrey:    "{{dim}}",
			Bold:       "{{bold}}",
			Italic:     "{{italic}}",
			Cyan:       "{{color:cyan}}",
			Yellow:     "{{color:yellow}}",
			Red:        "{{color:red}}",
			Green:      "{{color:green}}",
			Orange:     "{{color:orange}}",
			Magenta:    "{{color:magenta}}",
			BoldGreen:  "{{color:bold_green}}",
			BoldYellow: "{{color:bold_yellow}}",
			BoldRed:    "{{color:bold_red}}",
		},
	}
}

// richProbesCfg is the all-widgets-on config with a roomy table so the master's
// multiple request groups (and their subagent ↳ rows) surface in the snapshot.
func richProbesCfg() probes.Config {
	c := config.ToProbesConfig(*config.Default())
	c.TableRows = 20
	return c
}

func goldenPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(testutil.ProjectRoot(t), "tests/statusline/testdata/golden", name+".txt")
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
		FiveHour: stdin.RateWindow{UsedPercentage: pct5h, ResetsAt: jsonTime(goldenNow.Add(reset5h))},
		SevenDay: stdin.RateWindow{UsedPercentage: pct7d, ResetsAt: jsonTime(goldenNow.Add(reset7d))},
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

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
