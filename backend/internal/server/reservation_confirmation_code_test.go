package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func seedPublicReservationForConfirmationCode(t *testing.T, slug, code string) *database.TableReservation {
	t.Helper()
	business, _ := seedPublicReservationVenueWithAssignedTable(t, slug)

	res := &database.TableReservation{
		BusinessID:       business.ID,
		CustomerName:     "Code Guest",
		CustomerEmail:    "code-guest@example.com",
		CustomerPhone:    "5550000539",
		PartySize:        2,
		ReservationTime:  time.Now().UTC().Add(48 * time.Hour),
		Duration:         90,
		Status:           "confirmed",
		Source:           "customer",
		ConfirmationCode: code,
		CreatedBy:        "customer",
	}
	require.NoError(t, database.GetDB().Create(res).Error)
	return res
}

func getPublicReservationByCode(code string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "confirmationCode", Value: code}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	GetPublicReservationByCode(c)
	return w
}

func cancelPublicReservationByCode(code string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "confirmationCode", Value: code}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	CancelPublicReservation(c)
	return w
}

func assertReservationNotFoundJSON(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "Reservation not found", body["error"])
	require.Len(t, body, 1, "unknown-code 404 must stay a generic single-field error")
}

func TestGetPublicReservationByCodeAcceptsCaseVariants(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stored := seedPublicReservationForConfirmationCode(t, "res-code-get", "ABC123DEF456")

	for _, code := range []string{
		"ABC123DEF456",
		"abc123def456",
		"AbC123dEf456",
		"  ABC123DEF456  ",
		"  abc123def456  ",
	} {
		w := getPublicReservationByCode(code)
		require.Equal(t, http.StatusOK, w.Code, "code %q: %s", code, w.Body.String())

		var details struct {
			Reservation struct {
				ID               uint   `json:"id"`
				ConfirmationCode string `json:"confirmation_code"`
			} `json:"reservation"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &details), "code %q", code)
		require.Equal(t, stored.ID, details.Reservation.ID, "code %q", code)
		require.Equal(t, "ABC123DEF456", details.Reservation.ConfirmationCode, "code %q", code)
	}
}

func TestCancelPublicReservationAcceptsCaseVariants(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stored := seedPublicReservationForConfirmationCode(t, "res-code-cancel", "C0DE539ABCDE")

	w := cancelPublicReservationByCode("  c0de539abcde  ")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "Reservation cancelled successfully")

	var body struct {
		Reservation struct {
			ID     uint   `json:"id"`
			Status string `json:"status"`
		} `json:"reservation"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, stored.ID, body.Reservation.ID)
	require.Equal(t, "cancelled", body.Reservation.Status)

	already := cancelPublicReservationByCode("c0de539abcde")
	require.Equal(t, http.StatusOK, already.Code, already.Body.String())
	require.Contains(t, already.Body.String(), "Reservation was already cancelled")
}

func TestGetPublicReservationByCodeUnknownIsGeneric404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)

	for _, code := range []string{"NOPE00000000", "nope00000000", "   "} {
		assertReservationNotFoundJSON(t, getPublicReservationByCode(code))
	}
}

func TestCancelPublicReservationUnknownIsGeneric404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)

	for _, code := range []string{"NOPE00000000", "nope00000000", strings.Repeat("z", 12)} {
		assertReservationNotFoundJSON(t, cancelPublicReservationByCode(code))
	}
}
