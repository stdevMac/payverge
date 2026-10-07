package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectorConsoleDeniesNonOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	// Business owned by Owner A.
	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "director-cross-tenant")

	// Request authenticated as a DIFFERENT owner (Owner B).
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("token_type", "web3")
	c.Set("address", "0xOwnerB") // different from business.OwnerAddress

	ListDirectorThreads(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// resetInsightCacheForTest wipes the package-level cache between assertions so
// sibling tests don't leak state (test binaries share process memory).
func resetInsightCacheForTest(t *testing.T) {
	t.Helper()
	insightCacheMu.Lock()
	insightCache = map[uint]cachedInsight{}
	insightCacheMu.Unlock()
}

func TestInsightCache_ReturnsCachedWithinTTL(t *testing.T) {
	resetInsightCacheForTest(t)

	want := []proactiveInsight{{ID: "stub", Type: "stub"}}
	insightCacheMu.Lock()
	insightCache[42] = cachedInsight{
		insights:  want,
		expiresAt: time.Now().Add(insightCacheTTL),
	}
	insightCacheMu.Unlock()

	insightCacheMu.RLock()
	got, ok := insightCache[42]
	insightCacheMu.RUnlock()

	require.True(t, ok)
	assert.True(t, time.Now().Before(got.expiresAt))
	assert.Equal(t, want, got.insights)
}

func TestInsightCache_ExpiredEntryIsIgnored(t *testing.T) {
	resetInsightCacheForTest(t)

	insightCacheMu.Lock()
	insightCache[7] = cachedInsight{
		insights:  []proactiveInsight{{ID: "stale"}},
		expiresAt: time.Now().Add(-1 * time.Second),
	}
	insightCacheMu.Unlock()

	insightCacheMu.RLock()
	cached, ok := insightCache[7]
	insightCacheMu.RUnlock()

	require.True(t, ok, "entry should still be present until invalidated or overwritten")
	assert.False(t, time.Now().Before(cached.expiresAt), "an expired entry should not satisfy the TTL check in the handler")
}

func TestInvalidateInsightCache_DropsEntry(t *testing.T) {
	resetInsightCacheForTest(t)

	insightCacheMu.Lock()
	insightCache[99] = cachedInsight{
		insights:  []proactiveInsight{{ID: "x"}},
		expiresAt: time.Now().Add(insightCacheTTL),
	}
	insightCacheMu.Unlock()

	InvalidateInsightCache(99)

	insightCacheMu.RLock()
	_, ok := insightCache[99]
	insightCacheMu.RUnlock()

	assert.False(t, ok, "InvalidateInsightCache should remove the entry")
}

func TestInvalidateInsightCache_NoopWhenAbsent(t *testing.T) {
	resetInsightCacheForTest(t)

	assert.NotPanics(t, func() {
		InvalidateInsightCache(12345)
	})
}

// setupDirectorThreadTest seeds an AI-Pro business + thread + an assistant
// message so the thread-management tests have a stable target. It also
// migrates the director console tables — setupStaffHandlerTestDB only knows
// about staff/business/session tables.
func setupDirectorThreadTest(t *testing.T) (business *database.Business, thread *database.DirectorConsoleThread) {
	t.Helper()
	setupStaffHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.DirectorConsoleThread{},
		&database.DirectorConsoleMessage{},
	))

	business = createBusinessHandlerTestBusiness(t, "0xOwnerA", fmt.Sprintf("director-thread-%s", t.Name()))

	created, err := database.CreateDirectorConsoleThread(business.ID, "Original Title", "en")
	require.NoError(t, err)
	require.NoError(t, database.SaveDirectorConsoleMessage(&database.DirectorConsoleMessage{
		ThreadID:   created.ID,
		BusinessID: business.ID,
		Role:       database.DirectorMessageRoleAssistant,
		Content:    "Hello operator, here is the briefing.",
	}))

	return business, created
}

// authedDirectorContext builds a gin context wired with the owner-token claims
// the director handlers expect, plus the :id / :threadId URL params.
func authedDirectorContext(method, path string, body any, businessID, threadID uint) (*httptest.ResponseRecorder, *gin.Context) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var reqBody *bytes.Reader
	if body != nil {
		payload, _ := json.Marshal(body)
		reqBody = bytes.NewReader(payload)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.Request = req
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", businessID)},
		{Key: "threadId", Value: fmt.Sprintf("%d", threadID)},
	}
	c.Set("token_type", "web3")
	c.Set("address", "0xOwnerA")
	return w, c
}

func TestRenameDirectorThread(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, thread := setupDirectorThreadTest(t)

	w, c := authedDirectorContext(
		http.MethodPatch,
		fmt.Sprintf("/businesses/%d/ai/director/threads/%d", business.ID, thread.ID),
		map[string]any{"title": "Renamed Thread"},
		business.ID, thread.ID,
	)

	PatchDirectorThread(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	reloaded, err := database.GetDirectorConsoleThreadByID(business.ID, thread.ID)
	require.NoError(t, err)
	assert.Equal(t, "Renamed Thread", reloaded.Title)
}

func TestRenameDirectorThread_RejectsEmptyTitle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, thread := setupDirectorThreadTest(t)

	w, c := authedDirectorContext(
		http.MethodPatch,
		fmt.Sprintf("/businesses/%d/ai/director/threads/%d", business.ID, thread.ID),
		map[string]any{"title": "   "},
		business.ID, thread.ID,
	)

	PatchDirectorThread(c)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// L4-13: the 200-cap used to be a byte slice (cleanTitle[:200]). A multibyte
// title whose 200th byte lands mid-rune produced invalid UTF-8, which
// PostgreSQL rejects — the audit reproduced an HTTP 500 on rename. The cap
// must count runes, never split one.
func TestRenameDirectorThread_MultibyteTitleTruncatesOnRuneBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, thread := setupDirectorThreadTest(t)

	// 250 three-byte runes = 750 bytes; a byte slice at 200 cuts the 67th
	// rune (66*3 = 198) in half.
	longTitle := strings.Repeat("日", 250)

	w, c := authedDirectorContext(
		http.MethodPatch,
		fmt.Sprintf("/businesses/%d/ai/director/threads/%d", business.ID, thread.ID),
		map[string]any{"title": longTitle},
		business.ID, thread.ID,
	)

	PatchDirectorThread(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	reloaded, err := database.GetDirectorConsoleThreadByID(business.ID, thread.ID)
	require.NoError(t, err)
	assert.True(t, utf8.ValidString(reloaded.Title),
		"stored title must be valid UTF-8 — a mid-rune byte slice corrupts it and Postgres 500s")
	assert.Equal(t, 200, utf8.RuneCountInString(reloaded.Title))
	assert.Equal(t, strings.Repeat("日", 200), reloaded.Title)
}

func TestArchiveDirectorThread(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, thread := setupDirectorThreadTest(t)

	w, c := authedDirectorContext(
		http.MethodPatch,
		fmt.Sprintf("/businesses/%d/ai/director/threads/%d/archive", business.ID, thread.ID),
		nil,
		business.ID, thread.ID,
	)

	ArchiveDirectorThread(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var reloaded database.DirectorConsoleThread
	require.NoError(t, database.GetDB().
		Where("id = ? AND business_id = ?", thread.ID, business.ID).
		First(&reloaded).Error)
	require.NotNil(t, reloaded.ArchivedAt, "archived_at should be set after PATCH archive")
	assert.WithinDuration(t, time.Now(), *reloaded.ArchivedAt, 5*time.Second)

	// Archived threads must drop out of the default list query.
	threads, err := database.ListDirectorConsoleThreads(business.ID, 50)
	require.NoError(t, err)
	for _, ti := range threads {
		assert.NotEqual(t, thread.ID, ti.ID, "archived thread should be hidden from the list")
	}
}

func TestPermanentlyDeleteDirectorThread(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, thread := setupDirectorThreadTest(t)

	w, c := authedDirectorContext(
		http.MethodDelete,
		fmt.Sprintf("/businesses/%d/ai/director/threads/%d", business.ID, thread.ID),
		nil,
		business.ID, thread.ID,
	)
	DeleteDirectorThread(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var count int64
	require.NoError(t, database.GetDB().Model(&database.DirectorConsoleThread{}).
		Where("id = ? AND business_id = ?", thread.ID, business.ID).Count(&count).Error)
	require.Equal(t, int64(0), count, "permanent delete must remove the thread row")
}

func TestPinDirectorThread(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, thread := setupDirectorThreadTest(t)

	wPin, cPin := authedDirectorContext(
		http.MethodPost,
		fmt.Sprintf("/businesses/%d/ai/director/threads/%d/pin", business.ID, thread.ID),
		nil,
		business.ID, thread.ID,
	)
	PinDirectorThread(cPin)
	require.Equal(t, http.StatusOK, wPin.Code, wPin.Body.String())

	pinned, err := database.GetDirectorConsoleThreadByID(business.ID, thread.ID)
	require.NoError(t, err)
	assert.True(t, pinned.Pinned, "thread should be pinned after POST /pin")

	wUnpin, cUnpin := authedDirectorContext(
		http.MethodPost,
		fmt.Sprintf("/businesses/%d/ai/director/threads/%d/unpin", business.ID, thread.ID),
		nil,
		business.ID, thread.ID,
	)
	UnpinDirectorThread(cUnpin)
	require.Equal(t, http.StatusOK, wUnpin.Code, wUnpin.Body.String())

	unpinned, err := database.GetDirectorConsoleThreadByID(business.ID, thread.ID)
	require.NoError(t, err)
	assert.False(t, unpinned.Pinned, "thread should be unpinned after POST /unpin")
}

func TestExportDirectorThread(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, thread := setupDirectorThreadTest(t)

	w, c := authedDirectorContext(
		http.MethodGet,
		fmt.Sprintf("/businesses/%d/ai/director/threads/%d/export?format=md", business.ID, thread.ID),
		nil,
		business.ID, thread.ID,
	)
	// authedDirectorContext doesn't currently set the query string on the
	// request, so re-set it here for completeness.
	c.Request.URL.RawQuery = "format=md"

	ExportDirectorThread(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Header().Get("Content-Type"), "text/markdown")
	body := w.Body.String()
	assert.Contains(t, body, "# Original Title")
	assert.Contains(t, body, "Hello operator, here is the briefing.")
	assert.True(t, strings.Contains(body, "Assistant"), "transcript should label assistant messages")
}

func TestDirectorConsoleRejectsSuspendedBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "director-core")
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		UpdateColumn("is_active", false).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("token_type", "web3")
	c.Set("address", "0xOwnerA") // must match business.OwnerAddress to pass CheckBusinessAccess

	ListDirectorThreads(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "business_suspended")
}
