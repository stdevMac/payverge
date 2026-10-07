package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
)

func setupAITakeoverAlertTest(t *testing.T) (*gorm.DB, *database.Business, *database.AiWaiterConversation, *database.Table) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&database.Menu{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
		&database.BusinessAlertSettings{},
	))
	business := createAIWaiterBusiness(t, "takeover-"+t.Name()[len(t.Name())-8:], true)
	table := createAIWaiterTable(t, business.ID, "T-1")
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: "[]", IsActive: true, Version: 1}).Error)
	conv := createAIWaiterConversation(t, business.ID, "takeover-sess")
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)
	return db, business, conv, table
}

func guestAIWaiterRouter(t *testing.T) *gin.Engine {
	t.Helper()
	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	return router
}

func openTakeoverAlerts(t *testing.T, db *gorm.DB, businessID uint, convID uint) []database.OperationalAlert {
	t.Helper()
	var alerts []database.OperationalAlert
	require.NoError(t, db.Where(
		"business_id = ? AND alert_type = ? AND resource_type = ? AND resource_id = ?",
		businessID, database.OperationalAlertTypeAITakeover,
		database.OperationalAlertResourceTypeAIConversation, convID,
	).Find(&alerts).Error)
	return alerts
}

func TestHandleAIWaiter_PausedUnclaimed_CreatesTakeoverAlert(t *testing.T) {
	db, business, conv, table := setupAITakeoverAlertTest(t)
	require.NoError(t, db.Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).
		Update("is_paused", true).Error)

	router := guestAIWaiterRouter(t)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": conv.SessionID, "mode": "ordering", "table_code": table.TableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "hello? anyone there?"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	alerts := openTakeoverAlerts(t, db, business.ID, conv.ID)
	require.Len(t, alerts, 1)
	require.Equal(t, database.OperationalAlertStatusOpen, alerts[0].Status)
	require.Equal(t, database.OperationalAlertPriorityUrgent, alerts[0].Priority)
}

func TestHandleAIWaiter_PausedClaimed_NoTakeoverAlert(t *testing.T) {
	db, business, conv, table := setupAITakeoverAlertTest(t)
	staff := createAIWaiterStaff(t, business.ID, database.StaffRoleServer, "claimer@example.com")
	require.NoError(t, db.Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).
		Updates(map[string]interface{}{
			"is_paused":           true,
			"claimed_by_staff_id": staff.ID,
			"claimed_by_name":     staff.Name,
			"claimed_by_role":     string(staff.Role),
			"claimed_at":          time.Now(),
		}).Error)

	router := guestAIWaiterRouter(t)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": conv.SessionID, "mode": "ordering", "table_code": table.TableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "thanks!"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	require.Empty(t, openTakeoverAlerts(t, db, business.ID, conv.ID),
		"an operator already holds the claim — no alert")
}

func TestHandleAIWaiter_PausedClaimedByOwner_NoTakeoverAlert(t *testing.T) {
	db, business, conv, table := setupAITakeoverAlertTest(t)
	// An owner claim (via ClaimAiConversation with an owner principal) writes a
	// fresh claimed_at but a NULL claimed_by_staff_id.
	require.NoError(t, db.Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).
		Updates(map[string]interface{}{
			"is_paused":           true,
			"claimed_by_staff_id": nil,
			"claimed_by_name":     "Owner",
			"claimed_by_role":     "owner",
			"claimed_at":          time.Now(),
		}).Error)

	router := guestAIWaiterRouter(t)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": conv.SessionID, "mode": "ordering", "table_code": table.TableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "one more question"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	require.Empty(t, openTakeoverAlerts(t, db, business.ID, conv.ID),
		"the owner holds a fresh claim — no alert")
}

// seedOpenTakeoverAlert opens an ai_takeover alert for the conversation the way
// production does (guest wrote into a paused, unclaimed chat).
func seedOpenTakeoverAlert(t *testing.T, db *gorm.DB, businessID, convID uint) {
	t.Helper()
	require.NoError(t, operational_alerts.NewService(db).
		CreateAITakeoverAlert(context.Background(), businessID, int64(convID)))
	alerts := openTakeoverAlerts(t, db, businessID, convID)
	require.Len(t, alerts, 1)
	require.Equal(t, database.OperationalAlertStatusOpen, alerts[0].Status)
}

func requireTakeoverAlertResolved(t *testing.T, db *gorm.DB, businessID, convID uint) {
	t.Helper()
	alerts := openTakeoverAlerts(t, db, businessID, convID)
	require.Len(t, alerts, 1)
	require.Equal(t, database.OperationalAlertStatusResolved, alerts[0].Status)
}

// A guest messaging a conversation whose claim has gone stale must NOT be
// stranded behind the "a human is assisting you" ack forever: the stale claim
// is released inline (same semantics as the dashboard sweep), the AI answers
// again, and the open ai_takeover alert resolves — the guest is back with the AI.
func TestHandleAIWaiter_StaleClaim_ReleasesAndAIAnswers(t *testing.T) {
	db, business, conv, table := setupAITakeoverAlertTest(t)
	staff := createAIWaiterStaff(t, business.ID, database.StaffRoleServer, "gone@example.com")
	stale := time.Now().Add(-aiClaimIdleTTL - time.Minute)
	require.NoError(t, db.Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).
		Updates(map[string]interface{}{
			"is_paused":           true,
			"claimed_by_staff_id": staff.ID,
			"claimed_by_name":     staff.Name,
			"claimed_by_role":     string(staff.Role),
			"claimed_at":          stale,
		}).Error)
	seedOpenTakeoverAlert(t, db, business.ID, conv.ID)

	// Stub the model so the resumed-AI path can answer.
	stub := &stubWaiterProvider{resp: &llm.Response{Text: "AI is back with you!"}}
	aiSvc, err := services.NewAIService(stub, llm.ModelConfig{Chat: "test-chat", Image: "test-image", Director: "test-director", Menu: "test-menu"})
	require.NoError(t, err)
	SetAIService(aiSvc)
	t.Cleanup(func() { SetAIService(nil) })

	router := guestAIWaiterRouter(t)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": conv.SessionID, "mode": "ordering", "table_code": table.TableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "hello?"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// The guest gets a real AI answer, not the human-assisting ack.
	var resp struct {
		HumanAck bool `json:"human_ack"`
		Parts    []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.False(t, resp.HumanAck, "stale claim must be released, not acked as human-assisted")
	require.NotEmpty(t, resp.Parts)
	require.NotEmpty(t, resp.Parts[0].Text)
	require.Equal(t, "You're welcome — I'm happy to help.", resp.Parts[0].Text)
	require.NotEqual(t, "AI is back with you!", resp.Parts[0].Text)

	// Claim cleared + conversation unpaused.
	var reloaded database.AiWaiterConversation
	require.NoError(t, db.First(&reloaded, conv.ID).Error)
	require.Nil(t, reloaded.ClaimedByStaffID)
	require.Nil(t, reloaded.ClaimedAt)
	require.False(t, reloaded.IsPaused)

	// The takeover alert resolved — the guest is back with the AI.
	requireTakeoverAlertResolved(t, db, business.ID, conv.ID)
}

func TestToggleAiPause_UnpauseResolvesTakeoverAlert(t *testing.T) {
	db, business, conv, _ := setupAITakeoverAlertTest(t)
	staff := createAIWaiterStaff(t, business.ID, database.StaffRoleManager, "unpause@example.com")
	require.NoError(t, db.Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).
		Update("is_paused", true).Error)
	seedOpenTakeoverAlert(t, db, business.ID, conv.ID)

	router := aiStaffRouter(staff.ID, database.StaffRoleManager, business.ID)
	w := performAIWaiterRequest(t, router, http.MethodPost,
		fmt.Sprintf("/businesses/%d/ai/conversations/%d/pause", business.ID, conv.ID),
		map[string]any{"is_paused": false})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	requireTakeoverAlertResolved(t, db, business.ID, conv.ID)
}

func TestReleaseAiConversation_ResolvesTakeoverAlert(t *testing.T) {
	db, business, conv, _ := setupAITakeoverAlertTest(t)
	staff := createAIWaiterStaff(t, business.ID, database.StaffRoleServer, "release@example.com")
	router := aiStaffRouter(staff.ID, database.StaffRoleServer, business.ID)
	require.Equal(t, http.StatusOK,
		performAIWaiterRequest(t, router, http.MethodPost, claimPath(business.ID, conv.ID), nil).Code)
	// Alert opened while the conversation was paused/unattended.
	seedOpenTakeoverAlert(t, db, business.ID, conv.ID)

	w := performAIWaiterRequest(t, router, http.MethodPost,
		fmt.Sprintf("/businesses/%d/ai/conversations/%d/release", business.ID, conv.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	requireTakeoverAlertResolved(t, db, business.ID, conv.ID)
}

func TestCloseAiConversation_ResolvesAlertAndClearsClaim(t *testing.T) {
	db, business, conv, _ := setupAITakeoverAlertTest(t)
	staff := createAIWaiterStaff(t, business.ID, database.StaffRoleManager, "close@example.com")
	require.NoError(t, db.Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).
		Updates(map[string]interface{}{
			"is_paused":           true,
			"claimed_by_staff_id": staff.ID,
			"claimed_by_name":     "Closer",
			"claimed_by_role":     "manager",
			"claimed_at":          time.Now(),
		}).Error)
	seedOpenTakeoverAlert(t, db, business.ID, conv.ID)

	router := aiStaffRouter(staff.ID, database.StaffRoleManager, business.ID)
	w := performAIWaiterRequest(t, router, http.MethodPost,
		fmt.Sprintf("/businesses/%d/ai/conversations/%d/close", business.ID, conv.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var reloaded database.AiWaiterConversation
	require.NoError(t, db.First(&reloaded, conv.ID).Error)
	require.Equal(t, "closed", reloaded.Status)
	require.Nil(t, reloaded.ClaimedByStaffID, "closed row must not keep a claim")
	require.Nil(t, reloaded.ClaimedAt)
	require.Empty(t, reloaded.ClaimedByName)
	require.Empty(t, reloaded.ClaimedByRole)
	require.False(t, reloaded.IsPaused, "closed row must not stay paused")

	requireTakeoverAlertResolved(t, db, business.ID, conv.ID)
}

func TestPostAiReply_ResolvesTakeoverAlert(t *testing.T) {
	db, business, conv, _ := setupAITakeoverAlertTest(t)
	// A human answered, so the alert resolves. Decision #4 removed the
	// owner/manager reply-without-claiming bypass, so the manager holds the claim
	// here — this test is about alert resolution, not about who may reply
	// unclaimed (that rule is asserted in ai_dashboard_claim_test.go).
	staff := createAIWaiterStaff(t, business.ID, database.StaffRoleManager, "reply@example.com")
	require.NoError(t, db.Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).
		Updates(map[string]interface{}{
			"is_paused":           true,
			"claimed_by_staff_id": staff.ID,
			"claimed_by_name":     "Replier",
			"claimed_by_role":     "manager",
			"claimed_at":          time.Now(),
		}).Error)
	seedOpenTakeoverAlert(t, db, business.ID, conv.ID)

	router := aiStaffRouter(staff.ID, database.StaffRoleManager, business.ID)
	w := performAIWaiterRequest(t, router, http.MethodPost,
		fmt.Sprintf("/businesses/%d/ai/conversations/%d/reply", business.ID, conv.ID),
		map[string]any{"content": "I'm here, how can I help?"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	requireTakeoverAlertResolved(t, db, business.ID, conv.ID)
}

func TestGetAiConversations_SweepResolvesTakeoverAlerts(t *testing.T) {
	db, business, conv, _ := setupAITakeoverAlertTest(t)
	staff := createAIWaiterStaff(t, business.ID, database.StaffRoleManager, "sweep@example.com")
	stale := time.Now().Add(-aiClaimIdleTTL - time.Minute)
	require.NoError(t, db.Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).
		Updates(map[string]interface{}{
			"is_paused":           true,
			"claimed_by_staff_id": staff.ID,
			"claimed_by_name":     "Gone",
			"claimed_by_role":     "manager",
			"claimed_at":          stale,
		}).Error)
	seedOpenTakeoverAlert(t, db, business.ID, conv.ID)

	router := aiStaffRouter(staff.ID, database.StaffRoleManager, business.ID)
	w := performAIWaiterRequest(t, router, http.MethodGet,
		fmt.Sprintf("/businesses/%d/ai/conversations", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	requireTakeoverAlertResolved(t, db, business.ID, conv.ID)
}

func TestClaimAiConversation_ResolvesTakeoverAlert(t *testing.T) {
	db, business, conv, _ := setupAITakeoverAlertTest(t)
	staff := createAIWaiterStaff(t, business.ID, database.StaffRoleServer, "resolver@example.com")

	// Open takeover alert exists (guest wrote into a paused, unclaimed chat).
	require.NoError(t, db.Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).
		Update("is_paused", true).Error)
	router := guestAIWaiterRouter(t)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": conv.SessionID, "mode": "ordering", "table_code": conv.TableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "hello?"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	alerts := openTakeoverAlerts(t, db, business.ID, conv.ID)
	require.Len(t, alerts, 1)
	require.Equal(t, database.OperationalAlertStatusOpen, alerts[0].Status)

	// Operator claims the conversation → alert resolves.
	wc := performAIWaiterRequest(t, aiStaffRouter(staff.ID, database.StaffRoleServer, business.ID),
		http.MethodPost, claimPath(business.ID, conv.ID), nil)
	require.Equal(t, http.StatusOK, wc.Code, wc.Body.String())

	alerts = openTakeoverAlerts(t, db, business.ID, conv.ID)
	require.Len(t, alerts, 1)
	require.Equal(t, database.OperationalAlertStatusResolved, alerts[0].Status)
}
