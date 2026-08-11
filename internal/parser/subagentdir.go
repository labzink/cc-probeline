package parser

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// SubagentDirInput carries everything needed to locate the session directory
// that holds a session's subagent transcripts. All fields are optional: each
// resolution strategy is skipped when its inputs are missing, and an empty
// result means "no subagents to show" (fail-soft).
type SubagentDirInput struct {
	// TranscriptPath is stdin's transcript_path — the absolute path of the
	// session JSONL as Claude Code itself reports it. Most reliable input.
	TranscriptPath string
	// CWD is stdin's cwd, used by the slug-based fallbacks.
	CWD string
	// SessionID is stdin's session_id, used by the slug-based fallbacks.
	SessionID string
	// HomeDir is the user's home directory (os.UserHomeDir()).
	HomeDir string
	// ConfigDirEnv is CLAUDE_CONFIG_DIR; when non-empty it replaces
	// HomeDir + "/.claude" as the base path.
	ConfigDirEnv string
}

// SessionDirCandidates returns every existing directory that may hold this
// session's subagent transcripts, most-trusted first and de-duplicated.
//
// Three independent strategies are tried, because each fails in a different
// situation and the cheap fix is to try them all rather than pick one:
//
//  1. Derived from TranscriptPath by dropping the ".jsonl" suffix. Claude Code
//     stores a session's subagents in a directory named exactly like its
//     transcript, so this needs no guessing at all — it survives paths with
//     underscores, dots or non-Latin characters, symlinked working directories,
//     and over-long paths that Claude Code truncates and hashes.
//  2. The historical formula: <base>/projects/<ProjectSlug(cwd)>/<session-id>.
//     Kept verbatim so behaviour that works today keeps working.
//  3. The same formula with Claude Code's real slug rule (ProjectSlugCC), which
//     replaces every non-alphanumeric character. This is what finds sessions for
//     projects whose path contains "_" or "." — where strategy 2 looks in a
//     directory that does not exist.
//
// Strategies 2 and 3 produce an identical path for plain latin/dash paths, so
// the de-duplication below collapses them and nothing changes for such projects.
func SessionDirCandidates(in SubagentDirInput) []string {
	var out []string
	seen := make(map[string]struct{}, 3)

	add := func(dir, origin string) {
		if dir == "" {
			return
		}
		clean := filepath.Clean(dir)
		if _, dup := seen[clean]; dup {
			return
		}
		info, err := os.Stat(clean)
		if err != nil || !info.IsDir() {
			slog.Debug("parser.subagentdir: candidate absent", "origin", origin, "path", clean)
			return
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
		slog.Debug("parser.subagentdir: candidate accepted", "origin", origin, "path", clean)
	}

	// 1. From the transcript path Claude Code reported.
	if tp := in.TranscriptPath; strings.HasSuffix(tp, ".jsonl") {
		add(strings.TrimSuffix(tp, ".jsonl"), "transcript")
	}

	// 2/3. Slug-based reconstruction. Both need cwd + session id + a base dir.
	if in.CWD != "" && in.SessionID != "" {
		projects := projectsRoot(in.HomeDir, in.ConfigDirEnv)
		if projects != "" {
			if legacy, err := ProjectSlug(in.CWD); err == nil {
				add(filepath.Join(projects, legacy, in.SessionID), "slug-legacy")
			}
			if dir := resolveProjectDir(projects, ProjectSlugCC(in.CWD)); dir != "" {
				add(filepath.Join(dir, in.SessionID), "slug-cc")
			}
		}
	}

	return out
}

// projectsRoot resolves <base>/projects, where base is CLAUDE_CONFIG_DIR when
// set and ~/.claude otherwise. Returns "" when neither is known.
func projectsRoot(homeDir, configDirEnv string) string {
	base := configDirEnv
	if base == "" {
		if homeDir == "" {
			return ""
		}
		base = filepath.Join(homeDir, ".claude")
	}
	return filepath.Join(base, "projects")
}

// resolveProjectDir maps a Claude Code slug to the project directory on disk.
//
// The direct path is returned when it exists. Otherwise, for a slug long enough
// that Claude Code truncates it, the real directory is "<prefix>-<hash>"; since
// the hash is not reproduced here, the projects root is scanned for a single
// directory carrying that prefix. Ambiguous or absent → "".
func resolveProjectDir(projectsRoot, slug string) string {
	if slug == "" {
		return ""
	}
	direct := filepath.Join(projectsRoot, slug)
	if info, err := os.Stat(direct); err == nil && info.IsDir() {
		return direct
	}

	prefix, truncated := ProjectSlugCCTruncated(slug)
	if !truncated {
		return ""
	}
	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		return ""
	}
	var match string
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), prefix+"-") {
			continue
		}
		if match != "" {
			// More than one truncated candidate: refuse to guess.
			slog.Debug("parser.subagentdir: ambiguous truncated slug", "prefix", prefix)
			return ""
		}
		match = filepath.Join(projectsRoot, e.Name())
	}
	return match
}
