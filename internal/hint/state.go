package hint

import "time"

// State is the per-session rotation persisted to disk. ShownIndices records
// already-displayed hint indices; once len(ShownIndices) reaches len(Hints)
// the widget hides the hint row entirely (Widget.Pick returns "").
type State struct {
	ShownIndices []int     `json:"shown_indices"`
	CurrentIndex int       `json:"current_index"`
	LastSwitch   time.Time `json:"last_switch"`
}

// StatePath returns the on-disk path for sessionID's hint state, or ""
// when sessionID is empty or HOME/XDG_CACHE_HOME are both unset
// (memory-only mode).
//
// Phase 4.4.0 foundation: stub returns "". Real path resolution lands in 4.4.b.
func StatePath(sessionID string) string {
	_ = sessionID
	return ""
}

// Load reads the persisted State. Returns State{} (zero) when the file is
// missing, corrupt, or sessionID is empty. Never panics.
//
// Phase 4.4.0 foundation: stub returns State{}. Real I/O lands in 4.4.b.
func Load(sessionID string) State {
	_ = sessionID
	return State{}
}

// Save writes State atomically under flock. No-op (returns nil) when
// sessionID is empty.
//
// Phase 4.4.0 foundation: stub returns nil. Real I/O lands in 4.4.b.
func Save(sessionID string, s State) error {
	_ = sessionID
	_ = s
	return nil
}

// AllShown reports whether every hint index has been displayed at least once.
//
// Phase 4.4.0 foundation: stub returns false. Real logic lands in 4.4.b.
func (s *State) AllShown(total int) bool {
	_ = total
	return false
}

// Advance moves to the next unseen index and stamps LastSwitch.
//
// Phase 4.4.0 foundation: stub no-op. Real logic lands in 4.4.b.
func (s *State) Advance(total int, now time.Time) {
	_ = total
	_ = now
}
