package probes

// Probe registries define the display order of probes on each status-line row.
// Each registry is a fixed-order slice: position in the slice == left-to-right
// render position for that row.
//
// Row assignment (from Phase 4 concept §A4):
//
//	Line0Registry    — P0 probes: email, project name, quota indicator
//	Line1Registry    — P1 probes: model, effort, git, ctx, cost, time
//	Line2Registry    — P2 probes: cache aggregate row (CacheProbe)
//	SubagentRegistry — subagent status probes (one per active subagent)
var (
	// Line0Registry holds P0 probes rendered on the first status line.
	// Order: email, project, quota.
	Line0Registry = []Probe{
		&EmailProbe{},
		&ProjectProbe{},
		&QuotaProbe{},
	}

	// Line1Registry holds P1/P2 probes rendered on the second status line.
	// Order: model, effort, git, ctx, cost, time.
	Line1Registry = []Probe{
		&ModelProbe{},
		&EffortProbe{},
		&GitProbe{},
		&CtxProbe{},
		&CostProbe{},
		&TimeProbe{},
	}

	// Line2Registry holds the cache aggregate row (single probe renders the whole row).
	Line2Registry = []Probe{
		&CacheProbe{},
	}

	// SubagentRegistry holds the subagent status probe, rendered only in the
	// subagent status line code-path.
	SubagentRegistry = []Probe{
		&SubagentProbe{},
	}
)
