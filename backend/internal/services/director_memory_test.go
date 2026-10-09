package services

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

func msg(id uint, role database.DirectorMessageRole, content string) database.DirectorConsoleMessage {
	return database.DirectorConsoleMessage{ID: id, Role: role, Content: content}
}

func TestBuildPriorTurns_ExcludesCurrentAndBounds(t *testing.T) {
	// Ascending chronological order, as ListDirectorConsoleMessages returns.
	in := []database.DirectorConsoleMessage{
		msg(1, database.DirectorMessageRoleUser, "q1"),
		msg(2, database.DirectorMessageRoleAssistant, "a1"),
		msg(3, database.DirectorMessageRoleUser, "q2"),
		msg(4, database.DirectorMessageRoleAssistant, "a2"),
		msg(5, database.DirectorMessageRoleUser, "current"), // current turn
	}
	got := buildPriorTurns(in, 5, 3)

	if len(got) != 4 {
		t.Fatalf("want 4 prior turns, got %d", len(got))
	}
	if got[0].Role != llm.RoleUser || got[0].Text != "q1" {
		t.Fatalf("first turn wrong: %+v", got[0])
	}
	if got[1].Role != llm.RoleAssistant || got[1].Text != "a1" {
		t.Fatalf("assistant turn wrong: %+v", got[1])
	}
	for _, m := range got {
		if m.Text == "current" {
			t.Fatal("current turn must be excluded")
		}
	}
}

func TestBuildPriorTurns_BoundsToMaxTurns(t *testing.T) {
	in := []database.DirectorConsoleMessage{
		msg(1, database.DirectorMessageRoleUser, "q1"),
		msg(2, database.DirectorMessageRoleAssistant, "a1"),
		msg(3, database.DirectorMessageRoleUser, "q2"),
		msg(4, database.DirectorMessageRoleAssistant, "a2"),
		msg(5, database.DirectorMessageRoleUser, "q3"),
		msg(6, database.DirectorMessageRoleAssistant, "a3"),
		msg(7, database.DirectorMessageRoleUser, "q4"),
		msg(8, database.DirectorMessageRoleAssistant, "a4"),
	}
	got := buildPriorTurns(in, 0, 2) // maxTurns 2 → at most 4 messages
	if len(got) != 4 {
		t.Fatalf("want 4 (last 2 exchanges), got %d", len(got))
	}
	if got[0].Text != "q3" {
		t.Fatalf("want newest 2 exchanges; first is %q", got[0].Text)
	}
}

func TestBuildPriorTurns_EmptyAndZero(t *testing.T) {
	if got := buildPriorTurns(nil, 0, 3); got != nil {
		t.Fatalf("nil input → nil, got %+v", got)
	}
	if got := buildPriorTurns([]database.DirectorConsoleMessage{msg(1, database.DirectorMessageRoleUser, "x")}, 0, 0); got != nil {
		t.Fatalf("maxTurns 0 → nil, got %+v", got)
	}
}

func TestBuildPriorTurns_DropsWhitespaceAndUnknownRoles(t *testing.T) {
	in := []database.DirectorConsoleMessage{
		msg(1, database.DirectorMessageRoleUser, "   "),       // whitespace-only → dropped
		msg(2, database.DirectorMessageRole("system"), "sys"), // non-user/assistant role → dropped
		msg(3, database.DirectorMessageRoleUser, "real"),
		msg(4, database.DirectorMessageRoleAssistant, "answer"),
	}
	got := buildPriorTurns(in, 0, 3)
	if len(got) != 2 {
		t.Fatalf("want 2 kept turns, got %d (%+v)", len(got), got)
	}
	if got[0].Text != "real" || got[1].Text != "answer" {
		t.Fatalf("want [real answer], got %+v", got)
	}
}

func BenchmarkBuildPriorTurns(b *testing.B) {
	in := make([]database.DirectorConsoleMessage, 0, 12)
	for i := uint(1); i <= 12; i++ {
		role := database.DirectorMessageRoleUser
		if i%2 == 0 {
			role = database.DirectorMessageRoleAssistant
		}
		in = append(in, msg(i, role, "some prior message text of moderate length"))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildPriorTurns(in, 11, directorMemoryMaxTurns)
	}
}
