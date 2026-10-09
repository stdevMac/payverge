package fiscal

import (
	"context"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
)

// GuestMayExposeFiscalReceipt reports whether the public guest factura
// contract may surface any fiscal status for this business. Fail-closed:
// a missing/disabled fiscal_enabled control or a demo/test tenant is hidden.
func GuestMayExposeFiscalReceipt(ctx context.Context, db *gorm.DB, businessID uint) bool {
	if db == nil || businessID == 0 {
		return false
	}
	enabled, err := automaticFiscalEnabled(ctx, db)
	if err != nil || !enabled {
		return false
	}
	return guestProductionFiscalBusiness(db, businessID)
}

// guestProductionFiscalBusiness is the guest-facing counterpart of
// isProductionFiscalBusiness. Missing rows fail closed (unlike the enqueue
// helper, which stays open so unit tests that seed only bills still work).
func guestProductionFiscalBusiness(db *gorm.DB, businessID uint) bool {
	if db == nil || businessID == 0 {
		return false
	}
	var biz database.Business
	err := db.Select("id", "kind", "is_demo").Where("id = ?", businessID).First(&biz).Error
	if err != nil {
		return false
	}
	if biz.IsDemo {
		return false
	}
	switch biz.Kind {
	case database.BusinessKindDemo, database.BusinessKindTest:
		return false
	default:
		return true
	}
}

// IsDemoPlaceholderFiscalReceipt reports seed/demo markers that must never
// be returned as an authorized CAE/QR on the guest contract.
func IsDemoPlaceholderFiscalReceipt(receipt *database.FiscalReceipt) bool {
	if receipt == nil {
		return false
	}
	auth := strings.ToUpper(strings.TrimSpace(ptrStringValue(receipt.AuthCode)))
	if strings.HasPrefix(auth, "AUTH-") {
		return true
	}
	number := strings.ToUpper(strings.TrimSpace(ptrStringValue(receipt.ReceiptNumber)))
	if strings.HasPrefix(number, "DEMO-") {
		return true
	}
	qr := strings.ToLower(strings.TrimSpace(ptrStringValue(receipt.QRPayload)))
	return strings.Contains(qr, "payverge.local")
}

func ptrStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
