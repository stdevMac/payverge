package database

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// The public handlers map missing reservations to 404 via errors.Is on this
// sentinel — fragile string comparison must not come back.
func TestReservationGettersReturnNotFoundSentinel(t *testing.T) {
	setupReservationPerfTestDB(t, nil)

	_, err := GetReservationByBusinessAndID(1, 999999)
	require.True(t, errors.Is(err, ErrReservationNotFound), "GetReservationByBusinessAndID: %v", err)

	_, err = GetReservationByConfirmationCode("NOPE00000000")
	require.True(t, errors.Is(err, ErrReservationNotFound), "GetReservationByConfirmationCode: %v", err)
}
