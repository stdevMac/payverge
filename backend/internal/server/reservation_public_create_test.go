package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// TestPublicReservationCreateRequiresValidEmail guards the public (guest)
// reservation create endpoint: email is the guest's only channel for approval
// results, confirmations, cancellations and reminders, so an online booking
// without a reachable email must be rejected with a 400 that names the email
// problem. Staff-created bookings (walk-ins/phone) go through the
// authenticated handler and stay email-optional — this test covers only
// CreatePublicReservation.
func TestPublicReservationCreateRequiresValidEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	business := createPublicBusinessRouteTestBusiness(t, "email-req-biz", true, true)

	require.NoError(t, database.GetDB().Create(&database.ReservationSettings{
		BusinessID:           business.ID,
		Enabled:              true,
		MaxAdvanceDays:       30,
		MinAdvanceMinutes:    30,
		MinPartySize:         1,
		MaxPartySize:         20,
		DefaultDuration:      120,
		SlotIntervalMinutes:  30,
		ServiceBufferMinutes: 15,
		AutoAssignTables:     true,
		ApprovalMode:         database.ReservationApprovalAuto,
		AllowWaitlist:        true,
		ExternalPartnerLinks: database.JSONRawMessage("[]"),
	}).Error)

	reservationTime := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Hour).Format(time.RFC3339)

	post := func(t *testing.T, email string) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(map[string]interface{}{
			"customer_name":    "Guest Booker",
			"customer_email":   email,
			"party_size":       2,
			"reservation_time": reservationTime,
		})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		CreatePublicReservation(c)
		return w
	}

	countRows := func(t *testing.T) int64 {
		t.Helper()
		var count int64
		require.NoError(t, database.GetDB().Model(&database.TableReservation{}).
			Where("business_id = ?", business.ID).Count(&count).Error)
		return count
	}

	for _, tc := range []struct {
		name  string
		email string
	}{
		{"missing", ""},
		{"garbage", "not-an-email"},
		// net/mail.ParseAddress accepts these RFC 5322 mailboxes, but they
		// cannot receive confirmation / approval / reminder mail.
		{"no-dot domain", "john@gmail"},
		{"localhost", "john@localhost"},
		{"single-label", "a@b"},
		{"display-name no-dot", "John <john@gmail>"},
		{"ipv4 literal", "user@[127.0.0.1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := countRows(t)
			w := post(t, tc.email)
			require.Equal(t, http.StatusBadRequest, w.Code,
				"public create must 400 on a missing/invalid customer_email, got %d: %s", w.Code, w.Body.String())
			require.Contains(t, strings.ToLower(w.Body.String()), "email",
				"the 400 body must tell the guest the email is the problem")
			require.EqualValues(t, before, countRows(t),
				"invalid guest email must not persist a reservation row")
		})
	}

	t.Run("valid email is not rejected on email grounds", func(t *testing.T) {
		before := countRows(t)
		w := post(t, "guest@example.com")
		// The booking may still fail later for non-email reasons (availability,
		// operating hours, etc.). What must never happen is a 400 that blames
		// the email.
		if w.Code == http.StatusBadRequest {
			require.NotContains(t, strings.ToLower(w.Body.String()), "email",
				"a valid email must not trigger the email-validation 400")
		}
		if w.Code != http.StatusCreated && w.Code != http.StatusOK {
			require.EqualValues(t, before, countRows(t),
				"a later non-email failure must not persist a reservation")
		}
	})
}

// TestCreateReservationStaffEmailOptional locks the authenticated walk-in
// path: phone/door bookings stay email-optional even after the public
// guest gate started requiring a deliverable mailbox.
func TestCreateReservationStaffEmailOptional(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{},
		&database.TableReservation{},
		&database.ReservationSettings{},
		&database.ReservationStatusHistory{},
		&database.BusinessOperatingHours{},
		&database.Bill{},
	))

	business := createBusinessHandlerTestBusiness(t, "0xOwnerWalkInEmail", "biz-reservation-staff-email-opt")
	for day := 0; day < 7; day++ {
		require.NoError(t, database.GetDB().Create(&database.BusinessOperatingHours{
			BusinessID: business.ID,
			DayOfWeek:  day,
			OpenTime:   "00:00",
			CloseTime:  "23:59",
		}).Error)
	}
	require.NoError(t, database.GetDB().Create(&database.Table{
		BusinessID:   business.ID,
		TableCode:    "T1",
		Name:         "T1",
		Capacity:     4,
		IsActive:     true,
		IsReservable: true,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.ReservationSettings{
		BusinessID:           business.ID,
		Enabled:              true,
		MaxAdvanceDays:       30,
		MinAdvanceMinutes:    30,
		MinPartySize:         1,
		MaxPartySize:         20,
		DefaultDuration:      120,
		SlotIntervalMinutes:  30,
		ServiceBufferMinutes: 15,
		AutoAssignTables:     true,
		ApprovalMode:         database.ReservationApprovalAuto,
		AllowWaitlist:        true,
		ExternalPartnerLinks: database.JSONRawMessage("[]"),
	}).Error)

	now := time.Now().UTC().Add(48 * time.Hour)
	reservationTime := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, time.UTC).Format(time.RFC3339)
	body, err := json.Marshal(map[string]interface{}{
		"customer_name":    "Walk-in Guest",
		"party_size":       2,
		"reservation_time": reservationTime,
		"source":           "staff",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CreateReservation(c)

	if w.Code == http.StatusBadRequest {
		require.NotContains(t, strings.ToLower(w.Body.String()), "email",
			"staff create must stay email-optional; got %s", w.Body.String())
	}
	require.Equal(t, http.StatusCreated, w.Code,
		"staff walk-in without email must still create, got %d: %s", w.Code, w.Body.String())

	var stored database.TableReservation
	require.NoError(t, database.GetDB().Where("business_id = ?", business.ID).First(&stored).Error)
	require.Empty(t, stored.CustomerEmail, "staff create with omitted email must persist an empty mailbox")
}

// TestCreatePublicReservation_ClosedBusinessIsUnavailable guards the
// public booking lock: an administrator-closed business (is_active=true,
// closed_at set) still resolves by custom URL but must not accept online
// reservations. Staff reservation APIs already return 403 via
// RequireOperationalBusiness; guest orders/delivery gate IsBusinessOperational.
func TestCreatePublicReservation_ClosedBusinessIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	business := createPublicBusinessRouteTestBusiness(t, "closed-res-biz", true, true)
	closedAt := time.Now().Add(-time.Hour)
	business.ClosedAt = &closedAt
	require.NoError(t, database.GetDB().Save(business).Error)
	// Avoid a stale active snapshot if another test primed this custom URL.
	services.InvalidateBusinessCustomURL(business.CustomURL)

	require.NoError(t, database.GetDB().Create(&database.ReservationSettings{
		BusinessID:           business.ID,
		Enabled:              true,
		MaxAdvanceDays:       30,
		MinAdvanceMinutes:    30,
		MinPartySize:         1,
		MaxPartySize:         20,
		DefaultDuration:      120,
		SlotIntervalMinutes:  30,
		ServiceBufferMinutes: 15,
		AutoAssignTables:     true,
		ApprovalMode:         database.ReservationApprovalAuto,
		AllowWaitlist:        true,
		ExternalPartnerLinks: database.JSONRawMessage("[]"),
	}).Error)

	reservationTime := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Hour).Format(time.RFC3339)
	body, err := json.Marshal(map[string]interface{}{
		"customer_name":    "Guest Booker",
		"customer_email":   "guest@example.com",
		"party_size":       2,
		"reservation_time": reservationTime,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CreatePublicReservation(c)

	require.Equal(t, http.StatusForbidden, w.Code,
		"closed business must 403 public create, got %d: %s", w.Code, w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "business_unavailable", resp["code"])
	require.Contains(t, strings.ToLower(fmt.Sprint(resp["error"])), "reservation")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.TableReservation{}).
		Where("business_id = ?", business.ID).Count(&count).Error)
	require.EqualValues(t, 0, count, "no reservation row must be persisted for a gated business")
}

func TestCreatePublicReservation_RejectsGuestDuration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, _ := seedPublicReservationVenueWithAssignedTable(t, "guest-duration-reject")

	now := time.Now().UTC().Add(48 * time.Hour)
	reservationTime := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, time.UTC).Format(time.RFC3339)
	body, err := json.Marshal(map[string]interface{}{
		"customer_name":    "Guest Duration Probe",
		"customer_email":   "guest-duration@example.com",
		"party_size":       2,
		"reservation_time": reservationTime,
		"duration":         100000,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	CreatePublicReservation(c)

	require.Equal(t, http.StatusBadRequest, w.Code,
		"guest-supplied duration must 400 without creating, got %d: %s", w.Code, w.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "duration_not_allowed", resp["code"])
	require.Contains(t, strings.ToLower(fmt.Sprint(resp["error"])), "duration")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.TableReservation{}).
		Where("business_id = ?", business.ID).Count(&count).Error)
	require.EqualValues(t, 0, count, "guest duration probe must not persist a reservation")
}

func TestGetPublicReservationSettings_ClosedBusinessDisablesEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	business := createPublicBusinessRouteTestBusiness(t, "closed-res-settings", true, true)
	closedAt := time.Now().Add(-time.Hour)
	business.ClosedAt = &closedAt
	require.NoError(t, database.GetDB().Save(business).Error)
	services.InvalidateBusinessCustomURL(business.CustomURL)

	require.NoError(t, database.GetDB().Create(&database.ReservationSettings{
		BusinessID:           business.ID,
		Enabled:              true,
		MaxAdvanceDays:       30,
		MinAdvanceMinutes:    30,
		MinPartySize:         1,
		MaxPartySize:         20,
		DefaultDuration:      120,
		SlotIntervalMinutes:  30,
		ServiceBufferMinutes: 15,
		AutoAssignTables:     true,
		ApprovalMode:         database.ReservationApprovalAuto,
		AllowWaitlist:        true,
		ExternalPartnerLinks: database.JSONRawMessage("[]"),
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	GetPublicReservationSettings(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, false, resp["enabled"], "closed venue must not advertise bookable reservations")
	_, hasNext := resp["next_available_slot"]
	require.False(t, hasNext, "closed settings must omit next_available_slot")

	avail := httptest.NewRecorder()
	ac, _ := gin.CreateTestContext(avail)
	ac.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	ac.Request = httptest.NewRequest(http.MethodGet, "/?date=2026-07-03&party_size=2", nil)
	GetReservationAvailability(ac)
	require.Equal(t, http.StatusForbidden, avail.Code, avail.Body.String())
}

func getPublicReservationSettingsJSON(t *testing.T, customURL string) (int, map[string]interface{}) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: customURL}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	GetPublicReservationSettings(c)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp
}

func TestGetPublicReservationSettings_ReactivationRestoresEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	business := createPublicBusinessRouteTestBusiness(t, "reactivate-res-settings", true, true)
	closedAt := time.Now().Add(-time.Hour)
	business.ClosedAt = &closedAt
	require.NoError(t, database.GetDB().Save(business).Error)
	services.InvalidateBusinessCustomURL(business.CustomURL)

	require.NoError(t, database.GetDB().Create(&database.ReservationSettings{
		BusinessID:           business.ID,
		Enabled:              true,
		MaxAdvanceDays:       30,
		MinAdvanceMinutes:    30,
		MinPartySize:         1,
		MaxPartySize:         20,
		DefaultDuration:      120,
		SlotIntervalMinutes:  30,
		ServiceBufferMinutes: 15,
		AutoAssignTables:     true,
		ApprovalMode:         database.ReservationApprovalAuto,
		AllowWaitlist:        true,
		ExternalPartnerLinks: database.JSONRawMessage("[]"),
	}).Error)

	code, resp := getPublicReservationSettingsJSON(t, business.CustomURL)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, false, resp["enabled"])

	require.NoError(t, database.GetDB().Model(business).Update("closed_at", nil).Error)
	services.InvalidateBusinessCustomURL(business.CustomURL)

	code, resp = getPublicReservationSettingsJSON(t, business.CustomURL)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, true, resp["enabled"], "reopening without republishing must restore bookable settings")
}
