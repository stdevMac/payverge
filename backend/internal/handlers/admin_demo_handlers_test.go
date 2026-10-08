package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	demosvc "github.com/stdevmac/payverge/backend/internal/demo"
)

type fakeAdminDemoService struct {
	summaryAdmin  uint
	summaryEnsure bool
	resetAdmin    uint

	summary *demosvc.Summary
}

func (f *fakeAdminDemoService) SummaryForAdmin(_ context.Context, adminUserID uint, ensure bool) (*demosvc.Summary, error) {
	f.summaryAdmin = adminUserID
	f.summaryEnsure = ensure
	return f.summary, nil
}

func (f *fakeAdminDemoService) ResetForAdmin(_ context.Context, adminUserID uint) (*database.DemoInstance, error) {
	f.resetAdmin = adminUserID
	return f.summary.Instance, nil
}

func (f *fakeAdminDemoService) AppendDueDaysForAdmin(_ context.Context, adminUserID uint) error {
	f.summaryAdmin = adminUserID
	return nil
}

func (f *fakeAdminDemoService) VerifyForAdmin(_ context.Context, _ uint) (demosvc.VerificationResult, error) {
	return f.summary.Verification, nil
}

func TestAdminDemoGetEnsuresAuthenticatedAdminSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeAdminDemoService{summary: demoHandlerSummary(42)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", float64(42))
		c.Next()
	})
	router.GET("/admin/demo", NewAdminDemoHandler(fake).Get)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/demo", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, uint(42), fake.summaryAdmin)
	require.True(t, fake.summaryEnsure)
	require.Contains(t, w.Body.String(), `"admin_user_id":42`)
	require.Contains(t, w.Body.String(), `"status":"passed"`)
	require.Contains(t, w.Body.String(), `"businesses"`)
}

func TestAdminDemoResetUsesAuthenticatedAdminOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeAdminDemoService{summary: demoHandlerSummary(77)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", uint(77))
		c.Next()
	})
	router.POST("/admin/demo/reset", NewAdminDemoHandler(fake).Reset)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/demo/reset", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, uint(77), fake.resetAdmin)
	require.Equal(t, uint(77), fake.summaryAdmin)
	require.False(t, fake.summaryEnsure)
}

func demoHandlerSummary(adminID uint) *demosvc.Summary {
	coreID := uint(101)
	aiProID := uint(102)
	return &demosvc.Summary{
		Instance: &database.DemoInstance{
			ID:                  9,
			AdminUserID:         adminID,
			PrimaryBusinessID:   &coreID,
			SecondaryBusinessID: &aiProID,
			Status:              database.DemoInstanceStatusReady,
			SeedVersion:         "test",
		},
		Businesses: []database.Business{
			{ID: coreID, Name: "Core Demo", IsDemo: true},
			{ID: aiProID, Name: "AI Pro Demo", IsDemo: true},
		},
		Runs: []database.DemoRun{
			{ID: 1, AdminUserID: adminID, RunType: database.DemoRunTypeEnsure, Status: database.DemoRunStatusSucceeded},
		},
		Verification: demosvc.VerificationResult{
			Status: demosvc.VerificationPassed,
			Coverage: []demosvc.CoverageCheck{
				{Key: "menu_images", Status: demosvc.CoveragePassed, Count: 2},
			},
		},
	}
}
