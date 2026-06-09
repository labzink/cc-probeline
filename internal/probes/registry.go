package probes

// Probe registries define the display order of probes on each status-line row.
// Each registry is a fixed-order slice: position in the slice == left-to-right
// render position for that row.
//
// Row assignment (from Phase 4 concept §A4):
//
//	Line0Registry    — row 0 of the multiline output: email, project name, quota indicator
//	Line1Registry    — row 1 of the multiline output: model, effort, git, ctx, cost, time
//	Line2Registry    — row 2 of the multiline output: cache aggregate row (CacheProbe)
//	SubagentRegistry — subagent status probes (one per active subagent)
var (
	// Line0Registry holds probes rendered on the first status line (row 0).
	// Order: email, project, quota.
	Line0Registry = []Probe{
		&EmailProbe{},
		&ProjectProbe{},
		&QuotaProbe{},
	}

	// Line1Registry holds probes rendered on the second status line (row 1).
	// Order: model (includes effort icon inline), git, ctx, cost, time.
	Line1Registry = []Probe{
		&ModelProbe{},
		&GitProbe{},
		&CtxProbe{},
		&CostProbe{},
		&TimeProbe{},
	}

	// Line2Registry is empty since Phase 6.9.e: the cache-aggregate row (line 2)
	// was removed in favour of per-row cache columns + TTL suffix in the table
	// (T-13). CacheProbe was deleted in Phase 7 (BL-33).
	Line2Registry = []Probe{}

	// SubagentRegistry holds the subagent status probe, rendered only in the
	// subagent status line code-path.
	SubagentRegistry = []Probe{
		&SubagentProbe{},
	}
)
