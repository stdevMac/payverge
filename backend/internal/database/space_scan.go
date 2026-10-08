package database

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ErrSpaceScanSessionNotFound is returned for missing/foreign scan sessions.
var ErrSpaceScanSessionNotFound = errors.New("space scan session not found")

// ErrSpaceScanUploadNotFound is returned for missing/foreign scan uploads.
var ErrSpaceScanUploadNotFound = errors.New("space scan upload not found")

// HashScanToken returns the SHA-256 hex digest of an opaque scan token.
// Only the hash is stored; the raw token is never persisted.
func HashScanToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

// ScanTokenPrefix returns the first 8 characters of the raw token for support display.
func ScanTokenPrefix(rawToken string) string {
	if len(rawToken) <= 8 {
		return rawToken
	}
	return rawToken[:8]
}

// CreateSpaceScanSession inserts a new scan session. rawToken is hashed before store.
func CreateSpaceScanSession(session *SpaceScanSession, rawToken string) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	if session.BusinessID == 0 {
		return fmt.Errorf("business_id is required")
	}
	if rawToken == "" {
		return fmt.Errorf("raw token is required")
	}
	if session.ExpiresAt.IsZero() {
		return fmt.Errorf("expires_at is required")
	}
	session.TokenHash = HashScanToken(rawToken)
	session.TokenPrefix = ScanTokenPrefix(rawToken)
	if session.Status == "" {
		session.Status = ScanStatusWaitingForPhone
	}
	if len(session.DeviceMetaJSON) == 0 {
		session.DeviceMetaJSON = JSONRawMessage(`{}`)
	}
	if err := db.Create(session).Error; err != nil {
		return fmt.Errorf("create space scan session: %w", err)
	}
	return nil
}

// GetSpaceScanSessionByID returns a session only when it belongs to businessID.
func GetSpaceScanSessionByID(businessID, sessionID uint) (*SpaceScanSession, error) {
	var s SpaceScanSession
	err := db.Where("id = ? AND business_id = ?", sessionID, businessID).First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSpaceScanSessionNotFound
		}
		return nil, fmt.Errorf("get space scan session: %w", err)
	}
	return &s, nil
}

// GetSpaceScanSessionByIDUnscoped loads a session by primary key only.
// For worker/janitor paths that already hold a trusted session id from the queue
// (not an HTTP client). Prefer GetSpaceScanSessionByID for request handlers.
func GetSpaceScanSessionByIDUnscoped(sessionID uint) (*SpaceScanSession, error) {
	if sessionID == 0 {
		return nil, ErrSpaceScanSessionNotFound
	}
	var s SpaceScanSession
	err := db.Where("id = ?", sessionID).First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSpaceScanSessionNotFound
		}
		return nil, fmt.Errorf("get space scan session by id: %w", err)
	}
	return &s, nil
}

// GetSpaceScanSessionByTokenHash looks up a session by token hash (phone join path).
// Still returns business_id on the row so callers can re-scope subsequent queries.
func GetSpaceScanSessionByTokenHash(tokenHash string) (*SpaceScanSession, error) {
	var s SpaceScanSession
	err := db.Where("token_hash = ?", tokenHash).First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSpaceScanSessionNotFound
		}
		return nil, fmt.Errorf("get space scan session by token: %w", err)
	}
	return &s, nil
}

// ListSpaceScanSessions lists sessions for a business, optionally filtered by status.
func ListSpaceScanSessions(businessID uint, status string) ([]SpaceScanSession, error) {
	var sessions []SpaceScanSession
	q := db.Where("business_id = ?", businessID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Order("created_at DESC").Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("list space scan sessions: %w", err)
	}
	return sessions, nil
}

// UpdateSpaceScanSessionStatus updates status/progress fields with business scoping.
func UpdateSpaceScanSessionStatus(
	businessID, sessionID uint,
	status string,
	progressPct int,
	progressMessage *string,
	errorCode, errorMessage *string,
) error {
	updates := map[string]interface{}{
		"status":       status,
		"progress_pct": progressPct,
		"updated_at":   time.Now().UTC(),
	}
	if progressMessage != nil {
		updates["progress_message"] = *progressMessage
	}
	if errorCode != nil {
		updates["error_code"] = *errorCode
	}
	if errorMessage != nil {
		updates["error_message"] = *errorMessage
	}
	now := time.Now().UTC()
	switch status {
	case ScanStatusPhoneConnected:
		updates["connected_at"] = now
	case ScanStatusCompleted, ScanStatusReviewReady, ScanStatusFailed, ScanStatusCancelled, ScanStatusExpired:
		updates["completed_at"] = now
	}
	res := db.Model(&SpaceScanSession{}).
		Where("id = ? AND business_id = ?", sessionID, businessID).
		Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("update space scan session: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrSpaceScanSessionNotFound
	}
	return nil
}

// SetSpaceScanSessionResult stores the processed draft layout result.
func SetSpaceScanSessionResult(businessID, sessionID uint, layout JSONRawMessage, status string) error {
	if status == "" {
		status = ScanStatusReviewReady
	}
	now := time.Now().UTC()
	res := db.Model(&SpaceScanSession{}).
		Where("id = ? AND business_id = ?", sessionID, businessID).
		Updates(map[string]interface{}{
			"result_layout_json": layout,
			"status":             status,
			"progress_pct":       100,
			"completed_at":       now,
			"updated_at":         now,
		})
	if res.Error != nil {
		return fmt.Errorf("set space scan session result: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrSpaceScanSessionNotFound
	}
	return nil
}

// ExpireStaleSpaceScanSessions marks active sessions past expires_at as expired.
func ExpireStaleSpaceScanSessions(now time.Time) (int64, error) {
	active := []string{
		ScanStatusWaitingForPhone,
		ScanStatusPhoneConnected,
		ScanStatusScanning,
		ScanStatusUploading,
		ScanStatusProcessing,
		ScanStatusReviewReady,
	}
	res := db.Model(&SpaceScanSession{}).
		Where("status IN ? AND expires_at < ?", active, now).
		Updates(map[string]interface{}{
			"status":       ScanStatusExpired,
			"completed_at": now,
			"updated_at":   now,
		})
	if res.Error != nil {
		return 0, fmt.Errorf("expire stale scan sessions: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// CreateSpaceScanUpload records an uploaded artifact for a session.
func CreateSpaceScanUpload(upload *SpaceScanUpload) error {
	if upload == nil {
		return fmt.Errorf("upload is nil")
	}
	if upload.BusinessID == 0 || upload.SessionID == 0 {
		return fmt.Errorf("business_id and session_id are required")
	}
	if upload.UploadKind == "" {
		return fmt.Errorf("upload_kind is required")
	}
	// Tenant scope: session must belong to same business.
	if _, err := GetSpaceScanSessionByID(upload.BusinessID, upload.SessionID); err != nil {
		return err
	}
	if upload.FormatVersion == 0 {
		upload.FormatVersion = 1
	}
	if err := db.Create(upload).Error; err != nil {
		return fmt.Errorf("create space scan upload: %w", err)
	}
	return nil
}

// ListSpaceScanUploads lists uploads for a session scoped to businessID.
func ListSpaceScanUploads(businessID, sessionID uint) ([]SpaceScanUpload, error) {
	var uploads []SpaceScanUpload
	if err := db.Where("business_id = ? AND session_id = ?", businessID, sessionID).
		Order("part_index ASC, id ASC").Find(&uploads).Error; err != nil {
		return nil, fmt.Errorf("list space scan uploads: %w", err)
	}
	return uploads, nil
}

// GetSpaceScanUploadByIdempotencyKey returns an upload by its unique idempotency key.
func GetSpaceScanUploadByIdempotencyKey(idempotencyKey string) (*SpaceScanUpload, error) {
	if idempotencyKey == "" {
		return nil, ErrSpaceScanUploadNotFound
	}
	var u SpaceScanUpload
	err := db.Where("idempotency_key = ?", idempotencyKey).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSpaceScanUploadNotFound
		}
		return nil, fmt.Errorf("get space scan upload by idempotency: %w", err)
	}
	return &u, nil
}

// RevokeSpaceScanSessionToken overwrites token_hash so the raw token can no
// longer resolve the session (cancel/complete). Prefix is kept for support.
func RevokeSpaceScanSessionToken(businessID, sessionID uint) error {
	// Deterministic revoked marker unique per session (satisfies unique constraint).
	revokedHash := HashScanToken(fmt.Sprintf("revoked:%d:%d:%d", businessID, sessionID, time.Now().UnixNano()))
	res := db.Model(&SpaceScanSession{}).
		Where("id = ? AND business_id = ?", sessionID, businessID).
		Updates(map[string]interface{}{
			"token_hash": revokedHash,
			"updated_at": time.Now().UTC(),
		})
	if res.Error != nil {
		return fmt.Errorf("revoke scan token: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrSpaceScanSessionNotFound
	}
	return nil
}

// ListSpaceScanSessionsNeedingWork returns sessions in processing status for the worker.
func ListSpaceScanSessionsNeedingWork(limit int) ([]SpaceScanSession, error) {
	if limit <= 0 {
		limit = 20
	}
	var sessions []SpaceScanSession
	if err := db.Where("status = ?", ScanStatusProcessing).
		Order("updated_at ASC").
		Limit(limit).
		Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("list scan sessions needing work: %w", err)
	}
	return sessions, nil
}

// ListSpaceScanUploadsPastRetention returns complete uploads whose parent
// session retain_raw_until is before now (for S3 cleanup).
func ListSpaceScanUploadsPastRetention(now time.Time, limit int) ([]SpaceScanUpload, error) {
	if limit <= 0 {
		limit = 100
	}
	var uploads []SpaceScanUpload
	// Join-style: sessions where retain_raw_until < now, then uploads with s3_key set.
	if err := db.Table("space_scan_uploads AS u").
		Select("u.*").
		Joins("INNER JOIN space_scan_sessions AS s ON s.id = u.session_id").
		Where("u.s3_key IS NOT NULL AND u.s3_key <> '' AND s.retain_raw_until IS NOT NULL AND s.retain_raw_until < ?", now).
		Limit(limit).
		Find(&uploads).Error; err != nil {
		return nil, fmt.Errorf("list scan uploads past retention: %w", err)
	}
	return uploads, nil
}

// ClearSpaceScanUploadS3Key nulls s3_key after successful artifact deletion.
func ClearSpaceScanUploadS3Key(businessID, uploadID uint) error {
	res := db.Model(&SpaceScanUpload{}).
		Where("id = ? AND business_id = ?", uploadID, businessID).
		Updates(map[string]interface{}{
			"s3_key": nil,
		})
	if res.Error != nil {
		return fmt.Errorf("clear scan upload s3_key: %w", res.Error)
	}
	return nil
}

// TransitionSpaceScanSessionToProcessing moves uploading → processing if still active.
func TransitionSpaceScanSessionToProcessing(businessID, sessionID uint) error {
	now := time.Now().UTC()
	msg := "processing scan"
	res := db.Model(&SpaceScanSession{}).
		Where("id = ? AND business_id = ? AND status IN ?", sessionID, businessID,
			[]string{ScanStatusUploading, ScanStatusScanning, ScanStatusPhoneConnected, ScanStatusWaitingForPhone}).
		Updates(map[string]interface{}{
			"status":           ScanStatusProcessing,
			"progress_pct":     50,
			"progress_message": msg,
			"updated_at":       now,
		})
	if res.Error != nil {
		return fmt.Errorf("transition scan to processing: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// Already processing or terminal — check current state.
		s, err := GetSpaceScanSessionByID(businessID, sessionID)
		if err != nil {
			return err
		}
		if s.Status == ScanStatusProcessing {
			return nil
		}
		return fmt.Errorf("session cannot enter processing from status %s", s.Status)
	}
	return nil
}

// ExpireSpaceScanPairCode removes the pair code from a session that is still
// waiting_for_phone, so no pair code can connect it any more. It only touches
// pending sessions: a session a device already paired keeps its state, token,
// and pair code. Operators with business access can still connect, or start a
// new scan for a fresh code. Returns whether a pending code was expired.
func ExpireSpaceScanPairCode(businessID, sessionID uint) (bool, error) {
	expired := false
	err := db.Transaction(func(tx *gorm.DB) error {
		var s SpaceScanSession
		if err := tx.Select("id", "device_meta_json").
			Where("id = ? AND business_id = ? AND status = ?", sessionID, businessID, ScanStatusWaitingForPhone).
			First(&s).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		meta := map[string]interface{}{}
		if len(s.DeviceMetaJSON) > 0 {
			_ = json.Unmarshal(s.DeviceMetaJSON, &meta)
		}
		if _, ok := meta["pair_code_hash"]; !ok {
			return nil
		}
		delete(meta, "pair_code_hash")
		meta["pair_code_expired"] = true
		raw, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		res := tx.Model(&SpaceScanSession{}).
			Where("id = ? AND business_id = ? AND status = ?", sessionID, businessID, ScanStatusWaitingForPhone).
			Updates(map[string]interface{}{"device_meta_json": JSONRawMessage(raw), "updated_at": time.Now().UTC()})
		if res.Error != nil {
			return res.Error
		}
		expired = res.RowsAffected > 0
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("expire space scan pair code: %w", err)
	}
	return expired, nil
}
