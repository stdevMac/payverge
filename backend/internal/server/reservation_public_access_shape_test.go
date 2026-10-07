package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// TestPublicReservationGETPathsDoNotWrite is the #558 write-on-read gate:
// GET settings and GET availability must not INSERT/UPDATE, even when the
// stored settings row would trip normalizeReservationSettings (needsUpdate).
func TestPublicReservationGETPathsDoNotWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	business := seedPublicReservationAccessShapeVenue(t, "resv-get-nowrite")

	// Force a needsUpdate row: GORM omits a struct-literal 0 in favor of the
	// column default, so the 0 must be written explicitly.
	require.NoError(t, database.GetDB().Model(&database.ReservationSettings{}).
		Where("business_id = ?", business.ID).
		Update("max_advance_days", 0).Error)

	var persisted database.ReservationSettings
	require.NoError(t, database.GetDB().Where("business_id = ?", business.ID).First(&persisted).Error)
	require.Equal(t, 0, persisted.MaxAdvanceDays)

	writes := trackReservationAccessShapeWrites(t, database.GetDB())

	settingsW := httptest.NewRecorder()
	sc, _ := gin.CreateTestContext(settingsW)
	sc.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	sc.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	GetPublicReservationSettings(sc)
	require.Equal(t, http.StatusOK, settingsW.Code, settingsW.Body.String())

	var settingsResp map[string]interface{}
	require.NoError(t, json.Unmarshal(settingsW.Body.Bytes(), &settingsResp))
	assert.Equal(t, float64(30), settingsResp["max_advance_days"],
		"public settings should still expose the in-memory normalized value")

	date := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	availW := httptest.NewRecorder()
	ac, _ := gin.CreateTestContext(availW)
	ac.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	ac.Request = httptest.NewRequest(http.MethodGet, "/?date="+date+"&party_size=2", nil)
	GetReservationAvailability(ac)
	require.Equal(t, http.StatusOK, availW.Code, availW.Body.String())

	assert.Empty(t, writes.snapshot(), "public reservation GET paths must not INSERT or UPDATE")

	var after database.ReservationSettings
	require.NoError(t, database.GetDB().Where("business_id = ?", business.ID).First(&after).Error)
	assert.Equal(t, 0, after.MaxAdvanceDays, "GET must not persist normalized reservation settings")
}

func seedPublicReservationAccessShapeVenue(t *testing.T, slug string) *database.Business {
	t.Helper()

	business := createPublicBusinessRouteTestBusiness(t, slug, true, true)
	services.InvalidateBusinessCustomURL(business.CustomURL)

	require.NoError(t, database.GetDB().Create(&database.ReservationSettings{
		BusinessID:           business.ID,
		Enabled:              true,
		MaxAdvanceDays:       30,
		MinAdvanceMinutes:    0,
		MinPartySize:         1,
		MaxPartySize:         20,
		DefaultDuration:      60,
		SlotIntervalMinutes:  30,
		ServiceBufferMinutes: 15,
		AutoAssignTables:     true,
		ApprovalMode:         database.ReservationApprovalAuto,
		AllowWaitlist:        true,
		ExternalPartnerLinks: database.JSONRawMessage("[]"),
	}).Error)

	for day := 0; day < 7; day++ {
		require.NoError(t, database.GetDB().Create(&database.BusinessOperatingHours{
			BusinessID: business.ID,
			DayOfWeek:  day,
			OpenTime:   "10:00",
			CloseTime:  "22:00",
			IsClosed:   false,
		}).Error)
	}

	require.NoError(t, database.GetDB().Create(&database.Table{
		BusinessID: business.ID,
		TableCode:  "ACCESS-" + slug,
		Name:       "Access Shape Table",
		Capacity:   4,
		IsActive:   true,
	}).Error)

	return business
}

type reservationAccessShapeWriteLog struct {
	mu      sync.Mutex
	entries []string
}

func (l *reservationAccessShapeWriteLog) add(kind, table string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, kind+" "+table)
}

func (l *reservationAccessShapeWriteLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.entries))
	copy(out, l.entries)
	return out
}

func trackReservationAccessShapeWrites(t *testing.T, gormDB *gorm.DB) *reservationAccessShapeWriteLog {
	t.Helper()

	log := &reservationAccessShapeWriteLog{}
	label := fmt.Sprintf("%d", time.Now().UnixNano())
	createName := "payverge:test_public_resv_fail_create_" + label
	updateName := "payverge:test_public_resv_fail_update_" + label

	require.NoError(t, gormDB.Callback().Create().Before("gorm:create").Register(createName, func(tx *gorm.DB) {
		table := ""
		if tx.Statement != nil {
			table = tx.Statement.Table
		}
		log.add("INSERT", table)
	}))
	require.NoError(t, gormDB.Callback().Update().Before("gorm:update").Register(updateName, func(tx *gorm.DB) {
		table := ""
		if tx.Statement != nil {
			table = tx.Statement.Table
		}
		log.add("UPDATE", table)
	}))
	t.Cleanup(func() {
		_ = gormDB.Callback().Create().Remove(createName)
		_ = gormDB.Callback().Update().Remove(updateName)
	})
	return log
}
