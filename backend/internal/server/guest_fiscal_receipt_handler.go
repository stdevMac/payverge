package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
	"github.com/stdevmac/payverge/backend/internal/s3"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// latestGuestFiscalReceipt resolves the bill behind the guest capability token
// and returns its newest AUTHORIZED fiscal receipt. hasAny reports whether ANY
// fiscal receipt row exists for the bill (authorized or not) so callers can
// distinguish "pending" from "none". Demo seed rows, fiscal_disabled, and
// non-production tenants never surface as authorized (#544).
func latestGuestFiscalReceipt(ctx context.Context, token string) (*database.Bill, *database.FiscalReceipt, bool, error) {
	bill, _, err := loadPublicGuestBillByToken(token)
	if err != nil {
		return nil, nil, false, err
	}
	db := database.GetDB()
	if !fiscal.GuestMayExposeFiscalReceipt(ctx, db, bill.BusinessID) {
		return bill, nil, false, nil
	}
	var receipt database.FiscalReceipt
	dbErr := db.
		Select("id", "receipt_type", "receipt_number", "auth_code", "auth_expires_at",
			"issued_at", "created_at", "qr_payload", "pdf_path", "total_amount_cents").
		Where("bill_id = ? AND business_id = ? AND status = ?",
			bill.ID, bill.BusinessID, database.FiscalStatusAuthorized).
		Order("id DESC").
		First(&receipt).Error
	if dbErr == nil {
		if fiscal.IsDemoPlaceholderFiscalReceipt(&receipt) {
			return bill, nil, false, nil
		}
		return bill, &receipt, true, nil
	}
	if !errors.Is(dbErr, gorm.ErrRecordNotFound) {
		return nil, nil, false, dbErr
	}
	var count int64
	if err := db.Model(&database.FiscalReceipt{}).
		Where("bill_id = ? AND business_id = ?", bill.ID, bill.BusinessID).
		Count(&count).Error; err != nil {
		return nil, nil, false, err
	}
	return bill, nil, count > 0, nil
}

// GetGuestFiscalReceipt is the guest-side factura status endpoint:
// GET /guest/bill/:bill_token/fiscal-receipt. The opaque bill token is the
// capability; the response exposes only what the printed comprobante itself
// would show (no emitter credentials, no raw provider payloads).
func GetGuestFiscalReceipt(c *gin.Context) {
	token := strings.TrimSpace(c.Param("bill_token"))
	if token == "" {
		respondPublicGuestBillLookupError(c, errPublicGuestBillNotFound)
		return
	}
	_, receipt, hasAny, err := latestGuestFiscalReceipt(c.Request.Context(), token)
	if err != nil {
		respondPublicGuestBillLookupError(c, err)
		return
	}
	if receipt == nil {
		status := "none"
		if hasAny {
			status = "pending"
		}
		c.JSON(http.StatusOK, gin.H{"status": status})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":          "authorized",
		"receipt_type":    receipt.ReceiptType,
		"receipt_number":  strings.TrimSpace(strValueOrEmpty(receipt.ReceiptNumber)),
		"auth_code":       strings.TrimSpace(strValueOrEmpty(receipt.AuthCode)),
		"auth_expires_at": receipt.AuthExpiresAt,
		"issued_at":       receipt.IssuedAt,
		"qr_payload":      strings.TrimSpace(strValueOrEmpty(receipt.QRPayload)),
		"pdf_available":   strings.TrimSpace(strValueOrEmpty(receipt.PDFPath)) != "",
	})
}

// GetGuestFiscalReceiptPDF streams the factura PDF from protected S3:
// GET /guest/bill/:bill_token/fiscal-receipt/pdf.
func GetGuestFiscalReceiptPDF(c *gin.Context) {
	token := strings.TrimSpace(c.Param("bill_token"))
	if token == "" {
		respondPublicGuestBillLookupError(c, errPublicGuestBillNotFound)
		return
	}
	bill, receipt, _, err := latestGuestFiscalReceipt(c.Request.Context(), token)
	if err != nil {
		respondPublicGuestBillLookupError(c, err)
		return
	}
	if receipt == nil || strings.TrimSpace(strValueOrEmpty(receipt.PDFPath)) == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Fiscal receipt PDF not available"})
		return
	}
	data, err := s3.DownloadFileProtected(*receipt.PDFPath)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to load fiscal receipt PDF"})
		return
	}
	filename := fmt.Sprintf("factura-%s.pdf", bill.BillNumber)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "application/pdf", data)
}

// strValueOrEmpty returns the dereferenced string or "" for nil.
func strValueOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
