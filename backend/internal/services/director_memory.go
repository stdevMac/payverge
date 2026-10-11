package services

import (
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

const (
	// directorMemoryMaxTurns is how many prior exchanges (user+assistant) the
	// loop replays. directorMemoryMessageLimit bounds the DB read feeding it.
	directorMemoryMaxTurns     = 3
	directorMemoryMessageLimit = 12
)

// buildPriorTurns converts persisted thread messages (ascending chronological,
// as ListDirectorConsoleMessages returns) into compact llm.Message turns for
// the loop. It excludes the just-saved current user message and keeps at most
// maxTurns exchanges. Assistant turns reuse the stored summary (Content), which
// is already PII-redacted at save time — never the full structured JSON.
//
// Pass currentUserMsgID 0 when there is no message to exclude: DB-fetched rows
// always carry a real autoincrement ID (>= 1), so 0 is a safe "exclude nothing"
// sentinel. Messages whose role is neither user nor assistant (e.g. system) and
// whitespace-only messages are dropped.
//
// excludeIDs drops further rows by ID. Regenerate uses it for the trailing
// assistant answer that is still persisted (it is only swapped out once a
// replacement exists — L4-15) so the model never sees the answer it is
// replacing as a prior turn.
func buildPriorTurns(messages []database.DirectorConsoleMessage, currentUserMsgID uint, maxTurns int, excludeIDs ...uint) []llm.Message {
	if maxTurns <= 0 || len(messages) == 0 {
		return nil
	}
	var excluded map[uint]struct{}
	if len(excludeIDs) > 0 {
		excluded = make(map[uint]struct{}, len(excludeIDs))
		for _, id := range excludeIDs {
			excluded[id] = struct{}{}
		}
	}
	turns := make([]llm.Message, 0, maxTurns*2)
	for _, m := range messages {
		if m.ID != 0 && m.ID == currentUserMsgID {
			continue
		}
		if _, skip := excluded[m.ID]; skip && m.ID != 0 {
			continue
		}
		text := strings.TrimSpace(m.Content)
		if text == "" {
			continue
		}
		switch m.Role {
		case database.DirectorMessageRoleUser:
			turns = append(turns, llm.Message{Role: llm.RoleUser, Text: text})
		case database.DirectorMessageRoleAssistant:
			turns = append(turns, llm.Message{Role: llm.RoleAssistant, Text: text})
		}
	}
	if limit := maxTurns * 2; len(turns) > limit {
		turns = turns[len(turns)-limit:]
	}
	if len(turns) == 0 {
		return nil
	}
	return turns
}
