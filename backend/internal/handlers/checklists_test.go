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

// newChecklistTestServer wires the checklist routes with an injected staff
// identity (mirrors the real auth middleware) over an in-memory SQLite DB that
// has the engagement tables migrated.
func newChecklistTestServer(t *testing.T, staffID uint, role database.StaffRole) (
	func(method, url string, body interface{}) *httptest.ResponseRecorder, *database.DB, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{}, &database.Staff{}, &database.Position{}, &database.StaffPosition{},
		&database.ChecklistTemplate{}, &database.ChecklistItem{},
		&database.ChecklistRun{}, &database.ChecklistItemCompletion{}))
	database.SetTestDB(gormDB)
	d := database.GetDBWrapper()
	h := NewChecklistHandler(d)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if staffID != 0 {
			c.Set("staff_id", staffID)
			c.Set("staff_role", string(role))
		}
		c.Next()
	})
	r.GET("/b/:id/checklists/templates", h.ListTemplates)
	r.POST("/b/:id/checklists/templates", h.CreateTemplate)
	r.POST("/b/:id/checklists/runs", h.CreateRun)
	r.GET("/b/:id/checklists/runs", h.ListRuns)
	r.GET("/b/:id/checklists/runs/:runId", h.RunDetail)
	r.POST("/b/:id/checklists/runs/:runId/items/:itemId", h.TickItem)
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
	return do, d, func() { _ = sqlDB.Close() }
}

// checklistRouterAs drives the same routes as a different staff identity against
// the CURRENT test DB (set via SetTestDB).
func checklistRouterAs(staffID uint, role database.StaffRole) func(method, url string, body interface{}) *httptest.ResponseRecorder {
	h := NewChecklistHandler(database.GetDBWrapper())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("staff_id", staffID)
		c.Set("staff_role", string(role))
		c.Next()
	})
	r.GET("/b/:id/checklists/runs", h.ListRuns)
	r.GET("/b/:id/checklists/runs/:runId", h.RunDetail)
	r.POST("/b/:id/checklists/runs/:runId/items/:itemId", h.TickItem)
	return func(method, url string, body interface{}) *httptest.ResponseRecorder {
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
}

func seedChecklistRun(t *testing.T, d *database.DB, biz, assignedStaff uint) (uint, []uint) {
	t.Helper()
	// The assigned staff must belong to the business (InstantiateChecklistRun now
	// validates the assignment target).
	require.NoError(t, d.GetGorm().Create(&database.Staff{ID: assignedStaff, BusinessID: biz, Email: fmt.Sprintf("s%d@x.co", assignedStaff), Name: fmt.Sprintf("S%d", assignedStaff), Role: database.StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)
	tpl := &database.ChecklistTemplate{BusinessID: biz, Name: "Open", Kind: database.ChecklistKindOpening, CreatedByStaffID: 1, IsActive: true}
	require.NoError(t, d.GetGorm().Create(tpl).Error)
	a := &database.ChecklistItem{BusinessID: biz, TemplateID: tpl.ID, Label: "Lights", SortOrder: 0, IsRequired: true}
	require.NoError(t, d.GetGorm().Create(a).Error)
	run, err := d.InstantiateChecklistRun(biz, tpl.ID, &assignedStaff, nil, time.Now().UTC())
	require.NoError(t, err)
	return run.ID, []uint{a.ID}
}

// TestTickItemOnlyOnOwnRun: a server may not tick a run assigned to someone else.
func TestChecklistTickItemOnlyOnOwnRun(t *testing.T) {
	do, d, cleanup := newChecklistTestServer(t, 7, database.StaffRoleManager)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	runID, items := seedChecklistRun(t, d, 1, 7) // assigned to staff 7
	_ = do                                       // keep the seeding server alive

	// Caller is staff 9 (server) ticking staff 7's run → 403.
	asNine := checklistRouterAs(9, database.StaffRoleServer)
	w := asNine(http.MethodPost, fmt.Sprintf("/b/1/checklists/runs/%d/items/%d", runID, items[0]), map[string]any{"done": true})
	require.Equal(t, http.StatusForbidden, w.Code, "cross-staff tick must be 403")
}

// TestTickItemOwnRunSucceeds: the owning server can tick and flips status.
func TestChecklistTickItemOwnRunSucceeds(t *testing.T) {
	_, d, cleanup := newChecklistTestServer(t, 1, database.StaffRoleManager)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	runID, items := seedChecklistRun(t, d, 1, 7)

	asSeven := checklistRouterAs(7, database.StaffRoleServer)
	w := asSeven(http.MethodPost, fmt.Sprintf("/b/1/checklists/runs/%d/items/%d", runID, items[0]), map[string]any{"done": true})
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, database.ChecklistRunComplete, resp.Data.Status)
}

// TestChecklistRunDetailManagerSeesItems: a manager reads any run's detail (200).
func TestChecklistRunDetailManagerSeesItems(t *testing.T) {
	_, d, cleanup := newChecklistTestServer(t, 1, database.StaffRoleManager)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	runID, items := seedChecklistRun(t, d, 1, 7) // assigned to staff 7

	asMgr := checklistRouterAs(2, database.StaffRoleManager)
	w := asMgr(http.MethodGet, fmt.Sprintf("/b/1/checklists/runs/%d", runID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data struct {
			Run   struct{ ID uint } `json:"run"`
			Items []struct {
				ItemID uint   `json:"item_id"`
				Label  string `json:"label"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, runID, resp.Data.Run.ID)
	require.Len(t, resp.Data.Items, 1)
	require.Equal(t, items[0], resp.Data.Items[0].ItemID)
}

// TestChecklistRunDetailOwnerSeesItems: the assigned server reads own run (200).
func TestChecklistRunDetailOwnerSeesItems(t *testing.T) {
	_, d, cleanup := newChecklistTestServer(t, 1, database.StaffRoleManager)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	runID, _ := seedChecklistRun(t, d, 1, 7)

	asSeven := checklistRouterAs(7, database.StaffRoleServer)
	w := asSeven(http.MethodGet, fmt.Sprintf("/b/1/checklists/runs/%d", runID), nil)
	require.Equal(t, http.StatusOK, w.Code)
}

// TestChecklistRunDetailNonOwnerForbidden: a non-owning server → 403.
func TestChecklistRunDetailNonOwnerForbidden(t *testing.T) {
	_, d, cleanup := newChecklistTestServer(t, 1, database.StaffRoleManager)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	runID, _ := seedChecklistRun(t, d, 1, 7) // assigned to staff 7

	asNine := checklistRouterAs(9, database.StaffRoleServer)
	w := asNine(http.MethodGet, fmt.Sprintf("/b/1/checklists/runs/%d", runID), nil)
	require.Equal(t, http.StatusForbidden, w.Code, "non-owner read must be 403")
}

// TestChecklistCreateRunRejectsForeignStaff (MIN-3): POST a run assigning a staff
// not in the business → 400.
func TestChecklistCreateRunRejectsForeignStaff(t *testing.T) {
	do, d, cleanup := newChecklistTestServer(t, 1, database.StaffRoleManager)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	tpl := &database.ChecklistTemplate{BusinessID: 1, Name: "Open", Kind: database.ChecklistKindOpening, CreatedByStaffID: 1, IsActive: true}
	require.NoError(t, d.GetGorm().Create(tpl).Error)

	w := do(http.MethodPost, "/b/1/checklists/runs", map[string]any{"template_id": tpl.ID, "assigned_staff_id": 999})
	require.Equal(t, http.StatusBadRequest, w.Code, "foreign assigned staff must 400")
}

// TestChecklistCreateTemplateRejectsForeignPosition (MIN-3): POST a position-
// scoped template referencing a non-business position → 400.
func TestChecklistCreateTemplateRejectsForeignPosition(t *testing.T) {
	do, d, cleanup := newChecklistTestServer(t, 1, database.StaffRoleManager)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)

	w := do(http.MethodPost, "/b/1/checklists/templates", map[string]any{
		"name":        "Server onboarding",
		"kind":        database.ChecklistKindOnboarding,
		"position_id": 999,
		"items":       []map[string]any{{"label": "Shadow a shift"}},
	})
	require.Equal(t, http.StatusBadRequest, w.Code, "foreign position must 400")
}

// TestListRunsOwnScopeOnly: GET /checklists/runs returns only the caller's runs.
func TestChecklistListRunsOwnScopeOnly(t *testing.T) {
	_, d, cleanup := newChecklistTestServer(t, 1, database.StaffRoleManager)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	seedChecklistRun(t, d, 1, 7) // staff 7's run
	seedChecklistRun(t, d, 1, 9) // staff 9's run

	asSeven := checklistRouterAs(7, database.StaffRoleServer)
	w := asSeven(http.MethodGet, "/b/1/checklists/runs", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data []struct {
			AssignedStaffID *uint `json:"assigned_staff_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1, "must see only own run")
	require.NotNil(t, resp.Data[0].AssignedStaffID)
	require.Equal(t, uint(7), *resp.Data[0].AssignedStaffID)
}

// TestListRunsScopeBusiness proves the operator run-status list: a manager with
// ?scope=business gets EVERY run (template + assignee names joined), while a plain
// staff member's scope=business request is quietly served their own runs only.
func TestListRunsScopeBusiness(t *testing.T) {
	do, d, cleanup := newChecklistTestServer(t, 1, database.StaffRoleManager)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	// Two runs assigned to different staff.
	seedChecklistRun(t, d, 1, 7)
	seedChecklistRun(t, d, 1, 8)

	// Manager, scope=business → both runs, with display context.
	w := do(http.MethodGet, "/b/1/checklists/runs?scope=business", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data []struct {
			ID                uint   `json:"id"`
			TemplateName      string `json:"template_name"`
			AssignedStaffName string `json:"assigned_staff_name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 2, "manager sees every run in the business")
	require.Equal(t, "Open", resp.Data[0].TemplateName)

	// A plain server requesting scope=business gets only THEIR own runs (staff 7).
	asSeven := checklistRouterAs(7, database.StaffRoleServer)
	w = asSeven(http.MethodGet, "/b/1/checklists/runs?scope=business", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var own struct {
		Data []struct {
			AssignedStaffID *uint `json:"assigned_staff_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &own))
	require.Len(t, own.Data, 1, "no privilege escalation: plain staff still see only their own runs")
	require.NotNil(t, own.Data[0].AssignedStaffID)
	require.Equal(t, uint(7), *own.Data[0].AssignedStaffID)
}
