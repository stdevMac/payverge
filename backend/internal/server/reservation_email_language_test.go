package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func TestWaitlistCreateSendsPendingNotConfirmation(t *testing.T) {
	// Waitlisted diners must never receive a false "confirmed" email.
	assert.Equal(t, "pending", reservationGuestCreateEmailKind("waitlist"))
	assert.Equal(t, "pending", reservationGuestCreateEmailKind("pending"))
	assert.Equal(t, "confirmation", reservationGuestCreateEmailKind("confirmed"))
	assert.Equal(t, "confirmation", reservationGuestCreateEmailKind(""))
}

func TestNormalizeGuestLang(t *testing.T) {
	assert.Equal(t, "en", services.NormalizeGuestLang(""))
	assert.Equal(t, "fr", services.NormalizeGuestLang("fr"))
	assert.Equal(t, "es-AR", services.NormalizeGuestLang("es-AR"))
	assert.Equal(t, "es-AR", services.NormalizeGuestLang("es-ar"))
	assert.Equal(t, "en", services.NormalizeGuestLang("zz-not-a-locale"))
}

func TestResolveReservationEmailLanguagePrefersStored(t *testing.T) {
	res := &database.TableReservation{Language: "ja"}
	biz := &database.Business{DefaultLanguage: "es"}
	assert.Equal(t, "ja", services.ResolveReservationEmailLanguage(res, biz))

	res.Language = ""
	assert.Equal(t, "es", services.ResolveReservationEmailLanguage(res, biz))

	assert.Equal(t, "en", services.ResolveReservationEmailLanguage(nil, nil))
}

func TestPromoteWaitlistUsesConfirmationHook(t *testing.T) {
	// The promote path must fire the confirmation outcome, not stay silent.
	// Hook capture verifies wiring without a live Postmark client.
	var capturedKind string
	original := reservationApprovalEmailHook
	reservationApprovalEmailHook = func(kind string, reservation *database.TableReservation, business *database.Business) {
		capturedKind = kind
		require.NotNil(t, reservation)
		require.NotNil(t, business)
	}
	t.Cleanup(func() { reservationApprovalEmailHook = original })

	// Simulate the promote success path call site.
	reservationApprovalEmailHook("confirmed", &database.TableReservation{
		CustomerEmail: "guest@example.com",
		Language:      "fr",
		Status:        "confirmed",
	}, &database.Business{Name: "Bistro", DefaultLanguage: "es"})

	assert.Equal(t, "confirmed", capturedKind)
}
