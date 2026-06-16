// Package statusline composes the multi-line status output from the Phase 4.1
// probes Registry, the Phase 4.2 box-drawing table, and the Phase 4.4 hint
// widget. It lives above probes and renderer to avoid an import cycle:
// probes depends on renderer (Theme), and the assembler depends on both.
package statusline

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/labzink/cc-probeline/internal/cost"
	"github.com/labzink/cc-probeline/internal/hint"
	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/state"
)

// Assembler joins probes from the Registry into the final multi-line status
// string. Cols is the detected terminal width; Mode gates whether the per-turn
// table and footer are emitted.
type Assembler struct {
	Mode   mode.Mode
	Theme  renderer.Theme
	Cols   int
	Config probes.Config
}

// Render produces the full status string with marker tokens (no ANSI escapes).
// Caller is responsible for piping the result through renderer.Apply.
//
// Output structure:
//
//	line0  — probes from Line0Registry, separated by "{{dim}} • {{reset}}"
//	line1  — probes from Line1Registry, separated by "{{dim}} • {{reset}}"
//	line2  — probes from Line2Registry, separated by " | "
//	[table] — Standard mode only: last 20 per-turn rows + footer
//	[hint]  — appended when non-empty (C-12)
func (a *Assembler) Render(d probes.Data) string {
	// Resolve effective terminal width (§4.3 T-10).
	cols := a.Cols
	if cols == 0 {
		cols = renderer.DetectCols()
	}

	// Build the three header lines via FitLine (§4.3 T-11).
	const bulletSep = "{{dim}} • {{reset}}"
	const pipeSep = " | "

	// T-21: line0 uses registry order (no Priority sort) so email/project/quota
	// appear in registration order.
	// I2: line1 restores Priority-based sort so git (P=2) appears to the right of
	// ctx/cost/time (P=1). sortByPriority=false was an over-reach of the T-21 fix.
	l0 := renderer.FitLine(a.buildProbeEntries(probes.Line0Registry, d, false), cols, bulletSep)
	l1 := renderer.FitLine(a.buildProbeEntries(probes.Line1Registry, d, true), cols, bulletSep)

	lines := []string{l0, l1}

	// Phase 6.9.e (T-13): the cache-aggregate row (line 2) is removed from the
	// Standard (table) output — its data now lives in the per-turn table columns
	// and the per-row TTL suffix. SuperCompact has no table, so it keeps its
	// compact cache line.
	if a.Mode != mode.Standard {
		l2 := renderer.FitLine(a.buildProbeEntries(probes.Line2Registry, d, true), cols, pipeSep)
		lines = append(lines, l2)
	}

	// Standard mode: append per-turn table when there are rows (C-6).
	if a.Mode == mode.Standard {
		tableLines := a.perTurnTable(d, cols)
		lines = append(lines, tableLines...)
	}

	// Phase 4.4 hint widget (C-12): only append when non-empty.
	if h := a.hint(d); h != "" {
		lines = append(lines, h)
	}

	return strings.Join(lines, "\n")
}

// buildProbeEntries converts a probe registry slice into []renderer.ProbeEntry
// for consumption by FitLine. Invisible probes are excluded (§4.3 T-11).
//
// When sortByPriority is false (line0, line1), registry order is preserved:
// probes appear left-to-right in registration order; Priority is used only by
// FitLine/levelForPass for collapse ordering (T-21).
//
// When sortByPriority is true (line2), entries are sorted ascending by Priority
// so lower-number (higher-importance) probes appear leftmost (Phase 4.2 §C-9).
func (a *Assembler) buildProbeEntries(ps []probes.Probe, d probes.Data, sortByPriority bool) []renderer.ProbeEntry {
	entries := make([]renderer.ProbeEntry, 0, len(ps))
	for _, p := range ps {
		if !p.Visible(d, a.Config) {
			continue
		}
		pp := p // capture loop variable
		entries = append(entries, renderer.ProbeEntry{
			Priority: pp.Priority(),
			Render: func(level int) string {
				return pp.Render(d, a.Config, a.Theme, probes.Level(level))
			},
		})
	}
	if sortByPriority {
		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].Priority < entries[j].Priority
		})
	}
	return entries
}

// timedRow pairs a built UnifiedRow with the timestamp used to sort it into the
// merged newest-first stream (orchestrator turn Timestamp, subagent
// ActivationStart).
type timedRow struct {
	row renderer.UnifiedRow
	ts  time.Time
}

// perTurnTable builds the redesigned per-turn table for Standard mode (Phase
// 6.9.e). Orchestrator turns become one row each; every subagent collapses into
// a single row (last turn data, ↳N, cumulative Σ $ cost). Rows are merged into a
// single newest-first stream (orch by Timestamp, subagent by ActivationStart),
// capped to 20, then rendered with per-row TTL suffixes, dim wrapping and red
// cache-write markers.
//
// Returns nil when there are no turns.
func (a *Assembler) perTurnTable(d probes.Data, cols int) []string {
	if d.Session == nil || len(d.Session.Turns) == 0 {
		return nil
	}

	var st *state.Session
	if d.State != nil {
		st = d.State
	}

	timed := a.orchRows(d)

	// Compute freshestGroupStart: the minimum Timestamp among orchestrator turns
	// whose GroupID equals the highest group seen (maxOrchGroup). This marks the
	// chronological beginning of the current (freshest) orchestrator request.
	// Subagents activated strictly before this instant belong to an older request
	// and must be rendered dim (F15).
	freshestGroupStart := freshestGroupStartTime(d)

	timed = append(timed, a.subagentRows(d, st, freshestGroupStart, cols)...)

	// Merge newest-first by sort timestamp (orch Timestamp / sub ActivationStart).
	sort.SliceStable(timed, func(i, j int) bool {
		return timed[i].ts.After(timed[j].ts)
	})
	// Phase 6.95: cap the table at the user-configured row count ([general].table_rows,
	// default 10, capped ≤40 by the config setter). Zero means "not configured" →
	// fall back to the default. Older builds hard-capped at 20.
	rowCap := a.Config.TableRows
	if rowCap <= 0 {
		rowCap = 10
	}
	if len(timed) > rowCap {
		timed = timed[:rowCap]
	}

	rows := make([]renderer.UnifiedRow, len(timed))
	for i := range timed {
		rows[i] = timed[i].row
	}

	b := renderer.NewBuilder(cols)
	raw := b.RenderUnifiedRows(rows)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// orchRows builds the orchestrator UnifiedRows from d.Session.Turns (newest
// first), computing the freshest-group dim split, per-row TTL suffix (live on
// the freshest turn, frozen "⏱ 0m" on older expired turns — T-33) and the red
// cache-write marker (cache collapse / model switch — T-34/T-35).
func (a *Assembler) orchRows(d probes.Data) []timedRow {
	var st *state.Session
	if d.State != nil {
		st = d.State
	}

	// Orchestrator turns only, newest-first.
	orch := make([]parser.Turn, 0, len(d.Session.Turns))
	for _, t := range d.Session.Turns {
		if !t.IsSidechain {
			orch = append(orch, t)
		}
	}
	sort.SliceStable(orch, func(i, j int) bool {
		return orch[i].Timestamp.After(orch[j].Timestamp)
	})
	if len(orch) == 0 {
		return nil
	}

	maxOrchGroup := orch[0].GroupID
	for _, t := range orch {
		if t.GroupID > maxOrchGroup {
			maxOrchGroup = t.GroupID
		}
	}

	now := d.Now
	if now.IsZero() {
		now = time.Now()
	}

	out := make([]timedRow, 0, len(orch))
	for i, t := range orch {
		model := strings.TrimPrefix(t.Model, "claude-")
		costCell := "—"
		if v, ok := cost.PerTurn(st, t.UUID); ok {
			costCell = cost.Format(v)
		}
		hash := ""
		if t.Index > 0 {
			hash = fmt.Sprintf("%d", t.Index)
		}
		tool := t.ToolUse
		if t.Thinking {
			tool = renderer.ThinkingGlyph
		}

		// "previous orch turn" = the next (older) entry in newest-first order.
		var prev *parser.Turn
		if i+1 < len(orch) {
			prev = &orch[i+1]
		}
		// "next-newer orch turn" = the previous (newer) entry.
		var next *parser.Turn
		if i-1 >= 0 {
			next = &orch[i-1]
		}

		row := renderer.UnifiedRow{
			HashCell:      hash,
			Role:          t.Role,
			Model:         model,
			CacheRead:     t.Tokens.CacheRead,
			CacheCreate:   t.Tokens.CacheCreate,
			Out:           t.Tokens.Output,
			CostCell:      costCell,
			Tool:          tool,
			IsSidechain:   false,
			Dim:           t.GroupID < maxOrchGroup,
			GroupID:       t.GroupID,
			RedCacheWrite: orchRedCacheWrite(t, prev, a.Config.OrchTTLMinutes),
			TTLSuffix:     a.orchTTLSuffix(t, next, now, i == 0),
		}
		out = append(out, timedRow{row: row, ts: t.Timestamp})
	}
	return out
}

// orchRedCacheWrite reports whether the orchestrator turn t should paint its
// cache_create (write) sub-token red: the gap from the previous orch turn is
// ≥ the TTL window (cache expired — T-34) OR the model changed (cold cache —
// T-35). Returns false when there is no previous orch turn.
func orchRedCacheWrite(t parser.Turn, prev *parser.Turn, orchTTLMinutes int) bool {
	if prev == nil {
		return false
	}
	if renderer.CacheExpiredAt(prev.Timestamp, t.Timestamp, orchTTLMinutes) {
		return true
	}
	return t.Model != prev.Model
}

// orchTTLSuffix returns the TTL suffix for an orchestrator row. The freshest
// (top) row gets a live TTL from now. An older row gets a frozen "⏱ 0m" only
// when its cache expired before the next-newer orch turn arrived (gap ≥ window,
// T-33); otherwise it carries no suffix.
func (a *Assembler) orchTTLSuffix(t parser.Turn, next *parser.Turn, now time.Time, fresh bool) string {
	if fresh {
		return renderer.CacheTTL(now, t.Timestamp, 1, a.Config.OrchTTLMinutes, a.Theme.AnsiEnabled)
	}
	if next != nil && renderer.CacheExpiredAt(t.Timestamp, next.Timestamp, a.Config.OrchTTLMinutes) {
		// Frozen: reuse CacheTTL with now = next.Timestamp → remaining ≤ 0 branch.
		return renderer.CacheTTL(next.Timestamp, t.Timestamp, 1, a.Config.OrchTTLMinutes, a.Theme.AnsiEnabled)
	}
	return ""
}

// freshestGroupStartTime returns the minimum Timestamp among all orchestrator
// turns (non-sidechain) whose GroupID equals the maximum GroupID seen. This is
// the chronological start of the current (freshest) request. Returns the zero
// time when there are no orchestrator turns (callers treat zero as "no boundary"
// — Before(zero) is always false, so subagents are never dim).
func freshestGroupStartTime(d probes.Data) time.Time {
	if d.Session == nil {
		return time.Time{}
	}
	var maxGroup int
	for _, t := range d.Session.Turns {
		if !t.IsSidechain && t.GroupID > maxGroup {
			maxGroup = t.GroupID
		}
	}
	var minTS time.Time
	for _, t := range d.Session.Turns {
		if t.IsSidechain || t.GroupID != maxGroup {
			continue
		}
		if minTS.IsZero() || t.Timestamp.Before(minTS) {
			minTS = t.Timestamp
		}
	}
	return minTS
}

// subagentRows builds one collapsed UnifiedRow per subagent. The row shows the
// last turn's tokens/tool, "↳N" (N=CurrentTurnNum), a cumulative "Σ $" cost
// (SubagentTotal over the agent's turn UUIDs) and a live per-agent TTL. The
// cache_create is painted red when the gap between the last two turns is ≥ the
// TTL window (collapse — T-36); the TTL is never frozen for subagents. At
// terminal widths > 100 the tool cell is prefixed with the joined instance name.
// freshestGroupStart is the chronological start of the current orchestrator
// request; a subagent whose ActivationStart is strictly before this instant
// belongs to a completed request and must render dim (F15).
func (a *Assembler) subagentRows(d probes.Data, st *state.Session, freshestGroupStart time.Time, cols int) []timedRow {
	if len(d.Subagents) == 0 {
		return nil
	}

	now := d.Now
	if now.IsZero() {
		now = time.Now()
	}

	out := make([]timedRow, 0, len(d.Subagents))
	for _, sa := range d.Subagents {
		if len(sa.Turns) == 0 {
			continue
		}
		last := sa.Turns[len(sa.Turns)-1]
		model := strings.TrimPrefix(sa.Model, "claude-")
		if model == "" {
			model = strings.TrimPrefix(last.Model, "claude-")
		}

		role := sa.AgentType
		if role == "" {
			role = "agent"
		}

		// Cumulative cost over all of the agent's turn UUIDs.
		uuids := make([]string, 0, len(sa.Turns))
		for _, tn := range sa.Turns {
			uuids = append(uuids, tn.UUID)
		}
		costCell := "Σ " + cost.Format(cost.SubagentTotal(st, uuids))

		// Tool cell: last turn's tool; "<name≤16>: <tool>" at wide terminals.
		tool := last.ToolUse
		if tool == "" {
			tool = sa.LastTool
		}
		if cols > 100 {
			name := a.instanceName(sa)
			if name != "" {
				display := tool
				if display == "" {
					display = renderer.ThinkingGlyph
				}
				tool = name + ": " + display
			}
		}

		// Dim when the subagent was activated before the current (freshest) orch
		// request started — it belongs to a completed request (F15).
		dim := !freshestGroupStart.IsZero() && sa.ActivationStart.Before(freshestGroupStart)

		row := renderer.UnifiedRow{
			HashCell:      "↳" + fmt.Sprintf("%d", sa.CurrentTurnNum),
			Role:          role,
			Model:         model,
			CacheRead:     last.Tokens.CacheRead,
			CacheCreate:   last.Tokens.CacheCreate,
			Out:           last.Tokens.Output,
			CostCell:      costCell,
			Tool:          tool,
			IsSidechain:   true,
			Dim:           dim,
			GroupID:       0,
			SkipSeparator: true,
			RedCacheWrite: subagentRedCacheWrite(sa, a.Config.SubagentGapMinutes),
			TTLSuffix:     subagentTTLSuffix(now, sa.LastTimestamp, dim, a.Config.SubagentGapMinutes, a.Theme.AnsiEnabled),
		}
		out = append(out, timedRow{row: row, ts: sa.ActivationStart})
	}
	return out
}

// subagentTTLHoldMinutes is how long a subagent row keeps showing "⏱ 0m" after
// its cache window has expired, before the TTL suffix is dropped entirely. Unlike
// the orchestrator (whose expired "⏱ 0m" stays forever as a cache-rebuild marker),
// a subagent is short-lived: once it has been silent for window+hold minutes the
// suffix disappears (F3).
const subagentTTLHoldMinutes = 5

// subagentTTLSuffix renders the subagent row's TTL suffix using the subagent
// cache window (SubagentGapMinutes, default 5). While elapsed ≤ window it counts
// down; between window and window+hold it shows the expired "⏱ 0m".
//
// The drop (past window+hold → "") is gated on dim: only a subagent whose row has
// gone dim (its request moved into the past) drops its TTL. While the subagent is
// still in the active (non-dim) group, "⏱ 0m" holds indefinitely — so a freshly
// active subagent does not lose its TTL just because it has been silent a while.
// The moment a new human prompt dims the row, the window+hold countdown applies:
// if >window+hold has already elapsed the suffix drops at once, otherwise it
// lingers until the window+hold mark.
func subagentTTLSuffix(now, lastTS time.Time, dim bool, window int, ansi bool) string {
	if window <= 0 || lastTS.IsZero() {
		return ""
	}
	if dim && now.Sub(lastTS) > time.Duration(window+subagentTTLHoldMinutes)*time.Minute {
		return ""
	}
	return renderer.CacheTTL(now, lastTS, 1, window, ansi)
}

// subagentRedCacheWrite reports whether the subagent's collapsed row should
// paint its cache_create red: the gap between its last two turns is ≥ the
// subagent cache window (cache collapse — T-36). A model change within a subagent
// does NOT trigger this (orchestrator-only per spec). Fewer than two turns → false.
func subagentRedCacheWrite(sa parser.SubagentStats, windowMinutes int) bool {
	n := len(sa.Turns)
	if n < 2 {
		return false
	}
	return renderer.CacheExpiredAt(sa.Turns[n-2].Timestamp, sa.Turns[n-1].Timestamp, windowMinutes)
}

// instanceName resolves the subagent's display instance name from meta.json
// (the "description" field), truncated to 16 runes. Claude Code does not send a
// tasks[] array to the status line stdin, so there is no live name to join on;
// the description captured at spawn time is the only human-meaningful label
// (F7). Returns "" when the subagent has no description.
func (a *Assembler) instanceName(sa parser.SubagentStats) string {
	name := sa.Description
	if name == "" {
		return ""
	}
	runes := []rune(name)
	if len(runes) > 16 {
		name = strings.TrimRight(string(runes[:16]), " ,;:")
	}
	return name
}

// subagentAlertDetail builds the "<role>:<name>" Detail string for a
// SubagentCacheExpired alert, given the bare AgentID from the raw event.
//
// role = AgentType from SubagentStats (fallback "agent" when empty).
// name = Tasks[].Name where task.ID == AgentID (from stdin Payload).
// When the name cannot be resolved (no Tasks match), returns only the role.
func (a *Assembler) subagentAlertDetail(agentID string, d probes.Data) string {
	// Find the SubagentStats for this agentID.
	var sa *parser.SubagentStats
	for i := range d.Subagents {
		if d.Subagents[i].AgentID == agentID {
			sa = &d.Subagents[i]
			break
		}
	}
	role := "agent"
	if sa != nil && sa.AgentType != "" {
		role = sa.AgentType
	}

	// Look up name from Tasks[] by task.ID == agentID.
	for _, task := range d.Stdin.Tasks {
		if task.ID == agentID {
			if task.Name != "" {
				return role + ":" + task.Name
			}
			break
		}
	}
	return role
}

// hint returns the hint widget text for this session's rotation state.
// Returns "" when all hints have been shown and no critical alert is active
// (caller skips adding the hint row in that case, per C-12).
func (a *Assembler) hint(d probes.Data) string {
	// Phase 6.95.b: rotation state lives in state.Session.HintRotation (the
	// per-session state file), not a separate hint-<sid>.json. Read it from
	// d.State; when state is unavailable (SessionID==""), use a zero rotation
	// (memory-only — the advance below is simply not persisted).
	var rot hint.State
	if d.State != nil {
		rot = d.State.HintRotation
	}

	// D1 guard: skip session-derived alerts when no turns exist (§11).
	// A newly opened session has no turns; surfacing cache alerts at turn-zero
	// is noise because the cache has not been used yet.
	var cacheEvents []parser.CacheEvent
	if d.Session != nil && len(d.Session.Turns) > 0 {
		cacheEvents = d.Session.CacheEvents
	}
	// Merge subagent-scope events (Phase 6.95.d: inter-turn gap ≥ 5 min).
	// SubagentStats live outside SessionStats so Aggregate cannot include them.
	// Events are enriched with "<role>:<name>" Detail before recency filtering.
	if len(d.Subagents) > 0 {
		raw := parser.DetectSubagentCacheEvents(d.Subagents, d.Now)
		for _, ev := range raw {
			ev.Detail = a.subagentAlertDetail(ev.Detail, d)
			cacheEvents = append(cacheEvents, ev)
		}
	}

	// Phase 6.95.d: apply 2-min recency filter to transient events before
	// surfacing to hint.Widget. ConfigError (persistent) bypasses the filter.
	now := d.Now
	if now.IsZero() {
		now = time.Now()
	}
	var filteredEvents []parser.CacheEvent
	for _, ev := range cacheEvents {
		if hint.IsTransient(ev.Type) {
			if ev.Timestamp.IsZero() || now.Sub(ev.Timestamp) <= hint.TransientAlertWindow {
				filteredEvents = append(filteredEvents, ev)
			}
			// Events older than 2 min are silently dropped (no surfacing).
			continue
		}
		// ConfigError: always surface (persistent).
		filteredEvents = append(filteredEvents, ev)
	}

	// Phase 6: config-error alerts ALWAYS surface, even on an empty session.
	// ExtraCacheEvents are not session-derived and are not gated by D1.
	if len(d.ExtraCacheEvents) > 0 {
		filteredEvents = append(filteredEvents, d.ExtraCacheEvents...)
	}

	w := hint.Widget{
		State:      rot,
		StartIndex: d.HintStart,
		Events:     filteredEvents,
		UpdateHint: d.UpdateHint,
	}
	// now was initialised above for the recency filter; reuse it here.
	// (hypothesis insurance #3: deterministic clock for tests)
	text := w.Pick(now)
	// Persist the advanced rotation back into state.Session. main.Save(st)
	// after Render writes it to disk; when d.State is nil (no SessionID) the
	// advance is memory-only and simply discarded.
	if d.State != nil {
		d.State.HintRotation = w.State
	}
	return text
}

// Compile-time check: Assembler.Render must accept probes.Data.
// (Ensures refactors don't accidentally change the signature.)
var _ func(probes.Data) string = (&Assembler{}).Render
