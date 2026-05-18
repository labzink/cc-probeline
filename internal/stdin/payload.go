// Package stdin parses Claude Code statusLine hookData from os.Stdin.
// The JSON schema is owned by Claude Code; unknown fields trigger a slog.Warn
// so that schema drift is visible in logs without breaking the tool.
package stdin

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"
)

// Payload is the typed representation of the JSON object Claude Code sends
// to cc-probeline via stdin on each statusLine hook invocation.
// Parsing is done once in main; all probes read from the resulting struct.
type Payload struct {
	Model          Model         `json:"model"`
	Effort         Effort        `json:"effort"`
	SessionID      string        `json:"session_id"`
	TranscriptPath string        `json:"transcript_path"`
	Cwd            string        `json:"cwd"`
	ContextWindow  ContextWindow `json:"context_window"`
	Cost           Cost          `json:"cost"`
	Tasks          []Task        `json:"tasks,omitempty"`
	StrictMode     bool          `json:"-"` // set from env CC_PROBELINE_STRICT_STDIN, not from JSON
}

// Model describes the active Claude model reported by Claude Code.
type Model struct {
	ID   string `json:"id"`
	Name string `json:"display_name"`
}

// Effort describes the thinking effort level selected by the user.
// Known values: "low", "medium", "high", "xhigh", "max", "off".
type Effort struct {
	Level string `json:"level"`
}

// ContextWindow carries the context window capacity and per-token-type usage.
// CurrentUsage keys: "cache_read_input_tokens", "cache_creation_input_tokens",
// "input_tokens", "output_tokens".
type ContextWindow struct {
	Size         int            `json:"context_window_size"`
	CurrentUsage map[string]int `json:"current_usage"`
}

// Cost holds the accumulated cost and API wall-clock time for this session.
type Cost struct {
	TotalCostUSD       float64 `json:"total_cost_usd"`
	TotalAPIDurationMS int64   `json:"total_api_duration_ms"`
}

// Task represents one active or recently completed subagent task as reported
// by Claude Code in the subagentStatusLine hook.
type Task struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
	StartTime   time.Time `json:"startTime"`
	TokenCount  int       `json:"tokenCount"`
	Cwd         string    `json:"cwd"`
}

// knownTopLevelKeys is the set of JSON keys defined in Payload. It is used in
// non-strict mode to detect and log unknown fields for schema-drift tracking.
var knownTopLevelKeys = map[string]struct{}{
	"model":           {},
	"effort":          {},
	"session_id":      {},
	"transcript_path": {},
	"cwd":             {},
	"context_window":  {},
	"cost":            {},
	"tasks":           {},
}

// Decode reads a single JSON object from r and decodes it into a Payload.
//
// Behaviour:
//   - Empty or malformed input: returns a non-nil error. The error wraps
//     *json.SyntaxError or *json.UnmarshalTypeError, or carries the prefix
//     "stdin.payload: decode failed: " for EOF-style errors.
//   - Unknown top-level fields in non-strict mode: each unknown key triggers a
//     Warn-level slog message ("stdin.payload: unknown field"). Decode still
//     succeeds and returns the populated Payload.
//   - Unknown top-level fields in strict mode: returns
//     fmt.Errorf("stdin.payload: unknown field %q", key) immediately.
func Decode(r io.Reader, strict bool) (Payload, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return Payload{}, fmt.Errorf("stdin.payload: decode failed: %w", err)
	}

	// First pass: decode into a map to check for unknown fields.
	// This also catches empty input and malformed JSON early.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		// Wrap so callers can match on prefix; errors.As still works because
		// the underlying *json.SyntaxError is preserved via %w.
		return Payload{}, fmt.Errorf("stdin.payload: decode failed: %w", err)
	}

	for key := range top {
		if _, known := knownTopLevelKeys[key]; !known {
			if strict {
				return Payload{}, fmt.Errorf("stdin.payload: unknown field %q", key)
			}
			slog.Warn("stdin.payload: unknown field", "field", key)
		}
	}

	// Second pass: decode into the typed Payload (unknown fields silently ignored
	// by default json.Unmarshal — we already handled them above).
	var p Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return Payload{}, err
	}

	return p, nil
}
