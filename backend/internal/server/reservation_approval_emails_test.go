package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestApprovalOutcomeEmailSelection(t *testing.T) {
	res := &database.TableReservation{Status: "confirmed", CustomerEmail: "g@example.com"}
	assert.Equal(t, "confirmed", approvalOutcomeEmailKind("pending", res))

	res = &database.TableReservation{Status: "cancelled", CustomerEmail: "g@example.com", CancellationReason: "full tonight"}
	assert.Equal(t, "declined", approvalOutcomeEmailKind("pending", res))

	res = &database.TableReservation{Status: "cancelled", CustomerEmail: "g@example.com"}
	assert.Equal(t, "cancelled", approvalOutcomeEmailKind("confirmed", res), "non-pending cancels keep the regular cancelled email")

	res = &database.TableReservation{Status: "confirmed", CustomerEmail: ""}
	assert.Equal(t, "", approvalOutcomeEmailKind("pending", res), "no email address, no email")

	res = &database.TableReservation{Status: "seated", CustomerEmail: "g@example.com"}
	assert.Equal(t, "", approvalOutcomeEmailKind("confirmed", res), "non-outcome transitions earn no approval email")

	assert.Equal(t, "", approvalOutcomeEmailKind("pending", nil), "nil reservation, no email")

	require.NotNil(t, reservationApprovalEmailHook)
}
