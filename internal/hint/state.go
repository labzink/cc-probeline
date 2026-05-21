package hint

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// State is the per-session rotation persisted to disk. ShownIndices records
// already-displayed hint indices; once len(ShownIndices) reaches len(Hints)
// the widget hides the hint row entirely (Widget.Pick returns "").
type State struct {
	ShownIndices []int     `json:"shown_indices"`
	CurrentIndex int       `json:"current_index"`
	LastSwitch   time.Time `json:"last_switch"`
}

// stateFile is the on-disk envelope following specs.md §B2.
type stateFile struct {
	V          int             `json:"v"`
	Key        string          `json:"key"`
	CreatedAt  time.Time       `json:"created_at"`
	TTLSeconds *int            `json:"ttl_seconds"`
	Data       json.RawMessage `json:"data"`
}

const stateVersion = 1

// cacheDir returns the cc-probeline cache directory, or "" when HOME/XDG unset.
func cacheDir() string {
	if x := os.Getenv("XDG_CACHE_HOME"); x != "" {
		return filepath.Join(x, "cc-probeline")
	}
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".cache", "cc-probeline")
}

// StatePath returns the on-disk path for sessionID's hint state, or ""
// when sessionID is empty or HOME/XDG_CACHE_HOME are both unset
// (memory-only mode).
func StatePath(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	d := cacheDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "hint-"+sessionID+".json")
}

// Load reads the persisted State. Returns State{} (zero) when the file is
// missing, corrupt, or sessionID is empty. Never panics.
func Load(sessionID string) State {
	p := StatePath(sessionID)
	if p == "" {
		return State{}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return State{}
	}
	var f stateFile
	if err := json.Unmarshal(b, &f); err != nil {
		return State{}
	}
	var s State
	if err := json.Unmarshal(f.Data, &s); err != nil {
		return State{}
	}
	return s
}

// Save writes State atomically under flock. No-op when sessionID is empty
// or path cannot be resolved.
func Save(sessionID string, s State) error {
	p := StatePath(sessionID)
	if p == "" {
		return errors.New("hint.Save: empty sessionID or HOME unset")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	fl := flock.New(p + ".lock")
	if err := fl.Lock(); err != nil {
		return err
	}
	defer fl.Unlock() //nolint:errcheck

	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	envelope := stateFile{
		V:         stateVersion,
		Key:       "hint",
		CreatedAt: time.Now(),
		Data:      data,
	}
	out, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// AllShown reports whether every hint index has been displayed at least once.
func (s *State) AllShown(total int) bool {
	return len(s.ShownIndices) >= total
}

// Advance moves to the next unseen index and stamps LastSwitch.
// When all indices are shown, Advance is a no-op (caller checks AllShown first).
func (s *State) Advance(total int, now time.Time) {
	if total <= 0 {
		return
	}
	seen := make(map[int]bool, len(s.ShownIndices))
	for _, idx := range s.ShownIndices {
		seen[idx] = true
	}
	// Search for the next unseen index starting after CurrentIndex.
	for i := 0; i < total; i++ {
		cand := (s.CurrentIndex + i + 1) % total
		if !seen[cand] {
			s.CurrentIndex = cand
			s.ShownIndices = append(s.ShownIndices, cand)
			s.LastSwitch = now
			return
		}
	}
	// All shown — caller handles via AllShown.
}
