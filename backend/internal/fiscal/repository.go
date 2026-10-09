package fiscal

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type CreateJobInput struct {
	BusinessID           uint
	SettingsID           uint
	ReceiptID            *uint
	BillID               uint
	PaymentID            *uint
	AlternativePaymentID *uint
	Action               string
	IdempotencyKey       string
	CreatedBy            string
}

func (r *Repository) GetActiveSettings(businessID uint) (*database.BusinessFiscalSettings, error) {
	settings, err := r.GetSettings(businessID)
	if err != nil || settings == nil || settings.Mode == database.FiscalModeOff {
		return nil, err
	}
	return settings, nil
}

func (r *Repository) GetSettings(businessID uint) (*database.BusinessFiscalSettings, error) {
	var settings database.BusinessFiscalSettings
	err := r.db.Where("business_id = ?", businessID).
		Order("updated_at DESC, id DESC").
		First(&settings).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &settings, nil
}

func (r *Repository) UpsertSettings(settings *database.BusinessFiscalSettings) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		existing, sameProvider, err := r.findSettingsUpdateTarget(tx, settings)
		if err != nil {
			return err
		}
		preserveCredentialFields(settings, existing, sameProvider)
		existing.Country = settings.Country
		existing.Provider = settings.Provider
		existing.Mode = settings.Mode
		existing.Environment = settings.Environment
		existing.TaxID = settings.TaxID
		existing.TaxCondition = settings.TaxCondition
		existing.PointOfSale = settings.PointOfSale
		existing.CredentialsEncrypted = settings.CredentialsEncrypted
		existing.CredentialsFingerprint = settings.CredentialsFingerprint
		existing.CredentialsExpiresAt = settings.CredentialsExpiresAt
		existing.ProviderConfig = settings.ProviderConfig
		existing.SetupStatus = settings.SetupStatus
		existing.LastValidatedAt = settings.LastValidatedAt
		existing.LastValidationError = settings.LastValidationError
		if err := tx.Save(&existing).Error; err != nil {
			return err
		}
		*settings = existing
		return nil
	})
}

func (r *Repository) findSettingsUpdateTarget(tx *gorm.DB, settings *database.BusinessFiscalSettings) (database.BusinessFiscalSettings, bool, error) {
	var sameProvider database.BusinessFiscalSettings
	err := tx.Where("business_id = ? AND country = ? AND provider = ?", settings.BusinessID, settings.Country, settings.Provider).
		First(&sameProvider).Error
	if err == nil {
		return sameProvider, true, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return database.BusinessFiscalSettings{}, false, err
	}

	var existing database.BusinessFiscalSettings
	err = tx.Where("business_id = ?", settings.BusinessID).
		Order("updated_at DESC, id DESC").
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := tx.Create(settings).Error; err != nil {
			return database.BusinessFiscalSettings{}, false, err
		}
		return *settings, true, nil
	}
	if err != nil {
		return database.BusinessFiscalSettings{}, false, err
	}
	return existing, false, nil
}

func preserveCredentialFields(settings *database.BusinessFiscalSettings, existing database.BusinessFiscalSettings, sameProvider bool) {
	if !sameProvider {
		return
	}
	if len(settings.CredentialsEncrypted) == 0 {
		settings.CredentialsEncrypted = existing.CredentialsEncrypted
	}
	if settings.CredentialsFingerprint == "" {
		settings.CredentialsFingerprint = existing.CredentialsFingerprint
	}
	if settings.CredentialsExpiresAt == nil {
		settings.CredentialsExpiresAt = existing.CredentialsExpiresAt
	}
}

func (r *Repository) CreateJobIfNotExists(input CreateJobInput) (*database.FiscalJob, error) {
	job := database.FiscalJob{
		BusinessID:           input.BusinessID,
		SettingsID:           input.SettingsID,
		ReceiptID:            input.ReceiptID,
		BillID:               input.BillID,
		PaymentID:            input.PaymentID,
		AlternativePaymentID: input.AlternativePaymentID,
		Action:               input.Action,
		IdempotencyKey:       input.IdempotencyKey,
		Status:               database.FiscalStatusPending,
		MaxAttempts:          5,
		CreatedBy:            defaultActor(input.CreatedBy),
	}
	err := r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(&job).Error
	if err != nil {
		return nil, err
	}
	var out database.FiscalJob
	if err := r.db.Where("idempotency_key = ?", input.IdempotencyKey).First(&out).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

// JobContext carries the projected rows the worker needs to build an IssueInput
// and persist a receipt for a claimed job. It is deliberately narrow: only the
// columns actually consumed downstream are selected, so the hot worker path
// never issues a `SELECT *` over the wide bills/business tables.
type JobContext struct {
	Bill     database.Bill
	Settings database.BusinessFiscalSettings
	Business database.Business
}

// LoadJobContext loads the bill, fiscal settings, and business referenced by a
// claimed job using explicit column projections (no SELECT *, no N+1). Each row
// is fetched once by primary key.
func (r *Repository) LoadJobContext(job database.FiscalJob) (*JobContext, error) {
	var ctx JobContext

	if err := r.db.Model(&database.Bill{}).
		Select(billProjectionColumns).
		Where("id = ?", job.BillID).
		First(&ctx.Bill).Error; err != nil {
		return nil, err
	}

	if err := r.db.Model(&database.BusinessFiscalSettings{}).
		Select(settingsProjectionColumns).
		Where("id = ?", job.SettingsID).
		First(&ctx.Settings).Error; err != nil {
		return nil, err
	}

	if err := r.db.Model(&database.Business{}).
		Select(businessProjectionColumns).
		Where("id = ?", job.BusinessID).
		First(&ctx.Business).Error; err != nil {
		return nil, err
	}

	return &ctx, nil
}

// billProjectionColumns lists only the bill columns the fiscal pipeline reads:
// identity, money (cents), status, and the fiscal-customer identity columns
// (Task 21). Wide JSON/text columns (items, notes) are intentionally excluded.
var billProjectionColumns = []string{
	"id", "business_id", "bill_number", "subtotal", "tax_amount",
	"service_fee_amount", "loyalty_discount_cents", "total_amount", "paid_amount", "tip_amount",
	"status", "fiscal_customer_doc_type", "fiscal_customer_doc_number",
	"fiscal_customer_tax_condition", "fiscal_customer_name",
	"fiscal_customer_email",
	// fiscal_customer_guest_session gates the issuance-time payer-binding check
	// (dropUnpaidGuestFiscalIdentity): NULL skips it without a query.
	"fiscal_customer_guest_session",
	// crm_customer_id lets the delivery path resolve the recipient email with a
	// single narrow customers lookup (no Preload of the wide customer row).
	"crm_customer_id",
}

// settingsProjectionColumns lists the fiscal-settings columns needed to build an
// IssueInput and resolve a provider via the factory (which reads the encrypted
// credentials column).
var settingsProjectionColumns = []string{
	"id", "business_id", "country", "provider", "mode", "environment",
	"tax_id", "tax_condition", "point_of_sale", "credentials_encrypted",
	"credentials_fingerprint", "credentials_expires_at", "provider_config",
	"setup_status",
}

// businessProjectionColumns lists the business columns the receipt/IssueInput
// needs (emitter identity + default_currency, which billCurrency uses to resolve
// the fiscal currency since Bill.Currency is a transient gorm:"-" field). The
// bills/business tables are wide, so an explicit projection avoids hauling
// unrelated columns into the hot worker path.
var businessProjectionColumns = []string{
	"id", "business_id", "name", "owner_address", "default_currency",
	"tax_rate",
}

// SaveReceiptForJob persists a FiscalReceipt produced by processing a job and
// records it on the job, both inside one transaction. Returns the created
// receipt ID.
//
// produced_receipt_id always becomes the new receipt. receipt_id is the input
// the job was enqueued against: issue jobs adopt the new receipt (there is no
// prior receipt), credit-note jobs keep pointing at the original so later
// credits still count against it. A deferred credit note (receipt_id NULL)
// is bound to originalReceiptID when that id is supplied.
//
// When the receipt is an authorized issue document, durable per-channel
// delivery tasks (artifact/email/print) are enqueued in the SAME transaction
// so a crash cannot leave an authorized receipt without delivery intent
// (Wave 4 crash-window contract). Email is always enqueued; the worker
// skips send when no recipient resolves at execution (no PII on the task).
func (r *Repository) SaveReceiptForJob(receipt *database.FiscalReceipt, jobID uint, originalReceiptID *uint) (uint, error) {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(receipt).Error; err != nil {
			return err
		}
		var job database.FiscalJob
		if err := tx.Select("id", "action", "receipt_id").First(&job, jobID).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{
			"produced_receipt_id": receipt.ID,
		}
		switch job.Action {
		case ActionIssueReceipt:
			updates["receipt_id"] = receipt.ID
		case ActionCreditNote:
			if job.ReceiptID == nil && originalReceiptID != nil {
				updates["receipt_id"] = *originalReceiptID
			}
		}
		if err := tx.Model(&database.FiscalJob{}).
			Where("id = ?", jobID).
			Updates(updates).Error; err != nil {
			return err
		}
		if receipt.Status == database.FiscalStatusAuthorized && receipt.Action == ActionIssueReceipt {
			return r.enqueueAuthorizedDeliveryTasksInTx(tx, receipt, time.Now().UTC())
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return receipt.ID, nil
}

// enqueueAuthorizedDeliveryTasksInTx creates artifact+email+print delivery
// tasks for an authorized issue receipt. Idempotent on (receipt_id, channel).
func (r *Repository) enqueueAuthorizedDeliveryTasksInTx(tx *gorm.DB, receipt *database.FiscalReceipt, now time.Time) error {
	if receipt == nil {
		return nil
	}
	locale := ""
	// Locale is non-PII; recipient is re-resolved at worker execution.
	channels := []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelArtifact, Locale: locale},
		{Channel: database.FiscalDeliveryChannelEmail, Locale: locale},
		{Channel: database.FiscalDeliveryChannelPrint, Locale: locale},
	}
	baseKey := fmt.Sprintf("receipt:%d:channel", receipt.ID)
	return r.EnqueueDeliveryTasks(tx, receipt.ID, receipt.BusinessID, channels, baseKey, now)
}

// UpdateReceiptStatus reconciles a fiscal_receipts row to a terminal status
// discovered by a status_check job (e.g. an earlier failed_retryable receipt that
// FECompConsultar now reports authorized). It is a narrow column update by primary
// key — status + updated_at only — so it never issues a SELECT * or rehydrates the
// wide receipt aggregate.
func (r *Repository) UpdateReceiptStatus(receiptID uint, status database.FiscalStatus, now time.Time) error {
	// F-STATUSDOWN: this is the DOWNGRADE/reconcile path (a status_check resolving a
	// non-authorized terminal state, or a budget-exhaustion). It must never overwrite
	// an authorized or credited receipt — a stale or erroneous reconcile flipping a
	// genuinely-CAE'd invoice to failed_permanent would hide a real legal invoice.
	// The authorized backfill goes through BackfillAuthorizedReceipt, not here, so
	// guarding against these two terminal states here is safe.
	return r.db.Model(&database.FiscalReceipt{}).
		Where("id = ? AND status NOT IN ?", receiptID,
			[]database.FiscalStatus{database.FiscalStatusAuthorized, database.FiscalStatusCredited}).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": now,
		}).Error
}

// BackfillAuthorizedReceiptFields carries the authorization detail a status_check
// reconcile resolved for a now-authorized receipt. Empty/nil fields are NOT
// written, so a reconcile that can only confirm the status (e.g. an
// un-rebuildable QR, a missing CAE) never clobbers an existing non-empty column.
type BackfillAuthorizedReceiptFields struct {
	AuthCode      string
	ReceiptNumber string
	AuthExpiresAt *time.Time
	QRPayload     string
	IssuedAt      *time.Time
}

// BackfillAuthorizedReceipt flips a receipt to authorized and backfills the
// authorization tuple a reconcile resolved (D-3). It is a narrow Updates(map) by
// primary key writing only the non-empty fields plus status + updated_at, so a
// status-only confirmation never clobbers CAE/QR/number with empty values.
// issued_at is stamped only when the row was never issued, so a re-confirmation
// can never move an already-set legal issue date.
func (r *Repository) BackfillAuthorizedReceipt(receiptID uint, fields BackfillAuthorizedReceiptFields, now time.Time) error {
	updates := map[string]interface{}{
		"status":     database.FiscalStatusAuthorized,
		"updated_at": now,
	}
	if fields.AuthCode != "" {
		updates["auth_code"] = fields.AuthCode
	}
	if fields.ReceiptNumber != "" {
		updates["receipt_number"] = fields.ReceiptNumber
	}
	if fields.AuthExpiresAt != nil {
		updates["auth_expires_at"] = *fields.AuthExpiresAt
	}
	if fields.QRPayload != "" {
		updates["qr_payload"] = fields.QRPayload
	}
	// Both writes run in one transaction so a crash can never leave a receipt
	// status=authorized with issued_at NULL (which would render a PDF dated from
	// created_at instead of the AFIP authorization date). The issued_at IS NULL
	// guard still holds inside the transaction — a re-confirmation never moves a
	// legal issue date that an earlier authorization already set.
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&database.FiscalReceipt{}).
			Where("id = ?", receiptID).
			Updates(updates).Error; err != nil {
			return err
		}
		if fields.IssuedAt != nil {
			if err := tx.Model(&database.FiscalReceipt{}).
				Where("id = ? AND issued_at IS NULL", receiptID).
				Update("issued_at", *fields.IssuedAt).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// MarkReceiptDelivered stamps delivered_at on a fiscal_receipts row. It is a
// narrow column update by primary key (delivered_at + updated_at only) so it
// never rehydrates the wide receipt aggregate. Setting delivered_at is the
// idempotency guard against re-sending an already-delivered receipt.
func (r *Repository) MarkReceiptDelivered(receiptID uint, now time.Time) error {
	return r.db.Model(&database.FiscalReceipt{}).
		Where("id = ?", receiptID).
		Updates(map[string]interface{}{
			"delivered_at": now,
			"updated_at":   now,
		}).Error
}

// ListUndeliveredAuthorizedReceipts returns authorized receipts whose delivery
// never completed (delivered_at IS NULL) and which were issued before olderThan
// (a grace window so the sweep does not race the inline post-issue delivery).
// Oldest-first and bounded by limit so a backlog drains over multiple sweeps.
// The partial index on (issued_at) WHERE
// status='authorized' AND delivered_at IS NULL keeps this selective. Full rows
// are returned because re-delivery (RenderReceiptPDF + email) needs the receipt.
func (r *Repository) ListUndeliveredAuthorizedReceipts(olderThan time.Time, limit int) ([]database.FiscalReceipt, error) {
	if limit <= 0 {
		limit = 25
	}
	var receipts []database.FiscalReceipt
	err := r.db.
		Where("status = ? AND delivered_at IS NULL AND issued_at IS NOT NULL AND issued_at < ?",
			database.FiscalStatusAuthorized, olderThan).
		Order("issued_at ASC, id ASC").
		Limit(limit).
		Find(&receipts).Error
	if err != nil {
		return nil, err
	}
	return receipts, nil
}

// ClaimReceiptDelivery atomically reserves an undelivered receipt for one
// worker: it stamps delivery_locked_at/by WHERE delivered_at IS NULL and the
// lock is free or stale (older than staleBefore). RowsAffected==1 means this
// worker won. Mirrors the ClaimDueFiscalJobs pattern so a crash mid-delivery
// is reclaimed after the TTL instead of stranding the lock.
func (r *Repository) ClaimReceiptDelivery(receiptID uint, lockedBy string, staleBefore time.Time) (bool, error) {
	res := r.db.Model(&database.FiscalReceipt{}).
		Where("id = ? AND delivered_at IS NULL AND (delivery_locked_at IS NULL OR delivery_locked_at < ?)",
			receiptID, staleBefore).
		Updates(map[string]interface{}{"delivery_locked_at": time.Now(), "delivery_locked_by": lockedBy})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// UpdateReceiptArtifactPaths writes the stored PDF/QR object paths on a receipt
// row. It is a narrow column update by primary key (no SELECT *). Safe to re-run:
// the paths are an idempotent overwrite and do not gate redelivery.
func (r *Repository) UpdateReceiptArtifactPaths(receiptID uint, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	return r.db.Model(&database.FiscalReceipt{}).
		Where("id = ?", receiptID).
		Updates(updates).Error
}

// RecordJobAttempt persists the provider voucher number a job is about to
// request, before that request is sent. A later retry reads it back and
// reconciles that exact number instead of allocating LastAuthorized+1.
//
// The write is CAS'd on locked_by: a worker whose lock was reclaimed must not
// overwrite the new owner's recorded attempt, and must not send the request.
// Anything other than exactly one updated row is an error.
func (r *Repository) RecordJobAttempt(jobID uint, lockedBy, providerReceiptID string) error {
	if lockedBy == "" {
		return fmt.Errorf("record attempt for fiscal job %d: job is not locked", jobID)
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		var settingsID uint
		if err := tx.Model(&database.FiscalJob{}).Where("id = ?", jobID).
			Select("settings_id").Scan(&settingsID).Error; err != nil {
			return err
		}
		// Serialize attempt recording per series. Without this, two concurrent
		// transactions recording the same number each update only their own row
		// and, under READ COMMITTED, neither sees the other's uncommitted claim,
		// so both claims survive. Locking the settings row makes the second
		// transaction see the first's committed claim and supersede it.
		// Postgres-only: SQLite (tests) cannot render FOR UPDATE and is
		// single-writer anyway.
		if settingsID != 0 && rowLockingSupported(tx.Dialector.Name()) {
			var lockedID uint
			if err := tx.Model(&database.BusinessFiscalSettings{}).
				Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ?", settingsID).Select("id").Scan(&lockedID).Error; err != nil {
				return err
			}
		}
		res := tx.Model(&database.FiscalJob{}).
			Where("id = ? AND locked_by = ?", jobID, lockedBy).
			Update("attempted_provider_receipt_id", providerReceiptID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return fmt.Errorf("record attempt for fiscal job %d: lock no longer held by %q", jobID, lockedBy)
		}
		// The provider records an attempt only after reading LastAuthorized
		// under the series lock, so this number was unused at AFIP when it was
		// chosen. Another still-live job that recorded the same number made an
		// earlier attempt that never committed; leaving it would let that job's
		// retry consult the number, see this job's CAE and adopt it before this
		// job has saved its receipt. Clear the stale claim so that retry
		// allocates a fresh number instead. Authorized/terminal jobs are never
		// touched, and the provider receipt id embeds voucher type and point of
		// sale, so the match stays within the same series.
		return tx.Model(&database.FiscalJob{}).
			Where("settings_id = ? AND id <> ? AND attempted_provider_receipt_id = ? AND status IN ?",
				settingsID, jobID, providerReceiptID,
				[]database.FiscalStatus{database.FiscalStatusPending, database.FiscalStatusFailedRetryable}).
			Update("attempted_provider_receipt_id", nil).Error
	})
}

// ProviderReceiptClaimedByOtherJob reports whether an authorized receipt
// stored for another job already holds providerReceiptID in this job's
// settings (the series). SaveReceiptForJob creates the receipt and sets
// produced_receipt_id in one transaction, so an authorized receipt holding
// that id belongs to the job that received the CAE. The job's own produced
// receipt is excluded.
func (r *Repository) ProviderReceiptClaimedByOtherJob(job database.FiscalJob, providerReceiptID string) (bool, error) {
	q := r.db.Model(&database.FiscalReceipt{}).
		Where("settings_id = ? AND provider_receipt_id = ? AND status = ?",
			job.SettingsID, providerReceiptID, database.FiscalStatusAuthorized)
	if job.ProducedReceiptID != nil {
		q = q.Where("id <> ?", *job.ProducedReceiptID)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// MarkJobResult writes the terminal/transition state for a job and clears the
// lock. It is CAS'd on locked_by (F-RECLAIM): only the worker that currently owns
// the lock may write the result. A worker whose lock was reclaimed mid-flight
// (locked_by cleared and possibly re-owned by another worker) writes nothing, so
// its late result cannot clobber the reclaiming worker's outcome under 2+ replicas.
func (r *Repository) MarkJobResult(jobID uint, lockedBy string, updates map[string]interface{}, now time.Time) error {
	_, err := r.MarkJobResultApplied(jobID, lockedBy, updates, now)
	return err
}

// MarkJobResultApplied is MarkJobResult that also reports whether this worker
// still owned the lock and the result was written. Callers use it to skip
// follow-up side effects (alerts, metrics, audit) for a result that was dropped.
func (r *Repository) MarkJobResultApplied(jobID uint, lockedBy string, updates map[string]interface{}, now time.Time) (bool, error) {
	if updates == nil {
		updates = map[string]interface{}{}
	}
	updates["locked_at"] = nil
	updates["locked_by"] = nil
	updates["updated_at"] = now
	res := r.db.Model(&database.FiscalJob{}).
		Where("id = ? AND locked_by = ?", jobID, lockedBy).
		Updates(updates)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// WriteAudit appends a fiscal_audit_event row describing a job outcome.
func (r *Repository) WriteAudit(event *database.FiscalAuditEvent) error {
	return r.db.Create(event).Error
}

// SumAuthorizedCreditCents returns the total cents already credited against an
// original receipt by AUTHORIZED credit-note jobs. A nil CreditAmountCents means a
// full credit, so it counts as fullTotal (the original receipt total). Pending /
// failed credit-note jobs are excluded — only realized (authorized) credits count
// toward "fully credited" (F-CREDITED).
func (r *Repository) SumAuthorizedCreditCents(originalReceiptID uint, fullTotal int64) (int64, error) {
	var sum int64
	err := r.db.Model(&database.FiscalJob{}).
		Where("receipt_id = ? AND action = ? AND status = ?",
			originalReceiptID, ActionCreditNote, database.FiscalStatusAuthorized).
		Select("COALESCE(SUM(COALESCE(credit_amount_cents, ?)), 0)", fullTotal).
		Scan(&sum).Error
	if err != nil {
		return 0, err
	}
	return sum, nil
}

// SumLiveCreditCents sums the cents reserved by credit-note jobs that are still
// live against original. Authorized, pending, and retryable jobs count;
// failed_permanent, rejected, and cancelled do not. A nil credit_amount_cents
// counts as original.TotalAmountCents (a full credit).
//
// A job matches when receipt_id is the original receipt, or when a deferred
// credit has not been bound yet (receipt_id IS NULL) and its bill_id and
// business_id match the original. excludeJobID (when > 0) and excludeKey
// (when non-empty) drop the current job so a retry does not count itself.
// One SQL aggregate; no job rows are hydrated.
func (r *Repository) SumLiveCreditCents(tx *gorm.DB, original database.FiscalReceipt, excludeJobID uint, excludeKey string) (int64, error) {
	if tx == nil {
		tx = r.db
	}
	query := tx.Model(&database.FiscalJob{}).
		Where("action = ?", ActionCreditNote).
		Where("status NOT IN ?", []database.FiscalStatus{
			database.FiscalStatusFailedPermanent,
			database.FiscalStatusRejected,
			database.FiscalStatusCancelled,
		}).
		Where("(receipt_id = ? OR (receipt_id IS NULL AND bill_id = ? AND business_id = ?))",
			original.ID, original.BillID, original.BusinessID)
	if excludeJobID > 0 {
		query = query.Where("id <> ?", excludeJobID)
	}
	if excludeKey != "" {
		query = query.Where("idempotency_key <> ?", excludeKey)
	}
	var sum int64
	err := query.Select("COALESCE(SUM(COALESCE(credit_amount_cents, ?)), 0)", original.TotalAmountCents).
		Scan(&sum).Error
	if err != nil {
		return 0, err
	}
	return sum, nil
}

// MarkReceiptCreditedIfAuthorized CAS-transitions an original issue receipt
// authorized → credited. The status guard makes it idempotent and ensures only an
// authorized receipt is moved (never a re-credit of an already-credited row).
// Returns true when this call performed the transition.
func (r *Repository) MarkReceiptCreditedIfAuthorized(receiptID uint, now time.Time) (bool, error) {
	res := r.db.Model(&database.FiscalReceipt{}).
		Where("id = ? AND status = ?", receiptID, database.FiscalStatusAuthorized).
		Updates(map[string]interface{}{
			"status":     database.FiscalStatusCredited,
			"updated_at": now,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// splitStatusFilter turns a status query value ("authorized" or a comma list
// like "failed_permanent,rejected") into trimmed non-empty entries.
func splitStatusFilter(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func (r *Repository) ListReceipts(businessID uint, status string, limit int) ([]database.FiscalReceipt, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := r.db.Where("business_id = ?", businessID).Order("created_at DESC").Limit(limit)
	if statuses := splitStatusFilter(status); len(statuses) == 1 {
		query = query.Where("status = ?", statuses[0])
	} else if len(statuses) > 1 {
		query = query.Where("status IN ?", statuses)
	}
	var receipts []database.FiscalReceipt
	if err := query.Find(&receipts).Error; err != nil {
		return nil, err
	}
	normalizeReceiptTypes(receipts)
	return receipts, nil
}

// ListIssuableBills returns recent paid (or already-invoiced) bills for the
// invoice picker. Newest closed first. Optional q matches bill_number, table
// name, or guest fiscal name (case-insensitive). Limit is clamped to 1..20
// (default 20).
//
// Rows with a blocking issue_receipt set ExistingReceiptID so the UI can offer
// "View invoice" instead of an empty "no paid bills waiting" state (issue 260).
// TotalAmount is projected in dollars for the FE money contract.
type issuableScope int

const (
	// Paid or collected — the cheap dinner-service arm (index-friendly).
	issuableScopePaidLike issuableScope = iota
	// Closed/invoiced with no paid_amount and status != paid.
	issuableScopeInvoicedOnly
)

type issuableRow struct {
	BillID               uint
	BillNumber           string
	TableLabel           string
	ClosedAt             *time.Time
	TotalCents           int64
	Currency             string
	CustomerDocType      *string
	CustomerDocNumber    *string
	CustomerTaxCondition *string
	CustomerName         *string
	ExistingReceiptID    *uint
}

// blockingIssueReceiptStatuses are the issue-receipt statuses that make a bill
// count as already invoiced: the factura is live at the provider, or on its way
// there. failed_retryable / failed_permanent / rejected / cancelled deliberately
// stay out — those bills are genuinely still issuable, and manual issue is how
// an operator recovers them.
//
// This is the Go-side definition, used by the manual-issue guard. The picker's
// subquery inlines the same three literals (GORM's `IN ?` expansion inside that
// subquery 500'd it), and TestListIssuableBillsAccessShape asserts the emitted
// SQL still mentions every status listed here — so the list, which surfaces the
// state as existing_receipt_id, and the write path, which refuses it, cannot
// disagree about what "already invoiced" means.
var blockingIssueReceiptStatuses = []database.FiscalStatus{
	database.FiscalStatusPending,
	database.FiscalStatusAuthorized,
	database.FiscalStatusCredited,
}

// FindBlockingIssueReceiptID returns the newest issue receipt that blocks a new
// issue for this bill, or 0 when the bill is still issuable. Only the id is
// projected — callers need existence, not the row.
func (r *Repository) FindBlockingIssueReceiptID(businessID, billID uint) (uint, error) {
	var ids []uint
	if err := r.db.Model(&database.FiscalReceipt{}).
		Where("business_id = ? AND bill_id = ? AND action = ? AND status IN ?",
			businessID, billID, ActionIssueReceipt, blockingIssueReceiptStatuses).
		Order("id DESC").
		Limit(1).
		Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return ids[0], nil
}

func (r *Repository) ListIssuableBills(businessID uint, q string, limit int) ([]IssuableBill, error) {
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	q = strings.TrimSpace(q)

	// Two bounded scans instead of one OR+EXISTS over the whole bill table:
	// paid/collected rows stay on the legacy-cheap predicate; already-invoiced
	// leftovers are a second Limit-N join so "217 invoices exist" cannot render
	// as an empty waiting list (issue 260).
	paidLike, err := r.scanIssuableBills(businessID, q, limit, issuableScopePaidLike)
	if err != nil {
		return nil, err
	}
	invoiced, err := r.scanIssuableBills(businessID, q, limit, issuableScopeInvoicedOnly)
	if err != nil {
		return nil, err
	}
	return mergeIssuableBillsNewest(paidLike, invoiced, limit), nil
}

func (r *Repository) scanIssuableBills(businessID uint, q string, limit int, scope issuableScope) ([]IssuableBill, error) {
	query := r.db.Table("bills").
		Select(`
			bills.id AS bill_id,
			bills.bill_number AS bill_number,
			COALESCE(tables.name, '') AS table_label,
			bills.closed_at AS closed_at,
			bills.total_amount AS total_cents,
			COALESCE(NULLIF(TRIM(businesses.default_currency), ''), 'ARS') AS currency,
			bills.fiscal_customer_doc_type AS customer_doc_type,
			bills.fiscal_customer_doc_number AS customer_doc_number,
			bills.fiscal_customer_tax_condition AS customer_tax_condition,
			bills.fiscal_customer_name AS customer_name,
			blocking.id AS existing_receipt_id
		`).
		Joins("LEFT JOIN tables ON tables.id = bills.table_id AND tables.business_id = bills.business_id").
		Joins("LEFT JOIN businesses ON businesses.id = bills.business_id").
		// The blocking statuses are INLINED literals on purpose: GORM's `IN ?`
		// expansion inside this subquery 500'd the picker, and
		// TestListIssuableBillsAccessShape pins them against
		// blockingIssueReceiptStatuses so this list and the manual-issue guard
		// cannot drift apart.
		Joins(`LEFT JOIN (
			SELECT bill_id, MAX(id) AS id
			FROM fiscal_receipts
			WHERE business_id = ?
			  AND action = ?
			  AND status IN ('pending', 'authorized', 'credited')
			GROUP BY bill_id
		) blocking ON blocking.bill_id = bills.id`, businessID, ActionIssueReceipt)

	switch scope {
	case issuableScopePaidLike:
		query = query.Where(
			"bills.business_id = ? AND (bills.status = ? OR bills.paid_amount > 0)",
			businessID, database.BillStatusPaid,
		)
	case issuableScopeInvoicedOnly:
		query = query.
			Joins(`INNER JOIN (
				SELECT DISTINCT bill_id
				FROM fiscal_receipts
				WHERE business_id = ?
			) any_receipt ON any_receipt.bill_id = bills.id`, businessID).
			Where(
				"bills.business_id = ? AND bills.status <> ? AND bills.paid_amount = 0",
				businessID, database.BillStatusPaid,
			)
	}

	if q != "" {
		pattern := "%" + strings.ToLower(q) + "%"
		query = query.Where(
			`LOWER(bills.bill_number) LIKE ?
			 OR LOWER(COALESCE(tables.name, '')) LIKE ?
			 OR LOWER(COALESCE(bills.fiscal_customer_name, '')) LIKE ?
			 OR CAST(bills.id AS TEXT) LIKE ?`,
			pattern, pattern, pattern, pattern,
		)
	}

	var rows []issuableRow
	if err := query.
		Order("bills.closed_at DESC, bills.id DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]IssuableBill, len(rows))
	for i, row := range rows {
		out[i] = IssuableBill{
			BillID:               row.BillID,
			BillNumber:           row.BillNumber,
			TableLabel:           row.TableLabel,
			ClosedAt:             row.ClosedAt,
			TotalAmount:          float64(row.TotalCents) / 100.0,
			Currency:             row.Currency,
			CustomerDocType:      strings.TrimSpace(strValue(row.CustomerDocType)),
			CustomerDocNumber:    strings.TrimSpace(strValue(row.CustomerDocNumber)),
			CustomerTaxCondition: strings.TrimSpace(strValue(row.CustomerTaxCondition)),
			CustomerName:         strings.TrimSpace(strValue(row.CustomerName)),
			ExistingReceiptID:    row.ExistingReceiptID,
		}
	}
	return out, nil
}

func mergeIssuableBillsNewest(paidLike, invoiced []IssuableBill, limit int) []IssuableBill {
	seen := make(map[uint]struct{}, len(paidLike)+len(invoiced))
	out := make([]IssuableBill, 0, len(paidLike)+len(invoiced))
	for _, row := range paidLike {
		if _, ok := seen[row.BillID]; ok {
			continue
		}
		seen[row.BillID] = struct{}{}
		out = append(out, row)
	}
	for _, row := range invoiced {
		if _, ok := seen[row.BillID]; ok {
			continue
		}
		seen[row.BillID] = struct{}{}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ci, cj := out[i].ClosedAt, out[j].ClosedAt
		switch {
		case ci == nil && cj == nil:
			return out[i].BillID > out[j].BillID
		case ci == nil:
			return false
		case cj == nil:
			return true
		case !ci.Equal(*cj):
			return ci.After(*cj)
		default:
			return out[i].BillID > out[j].BillID
		}
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// receiptNeedsAttentionStatuses are fiscal receipt statuses that flag a row as
// needing operator attention (retry / investigate).
var receiptNeedsAttentionStatuses = []database.FiscalStatus{
	database.FiscalStatusFailedRetryable,
	database.FiscalStatusFailedPermanent,
}

// ListReceiptsPage returns a paginated receipts list with embedded delivery
// badges loaded in a single IN query, plus needs_attention per row.
func (r *Repository) ListReceiptsPage(params ListReceiptsParams) (*ReceiptsPage, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	query := r.db.Model(&database.FiscalReceipt{}).
		Where("fiscal_receipts.business_id = ?", params.BusinessID)

	if statuses := splitStatusFilter(params.Status); len(statuses) == 1 {
		query = query.Where("fiscal_receipts.status = ?", statuses[0])
	} else if len(statuses) > 1 {
		query = query.Where("fiscal_receipts.status IN ?", statuses)
	}
	if rt := strings.TrimSpace(params.ReceiptType); rt != "" {
		if values := ReceiptTypeFilterValues(rt); len(values) == 1 {
			query = query.Where("fiscal_receipts.receipt_type = ?", values[0])
		} else if len(values) > 1 {
			query = query.Where("fiscal_receipts.receipt_type IN ?", values)
		}
	}
	if params.Start != nil {
		query = query.Where("fiscal_receipts.created_at >= ?", params.Start.UTC())
	}
	if params.End != nil {
		query = query.Where("fiscal_receipts.created_at < ?", params.End.UTC())
	}
	if params.NeedsAttention {
		// Receipt failed_* OR EXISTS a dead delivery task for this receipt.
		query = query.Where(
			"fiscal_receipts.status IN ? OR EXISTS (SELECT 1 FROM fiscal_delivery_tasks t WHERE t.receipt_id = fiscal_receipts.id AND t.status = ?)",
			receiptNeedsAttentionStatuses,
			database.FiscalDeliveryStatusDead,
		)
	}
	if q := strings.TrimSpace(params.Q); q != "" {
		pattern := "%" + strings.ToLower(q) + "%"
		query = query.Where(
			`CAST(fiscal_receipts.bill_id AS TEXT) LIKE ?
			 OR LOWER(COALESCE(fiscal_receipts.receipt_number, '')) LIKE ?
			 OR LOWER(COALESCE(fiscal_receipts.customer_name, '')) LIKE ?
			 OR EXISTS (
				SELECT 1 FROM bills b
				LEFT JOIN tables t ON t.id = b.table_id AND t.business_id = b.business_id
				WHERE b.id = fiscal_receipts.bill_id
				  AND (
					LOWER(COALESCE(b.bill_number, '')) LIKE ?
					OR LOWER(COALESCE(t.name, '')) LIKE ?
					OR LOWER(COALESCE(b.fiscal_customer_name, '')) LIKE ?
				  )
			 )`,
			pattern, pattern, pattern, pattern, pattern, pattern,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	var receipts []database.FiscalReceipt
	if err := query.
		Order("fiscal_receipts.created_at DESC, fiscal_receipts.id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&receipts).Error; err != nil {
		return nil, err
	}

	// ONE query for all delivery tasks on this page — no per-row N+1.
	deliveryByReceipt := make(map[uint][]ReceiptDeliveryBadge, len(receipts))
	tableByBill := make(map[uint]string, len(receipts))
	if len(receipts) > 0 {
		ids := make([]uint, len(receipts))
		billIDs := make([]uint, 0, len(receipts))
		seenBill := make(map[uint]struct{}, len(receipts))
		for i, rec := range receipts {
			ids[i] = rec.ID
			if _, ok := seenBill[rec.BillID]; !ok {
				seenBill[rec.BillID] = struct{}{}
				billIDs = append(billIDs, rec.BillID)
			}
		}
		var tasks []database.FiscalDeliveryTask
		if err := r.db.
			Where("receipt_id IN ?", ids).
			Order("id ASC").
			Find(&tasks).Error; err != nil {
			return nil, err
		}
		for _, t := range tasks {
			deliveryByReceipt[t.ReceiptID] = append(deliveryByReceipt[t.ReceiptID], ReceiptDeliveryBadge{
				TaskID:  t.ID,
				Channel: t.Channel,
				Status:  t.Status,
			})
		}

		type billTableRow struct {
			BillID     uint
			TableLabel string
		}
		var billTables []billTableRow
		// Best-effort: thin test DBs may lack bills/tables. Never fail the
		// receipts page over a dinner-service enrichment column.
		if err := r.db.Table("bills").
			Select("bills.id AS bill_id, COALESCE(tables.name, '') AS table_label").
			Joins("LEFT JOIN tables ON tables.id = bills.table_id AND tables.business_id = bills.business_id").
			Where("bills.id IN ?", billIDs).
			Scan(&billTables).Error; err == nil {
			for _, row := range billTables {
				tableByBill[row.BillID] = row.TableLabel
			}
		}
	}

	normalizeReceiptTypes(receipts)

	out := make([]ReceiptRow, len(receipts))
	for i, rec := range receipts {
		badges := deliveryByReceipt[rec.ID]
		if badges == nil {
			badges = []ReceiptDeliveryBadge{}
		}
		out[i] = ReceiptRow{
			FiscalReceipt:  rec,
			Delivery:       badges,
			NeedsAttention: receiptRowNeedsAttention(rec.Status, badges),
			TableLabel:     tableByBill[rec.BillID],
		}
	}

	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return &ReceiptsPage{
		Receipts:   out,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func receiptRowNeedsAttention(status database.FiscalStatus, badges []ReceiptDeliveryBadge) bool {
	switch status {
	case database.FiscalStatusFailedRetryable, database.FiscalStatusFailedPermanent:
		return true
	}
	for _, b := range badges {
		if b.Status == database.FiscalDeliveryStatusDead {
			return true
		}
	}
	return false
}

func normalizeReceiptTypes(receipts []database.FiscalReceipt) {
	for i := range receipts {
		receipts[i].ReceiptType = NormalizeStoredReceiptTypeForCountry(
			receipts[i].Country, receipts[i].ReceiptType,
		)
	}
}

func defaultActor(actor string) string {
	if actor == "" {
		return "system"
	}
	return actor
}
