package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"time"

	"github.com/labzink/cc-probeline/internal/settingsfile"
)

// flagValue returns the value for the named flag in os.Args, or "" if not found.
// Supports both "--flag value" and "--flag=value" forms.
func flagValue(flag string) string {
	for i, a := range os.Args[1:] {
		if a == flag && i+2 < len(os.Args) {
			return os.Args[i+2]
		}
		prefix := flag + "="
		if len(a) > len(prefix) && a[:len(prefix)] == prefix {
			return a[len(prefix):]
		}
	}
	return ""
}

// flagIntOr returns the integer value for the named flag, or defaultVal if
// not present or not parseable.
func flagIntOr(flag string, defaultVal int) int {
	v := flagValue(flag)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

// runInstallImpl implements the "install" subcommand. It is invoked from
// main.go via runInstall().
//
// Exit codes:
//
//	0  – success
//	2  – I/O, parse, or merge error
func runInstallImpl() int {
	setupLogger(os.Getenv("CC_PROBELINE_LOG"), os.Getenv("CC_PROBELINE_DEBUG") == "1")

	if !hasFlag("--merge-settings") {
		fmt.Println("cc-probeline install: only --merge-settings supported in Phase 5")
		fmt.Println("(use scripts/install.sh or scripts/install.ps1 for the binary)")
		fmt.Println("Full install (Phase 7) will wire the binary automatically.")
		return 2
	}

	bin := flagValue("--binary-path")
	if bin == "" {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cc-probeline: cannot determine executable path:", err)
			return 2
		}
		abs, err := filepath.Abs(exe)
		if err != nil {
			fmt.Fprintln(os.Stderr, "cc-probeline: cannot resolve executable path:", err)
			return 2
		}
		bin = abs
	}

	opts := settingsfile.InsertOpts{
		BinaryPath:      bin,
		RefreshInterval: flagIntOr("--refresh-interval", 5),
		Padding:         0,
		Force:           hasFlag("--force"),
	}

	path := settingsfile.Path()
	if path == "" {
		fmt.Fprintln(os.Stderr, "cc-probeline: cannot determine home directory")
		return 2
	}

	s, err := settingsfile.Load(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintln(os.Stderr, "cc-probeline: cannot read", path, ":", err)
		return 2
	}
	if s == nil {
		s = settingsfile.Settings{}
	}

	// InsertStatusLine first: refuse foreign statusLine before creating a backup.
	next, err := settingsfile.InsertStatusLine(s, opts)
	if errors.Is(err, settingsfile.ErrForeignStatusLine) {
		fmt.Fprintln(os.Stderr, "cc-probeline: settings.json already has a non-cc-probeline statusLine.")
		fmt.Fprintln(os.Stderr, "Re-run with --force to overwrite (backup is saved automatically).")
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cc-probeline: merge failed:", err)
		return 2
	}

	// Idempotent: skip backup + write when merge produces no change.
	// Prevents BackupPath collisions on rapid re-installs (HHMMSS resolution).
	if reflect.DeepEqual(s, next) {
		fmt.Println("cc-probeline: statusLine already wired in", path)
		return 0
	}

	// Backup only when we are about to write (file exists and merge succeeded).
	if _, statErr := os.Stat(path); statErr == nil {
		if _, bakErr := settingsfile.Backup(path, time.Now()); bakErr != nil {
			fmt.Fprintln(os.Stderr, "cc-probeline: backup failed:", bakErr)
			return 2
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		fmt.Fprintln(os.Stderr, "cc-probeline: cannot create settings directory:", err)
		return 2
	}

	if err := settingsfile.Save(path, next); err != nil {
		fmt.Fprintln(os.Stderr, "cc-probeline: write failed:", err)
		return 2
	}

	fmt.Println("cc-probeline: statusLine wired in", path)
	return 0
}
