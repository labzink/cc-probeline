package parser

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// agentFileRegexp matches agent-<id>.jsonl filenames and captures the id.
var agentFileRegexp = regexp.MustCompile(`^agent-([A-Za-z0-9_-]+)\.jsonl$`)

// SubagentStats is an aggregated snapshot of one subagent in the active session.
//
// Tie-key for subagentStatusLine widget rendering is AgentID. Phase 4 probe
// intersects this slice with stdin tasks[].id to select active subagents.
//
// Zero value (empty fields) is valid — e.g. for a freshly started subagent
// whose JSONL file is still empty.
type SubagentStats struct {
	// AgentID is the suffix of agent-<id>.jsonl and the "agentId" field inside
	// each JSONL record. Stable identifier within one CC session.
	AgentID string

	// AgentType is the "agentType" field from meta.json. Empty when meta.json
	// is absent. Examples: "general-purpose", "code-reviewer".
	AgentType string

	// Description is the "description" field from meta.json (user-provided).
	// May be long (~200 chars). Truncation is handled by Phase 4.
	Description string

	// Model is the canonical model name (see canonicalModelKey).
	// Taken from the last assistant record in the JSONL. "unknown" when JSONL
	// is empty or all records lack a model field.
	Model string

	// Tokens holds aggregated usage fields across all records (after Dedup).
	// Same type as SessionStats.Totals.
	Tokens TokenCounts

	// FirstTimestamp is the timestamp of the first assistant record (after
	// dedup+sort). Zero time when JSONL is empty.
	FirstTimestamp time.Time

	// LastTimestamp is the timestamp of the last assistant record. Used for
	// freshness checks in Phase 4/5.
	LastTimestamp time.Time

	// TurnCount and ToolUseCount mirror the same fields in SessionStats.
	TurnCount    int
	ToolUseCount int

	// LastTool is the name of the tool_use ContentBlock from the last record
	// that contained any tool_use block. Empty when no tool_use occurred.
	LastTool string

	// JSONLPath is the absolute path to agent-<id>.jsonl. Used by Phase 5 as
	// a cache key and for mtime-based invalidation.
	JSONLPath string
}

// subagentFile pairs a JSONL path with its sibling meta path before aggregation.
type subagentFile struct {
	agentID   string
	jsonlPath string
	metaPath  string
	modTime   time.Time // mtime of the JSONL file — used for sort tie-breaking
}

// subagentMeta holds the decoded content of agent-<id>.meta.json.
type subagentMeta struct {
	AgentType   string `json:"agentType"`
	Description string `json:"description"`
}

// CollectSubagents reads the subagents directory under sessionDir, parses every
// agent-*.jsonl via ParseLines, aggregates each, and returns a SubagentStats
// slice sorted by LastTimestamp descending (most recently active first).
//
// Behaviour:
//   - sessionDir is the directory <projects-root>/<slug>/<session-id>/.
//     CollectSubagents appends "subagents/" internally.
//   - Missing subagents/ dir → ([]SubagentStats{}, nil). Fail-soft.
//   - One agent-*.jsonl fails to open or parse → log Error, skip that subagent,
//     continue with others. The per-file error never propagates to the caller.
//   - meta.json missing or malformed → AgentType/Description remain "".
//     Logged at Warn level.
//   - Subagent JSONL with zero assistant records → SubagentStats with AgentID +
//     JSONLPath populated; all aggregates zero. Returned, not skipped.
//
// Returns a non-nil error only when listing subagents/ itself fails for an
// unexpected reason (ENOENT is the fail-soft case above).
func CollectSubagents(sessionDir string) ([]SubagentStats, error) {
	slog.Debug("parser.subagent: collect", "sessionDir", sessionDir)

	files, err := listSubagentFiles(sessionDir)
	if err != nil {
		return nil, err
	}
	slog.Info("parser.subagent: enumerated", "count", len(files))

	results := make([]SubagentStats, 0, len(files))

	for _, f := range files {
		meta, metaErr := parseMeta(f.metaPath)
		if metaErr != nil {
			slog.Warn("parser.subagent: meta unavailable",
				"agentID", f.agentID,
				"path", f.metaPath,
				"err", metaErr,
			)
			// Continue with zero meta — SubagentStats will have empty AgentType/Description.
		}

		fh, openErr := os.Open(f.jsonlPath)
		if openErr != nil {
			slog.Error("parser.subagent: open failed",
				"agentID", f.agentID,
				"path", f.jsonlPath,
				"err", openErr,
			)
			continue
		}
		records, parseErrs, scanErr := ParseLines(fh)
		fh.Close()

		if len(parseErrs) > 0 {
			slog.Debug("parser.subagent: parse errors",
				"agentID", f.agentID,
				"count", len(parseErrs),
			)
		}
		if scanErr != nil {
			slog.Error("parser.subagent: open failed",
				"agentID", f.agentID,
				"path", f.jsonlPath,
				"err", scanErr,
			)
			continue
		}

		if len(records) == 0 {
			slog.Info("parser.subagent: empty transcript", "agentID", f.agentID)
		}

		stats := aggregateSubagent(records)
		stats.AgentID = f.agentID
		stats.AgentType = meta.AgentType
		stats.Description = meta.Description
		stats.JSONLPath = f.jsonlPath

		results = append(results, stats)
	}

	// Sort: LastTimestamp DESC; tie-break by AgentID ASC (deterministic).
	sort.SliceStable(results, func(i, j int) bool {
		ti := results[i].LastTimestamp
		tj := results[j].LastTimestamp
		if ti.Equal(tj) {
			return results[i].AgentID < results[j].AgentID
		}
		return ti.After(tj)
	})

	return results, nil
}

// listSubagentFiles enumerates agent-*.jsonl files under sessionDir/subagents/
// and pairs each with its sibling agent-*.meta.json.
//
// Missing subagents/ dir → ([], nil). Other ReadDir errors → ([], err).
func listSubagentFiles(sessionDir string) ([]subagentFile, error) {
	subDir := filepath.Join(sessionDir, "subagents")

	entries, err := os.ReadDir(subDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			slog.Debug("parser.subagent: missing dir",
				"sessionDir", sessionDir,
				"path", subDir,
			)
			return nil, nil
		}
		slog.Error("parser.subagent: io error",
			"op", "readDir",
			"path", subDir,
			"err", err,
		)
		return nil, err
	}

	// Sort entries by name for deterministic ordering.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var files []subagentFile
	for _, e := range entries {
		name := e.Name()

		// Skip directories and non-regular files (symlinks, devices, etc.).
		if e.IsDir() || !e.Type().IsRegular() {
			slog.Debug("parser.subagent: skip entry",
				"name", name,
				"reason", "name-not-matched",
			)
			continue
		}

		m := agentFileRegexp.FindStringSubmatch(name)
		if m == nil {
			slog.Debug("parser.subagent: skip entry",
				"name", name,
				"reason", "name-not-matched",
			)
			continue
		}
		agentID := m[1]

		info, infoErr := e.Info()
		if infoErr != nil {
			slog.Warn("parser.subagent: meta unavailable",
				"agentID", agentID,
				"path", filepath.Join(subDir, name),
				"err", infoErr,
			)
			continue
		}

		files = append(files, subagentFile{
			agentID:   agentID,
			jsonlPath: filepath.Join(subDir, name),
			metaPath:  filepath.Join(subDir, "agent-"+agentID+".meta.json"),
			modTime:   info.ModTime(),
		})
	}

	return files, nil
}

// aggregateSubagent computes SubagentStats from a sorted, deduplicated []Record
// (already processed by ParseLines).
//
// Token aggregates, TurnCount, and ToolUseCount follow the same logic as
// Aggregate in session.go. Model is taken from the last record's Model field
// (canonical form via canonicalModelKey). LastTool is the name of the last
// tool_use ContentBlock encountered scanning records in forward order
// (overwritten on each tool_use → keeps the final one).
//
// Pure function. AgentID, AgentType, Description, and JSONLPath are filled by
// the caller (CollectSubagents).
func aggregateSubagent(records []Record) SubagentStats {
	if len(records) == 0 {
		return SubagentStats{}
	}

	var s SubagentStats
	s.FirstTimestamp = records[0].Timestamp

	for _, rec := range records {
		s.Tokens.Input += rec.Usage.Input
		s.Tokens.Output += rec.Usage.Output
		s.Tokens.CacheRead += rec.Usage.CacheRead
		s.Tokens.CacheCreate += rec.Usage.CacheCreate
		s.Tokens.CacheCreate5m += rec.Usage.CacheCreate5m
		s.Tokens.CacheCreate1h += rec.Usage.CacheCreate1h
		s.TurnCount++

		for _, c := range rec.Content {
			if c.Type == "tool_use" {
				s.ToolUseCount++
				s.LastTool = c.ToolName // overwrite — keeps last in iteration order
			}
		}
	}

	s.LastTimestamp = records[len(records)-1].Timestamp
	s.Model = canonicalModelKey(records[len(records)-1].Model)
	return s
}

// parseMeta reads metaPath (a JSON file with {agentType, description}).
// Missing file → (subagentMeta{}, nil). Bad JSON or other I/O → (subagentMeta{}, err).
// Unknown extra fields are ignored (json.Decoder default behaviour).
func parseMeta(metaPath string) (subagentMeta, error) {
	f, err := os.Open(metaPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return subagentMeta{}, nil
		}
		return subagentMeta{}, err
	}
	defer f.Close()

	var m subagentMeta
	if err := json.NewDecoder(f).Decode(&m); err != nil {
		return subagentMeta{}, err
	}
	return m, nil
}
