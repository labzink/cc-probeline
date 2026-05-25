package main

import (
	"errors"
	"fmt"
	"io"
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

// copyFile copies src to dst by reading the full payload and writing via
// tmp + rename for atomic replacement.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close() //nolint:errcheck
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// runUninstallImpl removes the statusLine block written by cc-probeline from
// ~/.claude/settings.json. When an install-state file records a pre-install
// backup of a foreign statusLine, that backup is restored byte-for-byte
// instead of removing the block.
//
// Exit codes:
//
//	0 – success (including "nothing to do" cases)
//	2 – I/O or parse error
func runUninstallImpl() int {
	setupLogger(os.Getenv("CC_PROBELINE_LOG"), os.Getenv("CC_PROBELINE_DEBUG") == "1")

	dryRun := hasFlag("--dry-run")
	keepBinary := hasFlag("--keep-binary")
	_ = keepBinary // MVP: binary removal not implemented (Phase 5, §4.3)

	path := settingsfile.Path()
	if path == "" {
		fmt.Fprintln(os.Stderr, "cc-probeline: cannot determine home directory")
		return 2
	}

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

	// Try to restore from an install-state backup (foreign statusLine that
	// was replaced during install). Falls back to plain block removal when
	// no state, corrupted state, or missing backup file.
	statePath := settingsfile.StatePath()
	var restoreFrom string
	if statePath != "" {
		st, loadErr := settingsfile.LoadState(statePath)
		if loadErr != nil {
			fmt.Fprintln(os.Stderr, "cc-probeline: warning: install state unreadable, falling back to plain removal:", loadErr)
		} else if st != nil && st.PreInstallBackup != "" {
			if _, statErr := os.Stat(st.PreInstallBackup); statErr == nil {
				restoreFrom = st.PreInstallBackup
			} else {
				fmt.Fprintln(os.Stderr, "cc-probeline: warning: recorded backup missing, falling back to plain removal:", st.PreInstallBackup)
			}
		}
	}

	if dryRun {
		if restoreFrom != "" {
			fmt.Println("cc-probeline: dry-run — would restore previous statusLine from", restoreFrom)
		} else {
			fmt.Println("cc-probeline: dry-run — would remove statusLine block")
		}
		return 0
	}

	if restoreFrom != "" {
		// Restore overwrites settings.json with the byte-for-byte pre-install
		// content. The atomic tmp+rename in copyFile is sufficient — an extra
		// safety backup would only collide with the install-time backup when
		// both run inside the same HHMMSS window.
		if err := copyFile(restoreFrom, path); err != nil {
			fmt.Fprintln(os.Stderr, "cc-probeline: restore failed:", err)
			return 2
		}
		if err := settingsfile.RemoveState(statePath); err != nil {
			fmt.Fprintln(os.Stderr, "cc-probeline: warning: could not remove install state:", err)
		}
		fmt.Println("cc-probeline: restored previous statusLine from", restoreFrom)
		fmt.Println("To remove the binary: rm", os.Args[0])
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
	if statePath != "" {
		_ = settingsfile.RemoveState(statePath)
	}
	fmt.Println("cc-probeline: uninstalled (backup:", bak+")")
	fmt.Println("To remove the binary: rm", os.Args[0])
	return 0
}
