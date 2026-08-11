// Package parser_test — tests for locating subagent transcripts on disk.
//
// Two independent failure modes are covered, both observed live on CC 2.1.227:
//
//  1. Workflow-spawned subagents are written to
//     <session>/subagents/workflows/<workflow-id>/agent-*.jsonl — one level
//     deeper than plain subagents. A flat directory read finds none of them.
//  2. Claude Code names a project directory by replacing EVERY non-alphanumeric
//     character with "-", so a project path containing "_" (or "." or a space)
//     resolves to a directory that ProjectSlug's slash-only rule never predicts.
//
// API under test:
//
//	parser.ProjectSlugCC(cwd string) string
//	parser.SessionDirCandidates(parser.SubagentDirInput) []string
//	parser.CollectSubagentsAcross(ctx, dirs []string, sessionID string) ([]SubagentStats, error)
package parser_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/parser"
)

// writeAgent creates agent-<id>.jsonl (one assistant record carrying sessionID)
// plus its meta.json sibling inside dir.
func writeAgent(t *testing.T, dir, id, sessionID, agentType string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	rec := `{"type":"assistant","uuid":"u-` + id + `","sessionId":"` + sessionID + `",` +
		`"timestamp":"2026-08-11T12:00:00.000Z","agentId":"` + id + `","isSidechain":true,` +
		`"message":{"model":"claude-haiku-4-5-20251001","usage":{"input_tokens":10,"output_tokens":5,` +
		`"cache_read_input_tokens":100,"cache_creation_input_tokens":20},` +
		`"content":[{"type":"text","text":"hi"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "agent-"+id+".jsonl"), []byte(rec), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}
	meta := `{"agentType":"` + agentType + `","model":"haiku"}`
	if err := os.WriteFile(filepath.Join(dir, "agent-"+id+".meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
}

// agentIDs extracts the AgentID of every collected subagent.
func agentIDs(subs []parser.SubagentStats) []string {
	out := make([]string, len(subs))
	for i, s := range subs {
		out[i] = s.AgentID
	}
	return out
}

// TestCollectSubagents_WorkflowNesting is the regression for the reported bug:
// subagents spawned by a dynamic workflow live in subagents/workflows/<id>/ and
// must be collected alongside plain ones sitting directly in subagents/.
func TestCollectSubagents_WorkflowNesting(t *testing.T) {
	sessionDir := t.TempDir()
	subagents := filepath.Join(sessionDir, "subagents")

	writeAgent(t, subagents, "aplain001", "sess-1", "general-purpose")
	wfDir := filepath.Join(subagents, "workflows", "wf_b55c3cd8-f0b")
	writeAgent(t, wfDir, "awf001", "sess-1", "workflow-subagent")
	writeAgent(t, wfDir, "awf002", "sess-1", "workflow-subagent")

	// journal.jsonl sits next to the workflow agents and must never be mistaken
	// for one (it carries no agent- prefix).
	if err := os.WriteFile(filepath.Join(wfDir, "journal.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write journal: %v", err)
	}

	subs, err := parser.CollectSubagents(context.Background(), sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: %v", err)
	}
	if len(subs) != 3 {
		t.Fatalf("want 3 subagents (1 plain + 2 workflow), got %d: %v", len(subs), agentIDs(subs))
	}

	got := strings.Join(agentIDs(subs), ",")
	for _, want := range []string{"aplain001", "awf001", "awf002"} {
		if !strings.Contains(got, want) {
			t.Errorf("subagent %q missing from collected set %v", want, agentIDs(subs))
		}
	}
	for _, s := range subs {
		if s.SessionID != "sess-1" {
			t.Errorf("agent %s: SessionID = %q, want %q (parent session id from its own records)",
				s.AgentID, s.SessionID, "sess-1")
		}
	}
}

// TestCollectSubagentsAcross_MergesAndDedupes verifies the merge contract: two
// directories both contribute, the same directory listed twice contributes once.
func TestCollectSubagentsAcross_MergesAndDedupes(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	writeAgent(t, filepath.Join(dirA, "subagents"), "aaa111", "sess-1", "dev")
	writeAgent(t, filepath.Join(dirB, "subagents"), "bbb222", "sess-1", "dev")

	subs, err := parser.CollectSubagentsAcross(context.Background(),
		[]string{dirA, dirB, dirA}, "sess-1")
	if err != nil {
		t.Fatalf("CollectSubagentsAcross: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("want 2 merged subagents (dirA listed twice must not duplicate), got %d: %v",
			len(subs), agentIDs(subs))
	}
}

// TestCollectSubagentsAcross_SkipsForeignSession guards against pulling in a
// parallel session's subagents: a transcript whose own sessionId differs is
// dropped, while a transcript with no records at all is kept (freshly spawned).
func TestCollectSubagentsAcross_SkipsForeignSession(t *testing.T) {
	dir := t.TempDir()
	subagents := filepath.Join(dir, "subagents")
	writeAgent(t, subagents, "amine01", "sess-mine", "dev")
	writeAgent(t, subagents, "atheirs1", "sess-other", "dev")

	// Empty transcript: nothing to read a session id from — must survive.
	if err := os.WriteFile(filepath.Join(subagents, "agent-aempty1.jsonl"), nil, 0o644); err != nil {
		t.Fatalf("write empty: %v", err)
	}

	subs, err := parser.CollectSubagentsAcross(context.Background(), []string{dir}, "sess-mine")
	if err != nil {
		t.Fatalf("CollectSubagentsAcross: %v", err)
	}

	ids := strings.Join(agentIDs(subs), ",")
	if strings.Contains(ids, "atheirs1") {
		t.Errorf("foreign subagent (sessionId=sess-other) leaked into results: %v", agentIDs(subs))
	}
	if !strings.Contains(ids, "amine01") {
		t.Errorf("own subagent missing from results: %v", agentIDs(subs))
	}
	if !strings.Contains(ids, "aempty1") {
		t.Errorf("id-less (empty) transcript was dropped; a freshly spawned agent must be kept: %v",
			agentIDs(subs))
	}
}

// TestProjectSlugCC_ReplacesEveryNonAlnum locks Claude Code's own rule:
// /[^a-zA-Z0-9]/g -> "-", per character, no collapsing of runs.
func TestProjectSlugCC_ReplacesEveryNonAlnum(t *testing.T) {
	cases := []struct {
		cwd  string
		want string
	}{
		{"/home/me/projects/best_job", "-home-me-projects-best-job"},
		{"/Users/me/Projects/cc-probeline", "-Users-me-Projects-cc-probeline"},
		{"/home/me/my.app", "-home-me-my-app"},
		{"/home/me/two__scores", "-home-me-two--scores"}, // runs are NOT collapsed
		{"/home/me/with space", "-home-me-with-space"},
	}
	for _, c := range cases {
		if got := parser.ProjectSlugCC(c.cwd); got != c.want {
			t.Errorf("ProjectSlugCC(%q) = %q, want %q", c.cwd, got, c.want)
		}
	}

	// The legacy slash-only rule must stay untouched — it is still candidate #2.
	legacy, err := parser.ProjectSlug("/home/me/projects/best_job")
	if err != nil {
		t.Fatalf("ProjectSlug: %v", err)
	}
	if legacy != "-home-me-projects-best_job" {
		t.Errorf("ProjectSlug changed behaviour: got %q, want the underscore preserved", legacy)
	}
}

// TestSessionDirCandidates covers the three resolution strategies: the
// transcript-derived path, the legacy slug and Claude Code's slug rule. Only
// directories that exist are returned, most-trusted first, without duplicates.
func TestSessionDirCandidates(t *testing.T) {
	home := t.TempDir()
	projects := filepath.Join(home, ".claude", "projects")
	const sessionID = "sess-abc"

	t.Run("from_transcript_path", func(t *testing.T) {
		projDir := filepath.Join(projects, "-tmp-proj")
		sessionDir := filepath.Join(projDir, sessionID)
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		got := parser.SessionDirCandidates(parser.SubagentDirInput{
			TranscriptPath: sessionDir + ".jsonl",
			HomeDir:        home,
		})
		if len(got) != 1 || got[0] != sessionDir {
			t.Fatalf("want [%s] from transcript_path alone, got %v", sessionDir, got)
		}
	})

	t.Run("underscore_project_found_via_cc_rule", func(t *testing.T) {
		// Claude Code stored the project as "-tmp-best-job"; the legacy rule
		// would look for "-tmp-best_job", which does not exist.
		ccDir := filepath.Join(projects, "-tmp-best-job", sessionID)
		if err := os.MkdirAll(ccDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		got := parser.SessionDirCandidates(parser.SubagentDirInput{
			CWD:       "/tmp/best_job",
			SessionID: sessionID,
			HomeDir:   home,
		})
		if len(got) != 1 || got[0] != ccDir {
			t.Fatalf("want [%s] (CC slug rule), got %v", ccDir, got)
		}
	})

	t.Run("plain_path_both_rules_agree_no_duplicate", func(t *testing.T) {
		dir := filepath.Join(projects, "-tmp-plain", sessionID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		got := parser.SessionDirCandidates(parser.SubagentDirInput{
			CWD:       "/tmp/plain",
			SessionID: sessionID,
			HomeDir:   home,
		})
		if len(got) != 1 {
			t.Fatalf("legacy and CC rules produce the same path; want 1 candidate, got %v", got)
		}
	})

	t.Run("nothing_exists_returns_empty", func(t *testing.T) {
		got := parser.SessionDirCandidates(parser.SubagentDirInput{
			TranscriptPath: filepath.Join(home, "no", "such", "session.jsonl"),
			CWD:            "/tmp/absent",
			SessionID:      sessionID,
			HomeDir:        home,
		})
		if len(got) != 0 {
			t.Fatalf("want no candidates when nothing exists, got %v", got)
		}
	})

	t.Run("config_dir_env_overrides_home", func(t *testing.T) {
		altBase := t.TempDir()
		dir := filepath.Join(altBase, "projects", "-tmp-alt", sessionID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		got := parser.SessionDirCandidates(parser.SubagentDirInput{
			CWD:          "/tmp/alt",
			SessionID:    sessionID,
			HomeDir:      home,
			ConfigDirEnv: altBase,
		})
		if len(got) != 1 || got[0] != dir {
			t.Fatalf("want [%s] under CLAUDE_CONFIG_DIR, got %v", dir, got)
		}
	})
}
