package server

import (
	"errors"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/verification"
)

// verifyPersistedOperatorSession checks the live identity state behind an
// already-authenticated persisted session. Email and Google sessions cannot
// outlive provider verification or an email-address change; other session
// realms are outside this boundary.
func verifyPersistedOperatorSession(sess *session.UserSession) (bool, error) {
	if sess == nil {
		return false, nil
	}
	switch sess.Provider {
	case "email", "register", "google":
		if sess.UserID == nil || *sess.UserID == 0 {
			return false, nil
		}
		if database.GetDB() == nil {
			return false, errors.New("operator verification database unavailable")
		}
		return verification.NewService(database.GetDB()).IsOperatorVerified(*sess.UserID, sess.Provider)
	case demomode.SessionProvider, demomode.StaffSessionProvider:
		// One-click public-demo owner and staff sessions live only while
		// DEMO_MODE is on: switching it off ends every one of them at the
		// next request.
		return config.DemoModeEnabled(), nil
	default:
		return true, nil
	}
}

func revokeUnverifiedOperatorSession(sess *session.UserSession) error {
	if sess == nil || session.GlobalStore == nil {
		return nil
	}
	return session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonEmailUnverified)
}
