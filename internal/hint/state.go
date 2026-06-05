package hint

import "time"

// State is the per-session hint rotation. ShownIndices records already-displayed
// hint indices; once len(ShownIndices) reaches len(Hints) the widget hides the
// hint row entirely (Widget.Pick returns "").
//
// Phase 6.95.b: persistence moved into state.Session.HintRotation (the unified
// per-session state file). This type carries only the rotation data and its
// pure transition logic (AllShown/Advance); it performs no disk I/O.
type State struct {
	ShownIndices []int     `json:"shown_indices"`
	CurrentIndex int       `json:"current_index"`
	LastSwitch   time.Time `json:"last_switch"`
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
