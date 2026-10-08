package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/agents"
	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type opsV2HandlerStub struct {
	result        *agents.OpsAskResult
	messages      []database.OpsAssistantMessage
	lastAsk       agents.OpsAskRequest
	listCallCount int
}

func (s *opsV2HandlerStub) Ask(_ context.Context, req agents.OpsAskRequest) (*agents.OpsAskResult, error) {
	s.lastAsk = req
	return s.result, nil
}

func (s *opsV2HandlerStub) ListMessages(uint, uint) ([]database.OpsAssistantMessage, error) {
	s.listCallCount++
	return s.messages, nil
}

func TestEnsureOpsAssistantDisabledWithoutService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	opsAssistantService = nil
	r := gin.New()
	r.GET("/threads/:threadId/messages", GetOpsAssistantThreadMessages)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/threads/1/messages", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", w.Code)
	}
}

func TestOpsAskV2EnabledHandlerReturnsCompatibilityAndTypedEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupOpsStubDB(t)
	business := createOwnedBusiness(t, "0xOpsV2Owner", "Ops V2")
	require.NoError(t, database.GetDB().Save(business).Error)

	v2 := assistantcontract.NewResponse("ops-handler-v2", "Structured answer")
	v2JSON, err := json.Marshal(v2)
	require.NoError(t, err)
	stub := &opsV2HandlerStub{result: &agents.OpsAskResult{
		Thread: database.OpsAssistantThread{ID: 7, BusinessID: business.ID},
		AssistantMessage: database.OpsAssistantMessage{
			ID: 9, ThreadID: 7, BusinessID: business.ID, Role: database.OpsAssistantRoleAssistant,
			StructuredResponse: string(v2JSON),
		},
		Response:   agents.StructuredResponse{Answer: "Structured answer"},
		ResponseV2: v2,
	}}
	SetOpsAssistantService(stub)
	defer SetOpsAssistantService(nil)

	body, err := json.Marshal(map[string]any{
		"message": "How do I configure AI Waiter?", "locale": "en", "active_tab": "overview",
	})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("token_type", "web3")
	c.Set("address", "0xOpsV2Owner")
	c.Set("business_owner_address", "0xOpsV2Owner")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	AskOpsAssistant(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Contains(t, payload, "response")
	assert.Contains(t, payload, "response_v2")
	var wireV2 assistantcontract.Response
	require.NoError(t, json.Unmarshal(payload["response_v2"], &wireV2))
	assert.Equal(t, v2, wireV2)
	var wireMessage database.OpsAssistantMessage
	require.NoError(t, json.Unmarshal(payload["assistant_message"], &wireMessage))
	assert.Empty(t, wireMessage.StructuredResponse, "persistence shadow must never be a nested wire channel")
	assert.False(t, stub.lastAsk.Access.Suspended)
	assert.True(t, stub.lastAsk.Access.EffectivePermissions["ai_waiter:read"], "owner access must be assembled server-side")
}

func TestOpsHistoryReturnsParsedV2AndSafeLegacyProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupOpsStubDB(t)
	business := createOwnedBusiness(t, "0xOpsHistoryOwner", "Ops History")
	v2 := assistantcontract.NewResponse("ops-history-v2", "V2 answer")
	v2JSON, err := json.Marshal(v2)
	require.NoError(t, err)
	stub := &opsV2HandlerStub{messages: []database.OpsAssistantMessage{
		{ID: 1, ThreadID: 8, BusinessID: business.ID, Role: database.OpsAssistantRoleAssistant, Content: "V2 answer", StructuredResponse: string(v2JSON)},
		{ID: 2, ThreadID: 8, BusinessID: business.ID, Role: database.OpsAssistantRoleAssistant, Content: "Legacy answer", StructuredResponse: `{"answer":"Legacy answer","steps":[],"actions":[],"follow_ups":[]}`},
		{ID: 3, ThreadID: 8, BusinessID: business.ID, Role: database.OpsAssistantRoleAssistant, Content: "Readable fallback", StructuredResponse: `{"answer":`},
	}}
	SetOpsAssistantService(stub)
	defer SetOpsAssistantService(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("token_type", "web3")
	c.Set("address", "0xOpsHistoryOwner")
	c.Set("business_owner_address", "0xOpsHistoryOwner")
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprint(business.ID)},
		{Key: "threadId", Value: "8"},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetOpsAssistantThreadMessages(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var payload struct {
		Messages []struct {
			ID                 uint                       `json:"id"`
			Content            string                     `json:"content"`
			StructuredResponse agents.StructuredResponse  `json:"structured_response"`
			ResponseV2         assistantcontract.Response `json:"response_v2"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Len(t, payload.Messages, 3)
	assert.Equal(t, v2, payload.Messages[0].ResponseV2)
	assert.Equal(t, "Legacy answer", payload.Messages[1].StructuredResponse.Answer)
	require.NoError(t, assistantcontract.Validate(payload.Messages[1].ResponseV2))
	assert.Equal(t, "Readable fallback", payload.Messages[2].StructuredResponse.Answer)
	require.NoError(t, assistantcontract.Validate(payload.Messages[2].ResponseV2))
	assert.Empty(t, payload.Messages[2].ResponseV2.Actions)
}

func TestOpsAskReplayReusesPersistedAssistantMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupOpsStubDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.OpsAssistantRequest{}))
	business := createOwnedBusiness(t, "0xOpsReplayV1Owner", "Ops replay V1 rollout")
	require.NoError(t, database.GetDB().Save(business).Error)
	service := agents.NewOpsAssistantService(nil, agents.NewRegistry(), database.GetDBWrapper(), nil)
	SetOpsAssistantService(service)
	t.Cleanup(func() { SetOpsAssistantService(nil) })

	body, err := json.Marshal(map[string]any{
		"message": "How do I add a menu item?", "locale": "en",
		"client_request_id": "rollout-replay-request",
	})
	require.NoError(t, err)
	request := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("token_type", "web3")
		c.Set("address", "0xOpsReplayV1Owner")
		c.Set("business_owner_address", "0xOpsReplayV1Owner")
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(business.ID)}}
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		AskOpsAssistant(c)
		return w
	}
	assertDualWire := func(w *httptest.ResponseRecorder) database.OpsAssistantMessage {
		t.Helper()
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var payload struct {
			Response         agents.StructuredResponse    `json:"response"`
			ResponseV2       *assistantcontract.Response  `json:"response_v2"`
			AssistantMessage database.OpsAssistantMessage `json:"assistant_message"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
		require.NotEmpty(t, payload.Response.Answer)
		require.NotNil(t, payload.ResponseV2)
		require.Empty(t, payload.AssistantMessage.StructuredResponse,
			"the raw persisted envelope must stay off the wire; clients read response_v2")
		return payload.AssistantMessage
	}

	first := assertDualWire(request())
	require.NotZero(t, first.ID)
	var persisted database.OpsAssistantMessage
	require.NoError(t, database.GetDB().First(&persisted, first.ID).Error)
	require.NotEmpty(t, persisted.StructuredResponse, "the validated V2 envelope must be durably persisted")
	var persistedV2 assistantcontract.Response
	require.NoError(t, json.Unmarshal([]byte(persisted.StructuredResponse), &persistedV2))
	require.NoError(t, assistantcontract.Validate(persistedV2))

	replayed := assertDualWire(request())
	require.Equal(t, first.ID, replayed.ID, "same client request must use the durable replay path")
	var assistantCount int64
	require.NoError(t, database.GetDB().Model(&database.OpsAssistantMessage{}).
		Where("business_id = ? AND role = ?", business.ID, database.OpsAssistantRoleAssistant).
		Count(&assistantCount).Error)
	require.Equal(t, int64(1), assistantCount, "replay must not create another assistant response")
}

func TestAskOpsAssistant_TurnOnAIWaiterDescribesDinerUI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupOpsStubDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.OpsAssistantRequest{}))
	business := createOwnedBusiness(t, "0xOpsDinerUIOwner", "Ops diner UI")
	require.NoError(t, database.GetDB().Save(business).Error)
	SetOpsAssistantService(agents.NewOpsAssistantService(nil, agents.NewRegistry(), database.GetDBWrapper(), nil))
	t.Cleanup(func() { SetOpsAssistantService(nil) })

	body, err := json.Marshal(map[string]any{
		"message": "How do I turn on the AI Waiter… what will diners see?",
		"locale":  "en", "active_tab": "overview",
	})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("token_type", "web3")
	c.Set("address", "0xOpsDinerUIOwner")
	c.Set("business_owner_address", "0xOpsDinerUIOwner")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	AskOpsAssistant(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var payload struct {
		Usage      agents.Usage               `json:"usage"`
		Response   agents.StructuredResponse  `json:"response"`
		ResponseV2 assistantcontract.Response `json:"response_v2"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, "deterministic-guide", payload.Usage.Model)
	lower := strings.ToLower(payload.Response.Answer + " " + payload.ResponseV2.Answer.Content)
	assert.Contains(t, lower, "qr")
	assert.Contains(t, lower, "sage")
	assert.NotContains(t, lower, "setup progress")
	assert.NotContains(t, lower, "set name and priority")
	// #874: a lone guide is the answer itself; its identity rides on the source.
	assert.Empty(t, payload.ResponseV2.Sections)
	require.Len(t, payload.ResponseV2.Sources, 1)
	assert.Equal(t, "guide:guest-order-experience", payload.ResponseV2.Sources[0].ID)
}

func TestOpsHistoryCrossBusinessAccessIsForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupOpsStubDB(t)
	createOwnedBusiness(t, "0xFirstOwner", "First")
	other := createOwnedBusiness(t, "0xOtherOwner", "Other")
	stub := &opsV2HandlerStub{}
	SetOpsAssistantService(stub)
	defer SetOpsAssistantService(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("token_type", "web3")
	c.Set("address", "0xFirstOwner")
	c.Set("business_owner_address", "0xFirstOwner")
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprint(other.ID)},
		{Key: "threadId", Value: "99"},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetOpsAssistantThreadMessages(c)

	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Zero(t, stub.listCallCount, "history must not be queried across businesses")
}

func TestOpsAccessV2SnapshotHonorsOwnerAndStaffWildcardDeny(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rbacMiddleware = &RBACMiddleware{db: nil}
	business := &database.Business{IsActive: true}

	owner, _ := gin.CreateTestContext(httptest.NewRecorder())
	owner.Set("token_type", "web3")
	owner.Set("address", "0xOwner")
	owner.Set("business_owner_address", "0xOwner")
	ownerSnapshot := buildOpsAccessSnapshot(owner, business)
	assert.True(t, ownerSnapshot.EffectivePermissions["ai_waiter:read"])
	assert.True(t, ownerSnapshot.EffectivePermissions["plugins:write"])

	staff, _ := gin.CreateTestContext(httptest.NewRecorder())
	staff.Set("token_type", "staff")
	staff.Set("staff_role", string(database.StaffRoleKitchen))
	staff.Set("staff_id", uint(42))
	staff.Set("staff_custom_permissions", `["plugins:*","ai_waiter:*"]`)
	staff.Set("staff_permission_denies", []string{"ai_waiter:read", "menu:read"})
	staffBusiness := *business
	staffSnapshot := buildOpsAccessSnapshot(staff, &staffBusiness)
	assert.True(t, staffSnapshot.EffectivePermissions["plugins:write"], "wildcard grant must satisfy the concrete guide permission")
	assert.False(t, staffSnapshot.EffectivePermissions["ai_waiter:read"], "exact deny must override wildcard grant")
	assert.True(t, staffSnapshot.HiddenGuideIDs["ai-waiter-configure"], "staff-hidden plan areas must be represented in the finalizer snapshot")
	assert.True(t, staffSnapshot.HiddenGuideIDs["menu-add-item"], "a staff member without destination menu:read must not see menu guidance")
	assert.Empty(t, ownerSnapshot.HiddenGuideIDs, "owners retain readable locked guidance")

	readOnlyMenu, _ := gin.CreateTestContext(httptest.NewRecorder())
	readOnlyMenu.Set("token_type", "staff")
	readOnlyMenu.Set("staff_role", string(database.StaffRoleKitchen))
	readOnlyMenu.Set("staff_id", uint(43))
	readOnlyMenu.Set("staff_custom_permissions", `["menu:read"]`)
	readOnlyMenu.Set("staff_permission_denies", []string{})
	readOnlySnapshot := buildOpsAccessSnapshot(readOnlyMenu, &staffBusiness)
	assert.False(t, readOnlySnapshot.HiddenGuideIDs["menu-add-item"], "destination read permission keeps guidance visible")
	assert.False(t, readOnlySnapshot.EffectivePermissions["menu:write"], "write action remains independently disabled")
}

func TestOpsAccessV2SnapshotUsesCanonicalLockState(t *testing.T) {
	closedAt := time.Now().Add(-time.Hour)
	tests := []struct {
		name      string
		business  database.Business
		suspended bool
	}{
		{name: "active", business: database.Business{IsActive: true}, suspended: false},
		{name: "suspended by administrator", business: database.Business{IsActive: false}, suspended: true},
		{name: "closed by administrator", business: database.Business{IsActive: true, ClosedAt: &closedAt}, suspended: true},
		{name: "demo remains active", business: database.Business{IsDemo: true}, suspended: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Set("token_type", "web3")
			ctx.Set("address", "0xOwner")
			ctx.Set("business_owner_address", "0xOwner")
			snapshot := buildOpsAccessSnapshot(ctx, &tt.business)
			assert.Equal(t, tt.suspended, snapshot.Suspended)
			assert.Equal(t, tt.suspended, buildOpsToolEnv(ctx, &tt.business).IsSuspended)
		})
	}
}
