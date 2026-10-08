package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupManagerPinTestDB mirrors setupTestDB but also migrates Staff and the
// CompVoidAudit table so the PIN service can read/write them.
func setupManagerPinTestDB(t *testing.T) {
	t.Helper()
	setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&Staff{}, &CompVoidAudit{}))
}

func TestValidateRawPin(t *testing.T) {
	cases := []struct {
		name    string
		pin     string
		wantErr error
	}{
		{"valid 4-digit", "1234", nil},
		{"valid 6-digit", "987654", nil},
		{"too short", "12", ErrPinTooShort},
		{"too long", "1234567890123", ErrPinTooLong},
		{"non-numeric", "12ab", ErrPinNotNumeric},
		{"empty", "", ErrPinTooShort},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRawPin(tc.pin)
			if tc.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

func TestSetAndVerifyStaffPin(t *testing.T) {
	setupManagerPinTestDB(t)

	staff := &Staff{
		BusinessID: 1,
		Email:      "manager@example.com",
		Name:       "Test Manager",
		Role:       StaffRoleManager,
		IsActive:   true,
		InvitedBy:  "owner",
	}
	require.NoError(t, db.Create(staff).Error)

	// Initially no PIN configured.
	has, err := StaffHasPin(staff.ID)
	require.NoError(t, err)
	assert.False(t, has, "fresh staff should not have a PIN")

	// Verify against an empty hash returns ErrPinNotSet.
	err = VerifyStaffPin(staff.ID, "1234")
	assert.ErrorIs(t, err, ErrPinNotSet)

	// Set a PIN.
	require.NoError(t, SetStaffPin(staff.ID, "1234"))

	has, err = StaffHasPin(staff.ID)
	require.NoError(t, err)
	assert.True(t, has, "staff should report having a PIN after SetStaffPin")

	// Correct PIN matches.
	assert.NoError(t, VerifyStaffPin(staff.ID, "1234"))

	// Wrong PIN returns ErrPinInvalid.
	assert.ErrorIs(t, VerifyStaffPin(staff.ID, "9999"), ErrPinInvalid)

	// Rotation replaces the hash.
	require.NoError(t, SetStaffPin(staff.ID, "555555"))
	assert.NoError(t, VerifyStaffPin(staff.ID, "555555"))
	assert.ErrorIs(t, VerifyStaffPin(staff.ID, "1234"), ErrPinInvalid)
}

func TestRecordCompVoidAudit(t *testing.T) {
	setupManagerPinTestDB(t)

	staffID := uint(42)
	amount := int64(750)
	entry := &CompVoidAudit{
		BusinessID:  9,
		StaffID:     &staffID,
		TargetType:  "bill_item",
		TargetID:    "abc-123",
		Action:      "void",
		Reason:      "spilled",
		AmountCents: &amount,
		PinPresent:  true,
	}
	require.NoError(t, RecordCompVoidAudit(entry))
	assert.NotZero(t, entry.ID, "audit row should receive an autoincrement id")

	var fetched CompVoidAudit
	require.NoError(t, db.First(&fetched, entry.ID).Error)
	assert.Equal(t, "void", fetched.Action)
	assert.Equal(t, "spilled", fetched.Reason)
	require.NotNil(t, fetched.AmountCents)
	assert.Equal(t, int64(750), *fetched.AmountCents)
	assert.True(t, fetched.PinPresent)
}
