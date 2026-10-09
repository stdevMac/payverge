package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/spaces/scan"
	"github.com/stdevmac/payverge/backend/internal/utils"
)

// Scan config defaults (overridable via env).
const (
	defaultSpaceScanSessionTTLMinutes = 30
	defaultSpaceScanMaxUploadBytes    = 25 << 20 // 25 MiB
	defaultSpaceScanRawRetentionDays  = 7
	defaultSpaceScanMaxFrames         = 120
)

// spaceScanArtifactStore is set at startup; defaults to S3+local composite.
var spaceScanArtifactStore scan.ArtifactStore

// SetSpaceScanArtifactStore wires storage from main.go (optional; defaults on first use).
func SetSpaceScanArtifactStore(store scan.ArtifactStore) {
	spaceScanArtifactStore = store
}

func getSpaceScanStore() scan.ArtifactStore {
	if spaceScanArtifactStore != nil {
		return spaceScanArtifactStore
	}
	spaceScanArtifactStore = scan.DefaultArtifactStore("")
	return spaceScanArtifactStore
}

func spaceScanSessionTTL() time.Duration {
	mins := envIntOr("SPACE_SCAN_SESSION_TTL_MINUTES", defaultSpaceScanSessionTTLMinutes)
	if mins <= 0 {
		mins = defaultSpaceScanSessionTTLMinutes
	}
	return time.Duration(mins) * time.Minute
}

func spaceScanMaxUploadBytes() int64 {
	n := envIntOr("SPACE_SCAN_MAX_UPLOAD_BYTES", defaultSpaceScanMaxUploadBytes)
	if n <= 0 {
		return defaultSpaceScanMaxUploadBytes
	}
	return int64(n)
}

func spaceScanRawRetentionDays() int {
	return envIntOr("SPACE_SCAN_RAW_RETENTION_DAYS", defaultSpaceScanRawRetentionDays)
}

func spaceScanMaxFrames() int {
	return envIntOr("SPACE_SCAN_MAX_FRAMES", defaultSpaceScanMaxFrames)
}

func envIntOr(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// generateOpaqueScanToken returns a 32+ byte base64url token (raw, returned once).
func generateOpaqueScanToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// generatePairCode returns a 6-digit numeric pair code.
func generatePairCode() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	n := int(buf[0])<<16 | int(buf[1])<<8 | int(buf[2])
	return fmt.Sprintf("%06d", n%1000000), nil
}

func hashPairCode(code string) string {
	sum := sha256.Sum256([]byte("pair:" + strings.TrimSpace(code)))
	return hex.EncodeToString(sum[:])
}

// CreateSpaceScanSessionRequest is the body for POST .../scan-sessions.
type CreateSpaceScanSessionRequest struct {
	IdempotencyKey *string `json:"idempotency_key"`
	CalibrationMm  *int    `json:"calibration_mm"`
}

// CreateSpaceScanSession handles POST /businesses/:id/spaces/:spaceId/scan-sessions
func CreateSpaceScanSession(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	spaceID, ok := parseSpaceIDParam(c)
	if !ok {
		return
	}
	if _, err := database.GetRestaurantSpaceByID(business.ID, spaceID); err != nil {
		if errors.Is(err, database.ErrSpaceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Space not found", "code": "not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load space"})
		return
	}

	var req CreateSpaceScanSessionRequest
	_ = c.ShouldBindJSON(&req)

	// Idempotent create by business + key.
	if req.IdempotencyKey != nil && strings.TrimSpace(*req.IdempotencyKey) != "" {
		key := strings.TrimSpace(*req.IdempotencyKey)
		existing, err := database.ListSpaceScanSessions(business.ID, "")
		if err == nil {
			for i := range existing {
				if existing[i].IdempotencyKey != nil && *existing[i].IdempotencyKey == key {
					// Do not re-return raw token.
					c.JSON(http.StatusOK, gin.H{
						"session":    existing[i],
						"token":      nil,
						"pair_code":  nil,
						"idempotent": true,
						"message":    "existing session returned; token not reissued",
					})
					return
				}
			}
		}
	}

	rawToken, err := generateOpaqueScanToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session token"})
		return
	}
	pairCode, err := generatePairCode()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create pair code"})
		return
	}

	now := time.Now().UTC()
	retainDays := spaceScanRawRetentionDays()
	if retainDays <= 0 {
		retainDays = defaultSpaceScanRawRetentionDays
	}
	retainUntil := now.AddDate(0, 0, retainDays)
	sid := spaceID
	userID, staffID := spaceActorIDs(c)

	deviceMeta, _ := json.Marshal(map[string]interface{}{
		"pair_code_hash": hashPairCode(pairCode),
	})

	session := &database.SpaceScanSession{
		BusinessID:       business.ID,
		SpaceID:          &sid,
		CreatedByUserID:  userID,
		CreatedByStaffID: staffID,
		Status:           database.ScanStatusWaitingForPhone,
		ExpiresAt:        now.Add(spaceScanSessionTTL()),
		CalibrationMm:    req.CalibrationMm,
		IdempotencyKey:   req.IdempotencyKey,
		DeviceMetaJSON:   database.JSONRawMessage(deviceMeta),
		RetainRawUntil:   &retainUntil,
	}
	if err := database.CreateSpaceScanSession(session, rawToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create scan session"})
		return
	}

	publishSpaceScanUpdated(business.ID, session)

	c.JSON(http.StatusCreated, gin.H{
		"session":    session,
		"token":      rawToken, // returned once
		"pair_code":  pairCode, // returned once for phone pairing without account JWT
		"expires_at": session.ExpiresAt,
	})
}

// GetSpaceScanSession handles GET /businesses/:id/scan-sessions/:sessionId
func GetSpaceScanSession(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	sessionID, err := parseUintParam(c, "sessionId")
	if err != nil || sessionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session ID", "code": "invalid_id"})
		return
	}
	session, err := database.GetSpaceScanSessionByID(business.ID, sessionID)
	if err != nil {
		if errors.Is(err, database.ErrSpaceScanSessionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Session not found", "code": "not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load session"})
		return
	}
	uploads, _ := database.ListSpaceScanUploads(business.ID, sessionID)
	c.JSON(http.StatusOK, gin.H{"session": session, "uploads": uploads})
}

// CancelSpaceScanSession handles POST /businesses/:id/scan-sessions/:sessionId/cancel
func CancelSpaceScanSession(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	sessionID, err := parseUintParam(c, "sessionId")
	if err != nil || sessionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session ID", "code": "invalid_id"})
		return
	}
	session, err := database.GetSpaceScanSessionByID(business.ID, sessionID)
	if err != nil {
		if errors.Is(err, database.ErrSpaceScanSessionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Session not found", "code": "not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load session"})
		return
	}
	switch session.Status {
	case database.ScanStatusCompleted, database.ScanStatusCancelled, database.ScanStatusExpired:
		c.JSON(http.StatusOK, gin.H{"session": session, "status": session.Status})
		return
	}
	msg := "cancelled by operator"
	if err := database.UpdateSpaceScanSessionStatus(business.ID, sessionID, database.ScanStatusCancelled, session.ProgressPct, &msg, nil, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel session"})
		return
	}
	_ = database.RevokeSpaceScanSessionToken(business.ID, sessionID)
	session, _ = database.GetSpaceScanSessionByID(business.ID, sessionID)
	publishSpaceScanUpdated(business.ID, session)
	c.JSON(http.StatusOK, gin.H{"session": session})
}

// RetryProcessSpaceScanSession handles POST .../scan-sessions/:sessionId/retry-process
func RetryProcessSpaceScanSession(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	sessionID, err := parseUintParam(c, "sessionId")
	if err != nil || sessionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session ID", "code": "invalid_id"})
		return
	}
	session, err := database.GetSpaceScanSessionByID(business.ID, sessionID)
	if err != nil {
		if errors.Is(err, database.ErrSpaceScanSessionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Session not found", "code": "not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load session"})
		return
	}
	switch session.Status {
	case database.ScanStatusFailed, database.ScanStatusReviewReady, database.ScanStatusUploading, database.ScanStatusProcessing:
		// ok
	default:
		c.JSON(http.StatusConflict, gin.H{"error": "Session cannot be reprocessed from current status", "code": "invalid_status", "status": session.Status})
		return
	}
	msg := "reprocessing"
	_ = database.UpdateSpaceScanSessionStatus(business.ID, sessionID, database.ScanStatusProcessing, 50, &msg, nil, nil)
	services.EnqueueSpaceScanSession(sessionID)
	session, _ = database.GetSpaceScanSessionByID(business.ID, sessionID)
	publishSpaceScanUpdated(business.ID, session)
	c.JSON(http.StatusOK, gin.H{"session": session, "enqueued": true})
}

// --- Public / mobile token routes ---

func loadSessionByToken(c *gin.Context) (*database.SpaceScanSession, bool) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" || len(token) < 16 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found", "code": "not_found"})
		return nil, false
	}
	hash := database.HashScanToken(token)
	session, err := database.GetSpaceScanSessionByTokenHash(hash)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found", "code": "not_found"})
		return nil, false
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		if session.Status != database.ScanStatusExpired &&
			session.Status != database.ScanStatusCancelled &&
			session.Status != database.ScanStatusCompleted {
			_ = database.UpdateSpaceScanSessionStatus(session.BusinessID, session.ID, database.ScanStatusExpired, session.ProgressPct, nil, nil, nil)
			_ = database.RevokeSpaceScanSessionToken(session.BusinessID, session.ID)
		}
		c.JSON(http.StatusGone, gin.H{"error": "Session expired", "code": "expired"})
		return nil, false
	}
	switch session.Status {
	case database.ScanStatusCancelled, database.ScanStatusExpired, database.ScanStatusCompleted:
		c.JSON(http.StatusGone, gin.H{"error": "Session no longer active", "code": session.Status})
		return nil, false
	}
	return session, true
}

// PublicSpaceScanSessionMeta handles GET /api/v1/space-scan/:token
// Returns session metadata only (space name, business name, status, expires) — no broad account access.
func PublicSpaceScanSessionMeta(c *gin.Context) {
	session, ok := loadSessionByToken(c)
	if !ok {
		return
	}
	business, err := database.GetBusinessByID(session.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found", "code": "not_found"})
		return
	}
	spaceName := ""
	if session.SpaceID != nil {
		if sp, err := database.GetRestaurantSpaceByID(session.BusinessID, *session.SpaceID); err == nil {
			spaceName = sp.Name
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"status":        session.Status,
		"expires_at":    session.ExpiresAt,
		"progress_pct":  session.ProgressPct,
		"business_name": business.Name,
		"space_name":    spaceName,
		// No business_id / space_id / internal ids in public response.
	})
}

// sessionClaimsStillValid checks session revocation without aborting the request.
// Used by public space-scan connect so a stale cookie falls through to pair_code.
func sessionClaimsStillValid(claims map[string]interface{}, tokenString string) bool {
	if session.GlobalStore == nil {
		// Match non-production hybrid auth: allow when store is unavailable outside prod.
		return !utils.IsProduction()
	}
	sessionIDFloat, ok := claims["session_id"].(float64)
	if !ok || sessionIDFloat == 0 {
		return false
	}
	valid, err := session.GlobalStore.Validate(uint(sessionIDFloat), session.HashToken(tokenString))
	return err == nil && valid
}

// tryHydrateOptionalHybridAuth best-effort loads staff/user/web3 JWT from cookies
// or Authorization into gin context. Never aborts: missing/invalid tokens leave
// the request anonymous so pair_code auth still works on public routes.
func tryHydrateOptionalHybridAuth(c *gin.Context) {
	if c == nil {
		return
	}
	if _, ok := c.Get("staff_id"); ok {
		return
	}
	if _, ok := c.Get("user_id"); ok {
		return
	}
	if _, ok := c.Get("address"); ok {
		return
	}

	tokenString := extractTokenFromRequest(c, "staff_token", "session_token")
	if tokenString == "" {
		return
	}

	if claims, err := VerifyStaffToken(tokenString); err == nil {
		if !sessionClaimsStillValid(claims, tokenString) {
			return
		}
		staffID := claimUint(claims, "staff_id")
		if staffID == 0 {
			return
		}
		staff, err := database.GetDBWrapper().StaffService.GetByID(staffID)
		if err != nil || staff == nil || !staff.IsActive {
			return
		}
		tokenVersion := claimUint(claims, "authz_version")
		liveVersion := uint(staff.AuthzVersion)
		if liveVersion == 0 {
			liveVersion = 1
		}
		if tokenVersion != liveVersion {
			return
		}
		c.Set("staff_id", staff.ID)
		c.Set("staff_email", staff.Email)
		c.Set("staff_name", staff.Name)
		c.Set("staff_role", string(staff.Role))
		c.Set("staff_business_id", staff.BusinessID)
		c.Set("staff_authz_version", int(liveVersion))
		c.Set("token_type", "staff")
		c.Set("staff_custom_permissions", staff.CustomPermissions)
		return
	}

	if claims, err := VerifyToken(tokenString); err == nil {
		if !sessionClaimsStillValid(claims, tokenString) {
			return
		}
		tokenType, _ := claims["type"].(string)
		switch tokenType {
		case "web3":
			if addr, ok := claims["address"].(string); ok && addr != "" {
				c.Set("address", addr)
				c.Set("token_type", "web3")
			}
		case "user":
			c.Set("user_id", claims["user_id"])
			c.Set("email", claims["email"])
			// Optional auth on a public route: a privileged role claim is
			// only honoured when users.role still agrees (M-role).
			if role, _ := claims["role"].(string); role != "admin" || liveUserIsPlatformAdmin(claimUint(claims, "user_id")) {
				c.Set("role", claims["role"])
			}
			c.Set("token_type", "user")
			if address, ok := claims["address"].(string); ok && address != "" {
				c.Set("address", address)
				c.Set("wallet_address", address)
			}
		}
	}
}

// PublicSpaceScanConnect handles POST /api/v1/space-scan/:token/connect
// Requires authenticated authorized user OR short-lived pair code from session create.
func PublicSpaceScanConnect(c *gin.Context) {
	session, ok := loadSessionByToken(c)
	if !ok {
		return
	}

	var body struct {
		PairCode   string                 `json:"pair_code"`
		DeviceMeta map[string]interface{} `json:"device_meta"`
	}
	_ = c.ShouldBindJSON(&body)

	// Public routes do not run HybridAuthenticationMiddleware — hydrate optional
	// JWT/cookies so logged-in staff/owners can connect without pair_code.
	tryHydrateOptionalHybridAuth(c)

	authorized := false
	// Authenticated path: hybrid JWT with business access.
	if _, hasAddr := c.Get("address"); hasAddr {
		if biz, err := database.GetBusinessByID(session.BusinessID); err == nil && CheckBusinessAccess(c, biz) {
			authorized = true
		}
	}
	if !authorized {
		if _, hasUID := c.Get("user_id"); hasUID {
			if biz, err := database.GetBusinessByID(session.BusinessID); err == nil && CheckBusinessAccess(c, biz) {
				authorized = true
			}
		}
	}
	if !authorized {
		if staffBiz, ok := c.Get("staff_business_id"); ok {
			if bizID, ok := extractContextUint(staffBiz); ok && bizID == session.BusinessID {
				authorized = true
			}
		}
	}
	if !authorized {
		// Pair code path (primary for phones without an operator session).
		if strings.TrimSpace(body.PairCode) == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "pair_code or authorized session required", "code": "unauthorized"})
			return
		}
		want := hashPairCode(body.PairCode)
		var meta map[string]interface{}
		_ = json.Unmarshal(session.DeviceMetaJSON, &meta)
		got, _ := meta["pair_code_hash"].(string)
		pairExhausted := func() {
			c.JSON(http.StatusGone, gin.H{"error": "Too many invalid pair codes; start a new scan", "code": "pair_attempts_exceeded"})
		}
		if session.Status != database.ScanStatusWaitingForPhone {
			// Already paired: the pair code only re-attaches the paired phone
			// (for example after a reload). Wrong codes are neither counted nor
			// destructive, so a stranger cannot tear down a live session; a
			// guessed code here grants nothing the session token does not.
			if got == "" || got != want {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid pair code", "code": "invalid_pair_code"})
				return
			}
		} else {
			if got == "" {
				// The pending code was expired after too many attempts.
				pairExhausted()
				return
			}
			// Claim the attempt atomically before comparing, so parallel
			// guesses cannot exceed the cap.
			attempt, allowed := spaceScanPairFailures.reserve(session.ID)
			if !allowed {
				_, _ = database.ExpireSpaceScanPairCode(session.BusinessID, session.ID)
				pairExhausted()
				return
			}
			if got != want {
				if attempt >= spaceScanMaxPairFailures {
					// Expire only the pending pair code; the session, its token
					// and any operator state stay intact.
					if _, err := database.ExpireSpaceScanPairCode(session.BusinessID, session.ID); err != nil {
						log.Printf("[SpaceScan] expire pair code for session %d failed: %v", session.ID, err)
					}
					pairExhausted()
					return
				}
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid pair code", "code": "invalid_pair_code"})
				return
			}
			spaceScanPairFailures.forget(session.ID)
		}
		authorized = true
	}
	if !authorized {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized", "code": "unauthorized"})
		return
	}

	// Merge device meta (do not overwrite pair_code_hash).
	if len(body.DeviceMeta) > 0 {
		var meta map[string]interface{}
		_ = json.Unmarshal(session.DeviceMetaJSON, &meta)
		if meta == nil {
			meta = map[string]interface{}{}
		}
		for k, v := range body.DeviceMeta {
			if k == "pair_code_hash" {
				continue
			}
			meta[k] = v
		}
		if b, err := json.Marshal(meta); err == nil {
			_ = database.GetDB().Model(&database.SpaceScanSession{}).
				Where("id = ? AND business_id = ?", session.ID, session.BusinessID).
				Update("device_meta_json", database.JSONRawMessage(b)).Error
		}
	}

	if session.Status == database.ScanStatusWaitingForPhone {
		msg := "phone connected"
		_ = database.UpdateSpaceScanSessionStatus(session.BusinessID, session.ID, database.ScanStatusPhoneConnected, 5, &msg, nil, nil)
	}
	session, _ = database.GetSpaceScanSessionByID(session.BusinessID, session.ID)
	publishSpaceScanUpdated(session.BusinessID, session)
	c.JSON(http.StatusOK, gin.H{
		"status":       session.Status,
		"expires_at":   session.ExpiresAt,
		"progress_pct": session.ProgressPct,
	})
}

// PublicSpaceScanStatus handles POST /api/v1/space-scan/:token/status
func PublicSpaceScanStatus(c *gin.Context) {
	session, ok := loadSessionByToken(c)
	if !ok {
		return
	}
	var body struct {
		Status          string `json:"status"` // scanning | uploading
		ProgressPct     *int   `json:"progress_pct"`
		ProgressMessage string `json:"progress_message"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	status := strings.TrimSpace(body.Status)
	switch status {
	case database.ScanStatusScanning, database.ScanStatusUploading:
		// ok
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be scanning or uploading", "code": "invalid_argument"})
		return
	}
	// Only allow forward progress from connected/scanning/uploading (a device
	// must pair first; waiting_for_phone is not accepted).
	switch session.Status {
	case database.ScanStatusPhoneConnected, database.ScanStatusScanning, database.ScanStatusUploading:
		// ok
	default:
		c.JSON(http.StatusConflict, gin.H{"error": "Cannot update status", "code": "invalid_status", "status": session.Status})
		return
	}
	pct := session.ProgressPct
	if body.ProgressPct != nil {
		pct = *body.ProgressPct
		if pct < 0 {
			pct = 0
		}
		if pct > 99 {
			pct = 99
		}
	}
	var msg *string
	if body.ProgressMessage != "" {
		m := body.ProgressMessage
		msg = &m
	}
	if err := database.UpdateSpaceScanSessionStatus(session.BusinessID, session.ID, status, pct, msg, nil, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update status"})
		return
	}
	session, _ = database.GetSpaceScanSessionByID(session.BusinessID, session.ID)
	publishSpaceScanUpdated(session.BusinessID, session)
	c.JSON(http.StatusOK, gin.H{"status": session.Status, "progress_pct": session.ProgressPct})
}

// PublicSpaceScanUpload handles POST /api/v1/space-scan/:token/uploads
func PublicSpaceScanUpload(c *gin.Context) {
	session, ok := loadSessionByToken(c)
	if !ok {
		return
	}
	// A device must pair (connect) before it may upload: waiting_for_phone is
	// deliberately not accepted.
	switch session.Status {
	case database.ScanStatusPhoneConnected, database.ScanStatusScanning, database.ScanStatusUploading:
		// ok
	default:
		c.JSON(http.StatusConflict, gin.H{"error": "Uploads not accepted in current status", "code": "invalid_status"})
		return
	}

	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idempotencyKey == "" {
		idempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-key"))
	}
	if idempotencyKey != "" {
		if existing, err := database.GetSpaceScanUploadByIdempotencyKey(idempotencyKey); err == nil {
			if existing.BusinessID != session.BusinessID || existing.SessionID != session.ID {
				c.JSON(http.StatusConflict, gin.H{"error": "Idempotency key conflict", "code": "idempotency_conflict"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"upload": existing, "idempotent": true})
			return
		}
	}

	maxBytes := spaceScanMaxUploadBytes()
	contentType := ""
	uploadKind := ""
	var payload []byte
	var checksum string
	partIndex := 0

	// Multipart or JSON structured payload.
	ct := c.ContentType()
	if strings.HasPrefix(ct, "multipart/") {
		if err := c.Request.ParseMultipartForm(maxBytes + (1 << 20)); err != nil {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Payload too large", "code": "payload_too_large"})
			return
		}
		uploadKind = strings.TrimSpace(c.PostForm("upload_kind"))
		if uploadKind == "" {
			uploadKind = database.ScanUploadRoomPlanJSON
		}
		if rawPart := c.PostForm("part_index"); rawPart != "" {
			if n, err := strconv.Atoi(rawPart); err == nil {
				partIndex = n
			}
		}
		file, err := c.FormFile("file")
		if err != nil {
			// Allow raw JSON field "payload".
			raw := c.PostForm("payload")
			if raw == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "file or payload required", "code": "invalid_argument"})
				return
			}
			payload = []byte(raw)
			contentType = "application/json"
		} else {
			if file.Size > maxBytes {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Payload too large", "code": "payload_too_large"})
				return
			}
			f, err := file.Open()
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read upload"})
				return
			}
			defer f.Close()
			payload, err = io.ReadAll(io.LimitReader(f, maxBytes+1))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read upload"})
				return
			}
			if int64(len(payload)) > maxBytes {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Payload too large", "code": "payload_too_large"})
				return
			}
			contentType = file.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "application/octet-stream"
			}
		}
	} else {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes+1)
		var body struct {
			UploadKind     string          `json:"upload_kind"`
			ContentType    string          `json:"content_type"`
			ChecksumSHA256 string          `json:"checksum_sha256"`
			PartIndex      int             `json:"part_index"`
			Payload        json.RawMessage `json:"payload"`
			// Base64 alternative for binary frames.
			PayloadBase64 string `json:"payload_base64"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			RespondBindError(c, err)
			return
		}
		uploadKind = strings.TrimSpace(body.UploadKind)
		if uploadKind == "" {
			uploadKind = database.ScanUploadRoomPlanJSON
		}
		partIndex = body.PartIndex
		contentType = body.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		if body.PayloadBase64 != "" {
			decoded, err := base64.StdEncoding.DecodeString(body.PayloadBase64)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload_base64", "code": "invalid_argument"})
				return
			}
			payload = decoded
		} else if len(body.Payload) > 0 {
			payload = body.Payload
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "payload required", "code": "invalid_argument"})
			return
		}
		if int64(len(payload)) > maxBytes {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Payload too large", "code": "payload_too_large"})
			return
		}
		if body.ChecksumSHA256 != "" {
			checksum = strings.ToLower(strings.TrimSpace(body.ChecksumSHA256))
		}
	}

	if !validScanUploadKind(uploadKind) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Unsupported upload_kind (no processor). Use roomplan_json, keyframes, or metadata.",
			"code":  "unsupported_upload_kind",
			"kind":  uploadKind,
		})
		return
	}
	if !validScanContentType(contentType, uploadKind) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported content type", "code": "invalid_content_type"})
		return
	}

	// Frame count cap for keyframe payloads.
	if uploadKind == database.ScanUploadKeyframes {
		var frames []interface{}
		if json.Unmarshal(payload, &frames) == nil {
			if len(frames) > spaceScanMaxFrames() {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Too many frames", "code": "max_frames_exceeded"})
				return
			}
		}
	}

	sum := sha256.Sum256(payload)
	computed := hex.EncodeToString(sum[:])
	if checksum != "" && checksum != computed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Checksum mismatch", "code": "checksum_mismatch"})
		return
	}
	checksum = computed

	key := fmt.Sprintf("biz-%d/session-%d/%s-%d-%s", session.BusinessID, session.ID, uploadKind, partIndex, checksum[:16])
	location, err := getSpaceScanStore().Put(key, payload, contentType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store upload"})
		return
	}

	byteSize := int64(len(payload))
	upload := &database.SpaceScanUpload{
		SessionID:      session.ID,
		BusinessID:     session.BusinessID,
		UploadKind:     uploadKind,
		ContentType:    &contentType,
		ByteSize:       &byteSize,
		ChecksumSHA256: &checksum,
		S3Key:          &location,
		PartIndex:      partIndex,
		IsComplete:     true,
	}
	if idempotencyKey != "" {
		upload.IdempotencyKey = &idempotencyKey
	}
	if err := database.CreateSpaceScanUpload(upload); err != nil {
		// Race on idempotency unique index.
		if idempotencyKey != "" {
			if existing, gerr := database.GetSpaceScanUploadByIdempotencyKey(idempotencyKey); gerr == nil {
				c.JSON(http.StatusOK, gin.H{"upload": existing, "idempotent": true})
				return
			}
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record upload"})
		return
	}

	// Mark session uploading if still early.
	if session.Status != database.ScanStatusUploading {
		msg := "uploading"
		_ = database.UpdateSpaceScanSessionStatus(session.BusinessID, session.ID, database.ScanStatusUploading, 30, &msg, nil, nil)
	}
	session, _ = database.GetSpaceScanSessionByID(session.BusinessID, session.ID)
	publishSpaceScanUpdated(session.BusinessID, session)

	c.JSON(http.StatusCreated, gin.H{"upload": upload})
}

// PublicSpaceScanCompleteUpload handles POST /api/v1/space-scan/:token/complete-upload
func PublicSpaceScanCompleteUpload(c *gin.Context) {
	session, ok := loadSessionByToken(c)
	if !ok {
		return
	}
	switch session.Status {
	case database.ScanStatusUploading, database.ScanStatusScanning, database.ScanStatusPhoneConnected:
		// ok
	case database.ScanStatusProcessing, database.ScanStatusReviewReady:
		c.JSON(http.StatusOK, gin.H{"session": session, "status": session.Status})
		return
	default:
		c.JSON(http.StatusConflict, gin.H{"error": "Cannot complete upload", "code": "invalid_status", "status": session.Status})
		return
	}

	uploads, err := database.ListSpaceScanUploads(session.BusinessID, session.ID)
	if err != nil || len(uploads) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No uploads present", "code": "no_uploads"})
		return
	}

	if err := database.TransitionSpaceScanSessionToProcessing(session.BusinessID, session.ID); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Failed to start processing", "code": "invalid_status"})
		return
	}
	services.EnqueueSpaceScanSession(session.ID)
	session, _ = database.GetSpaceScanSessionByID(session.BusinessID, session.ID)
	publishSpaceScanUpdated(session.BusinessID, session)
	c.JSON(http.StatusOK, gin.H{"session": session, "status": session.Status, "enqueued": true})
}

// PublicSpaceScanResult handles GET /api/v1/space-scan/:token/result.
// Short review window: expired sessions never return layout; completed/cancelled/
// revoked tokens 404. Token is revoked on cancel/complete (not on process).
func PublicSpaceScanResult(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found", "code": "not_found"})
		return
	}
	hash := database.HashScanToken(token)
	session, err := database.GetSpaceScanSessionByTokenHash(hash)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found", "code": "not_found"})
		return
	}
	// Always enforce expiry — no perpetual review_ready access after TTL.
	if time.Now().UTC().After(session.ExpiresAt) {
		if session.Status != database.ScanStatusExpired &&
			session.Status != database.ScanStatusCancelled &&
			session.Status != database.ScanStatusCompleted {
			_ = database.UpdateSpaceScanSessionStatus(session.BusinessID, session.ID, database.ScanStatusExpired, session.ProgressPct, nil, nil, nil)
			_ = database.RevokeSpaceScanSessionToken(session.BusinessID, session.ID)
		}
		c.JSON(http.StatusGone, gin.H{"error": "Session expired", "code": "expired"})
		return
	}
	switch session.Status {
	case database.ScanStatusCancelled, database.ScanStatusExpired, database.ScanStatusCompleted:
		c.JSON(http.StatusGone, gin.H{"error": "Session no longer active", "code": session.Status})
		return
	case database.ScanStatusReviewReady:
		// ok — only review-ready returns layout during the short window
	default:
		c.JSON(http.StatusConflict, gin.H{
			"error":  "Result not ready",
			"code":   "not_ready",
			"status": session.Status,
		})
		return
	}
	layout := session.ResultLayoutJSON
	if len(layout) == 0 {
		layout = database.JSONRawMessage(`{}`)
	}
	// Expose current draft_revision so mobile apply-review can send strict CAS
	// without an authenticated draft read.
	var draftRevision int64
	if session.SpaceID != nil && *session.SpaceID > 0 {
		if sp, err := database.GetRestaurantSpaceByID(session.BusinessID, *session.SpaceID); err == nil && sp != nil {
			draftRevision = sp.DraftRevision
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"status":             session.Status,
		"layout":             json.RawMessage(layout),
		"draft_revision":     draftRevision,
		"draft_apply_failed": session.ErrorCode != nil && *session.ErrorCode == "draft_apply_failed",
		"error_code":         session.ErrorCode,
		"error_message":      session.ErrorMessage,
		"expires_at":         session.ExpiresAt,
	})
}

// PublicSpaceScanApplyReview handles POST /api/v1/space-scan/:token/apply-review.
// Persists a filtered layout to the bound space draft and completes/revokes the session.
// This is the mobile (and optional desktop) path so "apply" is never local-only.
func PublicSpaceScanApplyReview(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found", "code": "not_found"})
		return
	}
	hash := database.HashScanToken(token)
	session, err := database.GetSpaceScanSessionByTokenHash(hash)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found", "code": "not_found"})
		return
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		_ = database.RevokeSpaceScanSessionToken(session.BusinessID, session.ID)
		c.JSON(http.StatusGone, gin.H{"error": "Session expired", "code": "expired"})
		return
	}
	if session.Status != database.ScanStatusReviewReady {
		c.JSON(http.StatusConflict, gin.H{"error": "Session not ready for review apply", "code": "invalid_status", "status": session.Status})
		return
	}
	if session.SpaceID == nil || *session.SpaceID == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Session not bound to a space", "code": "no_space"})
		return
	}

	var body struct {
		Layout           json.RawMessage `json:"layout" binding:"required"`
		ExpectedRevision int64           `json:"expected_revision"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	if len(body.Layout) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "layout required", "code": "invalid_argument"})
		return
	}
	// Strict CAS: public token path must not silently pin to "current" revision
	// (that races with concurrent operator edits and clobbers them).
	if body.ExpectedRevision <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "expected_revision is required",
			"code":  "expected_revision_required",
		})
		return
	}

	// Constrain token blast radius: submitted layout must be a filtered subset
	// of this session's result_layout_json (not an arbitrary draft rewrite).
	resultJSON := []byte(session.ResultLayoutJSON)
	if len(resultJSON) == 0 {
		resultJSON = []byte(`{}`)
	}
	space, res, uerr := spaceDomainService.ApplyReviewLayoutFromScanResult(
		session.BusinessID, *session.SpaceID, body.ExpectedRevision, resultJSON, body.Layout,
	)
	if uerr != nil {
		if mapSpaceDomainError(c, uerr) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to apply review layout"})
		return
	}

	// Complete + revoke so the token can no longer read the layout.
	if err := spaceDomainService.CompleteScanSession(session.BusinessID, session.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Layout applied but session complete failed"})
		return
	}
	publishSpaceScanUpdated(session.BusinessID, &database.SpaceScanSession{
		ID: session.ID, BusinessID: session.BusinessID, SpaceID: session.SpaceID,
		Status: database.ScanStatusCompleted, ProgressPct: 100,
	})

	c.JSON(http.StatusOK, gin.H{
		"status":         database.ScanStatusCompleted,
		"space":          space,
		"draft_revision": space.DraftRevision,
		"validation":     res,
		"completed":      true,
	})
}

// CompleteSpaceScanSession handles POST /inside/businesses/:id/scan-sessions/:sessionId/complete
// (authenticated operator path after desktop review apply).
func CompleteSpaceScanSession(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	sessionID, err := strconv.ParseUint(c.Param("sessionId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session id", "code": "invalid_argument"})
		return
	}
	session, err := database.GetSpaceScanSessionByID(business.ID, uint(sessionID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found", "code": "not_found"})
		return
	}
	if err := spaceDomainService.CompleteScanSession(business.ID, session.ID); err != nil {
		if mapSpaceDomainError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete session"})
		return
	}
	publishSpaceScanUpdated(business.ID, &database.SpaceScanSession{
		ID: session.ID, BusinessID: business.ID, SpaceID: session.SpaceID,
		Status: database.ScanStatusCompleted, ProgressPct: 100,
	})
	c.JSON(http.StatusOK, gin.H{"status": database.ScanStatusCompleted, "completed": true})
}

// processableScanUploadKinds are the only kinds with a SpaceScanProcessor.
// video/depth are rejected at upload until a processor ships.
func processableScanUploadKinds() map[string]bool {
	return map[string]bool{
		database.ScanUploadRoomPlanJSON: true,
		database.ScanUploadKeyframes:    true,
		database.ScanUploadMetadata:     true,
	}
}

func validScanUploadKind(kind string) bool {
	return processableScanUploadKinds()[kind]
}

func validScanContentType(ct, kind string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if ct == "" {
		return true
	}
	switch kind {
	case database.ScanUploadRoomPlanJSON, database.ScanUploadKeyframes, database.ScanUploadMetadata:
		return strings.HasPrefix(ct, "application/json") || ct == "application/octet-stream" || ct == "text/plain"
	default:
		return false
	}
}

func publishSpaceScanUpdated(businessID uint, session *database.SpaceScanSession) {
	if session == nil {
		return
	}
	payload := gin.H{
		"session_id":       session.ID,
		"status":           session.Status,
		"progress_pct":     session.ProgressPct,
		"progress_message": session.ProgressMessage,
	}
	if session.SpaceID != nil {
		payload["space_id"] = *session.SpaceID
	}
	events.GetHub().PublishJSON(businessID, "space.scan.updated", payload)
}
