// Package claudejson reads narrowly-scoped fields from ~/.claude.json.
//
// Security note: ~/.claude.json contains OAuth tokens and other sensitive data.
// This package decodes ONLY the fields it needs — the wire structs carry nothing
// else, so nothing else ever reaches memory. File contents are never logged; on
// error only the fact is logged. See usage.go for the usage-cache reader.
//
// Path resolution (in priority order):
//  1. CC_PROBELINE_CLAUDE_JSON env var — full path to the file (used by tests).
//  2. $HOME/.claude.json (production default).
//
// The result is cached by the file's mtime: a second call that finds the same
// mtime returns the cached value without re-reading the file.
package claudejson

import "os"

// claudeJSONPath returns the path to ~/.claude.json, honouring the test
// override env var CC_PROBELINE_CLAUDE_JSON.
func claudeJSONPath() string {
	if p := os.Getenv("CC_PROBELINE_CLAUDE_JSON"); p != "" {
		return p
	}
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	return home + "/.claude.json"
}
