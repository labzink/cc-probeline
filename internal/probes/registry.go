package probes

// Probe registries define the display order of probes on each status-line row.
// Each registry is a fixed-order slice: position in the slice == left-to-right
// render position for that row.
//
// Population strategy: subtasks 4.1.a, 4.1.b, 4.1.c append their probe
// instances via explicit slice literals in their respective source files.
// No init() magic — the final ordering is visible here in one place.
//
// Row assignment (from Phase 4 concept §A4):
//
//	Line0Registry    — P0 probes: email, project name, quota indicator
//	Line1Registry    — P1 probes: model, effort, git, ctx, cost, time
//	Line2Registry    — P2 probes: cache aggregate row (CacheProbe)
//	SubagentRegistry — subagent status probes (one per active subagent)
var (
	Line0Registry    []Probe
	Line1Registry    []Probe
	Line2Registry    []Probe
	SubagentRegistry []Probe
)
