package database

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrFiscalJobNotRequeueable = errors.New("fiscal job cannot be requeued in its current state")

// AdminFiscalSummary aggregates platform-wide fiscal compliance metrics.
type AdminFiscalSummary struct {
	BusinessesWithFiscal int64            `json:"businesses_with_fiscal"`
	JobsByStatus         map[string]int64 `json:"jobs_by_status"`
	ReceiptsByStatus     map[string]int64 `json:"receipts_by_status"`
	DueJobs              int64            `json:"due_jobs"`
	FailedRetryableJobs  int64            `json:"failed_retryable_jobs"`
	FailedPermanentJobs  int64            `json:"failed_permanent_jobs"`
}

// AdminFiscalJobRow is the admin fiscal queue projection: the job columns plus
// the business display name so operators never see raw "#1" / "#2" IDs alone.
type AdminFiscalJobRow struct {
	ID                   uint         `json:"id"`
	BusinessID           uint         `json:"business_id"`
	BusinessName         string       `json:"business_name"`
	SettingsID           uint         `json:"settings_id"`
	ReceiptID            *uint        `json:"receipt_id"`
	BillID               uint         `json:"bill_id"`
	PaymentID            *uint        `json:"payment_id"`
	AlternativePaymentID *uint        `json:"alternative_payment_id"`
	Action               string       `json:"action"`
	IdempotencyKey       string       `json:"idempotency_key"`
	CreditAmountCents    *int64       `json:"credit_amount_cents"`
	Status               FiscalStatus `json:"status"`
	Attempts             int          `json:"attempts"`
	MaxAttempts          int          `json:"max_attempts"`
	NextAttemptAt        *time.Time   `json:"next_attempt_at"`
	LastErrorCode        *string      `json:"last_error_code"`
	LastErrorMessage     *string      `json:"last_error_message"`
	LockedAt             *time.Time   `json:"locked_at"`
	LockedBy             *string      `json:"locked_by"`
	CreatedBy            string       `json:"created_by"`
	CreatedAt            time.Time    `json:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at"`
}

// AdminFiscalReceiptRow is the admin failed-receipt projection with business name.
type AdminFiscalReceiptRow struct {
	ID               uint         `json:"id"`
	BusinessID       uint         `json:"business_id"`
	BusinessName     string       `json:"business_name"`
	BillID           uint         `json:"bill_id"`
	Country          string       `json:"country"`
	Provider         string       `json:"provider"`
	ReceiptType      string       `json:"receipt_type"`
	ReceiptNumber    *string      `json:"receipt_number"`
	Status           FiscalStatus `json:"status"`
	ErrorCode        *string      `json:"error_code"`
	ErrorMessage     *string      `json:"error_message"`
	TotalAmountCents int64        `json:"total_amount_cents"`
	Currency         string       `json:"currency"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
}

// GetAdminFiscalSummary returns counts for the admin fiscal dashboard.
// All job/receipt counters are scoped to kind=real so demo/test AFIP data
// never inflates the production queue (Task 12 + Task 15).
func GetAdminFiscalSummary() (AdminFiscalSummary, error) {
	out := AdminFiscalSummary{
		JobsByStatus:     map[string]int64{},
		ReceiptsByStatus: map[string]int64{},
	}
	db := GetDB()
	if db == nil {
		return out, nil
	}

	// Scope to kind=real so demo/test fiscal settings never inflate the
	// production fiscal queue summary (Task 12 single registry).
	_ = db.Model(&BusinessFiscalSettings{}).
		Joins("JOIN businesses ON businesses.id = business_fiscal_settings.business_id").
		Where("business_fiscal_settings.mode <> ?", FiscalModeOff).
		Where("businesses.kind = ?", BusinessKindReal).
		Distinct("business_fiscal_settings.business_id").
		Count(&out.BusinessesWithFiscal).Error

	type statusCount struct {
		Status string
		Count  int64
	}
	var jobCounts []statusCount
	if err := db.Table("fiscal_jobs").
		Select("fiscal_jobs.status as status, COUNT(*) as count").
		Joins("JOIN businesses ON businesses.id = fiscal_jobs.business_id").
		Where("businesses.kind = ?", BusinessKindReal).
		Group("fiscal_jobs.status").
		Scan(&jobCounts).Error; err != nil {
		return out, err
	}
	for _, row := range jobCounts {
		out.JobsByStatus[row.Status] = row.Count
		if row.Status == string(FiscalStatusFailedRetryable) {
			out.FailedRetryableJobs = row.Count
		}
		if row.Status == string(FiscalStatusFailedPermanent) {
			out.FailedPermanentJobs = row.Count
		}
	}

	var receiptCounts []statusCount
	if err := db.Table("fiscal_receipts").
		Select("fiscal_receipts.status as status, COUNT(*) as count").
		Joins("JOIN businesses ON businesses.id = fiscal_receipts.business_id").
		Where("businesses.kind = ?", BusinessKindReal).
		Group("fiscal_receipts.status").
		Scan(&receiptCounts).Error; err != nil {
		return out, err
	}
	for _, row := range receiptCounts {
		out.ReceiptsByStatus[row.Status] = row.Count
	}

	now := time.Now().UTC()
	out.DueJobs, _ = CountDueFiscalJobsForKind(db, now, string(BusinessKindReal))
	return out, nil
}

// ListAdminFiscalJobsWithKind is the kind-aware list entry point.
// kind follows AdminBusinessFilter: real (default), demo, test, or all.
func ListAdminFiscalJobsWithKind(limit, offset int, status, kind string) ([]AdminFiscalJobRow, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	db := GetDB()
	if db == nil {
		return nil, 0, nil
	}

	base := db.Table("fiscal_jobs").
		Joins("JOIN businesses ON businesses.id = fiscal_jobs.business_id")
	k := AdminBusinessFilter{Kind: kind}.NormalizeKind()
	if k != AdminBusinessKindAll {
		base = base.Where("businesses.kind = ?", k)
	}
	if status != "" {
		base = base.Where("fiscal_jobs.status = ?", status)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []AdminFiscalJobRow
	err := base.
		Select("fiscal_jobs.*, businesses.name AS business_name").
		Order("fiscal_jobs.updated_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error
	return rows, total, err
}

// ListAdminFiscalReceiptsWithKind is the kind-aware receipt list entry point.
func ListAdminFiscalReceiptsWithKind(limit, offset int, status, kind string) ([]AdminFiscalReceiptRow, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	db := GetDB()
	if db == nil {
		return nil, 0, nil
	}

	base := db.Table("fiscal_receipts").
		Joins("JOIN businesses ON businesses.id = fiscal_receipts.business_id")
	k := AdminBusinessFilter{Kind: kind}.NormalizeKind()
	if k != AdminBusinessKindAll {
		base = base.Where("businesses.kind = ?", k)
	}
	if status != "" {
		base = base.Where("fiscal_receipts.status = ?", status)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []AdminFiscalReceiptRow
	err := base.
		Select("fiscal_receipts.id, fiscal_receipts.business_id, businesses.name AS business_name, fiscal_receipts.bill_id, fiscal_receipts.country, fiscal_receipts.provider, fiscal_receipts.receipt_type, fiscal_receipts.receipt_number, fiscal_receipts.status, fiscal_receipts.error_code, fiscal_receipts.error_message, fiscal_receipts.total_amount_cents, fiscal_receipts.currency, fiscal_receipts.created_at, fiscal_receipts.updated_at").
		Order("fiscal_receipts.updated_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error
	return rows, total, err
}

// CountDueFiscalJobsForKind counts claimable due jobs scoped by business kind.
// kind empty / real → production queue; all → unscoped (legacy helper parity).
func CountDueFiscalJobsForKind(db *gorm.DB, now time.Time, kind string) (int64, error) {
	if db == nil {
		return 0, nil
	}
	query := db.Table("fiscal_jobs").
		Joins("JOIN businesses ON businesses.id = fiscal_jobs.business_id").
		Where(
			"fiscal_jobs.status IN ? AND (fiscal_jobs.next_attempt_at IS NULL OR fiscal_jobs.next_attempt_at <= ?) AND fiscal_jobs.locked_at IS NULL",
			claimableFiscalJobStatuses,
			now,
		)
	k := AdminBusinessFilter{Kind: kind}.NormalizeKind()
	if k != AdminBusinessKindAll {
		query = query.Where("businesses.kind = ?", k)
	}
	var count int64
	err := query.Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

// AdminRequeueFiscalJob resets a failed job back to pending when it is not active or terminal-success.
// A failed_permanent job is requeued only when allowPermanent is true.
func AdminRequeueFiscalJob(id uint, allowPermanent bool) (*FiscalJob, error) {
	db := GetDB()
	var job FiscalJob
	if err := db.First(&job, id).Error; err != nil {
		return nil, err
	}
	if job.Status == FiscalStatusPending || job.LockedAt != nil {
		return nil, ErrFiscalJobNotRequeueable
	}
	if job.Status == FiscalStatusFailedPermanent && !allowPermanent {
		return nil, ErrFiscalJobNotRequeueable
	}
	if job.Status != FiscalStatusFailedRetryable && job.Status != FiscalStatusFailedPermanent {
		return nil, ErrFiscalJobNotRequeueable
	}
	now := time.Now().UTC()
	updates := map[string]any{
		"status":          FiscalStatusPending,
		"attempts":        0,
		"next_attempt_at": now,
		"locked_at":       nil,
		"locked_by":       nil,
		"updated_at":      now,
	}
	res := db.Model(&FiscalJob{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return GetFiscalJobByID(id)
}

// GetFiscalJobByID loads a single fiscal job.
func GetFiscalJobByID(id uint) (*FiscalJob, error) {
	var job FiscalJob
	err := GetDB().First(&job, id).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}
