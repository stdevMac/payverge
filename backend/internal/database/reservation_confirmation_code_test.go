package database

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeConfirmationCode(t *testing.T) {
	assert.Equal(t, "ABC123DEF456", NormalizeConfirmationCode("ABC123DEF456"))
	assert.Equal(t, "ABC123DEF456", NormalizeConfirmationCode("abc123def456"))
	assert.Equal(t, "ABC123DEF456", NormalizeConfirmationCode("AbC123dEf456"))
	assert.Equal(t, "ABC123DEF456", NormalizeConfirmationCode("  abc123def456  "))
	assert.Equal(t, "", NormalizeConfirmationCode("   "))
	assert.Equal(t, "", NormalizeConfirmationCode(""))
}

func TestGetReservationByConfirmationCodeNormalizesCaseAndWhitespace(t *testing.T) {
	setupReservationPerfTestDB(t, nil)
	biz := helperReservationPerfBusiness(t)

	stored := &TableReservation{
		BusinessID:       biz.ID,
		CustomerName:     "Case Guest",
		CustomerEmail:    "case-guest@example.com",
		PartySize:        2,
		ReservationTime:  time.Now().UTC().Add(48 * time.Hour),
		Duration:         90,
		Status:           "confirmed",
		Source:           "customer",
		ConfirmationCode: "ABC123DEF456",
	}
	require.NoError(t, db.Create(stored).Error)

	variants := []string{
		"ABC123DEF456",
		"abc123def456",
		"AbC123dEf456",
		"  ABC123DEF456  ",
		"  abc123def456  ",
	}
	for _, code := range variants {
		loaded, err := GetReservationByConfirmationCode(code)
		require.NoError(t, err, "code %q", code)
		require.NotNil(t, loaded)
		assert.Equal(t, stored.ID, loaded.ID, "code %q", code)
		assert.Equal(t, "ABC123DEF456", loaded.ConfirmationCode, "code %q", code)
	}

	_, err := GetReservationByConfirmationCode("NOPE00000000")
	require.True(t, errors.Is(err, ErrReservationNotFound), "unknown code: %v", err)

	_, err = GetReservationByConfirmationCode("   ")
	require.True(t, errors.Is(err, ErrReservationNotFound), "blank code: %v", err)
}

func TestReservationConfirmationCodeExistsIsCaseInsensitive(t *testing.T) {
	setupReservationPerfTestDB(t, nil)
	biz := helperReservationPerfBusiness(t)

	require.NoError(t, db.Create(&TableReservation{
		BusinessID:       biz.ID,
		CustomerName:     "Exists Guest",
		PartySize:        2,
		ReservationTime:  time.Now().UTC().Add(48 * time.Hour),
		Duration:         90,
		Status:           "confirmed",
		Source:           "staff",
		ConfirmationCode: "C0LL1DE00001",
	}).Error)

	for _, code := range []string{"C0LL1DE00001", "c0ll1de00001", "  C0ll1DE00001  "} {
		exists, err := ReservationConfirmationCodeExists(code)
		require.NoError(t, err, "code %q", code)
		assert.True(t, exists, "code %q should collide with stored uppercase row", code)
	}

	exists, err := ReservationConfirmationCodeExists("NOPE00000000")
	require.NoError(t, err)
	assert.False(t, exists)

	exists, err = ReservationConfirmationCodeExists("   ")
	require.NoError(t, err)
	assert.False(t, exists)
}
