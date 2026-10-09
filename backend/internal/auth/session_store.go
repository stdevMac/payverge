package auth

// Re-export session utilities so callers within auth don't need to import
// the session package directly. The actual implementation lives in
// github.com/stdevmac/payverge/backend/internal/session to avoid import cycles between auth and server.

import "github.com/stdevmac/payverge/backend/internal/session"

// HashToken delegates to session.HashToken.
func HashToken(token string) string {
	return session.HashToken(token)
}

// CreateSessionInput is an alias for session.CreateInput.
type CreateSessionInput = session.CreateInput
