package parser

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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

	// TaskID is the identifier passed by Claude Code in subagentStatusLine
	// stdin (tasks[].id). For MVP this is left empty by CollectSubagents and
	// populated by Phase 4 probe code under the hypothesis task.id == AgentID
	// (specs.md §A3, CONCEPT §8 Q1). If hands-on verification in Phase 4
	// reveals a different mapping, this field absorbs the difference without
	// breaking the public API.
	TaskID string

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

	// TranscriptPath is the absolute path to agent-<id>.jsonl. Used by Phase 5
	// as a cache key and for mtime-based invalidation.
	TranscriptPath string

	// JSONLModTime is the modification time of the agent's JSONL file.
	// Used as a tie-break for sort when LastTimestamp is zero (e.g. a
	// freshly-spawned subagent whose transcript has no records yet).
	// Phase 5 cache layer also uses this for mtime-based invalidation
	// (see specs.md §B2).
	JSONLModTime time.Time

	// Turns holds one Turn per record, in record order. Each Turn has
	// Role=AgentType (fallback "agent"), IsSidechain=true, and GroupID=0
	// (assigned during merge in Phase 6.8.d). Added in Phase 6.8.0 for
	// interleaved table rendering.
	Turns []Turn

	// ActivationStart is the timestamp of the first assistant turn in the
	// current activation. An activation begins at agent launch or at the
	// most recent SendMessage boundary (user-text record). Used as the
	// sort anchor for subagent panel rows (spec-common §2.3).
	// Fallback (Insurance #1): when no user-text boundary exists, equals
	// FirstTimestamp (the very first assistant turn).
	ActivationStart time.Time

	// CurrentTurnNum is the number of assistant turns in the current
	// activation (1-based count). Used to render "↳N" in the # column.
	CurrentTurnNum int
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
// If sessionDir is empty or does not exist, returns ([], nil) — same as
// a missing subagents/ subdirectory.
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
//     TranscriptPath populated; all aggregates zero. Returned, not skipped.
//
// Ordering of the returned slice:
//  1. Primary: LastTimestamp DESC (most-recently-active first).
//  2. When two records share LastTimestamp (non-zero): AgentID ASC.
//  3. When LastTimestamp is zero on both (e.g. empty transcripts):
//     JSONLModTime DESC (most-recently-touched on disk first).
//
// Returns a non-nil error only when listing subagents/ itself fails for an
// unexpected reason (ENOENT is the fail-soft case above).
func CollectSubagents(ctx context.Context, sessionDir string) ([]SubagentStats, error) {
	if sessionDir == "" {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	slog.Debug("parser.subagent: collect", "sessionDir", sessionDir)

	files, err := listSubagentFiles(sessionDir)
	if err != nil {
		return nil, err
	}
	slog.Info("parser.subagent: enumerated", "count", len(files))

	results := make([]SubagentStats, 0, len(files))

	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		stats, err := collectOne(f)
		if err != nil {
			// Per-file error: log and skip. Never propagate to caller.
			continue
		}

		results = append(results, stats)
	}

	// Sort: primary LastTimestamp DESC; tie-break when both non-zero: AgentID ASC;
	// tie-break when both zero (empty transcripts): JSONLModTime DESC.
	sort.SliceStable(results, func(i, j int) bool {
		ti := results[i].LastTimestamp
		tj := results[j].LastTimestamp
		if ti.IsZero() && tj.IsZero() {
			// Both empty transcripts: most-recently-touched JSONL file first.
			return results[i].JSONLModTime.After(results[j].JSONLModTime)
		}
		if ti.Equal(tj) {
			return results[i].AgentID < results[j].AgentID
		}
		return ti.After(tj)
	})

	return results, nil
}

// collectOne opens, parses, and aggregates one subagent JSONL file.
// Returns the populated SubagentStats or an error (caller logs and skips).
func collectOne(f subagentFile) (SubagentStats, error) {
	meta, metaErr := parseMeta(f.metaPath)
	if metaErr != nil {
		slog.Warn("parser.subagent: meta unavailable",
			"agentID", f.agentID,
			"path", f.metaPath,
			"err", metaErr,
		)
		// Continue with zero meta — SubagentStats will have empty AgentType/Description.
	}

	// Scan for the last user-text boundary before opening for ParseLines.
	// ParseLines drops all non-assistant records, so boundaries must be found
	// in a separate raw pass. Error is non-fatal: fallback to zero boundary
	// (Insurance #1: ActivationStart = first turn).
	lastBoundary, boundaryErr := scanLastUserBoundary(f.jsonlPath)
	if boundaryErr != nil {
		slog.Debug("parser.subagent: boundary scan error",
			"agentID", f.agentID,
			"err", boundaryErr,
		)
	}

	fh, openErr := os.Open(f.jsonlPath)
	if openErr != nil {
		slog.Error("parser.subagent: open failed",
			"agentID", f.agentID,
			"path", f.jsonlPath,
			"err", openErr,
		)
		return SubagentStats{}, openErr
	}
	defer fh.Close()

	records, parseErrs, scanErr := ParseLines(fh)

	if len(parseErrs) > 0 {
		slog.Debug("parser.subagent: parse errors",
			"agentID", f.agentID,
			"count", len(parseErrs),
		)
	}
	if scanErr != nil {
		slog.Error("parser.subagent: scan failed",
			"agentID", f.agentID,
			"path", f.jsonlPath,
			"err", scanErr,
		)
		return SubagentStats{}, scanErr
	}

	if len(records) == 0 {
		if len(parseErrs) > 0 {
			slog.Warn("parser.subagent: all lines unparsable",
				"agentID", f.agentID,
				"path", f.jsonlPath,
				"parseErrs", len(parseErrs),
			)
		} else {
			slog.Info("parser.subagent: empty transcript", "agentID", f.agentID)
		}
	}

	stats := aggregateSubagent(records, lastBoundary)
	stats.AgentID = f.agentID
	stats.AgentType = meta.AgentType
	stats.Description = meta.Description
	stats.TranscriptPath = f.jsonlPath
	stats.JSONLModTime = f.modTime

	// Propagate AgentType into each Turn.Role (fallback "agent" when empty).
	role := meta.AgentType
	if role == "" {
		role = "agent"
	}
	for i := range stats.Turns {
		stats.Turns[i].Role = role
	}

	slog.Debug("parser.subagent: aggregated",
		"agentID", f.agentID,
		"turns", stats.TurnCount,
		"activationStart", stats.ActivationStart,
		"currentTurnNum", stats.CurrentTurnNum,
	)

	return stats, nil
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

		// Special-case orphan meta files: log Warn if companion .jsonl is missing.
		if strings.HasPrefix(name, "agent-") && strings.HasSuffix(name, ".meta.json") {
			base := strings.TrimSuffix(name, ".meta.json")
			jsonlPath := filepath.Join(subDir, base+".jsonl")
			if _, statErr := os.Stat(jsonlPath); errors.Is(statErr, fs.ErrNotExist) {
				slog.Warn("parser.subagent: orphan meta",
					"path", filepath.Join(subDir, name),
				)
			}
			continue // meta files are never primary entries
		}

		// Skip directories and non-regular files (symlinks, devices, etc.).
		if e.IsDir() || !e.Type().IsRegular() {
			slog.Debug("parser.subagent: skip non-regular",
				"name", name,
			)
			continue
		}

		m := agentFileRegexp.FindStringSubmatch(name)
		if m == nil {
			slog.Debug("parser.subagent: skip unmatched",
				"name", name,
			)
			continue
		}
		agentID := m[1]

		info, infoErr := e.Info()
		if infoErr != nil {
			slog.Warn("parser.subagent: stat failed",
				"agentID", agentID,
				"path", filepath.Join(subDir, name),
				"op", "Info",
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
// Phase 6.8.0: Turns is populated with one Turn per record. Each Turn has
// Role="agent" (IsSidechain=true) and GroupID=0; GroupID is assigned during
// the merge step in Phase 6.8.d based on timestamp.
//
// Phase 6.9.d: lastBoundary is the timestamp of the last user-text record in the
// agent transcript (scanned separately from ParseLines which drops non-assistant
// records). When non-zero, ActivationStart and CurrentTurnNum cover only the turns
// after that boundary. When zero (Insurance #1 fallback): ActivationStart = first
// turn timestamp, CurrentTurnNum = total turns.
//
// Pure function. AgentID, TaskID, AgentType, Description, TranscriptPath, and
// JSONLModTime are filled by the caller (CollectSubagents / collectOne).
func aggregateSubagent(records []Record, lastBoundary time.Time) SubagentStats {
	if len(records) == 0 {
		return SubagentStats{}
	}

	var s SubagentStats
	s.FirstTimestamp = records[0].Timestamp
	s.Turns = make([]Turn, 0, len(records))

	var prevTimestamp time.Time
	turnIndex := 0 // 1-based index among assistant records only
	for _, rec := range records {
		// User-text records are boundary markers; they do not produce a Turn.
		// (Same contract as Aggregate in session.go.)
		if rec.Type != "assistant" {
			continue
		}

		s.Tokens.Input += rec.Usage.Input
		s.Tokens.Output += rec.Usage.Output
		s.Tokens.CacheRead += rec.Usage.CacheRead
		s.Tokens.CacheCreate += rec.Usage.CacheCreate
		s.Tokens.CacheCreate5m += rec.Usage.CacheCreate5m
		s.Tokens.CacheCreate1h += rec.Usage.CacheCreate1h
		s.TurnCount++

		hasThinking := false
		hasToolUse := false
		toolUse := ""
		for _, c := range rec.Content {
			switch c.Type {
			case "thinking":
				hasThinking = true
			case "tool_use":
				s.ToolUseCount++
				hasToolUse = true
				if toolUse == "" {
					toolUse = c.ToolName
				}
				s.LastTool = c.ToolName // overwrite — keeps last in iteration order
			}
		}

		turnIndex++
		var dur time.Duration
		if turnIndex > 1 {
			dur = rec.Timestamp.Sub(prevTimestamp)
		}
		// Sidechain turns: Role="agent" (caller overwrites with AgentType), IsSidechain=true, GroupID=0.
		s.Turns = append(s.Turns, Turn{
			Index:       turnIndex,
			Role:        "agent",
			Model:       CanonicalModelKey(rec.Model),
			Tokens:      rec.Usage,
			ToolUse:     toolUse,
			Timestamp:   rec.Timestamp,
			Duration:    dur,
			IsSidechain: true,
			UUID:        rec.UUID,
			GroupID:     0,
			Thinking:    hasThinking && !hasToolUse,
		})
		prevTimestamp = rec.Timestamp
	}

	// Use the last Turn (assistant record) for LastTimestamp and Model.
	// records may now contain user boundary records at the end, so we cannot
	// use records[len-1] directly.
	if len(s.Turns) > 0 {
		last := s.Turns[len(s.Turns)-1]
		s.LastTimestamp = last.Timestamp
		s.Model = last.Model // already canonical from CanonicalModelKey above
	}

	// Compute ActivationStart and CurrentTurnNum.
	// When lastBoundary is non-zero, the current activation starts at the first
	// turn whose Timestamp is strictly after the boundary. Fallback (Insurance #1):
	// no boundary found → activation covers the entire transcript.
	// Default to sentinel meaning "no post-boundary turn found yet".
	activationIdx := len(s.Turns)
	if !lastBoundary.IsZero() {
		for i, t := range s.Turns {
			if t.Timestamp.After(lastBoundary) {
				activationIdx = i
				break
			}
		}
	}
	// If no turn is strictly after the boundary (or no boundary at all),
	// fall back to the first turn so the entire transcript is the activation.
	if activationIdx >= len(s.Turns) {
		activationIdx = 0
	}
	s.ActivationStart = s.Turns[activationIdx].Timestamp
	s.CurrentTurnNum = len(s.Turns) - activationIdx

	return s
}

// scanLastUserBoundary performs a raw line-scan of a JSONL file and returns the
// timestamp of the last user-text record (i.e. a record where type=="user" and
// content contains no tool_result blocks). This is the SendMessage boundary.
//
// ParseLines drops all non-assistant records, so this separate pass is required
// to detect activation boundaries (spec-common §2.3, Insurance #1).
//
// Returns zero time when no user-text record is found or on I/O error.
func scanLastUserBoundary(path string) (time.Time, error) {
	fh, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}
	defer fh.Close()

	// Minimal struct for boundary detection — only the fields we need.
	type boundaryContent struct {
		Type string `json:"type"`
	}
	type boundaryMessage struct {
		Content []boundaryContent `json:"content"`
	}
	type boundaryLine struct {
		Type      string          `json:"type"`
		Timestamp string          `json:"timestamp"`
		Message   boundaryMessage `json:"message"`
	}

	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var last time.Time

	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var bl boundaryLine
		if err := json.Unmarshal(line, &bl); err != nil {
			continue
		}
		if bl.Type != "user" {
			continue
		}
		// Check for tool_result blocks — those are NOT user-text boundaries.
		isToolResult := false
		for _, c := range bl.Message.Content {
			if c.Type == "tool_result" {
				isToolResult = true
				break
			}
		}
		if isToolResult {
			continue
		}
		// Valid user-text boundary: parse timestamp.
		if bl.Timestamp == "" {
			continue
		}
		ts, parseErr := time.Parse(time.RFC3339Nano, bl.Timestamp)
		if parseErr != nil {
			continue
		}
		last = ts.UTC()
	}

	if err := sc.Err(); err != nil {
		return time.Time{}, err
	}
	return last, nil
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
