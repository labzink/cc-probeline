// Package parser_test — system-injected user records must not advance the prompt
// group (Phase 6.9, task 1): CC writes meta caveats, slash-command expansions,
// task-completion notifications and interrupt markers as user-role records during
// autonomous operation. Counting them as prompt boundaries dimmed older rows even
// though no human prompt was entered.
package parser_test

import (
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
)

func TestAggregate_GroupID_SystemInjectedDoesNotAdvance(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	records := []parser.Record{
		makeUser("u1", base), // real prompt → group #1
		makeAssistant("a1", base.Add(1*time.Minute), false), // GroupID 1
		// System-injected records — none must advance the group:
		{Type: "user", UUID: "meta", Timestamp: base.Add(2 * time.Minute), IsMeta: true, Text: "<local-command-caveat>Caveat: ..."},
		{Type: "user", UUID: "notif", Timestamp: base.Add(3 * time.Minute), Text: "<task-notification>\n<task-id>x</task-id>"},
		{Type: "user", UUID: "cmd", Timestamp: base.Add(4 * time.Minute), Text: "<command-name>/clear</command-name>"},
		{Type: "user", UUID: "intr", Timestamp: base.Add(5 * time.Minute), Text: "[Request interrupted by user]"},
		makeAssistant("a2", base.Add(6*time.Minute), false), // still GroupID 1 (no real prompt between)
		// Real human prompt → group #2:
		{Type: "user", UUID: "u2", Timestamp: base.Add(7 * time.Minute), Text: "continue the work please"},
		makeAssistant("a3", base.Add(8*time.Minute), false), // GroupID 2
	}

	got := parser.Aggregate(records)

	want := map[string]int{"a1": 1, "a2": 1, "a3": 2}
	for _, turn := range got.Turns {
		if w, ok := want[turn.UUID]; ok && turn.GroupID != w {
			t.Errorf("uuid=%q GroupID=%d, want %d — system-injected records must not advance the group", turn.UUID, turn.GroupID, w)
		}
	}
}
