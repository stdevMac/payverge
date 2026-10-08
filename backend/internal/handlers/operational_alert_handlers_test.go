package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
)

func setupAlertHandlerTest(t *testing.T) (*gorm.DB, *database.Business, *database.Staff, *OperationalAlertHandlers) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.User{},
		&database.Staff{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
		&database.BusinessAlertSettings{},
	))
	business := &database.Business{
		BusinessId:     "ops-alert-handler",
		Name:           "Ops Alert Handler",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
	}
	require.NoError(t, db.Create(business).Error)
	staff := &database.Staff{
		BusinessID: business.ID,
		Name:       "Pedro",
		Email:      "pedro@example.com",
		Role:       database.StaffRoleServer,
		IsActive:   true,
	}
	require.NoError(t, db.Create(staff).Error)
	return db, business, staff, NewOperationalAlertHandlers(db)
}

func TestListAlertsReturnsItems(t *testing.T) {
	db, business, staff, h := setupAlertHandlerTest(t)
	svc := alerts.NewService(db)
	_, err := svc.UpsertAlert(t.Context(), alerts.UpsertAlertInput{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeOrderNew,
		ResourceType: database.OperationalAlertResourceTypeOrder,
		ResourceID:   1,
		Title:        "New order",
	})
	require.NoError(t, err)

	r := gin.New()
	r.GET("/businesses/:id/alerts", func(c *gin.Context) {
		c.Set("staff_id", staff.ID)
		c.Set("staff_business_id", business.ID)
		h.List(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/businesses/"+stringFromUint(business.ID)+"/alerts", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Alerts []database.OperationalAlert `json:"alerts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Alerts, 1)
}

func TestListAlertsRecentIncludesTerminalAndIsBounded(t *testing.T) {
	db, business, staff, h := setupAlertHandlerTest(t)
	now := time.Now()

	// 120 resolved alerts: 110 inside the 7-day window, 10 older than 7 days.
	for i := 0; i < 120; i++ {
		lastEvent := now.Add(-time.Duration(i) * time.Hour)
		if i >= 110 {
			lastEvent = now.AddDate(0, 0, -8).Add(-time.Duration(i) * time.Hour)
		}
		resolvedAt := lastEvent
		require.NoError(t, db.Create(&database.OperationalAlert{
			BusinessID:   business.ID,
			AlertType:    database.OperationalAlertTypeOrderNew,
			ResourceType: database.OperationalAlertResourceTypeOrder,
			ResourceID:   int64(i + 1),
			Status:       database.OperationalAlertStatusResolved,
			Title:        "Resolved",
			ResolvedAt:   &resolvedAt,
			LastEventAt:  lastEvent,
		}).Error)
	}
	// 5 open alerts inside the window.
	for i := 0; i < 5; i++ {
		require.NoError(t, db.Create(&database.OperationalAlert{
			BusinessID:   business.ID,
			AlertType:    database.OperationalAlertTypeOrderNew,
			ResourceType: database.OperationalAlertResourceTypeOrder,
			ResourceID:   int64(1000 + i),
			Status:       database.OperationalAlertStatusOpen,
			Title:        "Open",
			LastEventAt:  now.Add(-time.Duration(i) * time.Minute),
		}).Error)
	}

	r := gin.New()
	r.GET("/businesses/:id/alerts", func(c *gin.Context) {
		c.Set("staff_id", staff.ID)
		c.Set("staff_business_id", business.ID)
		h.List(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/businesses/"+stringFromUint(business.ID)+"/alerts?status=resolved,dismissed,open,claimed&recent=1", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Alerts []database.OperationalAlert `json:"alerts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Alerts, 100, "recent listing must be bounded to 100 rows")

	cutoff := now.AddDate(0, 0, -7)
	statuses := map[database.OperationalAlertStatus]int{}
	for i, alert := range body.Alerts {
		statuses[alert.Status]++
		require.False(t, alert.LastEventAt.Before(cutoff), "alert %d older than 7 days: %s", alert.ID, alert.LastEventAt)
		if i > 0 {
			require.False(t, body.Alerts[i-1].LastEventAt.Before(alert.LastEventAt), "alerts must be ordered last_event_at DESC")
		}
	}
	require.Equal(t, 5, statuses[database.OperationalAlertStatusOpen])
	require.Equal(t, 95, statuses[database.OperationalAlertStatusResolved], "terminal statuses must be included in recent mode")
}

func TestClaimAlertUsesAuthenticatedStaffName(t *testing.T) {
	db, business, staff, h := setupAlertHandlerTest(t)
	svc := alerts.NewService(db)
	alert, err := svc.UpsertAlert(t.Context(), alerts.UpsertAlertInput{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeOrderNew,
		ResourceType: database.OperationalAlertResourceTypeOrder,
		ResourceID:   1,
		Title:        "New order",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/claim", bytes.NewBufferString(`{"source":"row_click"}`))
	c.Params = gin.Params{
		{Key: "id", Value: stringFromUint(business.ID)},
		{Key: "alertId", Value: stringFromUint(alert.ID)},
	}
	c.Set("staff_id", staff.ID)
	c.Set("staff_business_id", business.ID)

	h.Claim(c)

	require.Equal(t, http.StatusOK, w.Code)
	var response database.OperationalAlert
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, database.OperationalAlertStatusClaimed, response.Status)
	require.Equal(t, "Pedro", response.ClaimedByName)
}

func TestClaimAlertRoutesToNamedStaff(t *testing.T) {
	db, business, staff, h := setupAlertHandlerTest(t)
	assignee := &database.Staff{
		BusinessID: business.ID,
		Name:       "Dana",
		Email:      "dana@example.com",
		Role:       database.StaffRoleServer,
		IsActive:   true,
		InvitedBy:  "owner",
	}
	require.NoError(t, db.Create(assignee).Error)
	svc := alerts.NewService(db)
	alert, err := svc.UpsertAlert(t.Context(), alerts.UpsertAlertInput{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeServiceCall,
		ResourceType: database.OperationalAlertResourceTypeTable,
		ResourceID:   2,
		Title:        "Service call",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"source":"tables_queue","staff_id":%d}`, assignee.ID)
	c.Request = httptest.NewRequest(http.MethodPost, "/claim", bytes.NewBufferString(body))
	c.Params = gin.Params{
		{Key: "id", Value: stringFromUint(business.ID)},
		{Key: "alertId", Value: stringFromUint(alert.ID)},
	}
	c.Set("staff_id", staff.ID)
	c.Set("staff_business_id", business.ID)

	h.Claim(c)

	require.Equal(t, http.StatusOK, w.Code)
	var response database.OperationalAlert
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, database.OperationalAlertStatusClaimed, response.Status)
	require.Equal(t, "Dana", response.ClaimedByName)
	require.NotNil(t, response.ClaimedByStaffID)
	require.Equal(t, assignee.ID, *response.ClaimedByStaffID)
}

func TestClaimAlertRejectsStaffFromAnotherBusiness(t *testing.T) {
	db, business, staff, h := setupAlertHandlerTest(t)
	other := &database.Business{
		BusinessId:     "ops-alert-staff-other",
		Name:           "Other",
		OwnerAddress:   "0xother",
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
	}
	require.NoError(t, db.Create(other).Error)
	foreign := &database.Staff{
		BusinessID: other.ID,
		Name:       "Foreign",
		Email:      "foreign@example.com",
		Role:       database.StaffRoleServer,
		IsActive:   true,
		InvitedBy:  "owner",
	}
	require.NoError(t, db.Create(foreign).Error)
	svc := alerts.NewService(db)
	alert, err := svc.UpsertAlert(t.Context(), alerts.UpsertAlertInput{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeServiceCall,
		ResourceType: database.OperationalAlertResourceTypeTable,
		ResourceID:   2,
		Title:        "Service call",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"source":"tables_queue","staff_id":%d}`, foreign.ID)
	c.Request = httptest.NewRequest(http.MethodPost, "/claim", bytes.NewBufferString(body))
	c.Params = gin.Params{
		{Key: "id", Value: stringFromUint(business.ID)},
		{Key: "alertId", Value: stringFromUint(alert.ID)},
	}
	c.Set("staff_id", staff.ID)
	c.Set("staff_business_id", business.ID)

	h.Claim(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var reloaded database.OperationalAlert
	require.NoError(t, db.First(&reloaded, alert.ID).Error)
	require.Equal(t, database.OperationalAlertStatusOpen, reloaded.Status)
}

func TestClaimAlertRejectsAlertFromAnotherBusiness(t *testing.T) {
	db, business, staff, h := setupAlertHandlerTest(t)
	other := &database.Business{
		BusinessId:     "ops-alert-other",
		Name:           "Other",
		OwnerAddress:   "0xother",
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
	}
	require.NoError(t, db.Create(other).Error)
	svc := alerts.NewService(db)
	alert, err := svc.UpsertAlert(t.Context(), alerts.UpsertAlertInput{
		BusinessID:   other.ID,
		AlertType:    database.OperationalAlertTypeOrderNew,
		ResourceType: database.OperationalAlertResourceTypeOrder,
		ResourceID:   1,
		Title:        "Other order",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/claim", bytes.NewBufferString(`{"source":"row_click"}`))
	c.Params = gin.Params{
		{Key: "id", Value: stringFromUint(business.ID)},
		{Key: "alertId", Value: stringFromUint(alert.ID)},
	}
	c.Set("staff_id", staff.ID)
	c.Set("staff_business_id", business.ID)

	h.Claim(c)

	require.Equal(t, http.StatusNotFound, w.Code)
	var reloaded database.OperationalAlert
	require.NoError(t, db.First(&reloaded, alert.ID).Error)
	require.Equal(t, database.OperationalAlertStatusOpen, reloaded.Status)
}

func stringFromUint(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}

// Fix 1: claiming an alert someone else already holds must surface a 409 with
// the current holder's name so the FE can show "already claimed by X".
func TestClaimAlertClaimedByOtherReturns409WithHolder(t *testing.T) {
	db, business, staff, h := setupAlertHandlerTest(t)
	first := &database.Staff{
		BusinessID: business.ID,
		Name:       "Sofia",
		Email:      "sofia@example.com",
		Role:       database.StaffRoleServer,
		IsActive:   true,
	}
	require.NoError(t, db.Create(first).Error)

	svc := alerts.NewService(db)
	alert, err := svc.UpsertAlert(t.Context(), alerts.UpsertAlertInput{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeOrderNew,
		ResourceType: database.OperationalAlertResourceTypeOrder,
		ResourceID:   7,
		Title:        "New order",
	})
	require.NoError(t, err)
	_, err = svc.ClaimAlertForBusiness(t.Context(), business.ID, alert.ID, alerts.Actor{StaffID: &first.ID, Name: first.Name}, "row_click")
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/claim", bytes.NewBufferString(`{"source":"row_click"}`))
	c.Params = gin.Params{
		{Key: "id", Value: stringFromUint(business.ID)},
		{Key: "alertId", Value: stringFromUint(alert.ID)},
	}
	c.Set("staff_id", staff.ID)
	c.Set("staff_business_id", business.ID)

	h.Claim(c)

	require.Equal(t, http.StatusConflict, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "Sofia", body["claimed_by_name"])

	// The steal attempt must not have flipped attribution.
	var reloaded database.OperationalAlert
	require.NoError(t, db.First(&reloaded, alert.ID).Error)
	require.Equal(t, "Sofia", reloaded.ClaimedByName)
}

// Fix 1: resolving an already-resolved alert stays a 200 (idempotent), and a
// missing alert stays a 404 — 409 is reserved for real state conflicts.
func TestResolveAlertIdempotentAndClaimResolvedConflicts(t *testing.T) {
	db, business, staff, h := setupAlertHandlerTest(t)
	svc := alerts.NewService(db)
	alert, err := svc.UpsertAlert(t.Context(), alerts.UpsertAlertInput{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeOrderNew,
		ResourceType: database.OperationalAlertResourceTypeOrder,
		ResourceID:   8,
		Title:        "New order",
	})
	require.NoError(t, err)
	_, err = svc.ResolveAlertByIDForBusiness(t.Context(), business.ID, alert.ID, alerts.Actor{StaffID: &staff.ID, Name: staff.Name}, "done")
	require.NoError(t, err)

	// Second resolve → 200.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/resolve", bytes.NewBufferString(`{"reason":"done"}`))
	c.Params = gin.Params{
		{Key: "id", Value: stringFromUint(business.ID)},
		{Key: "alertId", Value: stringFromUint(alert.ID)},
	}
	c.Set("staff_id", staff.ID)
	c.Set("staff_business_id", business.ID)
	h.Resolve(c)
	require.Equal(t, http.StatusOK, w.Code)

	// Claiming the resolved alert → 409.
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/claim", bytes.NewBufferString(`{"source":"row_click"}`))
	c.Params = gin.Params{
		{Key: "id", Value: stringFromUint(business.ID)},
		{Key: "alertId", Value: stringFromUint(alert.ID)},
	}
	c.Set("staff_id", staff.ID)
	c.Set("staff_business_id", business.ID)
	h.Claim(c)
	require.Equal(t, http.StatusConflict, w.Code)
}
