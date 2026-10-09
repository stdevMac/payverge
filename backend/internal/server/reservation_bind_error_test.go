package server

import (
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
)

func TestCheckInReservationBindError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{},
		&database.TableReservation{},
		&database.ReservationStatusHistory{},
		&database.ReservationSettings{},
	))

	business := createBusinessHandlerTestBusiness(t, "0xOwnerCheckIn", "biz-reservation-checkin-bind")
	claimedAt := time.Now().UTC()
	reservation := &database.TableReservation{
		BusinessID:       business.ID,
		CustomerName:     "Claimed Guest",
		PartySize:        2,
		ReservationTime:  time.Now().UTC().Add(48 * time.Hour),
		Duration:         90,
		Status:           "confirmed",
		Source:           "staff",
		ConfirmationCode: "CONF-CHECKIN-BIND",
		ClaimedByName:    "Owner",
		ClaimedByRole:    "owner",
		ClaimedAt:        &claimedAt,
	}
	require.NoError(t, database.GetDB().Create(reservation).Error)

	call := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{
			{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
			{Key: "reservationId", Value: fmt.Sprintf("%d", reservation.ID)},
		}
		c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		CheckInReservation(c)
		return w
	}

	t.Run("malformed json", func(t *testing.T) {
		w := call("{")
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Equal(t, ErrCodeInvalidInput, body["code"])
		require.Equal(t, "Please check the form and try again.", body["error"])
	})

	t.Run("empty body is not a bind error", func(t *testing.T) {
		w := call("")
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.NotEqual(t, ErrCodeInvalidInput, body["code"], "status=%d body=%s", w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), "Please check the form and try again.")
		// Binding succeeded and the service rejected a future check-in.
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Equal(t, "reservation_arrival_window", body["code"], "status=%d body=%s", w.Code, w.Body.String())
	})
}

func TestUpdateReservationSettingsValidationReturnsPlainText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.ReservationSettings{}))
	business := createBusinessHandlerTestBusiness(t, "0xOwnerSettings", "biz-reservation-settings-duration")
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

	w := putReservationSettings(t, business.ID, `{"default_duration":2000}`)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "default_duration must be at most 1440 minutes", body["error"])
	require.NotContains(t, w.Body.String(), ErrCodeInternal)
}

// TestGetReservationLookupErrorMapping pins API-RES-01: a missing reservation is
// 404, but a database failure is a generic 500 that leaks no driver text.
func TestGetReservationLookupErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{},
		&database.TableReservation{},
		&database.ReservationStatusHistory{},
		&database.ReservationSettings{},
	))
	business := createBusinessHandlerTestBusiness(t, "0xOwnerLookup", "biz-reservation-lookup-err")

	call := func(reservationID string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{
			{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
			{Key: "reservationId", Value: reservationID},
		}
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		GetReservation(c)
		return w
	}

	t.Run("missing row is 404", func(t *testing.T) {
		w := call("999999")
		require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	})

	t.Run("database failure is a generic 500", func(t *testing.T) {
		require.NoError(t, gormDB.Migrator().DropTable(&database.TableReservation{}))
		w := call("1")
		require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Equal(t, ErrCodeInternal, body["code"])
		require.NotContains(t, strings.ToLower(w.Body.String()), "table_reservations")
		require.NotContains(t, strings.ToLower(w.Body.String()), "no such table")
	})
}
