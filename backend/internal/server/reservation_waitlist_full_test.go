package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestCreateReservation_WaitlistFullReturns409 is the staff create handler:
// the 201st waitlist row for one slot is a 409, and the shared writer maps
// the same sentinel for the public guest create handler.
func TestCreateReservation_WaitlistFullReturns409(t *testing.T) {
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

	business := createBusinessHandlerTestBusiness(t, "0xOwnerWaitlistFull", "biz-reservation-waitlist-full")
	business.Timezone = "UTC"
	require.NoError(t, database.GetDB().Save(business).Error)
	for day := 0; day < 7; day++ {
		require.NoError(t, database.GetDB().Create(&database.BusinessOperatingHours{
			BusinessID: business.ID,
			DayOfWeek:  day,
			OpenTime:   "00:00",
			CloseTime:  "23:59",
		}).Error)
	}
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
	slot := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, time.UTC)
	rows := make([]database.TableReservation, 0, database.MaxWaitlistPerSlot)
	for i := 1; i <= database.MaxWaitlistPerSlot; i++ {
		pos := i
		rows = append(rows, database.TableReservation{
			BusinessID:       business.ID,
			CustomerName:     fmt.Sprintf("Wait Guest %d", i),
			PartySize:        2,
			ReservationTime:  slot,
			Duration:         90,
			Status:           "waitlist",
			Source:           "customer",
			ConfirmationCode: fmt.Sprintf("WAIT-FULL-%d", i),
			WaitlistPosition: &pos,
		})
	}
	require.NoError(t, database.GetDB().CreateInBatches(rows, 20).Error)

	body, err := json.Marshal(map[string]interface{}{
		"customer_name":    "One Too Many",
		"party_size":       2,
		"reservation_time": slot.Format(time.RFC3339),
		"source":           "staff",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CreateReservation(c)

	require.Equal(t, http.StatusConflict, w.Code, "body: %s", w.Body.String())
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Equal(t, "reservation_waitlist_full", payload["code"])
	require.Contains(t, payload["error"], "waitlist")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.TableReservation{}).
		Where("business_id = ? AND status = ?", business.ID, "waitlist").
		Count(&count).Error)
	require.Equal(t, int64(database.MaxWaitlistPerSlot), count)
}
