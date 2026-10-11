package handlers

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/accounting"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	s3pkg "github.com/stdevmac/payverge/backend/internal/s3"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
)

const maxLedgerAttachmentBytes = 10 << 20 // 10 MiB

// deleteLedgerAttachmentObject is indirected so tests can observe cleanup.
var deleteLedgerAttachmentObject = s3pkg.DeleteFileProtected

var allowedAttachmentTypes = map[string]struct{}{
	"application/pdf": {},
	"image/png":       {},
	"image/jpeg":      {},
	"image/webp":      {},
	"image/heic":      {},
}

// ListEntryAttachments GET /accounting/entries/:entryId/attachments
func (h *AccountingHandler) ListEntryAttachments(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	entryID, err := strconv.ParseUint(c.Param("entryId"), 10, 64)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "invalid entry id")
		return
	}
	if !h.entryBelongsToBusiness(uint(entryID), businessID) {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "entry not found")
		return
	}
	var rows []database.LedgerEntryAttachment
	if err := h.db.GetGorm().Where("entry_id = ? AND business_id = ?", entryID, businessID).
		Order("id ASC").Find(&rows).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list attachments")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

// UploadEntryAttachment POST multipart /accounting/entries/:entryId/attachments
func (h *AccountingHandler) UploadEntryAttachment(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	entryID, err := strconv.ParseUint(c.Param("entryId"), 10, 64)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "invalid entry id")
		return
	}
	var entry database.ManualLedgerEntry
	if err := h.db.GetGorm().Where("id = ? AND business_id = ?", entryID, businessID).First(&entry).Error; err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "entry not found")
		return
	}
	if err := accounting.CheckPeriodUnlocked(h.db.GetGorm(), businessID, entry.OccurredAt); err != nil {
		if respondPeriodLocked(c, err) {
			return
		}
	}

	file, hdr, err := c.Request.FormFile("file")
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "file is required")
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxLedgerAttachmentBytes+1))
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "failed to read file")
		return
	}
	if int64(len(raw)) > maxLedgerAttachmentBytes {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "file exceeds 10 MiB")
		return
	}
	ct, okType := detectAttachmentContentType(raw)
	if !okType {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "unsupported file type")
		return
	}
	name := fmt.Sprintf("%d_%s", time.Now().UnixNano(), sanitizeFileName(hdr.Filename))
	folder := fmt.Sprintf("ledger/%d/%d", businessID, entryID)
	location, err := s3pkg.UploadBytesProtected(raw, name, folder, ct)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to upload attachment")
		return
	}
	// Store the key (folder+name) for later download, not the full public-ish URL.
	key := folder + "/" + name
	_ = location
	row := database.LedgerEntryAttachment{
		EntryID:     uint(entryID),
		BusinessID:  businessID,
		S3Key:       key,
		FileName:    hdr.Filename,
		ContentType: ct,
		SizeBytes:   int64(len(raw)),
	}
	if uid, ok := c.Get("user_id"); ok {
		if id, ok := uid.(uint); ok {
			row.UploadedByUserID = &id
		}
	}
	if err := h.db.GetGorm().Create(&row).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to save attachment")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": row})
}

// DownloadEntryAttachment GET …/attachments/:attachmentId/download — streams protected object.
func (h *AccountingHandler) DownloadEntryAttachment(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	aid, err := strconv.ParseUint(c.Param("attachmentId"), 10, 64)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "invalid attachment id")
		return
	}
	var row database.LedgerEntryAttachment
	if err := h.db.GetGorm().Where("id = ? AND business_id = ?", aid, businessID).First(&row).Error; err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "attachment not found")
		return
	}
	body, err := s3pkg.DownloadFileProtected(row.S3Key)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to download attachment")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, row.FileName))
	c.Data(http.StatusOK, row.ContentType, body)
}

// DeleteEntryAttachment DELETE …/attachments/:attachmentId
func (h *AccountingHandler) DeleteEntryAttachment(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	aid, err := strconv.ParseUint(c.Param("attachmentId"), 10, 64)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "invalid attachment id")
		return
	}
	var row database.LedgerEntryAttachment
	if err := h.db.GetGorm().Where("id = ? AND business_id = ?", aid, businessID).First(&row).Error; err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "attachment not found")
		return
	}
	var entry database.ManualLedgerEntry
	if err := h.db.GetGorm().Where("id = ? AND business_id = ?", row.EntryID, businessID).First(&entry).Error; err == nil {
		if err := accounting.CheckPeriodUnlocked(h.db.GetGorm(), businessID, entry.OccurredAt); err != nil {
			if respondPeriodLocked(c, err) {
				return
			}
		}
	}
	if err := h.db.GetGorm().Delete(&row).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to delete attachment")
		return
	}
	// Best-effort object cleanup after the row is gone: a failure leaves an
	// unreachable orphan (logged) rather than a row pointing at nothing.
	if err := deleteLedgerAttachmentObject(row.S3Key); err != nil {
		logger.Logger.Warnf("[accounting] attachment %d object cleanup failed (orphan key %s): %v", row.ID, row.S3Key, err)
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *AccountingHandler) entryBelongsToBusiness(entryID, businessID uint) bool {
	var n int64
	h.db.GetGorm().Model(&database.ManualLedgerEntry{}).
		Where("id = ? AND business_id = ?", entryID, businessID).Count(&n)
	return n > 0
}

// detectAttachmentContentType derives the stored content type from the BYTES,
// never the filename: DetectContentType covers pdf/png/jpeg/webp; HEIC is not
// sniffed by the stdlib so it gets an explicit ISO-BMFF ftyp check. Anything
// else is rejected — trusting the extension would let arbitrary bytes (e.g.
// HTML) be stored and served under an allowed type.
func detectAttachmentContentType(raw []byte) (string, bool) {
	ct := http.DetectContentType(raw)
	if _, ok := allowedAttachmentTypes[ct]; ok {
		return ct, true
	}
	if isHEICBytes(raw) {
		return "image/heic", true
	}
	return "", false
}

// isHEICBytes verifies the ISO-BMFF ftyp box with a HEIF brand, since
// http.DetectContentType cannot sniff HEIC (golang/go#52144).
func isHEICBytes(raw []byte) bool {
	if len(raw) < 12 || string(raw[4:8]) != "ftyp" {
		return false
	}
	switch string(raw[8:12]) {
	case "heic", "heix", "hevc", "hevx", "heim", "heis", "mif1", "msf1":
		return true
	}
	return false
}

func sanitizeFileName(name string) string {
	base := filepath.Base(name)
	base = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, base)
	if base == "" || base == "." {
		return "file"
	}
	return base
}
