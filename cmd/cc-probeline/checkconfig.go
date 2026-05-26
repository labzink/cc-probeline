// checkconfig.go implements `cc-probeline check-config` subcommand.
// Phase 6.e GREEN will replace this stub with the full implementation.
package main

import (
	"io"
	"os"
)

// runCheckConfig is the entry point for `cc-probeline check-config [--verbose|-v] [--json]`.
// Returns exit code: 0 if no SeverityError (warnings allowed), 2 otherwise.
//
// Phase 6.e stub: always returns 0 (RED tests expect non-zero for broken configs).
// GREEN implementation: parse flags, call config.LoadCascade, format output, return 2 on error.
func runCheckConfig(args []string) int {
	return runCheckConfigImpl(args, os.Stdout, os.Stderr)
}

// runCheckConfigImpl is the testable core of runCheckConfig, writing to supplied
// writers instead of os.Stdout/os.Stderr.
// Stub returns 0; GREEN will implement full logic.
func runCheckConfigImpl(_ []string, _ io.Writer, _ io.Writer) int {
	return 0
}
