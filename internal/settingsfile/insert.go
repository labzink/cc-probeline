package settingsfile

import (
	"errors"
	"reflect"
)

// ErrForeignStatusLine is returned by InsertStatusLine when settings.json
// already contains a statusLine block that was not written by cc-probeline
// and Force is false (concept §5.2.4).
var ErrForeignStatusLine = errors.New("settings.json already has a non-cc-probeline statusLine")

// InsertOpts controls the behaviour of InsertStatusLine.
type InsertOpts struct {
	BinaryPath      string // absolute path to the cc-probeline binary; required
	RefreshInterval int    // seconds between renders; default 5 when zero (concept §5.1)
	Padding         int    // padding pixels; default 0
	Force           bool   // overwrite a foreign statusLine block (backup is caller's responsibility)
}

// InsertStatusLine returns a new Settings with our statusLine block applied.
//
// Cases (concept §5.2):
//   - §5.2.1/5.2.2: no statusLine present → insert our block.
//   - §5.2.3: our block present and identical → return s unchanged (idempotency).
//   - §5.2.3: our block present but different command/opts → replace with new block.
//   - §5.2.4: foreign block, !Force → return ErrForeignStatusLine.
//   - §5.2.4: foreign block, Force  → replace with our block.
//
// The input Settings map is never mutated.
func InsertStatusLine(s Settings, opts InsertOpts) (Settings, error) {
	// Apply default refreshInterval (concept §5.1).
	ri := opts.RefreshInterval
	if ri == 0 {
		ri = 5
	}

	// Build the new block.
	newBlock := map[string]any{
		"type":            "command",
		"command":         opts.BinaryPath,
		"padding":         opts.Padding,
		"refreshInterval": ri,
	}

	// Check if a statusLine already exists.
	existing, hasStatusLine := s["statusLine"]

	if hasStatusLine {
		existingBlock, isMap := existing.(map[string]any)
		if !isMap {
			// Unexpected type; treat as foreign.
			if !opts.Force {
				return Settings{}, ErrForeignStatusLine
			}
		} else if IsOurs(s) {
			// Our block: check for idempotency.
			// Build a comparable version using float64 for numeric fields
			// (same as JSON round-trip representation) to allow deep-equal with
			// existing blocks that were loaded from JSON.
			comparableBlock := map[string]any{
				"type":            "command",
				"command":         opts.BinaryPath,
				"padding":         toFloat64(opts.Padding),
				"refreshInterval": toFloat64(ri),
			}
			if reflect.DeepEqual(existingBlock, comparableBlock) {
				// Identical block: return s unchanged (idempotency, concept §5.2.3).
				out := make(Settings, len(s))
				for k, v := range s {
					out[k] = v
				}
				return out, nil
			}
			// Our block but different values: fall through to replace.
		} else {
			// Foreign block.
			if !opts.Force {
				return Settings{}, ErrForeignStatusLine
			}
			// Force=true: fall through to replace.
		}
	}

	// Build output: copy all existing keys and set new statusLine block.
	out := make(Settings, len(s)+1)
	for k, v := range s {
		out[k] = v
	}
	out["statusLine"] = newBlock
	return out, nil
}

// toFloat64 converts an int to float64 for deep-equal comparisons with
// JSON-decoded values (json.Unmarshal stores numbers as float64).
func toFloat64(n int) float64 {
	return float64(n)
}
