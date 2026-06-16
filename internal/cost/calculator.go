// Package cost implements the delta-based cost API for cc-probeline.
//
// Rationale: instead of maintaining a per-model pricing table (error-prone
// as Anthropic changes prices), we take the ccTotal USD value reported by CC
// itself and compute deltas relative to a per-session baseline captured on
// the first Reconcile call.
//
// See project_cost_methodology memory and spec-common.md §2.2/§2.3.
package cost

import (
	"fmt"
	"log/slog"
	"os"
	"sort"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/state"
)

// Reconcile recomputes the per-turn cost map from scratch on every call
// (Phase 7.46 — table-driven, no dependence on the lagging official ccTotal).
//
// Each turn's cost is estimated directly from its own token counts and the
// model price table (see estimateTurnUSD). Because a turn's tokens are fixed
// the moment it lands in the JSONL, its estimate is computed once and never
// moves — eliminating the per-tick "dancing numbers" of the old B2 scheme,
// which spread the whole (lagging, subagent-trickling) ccTotal across all turns
// every tick and made the biggest turn's displayed cost crawl for many ticks.
//
// SessionTotal is the sum of these per-turn estimates (immediate, no lag).
// The official ccTotal no longer drives any displayed value; price freshness is
// handled out-of-band by the network price table (Phase 7.46 Wave B).
//
// First call (st.Initialized=false): captures BaselineDurMS=durMS (for
// SessionDuration) and BaselineTurnTime, sets Initialized=true. BaselineCost is
// retained for compatibility but no longer feeds SessionTotal.
//
// LastSeenTotal / PromptCost are kept only to baseline prompt groups for
// LastRequest; they never drive the per-turn estimates.
func Reconcile(st *state.Session, ccTotal float64, durMS int64, turns []parser.Turn) {
	slog.Debug("cost.Reconcile start", "initialized", st.Initialized, "ccTotal", ccTotal, "durMS", durMS, "turns", len(turns))

	if !st.Initialized {
		st.BaselineCost = ccTotal
		st.BaselineDurMS = durMS
		st.BaselineTurnTime = maxTurnTime(turns)
		st.LastSeenTotal = ccTotal
		st.Initialized = true
		recomputePerTurn(st, turns)
		recomputeRecon(st, turns)
		recordCostSample(st, ccTotal, durMS, turns)
		slog.Info("cost.Reconcile baseline captured", "baseline", ccTotal, "baselineDurMS", durMS, "baselineTurnTime", st.BaselineTurnTime)
		return
	}

	// Record PromptCost[groupID] = cost at the start of each newly seen group,
	// using the pre-rise LastSeenTotal (the running total when the group began).
	// LastRequest = ccTotal − PromptCost[group].
	if st.PromptCost == nil {
		st.PromptCost = make(map[int]float64)
	}
	for _, t := range turns {
		if t.GroupID > 0 {
			if _, seen := st.PromptCost[t.GroupID]; !seen {
				st.PromptCost[t.GroupID] = st.LastSeenTotal
			}
		}
	}

	// LastSeenTotal is a monotonic high-water mark: a transient CC dip must not
	// lower it, else a later rise back would mis-baseline a prompt group.
	if ccTotal > st.LastSeenTotal {
		st.LastSeenTotal = ccTotal
	}

	recomputePerTurn(st, turns)
	recomputeRecon(st, turns)
	recordCostSample(st, ccTotal, durMS, turns)
}

// reconRdChkMin is the minimum cache-chain overshoot (in tokens) that triggers a
// reconstructed missing-turn surcharge. Values of 0/1 are treated as chain noise.
const reconRdChkMin = 1

// recomputeRecon rebuilds st.ReconCost for the current tick from the orchestrator
// cache chain. Walking orchestrator turns oldest-first, a turn's cache_read should
// equal the previous orchestrator turn's read+write (what was cached + what it just
// wrote). When it exceeds that by rdChk>reconRdChkMin tokens, a turn went MISSING
// from the JSONL between them: it re-read the full context (read = prev read+write)
// and wrote a small diff (write1h = rdChk tokens). Its cost — priced from THIS
// turn's model — is attributed to this turn as a surcharge, so the table reflects
// money CC billed that our token-only estimate cannot see. Subagents run a separate
// cache and are skipped; a cache reset (read dropped, e.g. /compact) breaks the
// chain and takes no surcharge. Derived fresh each tick (never cached) so it cannot
// double-count across ticks.
func recomputeRecon(st *state.Session, turns []parser.Turn) {
	orch := make([]parser.Turn, 0, len(turns))
	for _, t := range turns {
		if !t.IsSidechain && t.UUID != "" {
			orch = append(orch, t)
		}
	}
	sort.SliceStable(orch, func(i, j int) bool {
		return orch[i].Timestamp.Before(orch[j].Timestamp)
	})

	recon := make(map[string]float64)
	var prevRead, prevWrite int
	havePrev := false
	for _, t := range orch {
		read := t.Tokens.CacheRead
		w5, w1 := turnWrites(t)
		write := w5 + w1
		if havePrev && read >= prevRead {
			if rdChk := read - prevRead - prevWrite; rdChk > reconRdChkMin {
				w := ModelWeights(t.Model)
				vread := prevRead + prevWrite
				recon[t.UUID] = (float64(vread)*w.CacheRead + float64(rdChk)*w.CacheCreate1h) / 1e6
			}
		}
		prevRead, prevWrite = read, write
		havePrev = true
	}
	st.ReconCost = recon
	slog.Debug("cost.recomputeRecon", "reconstructed", len(recon))
}

// maxCostHistory caps the diagnostic Session.CostHistory trail (Phase 7.46).
const maxCostHistory = 400

// recordCostSample appends one Session.CostHistory entry for this tick — but
// only when the official ccTotal advanced since the last recorded sample (so a
// burst of identical 5-second ticks does not flood the trail). Each sample pairs
// the official ccTotal with our running estimate (Σ PerTurnCost), the API
// duration, the turn count, and the newest turn's UUID, so the reconciliation
// (official vs estimate, and the lag of the official meter) can be reconstructed
// offline. Diagnostic only — never drives display.
func recordCostSample(st *state.Session, ccTotal float64, durMS int64, turns []parser.Turn) {
	if n := len(st.CostHistory); n > 0 && st.CostHistory[n-1].CCTotal == ccTotal {
		return
	}
	var est float64
	for _, v := range st.PerTurnCost {
		est += v
	}
	sample := state.CostSample{
		DurMS:      durMS,
		CCTotal:    ccTotal,
		Estimate:   est,
		Turns:      len(st.PerTurnCost),
		NewestTurn: newestTurnUUID(turns),
	}
	if costDebugEnabled() {
		addCostDebug(&sample, st, turns)
	}
	st.CostHistory = append(st.CostHistory, sample)
	if len(st.CostHistory) > maxCostHistory {
		st.CostHistory = st.CostHistory[len(st.CostHistory)-maxCostHistory:]
	}
}

// costDebugEnabled reports whether the Phase 7.46 cost debug instrumentation is
// active (env CC_PROBELINE_COST_DEBUG=1). Off by default — release samples carry
// only the basic fields.
func costDebugEnabled() bool {
	return os.Getenv("CC_PROBELINE_COST_DEBUG") == "1"
}

// addCostDebug fills the debug instrumentation fields of sample from the current
// turns and per-turn cost map: the session-wide token vector by class, the same
// vector split per model, and the orchestrator vs subagent estimate split. The
// write classes use turnWrites so they bucket identically to the estimator.
func addCostDebug(sample *state.CostSample, st *state.Session, turns []parser.Turn) {
	var total state.TokenVec
	byModel := make(map[string]state.TokenVec)
	var estMain, estSub float64
	for _, t := range turns {
		w5, w1 := turnWrites(t)
		add := func(v *state.TokenVec) {
			v.In += t.Tokens.Input
			v.Read += t.Tokens.CacheRead
			v.W5 += w5
			v.W1 += w1
			v.Out += t.Tokens.Output
		}
		add(&total)
		key := parser.CanonicalModelKey(t.Model)
		mv := byModel[key]
		add(&mv)
		byModel[key] = mv

		if t.UUID != "" {
			if t.IsSidechain {
				estSub += st.PerTurnCost[t.UUID]
			} else {
				estMain += st.PerTurnCost[t.UUID]
			}
		}
	}
	sample.Tokens = &total
	sample.ByModel = byModel
	sample.EstMain = estMain
	sample.EstSub = estSub
}

// newestTurnUUID returns the UUID of the turn with the latest Timestamp, or ""
// when turns is empty. Used to tag a cost sample with the turn most likely to
// have triggered the official ccTotal advance.
func newestTurnUUID(turns []parser.Turn) string {
	var newest time.Time
	var uuid string
	for _, t := range turns {
		if t.UUID != "" && t.Timestamp.After(newest) {
			newest = t.Timestamp
			uuid = t.UUID
		}
	}
	return uuid
}

// recomputePerTurn rebuilds st.PerTurnCost for the current tick, reusing the
// previously persisted map as a cache. A turn's tokens are fixed the moment it
// lands in the JSONL, so its USD estimate never changes — therefore a UUID
// already present in the prior map is copied verbatim and estimateTurnUSD runs
// only for UUIDs seen for the first time. This eliminates the per-tick
// re-pricing of every historical turn (a 400-turn session no longer recomputes
// 400 estimates every ~5 seconds).
//
// The map is still rebuilt fresh (not mutated in place) so it contains exactly
// the current turns: any UUID that disappeared from the JSONL — e.g. after a
// /compact rewrite — naturally drops out instead of lingering and inflating
// SessionTotal. SessionTotal is the sum of this map; SubagentTotal sums the
// subagent turns' entries.
func recomputePerTurn(st *state.Session, turns []parser.Turn) {
	prev := st.PerTurnCost
	m := make(map[string]float64, len(turns))
	reused, priced := 0, 0
	for _, t := range turns {
		if t.UUID == "" {
			continue
		}
		if v, ok := prev[t.UUID]; ok {
			m[t.UUID] = v
			reused++
			continue
		}
		m[t.UUID] = estimateTurnUSD(t)
		priced++
	}
	st.PerTurnCost = m
	slog.Debug("cost.recomputePerTurn", "turns", len(m), "reused", reused, "priced", priced)
}

// estimateTurnUSD returns the standalone USD estimate for one turn from its own
// token counts and the model price table:
//
//	est = (in·In + read·Read + write5m·Write5m + write1h·Write1h + out·Out) / 1e6
//
// Cache-write tokens are split by TTL using the parsed CacheCreate5m/CacheCreate1h
// fields (5-minute writes cost 1.25×In, 1-hour writes 2×In). When CC reports
// only the lumped CacheCreate (older versions, no split), the TTL is inferred
// from the author: orchestrator turns cache at 1-hour TTL, subagent (sidechain)
// turns at 5-minute.
func estimateTurnUSD(t parser.Turn) float64 {
	w := ModelWeights(t.Model)
	write5m, write1h := turnWrites(t)
	units := float64(t.Tokens.Input)*w.In +
		float64(t.Tokens.CacheRead)*w.CacheRead +
		float64(write5m)*w.CacheCreate +
		float64(write1h)*w.CacheCreate1h +
		float64(t.Tokens.Output)*w.Out
	return units / 1e6
}

// turnWrites splits a turn's cache-write tokens into (5-minute, 1-hour) classes.
// It prefers the parsed per-TTL fields; when CC reports only a lumped
// CacheCreate count, the TTL is inferred from the author — orchestrator turns
// cache at 1-hour TTL, subagent (sidechain) turns at 5-minute. Shared by the
// estimator and the cost debug instrumentation so both bucket writes identically.
func turnWrites(t parser.Turn) (write5m, write1h int) {
	write5m = t.Tokens.CacheCreate5m
	write1h = t.Tokens.CacheCreate1h
	if write5m == 0 && write1h == 0 && t.Tokens.CacheCreate > 0 {
		if t.IsSidechain {
			write5m = t.Tokens.CacheCreate
		} else {
			write1h = t.Tokens.CacheCreate
		}
	}
	return write5m, write1h
}

// maxTurnTime returns the newest Timestamp among the given turns, or the zero
// time when turns is empty. Used to fix BaselineTurnTime at observation start.
func maxTurnTime(turns []parser.Turn) time.Time {
	var max time.Time
	for _, t := range turns {
		if t.Timestamp.After(max) {
			max = t.Timestamp
		}
	}
	return max
}

// SessionDuration returns the API duration elapsed since the baseline was
// captured on the first Reconcile call:
//
//	SessionDuration = durMS − st.BaselineDurMS
//
// Resets naturally on /clear because a new session_id produces a fresh state
// with a new BaselineDurMS. Returns 0 when st is nil or not yet initialized.
func SessionDuration(st *state.Session, durMS int64) int64 {
	if st == nil || !st.Initialized {
		return 0
	}
	return durMS - st.BaselineDurMS
}

// SubagentTotal returns the cumulative PerTurnCost for all turns whose UUID
// appears in the given list. Turns not present in st.PerTurnCost contribute 0.
// Returns 0 when st is nil or st.PerTurnCost is nil.
func SubagentTotal(st *state.Session, turnUUIDs []string) float64 {
	if st == nil || st.PerTurnCost == nil {
		return 0
	}
	var total float64
	for _, uuid := range turnUUIDs {
		total += st.PerTurnCost[uuid]
	}
	return total
}

// PerTurn returns the finalized cost for the given turn UUID: the token-table
// estimate plus any reconstructed missing-turn surcharge attributed to it
// (st.ReconCost, see recomputeRecon). Returns (0, false) when st is nil or the
// UUID is not in st.PerTurnCost. The caller should render "—" when ok=false.
// The surcharge is why the table header label is "~cost" — the per-turn figure
// is our estimate (reconstructed), not the official meter.
func PerTurn(st *state.Session, turnUUID string) (float64, bool) {
	if st == nil || st.PerTurnCost == nil {
		return 0, false
	}
	v, ok := st.PerTurnCost[turnUUID]
	if !ok {
		return 0, false
	}
	return v + st.ReconCost[turnUUID], true // ReconCost nil-safe: missing key → 0
}

// SessionTotal returns the session cost as the sum of the per-turn estimates
// built by Reconcile (Phase 7.46: table-driven, immediate, no dependence on the
// lagging official ccTotal — the ccTotal argument is ignored and kept only for
// call-site compatibility). The map already covers every turn (orchestrator and
// subagent), so the sum is the full-session estimate. Returns 0 when st is nil
// or no turns have been priced yet.
func SessionTotal(st *state.Session, _ float64) float64 {
	if st == nil {
		return 0
	}
	var total float64
	for _, v := range st.PerTurnCost {
		total += v
	}
	for _, v := range st.ReconCost { // reconstructed missing-turn surcharges
		total += v
	}
	return total
}

// LastRequest returns the cost attributable to the current prompt group:
//
//	LastRequest = ccTotal − st.PromptCost[curGroupID]
//
// When curGroupID is absent from PromptCost, the missing entry defaults to 0,
// so LastRequest = ccTotal (safe default: full session cost).
func LastRequest(st *state.Session, ccTotal float64, curGroupID int) float64 {
	if st.PromptCost == nil {
		return ccTotal
	}
	baseline := st.PromptCost[curGroupID] // zero value when absent
	return ccTotal - baseline
}

// Format renders a USD amount as "$X.XX". Used by the renderer footer.
func Format(usd float64) string {
	return fmt.Sprintf("$%.2f", usd)
}
