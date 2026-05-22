package settingsfile

import "errors"

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
//   - §5.2.4: foreign block, !Force → return ErrForeignStatusLine unchanged.
//   - §5.2.4: foreign block, Force  → replace with our block.
//
// The input Settings map is never mutated.
//
// Stub: real logic lands in GREEN (5.e).
func InsertStatusLine(s Settings, opts InsertOpts) (Settings, error) {
	return Settings{}, errors.New("stub: 5.e GREEN")
}
