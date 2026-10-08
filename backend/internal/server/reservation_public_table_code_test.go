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

const guestTableQRCredential = "QRACCESS531SECRET"
const guestTableQRPayload = "qr_payload_531_secret"

// assertPublicJSONOmitsGuestTableCredential is the #531 contract: unauthenticated
// reservation JSON must never include the table QR access code (or any other
// guest-table credential). A leaked table_code is a working /guest/table/:code
// credential.
func assertPublicJSONOmitsGuestTableCredential(t *testing.T, raw string) {
	t.Helper()
	lower := strings.ToLower(raw)
	for _, leak := range []string{
		`"table_code"`,
		`"qr_code"`,
		strings.ToLower(guestTableQRCredential),
		strings.ToLower(guestTableQRPayload),
	} {
		if strings.Contains(lower, leak) {
			t.Fatalf("public reservation JSON leaked guest table credential %s: %s", leak, raw)
		}
	}
}

func seedPublicReservationVenueWithAssignedTable(t *testing.T, slug string) (*database.Business, *database.Table) {
	t.Helper()
	setupPublicBusinessHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.ReservationStatusHistory{},
		&database.BusinessOperatingException{},
		&database.Bill{},
	))

	business := createPublicBusinessRouteTestBusiness(t, slug, true, true)
	require.NoError(t, database.GetDB().Save(business).Error)
	services.InvalidateBusinessCustomURL(business.CustomURL)

	for day := 0; day < 7; day++ {
		require.NoError(t, database.GetDB().Create(&database.BusinessOperatingHours{
			BusinessID: business.ID,
			DayOfWeek:  day,
			OpenTime:   "00:00",
			CloseTime:  "23:59",
			IsClosed:   false,
		}).Error)
	}

	table := &database.Table{
		BusinessID:   business.ID,
		TableCode:    guestTableQRCredential,
		QRCode:       guestTableQRPayload,
		Name:         "Patio 12",
		Capacity:     4,
		IsActive:     true,
		IsReservable: true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	require.NoError(t, database.GetDB().Create(&database.ReservationSettings{
		BusinessID:           business.ID,
		Enabled:              true,
		MaxAdvanceDays:       30,
		MinAdvanceMinutes:    0,
		MinPartySize:         1,
		MaxPartySize:         20,
		DefaultDuration:      120,
		SlotIntervalMinutes:  30,
		ServiceBufferMinutes: 0,
		AutoAssignTables:     true,
		ApprovalMode:         database.ReservationApprovalAuto,
		AllowWaitlist:        false,
		AllowCancellation:    true,
		CancellationDeadline: 0,
		ExternalPartnerLinks: database.JSONRawMessage("[]"),
	}).Error)

	return business, table
}

func postPublicReservation(t *testing.T, customURL string) *httptest.ResponseRecorder {
	t.Helper()
	// Midday UTC, ≥48h out. Truncating "now" to the hour made this flake
	// after 22:00 UTC: a 120-minute seating ending at midnight failed
	// reservationFitsOperatingWindow (CI 31975490163).
	now := time.Now().UTC().Add(48 * time.Hour)
	reservationTime := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, time.UTC).Format(time.RFC3339)
	body, err := json.Marshal(map[string]interface{}{
		"customer_name":    "Guest Booker",
		"customer_email":   "guest531@example.com",
		"customer_phone":   "5550000531",
		"party_size":       2,
		"reservation_time": reservationTime,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: customURL}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	CreatePublicReservation(c)
	return w
}

// TestPublicReservationResponsesOmitGuestTableCode is the #531 TDD gate:
// public create / get-by-confirmation-code / cancel must not emit table_code
// (or any guest table QR credential). Operator detail may still include it.
func TestPublicReservationResponsesOmitGuestTableCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, table := seedPublicReservationVenueWithAssignedTable(t, "table-code-leak")

	createW := postPublicReservation(t, business.CustomURL)
	require.Equal(t, http.StatusCreated, createW.Code, createW.Body.String())
	assertPublicJSONOmitsGuestTableCredential(t, createW.Body.String())

	var created struct {
		ID               uint   `json:"id"`
		ConfirmationCode string `json:"confirmation_code"`
		Status           string `json:"status"`
		TableID          *uint  `json:"table_id"`
	}
	require.NoError(t, json.Unmarshal(createW.Body.Bytes(), &created))
	require.NotEmpty(t, created.ConfirmationCode)
	require.Equal(t, "confirmed", created.Status)
	require.NotNil(t, created.TableID)
	require.Equal(t, table.ID, *created.TableID)

	getW := httptest.NewRecorder()
	gc, _ := gin.CreateTestContext(getW)
	gc.Params = gin.Params{{Key: "confirmationCode", Value: created.ConfirmationCode}}
	gc.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	GetPublicReservationByCode(gc)
	require.Equal(t, http.StatusOK, getW.Code, getW.Body.String())
	assertPublicJSONOmitsGuestTableCredential(t, getW.Body.String())

	var details struct {
		Reservation struct {
			TableName string `json:"table_name"`
		} `json:"reservation"`
	}
	require.NoError(t, json.Unmarshal(getW.Body.Bytes(), &details))
	require.Equal(t, table.Name, details.Reservation.TableName)

	cancelW := httptest.NewRecorder()
	cc, _ := gin.CreateTestContext(cancelW)
	cc.Params = gin.Params{{Key: "confirmationCode", Value: created.ConfirmationCode}}
	cc.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	CancelPublicReservation(cc)
	require.Equal(t, http.StatusOK, cancelW.Code, cancelW.Body.String())
	assertPublicJSONOmitsGuestTableCredential(t, cancelW.Body.String())

	operatorW := httptest.NewRecorder()
	oc, _ := gin.CreateTestContext(operatorW)
	oc.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "reservationId", Value: fmt.Sprintf("%d", created.ID)},
	}
	oc.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	GetReservation(oc)
	require.Equal(t, http.StatusOK, operatorW.Code, operatorW.Body.String())
	require.Contains(t, operatorW.Body.String(), `"table_code"`)
	require.Contains(t, operatorW.Body.String(), guestTableQRCredential)
}
