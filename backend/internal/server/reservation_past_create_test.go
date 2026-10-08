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
)

// L1-1 (B-6): the authenticated create must 400 a reservation whose slot is
// entirely in the past. Live audit repro: a create dated 2020 returned 201 and
// the row was then invisible under every horizon filter and uncancellable.
func TestCreateReservationRejectsPastDate(t *testing.T) {
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

	business := createBusinessHandlerTestBusiness(t, "0xOwnerPastDate", "biz-reservation-past-date")
	for day := 0; day < 7; day++ {
		require.NoError(t, database.GetDB().Create(&database.BusinessOperatingHours{
			BusinessID: business.ID,
			DayOfWeek:  day,
			OpenTime:   "00:00",
			CloseTime:  "23:59",
		}).Error)
	}
	require.NoError(t, database.GetDB().Create(&database.Table{
		BusinessID: business.ID,
		TableCode:  "T1",
		Name:       "T1",
		Capacity:   4,
		IsActive:   true,
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

	body, err := json.Marshal(map[string]interface{}{
		"customer_name":    "Time Traveler",
		"party_size":       2,
		"reservation_time": time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Hour).Format(time.RFC3339),
		"source":           "staff",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CreateReservation(c)

	require.Equal(t, http.StatusBadRequest, w.Code,
		"a fully past reservation_time must 400, got %d: %s", w.Code, w.Body.String())
	require.Contains(t, strings.ToLower(w.Body.String()), "past",
		"the 400 body must name the past-date problem")
	// L1-4: coded 400s so operator toasts localize via apiErrors — never bare
	// English-only gin/domain dumps without a stable wire code.
	var errBody map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errBody))
	require.Equal(t, "reservation_in_past", errBody["code"],
		"past-date create must emit reservation_in_past; body=%s", w.Body.String())

	var count int64
	require.NoError(t, database.GetDB().Model(&database.TableReservation{}).
		Where("business_id = ?", business.ID).Count(&count).Error)
	require.Zero(t, count, "no orphan row may be created")
}
