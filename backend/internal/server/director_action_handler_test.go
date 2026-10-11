package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/services/director_actions"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupDirectorActionTestDB initialises a SQLite in-memory database with all
// tables the apply handler tests need. Mirrors the pattern used by
// setupDirectorThreadTest / setupStaffHandlerTestDB in other server tests.
func setupDirectorActionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Menu{},
		&database.DirectorProposedAction{},
		&database.DirectorActionAudit{},
	))
	return gormDB
}

// stubDirectorActionService is a simple in-memory stub that lets handler tests
// control which error (or success result) Apply returns without standing up a
// real SQLite-backed DirectorActionService.
type stubDirectorActionService struct {
	applyErr   error
	applyRes   *services.ApplyDirectorActionResult
	proposeErr error
	proposeRes *services.ProposePriceChangeResult
}

func (s *stubDirectorActionService) Apply(_ context.Context, _ services.ApplyDirectorActionInput) (*services.ApplyDirectorActionResult, error) {
	return s.applyRes, s.applyErr
}

func (s *stubDirectorActionService) Undo(_ context.Context, _ services.UndoDirectorActionInput) (*services.ApplyDirectorActionResult, error) {
	return nil, errors.New("not implemented in this stub")
}

func (s *stubDirectorActionService) ProposePriceChange(_ context.Context, _ services.ProposePriceChangeInput) (*services.ProposePriceChangeResult, error) {
	return s.proposeRes, s.proposeErr
}

// newApplyContext creates a gin.Context wired with JSON body, owner token
// claims, and the :id route param.
func newApplyContext(t *testing.T, businessID uint, body any) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var reqBody *bytes.Reader
	if body != nil {
		payload, _ := json.Marshal(body)
		reqBody = bytes.NewReader(payload)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(http.MethodPost, "/ai/director/actions/apply", reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", businessID)}}
	c.Set("token_type", "web3")
	c.Set("address", "0xOwnerA")
	c.Set("user_id", float64(42)) // JWT float64 shape
	return w, c
}

// withStubApplyService replaces the package-level directorActionService with
// the stub and restores the original on test cleanup.
func withStubApplyService(t *testing.T, stub DirectorActionServiceAPI) {
	t.Helper()
	prev := directorActionService
	directorActionService = stub
	t.Cleanup(func() { directorActionService = prev })
}

func TestApplyDirectorAction_409_MenuChanged(t *testing.T) {
	setupDirectorActionTestDB(t)

	withStubApplyService(t, &stubDirectorActionService{
		applyErr: services.ErrDirectorActionVersionConflict,
	})

	w, c := newApplyContext(t, 1, map[string]any{
		"proposal_id": "pa_test123",
	})
	ApplyDirectorAction(c)

	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "menu_changed", resp["code"])
}

func TestApplyDirectorAction_409_NoItemsMatch(t *testing.T) {
	setupDirectorActionTestDB(t)

	withStubApplyService(t, &stubDirectorActionService{
		applyErr: services.ErrDirectorActionNoMatch,
	})

	w, c := newApplyContext(t, 1, map[string]any{
		"proposal_id": "pa_test123",
	})
	ApplyDirectorAction(c)

	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "no_items_match", resp["code"],
		"zero-match apply must carry its own code so the client can show an honest message, not 'menu changed'")
}

func TestApplyDirectorAction_428_ReconfirmRequired(t *testing.T) {
	setupDirectorActionTestDB(t)

	withStubApplyService(t, &stubDirectorActionService{
		applyErr: services.ErrDirectorActionReconfirmRequired,
	})

	w, c := newApplyContext(t, 1, map[string]any{
		"proposal_id": "pa_test123",
		"reconfirm":   false,
	})
	ApplyDirectorAction(c)

	assert.Equal(t, http.StatusPreconditionRequired, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "reconfirm_required", resp["code"])
}

func TestApplyDirectorAction_404_NotFound(t *testing.T) {
	setupDirectorActionTestDB(t)

	withStubApplyService(t, &stubDirectorActionService{
		applyErr: services.ErrDirectorActionNotFound,
	})

	w, c := newApplyContext(t, 1, map[string]any{
		"proposal_id": "pa_nonexistent",
	})
	ApplyDirectorAction(c)

	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestApplyDirectorAction_410_Gone_NotPending(t *testing.T) {
	setupDirectorActionTestDB(t)

	withStubApplyService(t, &stubDirectorActionService{
		applyErr: services.ErrDirectorActionNotPending,
	})

	w, c := newApplyContext(t, 1, map[string]any{
		"proposal_id": "pa_applied",
	})
	ApplyDirectorAction(c)

	assert.Equal(t, http.StatusGone, w.Code, w.Body.String())
}

func TestApplyDirectorAction_410_Gone_Expired(t *testing.T) {
	setupDirectorActionTestDB(t)

	withStubApplyService(t, &stubDirectorActionService{
		applyErr: services.ErrDirectorActionExpired,
	})

	w, c := newApplyContext(t, 1, map[string]any{
		"proposal_id": "pa_expired",
	})
	ApplyDirectorAction(c)

	assert.Equal(t, http.StatusGone, w.Code, w.Body.String())
}

func TestApplyDirectorAction_200_Applied(t *testing.T) {
	setupDirectorActionTestDB(t)

	withStubApplyService(t, &stubDirectorActionService{
		applyRes: &services.ApplyDirectorActionResult{
			NewMenuVersion: 2,
			ProposalID:     "pa_test123",
			AuditID:        7,
			Kind:           string(director_actions.KindAdjustPrices),
		},
	})

	w, c := newApplyContext(t, 1, map[string]any{
		"proposal_id": "pa_test123",
		"reconfirm":   false,
	})
	ApplyDirectorAction(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, true, resp["applied"])
	result, ok := resp["result"].(map[string]any)
	require.True(t, ok, "expected result object in response")
	assert.Equal(t, float64(2), result["new_menu_version"])
	assert.Equal(t, "pa_test123", result["proposal_id"])
}

func TestApplyDirectorAction_400_MissingProposalID(t *testing.T) {
	setupDirectorActionTestDB(t)

	withStubApplyService(t, &stubDirectorActionService{})

	// No proposal_id in body.
	w, c := newApplyContext(t, 1, map[string]any{})
	ApplyDirectorAction(c)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestApplyDirectorAction_503_ServiceNil(t *testing.T) {
	setupDirectorActionTestDB(t)

	// Force directorActionService to nil to simulate un-wired service.
	prev := directorActionService
	directorActionService = nil
	defer func() { directorActionService = prev }()

	w, c := newApplyContext(t, 1, map[string]any{
		"proposal_id": "pa_test123",
	})
	ApplyDirectorAction(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

func TestApplyDirectorAction_400_InvalidBusinessID(t *testing.T) {
	setupDirectorActionTestDB(t)

	withStubApplyService(t, &stubDirectorActionService{})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/ai/director/actions/apply",
		bytes.NewReader([]byte(`{"proposal_id":"pa_test"}`)))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: "not-a-number"}}
	c.Set("token_type", "web3")
	c.Set("user_id", float64(1))

	ApplyDirectorAction(c)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// --- Undo handler tests ---

// stubUndoDirectorActionService adds Undo support to the existing apply stub.
type stubUndoDirectorActionService struct {
	applyErr error
	applyRes *services.ApplyDirectorActionResult
	undoErr  error
	undoRes  *services.ApplyDirectorActionResult
}

func (s *stubUndoDirectorActionService) Apply(_ context.Context, _ services.ApplyDirectorActionInput) (*services.ApplyDirectorActionResult, error) {
	return s.applyRes, s.applyErr
}

func (s *stubUndoDirectorActionService) Undo(_ context.Context, _ services.UndoDirectorActionInput) (*services.ApplyDirectorActionResult, error) {
	return s.undoRes, s.undoErr
}

func (s *stubUndoDirectorActionService) ProposePriceChange(_ context.Context, _ services.ProposePriceChangeInput) (*services.ProposePriceChangeResult, error) {
	return nil, errors.New("not implemented in this stub")
}

// newUndoContext creates a gin.Context for undo requests.
func newUndoContext(t *testing.T, businessID uint, body any) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var reqBody *bytes.Reader
	if body != nil {
		payload, _ := json.Marshal(body)
		reqBody = bytes.NewReader(payload)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(http.MethodPost, "/ai/director/actions/undo", reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", businessID)}}
	c.Set("token_type", "web3")
	c.Set("address", "0xOwnerA")
	c.Set("user_id", float64(42))
	return w, c
}

func TestUndoDirectorAction_409_Stale(t *testing.T) {
	setupDirectorActionTestDB(t)

	stub := &stubUndoDirectorActionService{undoErr: services.ErrDirectorActionUndoStale}
	prev := directorActionService
	directorActionService = stub
	defer func() { directorActionService = prev }()

	w, c := newUndoContext(t, 1, map[string]any{"proposal_id": "pa_test"})
	UndoDirectorAction(c)

	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "menu_changed_since_apply", resp["code"])
}

func TestUndoDirectorAction_410_Expired(t *testing.T) {
	setupDirectorActionTestDB(t)

	stub := &stubUndoDirectorActionService{undoErr: services.ErrDirectorActionUndoExpired}
	prev := directorActionService
	directorActionService = stub
	defer func() { directorActionService = prev }()

	w, c := newUndoContext(t, 1, map[string]any{"proposal_id": "pa_test"})
	UndoDirectorAction(c)

	assert.Equal(t, http.StatusGone, w.Code, w.Body.String())
}

func TestUndoDirectorAction_409_AlreadyUndone(t *testing.T) {
	setupDirectorActionTestDB(t)

	stub := &stubUndoDirectorActionService{undoErr: services.ErrDirectorActionAlreadyUndone}
	prev := directorActionService
	directorActionService = stub
	defer func() { directorActionService = prev }()

	w, c := newUndoContext(t, 1, map[string]any{"proposal_id": "pa_test"})
	UndoDirectorAction(c)

	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

func TestUndoDirectorAction_200_Undone(t *testing.T) {
	setupDirectorActionTestDB(t)

	stub := &stubUndoDirectorActionService{
		undoRes: &services.ApplyDirectorActionResult{
			NewMenuVersion: 3,
			ProposalID:     "pa_test",
			AuditID:        5,
			Kind:           string(director_actions.KindAdjustPrices),
		},
	}
	prev := directorActionService
	directorActionService = stub
	defer func() { directorActionService = prev }()

	w, c := newUndoContext(t, 1, map[string]any{"proposal_id": "pa_test"})
	UndoDirectorAction(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, true, resp["undone"])
	result, ok := resp["result"].(map[string]any)
	require.True(t, ok, "expected result object in response")
	assert.Equal(t, float64(3), result["new_menu_version"])
}

func TestUndoDirectorAction_400_MissingProposalID(t *testing.T) {
	setupDirectorActionTestDB(t)

	stub := &stubUndoDirectorActionService{}
	prev := directorActionService
	directorActionService = stub
	defer func() { directorActionService = prev }()

	w, c := newUndoContext(t, 1, map[string]any{})
	UndoDirectorAction(c)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// TestApplyDirectorAction_FullStack exercises the handler with a real
// SQLite-backed DirectorActionService (no stub) to verify the end-to-end
// happy path from HTTP request through service to DB state.
func TestApplyDirectorAction_FullStack_HappyPath(t *testing.T) {
	gormDB := setupDirectorActionTestDB(t)

	// Seed business + menu v1 (burger $10).
	biz := &database.Business{
		BusinessId:     "biz-full-stack",
		Name:           "Full Stack Resto",
		SettlementAddr: "0xtest",
		TippingAddr:    "0xtest",
	}
	require.NoError(t, gormDB.Create(biz).Error)

	cats := []database.MenuCategory{{
		ID:   "cat-1",
		Name: "Mains",
		Items: []database.MenuItem{
			{ID: "i1", Name: "Burger", Price: 10.0, IsAvailable: true},
		},
	}}
	catsJSON, _ := json.Marshal(cats)
	menu := &database.Menu{
		BusinessID: biz.ID,
		Categories: string(catsJSON),
		IsActive:   true,
		Version:    1,
	}
	require.NoError(t, gormDB.Create(menu).Error)

	proposal := &database.DirectorProposedAction{
		PublicID:    "pa_full_stack_test",
		BusinessID:  biz.ID,
		ThreadID:    1,
		Kind:        string(director_actions.KindAdjustPrices),
		ParamsJSON:  `{"scope":"all","mode":"percent","value":20,"direction":"up"}`,
		PreviewJSON: `{"affected_count":1}`,
		MenuVersion: 1,
		Status:      database.DirectorProposalPending,
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	require.NoError(t, gormDB.Create(proposal).Error)

	// Wire a real service.
	realSvc := services.NewDirectorActionService(database.GetDBWrapper())
	prev := directorActionService
	directorActionService = realSvc
	defer func() { directorActionService = prev }()

	w, c := newApplyContext(t, biz.ID, map[string]any{
		"proposal_id": "pa_full_stack_test",
	})
	ApplyDirectorAction(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, true, resp["applied"])
	result, ok := resp["result"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(2), result["new_menu_version"])

	// Verify DB state: burger $12, proposal applied.
	_, reloadedCats, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	assert.InDelta(t, 12.0, reloadedCats[0].Items[0].Price, 0.001)

	var reloadedProposal database.DirectorProposedAction
	require.NoError(t, gormDB.First(&reloadedProposal, proposal.ID).Error)
	assert.Equal(t, database.DirectorProposalApplied, reloadedProposal.Status)
}

// --- ProposeDirectorPriceChange handler tests ---

// newProposeContext creates a gin.Context for propose-price-change requests.
func newProposeContext(t *testing.T, businessID uint, body any) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var reqBody *bytes.Reader
	if body != nil {
		payload, _ := json.Marshal(body)
		reqBody = bytes.NewReader(payload)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(http.MethodPost, "/ai/director/actions/propose-price-change", reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", businessID)}}
	c.Set("token_type", "web3")
	c.Set("address", "0xOwnerA")
	c.Set("user_id", float64(42))
	return w, c
}

// TestProposeDirectorPriceChange_FullStack_HappyPath exercises the handler with a
// real SQLite-backed DirectorActionService (no stub) and asserts the exact JSON
// response shape the frontend slice consumes: data.proposal.public_id (+ kind,
// preview, etc.).
func TestProposeDirectorPriceChange_FullStack_HappyPath(t *testing.T) {
	gormDB := setupDirectorActionTestDB(t)

	biz := &database.Business{
		BusinessId:     "biz-propose-full",
		Name:           "Propose Resto",
		SettlementAddr: "0xtest",
		TippingAddr:    "0xtest",
	}
	require.NoError(t, gormDB.Create(biz).Error)

	cats := []database.MenuCategory{{
		ID:   "cat-1",
		Name: "Mains",
		Items: []database.MenuItem{
			{ID: "i1", Name: "Burger", Price: 10.0, IsAvailable: true},
		},
	}}
	catsJSON, _ := json.Marshal(cats)
	require.NoError(t, gormDB.Create(&database.Menu{
		BusinessID: biz.ID,
		Categories: string(catsJSON),
		IsActive:   true,
		Version:    1,
	}).Error)

	realSvc := services.NewDirectorActionService(database.GetDBWrapper())
	prev := directorActionService
	directorActionService = realSvc
	defer func() { directorActionService = prev }()

	w, c := newProposeContext(t, biz.ID, map[string]any{
		"menu_item_id": "i1",
		"new_price":    14.0,
	})
	ProposeDirectorPriceChange(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok, "expected data object")
	proposal, ok := data["proposal"].(map[string]any)
	require.True(t, ok, "expected data.proposal object")
	assert.NotEmpty(t, proposal["public_id"], "frontend needs data.proposal.public_id")
	assert.Equal(t, string(director_actions.KindAdjustPrices), proposal["kind"])
	assert.Equal(t, false, proposal["requires_reconfirm"])

	// A pending, threadless proposal row must exist and be applyable through the
	// existing rail.
	row, err := database.GetDirectorProposedActionByPublicID(biz.ID, proposal["public_id"].(string))
	require.NoError(t, err)
	assert.Equal(t, database.DirectorProposalPending, row.Status)
	assert.Equal(t, uint(0), row.ThreadID)
}

func TestProposeDirectorPriceChange_400_NoIncrease(t *testing.T) {
	setupDirectorActionTestDB(t)
	withStubApplyService(t, &stubDirectorActionService{proposeErr: services.ErrNoPriceIncrease})

	w, c := newProposeContext(t, 1, map[string]any{"menu_item_id": "i1", "new_price": 5.0})
	ProposeDirectorPriceChange(c)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestProposeDirectorPriceChange_400_InvalidPrice(t *testing.T) {
	setupDirectorActionTestDB(t)
	withStubApplyService(t, &stubDirectorActionService{proposeErr: services.ErrInvalidProposedPrice})

	// new_price negative passes binding (non-zero) and reaches the service.
	w, c := newProposeContext(t, 1, map[string]any{"menu_item_id": "i1", "new_price": -3.0})
	ProposeDirectorPriceChange(c)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestProposeDirectorPriceChange_404_ItemNotFound(t *testing.T) {
	setupDirectorActionTestDB(t)
	withStubApplyService(t, &stubDirectorActionService{proposeErr: services.ErrDirectorActionNotFound})

	w, c := newProposeContext(t, 1, map[string]any{"menu_item_id": "ghost", "new_price": 14.0})
	ProposeDirectorPriceChange(c)

	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestProposeDirectorPriceChange_400_MissingFields(t *testing.T) {
	setupDirectorActionTestDB(t)
	withStubApplyService(t, &stubDirectorActionService{})

	// Missing menu_item_id (and new_price) → binding 400.
	w, c := newProposeContext(t, 1, map[string]any{})
	ProposeDirectorPriceChange(c)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestProposeDirectorPriceChange_503_ServiceNil(t *testing.T) {
	setupDirectorActionTestDB(t)
	prev := directorActionService
	directorActionService = nil
	defer func() { directorActionService = prev }()

	w, c := newProposeContext(t, 1, map[string]any{"menu_item_id": "i1", "new_price": 14.0})
	ProposeDirectorPriceChange(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}
