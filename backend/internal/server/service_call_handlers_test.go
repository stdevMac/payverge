package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupServiceCallTestDB prepares an in-memory sqlite DB with the models the
// guest service-call endpoints touch: table lookup + operational alerts.
func setupServiceCallTestDB(t *testing.T) {
	t.Helper()

	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
	))
}

func createServiceCallTable(t *testing.T, businessID uint, name, code string) *database.Table {
	t.Helper()

	table := &database.Table{
		BusinessID: businessID,
		Name:       name,
		TableCode:  strings.ToUpper(code),
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)
	return table
}

func postServiceCall(t *testing.T, code string, body string) *httptest.ResponseRecorder {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: code}}
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/service-call", code), bytes.NewReader([]byte(body)))
	c.Request.Header.Set("Content-Type", "application/json")

	CreateServiceCallByTableCode(c)
	return w
}

func getServiceCallStatus(t *testing.T, code string) *httptest.ResponseRecorder {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: code}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s/service-call", code), nil)

	GetServiceCallStatusByTableCode(c)
	return w
}

func decodeStatus(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()

	var body struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Status
}

func decodeReason(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()

	var body struct {
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Reason
}

func serviceCallAlertCount(t *testing.T, tableID uint) int64 {
	t.Helper()

	var count int64
	require.NoError(t, database.GetDB().Model(&database.OperationalAlert{}).
		Where("alert_type = ? AND resource_type = ? AND resource_id = ?",
			database.OperationalAlertTypeServiceCall,
			database.OperationalAlertResourceTypeTable,
			tableID).
		Count(&count).Error)
	return count
}

func TestCreateServiceCallByTableCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupServiceCallTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerSC1", "Service Call Biz 1")
	table := createServiceCallTable(t, business.ID, "T1", "svc-call-1")

	w := postServiceCall(t, table.TableCode, `{"reason":"water"}`)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "open", decodeStatus(t, w))
	assert.Equal(t, "water", decodeReason(t, w))

	var alert database.OperationalAlert
	require.NoError(t, database.GetDB().
		Where("alert_type = ? AND resource_type = ? AND resource_id = ?",
			database.OperationalAlertTypeServiceCall,
			database.OperationalAlertResourceTypeTable,
			table.ID).
		First(&alert).Error)
	assert.Equal(t, business.ID, alert.BusinessID)
	assert.Equal(t, database.OperationalAlertStatusOpen, alert.Status)
	assert.Equal(t, database.OperationalAlertPriorityUrgent, alert.Priority)
	assert.Contains(t, alert.Body, "water")
}

func TestCreateServiceCallRejectsBadReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupServiceCallTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerSC2", "Service Call Biz 2")
	table := createServiceCallTable(t, business.ID, "T2", "svc-call-2")

	for _, body := range []string{
		`{"reason":"<script>"}`,
		`{"reason":""}`,
		`{}`,
		`not json`,
	} {
		w := postServiceCall(t, table.TableCode, body)
		assert.Equal(t, http.StatusBadRequest, w.Code, "body: %s", body)
	}

	assert.EqualValues(t, 0, serviceCallAlertCount(t, table.ID))
}

func TestCreateServiceCallIdempotentWhileOpen(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupServiceCallTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerSC3", "Service Call Biz 3")
	table := createServiceCallTable(t, business.ID, "T3", "svc-call-3")

	first := postServiceCall(t, table.TableCode, `{"reason":"order"}`)
	require.Equal(t, http.StatusCreated, first.Code)

	second := postServiceCall(t, table.TableCode, `{"reason":"check"}`)
	assert.Equal(t, http.StatusOK, second.Code)
	assert.Equal(t, "open", decodeStatus(t, second))
	assert.Equal(t, "check", decodeReason(t, second))

	assert.EqualValues(t, 1, serviceCallAlertCount(t, table.ID))

	var alert database.OperationalAlert
	require.NoError(t, database.GetDB().
		Where("alert_type = ? AND resource_type = ? AND resource_id = ?",
			database.OperationalAlertTypeServiceCall,
			database.OperationalAlertResourceTypeTable,
			table.ID).
		First(&alert).Error)
	assert.Contains(t, string(alert.Metadata), `"reason":"check"`)
	assert.Contains(t, alert.Body, "check")
}

func TestCreateServiceCallCooldownAfterResolve(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupServiceCallTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerSC4", "Service Call Biz 4")
	table := createServiceCallTable(t, business.ID, "T4", "svc-call-4")

	first := postServiceCall(t, table.TableCode, `{"reason":"check"}`)
	require.Equal(t, http.StatusCreated, first.Code)

	// Resolve just now → cooldown blocks a fresh call.
	now := time.Now()
	require.NoError(t, database.GetDB().Model(&database.OperationalAlert{}).
		Where("alert_type = ? AND resource_id = ?", database.OperationalAlertTypeServiceCall, table.ID).
		Updates(map[string]any{
			"status":      database.OperationalAlertStatusResolved,
			"resolved_at": now,
		}).Error)

	blocked := postServiceCall(t, table.TableCode, `{"reason":"water"}`)
	assert.Equal(t, http.StatusTooManyRequests, blocked.Code)
	retryAfter, err := strconv.Atoi(blocked.Header().Get("Retry-After"))
	require.NoError(t, err, "429 must carry an integer Retry-After header")
	assert.Greater(t, retryAfter, 0)
	assert.EqualValues(t, 1, serviceCallAlertCount(t, table.ID))

	// Resolved 3 minutes ago → cooldown elapsed, a new call opens.
	stale := now.Add(-3 * time.Minute)
	require.NoError(t, database.GetDB().Model(&database.OperationalAlert{}).
		Where("alert_type = ? AND resource_id = ?", database.OperationalAlertTypeServiceCall, table.ID).
		Update("resolved_at", stale).Error)

	again := postServiceCall(t, table.TableCode, `{"reason":"water"}`)
	assert.Equal(t, http.StatusCreated, again.Code)
	assert.Equal(t, "open", decodeStatus(t, again))
	assert.EqualValues(t, 2, serviceCallAlertCount(t, table.ID))
}

func TestGetServiceCallStatusByTableCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupServiceCallTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerSC5", "Service Call Biz 5")
	table := createServiceCallTable(t, business.ID, "T5", "svc-call-5")

	// No call yet → none.
	none := getServiceCallStatus(t, table.TableCode)
	assert.Equal(t, http.StatusOK, none.Code)
	assert.Equal(t, "none", decodeStatus(t, none))

	created := postServiceCall(t, table.TableCode, `{"reason":"water"}`)
	require.Equal(t, http.StatusCreated, created.Code)

	// Staff claims the alert → guest sees "acknowledged".
	require.NoError(t, database.GetDB().Model(&database.OperationalAlert{}).
		Where("alert_type = ? AND resource_id = ?", database.OperationalAlertTypeServiceCall, table.ID).
		Update("status", database.OperationalAlertStatusClaimed).Error)

	acked := getServiceCallStatus(t, table.TableCode)
	assert.Equal(t, http.StatusOK, acked.Code)
	assert.Equal(t, "acknowledged", decodeStatus(t, acked))
	assert.Equal(t, "water", decodeReason(t, acked))
}

func TestCreateServiceCallClosedBusinessIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupServiceCallTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerSCClosed", "Service Call Closed")
	closedAt := time.Now().Add(-time.Hour)
	business.ClosedAt = &closedAt
	require.NoError(t, database.GetDB().Save(business).Error)
	table := createServiceCallTable(t, business.ID, "TClosed", "svc-call-closed")

	w := postServiceCall(t, table.TableCode, `{"reason":"water"}`)
	require.Equal(t, http.StatusForbidden, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "business_unavailable", resp["code"])

	// No alert row should be created for a gated business.
	assert.EqualValues(t, 0, serviceCallAlertCount(t, table.ID))
}

func TestGetServiceCallStatusPastSLAReturnsNone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupServiceCallTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerSCStale", "Service Call Stale")
	table := createServiceCallTable(t, business.ID, "TStale", "svc-call-stale")

	created := postServiceCall(t, table.TableCode, `{"reason":"water"}`)
	require.Equal(t, http.StatusCreated, created.Code)

	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, database.GetDB().Model(&database.OperationalAlert{}).
		Where("alert_type = ? AND resource_id = ?", database.OperationalAlertTypeServiceCall, table.ID).
		Update("last_event_at", stale).Error)

	got := getServiceCallStatus(t, table.TableCode)
	assert.Equal(t, http.StatusOK, got.Code)
	assert.Equal(t, "none", decodeStatus(t, got))
	assert.Empty(t, decodeReason(t, got))
}

func TestGetServiceCallStatusEmptyTableStaleCheckIsNone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupServiceCallTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerSCEmpty", "Service Call Empty")
	table := createServiceCallTable(t, business.ID, "TEmpty", "svc-call-empty")

	created := postServiceCall(t, table.TableCode, `{"reason":"check"}`)
	require.Equal(t, http.StatusCreated, created.Code)

	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, database.GetDB().Model(&database.OperationalAlert{}).
		Where("alert_type = ? AND resource_id = ?", database.OperationalAlertTypeServiceCall, table.ID).
		Update("last_event_at", stale).Error)

	got := getServiceCallStatus(t, table.TableCode)
	assert.Equal(t, http.StatusOK, got.Code)
	assert.Equal(t, "none", decodeStatus(t, got))
	assert.Empty(t, decodeReason(t, got))
}

func TestGetServiceCallStatusKeepsSameSeatingCheckPlease(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupServiceCallTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Bill{}))

	business := createOwnedBusiness(t, "0xOwnerSCCheck", "Service Call Check")
	table := createServiceCallTable(t, business.ID, "TCheck", "svc-call-check")

	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, database.GetDB().Create(&database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     "OPEN-CHECK-" + t.Name(),
		Status:         database.BillStatusOpen,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		Items:          "[]",
		CreatedAt:      stale.Add(-20 * time.Minute),
	}).Error)

	created := postServiceCall(t, table.TableCode, `{"reason":"check"}`)
	require.Equal(t, http.StatusCreated, created.Code)
	require.NoError(t, database.GetDB().Model(&database.OperationalAlert{}).
		Where("alert_type = ? AND resource_id = ?", database.OperationalAlertTypeServiceCall, table.ID).
		Update("last_event_at", stale).Error)

	got := getServiceCallStatus(t, table.TableCode)
	assert.Equal(t, http.StatusOK, got.Code)
	assert.Equal(t, "open", decodeStatus(t, got))
	assert.Equal(t, "check", decodeReason(t, got))
}

func TestCreateServiceCallAfterSLASkipsCooldown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupServiceCallTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerSCReopen", "Service Call Reopen")
	table := createServiceCallTable(t, business.ID, "TReopen", "svc-call-reopen")

	first := postServiceCall(t, table.TableCode, `{"reason":"water"}`)
	require.Equal(t, http.StatusCreated, first.Code)

	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, database.GetDB().Model(&database.OperationalAlert{}).
		Where("alert_type = ? AND resource_id = ?", database.OperationalAlertTypeServiceCall, table.ID).
		Update("last_event_at", stale).Error)

	again := postServiceCall(t, table.TableCode, `{"reason":"order"}`)
	assert.Equal(t, http.StatusCreated, again.Code)
	assert.Equal(t, "open", decodeStatus(t, again))
	assert.Equal(t, "order", decodeReason(t, again))
	assert.EqualValues(t, 2, serviceCallAlertCount(t, table.ID))
}

func TestServiceCallUnknownTableCode404s(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupServiceCallTestDB(t)

	post := postServiceCall(t, "no-such-code", `{"reason":"water"}`)
	assert.Equal(t, http.StatusNotFound, post.Code)

	get := getServiceCallStatus(t, "no-such-code")
	assert.Equal(t, http.StatusNotFound, get.Code)
}
