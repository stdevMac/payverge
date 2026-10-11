package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newCoverageTestServer(t *testing.T, staffID uint, role, tokenType string) (func(method, url string, body interface{}) *httptest.ResponseRecorder, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Staff{}, &database.Position{},
		&database.StaffPosition{}, &database.Shift{}, &database.OpenShiftClaim{}, &database.ShiftSwapRequest{}, &database.RBACAuditLog{}))
	// Partial unique index (status='pending') — the atomic double-claim guard,
	// mirroring db_config.go autoMigrate so the double-claim path returns 409.
	require.NoError(t, gormDB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_open_claims_one_pending ON open_shift_claims (business_id, shift_id, claiming_staff_id) WHERE status = 'pending'").Error)
	database.SetTestDB(gormDB)
	h := NewCoverageHandler(database.GetDBWrapper())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if staffID != 0 {
			c.Set("staff_id", staffID)
			c.Set("staff_role", role)
		}
		c.Set("token_type", tokenType)
		c.Next()
	})
	r.POST("/b/:id/shifts/:shiftId/claim", h.ClaimOpenShift)
	r.POST("/b/:id/shifts/:shiftId/swap", h.RequestSwap)
	r.POST("/b/:id/swaps/:swapId/accept", h.AcceptSwap)
	r.POST("/b/:id/swaps/:swapId/decision", h.Decide)
	r.POST("/b/:id/coverage/:requestId/cancel", h.Cancel)
	r.GET("/b/:id/coverage/open", h.ListOpen)
	r.GET("/b/:id/coverage/mine", h.ListMine)
	r.GET("/b/:id/coverage/history", h.History)
	do := func(method, url string, body interface{}) *httptest.ResponseRecorder {
		var rdr *bytes.Reader
		if body != nil {
			bb, _ := json.Marshal(body)
			rdr = bytes.NewReader(bb)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, url, rdr)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	return do, func() { _ = sqlDB.Close() }
}

func TestCoverageClaimHTTP(t *testing.T) {
	do, cleanup := newCoverageTestServer(t, 5, "server", "staff")
	defer cleanup()
	require.NoError(t, database.GetDBWrapper().GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, database.GetDBWrapper().GetGorm().Create(&database.StaffPosition{BusinessID: 1, StaffID: 5, PositionID: 9}).Error)
	require.NoError(t, database.GetDBWrapper().GetGorm().Create(&database.Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, Status: database.ShiftStatusOpen, CreatedByStaffID: 1}).Error)

	w := do(http.MethodPost, "/b/1/shifts/10/claim", nil)
	require.Equal(t, http.StatusCreated, w.Code)

	w = do(http.MethodPost, "/b/1/shifts/10/claim", nil) // double-claim -> 409
	require.Equal(t, http.StatusConflict, w.Code)

	w = do(http.MethodGet, "/b/1/coverage/mine", nil)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestCoverageClaimIneligible403(t *testing.T) {
	do, cleanup := newCoverageTestServer(t, 5, "server", "staff")
	defer cleanup()
	require.NoError(t, database.GetDBWrapper().GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	// staff 5 NOT assigned to the shift's position 9
	require.NoError(t, database.GetDBWrapper().GetGorm().Create(&database.Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, Status: database.ShiftStatusOpen, CreatedByStaffID: 1}).Error)

	w := do(http.MethodPost, "/b/1/shifts/10/claim", nil)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestCoverageDecisionBadKind400(t *testing.T) {
	do, cleanup := newCoverageTestServer(t, 0, "", "web3") // owner/approver
	defer cleanup()
	require.NoError(t, database.GetDBWrapper().GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	w := do(http.MethodPost, "/b/1/swaps/1/decision", map[string]interface{}{"kind": "bogus", "decision": "approve"})
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCoverageDecisionSwapApproveHTTP(t *testing.T) {
	do, cleanup := newCoverageTestServer(t, 0, "", "web3") // owner/approver
	defer cleanup()
	g := database.GetDBWrapper().GetGorm()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	staff6 := uint(6)
	require.NoError(t, g.Create(&database.Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, StaffID: ptrUintH(5), Status: database.ShiftStatusFilled, CreatedByStaffID: 1}).Error)
	require.NoError(t, g.Create(&database.ShiftSwapRequest{ID: 1, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5, AcceptingStaffID: &staff6, Kind: database.SwapKindSwap, Status: database.SwapStatusAccepted}).Error)

	w := do(http.MethodPost, "/b/1/swaps/1/decision", map[string]interface{}{"kind": "swap", "decision": "approve"})
	require.Equal(t, http.StatusOK, w.Code)

	// open-claim decision rides the SAME route, keyed on kind=open_claim
	require.NoError(t, g.Create(&database.Shift{ID: 20, BusinessID: 1, ScheduleID: 1, PositionID: 9, Status: database.ShiftStatusOpen, CreatedByStaffID: 1}).Error)
	require.NoError(t, g.Create(&database.OpenShiftClaim{ID: 7, BusinessID: 1, ShiftID: 20, ClaimingStaffID: 6, Status: database.OpenClaimStatusPending}).Error)
	w = do(http.MethodPost, "/b/1/swaps/7/decision", map[string]interface{}{"kind": "open_claim", "decision": "deny"})
	require.Equal(t, http.StatusOK, w.Code)
}

func TestCoverageCancelSwapHTTP(t *testing.T) {
	do, cleanup := newCoverageTestServer(t, 5, "server", "staff")
	defer cleanup()
	g := database.GetDBWrapper().GetGorm()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&database.ShiftSwapRequest{ID: 1, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5, Kind: database.SwapKindSwap, Status: database.SwapStatusOpen}).Error)

	// The requester cancels their own open swap.
	w := do(http.MethodPost, "/b/1/coverage/1/cancel", map[string]interface{}{"kind": "swap"})
	require.Equal(t, http.StatusOK, w.Code)

	var sw database.ShiftSwapRequest
	require.NoError(t, g.First(&sw, 1).Error)
	require.Equal(t, database.SwapStatusCancelled, sw.Status)

	// Cancelling again (now terminal) → 409.
	w = do(http.MethodPost, "/b/1/coverage/1/cancel", map[string]interface{}{"kind": "swap"})
	require.Equal(t, http.StatusConflict, w.Code)
}

func TestCoverageCancelForeignRequestIs403(t *testing.T) {
	// The caller is staff 7; the request belongs to staff 5.
	do, cleanup := newCoverageTestServer(t, 7, "server", "staff")
	defer cleanup()
	g := database.GetDBWrapper().GetGorm()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&database.ShiftSwapRequest{ID: 1, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5, Kind: database.SwapKindGiveup, Status: database.SwapStatusPendingApproval}).Error)

	w := do(http.MethodPost, "/b/1/coverage/1/cancel", map[string]interface{}{"kind": "giveup"})
	require.Equal(t, http.StatusForbidden, w.Code)

	var sw database.ShiftSwapRequest
	require.NoError(t, g.First(&sw, 1).Error)
	require.Equal(t, database.SwapStatusPendingApproval, sw.Status, "a foreign cancel must not mutate the request")
}

func TestCoverageWithdrawClaimHTTP(t *testing.T) {
	do, cleanup := newCoverageTestServer(t, 5, "server", "staff")
	defer cleanup()
	g := database.GetDBWrapper().GetGorm()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&database.OpenShiftClaim{ID: 1, BusinessID: 1, ShiftID: 10, ClaimingStaffID: 5, Status: database.OpenClaimStatusPending}).Error)

	w := do(http.MethodPost, "/b/1/coverage/1/cancel", map[string]interface{}{"kind": "open_claim"})
	require.Equal(t, http.StatusOK, w.Code)

	var cl database.OpenShiftClaim
	require.NoError(t, g.First(&cl, 1).Error)
	require.Equal(t, database.OpenClaimStatusWithdrawn, cl.Status)
}

func TestCoverageCancelBadKind400(t *testing.T) {
	do, cleanup := newCoverageTestServer(t, 5, "server", "staff")
	defer cleanup()
	require.NoError(t, database.GetDBWrapper().GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	w := do(http.MethodPost, "/b/1/coverage/1/cancel", map[string]interface{}{"kind": "bogus"})
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCoverageCancelOwnerForbidden(t *testing.T) {
	// An owner (no staff row) has no requests to cancel → 403.
	do, cleanup := newCoverageTestServer(t, 0, "", "web3")
	defer cleanup()
	require.NoError(t, database.GetDBWrapper().GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	w := do(http.MethodPost, "/b/1/coverage/1/cancel", map[string]interface{}{"kind": "swap"})
	require.Equal(t, http.StatusForbidden, w.Code)
}

func ptrUintH(v uint) *uint { return &v }

// TestCoverageHistoryHTTP proves an approver sees the resolved coverage feed with
// its enriched shape (kind/status/requester/decider/position), and that a
// non-terminal request is excluded. Money-free.
func TestCoverageHistoryHTTP(t *testing.T) {
	do, cleanup := newCoverageTestServer(t, 2, "manager", "staff")
	defer cleanup()
	g := database.GetDB()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	base := time.Date(2026, 7, 1, 16, 0, 0, 0, time.UTC)
	require.NoError(t, g.Create(&database.Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, StartsAt: base, EndsAt: base.Add(6 * time.Hour), Status: "filled", CreatedByStaffID: 1}).Error)
	decider := uint(2)
	res := base.Add(time.Hour)
	require.NoError(t, g.Create(&database.ShiftSwapRequest{ID: 100, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5, Kind: "swap", Status: "approved", ApprovedByStaffID: &decider, ResolvedAt: &res}).Error)
	require.NoError(t, g.Create(&database.ShiftSwapRequest{ID: 101, BusinessID: 1, ShiftID: 10, RequestingStaffID: 6, Kind: "swap", Status: "open"}).Error) // excluded

	w := do(http.MethodGet, "/b/1/coverage/history", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data []struct {
			Kind             string `json:"kind"`
			Status           string `json:"status"`
			RequesterStaffID uint   `json:"requester_staff_id"`
			DeciderStaffID   *uint  `json:"decider_staff_id"`
			PositionID       uint   `json:"position_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1, "only the resolved swap; the open one is excluded")
	require.Equal(t, "swap", resp.Data[0].Kind)
	require.Equal(t, "approved", resp.Data[0].Status)
	require.Equal(t, uint(5), resp.Data[0].RequesterStaffID)
	require.NotNil(t, resp.Data[0].DeciderStaffID)
	require.Equal(t, uint(2), *resp.Data[0].DeciderStaffID)
	require.Equal(t, uint(9), resp.Data[0].PositionID)
	require.NotContains(t, w.Body.String(), "$")
}

// TestCoverageHistoryForbidsNonApprover proves the history feed is a manager
// surface: a server (non-approver) is refused with 403.
func TestCoverageHistoryForbidsNonApprover(t *testing.T) {
	do, cleanup := newCoverageTestServer(t, 5, "server", "staff")
	defer cleanup()
	require.NoError(t, database.GetDB().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	w := do(http.MethodGet, "/b/1/coverage/history", nil)
	require.Equal(t, http.StatusForbidden, w.Code)
}

// TestCoverageHistoryAllowsEmailOwner proves email/password owners (token_type
// "user") are approvers — same as wallet owners ("web3"). Without this, the
// Schedule → History panel 403s for every non-wallet owner session.
func TestCoverageHistoryAllowsEmailOwner(t *testing.T) {
	do, cleanup := newCoverageTestServer(t, 0, "", "user")
	defer cleanup()
	require.NoError(t, database.GetDB().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	w := do(http.MethodGet, "/b/1/coverage/history", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Data []any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Data)
}
