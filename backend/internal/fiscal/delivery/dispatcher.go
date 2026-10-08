// Package delivery wires the real email/S3/print transports behind the
// fiscal.ReceiptDispatcher seam. It is intentionally separate from the core
// fiscal package so the worker's unit tests stay free of live transport deps;
// main.go injects this into the fiscal Service + worker.
package delivery

import (
	"context"
	"errors"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
	"github.com/stdevmac/payverge/backend/internal/s3"
	printsvc "github.com/stdevmac/payverge/backend/internal/services/print"
)

// Dispatcher is the production fiscal.ReceiptDispatcher: it uploads receipt
// artifacts to the protected S3 bucket, emails the PDF to the customer via the
// shared email server, and best-effort enqueues a receipt print job.
type Dispatcher struct {
	print *printsvc.Service
}

// The production dispatcher always attributes receipt email to its bill.
var _ fiscal.BillScopedReceiptEmailer = (*Dispatcher)(nil)

// New builds a Dispatcher. print may be nil (delivery still emails + uploads;
// print enqueue becomes a no-op).
func New(print *printsvc.Service) *Dispatcher {
	return &Dispatcher{print: print}
}

// UploadProtected stores bytes in the protected S3 bucket.
func (d *Dispatcher) UploadProtected(data []byte, name, folder, contentType string) (string, error) {
	return s3.UploadBytesProtected(data, name, folder, contentType)
}

// SendReceiptEmail emails the rendered receipt PDF to the customer. The receipt
// type is rendered to its human-facing AFIP title before sending. A nil email
// server (email not configured) is a silent no-op so delivery stays best-effort.
func (d *Dispatcher) SendReceiptEmail(to []string, businessName, receiptType, receiptNumber, totalDisplay, language string, pdf []byte) error {
	srv := emails.EmailServerInstance
	if srv == nil {
		return nil
	}
	return srv.SendFiscalReceiptEmail(to, businessName, fiscal.ReceiptTitle(receiptType), receiptNumber, totalDisplay, language, pdf)
}

// SendReceiptEmailForBill is SendReceiptEmail attributed to the bill, so the
// send counts against the business's tenant outbound email budget and a later
// bounce is traced back to the bill (fiscal.BillScopedReceiptEmailer).
func (d *Dispatcher) SendReceiptEmailForBill(businessID, billID uint, to []string, businessName, receiptType, receiptNumber, totalDisplay, language string, pdf []byte) error {
	srv := emails.EmailServerInstance
	if srv == nil {
		return nil
	}
	err := srv.ForReceipt(businessID, billID).SendFiscalReceiptEmail(to, businessName, fiscal.ReceiptTitle(receiptType), receiptNumber, totalDisplay, language, pdf)
	return deferIfBudgetExceeded(err)
}

// deferIfBudgetExceeded turns a tenant email budget refusal into a deferred
// delivery: the budget is a rolling daily quota, so the worker must wait for
// it rather than burn its retry schedule and dead-letter the receipt.
func deferIfBudgetExceeded(err error) error {
	if errors.Is(err, emails.ErrTenantMailBudgetExceeded) {
		return fiscal.DeferDelivery(err, fiscal.DefaultDeliveryDeferral)
	}
	return err
}

// EnqueueReceiptPrint best-effort enqueues a receipt print job. When no print
// service is wired this is a no-op; when no printer is configured the print
// service leaves the job pending (no error).
func (d *Dispatcher) EnqueueReceiptPrint(ctx context.Context, businessID, billID uint, language string) error {
	if d.print == nil {
		return nil
	}
	_, err := d.print.Enqueue(ctx, printsvc.EnqueueParams{
		BusinessID: businessID,
		Kind:       database.PrintJobKindReceipt,
		SourceType: "bill",
		SourceID:   billID,
		Language:   language,
		CreatedBy:  "fiscal-delivery",
	})
	return err
}
