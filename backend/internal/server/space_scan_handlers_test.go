package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/spaces"
	"github.com/stdevmac/payverge/backend/internal/spaces/scan"
)

func setupSpaceScanHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	disableAsyncOnboardingStampForTest(t)
	// Session IDs restart per test DB; give each test a fresh pair-attempt guard.
	origPairGuard := spaceScanPairFailures
	spaceScanPairFailures = newSpaceScanPairAttemptGuard(spaceScanMaxPairFailures)
	t.Cleanup(func() { spaceScanPairFailures = origPairGuard })

	dsn := fmt.Sprintf("file:spacescan-%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.StaffPermissionDeny{},
		&database.Table{},
		&database.RestaurantSpace{},
		&database.SpaceScanSession{},
		&database.SpaceScanUpload{},
		&database.SpaceLayoutAuditEvent{},
	))
	InitializeRBAC(database.GetDBWrapper())

	// Local artifact store for tests (no S3).
	dir := filepath.Join(t.TempDir(), "space-scans")
	store, err := scan.NewLocalArtifactStore(dir)
	require.NoError(t, err)
	SetSpaceScanArtifactStore(store)

	return gormDB
}

func TestCreateScanSession_ReturnsTokenOnce(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-create")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "Scan Room", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	c, w := spaceOwnerContext(t, biz, http.MethodPost,
		fmt.Sprintf("/inside/businesses/%s/spaces/%d/scan-sessions", biz.BusinessId, space.ID),
		map[string]any{},
	)
	c.Params = gin.Params{
		{Key: "id", Value: biz.BusinessId},
		{Key: "spaceId", Value: fmt.Sprintf("%d", space.ID)},
	}
	CreateSpaceScanSession(c)
	require.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	token, _ := resp["token"].(string)
	pair, _ := resp["pair_code"].(string)
	require.NotEmpty(t, token)
	require.NotEmpty(t, pair)
	require.GreaterOrEqual(t, len(token), 32)

	// Token is not stored raw — only hash.
	sessionMap := resp["session"].(map[string]any)
	assert.NotContains(t, w.Body.String(), `"token_hash"`)
	assert.NotZero(t, sessionMap["id"])
}

func TestPublicScanMeta_AndExpiry(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-meta")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "Patio", SpaceType: "outdoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	// Create session via handler
	c, w := spaceOwnerContext(t, biz, http.MethodPost,
		fmt.Sprintf("/inside/businesses/%s/spaces/%d/scan-sessions", biz.BusinessId, space.ID),
		map[string]any{},
	)
	c.Params = gin.Params{
		{Key: "id", Value: biz.BusinessId},
		{Key: "spaceId", Value: fmt.Sprintf("%d", space.ID)},
	}
	CreateSpaceScanSession(c)
	require.Equal(t, http.StatusCreated, w.Code)
	var created map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	token := created["token"].(string)

	// Public meta
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{{Key: "token", Value: token}}
	c2.Request = httptest.NewRequest(http.MethodGet, "/api/v1/space-scan/"+token, nil)
	PublicSpaceScanSessionMeta(c2)
	require.Equal(t, http.StatusOK, w2.Code)
	assert.Contains(t, w2.Body.String(), "Patio")
	assert.Contains(t, w2.Body.String(), biz.Name)
	// Must not leak internal ids
	assert.NotContains(t, w2.Body.String(), `"business_id"`)
	assert.NotContains(t, w2.Body.String(), `"space_id"`)

	// Force expire
	sid := uint(created["session"].(map[string]any)["id"].(float64))
	past := time.Now().UTC().Add(-time.Hour)
	require.NoError(t, database.GetDB().Model(&database.SpaceScanSession{}).
		Where("id = ?", sid).Update("expires_at", past).Error)

	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Params = gin.Params{{Key: "token", Value: token}}
	c3.Request = httptest.NewRequest(http.MethodGet, "/api/v1/space-scan/"+token, nil)
	PublicSpaceScanSessionMeta(c3)
	require.Equal(t, http.StatusGone, w3.Code)
	assert.Contains(t, w3.Body.String(), "expired")
}

func TestPublicScanConnect_PairCode(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-pair")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	c, w := spaceOwnerContext(t, biz, http.MethodPost,
		fmt.Sprintf("/inside/businesses/%s/spaces/%d/scan-sessions", biz.BusinessId, space.ID),
		map[string]any{},
	)
	c.Params = gin.Params{
		{Key: "id", Value: biz.BusinessId},
		{Key: "spaceId", Value: fmt.Sprintf("%d", space.ID)},
	}
	CreateSpaceScanSession(c)
	var created map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	token := created["token"].(string)
	pair := created["pair_code"].(string)

	// Bad pair code
	wBad := httptest.NewRecorder()
	cBad, _ := gin.CreateTestContext(wBad)
	bodyBad, _ := json.Marshal(map[string]any{"pair_code": "000000"})
	cBad.Params = gin.Params{{Key: "token", Value: token}}
	cBad.Request = httptest.NewRequest(http.MethodPost, "/api/v1/space-scan/"+token+"/connect", bytes.NewReader(bodyBad))
	cBad.Request.Header.Set("Content-Type", "application/json")
	PublicSpaceScanConnect(cBad)
	require.Equal(t, http.StatusUnauthorized, wBad.Code)

	// Good pair code
	wOk := httptest.NewRecorder()
	cOk, _ := gin.CreateTestContext(wOk)
	bodyOk, _ := json.Marshal(map[string]any{"pair_code": pair})
	cOk.Params = gin.Params{{Key: "token", Value: token}}
	cOk.Request = httptest.NewRequest(http.MethodPost, "/api/v1/space-scan/"+token+"/connect", bytes.NewReader(bodyOk))
	cOk.Request.Header.Set("Content-Type", "application/json")
	PublicSpaceScanConnect(cOk)
	require.Equal(t, http.StatusOK, wOk.Code)
	assert.Contains(t, wOk.Body.String(), database.ScanStatusPhoneConnected)
}

func TestPublicScanConnect_OwnerSessionWithoutPairCode(t *testing.T) {
	// Logged-in owner on the phone: connect via address context (optional hybrid auth
	// hydrates this from JWT/cookies in production; tests set it directly).
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-owner-auth")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R2", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	c, w := spaceOwnerContext(t, biz, http.MethodPost,
		fmt.Sprintf("/inside/businesses/%s/spaces/%d/scan-sessions", biz.BusinessId, space.ID),
		map[string]any{},
	)
	c.Params = gin.Params{
		{Key: "id", Value: biz.BusinessId},
		{Key: "spaceId", Value: fmt.Sprintf("%d", space.ID)},
	}
	CreateSpaceScanSession(c)
	var created map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	token := created["token"].(string)

	wOk := httptest.NewRecorder()
	cOk, _ := gin.CreateTestContext(wOk)
	bodyOk, _ := json.Marshal(map[string]any{"device_meta": map[string]any{"auth_path": "session"}})
	cOk.Params = gin.Params{{Key: "token", Value: token}}
	cOk.Request = httptest.NewRequest(http.MethodPost, "/api/v1/space-scan/"+token+"/connect", bytes.NewReader(bodyOk))
	cOk.Request.Header.Set("Content-Type", "application/json")
	cOk.Set("address", biz.OwnerAddress)
	PublicSpaceScanConnect(cOk)
	require.Equal(t, http.StatusOK, wOk.Code, wOk.Body.String())
	assert.Contains(t, wOk.Body.String(), database.ScanStatusPhoneConnected)
}

func TestPublicScanUpload_Idempotent(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-up")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	c, w := spaceOwnerContext(t, biz, http.MethodPost,
		fmt.Sprintf("/inside/businesses/%s/spaces/%d/scan-sessions", biz.BusinessId, space.ID),
		map[string]any{},
	)
	c.Params = gin.Params{
		{Key: "id", Value: biz.BusinessId},
		{Key: "spaceId", Value: fmt.Sprintf("%d", space.ID)},
	}
	CreateSpaceScanSession(c)
	var created map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	token := created["token"].(string)
	pair := created["pair_code"].(string)

	// Connect first
	bodyConn, _ := json.Marshal(map[string]any{"pair_code": pair})
	wConn := httptest.NewRecorder()
	cConn, _ := gin.CreateTestContext(wConn)
	cConn.Params = gin.Params{{Key: "token", Value: token}}
	cConn.Request = httptest.NewRequest(http.MethodPost, "/connect", bytes.NewReader(bodyConn))
	cConn.Request.Header.Set("Content-Type", "application/json")
	PublicSpaceScanConnect(cConn)
	require.Equal(t, http.StatusOK, wConn.Code)

	payload := map[string]any{
		"upload_kind": "roomplan_json",
		"payload": map[string]any{
			"unit":       "m",
			"dimensions": map[string]any{"width": 5, "height": 4},
			"walls":      []any{},
			"objects":    []any{},
		},
	}
	body, _ := json.Marshal(payload)

	doUpload := func() *httptest.ResponseRecorder {
		wU := httptest.NewRecorder()
		cU, _ := gin.CreateTestContext(wU)
		cU.Params = gin.Params{{Key: "token", Value: token}}
		cU.Request = httptest.NewRequest(http.MethodPost, "/uploads", bytes.NewReader(body))
		cU.Request.Header.Set("Content-Type", "application/json")
		cU.Request.Header.Set("Idempotency-Key", "upload-key-1")
		PublicSpaceScanUpload(cU)
		return wU
	}

	w1 := doUpload()
	require.Equal(t, http.StatusCreated, w1.Code, w1.Body.String())
	w2 := doUpload()
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	assert.Contains(t, w2.Body.String(), `"idempotent":true`)
}

func TestPublicScanUpload_InvalidPayload(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-inv")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	rawToken, err := generateOpaqueScanToken()
	require.NoError(t, err)
	sid := space.ID
	session := &database.SpaceScanSession{
		BusinessID: biz.ID,
		SpaceID:    &sid,
		Status:     database.ScanStatusPhoneConnected,
		ExpiresAt:  time.Now().UTC().Add(30 * time.Minute),
	}
	require.NoError(t, database.CreateSpaceScanSession(session, rawToken))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: rawToken}}
	body, _ := json.Marshal(map[string]any{"upload_kind": "not_a_real_kind", "payload": map[string]any{}})
	c.Request = httptest.NewRequest(http.MethodPost, "/uploads", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	PublicSpaceScanUpload(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCancelRevokesToken(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-cancel")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	c, w := spaceOwnerContext(t, biz, http.MethodPost,
		fmt.Sprintf("/inside/businesses/%s/spaces/%d/scan-sessions", biz.BusinessId, space.ID),
		map[string]any{},
	)
	c.Params = gin.Params{
		{Key: "id", Value: biz.BusinessId},
		{Key: "spaceId", Value: fmt.Sprintf("%d", space.ID)},
	}
	CreateSpaceScanSession(c)
	var created map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	token := created["token"].(string)
	sid := uint(created["session"].(map[string]any)["id"].(float64))

	c2, w2 := spaceOwnerContext(t, biz, http.MethodPost,
		fmt.Sprintf("/inside/businesses/%s/scan-sessions/%d/cancel", biz.BusinessId, sid), nil)
	c2.Params = gin.Params{
		{Key: "id", Value: biz.BusinessId},
		{Key: "sessionId", Value: fmt.Sprintf("%d", sid)},
	}
	CancelSpaceScanSession(c2)
	require.Equal(t, http.StatusOK, w2.Code)

	// Token no longer resolves
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Params = gin.Params{{Key: "token", Value: token}}
	c3.Request = httptest.NewRequest(http.MethodGet, "/api/v1/space-scan/"+token, nil)
	PublicSpaceScanSessionMeta(c3)
	require.Equal(t, http.StatusNotFound, w3.Code)
}

func TestCrossTenantScanSessionDenied(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	owner := createSpaceHandlerBusiness(t, "scan-own")
	other := createSpaceHandlerBusiness(t, "scan-oth")
	space := &database.RestaurantSpace{BusinessID: other.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	rawToken, err := generateOpaqueScanToken()
	require.NoError(t, err)
	sid := space.ID
	session := &database.SpaceScanSession{
		BusinessID: other.ID,
		SpaceID:    &sid,
		Status:     database.ScanStatusWaitingForPhone,
		ExpiresAt:  time.Now().UTC().Add(30 * time.Minute),
	}
	require.NoError(t, database.CreateSpaceScanSession(session, rawToken))

	// Owner of different business cannot read other session via inside API
	c, w := spaceOwnerContext(t, owner, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%s/scan-sessions/%d", other.BusinessId, session.ID), nil)
	c.Params = gin.Params{
		{Key: "id", Value: other.BusinessId},
		{Key: "sessionId", Value: fmt.Sprintf("%d", session.ID)},
	}
	GetSpaceScanSession(c)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestLocalArtifactStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := scan.NewLocalArtifactStore(dir)
	require.NoError(t, err)
	loc, err := store.Put("a/b.json", []byte(`{"ok":true}`), "application/json")
	require.NoError(t, err)
	data, err := store.Get(loc)
	require.NoError(t, err)
	assert.Equal(t, `{"ok":true}`, string(data))
	require.NoError(t, store.Delete(loc))
}

func TestScanStatus_ProcessingTransitions(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-trans")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	rawToken, err := generateOpaqueScanToken()
	require.NoError(t, err)
	sid := space.ID
	session := &database.SpaceScanSession{
		BusinessID: biz.ID,
		SpaceID:    &sid,
		Status:     database.ScanStatusPhoneConnected,
		ExpiresAt:  time.Now().UTC().Add(30 * time.Minute),
	}
	require.NoError(t, database.CreateSpaceScanSession(session, rawToken))

	// Forward to scanning
	bodyScan, _ := json.Marshal(map[string]any{"status": "scanning", "progress_pct": 20, "progress_message": "capturing"})
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Params = gin.Params{{Key: "token", Value: rawToken}}
	c1.Request = httptest.NewRequest(http.MethodPost, "/status", bytes.NewReader(bodyScan))
	c1.Request.Header.Set("Content-Type", "application/json")
	PublicSpaceScanStatus(c1)
	require.Equal(t, http.StatusOK, w1.Code, w1.Body.String())
	assert.Contains(t, w1.Body.String(), database.ScanStatusScanning)

	// Forward to uploading
	bodyUp, _ := json.Marshal(map[string]any{"status": "uploading", "progress_pct": 40})
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{{Key: "token", Value: rawToken}}
	c2.Request = httptest.NewRequest(http.MethodPost, "/status", bytes.NewReader(bodyUp))
	c2.Request.Header.Set("Content-Type", "application/json")
	PublicSpaceScanStatus(c2)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	assert.Contains(t, w2.Body.String(), database.ScanStatusUploading)

	// Invalid status rejected
	bodyBad, _ := json.Marshal(map[string]any{"status": "processing"})
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Params = gin.Params{{Key: "token", Value: rawToken}}
	c3.Request = httptest.NewRequest(http.MethodPost, "/status", bytes.NewReader(bodyBad))
	c3.Request.Header.Set("Content-Type", "application/json")
	PublicSpaceScanStatus(c3)
	require.Equal(t, http.StatusBadRequest, w3.Code)
}

func TestScanCompleteUpload_StartsProcessing(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-proc")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	rawToken, err := generateOpaqueScanToken()
	require.NoError(t, err)
	sid := space.ID
	session := &database.SpaceScanSession{
		BusinessID:  biz.ID,
		SpaceID:     &sid,
		Status:      database.ScanStatusUploading,
		ExpiresAt:   time.Now().UTC().Add(30 * time.Minute),
		ProgressPct: 40,
	}
	require.NoError(t, database.CreateSpaceScanSession(session, rawToken))

	// Seed an upload so complete has something to process.
	s3Key := "test/payload.json"
	checksum := "abc"
	size := int64(10)
	upload := &database.SpaceScanUpload{
		SessionID:      session.ID,
		BusinessID:     biz.ID,
		UploadKind:     "roomplan_json",
		S3Key:          &s3Key,
		ChecksumSHA256: &checksum,
		ByteSize:       &size,
		IsComplete:     true,
	}
	require.NoError(t, database.CreateSpaceScanUpload(upload))

	// CompleteUpload should move session to processing (worker may be a no-op in unit tests).
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: rawToken}}
	body, _ := json.Marshal(map[string]any{})
	c.Request = httptest.NewRequest(http.MethodPost, "/complete", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	PublicSpaceScanCompleteUpload(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.True(t,
		strings.Contains(w.Body.String(), database.ScanStatusProcessing) ||
			strings.Contains(w.Body.String(), `"enqueued":true`),
		w.Body.String(),
	)
}

func TestScanToken_UnknownToken404(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: "definitely-not-a-real-token-zzzz"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/space-scan/x", nil)
	PublicSpaceScanSessionMeta(c)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestPublicScanUpload_MissingIdempotencyStillWorks(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-no-idem")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	rawToken, err := generateOpaqueScanToken()
	require.NoError(t, err)
	sid := space.ID
	session := &database.SpaceScanSession{
		BusinessID: biz.ID,
		SpaceID:    &sid,
		Status:     database.ScanStatusPhoneConnected,
		ExpiresAt:  time.Now().UTC().Add(30 * time.Minute),
	}
	require.NoError(t, database.CreateSpaceScanSession(session, rawToken))

	payload := map[string]any{
		"upload_kind": "roomplan_json",
		"payload": map[string]any{
			"unit": "m", "dimensions": map[string]any{"width": 4, "height": 3},
			"walls": []any{}, "objects": []any{},
		},
	}
	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: rawToken}}
	c.Request = httptest.NewRequest(http.MethodPost, "/uploads", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	// No Idempotency-Key header
	PublicSpaceScanUpload(c)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

func TestPublicScanUpload_RejectsVideoAndDepth(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-vid")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	rawToken, err := generateOpaqueScanToken()
	require.NoError(t, err)
	sid := space.ID
	session := &database.SpaceScanSession{
		BusinessID: biz.ID,
		SpaceID:    &sid,
		Status:     database.ScanStatusPhoneConnected,
		ExpiresAt:  time.Now().UTC().Add(30 * time.Minute),
	}
	require.NoError(t, database.CreateSpaceScanSession(session, rawToken))

	for _, kind := range []string{"video", "depth"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "token", Value: rawToken}}
		body, _ := json.Marshal(map[string]any{
			"upload_kind": kind,
			"payload":     map[string]any{"frames": []any{}},
		})
		c.Request = httptest.NewRequest(http.MethodPost, "/uploads", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		PublicSpaceScanUpload(c)
		require.Equal(t, http.StatusBadRequest, w.Code, "kind=%s body=%s", kind, w.Body.String())
		assert.Contains(t, w.Body.String(), "unsupported_upload_kind")
	}
}

func TestPublicScanResult_ReadableAfterReviewReadyWithoutRevoke(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-res")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	rawToken, err := generateOpaqueScanToken()
	require.NoError(t, err)
	sid := space.ID
	layout := database.JSONRawMessage(`{"schema_version":1,"width_mm":8000,"height_mm":6000,"tables":[{"table_id":0,"name":"T1","x_mm":100,"y_mm":100,"width_mm":800,"height_mm":800,"shape":"round"}]}`)
	session := &database.SpaceScanSession{
		BusinessID:       biz.ID,
		SpaceID:          &sid,
		Status:           database.ScanStatusReviewReady,
		ExpiresAt:        time.Now().UTC().Add(30 * time.Minute),
		ResultLayoutJSON: layout,
		ProgressPct:      100,
	}
	require.NoError(t, database.CreateSpaceScanSession(session, rawToken))

	// Token still resolves after processing (not revoked on review_ready).
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: rawToken}}
	c.Request = httptest.NewRequest(http.MethodGet, "/result", nil)
	PublicSpaceScanResult(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"status":"review_ready"`)
	assert.Contains(t, w.Body.String(), `"table_id":0`)
	assert.Contains(t, w.Body.String(), "T1")
	// draft_revision is exposed for strict CAS on apply-review.
	var resultBody map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resultBody))
	assert.Contains(t, resultBody, "draft_revision")
}

func TestClearSpaceScanDraftApplyFailed_AfterSuccessfulApply(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "sticky-clear")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "Main", SpaceType: "indoor", DraftRevision: 1}
	require.NoError(t, database.CreateRestaurantSpace(space))
	rawToken, err := generateOpaqueScanToken()
	require.NoError(t, err)
	sid := space.ID
	session := &database.SpaceScanSession{
		BusinessID: biz.ID, SpaceID: &sid, Status: database.ScanStatusReviewReady,
		ExpiresAt: time.Now().UTC().Add(30 * time.Minute), ProgressPct: 100,
	}
	require.NoError(t, database.CreateSpaceScanSession(session, rawToken))
	require.NoError(t, database.MarkSpaceScanDraftApplyFailed(biz.ID, session.ID, "prior failure"))

	// Successful apply on empty draft, then clear sticky failure (worker success path).
	svc := spaces.NewService()
	layout := []byte(`{"schema_version":1,"width_mm":2000,"height_mm":2000,"tables":[
		{"table_id":0,"name":"T1","client_key":"c1","x_mm":10,"y_mm":10,"width_mm":400,"height_mm":400,"shape":"round"}
	]}`)
	_, _, err = svc.ApplyScanLayoutToDraft(biz.ID, space.ID, layout)
	require.NoError(t, err)
	require.NoError(t, database.ClearSpaceScanDraftApplyFailed(biz.ID, session.ID))

	reloaded, err := database.GetSpaceScanSessionByID(biz.ID, session.ID)
	require.NoError(t, err)
	assert.Nil(t, reloaded.ErrorCode, "sticky draft_apply_failed must be cleared after success")
	assert.Nil(t, reloaded.ErrorMessage)
}

func TestApplyScan_SecondPassOnScanOwnedDoesNotNeedFailureFlag(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "retry-scan-owned")
	prior := `{"schema_version":1,"width_mm":8000,"height_mm":6000,"tables":[
		{"table_id":0,"name":"ScanOnly","client_key":"s1","x_mm":10,"y_mm":10,"width_mm":400,"height_mm":400,"shape":"round"}
	]}`
	space := &database.RestaurantSpace{
		BusinessID: biz.ID, Name: "Main", SpaceType: "indoor", DraftRevision: 2,
		DraftLayoutJSON: database.JSONRawMessage(prior),
	}
	require.NoError(t, database.CreateRestaurantSpace(space))
	rawToken, err := generateOpaqueScanToken()
	require.NoError(t, err)
	sid := space.ID
	session := &database.SpaceScanSession{
		BusinessID: biz.ID, SpaceID: &sid, Status: database.ScanStatusReviewReady,
		ExpiresAt: time.Now().UTC().Add(30 * time.Minute), ProgressPct: 100,
	}
	require.NoError(t, database.CreateSpaceScanSession(session, rawToken))

	svc := spaces.NewService()
	// Second apply of same scan-owned shape must succeed (not operator conflict).
	_, _, err = svc.ApplyScanLayoutToDraft(biz.ID, space.ID, []byte(prior))
	require.NoError(t, err)
	// Worker would clear; assert we never needed to mark failed.
	reloaded, err := database.GetSpaceScanSessionByID(biz.ID, session.ID)
	require.NoError(t, err)
	assert.Nil(t, reloaded.ErrorCode)
}

func TestPublicScanResult_SurfacesDraftApplyFailed(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-daf")
	space := &database.RestaurantSpace{
		BusinessID: biz.ID, Name: "R1", SpaceType: "indoor", DraftRevision: 4,
		DraftLayoutJSON: database.JSONRawMessage(`{"schema_version":1,"width_mm":1000,"height_mm":1000,"tables":[{"table_id":1,"name":"Op","x_mm":0,"y_mm":0,"width_mm":100,"height_mm":100,"shape":"round"}]}`),
	}
	require.NoError(t, database.CreateRestaurantSpace(space))

	rawToken, err := generateOpaqueScanToken()
	require.NoError(t, err)
	sid := space.ID
	layout := database.JSONRawMessage(`{"schema_version":1,"width_mm":2000,"height_mm":2000,"tables":[]}`)
	session := &database.SpaceScanSession{
		BusinessID: biz.ID, SpaceID: &sid, Status: database.ScanStatusReviewReady,
		ExpiresAt: time.Now().UTC().Add(30 * time.Minute), ResultLayoutJSON: layout, ProgressPct: 100,
	}
	require.NoError(t, database.CreateSpaceScanSession(session, rawToken))
	require.NoError(t, database.MarkSpaceScanDraftApplyFailed(biz.ID, session.ID, "scan ready but draft apply failed — re-apply from review"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: rawToken}}
	c.Request = httptest.NewRequest(http.MethodGet, "/result", nil)
	PublicSpaceScanResult(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["draft_apply_failed"])
	assert.Equal(t, "draft_apply_failed", body["error_code"])
	assert.Equal(t, float64(4), body["draft_revision"])
}

func TestPublicScanResult_ExpiredDenied(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-exp")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	rawToken, err := generateOpaqueScanToken()
	require.NoError(t, err)
	sid := space.ID
	layout := database.JSONRawMessage(`{"schema_version":1,"width_mm":1000,"height_mm":1000}`)
	session := &database.SpaceScanSession{
		BusinessID:       biz.ID,
		SpaceID:          &sid,
		Status:           database.ScanStatusReviewReady,
		ExpiresAt:        time.Now().UTC().Add(-1 * time.Minute),
		ResultLayoutJSON: layout,
		ProgressPct:      100,
	}
	require.NoError(t, database.CreateSpaceScanSession(session, rawToken))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: rawToken}}
	c.Request = httptest.NewRequest(http.MethodGet, "/result", nil)
	PublicSpaceScanResult(c)
	require.Equal(t, http.StatusGone, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "width_mm")
}

func TestPublicScanResult_DeniedAfterComplete(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-done")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	rawToken, err := generateOpaqueScanToken()
	require.NoError(t, err)
	sid := space.ID
	// Result must include the candidate so apply-review accepts the filtered subset.
	layout := database.JSONRawMessage(`{"schema_version":1,"width_mm":2000,"height_mm":2000,"tables":[
		{"table_id":0,"name":"FromPhone","x_mm":10,"y_mm":10,"width_mm":400,"height_mm":400,"shape":"round","client_key":"c1"}
	]}`)
	session := &database.SpaceScanSession{
		BusinessID:       biz.ID,
		SpaceID:          &sid,
		Status:           database.ScanStatusReviewReady,
		ExpiresAt:        time.Now().UTC().Add(30 * time.Minute),
		ResultLayoutJSON: layout,
		ProgressPct:      100,
	}
	require.NoError(t, database.CreateSpaceScanSession(session, rawToken))

	// Apply-review completes session + revokes token (strict expected_revision required).
	wApply := httptest.NewRecorder()
	cApply, _ := gin.CreateTestContext(wApply)
	cApply.Params = gin.Params{{Key: "token", Value: rawToken}}
	body, _ := json.Marshal(map[string]any{
		"expected_revision": space.DraftRevision,
		"layout": map[string]any{
			"schema_version": 1, "width_mm": 2000, "height_mm": 2000,
			"tables": []any{
				map[string]any{"table_id": 0, "name": "FromPhone", "x_mm": 10, "y_mm": 10, "width_mm": 400, "height_mm": 400, "shape": "round", "client_key": "c1"},
			},
		},
	})
	cApply.Request = httptest.NewRequest(http.MethodPost, "/apply-review", bytes.NewReader(body))
	cApply.Request.Header.Set("Content-Type", "application/json")
	PublicSpaceScanApplyReview(cApply)
	require.Equal(t, http.StatusOK, wApply.Code, wApply.Body.String())
	assert.Contains(t, wApply.Body.String(), `"completed":true`)

	// Same token can no longer read layout.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: rawToken}}
	c.Request = httptest.NewRequest(http.MethodGet, "/result", nil)
	PublicSpaceScanResult(c)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestCompleteScanSession_RevokesToken(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-comp")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	c, w := spaceOwnerContext(t, biz, http.MethodPost,
		fmt.Sprintf("/inside/businesses/%s/spaces/%d/scan-sessions", biz.BusinessId, space.ID),
		map[string]any{},
	)
	c.Params = gin.Params{
		{Key: "id", Value: biz.BusinessId},
		{Key: "spaceId", Value: fmt.Sprintf("%d", space.ID)},
	}
	CreateSpaceScanSession(c)
	var created map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	token := created["token"].(string)
	sid := uint(created["session"].(map[string]any)["id"].(float64))

	// Force review_ready so complete is meaningful.
	require.NoError(t, database.UpdateSpaceScanSessionStatus(biz.ID, sid, database.ScanStatusReviewReady, 100, nil, nil, nil))

	c2, w2 := spaceOwnerContext(t, biz, http.MethodPost,
		fmt.Sprintf("/inside/businesses/%s/scan-sessions/%d/complete", biz.BusinessId, sid), nil)
	c2.Params = gin.Params{
		{Key: "id", Value: biz.BusinessId},
		{Key: "sessionId", Value: fmt.Sprintf("%d", sid)},
	}
	CompleteSpaceScanSession(c2)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())

	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Params = gin.Params{{Key: "token", Value: token}}
	c3.Request = httptest.NewRequest(http.MethodGet, "/result", nil)
	PublicSpaceScanResult(c3)
	require.Equal(t, http.StatusNotFound, w3.Code)
}

func TestPublicScanApplyReview_RejectsArbitraryForeignLayout(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-authz")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	// Existing table in ANOTHER space — must not be injectable via token apply.
	other := &database.RestaurantSpace{BusinessID: biz.ID, Name: "Other", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(other))
	foreign := &database.Table{BusinessID: biz.ID, TableCode: "F1", Name: "Foreign", Capacity: 2, IsActive: true}
	require.NoError(t, database.GetDB().Create(foreign).Error)
	require.NoError(t, database.AssignTablesToSpace(biz.ID, other.ID, []uint{foreign.ID}))

	rawToken, err := generateOpaqueScanToken()
	require.NoError(t, err)
	sid := space.ID
	result := database.JSONRawMessage(`{"schema_version":1,"width_mm":2000,"height_mm":2000,"tables":[
		{"table_id":0,"name":"ScanT","client_key":"s1","x_mm":10,"y_mm":10,"width_mm":400,"height_mm":400,"shape":"round"}
	]}`)
	session := &database.SpaceScanSession{
		BusinessID: biz.ID, SpaceID: &sid, Status: database.ScanStatusReviewReady,
		ExpiresAt: time.Now().UTC().Add(30 * time.Minute), ResultLayoutJSON: result, ProgressPct: 100,
	}
	require.NoError(t, database.CreateSpaceScanSession(session, rawToken))

	// Attempt to inject foreign table_id into draft via public token.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: rawToken}}
	body, _ := json.Marshal(map[string]any{
		"expected_revision": space.DraftRevision,
		"layout": map[string]any{
			"schema_version": 1, "width_mm": 2000, "height_mm": 2000,
			"tables": []any{
				map[string]any{"table_id": foreign.ID, "name": "Foreign", "x_mm": 0, "y_mm": 0, "width_mm": 400, "height_mm": 400, "shape": "round"},
			},
		},
	})
	c.Request = httptest.NewRequest(http.MethodPost, "/apply-review", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	PublicSpaceScanApplyReview(c)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "invalid_argument")
}

func TestPublicScanApplyReview_RequiresExpectedRevision(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-rev-req")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor", DraftRevision: 1}
	require.NoError(t, database.CreateRestaurantSpace(space))
	rawToken, err := generateOpaqueScanToken()
	require.NoError(t, err)
	sid := space.ID
	result := database.JSONRawMessage(`{"schema_version":1,"width_mm":2000,"height_mm":2000,"tables":[]}`)
	session := &database.SpaceScanSession{
		BusinessID: biz.ID, SpaceID: &sid, Status: database.ScanStatusReviewReady,
		ExpiresAt: time.Now().UTC().Add(30 * time.Minute), ResultLayoutJSON: result, ProgressPct: 100,
	}
	require.NoError(t, database.CreateSpaceScanSession(session, rawToken))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: rawToken}}
	body, _ := json.Marshal(map[string]any{
		"layout": map[string]any{"schema_version": 1, "width_mm": 2000, "height_mm": 2000, "tables": []any{}},
	})
	c.Request = httptest.NewRequest(http.MethodPost, "/apply-review", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	PublicSpaceScanApplyReview(c)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "expected_revision_required")
}

func TestPublicScanApplyReview_RejectsOperatorDraftClobber(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-op-draft")
	operatorDraft := `{"schema_version":1,"width_mm":5000,"height_mm":4000,"tables":[
		{"table_id":1,"name":"OperatorT","x_mm":100,"y_mm":100,"width_mm":800,"height_mm":800,"shape":"rectangle"}
	]}`
	space := &database.RestaurantSpace{
		BusinessID: biz.ID, Name: "R1", SpaceType: "indoor", DraftRevision: 4,
		DraftLayoutJSON: database.JSONRawMessage(operatorDraft),
	}
	require.NoError(t, database.CreateRestaurantSpace(space))
	rawToken, err := generateOpaqueScanToken()
	require.NoError(t, err)
	sid := space.ID
	result := database.JSONRawMessage(`{"schema_version":1,"width_mm":2000,"height_mm":2000,"tables":[
		{"table_id":0,"name":"ScanT","client_key":"s1","x_mm":10,"y_mm":10,"width_mm":400,"height_mm":400,"shape":"round"}
	]}`)
	session := &database.SpaceScanSession{
		BusinessID: biz.ID, SpaceID: &sid, Status: database.ScanStatusReviewReady,
		ExpiresAt: time.Now().UTC().Add(30 * time.Minute), ResultLayoutJSON: result, ProgressPct: 100,
	}
	require.NoError(t, database.CreateSpaceScanSession(session, rawToken))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: rawToken}}
	body, _ := json.Marshal(map[string]any{
		"expected_revision": space.DraftRevision,
		"layout": map[string]any{
			"schema_version": 1, "width_mm": 2000, "height_mm": 2000,
			"tables": []any{
				map[string]any{"table_id": 0, "name": "ScanT", "client_key": "s1", "x_mm": 10, "y_mm": 10, "width_mm": 400, "height_mm": 400, "shape": "round"},
			},
		},
	})
	c.Request = httptest.NewRequest(http.MethodPost, "/apply-review", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	PublicSpaceScanApplyReview(c)
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "draft_has_content")

	reloaded, err := database.GetRestaurantSpaceByID(biz.ID, space.ID)
	require.NoError(t, err)
	assert.Contains(t, string(reloaded.DraftLayoutJSON), "OperatorT")
	assert.NotContains(t, string(reloaded.DraftLayoutJSON), "ScanT")
}

func TestPublicScanApplyReview_RejectsInventedTables(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-invent")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	rawToken, err := generateOpaqueScanToken()
	require.NoError(t, err)
	sid := space.ID
	result := database.JSONRawMessage(`{"schema_version":1,"width_mm":2000,"height_mm":2000,"tables":[]}`)
	session := &database.SpaceScanSession{
		BusinessID: biz.ID, SpaceID: &sid, Status: database.ScanStatusReviewReady,
		ExpiresAt: time.Now().UTC().Add(30 * time.Minute), ResultLayoutJSON: result, ProgressPct: 100,
	}
	require.NoError(t, database.CreateSpaceScanSession(session, rawToken))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: rawToken}}
	body, _ := json.Marshal(map[string]any{
		"expected_revision": space.DraftRevision,
		"layout": map[string]any{
			"schema_version": 1, "width_mm": 2000, "height_mm": 2000,
			"tables": []any{
				map[string]any{"table_id": 0, "name": "Invented", "x_mm": 0, "y_mm": 0, "width_mm": 400, "height_mm": 400, "shape": "round"},
			},
		},
	})
	c.Request = httptest.NewRequest(http.MethodPost, "/apply-review", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	PublicSpaceScanApplyReview(c)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// TestPublicScanConnect_PairAttemptCapAndUploadNeedsPairing: an unpaired
// device cannot upload or advance status, and the fifth wrong pair code
// expires the session so the right code no longer works either.
func TestPublicScanConnect_PairAttemptCapAndUploadNeedsPairing(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	biz := createSpaceHandlerBusiness(t, "scan-cap")
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))

	c, w := spaceOwnerContext(t, biz, http.MethodPost,
		fmt.Sprintf("/inside/businesses/%s/spaces/%d/scan-sessions", biz.BusinessId, space.ID),
		map[string]any{},
	)
	c.Params = gin.Params{{Key: "id", Value: biz.BusinessId}, {Key: "spaceId", Value: fmt.Sprintf("%d", space.ID)}}
	CreateSpaceScanSession(c)
	var created map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	token := created["token"].(string)
	pair := created["pair_code"].(string)
	wrong := "000000"
	if pair == wrong {
		wrong = "111111"
	}

	call := func(h gin.HandlerFunc, path string, body map[string]any) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		raw, _ := json.Marshal(body)
		ctx.Params = gin.Params{{Key: "token", Value: token}}
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/space-scan/"+token+path, bytes.NewReader(raw))
		ctx.Request.Header.Set("Content-Type", "application/json")
		h(ctx)
		return rec
	}

	require.Equal(t, http.StatusConflict, call(PublicSpaceScanUpload, "/uploads", map[string]any{"upload_kind": "keyframes"}).Code)
	require.Equal(t, http.StatusConflict, call(PublicSpaceScanStatus, "/status", map[string]any{"status": "scanning"}).Code)

	for i := 1; i < spaceScanMaxPairFailures; i++ {
		require.Equal(t, http.StatusUnauthorized, call(PublicSpaceScanConnect, "/connect", map[string]any{"pair_code": wrong}).Code, "attempt %d", i)
	}
	require.Equal(t, http.StatusGone, call(PublicSpaceScanConnect, "/connect", map[string]any{"pair_code": wrong}).Code)
	// The session is burned: even the right code is refused now.
	require.NotEqual(t, http.StatusOK, call(PublicSpaceScanConnect, "/connect", map[string]any{"pair_code": pair}).Code)
}

// newPendingScanSessionForPairTest creates a waiting_for_phone scan session
// and returns its token, pair code, a wrong code and a handler caller.
func newPendingScanSessionForPairTest(t *testing.T, slug string) (string, string, string, func(gin.HandlerFunc, string, map[string]any) *httptest.ResponseRecorder) {
	t.Helper()
	biz := createSpaceHandlerBusiness(t, slug)
	space := &database.RestaurantSpace{BusinessID: biz.ID, Name: "R1", SpaceType: "indoor"}
	require.NoError(t, database.CreateRestaurantSpace(space))
	c, w := spaceOwnerContext(t, biz, http.MethodPost,
		fmt.Sprintf("/inside/businesses/%s/spaces/%d/scan-sessions", biz.BusinessId, space.ID),
		map[string]any{},
	)
	c.Params = gin.Params{{Key: "id", Value: biz.BusinessId}, {Key: "spaceId", Value: fmt.Sprintf("%d", space.ID)}}
	CreateSpaceScanSession(c)
	var created map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	token := created["token"].(string)
	pair := created["pair_code"].(string)
	wrong := "000000"
	if pair == wrong {
		wrong = "111111"
	}
	call := func(h gin.HandlerFunc, path string, body map[string]any) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		raw, _ := json.Marshal(body)
		ctx.Params = gin.Params{{Key: "token", Value: token}}
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/space-scan/"+token+path, bytes.NewReader(raw))
		ctx.Request.Header.Set("Content-Type", "application/json")
		h(ctx)
		return rec
	}
	return token, pair, wrong, call
}

// TestPublicScanConnect_WrongCodesCannotTearDownPairedSession: once a phone
// has paired, a stranger's wrong pair codes are neither counted nor
// destructive: the session keeps its status and token, and the paired phone
// keeps uploading.
func TestPublicScanConnect_WrongCodesCannotTearDownPairedSession(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	token, pair, wrong, call := newPendingScanSessionForPairTest(t, "scan-paired")

	require.Equal(t, http.StatusOK, call(PublicSpaceScanConnect, "/connect", map[string]any{"pair_code": pair}).Code)

	for i := 0; i < spaceScanMaxPairFailures*4; i++ {
		require.Equal(t, http.StatusUnauthorized, call(PublicSpaceScanConnect, "/connect", map[string]any{"pair_code": wrong}).Code, "attempt %d", i+1)
	}

	s, err := database.GetSpaceScanSessionByTokenHash(database.HashScanToken(token))
	require.NoError(t, err, "token must still resolve the paired session")
	require.Equal(t, database.ScanStatusPhoneConnected, s.Status)
	// The paired phone can still advance and reconnect with its code.
	require.Equal(t, http.StatusOK, call(PublicSpaceScanStatus, "/status", map[string]any{"status": "scanning"}).Code)
	require.Equal(t, http.StatusOK, call(PublicSpaceScanConnect, "/connect", map[string]any{"pair_code": pair}).Code)
}

// TestPublicScanConnect_CapExpiresPendingCodeOnly: hitting the cap on a
// pending session expires the pair code but keeps the session and its token,
// so an operator with business access can still connect it.
func TestPublicScanConnect_CapExpiresPendingCodeOnly(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	token, pair, wrong, call := newPendingScanSessionForPairTest(t, "scan-pending-cap")

	for i := 1; i < spaceScanMaxPairFailures; i++ {
		require.Equal(t, http.StatusUnauthorized, call(PublicSpaceScanConnect, "/connect", map[string]any{"pair_code": wrong}).Code)
	}
	require.Equal(t, http.StatusGone, call(PublicSpaceScanConnect, "/connect", map[string]any{"pair_code": wrong}).Code)

	// The bound is durable: a fresh process (empty in-memory counter) still
	// refuses the right code because the pending code was expired in the DB.
	spaceScanPairFailures = newSpaceScanPairAttemptGuard(spaceScanMaxPairFailures)
	require.Equal(t, http.StatusGone, call(PublicSpaceScanConnect, "/connect", map[string]any{"pair_code": pair}).Code)

	s, err := database.GetSpaceScanSessionByTokenHash(database.HashScanToken(token))
	require.NoError(t, err, "the session token must not be revoked")
	require.Equal(t, database.ScanStatusWaitingForPhone, s.Status, "the session must not be torn down")
}

// TestPublicScanConnect_ConcurrentWrongCodesRespectCap: parallel wrong codes
// cannot exceed the cap, because each attempt is claimed atomically before the
// code is compared.
func TestPublicScanConnect_ConcurrentWrongCodesRespectCap(t *testing.T) {
	setupSpaceScanHandlerTestDB(t)
	_, pair, wrong, call := newPendingScanSessionForPairTest(t, "scan-concurrent")

	const n = 40
	var wg sync.WaitGroup
	codes := make(chan int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- call(PublicSpaceScanConnect, "/connect", map[string]any{"pair_code": wrong}).Code
		}()
	}
	wg.Wait()
	close(codes)
	compared := 0
	for code := range codes {
		switch code {
		case http.StatusUnauthorized:
			compared++
		case http.StatusGone:
		default:
			t.Fatalf("unexpected status %d", code)
		}
	}
	require.Equal(t, spaceScanMaxPairFailures-1, compared, "only cap-1 wrong codes may be compared before the code expires")
	require.Equal(t, http.StatusGone, call(PublicSpaceScanConnect, "/connect", map[string]any{"pair_code": pair}).Code)
}
