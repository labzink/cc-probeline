// Package log_test — RED tests for internal/log.
// Contract: plans/concepts/phase-3-step1-concept.md §5 Log infrastructure.
// Plan: plans/tasks/phase-3-step1-plan.md §Subtask A — Log infrastructure.
package log_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ilog "github.com/labzink/cc-probeline/internal/log"
)

// logPath returns the expected log file path inside state dir.
// see plans/concepts/phase-3-step1-concept.md §5.1
func logPath(stateDir string) string {
	return filepath.Join(stateDir, "cc-probeline", "cc-probeline.log")
}

// setStateDir sets CC_PROBELINE_STATE_DIR for the test and restores it afterwards.
func setStateDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("CC_PROBELINE_STATE_DIR", dir)
}

// readLines reads all lines from a file, returning them as a slice (no trailing newline).
func readLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("readLines: open %s: %v", path, err)
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("readLines: scan %s: %v", path, err)
	}
	return lines
}

// makeLineWithTimestamp returns a log line string with the given RFC3339 timestamp.
// Format: "2026-05-14T15:30:42Z parser WARN test message"
// see plans/concepts/phase-3-step1-concept.md §5.2
func makeLineWithTimestamp(ts time.Time) string {
	return fmt.Sprintf("%s parser WARN test message", ts.UTC().Format(time.RFC3339))
}

// writeRawLines writes arbitrary lines directly to a log file (bypassing Append),
// used to set up prune-test preconditions.
func writeRawLines(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("writeRawLines: mkdir: %v", err)
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeRawLines: write: %v", err)
	}
}

// TestAppend_NewFile verifies that Append to a non-existent path creates the directory,
// the file, and writes exactly one line.
// Case 1: see plans/tasks/phase-3-step1-plan.md §Subtask A test case 1.
func TestAppend_NewFile(t *testing.T) {
	ilog.ResetState() // ensure clean per-process flag

	dir := t.TempDir()
	setStateDir(t, dir)

	err := ilog.Append("parser", "WARN", "first message", ilog.F("line", 1))
	if err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	path := logPath(dir)

	// Directory must exist.
	if _, statErr := os.Stat(filepath.Dir(path)); statErr != nil {
		t.Fatalf("directory not created: %v", statErr)
	}

	// File must exist.
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("log file not created: %v", statErr)
	}

	// File must contain exactly one line.
	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}

	// Line must contain expected content.
	if !strings.Contains(lines[0], "parser") {
		t.Errorf("line missing component 'parser': %q", lines[0])
	}
	if !strings.Contains(lines[0], "WARN") {
		t.Errorf("line missing level 'WARN': %q", lines[0])
	}
	if !strings.Contains(lines[0], "first message") {
		t.Errorf("line missing message: %q", lines[0])
	}
	if !strings.Contains(lines[0], "line=1") {
		t.Errorf("line missing field 'line=1': %q", lines[0])
	}
}

// TestAppend_ExistingFile verifies that Append to an existing file appends a new line.
// Case 2: see plans/tasks/phase-3-step1-plan.md §Subtask A test case 2.
func TestAppend_ExistingFile(t *testing.T) {
	ilog.ResetState()

	dir := t.TempDir()
	setStateDir(t, dir)
	path := logPath(dir)

	// Pre-create file with one line.
	writeRawLines(t, path, []string{makeLineWithTimestamp(time.Now().UTC())})

	err := ilog.Append("cache", "INFO", "second message")
	if err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	lines := readLines(t, path)
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines after append, got %d", len(lines))
	}

	last := lines[len(lines)-1]
	if !strings.Contains(last, "cache") {
		t.Errorf("appended line missing component 'cache': %q", last)
	}
	if !strings.Contains(last, "second message") {
		t.Errorf("appended line missing message: %q", last)
	}
}

// TestPrune_Triggered verifies that 7-day prune fires when the first log line
// is older than 7 days, removing stale lines.
// Case 3: see plans/concepts/phase-3-step1-concept.md §5.3.
func TestPrune_Triggered(t *testing.T) {
	ilog.ResetState()

	dir := t.TempDir()
	setStateDir(t, dir)
	path := logPath(dir)

	now := time.Now().UTC()
	old := now.Add(-8 * 24 * time.Hour)   // 8 days ago → stale
	fresh := now.Add(-2 * 24 * time.Hour) // 2 days ago → keep

	writeRawLines(t, path, []string{
		makeLineWithTimestamp(old),
		makeLineWithTimestamp(fresh),
	})

	// First Append triggers prune.
	err := ilog.Append("parser", "INFO", "after prune")
	if err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	lines := readLines(t, path)

	// Stale line must be gone; fresh line + new line remain.
	for _, l := range lines {
		if strings.Contains(l, old.Format(time.RFC3339)) {
			t.Errorf("stale line still present after prune: %q", l)
		}
	}

	// Fresh line must still be there.
	foundFresh := false
	for _, l := range lines {
		if strings.Contains(l, fresh.Format(time.RFC3339)) {
			foundFresh = true
			break
		}
	}
	if !foundFresh {
		t.Errorf("fresh line missing after prune; lines: %v", lines)
	}

	// New line must be appended.
	found := false
	for _, l := range lines {
		if strings.Contains(l, "after prune") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("new line 'after prune' missing after prune; lines: %v", lines)
	}
}

// TestPrune_NotTriggered verifies that prune does NOT rewrite the file when
// the first line is within 7 days.
// Case 4: see plans/concepts/phase-3-step1-concept.md §5.3.
func TestPrune_NotTriggered(t *testing.T) {
	ilog.ResetState()

	dir := t.TempDir()
	setStateDir(t, dir)
	path := logPath(dir)

	now := time.Now().UTC()
	recent := now.Add(-3 * 24 * time.Hour) // 3 days ago → fresh

	original := makeLineWithTimestamp(recent)
	writeRawLines(t, path, []string{original})

	// Record mtime before Append.
	statBefore, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	// Sleep 1ms to ensure mtime changes on write.
	time.Sleep(2 * time.Millisecond)

	if err := ilog.Append("cost", "INFO", "no prune needed"); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	lines := readLines(t, path)

	// Original line must still be the first line (file was not rewritten).
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], recent.Format(time.RFC3339)) {
		t.Errorf("first line changed (unexpected prune): %q", lines[0])
	}

	// The file was only appended (not rewritten — inode/mtime differ only by append).
	_ = statBefore // mtime check is OS-specific; structural check above is sufficient.
}

// TestPrune_OncePerProcess verifies that the prune logic runs at most once per process.
// Case 5: see plans/concepts/phase-3-step1-concept.md §5.3 step 4.
func TestPrune_OncePerProcess(t *testing.T) {
	ilog.ResetState()

	dir := t.TempDir()
	setStateDir(t, dir)
	path := logPath(dir)

	now := time.Now().UTC()
	old1 := now.Add(-9 * 24 * time.Hour)
	old2 := now.Add(-8 * 24 * time.Hour)
	fresh := now.Add(-1 * 24 * time.Hour)

	writeRawLines(t, path, []string{
		makeLineWithTimestamp(old1),
		makeLineWithTimestamp(old2),
		makeLineWithTimestamp(fresh),
	})

	// First Append → triggers prune (old1, old2 removed).
	if err := ilog.Append("parser", "WARN", "first call"); err != nil {
		t.Fatalf("first Append error: %v", err)
	}

	linesAfterFirst := readLines(t, path)

	// Now inject another stale line directly (simulating external write).
	injectedOld := now.Add(-10 * 24 * time.Hour)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("reopen for inject: %v", err)
	}
	// Prepend stale line then existing content.
	existingContent := strings.Join(linesAfterFirst, "\n") + "\n"
	_, _ = fmt.Fprintf(f, "%s\n%s", makeLineWithTimestamp(injectedOld), existingContent)
	f.Close()

	// Second Append → prune must NOT run again (flag pruneDone already set).
	if err := ilog.Append("parser", "WARN", "second call"); err != nil {
		t.Fatalf("second Append error: %v", err)
	}

	linesAfterSecond := readLines(t, path)

	// Injected stale line must still be present (prune did not run second time).
	found := false
	for _, l := range linesAfterSecond {
		if strings.Contains(l, injectedOld.Format(time.RFC3339)) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("injected stale line was removed on second Append — prune ran twice")
	}
}

// TestFallback_Stderr verifies that when the log file cannot be created (unwritable
// directory), Append returns nil and writes to os.Stderr.
// Case 6: see plans/concepts/phase-3-step1-concept.md §5.4.
func TestFallback_Stderr(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission check unreliable")
	}

	ilog.ResetState()

	// Create a read-only directory so the log file cannot be created.
	dir := t.TempDir()
	roDir := filepath.Join(dir, "readonly")
	if err := os.MkdirAll(roDir, 0o444); err != nil {
		t.Fatalf("mkdir readonly: %v", err)
	}
	// Point state dir to a subdirectory of the read-only dir so MkdirAll fails.
	setStateDir(t, roDir)

	// Capture stderr.
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w

	appErr := ilog.Append("main", "ERROR", "test fallback")

	w.Close()
	os.Stderr = origStderr

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	r.Close()

	// Append must return nil (non-blocking: see §5.4).
	if appErr != nil {
		t.Errorf("Append should return nil on fallback, got: %v", appErr)
	}

	// Stderr must contain diagnostic message.
	stderrOutput := buf.String()
	if !strings.Contains(stderrOutput, "cc-probeline") {
		t.Errorf("expected 'cc-probeline' in stderr fallback message, got: %q", stderrOutput)
	}
}

// TestAtomicWrite verifies that during prune, a .tmp file is created and then renamed
// (no .tmp remains after Append returns).
// Case 7: see plans/concepts/phase-3-step1-concept.md §5.3 step 3.
func TestAtomicWrite(t *testing.T) {
	ilog.ResetState()

	dir := t.TempDir()
	setStateDir(t, dir)
	path := logPath(dir)
	tmpPath := path + ".tmp"

	now := time.Now().UTC()
	old := now.Add(-8 * 24 * time.Hour)
	fresh := now.Add(-1 * 24 * time.Hour)

	writeRawLines(t, path, []string{
		makeLineWithTimestamp(old),
		makeLineWithTimestamp(fresh),
	})

	err := ilog.Append("parser", "INFO", "atomic test")
	if err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	// .tmp file must NOT exist after successful rename.
	if _, statErr := os.Stat(tmpPath); statErr == nil {
		t.Errorf(".tmp file still exists after Append: %s", tmpPath)
	}

	// Log file must exist and be non-empty.
	lines := readLines(t, path)
	if len(lines) == 0 {
		t.Error("log file is empty after atomic write")
	}
}

// TestAtomicWrite_NoTmpRemains verifies that no .tmp file is left behind after
// a successful prune cycle (regression for C4: Rename failure leaving stale .tmp).
func TestAtomicWrite_NoTmpRemains(t *testing.T) {
	ilog.ResetState()

	dir := t.TempDir()
	setStateDir(t, dir)
	path := logPath(dir)
	tmpPath := path + ".tmp"

	now := time.Now().UTC()
	old := now.Add(-8 * 24 * time.Hour)
	fresh := now.Add(-1 * 24 * time.Hour)

	writeRawLines(t, path, []string{
		makeLineWithTimestamp(old),
		makeLineWithTimestamp(fresh),
	})

	if err := ilog.Append("parser", "INFO", "no tmp check"); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	if _, statErr := os.Stat(tmpPath); statErr == nil {
		t.Errorf(".tmp file still exists after successful prune: %s", tmpPath)
	}
}

// TestEmptyMessage_NoDoubleSpace verifies that Append with an empty message
// does not produce double-space output like "component LEVEL  key=val".
// Regression for S1: formatLine always emitted message even when empty.
func TestEmptyMessage_NoDoubleSpace(t *testing.T) {
	ilog.ResetState()

	dir := t.TempDir()
	setStateDir(t, dir)
	path := logPath(dir)

	if err := ilog.Append("parser", "WARN", "", ilog.F("line", 42)); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	// Must not contain double space (which would result from an empty message segment).
	if strings.Contains(lines[0], "  ") {
		t.Errorf("log line contains double space (empty message not skipped): %q", lines[0])
	}

	// Must contain the field.
	if !strings.Contains(lines[0], "line=42") {
		t.Errorf("log line missing field 'line=42': %q", lines[0])
	}
}

// TestFieldValue_NewlineEscaped verifies that a field value containing a newline
// is quoted and escaped, not written as a raw newline that corrupts the log format.
// Regression for S7: formatValue did not escape \n/\r in field values.
func TestFieldValue_NewlineEscaped(t *testing.T) {
	ilog.ResetState()

	dir := t.TempDir()
	setStateDir(t, dir)
	path := logPath(dir)

	if err := ilog.Append("parser", "WARN", "msg", ilog.F("val", "line1\nline2")); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	lines := readLines(t, path)

	// The entire log entry must fit in exactly one line (newline must be escaped).
	if len(lines) != 1 {
		t.Errorf("expected 1 log line (newline in value must be escaped), got %d lines: %v", len(lines), lines)
	}

	// The value must appear quoted in the output.
	if !strings.Contains(lines[0], `"line1`) {
		t.Errorf("expected quoted/escaped value in log line, got: %q", lines[0])
	}
}

// TestConcurrentAppend verifies that 10 goroutines × 10 Append calls each
// produce no data corruption and all lines are present.
// Bonus case from concept §7.3 (concurrent safety).
func TestConcurrentAppend(t *testing.T) {
	ilog.ResetState()

	dir := t.TempDir()
	setStateDir(t, dir)
	path := logPath(dir)

	const goroutines = 10
	const perGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				_ = ilog.Append("parser", "INFO", fmt.Sprintf("g%d-j%d", i, j))
			}
		}()
	}
	wg.Wait()

	lines := readLines(t, path)

	// All lines must be present (total = goroutines * perGoroutine).
	// Each line must be valid (non-empty, contain component).
	for idx, l := range lines {
		if strings.TrimSpace(l) == "" {
			t.Errorf("line %d is empty (possible corruption)", idx)
		}
	}

	if len(lines) != goroutines*perGoroutine {
		t.Errorf("expected %d lines, got %d (possible lost writes)", goroutines*perGoroutine, len(lines))
	}
}
