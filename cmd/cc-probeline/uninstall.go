package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"time"

	"github.com/labzink/cc-probeline/internal/settingsfile"
)

// hasFlag returns true when flag is present in os.Args.
func hasFlag(flag string) bool {
	for _, a := range os.Args[1:] {
		if a == flag {
			return true
		}
	}
	return false
}

// runUninstallImpl removes the statusLine block written by cc-probeline from
// ~/.claude/settings.json. It is invoked from main.go via runUninstall().
//
// Exit codes:
//
//	0 – success (including "nothing to do" cases)
//	2 – I/O or parse error
func runUninstallImpl() int {
	dryRun := hasFlag("--dry-run")
	keepBinary := hasFlag("--keep-binary")
	_ = keepBinary // MVP: binary removal not implemented (Phase 5, §4.3)

	path := settingsfile.Path()

	s, err := settingsfile.Load(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Println("cc-probeline: settings.json not found — nothing to uninstall")
			return 0
		}
		fmt.Fprintln(os.Stderr, "cc-probeline: cannot read", path, ":", err)
		return 2
	}

	if !settingsfile.IsOurs(s) {
		fmt.Println("cc-probeline: statusLine in settings.json is not ours — leaving it alone")
		slog.Warn("uninstall skipped — foreign statusLine")
		return 0
	}

	if dryRun {
		fmt.Println("cc-probeline: dry-run — would remove statusLine block")
		return 0
	}

	bak, err := settingsfile.Backup(path, time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, "cc-probeline: backup failed:", err)
		return 2
	}

	if err := settingsfile.Save(path, settingsfile.RemoveStatusLine(s)); err != nil {
		fmt.Fprintln(os.Stderr, "cc-probeline: write failed:", err)
		return 2
	}

	fmt.Println("cc-probeline: uninstalled (backup:", bak+")")
	fmt.Println("To remove the binary: rm", os.Args[0])
	return 0
}
