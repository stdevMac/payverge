package demo

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/s3"
)

const (
	// DefaultSeedVersion is stamped on every seeded demo business. Bump it
	// whenever the generated data changes: a mismatch forces existing demo
	// businesses to wipe and reseed onto the new data.
	DefaultSeedVersion = "admin-demo-v11"
	defaultTimezone    = "America/Argentina/Buenos_Aires"
	defaultBaseline    = 30
	defaultEmailDomain = "payverge.local"

	VerificationPassed = "passed"
	VerificationFailed = "failed"
	CoveragePassed     = "passed"
	CoverageFailed     = "failed"

	// MaxPayrollRevenueRatio caps demo payroll at 40% of paid revenue so the
	// cost-health surface cannot truthfully render 329% labor over fake data.
	MaxPayrollRevenueRatio = 0.40
	// TargetPayrollRevenueRatio is the seed target (under the cap).
	TargetPayrollRevenueRatio = 0.28
)

// demoAssetURL returns the public URL of an embedded demo asset (rel is
// relative to seedAssetPrefix, e.g. "carta/provoleta.jpg"). It is resolved at
// seed time through s3.PublicURL, so the same seed yields
// "<PUBLIC_URL>/media/demo-arg/assets/..." on local storage and
// "<S3_PUBLIC_BASE_URL>/demo-arg/assets/..." behind a CDN — never a
// third-party host.
func demoAssetURL(rel string) string {
	return s3.PublicURL(seedAssetPrefix + "/" + strings.TrimLeft(rel, "/"))
}

// isOwnHostedDemoAsset reports whether a URL already lives in our public
// store under the demo asset prefix — such URLs never need re-hosting.
func isOwnHostedDemoAsset(url string) bool {
	return strings.HasPrefix(url, demoAssetURL(""))
}

// demoWallet is a synthetic venue identifier for owner/tipping display only.
// It must never be written as a live USDC settlement target: 0x…dE01 is in
// the reserved low-address range and would burn guest funds on mainnet.
const demoWallet = "0x000000000000000000000000000000000000dE01"

// demoPayerPool is a fixed set of distinct synthetic guest payers. Each is a
// full 40-hex wallet so UI wallet truncation works, but none is the zero
// address that previously paid every bill.
var demoPayerPool = []string{
	"0x111111111111111111111111111111111111a001",
	"0x222222222222222222222222222222222222a002",
	"0x333333333333333333333333333333333333a003",
	"0x444444444444444444444444444444444444a004",
	"0x555555555555555555555555555555555555a005",
	"0x666666666666666666666666666666666666a006",
	"0x777777777777777777777777777777777777a007",
	"0x888888888888888888888888888888888888a008",
	"0x999999999999999999999999999999999999a009",
	"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa00a",
	"0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb00b",
	"0xccccccccccccccccccccccccccccccccccccc00c",
}

type Options struct {
	Now          func() time.Time
	SeedVersion  string
	BaselineDays int
	EmailDomain  string
	// AssetRehoster copies demo gallery photos into our own bucket before
	// seeding. Nil disables re-hosting and seeds the stock URLs as before.
	AssetRehoster AssetRehoster
	// ShowroomOwnerUserID, when non-zero, owns the seeded venues
	// (businesses.user_id) instead of the platform admin. The public demo
	// (DEMO_MODE) sets it to a non-admin account so one-click "enter as
	// owner" never hands out the admin; demo_owner_user_id stays the admin.
	ShowroomOwnerUserID uint
}

type Service struct {
	db            *gorm.DB
	now           func() time.Time
	seedVersion   string
	baselineDays  int
	emailDomain   string
	assetRehoster AssetRehoster
	showroomOwner uint
}

type VerificationResult struct {
	Status   string          `json:"status"`
	Errors   []string        `json:"errors"`
	Coverage []CoverageCheck `json:"coverage"`
}

type CoverageCheck struct {
	Key     string `json:"key"`
	Status  string `json:"status"`
	Count   int64  `json:"count"`
	Message string `json:"message,omitempty"`
}

type profile struct {
	Key              string
	BusinessIDSuffix string // internal BusinessId only — never used for public CustomURL
	Name             string
	AIEnabled        bool
	AverageBillCents int64
	ServiceFeeRate   float64
	Accent           string
	Hero             string
	// FiscalCUITBody / FiscalPointOfSale give each venue its own AR tax
	// identity instead of one shared placeholder (#936). The body is a FAKE
	// documentation number (12345678 / 23456789), never a real taxpayer's;
	// demoFiscalCUIT only adds a valid mod-11 check digit.
	FiscalCUITBody    int
	FiscalPointOfSale int
}

type billLine struct {
	MenuItemID string  `json:"menu_item_id"`
	Name       string  `json:"name"`
	Price      float64 `json:"price"`
	Quantity   int     `json:"quantity"`
	Subtotal   float64 `json:"subtotal"`
}

type totals struct {
	SubtotalCents int64
	TaxCents      int64
	ServiceCents  int64
	TotalCents    int64
}

func NewService(db *gorm.DB, opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	seed := opts.SeedVersion
	if seed == "" {
		seed = DefaultSeedVersion
	}
	days := opts.BaselineDays
	if days <= 0 {
		days = defaultBaseline
	}
	return &Service{
		db:            db,
		now:           now,
		seedVersion:   seed,
		baselineDays:  days,
		emailDomain:   normalizeEmailDomain(opts.EmailDomain),
		assetRehoster: opts.AssetRehoster,
		showroomOwner: opts.ShowroomOwnerUserID,
	}
}

// galleryImageSources is the ordered source list for one profile's demo
// gallery. Declared once so warming and seeding cannot drift apart.
func galleryImageSources(p profile) []string {
	return []string{p.Hero, menuImage("demo-parrillada"), menuImage("demo-provoleta")}
}

// warmDemoAssets re-hosts every distinct demo gallery photo BEFORE the seed
// transaction opens. Fetching a CDN image from inside a transaction that also
// writes 30 days of demo data would hold row locks open across the network.
//
// Failures are logged and dropped: seeding then falls back to the stock URL.
func (s *Service) warmDemoAssets(ctx context.Context) {
	if s.assetRehoster == nil {
		return
	}
	seen := map[string]bool{}
	for _, p := range profiles() {
		for _, src := range galleryImageSources(p) {
			// Own-hosted assets are already in our bucket — copying them into
			// itself would only waste startup time (and can 403 on itself).
			if src == "" || seen[src] || isOwnHostedDemoAsset(src) {
				continue
			}
			seen[src] = true
			if _, err := s.assetRehoster.Rehost(ctx, src); err != nil {
				logger.Logger.Warnf("demo: re-hosting gallery asset %s failed, falling back to the source URL: %v", src, err)
			}
		}
	}
}

// hostedAsset resolves a source URL to our own copy without touching the
// network, so it is safe to call inside a transaction.
func (s *Service) hostedAsset(sourceURL string) string {
	if s.assetRehoster == nil || isOwnHostedDemoAsset(sourceURL) {
		return sourceURL
	}
	if hosted, ok := s.assetRehoster.Hosted(sourceURL); ok && strings.TrimSpace(hosted) != "" {
		return hosted
	}
	return sourceURL
}

func profiles() []profile {
	return []profile{
		{
			Key:              "primary",
			BusinessIDSuffix: "primary",
			// Public CustomURL is allocated from Name only (see database.AllocatePublicCustomURL).
			Name:             "Bodegón Mesa Larga",
			AIEnabled:        false,
			AverageBillCents: 6000000,
			// AR carta prices are IVA-final and tips are voluntary — no forced
			// service fee line on the check.
			ServiceFeeRate:    0,
			Accent:            "#1a6b6a",
			Hero:              demoAssetURL("venues/bodegon-mesa-larga-hero.jpg"),
			FiscalCUITBody:    12345678,
			FiscalPointOfSale: 1,
		},
		{
			Key:               "secondary",
			BusinessIDSuffix:  "secondary",
			Name:              "Parrilla Quebracho Azul",
			AIEnabled:         true,
			AverageBillCents:  12000000,
			ServiceFeeRate:    0,
			Accent:            "#0f766e",
			Hero:              demoAssetURL("venues/parrilla-quebracho-azul-hero.jpg"),
			FiscalCUITBody:    23456789,
			FiscalPointOfSale: 2,
		},
	}
}

func (s *Service) EnsureForAdmin(ctx context.Context, adminUserID uint) (*database.DemoInstance, error) {
	if adminUserID == 0 {
		return nil, errors.New("admin user id required")
	}
	if err := s.assertAdmin(ctx, adminUserID); err != nil {
		return nil, err
	}

	loc, err := time.LoadLocation(defaultTimezone)
	if err != nil {
		return nil, err
	}
	start, end := baselineWindow(s.now(), loc, s.baselineDays)

	// Outside the transaction on purpose — see warmDemoAssets.
	s.warmDemoAssets(ctx)

	var instance database.DemoInstance
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		initial := database.DemoInstance{
			AdminUserID:       adminUserID,
			Status:            database.DemoInstanceStatusCreating,
			SeedVersion:       s.seedVersion,
			BaselineStartDate: start,
			Timezone:          defaultTimezone,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("admin_user_id = ?", adminUserID).First(&instance).Error; err != nil {
			return err
		}
		// An empty stored seed_version means a legacy instance from before
		// version stamping — treat it as a mismatch too, otherwise the bump
		// silently stamps the new version WITHOUT reseeding and permanently
		// defuses every future forced-regeneration bump.
		if instance.SeedVersion != s.seedVersion {
			if err := s.deleteOwnedDemoData(ctx, tx, adminUserID); err != nil {
				return err
			}
			if err := tx.Where("admin_user_id = ?", adminUserID).Delete(&database.DemoRun{}).Error; err != nil {
				return err
			}
			instance.PrimaryBusinessID = nil
			instance.SecondaryBusinessID = nil
			instance.Status = database.DemoInstanceStatusCreating
			instance.SeedVersion = s.seedVersion
			instance.BaselineStartDate = start
			instance.LastSimulatedBusinessDate = nil
			instance.LastError = ""
			if err := tx.Omit(clause.Associations).Save(&instance).Error; err != nil {
				return err
			}
		}
		run := s.newRun(&instance, database.DemoRunTypeEnsure, &start, &end)
		if err := tx.Create(run).Error; err != nil {
			return err
		}
		if err := s.ensureStaticBusinesses(ctx, tx, &instance); err != nil {
			return s.failRun(tx, &instance, run, err)
		}
		if instance.LastSimulatedBusinessDate == nil {
			if err := s.generateDays(ctx, tx, &instance, start, end); err != nil {
				return s.failRun(tx, &instance, run, err)
			}
			// The pointer records the last FULLY simulated day. Today is
			// still in progress (future-closing bills were skipped), so it
			// must stay revisitable by the hourly append.
			last := end
			if !last.Before(normalizeBusinessDate(s.now(), loc)) {
				last = last.AddDate(0, 0, -1)
			}
			instance.BaselineStartDate = start
			instance.LastSimulatedBusinessDate = &last
		}
		// Re-scale payroll after days exist so labor ≤ 40% of real revenue.
		if err := s.scaleAllBusinessPayroll(ctx, tx, &instance); err != nil {
			return s.failRun(tx, &instance, run, err)
		}
		instance.Status = database.DemoInstanceStatusReady
		instance.SeedVersion = s.seedVersion
		instance.LastError = ""
		if err := tx.Omit(clause.Associations).Save(&instance).Error; err != nil {
			return err
		}
		return s.succeedRun(tx, run, map[string]int64{"businesses": 2})
	})
	if err != nil {
		// The transaction rolled back, taking failRun's LastError with it.
		// Persist the failure outside the tx so operators can see WHY the
		// showroom is stale instead of a single startup log line.
		s.db.WithContext(ctx).Model(&database.DemoInstance{}).
			Where("admin_user_id = ?", adminUserID).
			Updates(map[string]interface{}{"status": database.DemoInstanceStatusFailed, "last_error": err.Error()})
		return nil, err
	}
	return &instance, nil
}

func (s *Service) EnsureAllAdmins(ctx context.Context) error {
	var admins []database.User
	if err := s.db.WithContext(ctx).Where("role = ?", "admin").Order("id ASC").Find(&admins).Error; err != nil {
		return err
	}
	var errs []error
	for _, admin := range admins {
		if _, err := s.EnsureForAdmin(ctx, admin.ID); err != nil {
			errs = append(errs, fmt.Errorf("admin %d: %w", admin.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Service) AppendDueDays(ctx context.Context) error {
	// Include `failed` instances, not just `ready`. A transient append/seed
	// error flips an instance to `failed` via failRun, and nothing else in the
	// hourly path ever re-readies it — so before this the showroom froze until
	// the next backend restart (only startup's EnsureForAdmin re-readied it).
	var instances []database.DemoInstance
	if err := s.db.WithContext(ctx).
		Where("status IN ?", []string{database.DemoInstanceStatusReady, database.DemoInstanceStatusFailed}).
		Order("id ASC").Find(&instances).Error; err != nil {
		return err
	}
	var errs []error
	for _, instance := range instances {
		// Self-heal: re-ready a failed instance once per tick via the same
		// recovery path startup uses. EnsureForAdmin is idempotent and runs in
		// its own locked transaction; if it fails again it re-marks the
		// instance failed and the next hourly tick retries — no tight loop.
		if instance.Status == database.DemoInstanceStatusFailed {
			if _, err := s.EnsureForAdmin(ctx, instance.AdminUserID); err != nil {
				errs = append(errs, fmt.Errorf("demo instance %d recover: %w", instance.ID, err))
				continue
			}
		}
		if err := s.appendDueDaysForInstance(ctx, instance.ID); err != nil {
			errs = append(errs, fmt.Errorf("demo instance %d: %w", instance.ID, err))
		}
	}
	// Self-heal: purge delivery tasks that the fiscal backfill sweep enqueued
	// for simulated (provider=demo) receipts before the sweep learned to skip
	// them — their dead artifact/email tasks flagged every demo invoice as
	// needs_attention. Non-fatal: a failed purge retries next tick.
	if _, err := s.cleanupSimulatedDeliveryTasks(ctx); err != nil {
		errs = append(errs, fmt.Errorf("demo delivery-task cleanup: %w", err))
	}
	return errors.Join(errs...)
}

// cleanupSimulatedDeliveryTasks deletes fiscal delivery tasks attached to
// provider=demo receipts. Demo receipts never get real artifact/email/print
// delivery (the worker short-circuits them), so any queued or dead task on
// them is noise that inflates needs_attention. Returns rows removed.
func (s *Service) cleanupSimulatedDeliveryTasks(ctx context.Context) (int64, error) {
	res := s.db.WithContext(ctx).
		Where("receipt_id IN (?)", s.db.Model(&database.FiscalReceipt{}).
			Select("id").Where("provider = ?", "demo")).
		Delete(&database.FiscalDeliveryTask{})
	return res.RowsAffected, res.Error
}

func (s *Service) ResetForAdmin(ctx context.Context, adminUserID uint) (*database.DemoInstance, error) {
	if err := s.assertAdmin(ctx, adminUserID); err != nil {
		return nil, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var instance database.DemoInstance
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("admin_user_id = ?", adminUserID).First(&instance).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		run := s.newRun(&instance, database.DemoRunTypeReset, nil, nil)
		if err := tx.Create(run).Error; err != nil {
			return err
		}
		instance.Status = database.DemoInstanceStatusResetting
		if err := tx.Omit(clause.Associations).Save(&instance).Error; err != nil {
			return err
		}
		if err := s.deleteOwnedDemoData(ctx, tx, adminUserID); err != nil {
			return s.failRun(tx, &instance, run, err)
		}
		if err := tx.Where("admin_user_id = ?", adminUserID).Delete(&database.DemoRun{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&instance).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.EnsureForAdmin(ctx, adminUserID)
}

func (s *Service) VerifyForAdmin(ctx context.Context, adminUserID uint) (VerificationResult, error) {
	var instance database.DemoInstance
	if err := s.db.WithContext(ctx).Where("admin_user_id = ?", adminUserID).First(&instance).Error; err != nil {
		return VerificationResult{}, err
	}
	result, err := s.VerifyInstance(ctx, instance.ID)
	if err != nil {
		return VerificationResult{}, err
	}
	return result, nil
}

func (s *Service) VerifyInstance(ctx context.Context, instanceID uint) (VerificationResult, error) {
	var instance database.DemoInstance
	if err := s.db.WithContext(ctx).First(&instance, instanceID).Error; err != nil {
		return VerificationResult{}, err
	}
	result := VerificationResult{Status: VerificationPassed, Errors: []string{}}
	businessIDs := compactBusinessIDs(instance.PrimaryBusinessID, instance.SecondaryBusinessID)
	if len(businessIDs) != 2 {
		result.Errors = append(result.Errors, "demo instance must reference both primary and secondary showroom businesses")
	}
	for _, businessID := range businessIDs {
		var business database.Business
		if err := s.db.WithContext(ctx).First(&business, businessID).Error; err != nil {
			return VerificationResult{}, err
		}
		if !business.IsDemo {
			result.Errors = append(result.Errors, fmt.Sprintf("business %d is not marked demo", business.ID))
		}
		if business.DemoOwnerUserID == nil || *business.DemoOwnerUserID != instance.AdminUserID {
			result.Errors = append(result.Errors, fmt.Sprintf("business %d has wrong demo owner", business.ID))
		}
	}
	result.Coverage = s.coverageChecks(ctx, businessIDs)
	for _, check := range result.Coverage {
		if check.Status != CoveragePassed {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", check.Key, check.Message))
		}
	}
	if err := s.verifyMoney(ctx, businessIDs, &result); err != nil {
		return VerificationResult{}, err
	}
	if err := s.verifyRecency(ctx, &instance, businessIDs, &result); err != nil {
		return VerificationResult{}, err
	}
	if len(result.Errors) > 0 {
		result.Status = VerificationFailed
	}
	now := s.now().UTC()
	_ = s.db.WithContext(ctx).Model(&database.DemoInstance{}).Where("id = ?", instance.ID).Update("last_verified_at", now).Error
	return result, nil
}

// verifyRecency fails loudly when the newest payment/bill close predates the
// claimed generated-window end. Without this the Demo Center reported
// "Verification passed, 0 errors" for three weeks after admin_demo_append died.
func (s *Service) verifyRecency(ctx context.Context, instance *database.DemoInstance, businessIDs []uint, result *VerificationResult) error {
	if len(businessIDs) == 0 || instance.LastSimulatedBusinessDate == nil {
		return nil
	}
	claimedEnd := s.recencyClaimedEnd(instance)
	// Load newest timestamps via ORDER BY rather than MAX()+Scan(*time.Time),
	// which SQLite returns as string and cannot scan into *time.Time.
	var newestPay database.Payment
	payErr := s.db.WithContext(ctx).
		Joins("JOIN bills ON bills.id = payments.bill_id").
		Where("bills.business_id IN ?", businessIDs).
		Order("COALESCE(payments.confirmed_at, payments.created_at) DESC").
		First(&newestPay).Error
	var newestBill database.Bill
	billErr := s.db.WithContext(ctx).
		Where("business_id IN ?", businessIDs).
		Order("COALESCE(closed_at, created_at) DESC").
		First(&newestBill).Error
	if errors.Is(payErr, gorm.ErrRecordNotFound) && errors.Is(billErr, gorm.ErrRecordNotFound) {
		result.Errors = append(result.Errors, "recency: no payments or closed bills in claimed demo window")
		return nil
	}
	if payErr != nil && !errors.Is(payErr, gorm.ErrRecordNotFound) {
		return payErr
	}
	if billErr != nil && !errors.Is(billErr, gorm.ErrRecordNotFound) {
		return billErr
	}
	var newest time.Time
	if !errors.Is(payErr, gorm.ErrRecordNotFound) {
		if newestPay.ConfirmedAt != nil {
			newest = *newestPay.ConfirmedAt
		} else {
			newest = newestPay.CreatedAt
		}
	}
	if !errors.Is(billErr, gorm.ErrRecordNotFound) {
		billAt := newestBill.CreatedAt
		if newestBill.ClosedAt != nil {
			billAt = *newestBill.ClosedAt
		}
		if newest.IsZero() || billAt.After(newest) {
			newest = billAt
		}
	}
	if newest.IsZero() {
		result.Errors = append(result.Errors, "recency: no payments or closed bills in claimed demo window")
		return nil
	}
	newestDay := normalizeBusinessDate(newest, claimedEnd.Location())
	// Fail when the newest activity is more than 1 calendar day before the
	// claimed last-simulated business date (the audit gap was 22 days).
	if newestDay.Before(claimedEnd.AddDate(0, 0, -1)) {
		result.Errors = append(result.Errors, fmt.Sprintf(
			"recency: newest payment/bill activity %s predates claimed generated-window end %s (data is stale)",
			newestDay.Format("2006-01-02"),
			claimedEnd.Format("2006-01-02"),
		))
	}
	return nil
}

// recencyClaimedEnd is the last fully simulated business day. Today is always
// in progress (future-closing bills are skipped), so a pointer that has already
// been stamped with "today" must not fail recency against yesterday's payments.
func (s *Service) recencyClaimedEnd(instance *database.DemoInstance) time.Time {
	loc := time.UTC
	if tz := strings.TrimSpace(instance.Timezone); tz != "" {
		if loaded, err := time.LoadLocation(tz); err == nil {
			loc = loaded
		}
	}
	claimed := storedBusinessDate(*instance.LastSimulatedBusinessDate, loc)
	today := normalizeBusinessDate(s.now(), loc)
	if !claimed.Before(today) {
		return today.AddDate(0, 0, -1)
	}
	return claimed
}

func (s *Service) assertAdmin(ctx context.Context, adminUserID uint) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&database.User{}).Where("id = ? AND role = ?", adminUserID, "admin").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("admin user %d not found", adminUserID)
	}
	return nil
}

func (s *Service) appendDueDaysForInstance(ctx context.Context, instanceID uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var instance database.DemoInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&instance, instanceID).Error; err != nil {
			return err
		}
		if instance.LastSimulatedBusinessDate == nil {
			return nil
		}
		loc, err := time.LoadLocation(instance.Timezone)
		if err != nil {
			return err
		}
		today := normalizeBusinessDate(s.now(), loc)
		next := storedBusinessDate(*instance.LastSimulatedBusinessDate, loc).AddDate(0, 0, 1)
		appended := false
		for !next.After(today) {
			run := s.newRun(&instance, database.DemoRunTypeAppendDay, &next, &next)
			if err := tx.Create(run).Error; err != nil {
				return err
			}
			if err := s.generateDays(ctx, tx, &instance, next, next); err != nil {
				return s.failRun(tx, &instance, run, err)
			}
			if err := s.succeedRun(tx, run, map[string]int64{"days": 1}); err != nil {
				return err
			}
			appended = true
			// Today is only partially simulated (bills that close later were
			// skipped) — leave the pointer at yesterday so the next hourly
			// append revisits it and materializes newly-closed bills.
			if next.Before(today) {
				last := next
				instance.LastSimulatedBusinessDate = &last
			}
			next = next.AddDate(0, 0, 1)
		}
		if appended {
			if err := s.scaleAllBusinessPayroll(ctx, tx, &instance); err != nil {
				return err
			}
		}
		// Re-arm even when no new day was generated. The pay-online fixture
		// uses a 15-minute window; hourly append is otherwise a no-op once
		// last_simulated is today, so coverage dies until the next reseed (#433).
		// Abandoned leftovers are not recycled — mint a successor (#772).
		if err := s.rearmAwaitingPaymentDeliveries(ctx, tx, &instance); err != nil {
			return err
		}
		// Sessions written before cash_sales was derived from the books keep
		// their fabricated cash forever, because the append pointer only ever
		// walks forward. Re-derive the closed history every cycle so caja can
		// never disagree with sales/accounting again (#796). `today` is
		// excluded, so a live open drawer is never rewritten.
		baseline := normalizeBusinessDate(instance.BaselineStartDate, loc)
		for _, businessID := range compactBusinessIDs(instance.PrimaryBusinessID, instance.SecondaryBusinessID) {
			if err := s.ReconcileCashSessions(ctx, tx, businessID, baseline, today); err != nil {
				return err
			}
			// A ticket abandoned on the public demo storefront occupies a table
			// until the next full reseed, and reseeds are rare. The hourly pass
			// is what keeps the showroom floor honest between them (#904).
			if err := s.retireGhostFloorTickets(ctx, tx, businessID); err != nil {
				return err
			}
		}
		return tx.Omit(clause.Associations).Save(&instance).Error
	})
}

func (s *Service) rearmAwaitingPaymentDeliveries(ctx context.Context, tx *gorm.DB, instance *database.DemoInstance) error {
	for _, p := range profiles() {
		var businessID uint
		switch p.Key {
		case "primary":
			if instance.PrimaryBusinessID == nil {
				continue
			}
			businessID = *instance.PrimaryBusinessID
		case "secondary":
			if instance.SecondaryBusinessID == nil {
				continue
			}
			businessID = *instance.SecondaryBusinessID
		default:
			continue
		}
		var staff []database.Staff
		if err := tx.WithContext(ctx).Where("business_id = ? AND is_active = ?", businessID, true).Order("id ASC").Find(&staff).Error; err != nil {
			return err
		}
		if err := s.ensureAwaitingPaymentDelivery(ctx, tx, businessID, p, staff); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) scaleAllBusinessPayroll(ctx context.Context, tx *gorm.DB, instance *database.DemoInstance) error {
	for _, businessID := range compactBusinessIDs(instance.PrimaryBusinessID, instance.SecondaryBusinessID) {
		var staff []database.Staff
		if err := tx.WithContext(ctx).Where("business_id = ? AND is_active = ?", businessID, true).Order("id ASC").Find(&staff).Error; err != nil {
			return err
		}
		if err := s.scalePayrollToRevenue(ctx, tx, businessID, staff); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) newRun(instance *database.DemoInstance, runType string, from, to *time.Time) *database.DemoRun {
	return &database.DemoRun{
		DemoInstanceID:   instance.ID,
		AdminUserID:      instance.AdminUserID,
		RunType:          runType,
		Status:           database.DemoRunStatusRunning,
		SeedVersion:      s.seedVersion,
		StartedAt:        s.now().UTC(),
		BusinessDateFrom: from,
		BusinessDateTo:   to,
		RecordsCreated:   database.JSONRawMessage(`{}`),
		Verification:     database.JSONRawMessage(`{}`),
	}
}

func (s *Service) succeedRun(tx *gorm.DB, run *database.DemoRun, records map[string]int64) error {
	now := s.now().UTC()
	raw, _ := json.Marshal(records)
	return tx.Model(run).Updates(map[string]interface{}{
		"status":          database.DemoRunStatusSucceeded,
		"finished_at":     now,
		"records_created": database.JSONRawMessage(raw),
		"error":           "",
	}).Error
}

func (s *Service) failRun(tx *gorm.DB, instance *database.DemoInstance, run *database.DemoRun, err error) error {
	now := s.now().UTC()
	_ = tx.Model(run).Updates(map[string]interface{}{
		"status":      database.DemoRunStatusFailed,
		"finished_at": now,
		"error":       err.Error(),
	}).Error
	_ = tx.Model(instance).Updates(map[string]interface{}{
		"status":     database.DemoInstanceStatusFailed,
		"last_error": err.Error(),
	}).Error
	return err
}

func normalizeBusinessDate(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// storedBusinessDate rebuilds a business-day midnight in loc from a persisted
// date value. Postgres `date` columns come back as midnight UTC while SQLite
// round-trips the original loc-midnight instant; in both cases the calendar
// date lives in the value's OWN location, so it must be read with t.Date()
// (never t.In(loc), which shifts a UTC-midnight value into the previous day
// for western timezones).
func storedBusinessDate(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func baselineWindow(now time.Time, loc *time.Location, days int) (time.Time, time.Time) {
	if days < 1 {
		days = 1
	}
	end := normalizeBusinessDate(now, loc)
	return end.AddDate(0, 0, -(days - 1)), end
}

func enumerateDays(start, end time.Time) []time.Time {
	var out []time.Time
	for cur := start; !cur.After(end); cur = cur.AddDate(0, 0, 1) {
		out = append(out, cur)
	}
	return out
}

func compactBusinessIDs(ids ...*uint) []uint {
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id != nil && *id != 0 {
			out = append(out, *id)
		}
	}
	return out
}

func (s *Service) rng(adminUserID uint, profileKey string, day time.Time, namespace string) *rand.Rand {
	key := fmt.Sprintf("%s:%d:%s:%s:%s", s.seedVersion, adminUserID, profileKey, day.Format("2006-01-02"), namespace)
	sum := sha256.Sum256([]byte(key))
	return rand.New(rand.NewSource(int64(binary.BigEndian.Uint64(sum[:8]))))
}

// demoStaffPIN derives a stable 4-digit PIN from staff identity, keyed with
// the instance's server secret (see demoPINKey) so it cannot be computed from
// the staff email alone.
func demoStaffPIN(email string) string {
	return keyedDemoPIN(demoPINKey(), email, 0)
}

// assignDemoStaffPINs returns pairwise-distinct PINs for the given emails.
// Assignment walks emails in sorted order so seed and summary re-derive
// identical values without storing plaintext.
func assignDemoStaffPINs(emails []string) map[string]string {
	key := demoPINKey()
	return assignDistinctPINs(emails, func(email string, attempt int) string {
		return keyedDemoPIN(key, email, attempt)
	})
}

// assignDistinctPINs walks emails in sorted order and gives each the first
// pinFor(email, attempt) value (attempt 0, 1, ...) not already taken.
func assignDistinctPINs(emails []string, pinFor func(email string, attempt int) string) map[string]string {
	sorted := append([]string(nil), emails...)
	// Insertion sort to avoid importing sort for a tiny roster.
	for i := 1; i < len(sorted); i++ {
		j := i
		for j > 0 && sorted[j] < sorted[j-1] {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
			j--
		}
	}
	used := map[string]struct{}{}
	out := make(map[string]string, len(emails))
	for _, email := range sorted {
		pin := pinFor(email, 0)
		if _, ok := used[pin]; ok {
			for i := 1; i < 10000; i++ {
				candidate := pinFor(email, i)
				if _, taken := used[candidate]; !taken {
					pin = candidate
					break
				}
			}
		}
		used[pin] = struct{}{}
		out[email] = pin
	}
	return out
}

// demoBillNumber returns a real-path-style bill number (B{id}-{12hex}) that is
// deterministic for idempotent re-runs and never carries a DEMO- prefix.
func demoBillNumber(businessID uint, parts ...interface{}) string {
	args := make([]interface{}, 0, len(parts)+2)
	args = append(args, "demo-bill", businessID)
	args = append(args, parts...)
	return fmt.Sprintf("B%d-%s", businessID, deterministicUUID(args...)[:12])
}

// demoSyntheticTxHash returns a non-explorer-linkable synthetic tx id.
// Payment History must not deep-link these to basescan.
func demoSyntheticTxHash(parts ...interface{}) string {
	return "demo_tx_" + deterministicUUID(parts...)
}

// demoPayerAddr picks a guest payer from the pool by stable index.
func demoPayerAddr(seed uint) string {
	if len(demoPayerPool) == 0 {
		return demoWallet
	}
	return demoPayerPool[int(seed)%len(demoPayerPool)]
}

func computeTotals(lines []billLine, taxRate, serviceRate float64) totals {
	var subtotal int64
	for _, line := range lines {
		if line.Quantity <= 0 || line.Price <= 0 {
			continue
		}
		subtotal += int64(math.Round(line.Price * 100 * float64(line.Quantity)))
	}
	tax := int64(math.Round(float64(subtotal) * taxRate / 100))
	service := int64(math.Round(float64(subtotal) * serviceRate / 100))
	return totals{SubtotalCents: subtotal, TaxCents: tax, ServiceCents: service, TotalCents: subtotal + tax + service}
}

func deterministicUUID(parts ...interface{}) string {
	sum := sha256.Sum256([]byte(fmt.Sprint(parts...)))
	return fmt.Sprintf("%x-%x-%02x%x-%02x%x-%x",
		sum[0:4],
		sum[4:6],
		(sum[6]&0x0f)|0x40,
		sum[7:8],
		(sum[8]&0x3f)|0x80,
		sum[9:10],
		sum[10:16],
	)
}

func normalizeEmailDomain(value string) string {
	domain := strings.ToLower(strings.TrimSpace(value))
	if at := strings.LastIndex(domain, "@"); at >= 0 {
		domain = domain[at+1:]
	}
	domain = strings.Trim(domain, ". @")
	if domain == "" || !strings.Contains(domain, ".") {
		return defaultEmailDomain
	}
	return domain
}

func (s *Service) demoEmail(local string) string {
	local = strings.Trim(strings.ToLower(local), " @")
	if local == "" {
		local = "demo"
	}
	return local + "@" + s.emailDomain
}

func (s *Service) demoCustomerEmail(adminUserID, businessID uint, idx int) string {
	return s.demoEmail(fmt.Sprintf("demo+admin%d-business%d-customer%d", adminUserID, businessID, idx))
}

func weekStartMonday(day time.Time) time.Time {
	offset := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -offset)
}
