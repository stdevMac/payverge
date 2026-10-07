package handlers

import (
	"errors"
	"math"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var errEmailUnavailable = errors.New("email service unavailable")

// sendGuestReceiptEmail is a test seam over the global email server. It uses
// the SAME sender + template as the CRM auto-receipt (payments.go), so the
// guest-initiated copy is pixel-identical to the automatic one.
// businessID/billID are the origin refs a later bounce is attributed to (#562).
// The guest asked for this copy explicitly, so a send swallowed by the tenant
// dedupe window comes back as emails.ErrTenantMailDuplicate rather than nil.
var sendGuestReceiptEmail = func(to []string, businessName, paymentDate, paymentMethod, transactionID string, items []map[string]interface{}, totalAmount, language string, businessID, billID uint) error {
	if emails.EmailServerInstance == nil {
		return errEmailUnavailable
	}
	return emails.EmailServerInstance.ForReceipt(businessID, billID).ReportingDuplicates().SendPaymentReceiptEmail(
		to, businessName, paymentDate, paymentMethod, transactionID, items, totalAmount, language,
	)
}

type emailReceiptRequest struct {
	Email         string `json:"email" binding:"required"`
	Language      string `json:"language"`
	PaymentMethod string `json:"payment_method"`
	TransactionID string `json:"transaction_id"`
}

// EmailBillReceipt emails a receipt for a SETTLED bill to an address already
// associated with that bill. Public route keyed by the opaque bill
// public_token capability; rate-limited at the router (guestOrderRateLimiter)
// like every other guest-bill mutation.
// POST /api/v1/guest/bill/:bill_token/email-receipt
func (h *PaymentHandler) EmailBillReceipt(c *gin.Context) {
	var req emailReceiptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Email is required")
		return
	}
	address, err := mail.ParseAddress(strings.TrimSpace(req.Email))
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid email address")
		return
	}

	bill, err := h.resolveBillPaymentSummary(c, "")
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Bill not found")
		return
	}
	if bill.Status != database.BillStatusPaid && bill.Status != database.BillStatusClosed {
		server.RespondWithError(c, http.StatusConflict, "bill_not_settled", "Receipt is available once the bill is settled")
		return
	}

	redactedRecipient := logger.RedactEmail(address.Address)
	if h.db == nil || h.db.GetGorm() == nil {
		server.RespondWithError(c, http.StatusServiceUnavailable, "email_unavailable", "Could not send the receipt right now")
		return
	}
	if !guestReceiptRecipientAllowed(h.db.GetGorm(), bill.ID, address.Address) {
		server.RespondWithError(c, http.StatusForbidden, "receipt_recipient_unbound", "Receipt can only be sent to an email already associated with this bill")
		return
	}
	slotID, err := database.ClaimGuestReceiptSendSlot(h.db.GetGorm(), bill.ID, redactedRecipient)
	if errors.Is(err, database.ErrGuestReceiptSendLimit) {
		server.RespondWithError(c, http.StatusTooManyRequests, "receipt_send_limit", "Receipt email limit reached for this bill. Try again later.")
		return
	}
	if err != nil {
		logger.Logger.Warnf("Guest receipt send cap for bill %s failed: %v", bill.BillNumber, err)
		server.RespondWithError(c, http.StatusServiceUnavailable, "email_unavailable", "Could not send the receipt right now")
		return
	}

	business, err := database.GetBusinessByID(bill.BusinessID)
	if err != nil || business == nil {
		_ = database.ReleaseGuestReceiptSendSlot(h.db.GetGorm(), slotID)
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Business not found")
		return
	}

	language := strings.TrimSpace(req.Language)
	if !locales.IsGuestLocale(language) {
		language = business.DefaultLanguage
	}
	if language == "" {
		language = "en"
	}

	// Display currency for guest-facing amounts (mirrors payments.go:2297-2305).
	currency := strings.TrimSpace(business.DisplayCurrency)
	if currency == "" {
		currency = strings.TrimSpace(business.DefaultCurrency)
	}
	if currency == "" {
		currency = "USD"
	}

	items, err := h.db.GetBillItems(bill.ID)
	if err != nil {
		items = []database.BillItem{}
	}
	receiptItems := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		lineCents := int64(math.Round(item.Price * float64(item.Quantity) * 100))
		receiptItems = append(receiptItems, map[string]interface{}{
			"name":       item.Name,
			"quantity":   item.Quantity,
			"line_total": receiptEmailMoney(lineCents, currency),
		})
	}

	if err := sendGuestReceiptEmail(
		[]string{address.Address},
		business.Name,
		receiptEmailDate(time.Now(), language),
		sanitizeReceiptPaymentMethod(req.PaymentMethod),
		sanitizeReceiptTransactionID(req.TransactionID),
		receiptItems,
		receiptEmailMoney(bill.TotalAmount, currency),
		language,
		bill.BusinessID,
		bill.ID,
	); err != nil {
		_ = database.ReleaseGuestReceiptSendSlot(h.db.GetGorm(), slotID)
		if errors.Is(err, emails.ErrTenantMailDuplicate) {
			// The identical receipt went to this address moments ago (tenant
			// dedupe window), so nothing new was mailed and the per-bill slot is
			// handed back. The guest does have the receipt, so this stays a
			// success, but already_sent tells the client no second copy is coming.
			c.JSON(http.StatusOK, gin.H{
				"success":      true,
				"already_sent": true,
				"message":      "This receipt was emailed to this address a few minutes ago. Check your inbox and spam folder.",
			})
			return
		}
		if errors.Is(err, emails.ErrTenantMailBudgetExceeded) {
			// The venue's outbound email budget (EMAIL_TENANT_*) is spent.
			// That is a quota, not an outage: say so and let the guest retry later.
			logger.Logger.Warnf("Guest receipt email for bill %s to %s refused by tenant email budget: %v", bill.BillNumber, redactedRecipient, err)
			server.RespondWithError(c, http.StatusTooManyRequests, "email_budget_exceeded", "Email limit reached for this venue. Try again later.")
			return
		}
		logger.Logger.Warnf("Guest receipt email for bill %s to %s failed: %v", bill.BillNumber, redactedRecipient, err)
		server.RespondWithError(c, http.StatusServiceUnavailable, "email_unavailable", "Could not send the receipt right now")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "already_sent": false})
}

// Guest-supplied receipt display strings. They render in escaped template
// cells, but the receipt goes to an inbox, so neither may carry free text
// (a URL, an address, a sentence) on the venue's sending domain.
const (
	maxReceiptPaymentMethodRunes = 40
	maxReceiptTransactionIDLen   = 80
)

// sanitizeReceiptPaymentMethod keeps a payment-method label only when it looks
// like one: letters (any script, with combining marks), digits, spaces and
// "()-/_+&", at most 40 runes. Anything else, such as "." or ":" for a link or
// "@" for an address, drops the label instead of forwarding it.
func sanitizeReceiptPaymentMethod(value string) string {
	trimmed := strings.Join(strings.Fields(value), " ")
	if trimmed == "" || utf8.RuneCountInString(trimmed) > maxReceiptPaymentMethodRunes {
		return ""
	}
	for _, r := range trimmed {
		switch {
		case unicode.IsLetter(r), unicode.IsMark(r), unicode.IsDigit(r), r == ' ':
		case strings.ContainsRune("()-/_+&", r):
		default:
			return ""
		}
	}
	return trimmed
}

// sanitizeReceiptTransactionID keeps a provider or chain reference
// ([A-Za-z0-9_-], at most 80 bytes; a 0x tx hash is 66) and drops anything else.
func sanitizeReceiptTransactionID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(trimmed) > maxReceiptTransactionIDLen {
		return ""
	}
	for i := 0; i < len(trimmed); i++ {
		ch := trimmed[i]
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-') {
			return ""
		}
	}
	return trimmed
}

// guestReceiptRecipientAllowed is true only when address already appears on
// the bill (fiscal_customer_email), its linked CRM customer, or a delivery
// order for that bill. An empty association set rejects every send.
func guestReceiptRecipientAllowed(db *gorm.DB, billID uint, address string) bool {
	if db == nil {
		return false
	}
	want := strings.TrimSpace(address)
	if want == "" {
		return false
	}
	for _, have := range associatedGuestReceiptEmails(db, billID) {
		if strings.EqualFold(want, have) {
			return true
		}
	}
	return false
}

func associatedGuestReceiptEmails(db *gorm.DB, billID uint) []string {
	var emails []string
	add := func(raw string) {
		if trimmed := strings.TrimSpace(raw); trimmed != "" {
			emails = append(emails, trimmed)
		}
	}

	var bill database.Bill
	if err := db.Select("fiscal_customer_email", "crm_customer_id").First(&bill, billID).Error; err != nil {
		return emails
	}
	if bill.FiscalCustomerEmail != nil {
		add(*bill.FiscalCustomerEmail)
	}
	if bill.CRMCustomerID != nil && *bill.CRMCustomerID != 0 {
		var email string
		if err := db.Model(&database.Customer{}).
			Select("email").
			Where("id = ?", *bill.CRMCustomerID).
			Limit(1).
			Scan(&email).Error; err == nil {
			add(email)
		}
	}
	var deliveryEmail string
	if err := db.Model(&database.DeliveryOrder{}).
		Select("customer_email").
		Where("bill_id = ?", billID).
		Order("id DESC").
		Limit(1).
		Scan(&deliveryEmail).Error; err == nil {
		add(deliveryEmail)
	}
	return emails
}
