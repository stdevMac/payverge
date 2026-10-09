package services

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func requireFieldError(t *testing.T, err error, field, reason string) {
	t.Helper()
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrReservationFieldInvalid), "must unwrap to ErrReservationFieldInvalid: %v", err)
	var fieldErr *ReservationFieldError
	require.True(t, errors.As(err, &fieldErr))
	assert.Equal(t, field, fieldErr.Field)
	assert.Equal(t, reason, fieldErr.Reason)
}

func TestReservationFieldLimits_Boundaries(t *testing.T) {
	// Multi-byte runes prove the caps count characters, not bytes.
	require.NoError(t, validateReservationCustomerName(strings.Repeat("ñ", 80)))
	requireFieldError(t, validateReservationCustomerName(strings.Repeat("ñ", 81)), "customer_name", ReservationFieldReasonTooLong)

	require.NoError(t, validateReservationCustomerPhone(strings.Repeat("9", 32)))
	requireFieldError(t, validateReservationCustomerPhone(strings.Repeat("9", 33)), "customer_phone", ReservationFieldReasonTooLong)
	require.NoError(t, validateReservationCustomerPhone(""), "phone stays optional")

	require.NoError(t, validateReservationSpecialRequests(strings.Repeat("é", 300)))
	requireFieldError(t, validateReservationSpecialRequests(strings.Repeat("é", 301)), "special_requests", ReservationFieldReasonTooLong)
	require.NoError(t, validateReservationSpecialRequests(""), "special requests stay optional")

	var fieldErr *ReservationFieldError
	require.True(t, errors.As(validateReservationCustomerName(strings.Repeat("a", 81)), &fieldErr))
	assert.Equal(t, MaxReservationCustomerNameRunes, fieldErr.Max)
}

func TestReservationFieldLimits_RejectsControlCharacters(t *testing.T) {
	requireFieldError(t, validateReservationCustomerName(""), "customer_name", ReservationFieldReasonRequired)
	for _, name := range []string{
		"Ana\nClick http://evil.example",
		"Ana\rBcc: x@y",
		"Ana\tTab",
		"Ana\x00",
		"Ana\x1b[31m",
		"Ana\u0085",
		"Ana ‮lpm.exe",
		"Ana ⁦x⁩",
		"Ana\xff",
	} {
		requireFieldError(t, validateReservationCustomerName(name), "customer_name", ReservationFieldReasonInvalid)
	}
	requireFieldError(t, validateReservationCustomerPhone("+54 11\n5555"), "customer_phone", ReservationFieldReasonInvalid)

	require.NoError(t, validateReservationSpecialRequests("Window seat\nHigh chair\r\n\tplease"),
		"line breaks and tabs are fine in special requests")
	requireFieldError(t, validateReservationSpecialRequests("ok\x07"), "special_requests", ReservationFieldReasonInvalid)
	requireFieldError(t, validateReservationSpecialRequests("ok ‮"), "special_requests", ReservationFieldReasonInvalid)

	// Right-to-left names keep their LRM/RLM marks.
	require.NoError(t, validateReservationCustomerName("‏محمد"))
	require.NoError(t, validateReservationCustomerName("José-María O'Neil (party of 4)"))
}

func TestReservationServiceCreateReservation_RejectsOversizedGuestFields(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-field-caps")
	createTestTable(t, db, business.ID, "FC1", 4)
	configureReservationSettings(t, business.ID, nil)
	target := nextDayAt(19, 30)

	base := func() CreateReservationInput {
		return CreateReservationInput{
			CustomerName:    "Field Cap Guest",
			CustomerPhone:   "5551234567",
			CustomerEmail:   "caps@example.com",
			PartySize:       2,
			ReservationTime: target.Format(time.RFC3339),
		}
	}

	for _, tc := range []struct {
		name   string
		mutate func(*CreateReservationInput)
		field  string
		reason string
	}{
		{"name too long", func(in *CreateReservationInput) { in.CustomerName = strings.Repeat("a", 81) }, "customer_name", ReservationFieldReasonTooLong},
		{"name blank", func(in *CreateReservationInput) { in.CustomerName = "   " }, "customer_name", ReservationFieldReasonRequired},
		{"name newline", func(in *CreateReservationInput) { in.CustomerName = "Ana\nVisit evil.example" }, "customer_name", ReservationFieldReasonInvalid},
		{"phone too long", func(in *CreateReservationInput) { in.CustomerPhone = strings.Repeat("1", 33) }, "customer_phone", ReservationFieldReasonTooLong},
		{"requests too long", func(in *CreateReservationInput) { in.SpecialRequests = strings.Repeat("x", 301) }, "special_requests", ReservationFieldReasonTooLong},
	} {
		for _, isGuest := range []bool{true, false} {
			input := base()
			tc.mutate(&input)
			_, err := service.CreateReservation(business.ID, input, "host", isGuest)
			requireFieldError(t, err, tc.field, tc.reason)
		}
	}

	var count int64
	require.NoError(t, db.Model(&database.TableReservation{}).Where("business_id = ?", business.ID).Count(&count).Error)
	assert.Zero(t, count, "a refused field must not persist a booking")

	// Surrounding whitespace is trimmed before the cap is applied.
	input := base()
	input.CustomerName = "  " + strings.Repeat("a", 80) + "  "
	input.SpecialRequests = strings.Repeat("x", 300)
	created, err := service.CreateReservation(business.ID, input, "customer", true)
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("a", 80), created.CustomerName)
}

func TestReservationServiceUpdateReservation_ValidatesOnlyChangedFields(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-field-caps-update")
	createTestTable(t, db, business.ID, "FC2", 4)
	configureReservationSettings(t, business.ID, nil)

	created, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Legacy Guest",
		CustomerPhone:   "5550000000",
		CustomerEmail:   "legacy@example.com",
		PartySize:       2,
		ReservationTime: nextDayAt(20, 0).Format(time.RFC3339),
	}, "host", false)
	require.NoError(t, err)

	// A row written before the caps existed.
	legacyName := strings.Repeat("L", 120)
	require.NoError(t, db.Model(&database.TableReservation{}).Where("id = ?", created.ID).
		Update("customer_name", legacyName).Error)

	// The dashboard resends every field: an unchanged legacy name must not
	// block a phone edit.
	newPhone := "5559999999"
	updated, _, err := service.UpdateReservation(business.ID, created.ID, UpdateReservationInput{
		CustomerName:  &legacyName,
		CustomerPhone: &newPhone,
	}, "host")
	require.NoError(t, err)
	assert.Equal(t, newPhone, updated.CustomerPhone)
	assert.Equal(t, legacyName, updated.CustomerName)

	tooLong := strings.Repeat("N", 81)
	_, _, err = service.UpdateReservation(business.ID, created.ID, UpdateReservationInput{CustomerName: &tooLong}, "host")
	requireFieldError(t, err, "customer_name", ReservationFieldReasonTooLong)

	badPhone := "555\n0000"
	_, _, err = service.UpdateReservation(business.ID, created.ID, UpdateReservationInput{CustomerPhone: &badPhone}, "host")
	requireFieldError(t, err, "customer_phone", ReservationFieldReasonInvalid)

	longRequests := strings.Repeat("r", 301)
	_, _, err = service.UpdateReservation(business.ID, created.ID, UpdateReservationInput{SpecialRequests: &longRequests}, "host")
	requireFieldError(t, err, "special_requests", ReservationFieldReasonTooLong)

	var stored database.TableReservation
	require.NoError(t, db.First(&stored, created.ID).Error)
	assert.Equal(t, legacyName, stored.CustomerName, "refused edits must not persist")
	assert.Equal(t, newPhone, stored.CustomerPhone)
	assert.Empty(t, stored.SpecialRequests)
}
