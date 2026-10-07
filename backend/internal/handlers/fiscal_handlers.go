package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
	"github.com/stdevmac/payverge/backend/internal/server"
)

const maxCredentialBytes = 64 * 1024 // 64 KB per PEM file

type fiscalService interface {
	UpdateSettings(ctx context.Context, settings *database.BusinessFiscalSettings) error
	GetSettings(ctx context.Context, businessID uint) (*database.BusinessFiscalSettings, error)
	ListReceipts(ctx context.Context, businessID uint, status string) ([]database.FiscalReceipt, error)
	ListReceiptsPage(ctx context.Context, params fiscal.ListReceiptsParams) (*fiscal.ReceiptsPage, error)
	ListIssuableBills(ctx context.Context, businessID uint, q string) ([]fiscal.IssuableBill, error)
	IssueReceipt(ctx context.Context, businessID, billID uint, actor string) error
	IssueReceiptWithReceiver(ctx context.Context, businessID, billID uint, actor string, override *fiscal.ReceiverOverride) error
	RetryReceipt(ctx context.Context, businessID, receiptID uint, actor string) error
	ResendReceipt(ctx context.Context, businessID, receiptID uint, actor string) error
	IssueCreditNote(ctx context.Context, businessID, receiptID uint, amountCents int64, discriminator, reason, actor string) error
	SetCredentials(ctx context.Context, businessID uint, certPEM, keyPEM string) error
	ValidateSettings(ctx context.Context, businessID uint) (*database.BusinessFiscalSettings, error)
	ListReceiptDeliveryTasks(ctx context.Context, businessID, receiptID uint) ([]fiscal.DeliveryTaskView, error)
	RetryDeliveryTask(ctx context.Context, businessID, taskID uint, actor string) error
}

// FiscalHandlers exposes fiscal compliance settings and receipt endpoints.
type FiscalHandlers struct {
	svc fiscalService
}

func NewFiscalHandlers(svc *fiscal.Service) *FiscalHandlers {
	return &FiscalHandlers{svc: svc}
}

type fiscalSettingsRequest struct {
	Country      string              `json:"country"`
	Provider     string              `json:"provider"`
	Mode         database.FiscalMode `json:"mode"`
	Environment  string              `json:"environment"`
	TaxID        string              `json:"tax_id"`
	TaxCondition string              `json:"tax_condition"`
	PointOfSale  *int                `json:"point_of_sale"`
}

func (h *FiscalHandlers) UpdateSettings(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	var req fiscalSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if !req.Mode.IsValid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid fiscal mode"})
		return
	}
	settings := database.BusinessFiscalSettings{
		BusinessID:     businessID,
		Country:        req.Country,
		Provider:       req.Provider,
		Mode:           req.Mode,
		Environment:    req.Environment,
		TaxID:          req.TaxID,
		TaxCondition:   req.TaxCondition,
		PointOfSale:    req.PointOfSale,
		SetupStatus:    "draft",
		ProviderConfig: map[string]interface{}{},
	}
	if err := h.svc.UpdateSettings(c.Request.Context(), &settings); err != nil {
		if errors.Is(err, fiscal.ErrInvalidSettings) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// FIND-060: never surface GORM/driver/provider text on 500.
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not save fiscal settings")
		return
	}
	saved, err := h.svc.GetSettings(c.Request.Context(), businessID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not load fiscal settings")
		return
	}
	if saved == nil {
		saved = &settings
	}
	c.JSON(http.StatusOK, saved)
}

func (h *FiscalHandlers) GetSettings(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	settings, err := h.svc.GetSettings(c.Request.Context(), businessID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not load fiscal settings")
		return
	}
	c.JSON(http.StatusOK, gin.H{"settings": settings})
}

func (h *FiscalHandlers) ListReceipts(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}

	// page present (non-empty) → paginated envelope with delivery embed;
	// absent → legacy {"items": [...]} shape (no delivery badges).
	if pageStr := strings.TrimSpace(c.Query("page")); pageStr != "" {
		page, err := strconv.Atoi(pageStr)
		if err != nil || page < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page"})
			return
		}
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

		params := fiscal.ListReceiptsParams{
			BusinessID:  businessID,
			Status:      strings.TrimSpace(c.Query("status")),
			ReceiptType: strings.TrimSpace(c.Query("receipt_type")),
			Q:           strings.TrimSpace(c.Query("q")),
			Page:        page,
			PageSize:    pageSize,
		}

		if startStr := strings.TrimSpace(c.Query("start")); startStr != "" {
			start, err := time.Parse("2006-01-02", startStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start date"})
				return
			}
			start = start.UTC()
			params.Start = &start
		}
		if endStr := strings.TrimSpace(c.Query("end")); endStr != "" {
			end, err := time.Parse("2006-01-02", endStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end date"})
				return
			}
			// Half-open [start, end): treat YYYY-MM-DD end as exclusive next midnight.
			end = end.AddDate(0, 0, 1).UTC()
			params.End = &end
		}

		switch strings.ToLower(strings.TrimSpace(c.Query("needs_attention"))) {
		case "1", "true", "yes":
			params.NeedsAttention = true
		}

		result, err := h.svc.ListReceiptsPage(c.Request.Context(), params)
		if err != nil {
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not list fiscal receipts")
			return
		}
		// F-PII: mask customer tax-doc numbers on every row before response.
		for i := range result.Receipts {
			if result.Receipts[i].CustomerDocNumber != nil {
				masked := maskCustomerDocNumber(*result.Receipts[i].CustomerDocNumber)
				result.Receipts[i].CustomerDocNumber = &masked
			}
		}
		// Top-level ReceiptsPage fields (receipts, total, page, page_size, total_pages).
		c.JSON(http.StatusOK, result)
		return
	}

	receipts, err := h.svc.ListReceipts(c.Request.Context(), businessID, c.Query("status"))
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not list fiscal receipts")
		return
	}
	// F-PII: mask the customer's CUIT/DNI in the list response so recipient tax-doc
	// numbers are not exposed wholesale to every fiscal:read holder. The receipts
	// slice is a fresh query result, so mutating it here is safe.
	for i := range receipts {
		if receipts[i].CustomerDocNumber != nil {
			masked := maskCustomerDocNumber(*receipts[i].CustomerDocNumber)
			receipts[i].CustomerDocNumber = &masked
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": receipts})
}

// maskCustomerDocNumber redacts all but the last 3 characters of a tax document
// number (CUIT/DNI). Values of 3 or fewer characters are returned unchanged (there
// is nothing meaningful to mask).
func maskCustomerDocNumber(doc string) string {
	d := strings.TrimSpace(doc)
	const keep = 3
	if len(d) <= keep {
		return d
	}
	return strings.Repeat("*", len(d)-keep) + d[len(d)-keep:]
}

// ListIssuableBills handles GET /businesses/:id/fiscal/issuable-bills for the
// invoice-picker drawer: recent paid bills (issuable and already invoiced),
// optional q on bill number / table / guest, limit 20 newest first.
func (h *FiscalHandlers) ListIssuableBills(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	items, err := h.svc.ListIssuableBills(c.Request.Context(), businessID, c.Query("q"))
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not list issuable bills")
		return
	}
	if items == nil {
		items = []fiscal.IssuableBill{}
	}
	// L6-21: envelope matches the bill-list siblings (`.bills`).
	c.JSON(http.StatusOK, gin.H{"bills": items})
}

type issueReceiptRequest struct {
	BillID               uint   `json:"bill_id"`
	CustomerDocType      string `json:"customer_doc_type"`
	CustomerDocNumber    string `json:"customer_doc_number"`
	CustomerTaxCondition string `json:"customer_tax_condition"`
	CustomerName         string `json:"customer_name"`
}

func (h *FiscalHandlers) IssueReceipt(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	var req issueReceiptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if req.BillID == 0 {
		// Decided here, ahead of any fiscal service or provider work: a request
		// that names no bill is a client validation failure, and it answers with
		// a machine code so the operator UI can localize it per field instead of
		// echoing an English sentence back at the cashier (#906).
		server.RespondWithFieldError(c, http.StatusBadRequest, "bill_id", "bill_id is required")
		return
	}
	var override *fiscal.ReceiverOverride
	if req.CustomerDocType != "" || req.CustomerDocNumber != "" ||
		req.CustomerTaxCondition != "" || req.CustomerName != "" {
		override = &fiscal.ReceiverOverride{
			CustomerDocType:      req.CustomerDocType,
			CustomerDocNumber:    req.CustomerDocNumber,
			CustomerTaxCondition: req.CustomerTaxCondition,
			CustomerName:         req.CustomerName,
		}
	}
	if err := h.svc.IssueReceiptWithReceiver(c.Request.Context(), businessID, req.BillID, actorFromCtx(c), override); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "bill not found"})
			return
		}
		if errors.Is(err, fiscal.ErrInvalidReceiver) || errors.Is(err, fiscal.ErrCFIdentificationRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, fiscal.ErrReceiptAlreadyIssued) {
			server.RespondWithError(c, http.StatusConflict,
				server.ErrCodeFiscalReceiptAlreadyIssued,
				"This bill already has an invoice.")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not issue fiscal receipt")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "business_id": businessID})
}

// ResolveReceiptType handles GET /fiscal/resolve-type — single source of truth
// for the letter preview in IssueInvoiceDrawer (no TS rule duplication).
func (h *FiscalHandlers) ResolveReceiptType(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	settings, err := h.svc.GetSettings(c.Request.Context(), businessID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not load fiscal settings")
		return
	}
	emitterCond := ""
	country := ""
	if settings != nil {
		emitterCond = settings.TaxCondition
		country = settings.Country
	}
	docType := strings.TrimSpace(c.Query("doc_type"))
	docNumber := strings.TrimSpace(c.Query("doc_number"))
	taxCond := strings.TrimSpace(c.Query("tax_condition"))
	receiptType := fiscal.ResolveIssuableReceiptTypeForCountry(
		country, emitterCond, taxCond, docType, docNumber,
	)
	// Letter chip is Argentina-only (A/B/C). Non-AR types leave letter empty so
	// the FE can show "Invoice" / "Receipt" instead of "FACTURA B".
	letter := fiscal.ArgentinaLetterFromReceiptType(receiptType)
	c.JSON(http.StatusOK, gin.H{
		"receipt_type": receiptType,
		"letter":       letter,
		"country":      strings.ToUpper(strings.TrimSpace(country)),
	})
}

func (h *FiscalHandlers) RetryReceipt(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	receiptID, err := strconv.ParseUint(c.Param("receiptId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid receipt id"})
		return
	}
	if err := h.svc.RetryReceipt(c.Request.Context(), businessID, uint(receiptID), actorFromCtx(c)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "receipt not found"})
			return
		}
		if errors.Is(err, fiscal.ErrReceiptNotRetryable) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not retry fiscal receipt")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "business_id": businessID})
}

type creditNoteRequest struct {
	Reason      string `json:"reason"`
	AmountCents *int64 `json:"amount_cents"` // optional; omitted → full original total; present must be > 0
}

// CreditNote handles POST /businesses/:id/fiscal/receipts/:receiptId/credit. It
// enqueues an idempotent credit_note job against an authorized issue receipt so
// the worker can emit an AFIP nota de crédito. Reason is required for operator
// UX; amount_cents defaults to the full original total when omitted, and a
// present value must be in (0, receipt total] (400 otherwise).
func (h *FiscalHandlers) CreditNote(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	receiptID, err := strconv.ParseUint(c.Param("receiptId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid receipt id"})
		return
	}
	var req creditNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reason is required"})
		return
	}
	// Omitted amount_cents is a full credit. A present amount must be a
	// positive partial (the service also rejects one above the receipt total).
	var amountCents int64
	if req.AmountCents != nil {
		if *req.AmountCents <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "amount_cents must be greater than zero"})
			return
		}
		amountCents = *req.AmountCents
	}

	// Operator-dashboard credit: optional partial amount; empty discriminator →
	// one full NC key when amount is 0, or a distinct partial key when set.
	discriminator := ""
	if amountCents > 0 {
		discriminator = fmt.Sprintf("manual:%d", amountCents)
	}
	if err := h.svc.IssueCreditNote(c.Request.Context(), businessID, uint(receiptID), amountCents, discriminator, req.Reason, actorFromCtx(c)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "receipt not found"})
			return
		}
		if errors.Is(err, fiscal.ErrCreditNoteAmountInvalid) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, fiscal.ErrReceiptNotCreditable) || errors.Is(err, fiscal.ErrCreditNoteExceedsReceipt) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not issue credit note")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "business_id": businessID})
}

// ListReceiptDelivery handles GET /businesses/:id/fiscal/receipts/:receiptId/delivery.
// Returns per-channel delivery status (safe projection — error categories only,
// masked recipient). Scoped to businessID.
func (h *FiscalHandlers) ListReceiptDelivery(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	receiptID, err := strconv.ParseUint(c.Param("receiptId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid receipt id"})
		return
	}
	items, err := h.svc.ListReceiptDeliveryTasks(c.Request.Context(), businessID, uint(receiptID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "receipt not found"})
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not list receipt delivery")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// RetryDeliveryTask handles POST /businesses/:id/fiscal/delivery-tasks/:taskId/retry.
// Owner / fiscal:retry. Requeues only dead/failed channels; double-click safe.
func (h *FiscalHandlers) RetryDeliveryTask(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	taskID, err := strconv.ParseUint(c.Param("taskId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id"})
		return
	}
	if err := h.svc.RetryDeliveryTask(c.Request.Context(), businessID, uint(taskID), actorFromCtx(c)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "delivery task not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "business_id": businessID})
}

// ResendReceipt handles POST /businesses/:id/fiscal/receipts/:receiptId/resend.
// It re-sends/re-prints an authorized receipt to the customer, forcing delivery
// even if it was already delivered. A non-authorized receipt is a 409.
func (h *FiscalHandlers) ResendReceipt(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	receiptID, err := strconv.ParseUint(c.Param("receiptId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid receipt id"})
		return
	}
	if err := h.svc.ResendReceipt(c.Request.Context(), businessID, uint(receiptID), actorFromCtx(c)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "receipt not found"})
			return
		}
		if errors.Is(err, fiscal.ErrReceiptNotDeliverable) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not resend fiscal receipt")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "business_id": businessID})
}

// ValidateSettings runs a live AFIP validation (WSAA login + WSFE FEDummy) and
// persists the outcome on the settings row.
//
// Response contract: a *validation* failure (e.g. AFIP down, cert rejected) is
// returned as 200 with the updated settings, whose setup_status stays unchanged
// (i.e. not "ready") and last_validation_error is populated — so the frontend can
// render the failure inline. Only infrastructure failures (no settings row,
// factory not configured, DB write) return 4xx/5xx. Credential bytes are never
// echoed.
func (h *FiscalHandlers) ValidateSettings(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	settings, err := h.svc.ValidateSettings(c.Request.Context(), businessID)
	if err != nil {
		// settings != nil means the error is a provider validation failure; the
		// outcome is persisted on the row, so surface it as 200 for inline display.
		if settings != nil {
			c.JSON(http.StatusOK, gin.H{"settings": settings})
			return
		}
		if errors.Is(err, fiscal.ErrNoFiscalSettings) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not validate fiscal settings")
		return
	}
	c.JSON(http.StatusOK, gin.H{"settings": settings})
}

type uploadCredentialsJSONRequest struct {
	CertPEM string `json:"cert_pem"`
	KeyPEM  string `json:"key_pem"`
}

// UploadCredentials accepts fiscal credentials via:
//   - multipart form: files named "certificate" and "private_key" (each capped at 64 KB), OR
//   - JSON body: {"cert_pem": "...", "key_pem": "..."} (body capped at ~128 KB)
//
// On success returns 200 with fingerprint, expiry, and setup_status. The cert/key are
// never echoed back in the response.
func (h *FiscalHandlers) UploadCredentials(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}

	var certPEM, keyPEM string

	ct := c.ContentType()
	if ct == "multipart/form-data" || c.Request.MultipartForm != nil {
		var certTooLarge, keyTooLarge bool
		certPEM, certTooLarge = readMultipartField(c, "certificate")
		keyPEM, keyTooLarge = readMultipartField(c, "private_key")
		if certTooLarge || keyTooLarge {
			field := "certificate"
			if keyTooLarge {
				field = "private_key"
			}
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("%s exceeds 64KB limit", field)})
			return
		}
		if certPEM == "" || keyPEM == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "multipart fields 'certificate' and 'private_key' are required"})
			return
		}
	} else {
		// Cap the body before binding to ~128 KB (2× 64 KB PEM + JSON overhead).
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2*maxCredentialBytes+256)
		var req uploadCredentialsJSONRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			// http.MaxBytesReader sets a *http.MaxBytesError when the limit is hit.
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body exceeds 128KB limit"})
				return
			}
			server.RespondBindError(c, err)
			return
		}
		certPEM = req.CertPEM
		keyPEM = req.KeyPEM
	}

	if len(certPEM) == 0 || len(keyPEM) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cert_pem and key_pem are required"})
		return
	}
	if len(certPEM) > maxCredentialBytes || len(keyPEM) > maxCredentialBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "credential file exceeds 64 KB limit"})
		return
	}

	if err := h.svc.SetCredentials(c.Request.Context(), businessID, certPEM, keyPEM); err != nil {
		if errors.Is(err, fiscal.ErrNoFiscalSettings) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fiscal.ErrNoFiscalSettings.Error()})
			return
		}
		if errors.Is(err, fiscal.ErrInvalidCredentials) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "certificate and private key are not a valid matching pair"})
			return
		}
		log.Printf("fiscal UploadCredentials: failed to store credentials for business %d: %v", businessID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store credentials"})
		return
	}

	settings, err := h.svc.GetSettings(c.Request.Context(), businessID)
	if err != nil || settings == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "credentials stored but failed to reload settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"fingerprint":  settings.CredentialsFingerprint,
		"expires_at":   settings.CredentialsExpiresAt,
		"setup_status": settings.SetupStatus,
	})
}

// readMultipartField reads up to maxCredentialBytes from a named multipart file field.
// Returns (value, false) on success, ("", false) if field is absent, ("", true) if
// the file exceeds the size cap.
func readMultipartField(c *gin.Context, field string) (value string, tooLarge bool) {
	fh, err := c.FormFile(field)
	if err != nil {
		return "", false
	}
	if fh.Size > maxCredentialBytes {
		return "", true
	}
	f, err := fh.Open()
	if err != nil {
		return "", false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxCredentialBytes+1))
	if err != nil {
		return "", false
	}
	if len(data) > maxCredentialBytes {
		return "", true
	}
	return string(data), false
}
