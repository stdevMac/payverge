package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// The public booking form is the one place an anonymous visitor picks both
// the recipient and the text of an email. Its free-text fields are capped and
// refused with a field-specific 400 before anything is stored or sent.
func TestPublicReservationCreateRejectsOversizedOrControlCharFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	business := createPublicBusinessRouteTestBusiness(t, "relay-caps-biz", true, true)
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

	post := func(t *testing.T, overrides map[string]interface{}) *httptest.ResponseRecorder {
		t.Helper()
		body := map[string]interface{}{
			"customer_name":    "Guest Booker",
			"customer_email":   "guest@example.com",
			"party_size":       2,
			"reservation_time": reservationTime,
		}
		for k, v := range overrides {
			body[k] = v
		}
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
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
		name     string
		override map[string]interface{}
		code     string
		field    string
	}{
		{"name 81 runes", map[string]interface{}{"customer_name": strings.Repeat("é", 81)}, "reservation_field_too_long", "customer_name"},
		{"name with newline", map[string]interface{}{"customer_name": "Ana\nYour account is locked"}, "reservation_field_invalid", "customer_name"},
		{"phone 33", map[string]interface{}{"customer_phone": strings.Repeat("1", 33)}, "reservation_field_too_long", "customer_phone"},
		{"requests 301", map[string]interface{}{"special_requests": strings.Repeat("x", 301)}, "reservation_field_too_long", "special_requests"},
		{"requests bell", map[string]interface{}{"special_requests": "hi\x07"}, "reservation_field_invalid", "special_requests"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := countRows(t)
			w := post(t, tc.override)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			var resp struct {
				Code    string                 `json:"code"`
				Details map[string]interface{} `json:"details"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, tc.code, resp.Code)
			require.Equal(t, tc.field, resp.Details["field"])
			require.EqualValues(t, before, countRows(t), "a refused field must not persist a booking")
		})
	}

	t.Run("at the cap is not refused on field grounds", func(t *testing.T) {
		w := post(t, map[string]interface{}{
			"customer_name":    strings.Repeat("é", 80),
			"customer_phone":   strings.Repeat("1", 32),
			"special_requests": strings.Repeat("x", 299) + "\n",
		})
		require.NotContains(t, w.Body.String(), "reservation_field_", w.Body.String())
	})
}

// Source contract: the public booking handler must send through the
// booking-form mailer (per-recipient + per-business booking caps), never the
// bare global instance.
func TestCreatePublicReservationSendsThroughBookingFormBudget(t *testing.T) {
	src, err := os.ReadFile("reservation_handlers.go")
	require.NoError(t, err)
	body := string(src)
	start := strings.Index(body, "func CreatePublicReservation(")
	require.GreaterOrEqual(t, start, 0)
	end := strings.Index(body[start+1:], "\nfunc ")
	require.Greater(t, end, 0)
	fn := body[start : start+1+end]

	require.Contains(t, fn, "emails.EmailServerInstance.ForReservationRequest(reservation.BusinessID, reservation.ID)")
	direct := regexp.MustCompile(`EmailServerInstance(\.For[A-Za-z]+\([^)]*\))?\.Send[A-Za-z]+\(`).FindAllString(fn, -1)
	require.Empty(t, direct, "guest booking mail must go through guestMailer")
	require.Equal(t, 2, strings.Count(fn, "guestMailer.SendReservation"), "pending + confirmation")
}
