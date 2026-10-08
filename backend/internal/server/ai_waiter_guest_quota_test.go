package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// SEC H-ai-cost regression tests for the public AI waiter: budget checks run
// before the guest turn is persisted, the guest USD scope (not the owner's)
// gates guests, and per-network / per-device daily caps bound one client.

type guestQuotaFixture struct {
	db       *gorm.DB
	router   *gin.Engine
	path     string
	token    string
	table    string
	business *database.Business
}

func newGuestQuotaFixture(t *testing.T, suffix string) *guestQuotaFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("AI_WAITER_DAILY_MESSAGE_BUDGET", "100000")
	db := setupAIWaiterTestDB(t)
	token := suffix + "-token"
	business, tableCode := seedAIWaiterChatBusiness(t, db, suffix, token)
	svc, _ := services.NewAIService(okProvider{}, llm.ModelConfig{Chat: "m", Image: "i", Director: "d", Menu: "mn"})
	SetAIService(svc)
	t.Cleanup(func() { SetAIService(nil) })
	t.Cleanup(guestAIQuota.Reset)
	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	router.POST("/ai-waiter/:businessId/session", CreateAIWaiterSession)
	return &guestQuotaFixture{db: db, router: router, path: fmt.Sprintf("/ai-waiter/%d", business.ID), token: token, table: tableCode, business: business}
}

func (f *guestQuotaFixture) ask(t *testing.T, ip, deviceCookie, content string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"session_token": f.token, "mode": "ordering", "table_code": f.table, "language": "en",
		"history": []map[string]any{{"role": "user", "content": content}},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, f.path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = net.JoinHostPort(ip, "51000")
	if deviceCookie != "" {
		req.AddCookie(&http.Cookie{Name: aiDeviceCookieName, Value: deviceCookie})
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}

func (f *guestQuotaFixture) userTurns(t *testing.T, content string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, f.db.Model(&database.AiWaiterMessage{}).
		Where("role = ? AND content = ?", "user", content).Count(&n).Error)
	return n
}

func requireOverBudget(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"code":"over_budget"`)
}

// Before: the guest message was saved (and the business daily counter bumped)
// before the budget check, so an over-budget client kept growing
// ai_waiter_messages with every refused request.
func TestHandleAIWaiter_OverBudgetTurnIsNotPersisted(t *testing.T) {
	f := newGuestQuotaFixture(t, "persist-order")
	conv, ok := database.FindAiWaiterConversation(f.token, f.business.ID, "ordering", f.table)
	require.True(t, ok)
	// Historical semantics (save, then refuse once the stored count reaches the
	// cap) admit a turn only while existing+1 < cap.
	for i := 0; i < maxAIWaiterMessagesPerSession-2; i++ {
		require.NoError(t, f.db.Create(&database.AiWaiterMessage{ConversationID: conv.ID, Role: "assistant", Content: "x", CreatedAt: time.Now().UTC()}).Error)
	}
	// One slot left: this turn is admitted and saved (cap counts the pending turn).
	w := f.ask(t, "198.51.100.20", "", "last allowed")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.EqualValues(t, 1, f.userTurns(t, "last allowed"))

	before := database.CountAiWaiterMessages(conv.ID)
	for i := 0; i < 5; i++ {
		requireOverBudget(t, f.ask(t, "198.51.100.20", "", "flood"))
	}
	assert.Zero(t, f.userTurns(t, "flood"), "refused turns must not be persisted")
	assert.Equal(t, before, database.CountAiWaiterMessages(conv.ID))
}

func TestHandleAIWaiter_BusinessDailyBudgetRefusalIsNotPersisted(t *testing.T) {
	f := newGuestQuotaFixture(t, "persist-daily")
	t.Setenv("AI_WAITER_DAILY_MESSAGE_BUDGET", "1")
	requireOverBudget(t, f.ask(t, "198.51.100.21", "", "over daily"))
	assert.Zero(t, f.userTurns(t, "over daily"))
}

// Guests are gated by the guest scope; the owner's scope being exhausted must
// not lock guests out, and vice versa.
func TestHandleAIWaiter_UsesGuestDollarScopeNotOwnerScope(t *testing.T) {
	f := newGuestQuotaFixture(t, "guest-scope")
	t.Cleanup(func() { SetAICostGate(nil); SetGuestAICostGate(nil) })

	SetAICostGate(stubBudgetGate{over: true})
	SetGuestAICostGate(stubBudgetGate{over: false})
	w := f.ask(t, "198.51.100.30", "", "owner scope full")
	require.Equal(t, http.StatusOK, w.Code, "an exhausted owner scope must not block guests: %s", w.Body.String())

	SetAICostGate(stubBudgetGate{over: false})
	SetGuestAICostGate(stubBudgetGate{over: true})
	requireOverBudget(t, f.ask(t, "198.51.100.30", "", "guest scope full"))
	assert.Zero(t, f.userTurns(t, "guest scope full"))
}

func TestHandleAIWaiter_PerNetworkDailyCap(t *testing.T) {
	f := newGuestQuotaFixture(t, "ip-cap")
	t.Setenv(envAIWaiterDailyPerIP, "2")
	for i := 0; i < 2; i++ {
		require.Equal(t, http.StatusOK, f.ask(t, "203.0.113.70", "", fmt.Sprintf("ok %d", i)).Code)
	}
	requireOverBudget(t, f.ask(t, "203.0.113.70", "", "third"))
	assert.Zero(t, f.userTurns(t, "third"))
	// Another network is unaffected.
	require.Equal(t, http.StatusOK, f.ask(t, "203.0.113.71", "", "other network").Code)
	// An IPv6 client cannot rotate addresses inside its /64.
	require.Equal(t, http.StatusOK, f.ask(t, "2001:db8:5:6::1", "", "v6 a").Code)
	require.Equal(t, http.StatusOK, f.ask(t, "2001:db8:5:6::2", "", "v6 b").Code)
	requireOverBudget(t, f.ask(t, "2001:db8:5:6::3", "", "v6 c"))
}

func TestHandleAIWaiter_PerDeviceDailyCap(t *testing.T) {
	f := newGuestQuotaFixture(t, "device-cap")
	t.Setenv(envAIWaiterDailyPerDevice, "1")
	devA, ok := aiDeviceSigner.Mint()
	require.True(t, ok)
	devB, _ := aiDeviceSigner.Mint()

	require.Equal(t, http.StatusOK, f.ask(t, "192.0.2.90", devA, "dev a 1").Code)
	requireOverBudget(t, f.ask(t, "192.0.2.90", devA, "dev a 2"))
	// A second diner on the same venue Wi-Fi keeps working.
	require.Equal(t, http.StatusOK, f.ask(t, "192.0.2.90", devB, "dev b 1").Code)
	// A forged device id is ignored (only the per-network cap applies), so it
	// cannot be used to burn another device's quota or to dodge the cap.
	require.Equal(t, http.StatusOK, f.ask(t, "192.0.2.90", "dv_forged", "forged").Code)
	assert.Zero(t, guestAIQuota.Count("waiter-dev:dv_forged"))
}

func TestCreateAIWaiterSession_MintsSignedDeviceCookieOnce(t *testing.T) {
	f := newGuestQuotaFixture(t, "device-mint")
	post := func(cookie string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"table_code": f.table, "mode": "ordering", "language": "en"})
		req := httptest.NewRequest(http.MethodPost, f.path+"/session", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: aiDeviceCookieName, Value: cookie})
		}
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, req)
		return w
	}
	deviceCookie := func(w *httptest.ResponseRecorder) *http.Cookie {
		for _, ck := range w.Result().Cookies() {
			if ck.Name == aiDeviceCookieName {
				return ck
			}
		}
		return nil
	}

	w := post("")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	ck := deviceCookie(w)
	require.NotNil(t, ck, "session creation must mint the device cookie")
	assert.True(t, aiDeviceSigner.Valid(ck.Value))
	assert.True(t, ck.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, ck.SameSite)
	raw := strings.Join(w.Header().Values("Set-Cookie"), "\n")
	assert.Contains(t, raw, "pv_ai_waiter_session=", "session cookie still issued")

	// A valid device cookie is kept, not rotated (rotation would reset the cap).
	w = post(ck.Value)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, deviceCookie(w))

	// A forged one is replaced by a signed one.
	w = post("dv_forged")
	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, deviceCookie(w))
	assert.True(t, aiDeviceSigner.Valid(deviceCookie(w).Value))
}

// stubBudgetGate is a DollarBudgetGate with a fixed answer.
type stubBudgetGate struct{ over bool }

func (s stubBudgetGate) OverBudget(uint) bool { return s.over }
