package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

// ownerThreadsRouter wires ListDirectorThreads behind an owner (web3) context.
func ownerThreadsRouter() *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwnerA")
		c.Next()
	})
	router.GET("/businesses/:id/ai/director/threads", ListDirectorThreads)
	return router
}

func withDirectorService(t testing.TB) {
	prev := directorConsoleService
	directorConsoleService = services.NewDirectorConsoleService(database.GetDBWrapper(), nil, nil, nil)
	t.Cleanup(func() { directorConsoleService = prev })
}

// TestListDirectorThreads_SerializesPinnedAndOrdersPinnedFirst guards the fix
// for the half-broken pin: the DTO must carry `pinned` so the client can toggle
// Pin/Unpin, and pinned threads must sort to the top.
func TestListDirectorThreads_SerializesPinnedAndOrdersPinnedFirst(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupDirectorConsolePerfTestDB(t, logger.Default.LogMode(logger.Silent))
	withDirectorService(t)

	business := createDirectorConsolePerfBusiness(t, "pin-dto")
	older := createDirectorConsolePerfThread(t, business.ID, "Older")
	newer := createDirectorConsolePerfThread(t, business.ID, "Newer")
	// Make `newer` genuinely newer so absent pinning it would sort first.
	require.NoError(t, database.GetDB().Model(&database.DirectorConsoleThread{}).
		Where("id = ?", newer.ID).Update("updated_at", time.Now()).Error)
	require.NoError(t, database.GetDB().Model(&database.DirectorConsoleThread{}).
		Where("id = ?", older.ID).Update("updated_at", time.Now().Add(-time.Hour)).Error)
	// Pin the OLDER one — it must jump to the top and report pinned:true.
	require.NoError(t, database.GetDB().Model(&database.DirectorConsoleThread{}).
		Where("id = ?", older.ID).Update("pinned", true).Error)

	w := performDirectorConsoleRequest(t, ownerThreadsRouter(), http.MethodGet,
		fmt.Sprintf("/businesses/%d/ai/director/threads", business.ID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Threads []services.DirectorThreadDTO `json:"threads"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Threads, 2)
	assert.Equal(t, older.ID, resp.Threads[0].ID, "pinned thread sorts first")
	assert.True(t, resp.Threads[0].Pinned, "DTO must serialize pinned=true")
	assert.False(t, resp.Threads[1].Pinned, "unpinned thread reports pinned=false")

	// The raw JSON must contain the "pinned" key (not omitted) so the client
	// never reads undefined.
	assert.Contains(t, w.Body.String(), `"pinned"`)
}

// TestListDirectorThreads_ArchivedFilterAndPaging guards fix 1 (archived
// section) + fix 9 (offset/limit/total): active list excludes archived by
// default, `?archived=1` returns only archived, and paging adds a total.
func TestListDirectorThreads_ArchivedFilterAndPaging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupDirectorConsolePerfTestDB(t, logger.Default.LogMode(logger.Silent))
	withDirectorService(t)

	business := createDirectorConsolePerfBusiness(t, "archive-page")
	active1 := createDirectorConsolePerfThread(t, business.ID, "Active 1")
	_ = createDirectorConsolePerfThread(t, business.ID, "Active 2")
	archived := createDirectorConsolePerfThread(t, business.ID, "Archived 1")
	require.NoError(t, database.GetDB().Model(&database.DirectorConsoleThread{}).
		Where("id = ?", archived.ID).Update("archived_at", time.Now()).Error)

	router := ownerThreadsRouter()

	// Default: active only, no archived leak, legacy shape (no total needed).
	w := performDirectorConsoleRequest(t, router, http.MethodGet,
		fmt.Sprintf("/businesses/%d/ai/director/threads", business.ID))
	require.Equal(t, http.StatusOK, w.Code)
	var active struct {
		Threads []services.DirectorThreadDTO `json:"threads"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &active))
	require.Len(t, active.Threads, 2, "default list is active threads only")
	for _, th := range active.Threads {
		assert.NotEqual(t, archived.ID, th.ID)
	}

	// Archived filter: only the archived thread, with archived_at populated.
	w = performDirectorConsoleRequest(t, router, http.MethodGet,
		fmt.Sprintf("/businesses/%d/ai/director/threads?archived=1", business.ID))
	require.Equal(t, http.StatusOK, w.Code)
	var arch struct {
		Threads []services.DirectorThreadDTO `json:"threads"`
		Total   int64                        `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &arch))
	require.Len(t, arch.Threads, 1)
	assert.Equal(t, archived.ID, arch.Threads[0].ID)
	require.NotNil(t, arch.Threads[0].ArchivedAt)
	assert.EqualValues(t, 1, arch.Total)

	// Paging over active: limit=1 returns one row but total=2.
	w = performDirectorConsoleRequest(t, router, http.MethodGet,
		fmt.Sprintf("/businesses/%d/ai/director/threads?limit=1&offset=0", business.ID))
	require.Equal(t, http.StatusOK, w.Code)
	var paged struct {
		Threads []services.DirectorThreadDTO `json:"threads"`
		Total   int64                        `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &paged))
	require.Len(t, paged.Threads, 1)
	assert.EqualValues(t, 2, paged.Total, "total reflects the full active count, not the page")
	_ = active1
}

// ownerAppliedActionsRouter wires the applied-actions history behind an owner.
func ownerAppliedActionsRouter() *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwnerA")
		c.Set("user_id", uint(1))
		c.Next()
	})
	router.GET("/businesses/:id/ai/director/actions/applied", ListAppliedDirectorActions)
	return router
}

// TestListAppliedDirectorActions_UndoEligibility guards the applied history's
// server-truth can_undo. Apply bumps the menu version by exactly 1, so a real
// applied action is undoable while current == proposal.MenuVersion+1 (and it's
// in-window and not undone). The pre-fix formula compared proposal.MenuVersion
// directly to the current version, which is true only for legacy no-op applies
// — inverting eligibility for every real action (audit L4-17). Legacy no-op
// audits (empty before-snapshot) stay listed as undoable so the heal path can
// clear them.
func TestListAppliedDirectorActions_UndoEligibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupDirectorConsolePerfTestDB(t, logger.Default.LogMode(logger.Silent))
	require.NoError(t, db.AutoMigrate(
		&database.DirectorProposedAction{},
		&database.DirectorActionAudit{},
		&database.Menu{},
	))
	withDirectorService(t)

	business := createDirectorConsolePerfBusiness(t, "applied")
	thread := createDirectorConsolePerfThread(t, business.ID, "Applied Thread")

	// Menu is at version 3 (the undoable action's apply bumped 2→3).
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Version: 3, Categories: "[]"}).Error)

	// Real apply, nothing changed since: previewed at v2, apply bumped to v3.
	undoable := &database.DirectorProposedAction{
		PublicID: "pa_undoable", BusinessID: business.ID, ThreadID: thread.ID,
		Kind: "menu.adjust_prices", MenuVersion: 2, Status: database.DirectorProposalApplied,
	}
	require.NoError(t, database.CreateDirectorProposedAction(undoable))
	require.NoError(t, database.GetDB().Create(&database.DirectorActionAudit{
		BusinessID: business.ID, ThreadID: thread.ID, ProposedActionID: undoable.ID,
		ActorUserID: 1, Kind: "menu.adjust_prices", AppliedAt: time.Now(),
		BeforeJSON: `[{"item_id":"i1","price":10}]`, AfterJSON: `[{"item_id":"i1","price":12}]`,
	}).Error)

	// Real apply but the menu has advanced since (applied 1→2, now at 3): stale.
	stale := &database.DirectorProposedAction{
		PublicID: "pa_stale", BusinessID: business.ID, ThreadID: thread.ID,
		Kind: "menu.adjust_prices", MenuVersion: 1, Status: database.DirectorProposalApplied,
	}
	require.NoError(t, database.CreateDirectorProposedAction(stale))
	require.NoError(t, database.GetDB().Create(&database.DirectorActionAudit{
		BusinessID: business.ID, ThreadID: thread.ID, ProposedActionID: stale.ID,
		ActorUserID: 1, Kind: "menu.adjust_prices", AppliedAt: time.Now().Add(-2 * time.Minute),
		BeforeJSON: `[{"item_id":"i1","price":8}]`, AfterJSON: `[{"item_id":"i1","price":10}]`,
	}).Error)

	// Legacy no-op apply (pre-L4-17): empty before-snapshot, version never
	// bumped — undo heals it, so it must stay eligible.
	noop := &database.DirectorProposedAction{
		PublicID: "pa_noop", BusinessID: business.ID, ThreadID: thread.ID,
		Kind: "menu.adjust_prices", MenuVersion: 3, Status: database.DirectorProposalApplied,
	}
	require.NoError(t, database.CreateDirectorProposedAction(noop))
	require.NoError(t, database.GetDB().Create(&database.DirectorActionAudit{
		BusinessID: business.ID, ThreadID: thread.ID, ProposedActionID: noop.ID,
		ActorUserID: 1, Kind: "menu.adjust_prices", AppliedAt: time.Now().Add(-3 * time.Minute),
		BeforeJSON: `[]`, AfterJSON: `[]`,
	}).Error)

	// Applied then undone — must report can_undo=false.
	undone := &database.DirectorProposedAction{
		PublicID: "pa_undone", BusinessID: business.ID, ThreadID: thread.ID,
		Kind: "menu.set_availability", MenuVersion: 2, Status: database.DirectorProposalApplied,
	}
	require.NoError(t, database.CreateDirectorProposedAction(undone))
	undoneAt := time.Now()
	require.NoError(t, database.GetDB().Create(&database.DirectorActionAudit{
		BusinessID: business.ID, ThreadID: thread.ID, ProposedActionID: undone.ID,
		ActorUserID: 1, Kind: "menu.set_availability", AppliedAt: time.Now().Add(-time.Minute),
		BeforeJSON: `[{"item_id":"i1","is_available":true}]`,
		UndoneAt:   &undoneAt,
	}).Error)

	w := performDirectorConsoleRequest(t, ownerAppliedActionsRouter(), http.MethodGet,
		fmt.Sprintf("/businesses/%d/ai/director/actions/applied", business.ID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Actions []struct {
			ProposalID string  `json:"proposal_id"`
			Kind       string  `json:"kind"`
			CanUndo    bool    `json:"can_undo"`
			UndoneAt   *string `json:"undone_at"`
		} `json:"actions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Actions, 4)

	byID := map[string]bool{}
	for _, a := range resp.Actions {
		byID[a.ProposalID] = a.CanUndo
	}
	assert.True(t, byID["pa_undoable"], "in-window action whose apply produced the current version is undoable")
	assert.False(t, byID["pa_stale"], "action whose apply the menu has advanced past is NOT undoable")
	assert.True(t, byID["pa_noop"], "legacy no-op audit stays undoable so the heal path can clear it")
	assert.False(t, byID["pa_undone"], "already-undone action is not undoable")
}

// TestListAppliedDirectorActions_RequiresOwner ensures the read is owner-gated.
func TestListAppliedDirectorActions_RequiresOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("token_type", "staff")

	ListAppliedDirectorActions(c)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
