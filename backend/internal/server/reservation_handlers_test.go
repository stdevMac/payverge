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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func TestGetReservationsDefaultsToPaginatedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.TableReservation{}))

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-reservation-pages")
	targetDate := time.Now().UTC().AddDate(0, 0, 1).Truncate(24 * time.Hour)
	for i := 0; i < 25; i++ {
		require.NoError(t, database.GetDB().Create(&database.TableReservation{
			BusinessID:       business.ID,
			CustomerName:     fmt.Sprintf("Guest %02d", i),
			CustomerEmail:    fmt.Sprintf("guest%02d@example.com", i),
			PartySize:        2,
			ReservationTime:  targetDate.Add(time.Duration(i) * time.Minute),
			Duration:         60,
			Status:           "confirmed",
			Source:           "staff",
			ConfirmationCode: fmt.Sprintf("CONF-HANDLER-%02d", i),
		}).Error)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/?start_date=%s&end_date=%s", targetDate.Format("2006-01-02"), targetDate.Format("2006-01-02")),
		nil,
	)

	GetReservations(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Reservations []database.TableReservation `json:"reservations"`
		Total        int64                       `json:"total"`
		Page         int                         `json:"page"`
		PageSize     int                         `json:"page_size"`
		TotalPages   int                         `json:"total_pages"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Reservations, 20)
	assert.Equal(t, int64(25), resp.Total)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 20, resp.PageSize)
	assert.Equal(t, 2, resp.TotalPages)
}

func TestGetReservationTableOptionsReturnsStableContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{},
		&database.TableReservation{},
		&database.ReservationSettings{},
		&database.Bill{},
	))
	business := createBusinessHandlerTestBusiness(t, "0xOwnerOptions", "biz-reservation-options")
	business.Timezone = "America/New_York"
	require.NoError(t, database.GetDB().Save(business).Error)
	require.NoError(t, database.GetDB().Create(&database.ReservationSettings{
		BusinessID:           business.ID,
		Enabled:              true,
		MinPartySize:         1,
		MaxPartySize:         20,
		DefaultDuration:      120,
		ServiceBufferMinutes: 15,
		ApprovalMode:         database.ReservationApprovalAuto,
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/?reservation_time="+time.Now().UTC().Add(time.Hour).Format(time.RFC3339)+"&duration=120&party_size=2",
		nil,
	)

	GetReservationTableOptions(c)

	require.Equal(t, http.StatusOK, w.Code)
	var response struct {
		BusinessTimezone string                               `json:"business_timezone"`
		NearTerm         bool                                 `json:"near_term"`
		Tables           []services.ReservationTableOptionDTO `json:"tables"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "America/New_York", response.BusinessTimezone)
	assert.True(t, response.NearTerm)
	assert.Empty(t, response.Tables)
}

func TestGetReservations_AcceptsBusinessSlug(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.TableReservation{}))

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-reservation-slug")
	targetDate := time.Now().UTC().AddDate(0, 0, 1).Truncate(24 * time.Hour)
	require.NoError(t, database.GetDB().Create(&database.TableReservation{
		BusinessID:       business.ID,
		CustomerName:     "Slug Guest",
		CustomerEmail:    "slug@example.com",
		PartySize:        2,
		ReservationTime:  targetDate,
		Duration:         60,
		Status:           "confirmed",
		Source:           "staff",
		ConfirmationCode: "CONF-SLUG",
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: business.BusinessId}}
	c.Request = httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/?start_date=%s&end_date=%s", targetDate.Format("2006-01-02"), targetDate.Format("2006-01-02")),
		nil,
	)

	GetReservations(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"customer_name":"Slug Guest"`)
}

// R3-RS-1: date-only filters must anchor on the BUSINESS-local calendar day,
// not UTC midnight. West of UTC (here America/Argentina/Buenos_Aires, UTC-3) a
// 21:00-local dinner booking is 00:00 the NEXT day in UTC; parsing "Today" as
// UTC midnight pushed the window three hours early and dropped that booking out
// of the Today view entirely. This proves the 21:00-local booking is included.
func TestGetReservationsAnchorsDateFilterInBusinessTimezone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.TableReservation{}))

	business := createBusinessHandlerTestBusiness(t, "0xOwnerTZ", "biz-reservation-tz")
	// Buenos Aires is UTC-3 year-round.
	business.Timezone = "America/Argentina/Buenos_Aires"
	require.NoError(t, database.GetDB().Save(business).Error)

	loc, err := time.LoadLocation(business.Timezone)
	require.NoError(t, err)

	// A dinner booking at 21:00 LOCAL on 2026-07-03. In UTC that is
	// 2026-07-04T00:00:00Z — the trap the old UTC-midnight parse fell into.
	localDay := "2026-07-03"
	dinnerLocal := time.Date(2026, 7, 3, 21, 0, 0, 0, loc)
	require.NoError(t, database.GetDB().Create(&database.TableReservation{
		BusinessID:       business.ID,
		CustomerName:     "Dinner Guest",
		CustomerEmail:    "dinner@example.com",
		PartySize:        2,
		ReservationTime:  dinnerLocal.UTC(),
		Duration:         60,
		Status:           "confirmed",
		Source:           "staff",
		ConfirmationCode: "CONF-TZ-DINNER",
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/?start_date=%s&end_date=%s", localDay, localDay),
		nil,
	)

	GetReservations(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Reservations []database.TableReservation `json:"reservations"`
		Total        int64                       `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, int64(1), resp.Total, "21:00-local dinner booking must fall in the local Today window")
	require.Len(t, resp.Reservations, 1)
	assert.Equal(t, "Dinner Guest", resp.Reservations[0].CustomerName)
}

func TestCreateReservationRejectsDurationBeyondCap(t *testing.T) {
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

	business := createBusinessHandlerTestBusiness(t, "0xOwnerDuration", "biz-reservation-duration-cap")
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
		MinAdvanceMinutes:    0,
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
		"customer_name":    "Long Stay",
		"party_size":       2,
		"reservation_time": reservationTime,
		"duration":         2000,
		"source":           "staff",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CreateReservation(c)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Contains(t, strings.ToLower(w.Body.String()), "duration must be at most 1440")
	var errBody map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errBody))
	require.Equal(t, "reservation_invalid_duration", errBody["code"])

	var count int64
	require.NoError(t, database.GetDB().Model(&database.TableReservation{}).
		Where("business_id = ?", business.ID).Count(&count).Error)
	require.Zero(t, count)
}

func TestGetReservationStatsAnchorsDateFilterInBusinessTimezone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.TableReservation{}))

	business := createBusinessHandlerTestBusiness(t, "0xOwnerStatsTZ", "biz-reservation-stats-tz")
	business.Timezone = "America/Argentina/Buenos_Aires"
	require.NoError(t, database.GetDB().Save(business).Error)

	loc, err := time.LoadLocation(business.Timezone)
	require.NoError(t, err)

	localDay := "2026-07-03"
	dinnerLocal := time.Date(2026, 7, 3, 21, 0, 0, 0, loc)
	require.NoError(t, database.GetDB().Create(&database.TableReservation{
		BusinessID:       business.ID,
		CustomerName:     "Dinner Guest",
		CustomerEmail:    "dinner-stats@example.com",
		PartySize:        2,
		ReservationTime:  dinnerLocal.UTC(),
		Duration:         60,
		Status:           "confirmed",
		Source:           "staff",
		ConfirmationCode: "CONF-TZ-STATS",
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/?start_date=%s&end_date=%s", localDay, localDay),
		nil,
	)

	GetReservationStats(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Total float64 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp.Total, "21:00-local dinner booking must count in the local day stats window")
}

func TestGetReservationStatsDefaultsToTodayForward(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.TableReservation{}))

	business := createBusinessHandlerTestBusiness(t, "0xOwnerStatsDefault", "biz-reservation-stats-default")
	business.Timezone = "America/Argentina/Buenos_Aires"
	require.NoError(t, database.GetDB().Save(business).Error)

	loc, err := time.LoadLocation(business.Timezone)
	require.NoError(t, err)
	nowLocal := time.Now().In(loc)
	todayNoon := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 12, 0, 0, 0, loc)

	seed := func(name, email, code string, when time.Time) {
		t.Helper()
		require.NoError(t, database.GetDB().Create(&database.TableReservation{
			BusinessID:       business.ID,
			CustomerName:     name,
			CustomerEmail:    email,
			PartySize:        2,
			ReservationTime:  when.UTC(),
			Duration:         60,
			Status:           "confirmed",
			Source:           "staff",
			ConfirmationCode: code,
		}).Error)
	}

	// Old default start=today-30d would count both past rows; new default must not.
	seed("Past Ten", "past10@example.com", "CONF-STATS-PAST-10", todayNoon.AddDate(0, 0, -10))
	seed("Past Forty", "past40@example.com", "CONF-STATS-PAST-40", todayNoon.AddDate(0, 0, -40))
	seed("Today Guest", "today@example.com", "CONF-STATS-TODAY", todayNoon.Add(9*time.Hour))
	// Old default end=today would drop this; new default must include the forward horizon.
	seed("Future Guest", "future@example.com", "CONF-STATS-FUTURE", todayNoon.AddDate(0, 0, 60))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetReservationStats(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Total float64 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(2), resp.Total, "default stats window must be today-forward (exclude past 10/40d, include today + 60d)")
}

func TestGetReservationsRejectsUnknownStatusFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-reservation-status-filter")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/?status=confirmed,hacked", nil)

	GetReservations(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid reservation status: hacked")
}
