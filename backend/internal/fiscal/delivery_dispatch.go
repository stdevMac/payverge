package fiscal

import (
	"context"
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// ReceiptDispatcher is the best-effort delivery seam for an authorized fiscal
// receipt. It is split into the three independent side effects (S3 upload,
// email, print) so each can be stubbed in tests and wired to the real
// email/S3/print systems in main.go. Every method is best-effort: a failure is
// surfaced as an error for the caller to log, but never fails the fiscal job.
type ReceiptDispatcher interface {
	// UploadProtected stores bytes in the PROTECTED S3 bucket and returns the
	// stored object location (URL/key).
	UploadProtected(data []byte, name, folder, contentType string) (string, error)
	// SendReceiptEmail emails the rendered receipt PDF to the customer. pdf is the
	// attachment payload; language selects the es/es-AR/en template family.
	SendReceiptEmail(to []string, businessName, receiptType, receiptNumber, totalDisplay, language string, pdf []byte) error
	// EnqueueReceiptPrint best-effort enqueues a receipt print job. When no printer
	// is configured for the business the implementation leaves the job pending (no
	// error), mirroring the print service's posture.
	EnqueueReceiptPrint(ctx context.Context, businessID, billID uint, language string) error
}

// BillScopedReceiptEmailer is implemented by dispatchers that can attribute the
// receipt email to the bill it belongs to. The receipt goes to an address a
// guest typed on the bill, so the production dispatcher stamps the send with
// the bill's business: it is then counted against that tenant's outbound email
// budget. Once that budget is spent the production dispatcher returns a
// DeliveryDeferredError, so the worker waits for the budget instead of
// dead-lettering the receipt. Test stubs that only implement
// ReceiptDispatcher keep working through the unscoped method.
type BillScopedReceiptEmailer interface {
	SendReceiptEmailForBill(businessID, billID uint, to []string, businessName, receiptType, receiptNumber, totalDisplay, language string, pdf []byte) error
}

// sendBillReceiptEmail routes a receipt email through the bill-scoped sender
// when the dispatcher offers one.
func sendBillReceiptEmail(d ReceiptDispatcher, bill database.Bill, to []string, businessName, receiptType, receiptNumber, totalDisplay, language string, pdf []byte) error {
	if scoped, ok := d.(BillScopedReceiptEmailer); ok {
		return scoped.SendReceiptEmailForBill(bill.BusinessID, bill.ID, to, businessName, receiptType, receiptNumber, totalDisplay, language, pdf)
	}
	return d.SendReceiptEmail(to, businessName, receiptType, receiptNumber, totalDisplay, language, pdf)
}

// DeliverReceipt is the legacy single-shot delivery helper retained only for
// SweepUndeliveredReceipts while a pre-Wave-4 undelivered backlog drains (receipts
// with no fiscal_delivery_tasks rows). New authorizations enqueue durable
// per-channel tasks in SaveReceiptForJob; DeliveryWorker executes them with
// leases and retries. Prefer tasks + worker for all new delivery work.
//
// A nil dispatcher (delivery not wired) is a no-op.
func (s *Service) DeliverReceipt(ctx context.Context, receipt *database.FiscalReceipt, jobCtx *JobContext) {
	s.deliverReceipt(ctx, receipt, jobCtx, false)
}

func (s *Service) deliverReceipt(ctx context.Context, receipt *database.FiscalReceipt, jobCtx *JobContext, force bool) {
	if s.dispatcher == nil || receipt == nil || jobCtx == nil {
		return
	}
	// Idempotency guard: already delivered → never re-send (unless forced by an
	// explicit operator re-send).
	if !force && receipt.DeliveredAt != nil {
		return
	}

	// 1) Render the PDF + QR PNG. A render failure means there is nothing to
	// deliver; bail (DeliveredAt stays nil — the receipt is left undelivered, no
	// auto-retry today; an operator can re-send from the dashboard).
	pdf, err := RenderReceiptPDF(*receipt, jobCtx.Business, jobCtx.Settings)
	if err != nil {
		logger.Logger.Warnf("Fiscal delivery: render PDF for receipt %d failed: %v", receipt.ID, err)
		return
	}

	// 2) Upload the PDF (+ QR PNG) to the PROTECTED bucket. Best-effort: an upload
	// failure does not block the email/print, and PDFPath/QRImagePath are an
	// idempotent overwrite that does NOT gate redelivery.
	s.uploadReceiptArtifacts(s.dispatcher, receipt, pdf)

	// 3) Email the PDF to the customer when a recipient is present. DeliveredAt is
	// stamped only on a successful send (or when there is no recipient at all); a
	// transient email failure leaves the receipt undelivered (no auto-retry today).
	recipient := s.customerEmail(ctx, jobCtx.Bill)
	if recipient != "" {
		if err := sendBillReceiptEmail(
			s.dispatcher,
			jobCtx.Bill,
			[]string{recipient},
			emitterName(jobCtx.Business),
			receipt.ReceiptType,
			deref(receipt.ReceiptNumber),
			formatARS(receipt.TotalAmountCents),
			receiptEmailLanguage(jobCtx.Settings),
			pdf,
		); err != nil {
			logger.Logger.Warnf("Fiscal delivery: email receipt %d failed (left undelivered): %v", receipt.ID, err)
			return // leave DeliveredAt nil; no auto-retry today — operator can re-send.
		}
	}

	// 4) Best-effort print enqueue. A printer may not be configured; the print
	// service leaves the job pending in that case. Never gates DeliveredAt.
	if err := s.dispatcher.EnqueueReceiptPrint(ctx, jobCtx.Bill.BusinessID, jobCtx.Bill.ID, receiptEmailLanguage(jobCtx.Settings)); err != nil {
		logger.Logger.Warnf("Fiscal delivery: enqueue print for receipt %d failed: %v", receipt.ID, err)
	}

	// 5) Stamp DeliveredAt so the receipt is never re-delivered. A stamp-write
	// failure here is harmless (the email already went out); the DeliveredAt guard
	// only matters if delivery is ever re-driven (manual re-send / future sweep).
	now := s.now()
	if err := s.repo.MarkReceiptDelivered(receipt.ID, now); err != nil {
		logger.Logger.Warnf("Fiscal delivery: mark receipt %d delivered failed: %v", receipt.ID, err)
		return
	}
	receipt.DeliveredAt = &now
}

// uploadReceiptArtifacts uploads the PDF and QR PNG to the protected bucket and
// records PDFPath/QRImagePath on the receipt row. The dispatcher is passed
// explicitly (rather than read from s.dispatcher) so the delivery worker can
// supply its own dispatcher without mutating the shared Service field — that
// mutation was a latent data race once the worker fans out concurrently. All
// failures are logged and swallowed: artifact storage is independent of
// email/print delivery.
func (s *Service) uploadReceiptArtifacts(dispatcher ReceiptDispatcher, receipt *database.FiscalReceipt, pdf []byte) {
	folder := fmt.Sprintf("fiscal-receipts/%d", receipt.BusinessID)
	updates := map[string]interface{}{}

	pdfName := fmt.Sprintf("receipt-%d.pdf", receipt.ID)
	if loc, err := dispatcher.UploadProtected(pdf, pdfName, folder, "application/pdf"); err != nil {
		logger.Logger.Warnf("Fiscal delivery: upload PDF for receipt %d failed: %v", receipt.ID, err)
	} else if loc != "" {
		updates["pdf_path"] = loc
		receipt.PDFPath = strPtr(loc)
	}

	if qr := deref(receipt.QRPayload); strings.TrimSpace(qr) != "" {
		if png, err := RenderAFIPQRPNG(qr); err != nil {
			logger.Logger.Warnf("Fiscal delivery: render QR for receipt %d failed: %v", receipt.ID, err)
		} else {
			qrName := fmt.Sprintf("receipt-%d-qr.png", receipt.ID)
			if loc, err := dispatcher.UploadProtected(png, qrName, folder, "image/png"); err != nil {
				logger.Logger.Warnf("Fiscal delivery: upload QR for receipt %d failed: %v", receipt.ID, err)
			} else if loc != "" {
				updates["qr_image_path"] = loc
				receipt.QRImagePath = strPtr(loc)
			}
		}
	}

	if len(updates) > 0 {
		if err := s.repo.UpdateReceiptArtifactPaths(receipt.ID, updates); err != nil {
			logger.Logger.Warnf("Fiscal delivery: persist artifact paths for receipt %d failed: %v", receipt.ID, err)
		}
	}
}

// customerEmail resolves the recipient email for a bill. It tries the linked
// CRM customer first, then falls back to the bill's delivery order (delivery
// checkout creates the bill WITHOUT a CRMCustomerID and stores the guest email
// on DeliveryOrder.CustomerEmail — DELIV-FISC-1). Both lookups use narrow
// single-column projections so the delivery path never hauls wide rows into
// the worker, and both are best-effort: a lookup failure logs and yields ""
// (no recipient) rather than failing issuance. Returns "" when there is no
// email anywhere (Consumidor Final).
func (s *Service) customerEmail(ctx context.Context, bill database.Bill) string {
	// Guest-entered factura email wins: it is the address explicitly captured
	// for fiscal delivery at checkout.
	if bill.FiscalCustomerEmail != nil {
		if email := strings.TrimSpace(*bill.FiscalCustomerEmail); email != "" {
			return email
		}
	}
	if bill.CRMCustomerID != nil && *bill.CRMCustomerID != 0 {
		var email string
		if err := s.db.WithContext(ctx).
			Model(&database.Customer{}).
			Select("email").
			Where("id = ?", *bill.CRMCustomerID).
			Limit(1).
			Scan(&email).Error; err != nil {
			logger.Logger.Warnf("Fiscal delivery: load customer email for bill %d failed: %v", bill.ID, err)
		} else if email = strings.TrimSpace(email); email != "" {
			return email
		}
	}
	return s.deliveryOrderEmail(ctx, bill)
}

// deliveryOrderEmail resolves the guest email from the delivery order linked to
// the bill, when one exists. Narrow projection, best-effort: any error logs and
// returns "" so email resolution never fails fiscal issuance.
func (s *Service) deliveryOrderEmail(ctx context.Context, bill database.Bill) string {
	var email string
	if err := s.db.WithContext(ctx).
		Model(&database.DeliveryOrder{}).
		Select("customer_email").
		Where("bill_id = ?", bill.ID).
		Order("id DESC").
		Limit(1).
		Scan(&email).Error; err != nil {
		logger.Logger.Warnf("Fiscal delivery: load delivery-order email for bill %d failed: %v", bill.ID, err)
		return ""
	}
	return strings.TrimSpace(email)
}

// receiptEmailLanguage picks the email locale for a receipt. AR fiscal receipts
// are Argentine, so they go out in Rioplatense Spanish (es_ar); any other
// Spanish-issuing country falls back to es; everything else to en.
func receiptEmailLanguage(settings database.BusinessFiscalSettings) string {
	switch strings.ToUpper(strings.TrimSpace(settings.Country)) {
	case "AR":
		return "es_ar"
	case "ES", "MX", "CL", "UY", "CO", "PE":
		return "es"
	default:
		return "en"
	}
}
