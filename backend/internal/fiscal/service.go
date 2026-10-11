package fiscal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// rowLockingSupported reports whether the dialect supports SELECT ... FOR UPDATE.
// PostgreSQL does; SQLite (used in tests) errors if the clause is rendered, so the
// over-credit guard takes the row lock only on Postgres. Mirrors the claim path's
// fiscalJobsClaimUsesSkipLocked gate in the database package.
func rowLockingSupported(dialect string) bool {
	return dialect == "postgres"
}

var ErrInvalidSettings = errors.New("invalid fiscal settings")
var ErrReceiptNotRetryable = errors.New("receipt is not retryable")

// ErrReceiptNotCreditable is returned by IssueCreditNote when the target receipt
// is not an authorized issue receipt (e.g. it is still pending/failed, or is
// itself a credit note) and therefore cannot be credited.
var ErrReceiptNotCreditable = errors.New("receipt is not creditable")

// ErrReceiptNotDeliverable is returned by ResendReceipt when the target receipt
// is not authorized (only an authorized receipt has a deliverable PDF/QR).
var ErrReceiptNotDeliverable = errors.New("receipt is not deliverable")

// ErrNoFiscalSettings is returned by SetCredentials when no fiscal settings row
// exists yet for the business. The caller must configure fiscal settings before
// uploading credentials.
var ErrNoFiscalSettings = errors.New("fiscal settings must be configured before uploading credentials")

// ErrInvalidCredentials is returned by SetCredentials when the uploaded
// certificate and private key are not a valid matching pair. Callers must map
// it to a fixed client message; the wrapped detail is for server logs only.
var ErrInvalidCredentials = errors.New("invalid credential bundle")

// ErrPermanent marks a provider error as non-retryable. A Provider wrapping this
// sentinel (errors.Join / fmt.Errorf("...: %w", ErrPermanent)) signals the worker
// to fail the job permanently instead of scheduling a backoff retry. By default
// (unwrapped errors) provider failures are treated as transient/retryable.
var ErrPermanent = errors.New("fiscal: permanent provider failure")

// ErrCreditNoteExceedsReceipt is returned by IssueCreditNote when the requested
// credit amount, plus all already-enqueued/authorized credit notes for the same
// receipt, would exceed the original receipt total — an illegal over-credit.
var ErrCreditNoteExceedsReceipt = errors.New("credit note would exceed original receipt total")

// ErrCreditNoteAmountInvalid is returned by IssueCreditNote when the requested
// amount alone is negative or larger than the receipt total — a malformed
// request, distinct from a running-total conflict (ErrCreditNoteExceedsReceipt).
var ErrCreditNoteAmountInvalid = errors.New("credit amount must be positive and at most the receipt total")

type Service struct {
	db        *gorm.DB
	repo      *Repository
	providers *ProviderRegistry
	// factory builds a credential-aware provider per business settings. When set
	// it takes precedence over the static providers registry for job processing.
	factory ProviderFactory
	// dispatcher delivers an authorized receipt to the customer (S3 upload +
	// email + print). Nil means delivery is disabled (e.g. unit tests, or a deploy
	// that has not wired email/S3/print) — the worker simply skips delivery and
	// the fiscal job still completes.
	dispatcher ReceiptDispatcher
	// nowFn is an injectable clock used when stamping receipts/inputs; nil means
	// time.Now().
	nowFn func() time.Time
	// saveReceiptFn is an injectable seam over repo.SaveReceiptForJob so the
	// bounded persist-retry path (T4) can be unit-tested deterministically. Nil
	// means the real repository call. originalReceiptID is the issue receipt a
	// credit note credits (nil for issue jobs).
	saveReceiptFn func(receipt *database.FiscalReceipt, jobID uint, originalReceiptID *uint) (uint, error)
}

// maxReceiptSaveAttempts bounds the inline retry when persisting an authorized
// receipt after a successful AFIP authorization. When the result carries a
// provider receipt id, OnAttempt already persisted that number, so exhausting
// the inline saves is retryable: the next run reconciles the same voucher.
// When no number was recorded, re-issuing would request a new receipt number
// and a duplicate invoice, so the job goes terminal instead of being rescheduled.
const maxReceiptSaveAttempts = 3

// saveReceiptForJob persists a receipt for a job, routing through the injectable
// seam when set (tests) or the real repository otherwise.
func (s *Service) saveReceiptForJob(receipt *database.FiscalReceipt, jobID uint, originalReceiptID *uint) (uint, error) {
	if s.saveReceiptFn != nil {
		return s.saveReceiptFn(receipt, jobID, originalReceiptID)
	}
	return s.repo.SaveReceiptForJob(receipt, jobID, originalReceiptID)
}

// saveAuthorizedReceiptWithRetry persists an authorized receipt with a bounded
// inline retry. The receipt PK is reset before each attempt so a rolled-back
// Create does not leave a stale id that GORM would treat as an update.
// originalReceiptID is the issue receipt a credit note credits (nil for issues).
func (s *Service) saveAuthorizedReceiptWithRetry(receipt *database.FiscalReceipt, jobID uint, originalReceiptID *uint) (uint, error) {
	var lastErr error
	for attempt := 1; attempt <= maxReceiptSaveAttempts; attempt++ {
		receipt.ID = 0
		id, err := s.saveReceiptForJob(receipt, jobID, originalReceiptID)
		if err == nil {
			return id, nil
		}
		lastErr = err
	}
	return 0, lastErr
}

func NewService(db *gorm.DB, providers *ProviderRegistry) *Service {
	if providers == nil {
		providers = NewProviderRegistry()
	}
	return &Service{
		db:        db,
		repo:      NewRepository(db),
		providers: providers,
	}
}

// WithProviderFactory configures the credential-aware provider factory the
// service uses when processing claimed jobs. Returns the service for chaining.
func (s *Service) WithProviderFactory(factory ProviderFactory) *Service {
	s.factory = factory
	return s
}

// WithClock overrides the clock used to stamp receipts/inputs (for tests).
func (s *Service) WithClock(now func() time.Time) *Service {
	s.nowFn = now
	return s
}

// WithDelivery configures the receipt dispatcher used to deliver an authorized
// receipt to the customer (S3 upload + email + best-effort print). Returns the
// service for chaining. A nil dispatcher disables delivery entirely.
func (s *Service) WithDelivery(dispatcher ReceiptDispatcher) *Service {
	s.dispatcher = dispatcher
	return s
}

func (s *Service) UpdateSettings(ctx context.Context, settings *database.BusinessFiscalSettings) error {
	if err := normalizeAndValidateSettings(settings); err != nil {
		return err
	}
	return s.repo.UpsertSettings(settings)
}

func (s *Service) GetSettings(ctx context.Context, businessID uint) (*database.BusinessFiscalSettings, error) {
	return s.repo.GetSettings(businessID)
}

func (s *Service) ListReceipts(ctx context.Context, businessID uint, status string) ([]database.FiscalReceipt, error) {
	return s.repo.ListReceipts(businessID, status, 50)
}

// ListReceiptsPage returns a paginated receipts list with embedded delivery badges.
// Legacy ListReceipts is unchanged for callers that omit the page query param.
func (s *Service) ListReceiptsPage(ctx context.Context, params ListReceiptsParams) (*ReceiptsPage, error) {
	return s.repo.ListReceiptsPage(params)
}

// ListIssuableBills returns up to 20 recent paid bills for the invoice picker.
// Optional q filters by bill number, table label, or guest name. Already-invoiced
// bills include ExistingReceiptID so operators can open them mid-service.
func (s *Service) ListIssuableBills(ctx context.Context, businessID uint, q string) ([]IssuableBill, error) {
	return s.repo.ListIssuableBills(businessID, q, 20)
}

func (s *Service) HandleBillPaid(ctx context.Context, input BillPaidInput) error {
	enabled, controlErr := automaticFiscalEnabled(ctx, s.db)
	if controlErr != nil {
		logger.Logger.Warnf("Fiscal runtime control unavailable; paid-bill enqueue skipped: %v", controlErr)
		return nil
	}
	if !enabled {
		return nil
	}
	var bill database.Bill
	if err := s.db.WithContext(ctx).Where("id = ?", input.BillID).First(&bill).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if bill.Status != database.BillStatusPaid || bill.PaidAmount <= 0 {
		return nil
	}
	// Demo/test businesses never feed the production fiscal queue (Task 15).
	// Generating AFIP jobs for fake restaurants is a data-generation bug that
	// surfaced as hundreds of queued jobs on the admin fiscal page.
	if !isProductionFiscalBusiness(s.db.WithContext(ctx), bill.BusinessID) {
		return nil
	}
	settings, err := s.repo.GetActiveSettings(bill.BusinessID)
	if err != nil || settings == nil {
		return err
	}
	if settings.Mode == database.FiscalModeOff || settings.Mode == database.FiscalModeManual {
		return nil
	}
	return s.enqueueIssueJob(&bill, settings, input.PaymentID, input.AlternativePaymentID, input.Actor, false)
}

// isProductionFiscalBusiness reports whether automatic fiscal enqueue is allowed
// for this business. Missing rows (unit tests that seed only bills) still allow
// enqueue so existing coverage keeps working; any loaded demo/test row is denied.
func isProductionFiscalBusiness(db *gorm.DB, businessID uint) bool {
	if db == nil || businessID == 0 {
		return true
	}
	var biz database.Business
	err := db.Select("id", "kind", "is_demo").Where("id = ?", businessID).First(&biz).Error
	if err != nil {
		// No businesses row — preserve prior test behaviour (enqueue continues).
		return true
	}
	if biz.IsDemo {
		return false
	}
	switch biz.Kind {
	case database.BusinessKindDemo, database.BusinessKindTest:
		return false
	default:
		// real, empty (legacy default), or unknown → allow.
		return true
	}
}

// ReceiverOverride is an optional operator-supplied receptor identity applied
// at manual issue time. Non-empty fields win over Bill.FiscalCustomer* and are
// persisted on the bill so the async worker snapshots them onto the receipt.
type ReceiverOverride struct {
	CustomerDocType      string
	CustomerDocNumber    string
	CustomerTaxCondition string
	CustomerName         string
}

// ErrInvalidReceiver is returned when an issue override fails validation
// (CUIT checksum, DNI length, unknown condición).
var ErrInvalidReceiver = errors.New("invalid fiscal receiver")

// ErrCFIdentificationRequired is returned when Factura B exceeds the AFIP CF
// identification threshold without DNI/CUIT.
var ErrCFIdentificationRequired = errors.New("consumidor final identification required")

// ErrReceiptAlreadyIssued is returned by the manual issue path when the bill
// already carries a live issue receipt (pending / authorized / credited). The
// per-bill idempotency key means a second issue can never cut a second factura,
// but it used to make the request a silent no-op that the handler reported as
// "queued" — so the operator was told work happened that never did (#907).
var ErrReceiptAlreadyIssued = errors.New("bill already has a fiscal receipt")

func (s *Service) IssueReceipt(ctx context.Context, businessID, billID uint, actor string) error {
	return s.IssueReceiptWithReceiver(ctx, businessID, billID, actor, nil)
}

// IssueReceiptWithReceiver enqueues a manual issue job, optionally applying
// receiver overrides to the bill first (validated).
func (s *Service) IssueReceiptWithReceiver(ctx context.Context, businessID, billID uint, actor string, override *ReceiverOverride) error {
	var bill database.Bill
	if err := s.db.WithContext(ctx).Where("id = ? AND business_id = ?", billID, businessID).First(&bill).Error; err != nil {
		return err
	}
	if bill.Status != database.BillStatusPaid || bill.PaidAmount <= 0 {
		return nil
	}
	settings, err := s.repo.GetActiveSettings(bill.BusinessID)
	if err != nil || settings == nil {
		return err
	}
	if settings.Mode == database.FiscalModeOff {
		return nil
	}
	// The picker keeps already-invoiced bills visible and marks them with
	// existing_receipt_id (#260), and the drawer swaps Issue for "View invoice"
	// on those rows — but the manual bill-number field bypasses the picker
	// entirely. Refuse the re-issue here, on exactly the statuses the picker
	// blocks on, before any receptor override is written to the bill.
	blockingID, err := s.repo.FindBlockingIssueReceiptID(businessID, bill.ID)
	if err != nil {
		return err
	}
	if blockingID != 0 {
		return ErrReceiptAlreadyIssued
	}
	if override != nil {
		if err := applyReceiverOverride(s.db.WithContext(ctx), &bill, override); err != nil {
			return err
		}
	}
	if err := validateSynchronousIssue(settings, &bill); err != nil {
		return err
	}
	return s.enqueueIssueJob(&bill, settings, nil, nil, actor, true)
}

// validateSynchronousIssue mirrors the WSFE mapper's Factura B
// unidentified-consumidor-final threshold guard at manual issue time, so the
// operator gets an immediate 400 instead of a success toast followed by a
// silent failed_permanent job. Only ARCA issuance enforces it — the demo
// provider never reaches the WSFE mapper that rejects these.
func validateSynchronousIssue(settings *database.BusinessFiscalSettings, bill *database.Bill) error {
	if settings == nil || !strings.EqualFold(strings.TrimSpace(settings.Provider), "arca") {
		return nil
	}
	letter := ResolveIssuableReceiptType(
		settings.TaxCondition,
		strValue(bill.FiscalCustomerTaxCondition),
		strValue(bill.FiscalCustomerDocType),
		strValue(bill.FiscalCustomerDocNumber),
	)
	if letter != "factura_b" {
		return nil
	}
	docType, docNumber := customerDocumentFromBill(*bill)
	if docType != "" && strings.TrimSpace(docNumber) != "" {
		return nil // identified receptor — threshold does not apply
	}
	total := bill.TotalAmount
	if total <= 0 {
		total = bill.PaidAmount
	}
	if total >= CFIDThresholdCents() {
		return fmt.Errorf("%w: factura_b total %d cents meets the identification threshold (%d cents)",
			ErrCFIdentificationRequired, total, CFIDThresholdCents())
	}
	return nil
}

// SetCredentials validates certPEM/keyPEM as a matched RSA pair, encrypts them,
// and persists the bundle's fingerprint and expiry on the business's fiscal settings.
// Returns ErrNoFiscalSettings if no fiscal settings row exists yet (call UpdateSettings first).
// Returns ErrInvalidCredentials (with no persistence) if the cert/key are invalid or mismatched.
// Never logs keyPEM.
func (s *Service) SetCredentials(ctx context.Context, businessID uint, certPEM, keyPEM string) error {
	bundle, err := validateAndParseCertBundle([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCredentials, err)
	}
	enc, err := EncryptCredentialBundle(CredentialBundle{CertPEM: certPEM, KeyPEM: keyPEM})
	if err != nil {
		return fmt.Errorf("encrypt credentials: %w", err)
	}
	settings, err := s.repo.GetSettings(businessID)
	if err != nil {
		return err
	}
	if settings == nil {
		return ErrNoFiscalSettings
	}
	settings.CredentialsEncrypted = enc
	settings.CredentialsFingerprint = bundle.Fingerprint
	settings.CredentialsExpiresAt = &bundle.NotAfter
	settings.SetupStatus = "credentials_set"
	return s.repo.UpsertSettings(settings)
}

// ValidateSettings runs a live provider validation (for AR: WSAA login + WSFE
// FEDummy) against the business's persisted fiscal settings. On success it flips
// SetupStatus to "ready", stamps LastValidatedAt, and clears LastValidationError.
// On a validation failure it leaves SetupStatus untouched, records the failure in
// LastValidationError, and returns the (reloaded) settings together with the
// validation error so the caller can surface the failure inline.
//
// Contract: the *settings return is non-nil whenever a settings row exists, even
// when the validation error is non-nil. Infra failures (no settings, factory not
// configured, provider build, DB write) return (nil, err) with err != a provider
// validation error. Error messages never include credential/key material.
func (s *Service) ValidateSettings(ctx context.Context, businessID uint) (*database.BusinessFiscalSettings, error) {
	settings, err := s.repo.GetSettings(businessID)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return nil, ErrNoFiscalSettings
	}
	if s.factory == nil {
		return nil, errors.New("fiscal provider factory not configured")
	}

	provider, err := s.factory.Build(ctx, settings)
	if err != nil {
		return nil, fmt.Errorf("build fiscal provider: %w", err)
	}

	validationErr := provider.ValidateSettings(ctx, settingsFromModel(*settings))
	if validationErr != nil {
		msg := validationErr.Error()
		settings.LastValidationError = &msg
		// Leave SetupStatus and LastValidatedAt unchanged on failure.
		if saveErr := s.repo.UpsertSettings(settings); saveErr != nil {
			return nil, saveErr
		}
		return settings, validationErr
	}

	now := s.now()
	settings.SetupStatus = "ready"
	settings.LastValidatedAt = &now
	settings.LastValidationError = nil
	if saveErr := s.repo.UpsertSettings(settings); saveErr != nil {
		return nil, saveErr
	}
	return settings, nil
}

func (s *Service) RetryReceipt(ctx context.Context, businessID, receiptID uint, actor string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var receipt database.FiscalReceipt
		if err := tx.Where("id = ? AND business_id = ?", receiptID, businessID).
			First(&receipt).Error; err != nil {
			return err
		}
		if !isRetryableReceiptStatus(receipt.Status) {
			return ErrReceiptNotRetryable
		}
		key := fmt.Sprintf("business:%d:receipt:%d:action:retry", businessID, receiptID)
		var job database.FiscalJob
		err := tx.Where("idempotency_key = ?", key).First(&job).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			job = database.FiscalJob{
				BusinessID:     businessID,
				SettingsID:     receipt.SettingsID,
				ReceiptID:      &receipt.ID,
				BillID:         receipt.BillID,
				Action:         ActionStatusCheck,
				IdempotencyKey: key,
				Status:         database.FiscalStatusPending,
				MaxAttempts:    5,
				CreatedBy:      defaultActor(actor),
			}
			return tx.Create(&job).Error
		}
		if err != nil {
			return err
		}
		if isActiveRetryJob(job) {
			return nil
		}
		updates := map[string]interface{}{
			"status":             database.FiscalStatusPending,
			"attempts":           0,
			"max_attempts":       retryMaxAttempts(job.MaxAttempts),
			"next_attempt_at":    nil,
			"last_error_code":    nil,
			"last_error_message": nil,
			"locked_at":          nil,
			"locked_by":          nil,
			"created_by":         defaultActor(actor),
		}
		return tx.Model(&job).Updates(updates).Error
	})
}

// IssueCreditNote enqueues an idempotent credit_note job against an already
// authorized issue receipt so the worker (processCreditNoteJob) can emit an AFIP
// nota de crédito for it. It mirrors RetryReceipt's transactional
// find-by-idempotency-key → create-if-absent → no-op-if-active structure.
//
// The receipt must belong to businessID and be an authorized issue receipt; a
// receipt that is still pending/failed, or is itself a credit note, returns
// ErrReceiptNotCreditable and enqueues nothing.
//
// reason is accepted for the API/refund contract but is NOT persisted: FiscalJob
// has no reason column (and the worker does not set CreditNoteInput.Reason), so
// there is no sensible place to store it without inventing schema.
//
// amountCents is the fiscal amount to credit (net of tip): 0 credits the full
// original receipt total (the operator-dashboard action); a positive value
// credits exactly that amount (a partial refund). discriminator scopes the
// idempotency key so multiple partial refunds against one receipt each get their
// own credit note — an empty discriminator keeps the legacy single-credit-note
// key (one nota de crédito per receipt) for the manual operator action.
func (s *Service) IssueCreditNote(ctx context.Context, businessID, receiptID uint, amountCents int64, discriminator, reason, actor string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var receipt database.FiscalReceipt
		// F-CREDITRACE: lock the original receipt row for the duration of the txn so
		// two concurrent partial credits cannot both read the same running total and
		// both pass the over-credit guard (which would create an illegal over-credit /
		// negative-VAT position AFIP does not cross-check). Postgres-only; on SQLite
		// the sequential guard below still holds.
		query := tx.Where("id = ? AND business_id = ?", receiptID, businessID)
		if rowLockingSupported(tx.Dialector.Name()) {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(&receipt).Error; err != nil {
			return err
		}
		if !isCreditableReceipt(receipt) {
			return ErrReceiptNotCreditable
		}
		if amountCents < 0 {
			return ErrCreditNoteAmountInvalid
		}
		if amountCents > receipt.TotalAmountCents {
			// Still an over-credit for the refund paths that branch on
			// ErrCreditNoteExceedsReceipt; the dashboard maps it to 400.
			return fmt.Errorf("%w: %w", ErrCreditNoteAmountInvalid, ErrCreditNoteExceedsReceipt)
		}
		// Build the idempotency key first so the over-credit guard can exclude
		// this key from the running sum — a same-discriminator retry must not
		// double-count itself.
		key := fmt.Sprintf("business:%d:receipt:%d:action:credit_note", businessID, receiptID)
		if discriminator != "" {
			key = fmt.Sprintf("%s:%s", key, discriminator)
		}
		// Over-credit guard: sum CreditAmountCents (nil == full receipt total) of
		// every still-live credit-note job for this receipt — including a
		// deferred job that is not bound to a receipt yet — and reject when
		// adding the requested amount would exceed the original total. AFIP does
		// not cross-check CbtesAsoc sums, so two independently-authorized NCs
		// whose sum > the factura would create a legal over-credit / negative-VAT
		// position. Exclude the row whose idempotency_key equals ours so a retry
		// of the same discriminator (same key) does not count itself twice.
		credited, err := s.repo.SumLiveCreditCents(tx, receipt, 0, key)
		if err != nil {
			return err
		}
		requested := amountCents
		if requested <= 0 {
			requested = receipt.TotalAmountCents // 0 == full credit
		}
		if credited+requested > receipt.TotalAmountCents {
			return ErrCreditNoteExceedsReceipt
		}
		var creditAmount *int64
		if amountCents > 0 {
			amt := amountCents
			creditAmount = &amt
		}
		var job database.FiscalJob
		err = tx.Where("idempotency_key = ?", key).First(&job).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			job = database.FiscalJob{
				BusinessID:        businessID,
				SettingsID:        receipt.SettingsID,
				ReceiptID:         &receipt.ID,
				BillID:            receipt.BillID,
				Action:            ActionCreditNote,
				IdempotencyKey:    key,
				CreditAmountCents: creditAmount,
				Status:            database.FiscalStatusPending,
				MaxAttempts:       5,
				CreatedBy:         defaultActor(actor),
			}
			return tx.Create(&job).Error
		}
		if err != nil {
			return err
		}
		// A credit-note job for this (receipt, discriminator) already exists (the
		// unique idempotency_key guarantees at most one). No-op so repeated calls —
		// e.g. a retried refund of the SAME payment — never enqueue a second credit
		// note. Re-driving a terminal credit-note job is a deliberate operator
		// action via RetryReceipt, not an implicit side effect here.
		_ = job
		return nil
	})
}

// deferredCreditNoteMaxAttempts is the retry budget for a credit-note job
// created BEFORE its issue receipt authorized (refund raced issuance). With
// the worker's capped exponential backoff this covers hours of waiting for a
// slow/retrying AFIP issuance without going failed_permanent prematurely.
const deferredCreditNoteMaxAttempts = 48

// IssueCreditNoteDeferred enqueues a credit-note job for a bill whose issue
// receipt has NOT authorized yet (refund raced issuance). The job carries no
// receipt reference; processCreditNoteJob resolves the authorized issue
// receipt by bill_id on each attempt and stays retryable
// ("awaiting_issue_receipt") until it appears. Idempotent per (bill,
// discriminator) via the unique idempotency key — a retried refund of the
// same payment never enqueues a second deferred note.
func (s *Service) IssueCreditNoteDeferred(ctx context.Context, businessID, billID uint, amountCents int64, discriminator, actor string) error {
	if amountCents <= 0 {
		return fmt.Errorf("deferred credit note requires a positive amount, got %d", amountCents)
	}
	settings, err := s.repo.GetActiveSettings(businessID)
	if err != nil {
		return err
	}
	if settings == nil {
		return nil // fiscal off — nothing to credit
	}
	key := fmt.Sprintf("business:%d:bill:%d:action:credit_note_deferred", businessID, billID)
	if discriminator != "" {
		key = fmt.Sprintf("%s:%s", key, discriminator)
	}
	amt := amountCents
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing database.FiscalJob
		err := tx.Where("idempotency_key = ?", key).First(&existing).Error
		if err == nil {
			return nil // already enqueued (retried refund) — no-op
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		job := database.FiscalJob{
			BusinessID:        businessID,
			SettingsID:        settings.ID,
			BillID:            billID,
			Action:            ActionCreditNote,
			IdempotencyKey:    key,
			CreditAmountCents: &amt,
			Status:            database.FiscalStatusPending,
			MaxAttempts:       deferredCreditNoteMaxAttempts,
			CreatedBy:         defaultActor(actor),
		}
		return tx.Create(&job).Error
	})
}

// ResendReceipt re-drives delivery of an already-authorized receipt by creating
// a new uniquely-keyed delivery attempt (requeue dead/succeeded channels and
// ensure missing tasks exist). It is the operator "re-send / re-print" action —
// NOT an inline side effect. A non-authorized receipt returns
// ErrReceiptNotDeliverable; a missing receipt returns gorm.ErrRecordNotFound.
func (s *Service) ResendReceipt(ctx context.Context, businessID, receiptID uint, actor string) error {
	var receipt database.FiscalReceipt
	if err := s.db.WithContext(ctx).
		Where("id = ? AND business_id = ?", receiptID, businessID).
		First(&receipt).Error; err != nil {
		return err
	}
	if receipt.Status != database.FiscalStatusAuthorized {
		return ErrReceiptNotDeliverable
	}

	now := s.now()
	// Ensure tasks exist, then requeue every channel for a fresh attempt cycle.
	s.enqueueDeliveryForAuthorizedReceipt(receipt.ID)
	tasks, err := s.repo.ListDeliveryTasksForReceipt(businessID, receipt.ID)
	if err != nil {
		return err
	}
	for i := range tasks {
		task := tasks[i]
		switch task.Status {
		case database.FiscalDeliveryStatusSucceeded, database.FiscalDeliveryStatusDead,
			database.FiscalDeliveryStatusPending:
			// Force a new attempt: mark dead first if succeeded so Requeue accepts.
			if task.Status == database.FiscalDeliveryStatusSucceeded {
				_ = s.db.Model(&database.FiscalDeliveryTask{}).
					Where("id = ?", task.ID).
					Updates(map[string]interface{}{
						"status":       database.FiscalDeliveryStatusDead,
						"dead_at":      now,
						"succeeded_at": nil,
						"updated_at":   now,
					}).Error
			}
			if _, rqErr := s.repo.RequeueDeliveryTask(task.ID, businessID, defaultActor(actor), now); rqErr != nil {
				logger.Logger.Warnf("Fiscal resend: requeue task %d failed: %v", task.ID, rqErr)
			}
		}
	}

	rid := receipt.ID
	if auditErr := s.repo.WriteAudit(&database.FiscalAuditEvent{
		BusinessID: businessID,
		ReceiptID:  &rid,
		Actor:      defaultActor(actor),
		EventType:  "fiscal_receipt_resent",
		Message:    "operator requeued fiscal receipt delivery tasks",
	}); auditErr != nil {
		_ = auditErr
	}
	return nil
}

// DeliveryTaskView is the operator-safe projection of a delivery task.
// LastErrorCategory is a coarse category — never the raw error or recipient.
type DeliveryTaskView struct {
	ID                uint       `json:"id"`
	ReceiptID         uint       `json:"receipt_id"`
	Channel           string     `json:"channel"`
	Status            string     `json:"status"`
	Attempts          int        `json:"attempts"`
	MaxAttempts       int        `json:"max_attempts"`
	NextAttemptAt     *time.Time `json:"next_attempt_at,omitempty"`
	LastErrorCategory string     `json:"last_error_category,omitempty"`
	// MaskedRecipient is only set for email channel when a recipient exists;
	// always masked (e.g. g***@example.com). Never full email.
	MaskedRecipient string     `json:"masked_recipient,omitempty"`
	SucceededAt     *time.Time `json:"succeeded_at,omitempty"`
	DeadAt          *time.Time `json:"dead_at,omitempty"`
}

// ListReceiptDeliveryTasks returns business-scoped delivery task projections.
func (s *Service) ListReceiptDeliveryTasks(ctx context.Context, businessID, receiptID uint) ([]DeliveryTaskView, error) {
	// Verify receipt belongs to business.
	var receipt database.FiscalReceipt
	if err := s.db.WithContext(ctx).
		Select("id", "business_id", "bill_id").
		Where("id = ? AND business_id = ?", receiptID, businessID).
		First(&receipt).Error; err != nil {
		return nil, err
	}
	tasks, err := s.repo.ListDeliveryTasksForReceipt(businessID, receiptID)
	if err != nil {
		return nil, err
	}
	// Best-effort masked recipient for email rows.
	masked := ""
	if bill, berr := s.loadBillForDeliveryMask(ctx, receipt.BillID); berr == nil {
		if email := s.customerEmail(ctx, bill); email != "" {
			masked = maskEmail(email)
		}
	}
	out := make([]DeliveryTaskView, 0, len(tasks))
	for _, task := range tasks {
		view := DeliveryTaskView{
			ID:                task.ID,
			ReceiptID:         task.ReceiptID,
			Channel:           task.Channel,
			Status:            task.Status,
			Attempts:          task.Attempts,
			MaxAttempts:       task.MaxAttempts,
			NextAttemptAt:     task.NextAttemptAt,
			LastErrorCategory: categorizeDeliveryError(task.LastError),
			SucceededAt:       task.SucceededAt,
			DeadAt:            task.DeadAt,
		}
		if task.Channel == database.FiscalDeliveryChannelEmail && masked != "" {
			view.MaskedRecipient = masked
		}
		out = append(out, view)
	}
	return out, nil
}

// RetryDeliveryTask requeues a single dead/failed channel task for operators
// with fiscal:retry. Writes a FiscalAuditEvent. Idempotent on already-pending.
func (s *Service) RetryDeliveryTask(ctx context.Context, businessID, taskID uint, actor string) error {
	var task database.FiscalDeliveryTask
	if err := s.db.WithContext(ctx).
		Where("id = ? AND business_id = ?", taskID, businessID).
		First(&task).Error; err != nil {
		return err
	}
	if task.Status != database.FiscalDeliveryStatusDead &&
		!(task.Status == database.FiscalDeliveryStatusPending && task.LastError != "") {
		if task.Status == database.FiscalDeliveryStatusSucceeded ||
			task.Status == database.FiscalDeliveryStatusPending {
			return nil // double-click / already ok
		}
		return fmt.Errorf("task status %q is not requeueable", task.Status)
	}
	requeued, err := s.repo.RequeueDeliveryTask(taskID, businessID, defaultActor(actor), s.now())
	if err != nil {
		return err
	}
	rid := requeued.ReceiptID
	_ = s.repo.WriteAudit(&database.FiscalAuditEvent{
		BusinessID: businessID,
		ReceiptID:  &rid,
		Actor:      defaultActor(actor),
		EventType:  "fiscal_delivery_task_requeued",
		Message:    fmt.Sprintf("operator requeued delivery task %d channel %s", taskID, requeued.Channel),
		Metadata: map[string]interface{}{
			"task_id": taskID,
			"channel": requeued.Channel,
		},
	})
	return nil
}

func (s *Service) loadBillForDeliveryMask(ctx context.Context, billID uint) (database.Bill, error) {
	var bill database.Bill
	err := s.db.WithContext(ctx).
		Select("id", "business_id", "crm_customer_id").
		Where("id = ?", billID).
		First(&bill).Error
	return bill, err
}

func categorizeDeliveryError(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	switch {
	case strings.Contains(raw, "validation") || strings.Contains(raw, "permanent") ||
		strings.Contains(raw, "not_authorized") || strings.Contains(raw, "not_found"):
		return "validation"
	case strings.Contains(raw, "timeout") || strings.Contains(raw, "temporar") ||
		strings.Contains(raw, "503") || strings.Contains(raw, "429") ||
		strings.Contains(raw, "connection") || strings.Contains(raw, "smtp"):
		return "transient"
	case strings.Contains(raw, "render") || strings.Contains(raw, "pdf"):
		return "render"
	case strings.Contains(raw, "upload") || strings.Contains(raw, "s3"):
		return "storage"
	case strings.Contains(raw, "email") || strings.Contains(raw, "postmark") || strings.Contains(raw, "resend"):
		return "email"
	case strings.Contains(raw, "print"):
		return "print"
	default:
		return "unknown"
	}
}

// maskEmail redacts a local-part to first rune + *** + domain.
func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return "***"
	}
	local, domain := email[:at], email[at+1:]
	r := []rune(local)
	if len(r) == 0 {
		return "***@" + domain
	}
	return string(r[0]) + "***@" + domain
}

func (s *Service) enqueueIssueJob(
	bill *database.Bill,
	settings *database.BusinessFiscalSettings,
	paymentID *uint,
	alternativePaymentID *uint,
	actor string,
	reviveTerminal bool,
) error {
	key := buildIdempotencyKey(bill.BusinessID, bill.ID, ActionIssueReceipt)
	job, err := s.repo.CreateJobIfNotExists(CreateJobInput{
		BusinessID:           bill.BusinessID,
		SettingsID:           settings.ID,
		BillID:               bill.ID,
		PaymentID:            paymentID,
		AlternativePaymentID: alternativePaymentID,
		Action:               ActionIssueReceipt,
		IdempotencyKey:       key,
		CreatedBy:            actor,
	})
	if err != nil {
		return err
	}
	// Automatic enqueue must never resurrect a permanently failed job (it
	// would retry forever); an explicit operator action may.
	if !reviveTerminal || job == nil || job.Status != database.FiscalStatusFailedPermanent {
		return nil
	}
	return s.db.Model(&database.FiscalJob{}).
		Where("id = ? AND status = ?", job.ID, database.FiscalStatusFailedPermanent).
		Updates(map[string]interface{}{
			"status":             database.FiscalStatusPending,
			"attempts":           0,
			"max_attempts":       retryMaxAttempts(job.MaxAttempts),
			"next_attempt_at":    nil,
			"last_error_code":    nil,
			"last_error_message": nil,
			"locked_at":          nil,
			"locked_by":          nil,
			"created_by":         defaultActor(actor),
		}).Error
}

// buildIdempotencyKey is the per-BILL issue-job key. It deliberately does NOT
// include the payment/alt-payment dimension (F-DUPISSUE): the factura always
// covers the full bill total, so every issue path for a bill — the auto path
// (HandleBillPaid, with a concrete paymentID) and the manual operator path
// (IssueReceipt, nil/nil) — must collapse to a single job, deduped by the
// idx_fiscal_jobs_idempotency unique index. Keying on the payment would let one
// bill receive two full-bill facturas. The job still records payment_id/
// alternative_payment_id as reference columns; they just don't scope the key.
//
// Residual: a refund-then-repay of the same bill will not auto-reissue (the key
// already exists). That edge is acceptable — auto-reissuing after a credit note is
// not desired anyway — and is documented in the fiscal bug-fix spec.
func buildIdempotencyKey(businessID, billID uint, action string) string {
	return fmt.Sprintf("business:%d:bill:%d:action:%s:split:none", businessID, billID, action)
}

func normalizeAndValidateSettings(settings *database.BusinessFiscalSettings) error {
	settings.Country = strings.ToUpper(strings.TrimSpace(settings.Country))
	settings.Provider = strings.ToLower(strings.TrimSpace(settings.Provider))
	settings.Environment = strings.ToLower(strings.TrimSpace(settings.Environment))
	if settings.Environment == "" {
		settings.Environment = "sandbox"
	}
	if settings.SetupStatus == "" {
		settings.SetupStatus = "draft"
	}
	if !settings.Mode.IsValid() {
		return ErrInvalidSettings
	}
	switch {
	case settings.Country == "AR" && settings.Provider == "arca":
	case settings.Country == "AE" && settings.Provider == "edicom":
	default:
		return ErrInvalidSettings
	}
	switch settings.Environment {
	case "sandbox", "production":
	default:
		return ErrInvalidSettings
	}
	return nil
}

func isRetryableReceiptStatus(status database.FiscalStatus) bool {
	return status == database.FiscalStatusFailedRetryable
}

// isCreditableReceipt reports whether a receipt can be the target of a credit
// note: it must be an authorized issue receipt. A receipt that is still pending,
// failed, cancelled, already credited, or is itself a credit note cannot be
// credited.
func isCreditableReceipt(receipt database.FiscalReceipt) bool {
	return receipt.Action == ActionIssueReceipt && receipt.Status == database.FiscalStatusAuthorized
}

func isActiveRetryJob(job database.FiscalJob) bool {
	return job.Status == database.FiscalStatusPending || job.LockedAt != nil
}

func retryMaxAttempts(existing int) int {
	if existing > 0 {
		return existing
	}
	return 5
}
