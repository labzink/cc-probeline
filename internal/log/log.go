// Package log provides an append-only structured logger for cc-probeline.
// Writes go to a single file under the OS state directory; on failure the
// payload falls back to os.Stderr without returning an error to the caller.
package log

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Field is a key/value pair appended to a log line.
type Field struct {
	Key   string
	Value any
}

// F builds a Field; convenience constructor for call sites.
func F(key string, value any) Field {
	return Field{Key: key, Value: value}
}

var (
	mu        sync.Mutex
	pruneDone bool
)

// ResetState clears the once-per-process prune flag. Test-only.
func ResetState() {
	mu.Lock()
	defer mu.Unlock()
	pruneDone = false
}

// Append writes a single formatted line to the log file. On any I/O error
// the line is written to os.Stderr instead and nil is returned.
func Append(component, level, message string, fields ...Field) error {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now().UTC()
	line := formatLine(now, component, level, message, fields)

	path, err := resolveLogPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cc-probeline:", line)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "cc-probeline:", line)
		return nil
	}

	if !pruneDone {
		pruneIfNeeded(path, now)
		pruneDone = true
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cc-probeline:", line)
		return nil
	}
	defer f.Close()

	if _, err := fmt.Fprintln(f, line); err != nil {
		fmt.Fprintln(os.Stderr, "cc-probeline:", line)
	}
	return nil
}

func formatLine(ts time.Time, component, level, message string, fields []Field) string {
	var sb strings.Builder
	sb.WriteString(ts.Format(time.RFC3339))
	sb.WriteByte(' ')
	sb.WriteString(component)
	sb.WriteByte(' ')
	sb.WriteString(level)
	sb.WriteByte(' ')
	sb.WriteString(strings.ReplaceAll(message, "\n", " "))
	for _, fld := range fields {
		sb.WriteByte(' ')
		sb.WriteString(fld.Key)
		sb.WriteByte('=')
		sb.WriteString(formatValue(fld.Value))
	}
	return sb.String()
}

func formatValue(v any) string {
	s := fmt.Sprintf("%v", v)
	if strings.ContainsAny(s, " \t\"") {
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `"`, `\"`)
		return `"` + s + `"`
	}
	return s
}

func resolveLogPath() (string, error) {
	if override := os.Getenv("CC_PROBELINE_STATE_DIR"); override != "" {
		return filepath.Join(override, "cc-probeline", "cc-probeline.log"), nil
	}
	if runtime.GOOS == "windows" {
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			return "", fmt.Errorf("LOCALAPPDATA not set")
		}
		return filepath.Join(local, "cc-probeline", "state", "cc-probeline.log"), nil
	}
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "cc-probeline", "cc-probeline.log"), nil
}

func pruneIfNeeded(path string, now time.Time) {
	f, err := os.Open(path)
	if err != nil {
		return
	}

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		f.Close()
		return
	}
	firstTs, err := parseLineTimestamp(sc.Text())
	if err != nil {
		f.Close()
		return
	}

	threshold := now.Add(-7 * 24 * time.Hour)
	if !firstTs.Before(threshold) {
		f.Close()
		return
	}

	if _, err := f.Seek(0, 0); err != nil {
		f.Close()
		return
	}
	sc = bufio.NewScanner(f)
	var kept []string
	for sc.Scan() {
		line := sc.Text()
		ts, err := parseLineTimestamp(line)
		if err != nil {
			kept = append(kept, line)
			continue
		}
		if !ts.Before(threshold) {
			kept = append(kept, line)
		}
	}
	f.Close()

	tmp := path + ".tmp"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return
	}
	for _, l := range kept {
		if _, werr := fmt.Fprintln(out, l); werr != nil {
			out.Close()
			os.Remove(tmp)
			return
		}
	}
	if cerr := out.Close(); cerr != nil {
		os.Remove(tmp)
		return
	}
	_ = os.Rename(tmp, path)
}

func parseLineTimestamp(line string) (time.Time, error) {
	sp := strings.IndexByte(line, ' ')
	if sp <= 0 {
		return time.Time{}, fmt.Errorf("no timestamp")
	}
	return time.Parse(time.RFC3339, line[:sp])
}
