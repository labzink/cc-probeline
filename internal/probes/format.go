package probes

import "github.com/labzink/cc-probeline/internal/format"

// Unexported aliases keep existing call sites untouched while the real
// implementations live in internal/format (shared with the renderer).
var (
	middleTruncate = format.MiddleTruncate
	formatK        = format.FormatK
	formatMMSS     = format.FormatMMSS
)
