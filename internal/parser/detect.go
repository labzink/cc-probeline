package parser

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// ErrPlatformUnsupported was returned by ProjectSlug on Windows before Phase 7.
// Kept as an exported sentinel for callers that still reference it; ProjectSlug
// no longer returns it (Windows now resolves a best-effort slug, see windowsSlug).
var ErrPlatformUnsupported = errors.New("parser.detect: platform not supported (MVP: macOS/Linux only)")

// SessionLocation is the result of DetectActiveSession.
//
// Sentinel "empty" state: Empty == true, JSONLPath == "".
// Slug is always populated when the call succeeds (even if Empty).
type SessionLocation struct {
	// Slug is the canonical slug of the project directory (see ProjectSlug).
	Slug string
	// Dir is the absolute path to the slug directory (populated even when it
	// does not exist on disk).
	Dir string
	// JSONLPath is the absolute path to the active JSONL transcript.
	// Empty string when Empty == true.
	JSONLPath string
	// Empty is true when the slug directory does not exist or contains no
	// *.jsonl files. Callers should render token/cost segments from stdin
	// hookData only, hiding session-dependent segments.
	Empty bool
}

// DetectInput carries the parameters for DetectActiveSession.
//
// All string fields that represent paths must be absolute POSIX paths on Unix.
// Windows is not supported in the MVP (ProjectSlug returns ErrPlatformUnsupported).
type DetectInput struct {
	// CWD is the working directory of the Claude Code process. Required.
	CWD string
	// SessionID is the session_id from stdin hookData. Optional.
	// When set, the detector first tries <dir>/<SessionID>.jsonl before
	// falling back to mtime.
	SessionID string
	// TranscriptPath is the transcript_path from stdin hookData. Optional.
	// When set and the path is inside the computed slug directory, it is
	// returned directly without consulting SessionID or mtime.
	TranscriptPath string
	// HomeDir is the user's home directory. Required.
	// Typically os.UserHomeDir(). Extracted for testability.
	HomeDir string
	// ConfigDirEnv is the value of CLAUDE_CONFIG_DIR. Optional ("" if unset).
	// When non-empty it fully replaces HomeDir+"/.claude" as the base path.
	ConfigDirEnv string
}

// ProjectSlug converts an absolute CWD path to the slug Claude Code uses as
// the directory name under ~/.claude/projects/ (%USERPROFILE%\.claude\projects\
// on Windows).
//
// Formula (Unix): replace every "/" in filepath.Clean(cwd) with "-".
// Example: "/Users/me/Projects/foo" -> "-Users-me-Projects-foo".
//
// Formula (Windows): replace the drive colon and both path separators with "-"
// (see windowsSlug). Example: `C:\Users\me\foo` -> "C--Users-me-foo".
//
// Returns a non-nil error if a Unix cwd is not an absolute path.
func ProjectSlug(cwd string) (string, error) {
	if runtime.GOOS == "windows" {
		return windowsSlug(cwd), nil
	}
	clean := filepath.Clean(cwd)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("parser.detect: cwd must be absolute: %q", cwd)
	}
	return strings.ReplaceAll(clean, "/", "-"), nil
}

// windowsSlug canonicalizes a Windows working directory to the slug Claude Code
// uses as the directory name under %USERPROFILE%\.claude\projects\.
//
// It replaces the drive-letter colon and both path separators ("\" and "/") with
// "-": `C:\Users\me\foo` -> "C--Users-me-foo".
//
// BL-9 (partial): this formula is a best-effort match for Claude Code's Windows
// canonicalization and is NOT yet verified against a real folder created on a
// Windows install (no Windows machine was available for hands-on). It is a pure,
// GOOS-independent string transform so it can be unit-tested cross-platform.
//
// Fail-soft contract: if the resolved slug does not match the real folder, the
// caller (DetectActiveSession / main.go) finds no JSONL and renders without
// session segments — it never crashes. When verified, drop the "partial" note.
func windowsSlug(cwd string) string {
	return strings.NewReplacer(`\`, "-", "/", "-", ":", "-").Replace(cwd)
}

// DetectActiveSession locates the JSONL transcript for the currently active
// Claude Code session.
//
// Resolution order (highest priority first):
//  1. TranscriptPath — if set and inside the slug directory, returned directly.
//  2. SessionID — if set, tries <dir>/<SessionID>.jsonl; falls through on miss.
//  3. mtime fallback — newest *.jsonl in the slug directory.
//
// Returns (SessionLocation{Empty:true}, nil) when the slug directory does not
// exist or contains no *.jsonl files (fail-soft per specs.md §A2).
// Returns (SessionLocation{}, error) only for unexpected I/O errors.
func DetectActiveSession(in DetectInput) (SessionLocation, error) {
	slog.Debug("parser.detect: input",
		"cwd", in.CWD,
		"sessionID", in.SessionID,
		"transcriptPath", in.TranscriptPath,
		"configDirEnv", in.ConfigDirEnv,
		"homeDir", in.HomeDir,
	)

	slug, err := ProjectSlug(in.CWD)
	if err != nil {
		return SessionLocation{}, err
	}

	base := resolveBase(in)
	dir := filepath.Join(base, "projects", slug)
	loc := SessionLocation{Slug: slug, Dir: dir}

	// --- Step 4: TranscriptPath shortcut ---
	if in.TranscriptPath != "" {
		if validateTranscriptPath(in.TranscriptPath, dir) {
			loc.JSONLPath = in.TranscriptPath
			loc.Empty = false
			slog.Info("parser.detect: resolved",
				"slug", loc.Slug,
				"dir", loc.Dir,
				"jsonl", loc.JSONLPath,
				"empty", loc.Empty,
				"source", "transcriptPath",
			)
			return loc, nil
		}
		slog.Warn("parser.detect: transcript_path outside slug dir",
			"path", in.TranscriptPath,
			"dir", dir,
		)
		// Fall through to sessionID / mtime.
	}

	// --- Step 5: Stat the slug directory ---
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			loc.Empty = true
			slog.Info("parser.detect: resolved",
				"slug", loc.Slug,
				"dir", loc.Dir,
				"jsonl", loc.JSONLPath,
				"empty", loc.Empty,
				"source", "empty",
			)
			return loc, nil
		}
		slog.Error("parser.detect: io error",
			"op", "stat",
			"path", dir,
			"err", err,
		)
		return SessionLocation{}, fmt.Errorf("parser.detect: stat dir: %w", err)
	}
	if !info.IsDir() {
		return SessionLocation{}, fmt.Errorf("parser.detect: not a directory: %s", dir)
	}

	// --- Step 6: SessionID lookup ---
	if in.SessionID != "" {
		candidate := filepath.Join(dir, in.SessionID+".jsonl")
		if _, err := os.Stat(candidate); err == nil {
			loc.JSONLPath = candidate
			slog.Info("parser.detect: resolved",
				"slug", loc.Slug,
				"dir", loc.Dir,
				"jsonl", loc.JSONLPath,
				"empty", loc.Empty,
				"source", "sessionID",
			)
			return loc, nil
		}
		// File missing: log Warn and fall through to mtime.
		slog.Warn("parser.detect: sessionID jsonl not found",
			"sessionID", in.SessionID,
			"candidate", candidate,
			"fallback", "mtime",
		)
	}

	// --- Step 7: mtime fallback ---
	entries, err := os.ReadDir(dir)
	if err != nil {
		slog.Error("parser.detect: io error",
			"op", "readDir",
			"path", dir,
			"err", err,
		)
		return SessionLocation{}, fmt.Errorf("parser.detect: read dir: %w", err)
	}

	name, found := pickByMtime(entries, dir)
	if !found {
		loc.Empty = true
		slog.Info("parser.detect: resolved",
			"slug", loc.Slug,
			"dir", loc.Dir,
			"jsonl", loc.JSONLPath,
			"empty", loc.Empty,
			"source", "empty",
		)
		return loc, nil
	}

	loc.JSONLPath = filepath.Join(dir, name)
	slog.Info("parser.detect: resolved",
		"slug", loc.Slug,
		"dir", loc.Dir,
		"jsonl", loc.JSONLPath,
		"empty", loc.Empty,
		"source", "mtime",
	)
	return loc, nil
}

// resolveBase returns the base Claude config directory.
// When ConfigDirEnv is non-empty it is used directly; otherwise HomeDir+"/.claude".
func resolveBase(in DetectInput) string {
	if in.ConfigDirEnv != "" {
		return in.ConfigDirEnv
	}
	return filepath.Join(in.HomeDir, ".claude")
}

// validateTranscriptPath reports whether path resides inside dir.
// Uses filepath.Rel + filepath.IsLocal (Go 1.20) to detect path-traversal
// attempts including ".."-escape while correctly allowing filenames that
// start with ".." (e.g. "..hidden.jsonl").
// Also verifies that the file exists on disk via os.Stat.
func validateTranscriptPath(path, dir string) bool {
	rel, err := filepath.Rel(dir, filepath.Clean(path))
	if err != nil || !filepath.IsLocal(rel) {
		return false
	}
	_, statErr := os.Stat(path)
	return statErr == nil
}

// pickByMtime scans entries for the newest *.jsonl regular file.
// When two files share the same mtime, the alphabetically-first name wins
// (entries are sorted before the scan for determinism).
// Returns (filename, true) on success, ("", false) when no qualifying file
// is found.
func pickByMtime(entries []fs.DirEntry, dir string) (string, bool) {
	var newestName string
	var newestMtime time.Time

	// Sort alphabetically first so that ties resolve to the lexicographically
	// smallest name rather than depending on OS readdir ordering.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, e := range entries {
		// Skip directories and non-.jsonl files.
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		if !e.Type().IsRegular() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			// Transient error (e.g. concurrent deletion); skip this entry.
			continue
		}
		if newestName == "" || fi.ModTime().After(newestMtime) {
			newestName = e.Name()
			newestMtime = fi.ModTime()
		}
	}

	if newestName == "" {
		return "", false
	}
	return newestName, true
}
