package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAdminFiscalTestDB(t *testing.T) {
	t.Helper()
	// Unique DSN per test — shared-cache names collide across tests in the package.
	dsn := "file:admin_fiscal_" + t.Name() + "?mode=memory&cache=shared"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.BusinessFiscalSettings{},
		&database.FiscalJob{},
		&database.FiscalReceipt{},
	))
	database.SetTestDB(gormDB)
}

func TestGetAdminFiscalSummaryReturnsCounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAdminFiscalTestDB(t)

	biz := database.Business{
		BusinessId: "sum-1", Name: "Summary Cafe", OwnerName: "O",
		Kind: database.BusinessKindReal,
	}
	require.NoError(t, database.GetDB().Create(&biz).Error)
	require.NoError(t, database.GetDB().Create(&database.FiscalJob{
		BusinessID:     biz.ID,
		SettingsID:     1,
		BillID:         1,
		Action:         "issue_receipt",
		IdempotencyKey: "k1",
		Status:         database.FiscalStatusFailedRetryable,
		MaxAttempts:    5,
	}).Error)

	router := gin.New()
	router.GET("/admin/fiscal/summary", GetAdminFiscalSummary)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/fiscal/summary", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var summary database.AdminFiscalSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
	require.Equal(t, int64(1), summary.FailedRetryableJobs)
}

func TestPostAdminRequeueFiscalJobResetsStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAdminFiscalTestDB(t)

	job := database.FiscalJob{
		BusinessID:     2,
		SettingsID:     2,
		BillID:         2,
		Action:         "issue_receipt",
		IdempotencyKey: "k2",
		Status:         database.FiscalStatusFailedPermanent,
		Attempts:       3,
		MaxAttempts:    5,
	}
	require.NoError(t, database.GetDB().Create(&job).Error)

	router := gin.New()
	router.POST("/admin/fiscal/jobs/:id/requeue", PostAdminRequeueFiscalJob)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/fiscal/jobs/1/requeue", strings.NewReader(`{"allow_permanent":true}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Job database.FiscalJob `json:"job"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, database.FiscalStatusPending, resp.Job.Status)
	require.Equal(t, 0, resp.Job.Attempts)
}

func TestPostAdminRequeueFiscalJobRejectsAuthorizedJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAdminFiscalTestDB(t)

	require.NoError(t, database.GetDB().Create(&database.FiscalJob{
		BusinessID:     3,
		SettingsID:     3,
		BillID:         3,
		Action:         "issue_receipt",
		IdempotencyKey: "k3",
		Status:         database.FiscalStatusAuthorized,
		MaxAttempts:    5,
	}).Error)

	router := gin.New()
	router.POST("/admin/fiscal/jobs/:id/requeue", PostAdminRequeueFiscalJob)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/fiscal/jobs/1/requeue", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusConflict, w.Code)
}

func TestPostAdminRequeueFiscalJobAllowPermanent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAdminFiscalTestDB(t)
	db := database.GetDB()

	retryable := database.FiscalJob{
		BusinessID:     4,
		SettingsID:     4,
		BillID:         4,
		Action:         "issue_receipt",
		IdempotencyKey: "k-retryable",
		Status:         database.FiscalStatusFailedRetryable,
		Attempts:       2,
		MaxAttempts:    5,
	}
	permanent := database.FiscalJob{
		BusinessID:     5,
		SettingsID:     5,
		BillID:         5,
		Action:         "issue_receipt",
		IdempotencyKey: "k-permanent",
		Status:         database.FiscalStatusFailedPermanent,
		Attempts:       5,
		MaxAttempts:    5,
	}
	require.NoError(t, db.Create(&retryable).Error)
	require.NoError(t, db.Create(&permanent).Error)

	router := gin.New()
	router.POST("/admin/fiscal/jobs/:id/requeue", PostAdminRequeueFiscalJob)

	denied := postAdminRequeue(router, permanent.ID, "")
	require.Equal(t, http.StatusConflict, denied.Code, denied.Body.String())

	allowed := postAdminRequeue(router, permanent.ID, `{"allow_permanent":true}`)
	require.Equal(t, http.StatusOK, allowed.Code, allowed.Body.String())

	retry := postAdminRequeue(router, retryable.ID, "")
	require.Equal(t, http.StatusOK, retry.Code, retry.Body.String())

	malformed := postAdminRequeue(router, retryable.ID, `{`)
	require.Equal(t, http.StatusBadRequest, malformed.Code, malformed.Body.String())
}

func postAdminRequeue(router *gin.Engine, id uint, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/admin/fiscal/jobs/%d/requeue", id), reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(w, req)
	return w
}

// TestGetAdminFiscalJobs_ReturnsBusinessNameAndExcludesDemo locks Task 15 end-to-end:
// list payload carries business_name and kind!=real is excluded by default.
func TestGetAdminFiscalJobs_ReturnsBusinessNameAndExcludesDemo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAdminFiscalTestDB(t)
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&database.Business{}))

	realBiz := database.Business{
		BusinessId: "real-f", Name: "Real Fiscal Cafe", OwnerName: "O",
		Kind: database.BusinessKindReal,
	}
	demoBiz := database.Business{
		BusinessId: "demo-f", Name: "Demo Fiscal Cafe", OwnerName: "O",
		Kind: database.BusinessKindDemo, IsDemo: true,
	}
	require.NoError(t, db.Create(&realBiz).Error)
	require.NoError(t, db.Create(&demoBiz).Error)

	require.NoError(t, db.Create(&database.FiscalJob{
		BusinessID: realBiz.ID, SettingsID: 1, BillID: 11, Action: "issue_receipt",
		IdempotencyKey: "handler-real", Status: database.FiscalStatusFailedRetryable, MaxAttempts: 5,
	}).Error)
	require.NoError(t, db.Create(&database.FiscalJob{
		BusinessID: demoBiz.ID, SettingsID: 2, BillID: 12, Action: "issue_receipt",
		IdempotencyKey: "handler-demo", Status: database.FiscalStatusPending, MaxAttempts: 5,
	}).Error)

	router := gin.New()
	router.GET("/admin/fiscal/jobs", GetAdminFiscalJobs)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/fiscal/jobs", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Jobs  []database.AdminFiscalJobRow `json:"jobs"`
		Total int64                        `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.EqualValues(t, 1, resp.Total)
	require.Len(t, resp.Jobs, 1)
	require.Equal(t, "Real Fiscal Cafe", resp.Jobs[0].BusinessName)
	require.Equal(t, realBiz.ID, resp.Jobs[0].BusinessID)
}
