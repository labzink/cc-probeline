package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

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

// hasForeignStatusLine reports whether settings has a non-cc-probeline
// statusLine block (the case we need to confirm overwriting).
func hasForeignStatusLine(s settingsfile.Settings) bool {
	if s == nil {
		return false
	}
	if _, ok := s["statusLine"]; !ok {
		return false
	}
	return !settingsfile.IsOurs(s)
}

// promptForeignOverwrite asks the user whether to overwrite a foreign
// statusLine. Returns true when the user confirms (default Yes on Enter).
// Prints the prompt to stderr so it does not contaminate stdout when the
// caller is piping output.
func promptForeignOverwrite(existing string) bool {
	fmt.Fprintln(os.Stderr, "cc-probeline: another statusLine is already configured:")
	fmt.Fprintln(os.Stderr, "  "+existing)
	fmt.Fprintln(os.Stderr, "We will back it up and restore it automatically when you run `cc-probeline uninstall`.")
	fmt.Fprint(os.Stderr, "Replace it with cc-probeline? [Y/n] ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "" || answer == "y" || answer == "yes"
}

// existingCommand returns the statusLine.command currently configured in s,
// or "" when none is set.
func existingCommand(s settingsfile.Settings) string {
	sl, ok := s["statusLine"].(map[string]any)
	if !ok {
		return ""
	}
	cmd, _ := sl["command"].(string)
	return cmd
}

// canPrompt reports whether stdin and stderr are both attached to a TTY
// (required for interactive [Y/n] prompts).
func canPrompt() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}

// runInstallImpl implements the "install" subcommand. It is invoked from
// main.go via runInstall().
//
// Exit codes:
//
//	0  – success (including idempotent re-install and user cancellation)
//	2  – I/O, parse, or merge error; or foreign statusLine in non-interactive mode
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

	force := hasFlag("--force")
	opts := settingsfile.InsertOpts{
		BinaryPath:      bin,
		RefreshInterval: flagIntOr("--refresh-interval", 5),
		Padding:         0,
		Force:           force,
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

	hadForeign := hasForeignStatusLine(s)

	// Interactive foreign-overwrite confirmation: if foreign statusLine is
	// present and --force not passed, prompt the user (TTY only). CI/non-TTY
	// callers keep --force as the escape hatch.
	if hadForeign && !force {
		if !canPrompt() {
			fmt.Fprintln(os.Stderr, "cc-probeline: settings.json already has a non-cc-probeline statusLine.")
			fmt.Fprintln(os.Stderr, "Re-run with --force to overwrite (backup is saved automatically).")
			return 2
		}
		if !promptForeignOverwrite(existingCommand(s)) {
			fmt.Fprintln(os.Stderr, "cc-probeline: install cancelled — existing statusLine left untouched.")
			return 0
		}
		opts.Force = true
	}

	next, err := settingsfile.InsertStatusLine(s, opts)
	if errors.Is(err, settingsfile.ErrForeignStatusLine) {
		// Defensive: should not happen after the prompt branch above, but
		// keeps non-TTY/--force paths honest.
		fmt.Fprintln(os.Stderr, "cc-probeline: settings.json already has a non-cc-probeline statusLine.")
		fmt.Fprintln(os.Stderr, "Re-run with --force to overwrite (backup is saved automatically).")
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cc-probeline: merge failed:", err)
		return 2
	}

	// Idempotent: skip backup + write when merge produces no change.
	if reflect.DeepEqual(s, next) {
		fmt.Println("cc-probeline: statusLine already wired in", path)
		return 0
	}

	// Backup only when we are about to write (file exists and merge succeeded).
	var backupPath string
	if _, statErr := os.Stat(path); statErr == nil {
		bak, bakErr := settingsfile.Backup(path, time.Now())
		if bakErr != nil {
			fmt.Fprintln(os.Stderr, "cc-probeline: backup failed:", bakErr)
			return 2
		}
		backupPath = bak
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "cc-probeline: cannot create settings directory:", err)
		return 2
	}

	if err := settingsfile.Save(path, next); err != nil {
		fmt.Fprintln(os.Stderr, "cc-probeline: write failed:", err)
		return 2
	}

	// Persist install state so uninstall can restore the user's previous
	// statusLine. Only meaningful when we replaced a foreign block.
	if hadForeign && backupPath != "" {
		if sp := settingsfile.StatePath(); sp != "" {
			st := settingsfile.InstallState{
				PreInstallBackup: backupPath,
				HadForeign:       true,
			}
			if err := settingsfile.SaveState(sp, st); err != nil {
				// Non-fatal: install succeeded, restore on uninstall just
				// won't run. Warn so the user knows.
				fmt.Fprintln(os.Stderr, "cc-probeline: warning: could not save install state:", err)
			}
		}
	}

	fmt.Println("cc-probeline: statusLine wired in", path)
	if hadForeign && backupPath != "" {
		fmt.Println("cc-probeline: previous statusLine backed up to", backupPath)
		fmt.Println("cc-probeline: run `cc-probeline uninstall` to restore it.")
	}
	return 0
}
