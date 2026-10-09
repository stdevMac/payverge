package fiscal

import (
	"context"
	"encoding/json"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

type Mode = database.FiscalMode
type Status = database.FiscalStatus

const (
	ModeOff                  = database.FiscalModeOff
	ModeManual               = database.FiscalModeManual
	ModeAutomaticNonBlocking = database.FiscalModeAutomaticNonBlocking

	StatusPending         = database.FiscalStatusPending
	StatusAuthorized      = database.FiscalStatusAuthorized
	StatusRejected        = database.FiscalStatusRejected
	StatusFailedRetryable = database.FiscalStatusFailedRetryable
	StatusFailedPermanent = database.FiscalStatusFailedPermanent
	StatusCancelled       = database.FiscalStatusCancelled
	StatusCredited        = database.FiscalStatusCredited
)

const (
	ActionIssueReceipt = "issue_receipt"
	ActionCreditNote   = "credit_note"
	ActionStatusCheck  = "status_check"
)

// demoProviderName is the simulated fiscal provider used by demo businesses
// and sandbox environments (see internal/demo generator settings). Receipts
// issued under it must never reach real delivery transports.
const demoProviderName = "demo"

type Settings struct {
	ID                     uint
	BusinessID             uint
	Country                string
	Provider               string
	Mode                   Mode
	Environment            string
	TaxID                  string
	TaxCondition           string
	PointOfSale            *int
	CredentialsEncrypted   []byte `json:"-"`
	CredentialsFingerprint string
	CredentialsExpiresAt   *time.Time
	ProviderConfig         map[string]interface{}
	SetupStatus            string
}

type IssueLine struct {
	Description string
	Quantity    float64
	UnitCents   int64
	TotalCents  int64
	TaxRate     float64
}

type IssueInput struct {
	Business             database.Business
	Settings             Settings
	Bill                 database.Bill
	Payment              *database.Payment
	AlternativePayment   *database.AlternativePayment
	Lines                []IssueLine
	Currency             string
	ReceiptType          string
	TotalAmountCents     int64
	TipAmountCents       int64
	IssuedAt             time.Time
	IdempotencyKey       string
	CustomerDocType      string
	CustomerDocNumber    string
	CustomerTaxCondition string // RG 5616 receptor condición slug; empty → consumidor final
	CustomerName         string // razón social / name snapshotted onto the receipt
	CorporateAccountID   *uint
	CorporateAccountName string
	CorporateTaxID       string
	// AttemptedProviderReceiptID is the encoded "<type>-<pos>-<number>" tuple a
	// previous attempt of this job sent to the provider. Empty if none.
	AttemptedProviderReceiptID string
	// OnAttempt is called with the number the provider is about to request,
	// before that request is sent. A non-nil error aborts the request; the
	// provider must return a retryable error and must not contact the authority.
	OnAttempt func(providerReceiptID string) error
	// LockSeries, when set, serializes voucher-number allocation for one
	// provider series across workers/replicas. It returns a release func.
	LockSeries func(ctx context.Context, seriesKey string) (release func(), err error)
	// VoucherClaimed reports whether an authorized receipt stored for another job already holds providerReceiptID in this series. Nil means unknown (no check).
	VoucherClaimed func(providerReceiptID string) (bool, error)
}

type CreditNoteInput struct {
	OriginalReceipt database.FiscalReceipt
	Settings        Settings
	Reason          string
	IssuedAt        time.Time
	IdempotencyKey  string
	// AmountCents is the fiscal amount to credit (net of tip — tips never enter a
	// factura/nota de crédito). Zero means credit the full original receipt total
	// (the operator-dashboard "credit this receipt" action); a positive value
	// credits exactly that amount (a partial refund). Always int64 cents.
	AmountCents int64
	// Bill and BusinessTaxRate are the inputs the original factura's IVA split
	// was derived from (net, IVA, ImpOpEx at the business alícuota). The AR
	// mapper re-derives that split so a nota de crédito credits the same IVA
	// the factura declared, scaled to AmountCents for a partial credit.
	Bill            database.Bill
	BusinessTaxRate float64
	// AttemptedProviderReceiptID is the encoded "<type>-<pos>-<number>" tuple a
	// previous attempt of this job sent to the provider. Empty if none.
	AttemptedProviderReceiptID string
	// OnAttempt is called with the number the provider is about to request,
	// before that request is sent. A non-nil error aborts the request; the
	// provider must return a retryable error and must not contact the authority.
	OnAttempt func(providerReceiptID string) error
	// LockSeries, when set, serializes voucher-number allocation for one
	// provider series across workers/replicas. It returns a release func.
	LockSeries func(ctx context.Context, seriesKey string) (release func(), err error)
	// VoucherClaimed reports whether an authorized receipt stored for another job already holds providerReceiptID in this series. Nil means unknown (no check).
	VoucherClaimed func(providerReceiptID string) (bool, error)
}

type ProviderError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ReceiptResult struct {
	ProviderReceiptID string
	ReceiptType       string
	ReceiptNumber     string
	AuthCode          string
	AuthExpiresAt     *time.Time
	QRPayload         string
	PDFPath           string
	Status            Status
	RawRequest        json.RawMessage
	RawResponse       json.RawMessage
	ProviderErrors    []ProviderError
	// IssuedAt, when set, is the authority's issue date for a voucher adopted
	// on retry (for example AFIP CbteFch). The stored receipt must use it so
	// the row and its QR match the authorized voucher, not the retry's clock.
	IssuedAt *time.Time
}

type ReceiptStatus struct {
	Status         Status
	ProviderState  string
	ProviderErrors []ProviderError
	CheckedAt      time.Time

	// Authorization detail surfaced by a reconcile of an authorized voucher
	// (FECompConsultar in the AR provider). These let a status_check backfill a
	// receipt row whose original issue response was lost. They are populated only
	// when the voucher is authorized; otherwise they are zero. The CAE is not
	// secret, so it is safe to carry here.
	AuthCode      string     // CAE (Código de Autorización Electrónico)
	ReceiptNumber string     // authorized receipt number, as a decimal string
	AuthExpiresAt *time.Time // CAE expiry
	QRPayload     string     // rebuilt AFIP QR URL ("" when it cannot be built)
}

type BillPaidInput struct {
	BillID               uint
	PaymentID            *uint
	AlternativePaymentID *uint
	Source               string
	Actor                string
}

// ListReceiptsParams is the filter/pagination input for ListReceiptsPage.
type ListReceiptsParams struct {
	BusinessID     uint
	Status         string
	ReceiptType    string     // catalogue key; US invoice/receipt also match leftover AFIP letters
	Start, End     *time.Time // created_at half-open [start, end)
	NeedsAttention bool
	// Q optionally filters by receipt number, bill id/number, table label, or guest name.
	Q              string
	Page, PageSize int // default 20, max 100
}

// ReceiptDeliveryBadge is a compact delivery-task summary embedded on list rows
// so the FE does not N+1 GET /receipts/:id/delivery.
type ReceiptDeliveryBadge struct {
	TaskID  uint   `json:"task_id"`
	Channel string `json:"channel"`
	Status  string `json:"status"`
}

// ReceiptRow is one fiscal receipt list row with embedded delivery badges.
type ReceiptRow struct {
	database.FiscalReceipt
	Delivery       []ReceiptDeliveryBadge `json:"delivery"`
	NeedsAttention bool                   `json:"needs_attention"`
	// TableLabel is the dining table name for the underlying bill (dinner-service findability).
	TableLabel string `json:"table_label,omitempty"`
}

// ReceiptsPage is the paginated envelope for fiscal receipts.
type ReceiptsPage struct {
	Receipts   []ReceiptRow `json:"receipts"`
	Total      int64        `json:"total"`
	Page       int          `json:"page"`
	PageSize   int          `json:"page_size"`
	TotalPages int          `json:"total_pages"`
}

// IssuableBill is a recent paid bill for the invoice picker.
// When ExistingReceiptID is nil the bill can be issued; when set the operator
// should open that receipt instead of issuing a duplicate (demo/auto paths
// often already invoiced tonight's paid bills — empty "waiting" lists blocked dinner).
// TotalAmount is dollars (float64) to match the FE money wire contract.
type IssuableBill struct {
	BillID      uint       `json:"bill_id"`
	BillNumber  string     `json:"bill_number"`
	TableLabel  string     `json:"table_label,omitempty"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	TotalAmount float64    `json:"total_amount"`
	Currency    string     `json:"currency"`
	// Optional fiscal receptor defaults for the issue drawer (from bill).
	CustomerDocType      string `json:"customer_doc_type,omitempty"`
	CustomerDocNumber    string `json:"customer_doc_number,omitempty"`
	CustomerTaxCondition string `json:"customer_tax_condition,omitempty"`
	CustomerName         string `json:"customer_name,omitempty"`
	// ExistingReceiptID is set when a blocking issue_receipt already exists.
	ExistingReceiptID *uint `json:"existing_receipt_id,omitempty"`
}
