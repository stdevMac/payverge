package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestReservationDomainErrorCode_KnownMessages(t *testing.T) {
	cases := map[string]string{
		"selected time is too soon to book":                            "reservation_min_advance",
		"reservations must be made at least 30 minutes in advance":     "reservation_min_advance",
		"reservation time is in the past":                              "reservation_in_past",
		"reservations can only be made up to 30 days in advance":       "reservation_max_advance",
		"business is closed for the selected date":                     "reservation_business_closed",
		"reservation must fall within business operating hours":        "reservation_outside_hours",
		"selected time is too close to closing for a full reservation": "reservation_outside_hours",
		"party size must be between 1 and 20":                          "reservation_party_size",
		"slot capacity has been reached":                               "reservation_covers_limit",
		"capacity has been reached":                                    "reservation_covers_limit",
		"no tables are available for the requested time":               "reservation_no_tables",
		"cancellation window has closed":                               "reservation_cancel_window",
		"this reservation cannot be cancelled online":                  "reservation_cancel_never_open",
		"cannot seat a future-dated reservation":                       "reservation_arrival_window",
		"unsupported reservation transition":                           "reservation_invalid_transition",
		"duration must be greater than zero":                           "reservation_invalid_duration",
		"duration cannot be set for online bookings":                   "duration_not_allowed",
		"invalid reservation_time":                                     "reservation_invalid_time",
	}
	for msg, want := range cases {
		assert.Equal(t, want, reservationDomainErrorCode(msg), "msg=%q", msg)
	}
	assert.Empty(t, reservationDomainErrorCode("completely unknown product message"))
}

func TestRespondReservationTransitionError_EmitsCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/reservations", nil)

	respondReservationTransitionError(c, errors.New("party size must be between 1 and 20"))

	require.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "reservation_party_size", body["code"])
	assert.Contains(t, body["error"], "party size")
}

func TestRespondReservationWriteError_UnfinishedTableService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name string
		err  error
		code string
	}{
		{
			name: "kitchen_tickets_live",
			err:  database.ErrFloorLiveKitchenTickets,
			code: "kitchen_tickets_live",
		},
		{
			name: "orders_pending_approval",
			err:  database.ErrFloorPendingOrders,
			code: "orders_pending_approval",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/tables/1/seat", nil)
			respondReservationWriteError(c, tc.err)
			require.Equal(t, http.StatusConflict, w.Code)
			var body map[string]string
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, tc.code, body["code"])
		})
	}
}

func TestRespondReservationTransitionError_ConflictHasCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/reservations/1", nil)

	respondReservationTransitionError(c, database.ErrReservationStatusConflict)

	require.Equal(t, http.StatusConflict, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "reservation_status_conflict", body["code"])
}
