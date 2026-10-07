package emails

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// Tenant outbound mail budget (open-source hardening, H-tenant-mail / H-relay).
//
// Any tenant — including a fresh self-serve signup or a public demo business —
// can make Payverge send mail to an address somebody else typed: a guest
// reservation confirmation, a "send me my receipt" request, a staff invite.
// Without a budget the platform's sending domain is a free relay for whoever
// signs up. This file is the ONE choke point that bounds it.
//
// What is counted: every send whose EmailServer carries an origin with a
// BusinessID (ForReservation / ForBill / ForBusiness /
// ForReservationRequest / ForReservationFollowUp), and every notification
// whose structs.Notification.MailOrigin names a business (the delivery mail
// to the address on a delivery order goes this way, through
// EmailServerDispatcher). System mail — password reset, verification, login
// codes, admin alerts — has no origin and keeps its own limits.
//
// Where it is enforced: EmailServer.dispatch, after the suppression check and
// before the outbox enqueue or the synchronous provider call. A failed
// synchronous send gives its claim back.
//
// Storage: the existing auth_attempts (principal, kind) counter table
// — the same windowed-counter ledger the auth throttle and
// PIN lockout use. No new table. Every bump is a single atomic
// INSERT ... ON CONFLICT ... RETURNING, so concurrent senders on several
// replicas cannot overshoot a cap.

// ErrTenantMailBudgetExceeded is returned (wrapped, with the scope that
// tripped) when a tenant-triggered send would exceed a configured cap.
var ErrTenantMailBudgetExceeded = errors.New("tenant email budget exceeded")

// ErrTenantMailDuplicate reports that the identical message already went to
// the same recipients inside the dedupe window, so nothing was sent this time.
// Dispatch returns it only for an origin that opted in with
// ReportingDuplicates (a send a person explicitly asked for); every other
// caller gets nil, because for scheduler and retry paths "already delivered"
// is success and an error would only provoke another attempt.
var ErrTenantMailDuplicate = errors.New("identical tenant email already sent inside the dedupe window")

// MailPurposeReservationRequest marks the confirmation/pending mail sent to the
// address a public guest typed into the booking form.
const MailPurposeReservationRequest = "reservation_request"

// MailPurposeGuestBookingFollowUp marks scheduler mail (reminder, approval
// timeout decline, automatic no-show) about a booking an anonymous guest made
// through the public form. Nobody on the venue side chose that recipient, so it
// shares the guest-booking lane with MailPurposeReservationRequest.
const MailPurposeGuestBookingFollowUp = "guest_booking_follow_up"

// MailPurposeGuestDeliveryRequest marks the "order received" confirmation sent
// to the address an anonymous guest typed into the public delivery checkout.
// Like the booking form, nobody on the venue side chose that recipient, so it
// shares the guest-booking lane. Later mail about the order (accepted, status,
// cancelled, paid) follows a venue action and is ordinary operator mail.
const MailPurposeGuestDeliveryRequest = "guest_delivery_request"

// MailPurposeReceipt marks a payment or fiscal receipt sent to the guest who
// paid. Receipts count against their own cross-tenant key per recipient
// (tenantMailKindReceiptRecipientGlobalDay, same cap as the all-purpose key)
// instead of the all-purpose one, so sybil businesses flooding an address with
// staff invites cannot use up the allowance that address's genuine receipts
// need, and receipt floods cannot use up its invites either.
const MailPurposeReceipt = "receipt"

// The guest-booking lane: mail whose recipient was typed by an anonymous
// public caller (the booking form and the follow-ups it schedules, and the
// public delivery checkout's order-received confirmation). It has its
// own budget so anonymous traffic can never use up the mail the venue itself
// sends (staff invites, receipts, fiscal receipts, staff-edited bookings):
//
//   - lane mail still counts against the business daily cap, but the lane may
//     only use 1/tenantMailGuestLaneShareDivisor of it (and never more than
//     EMAIL_TENANT_RESERVATION_DAILY_CAP), so operator mail always keeps the
//     rest;
//   - one venue may send lane mail to one address at most
//     EMAIL_TENANT_RESERVATION_RECIPIENT_DAILY_CAP times a day;
//   - all venues together may send lane mail to one address at most
//     EMAIL_TENANT_RECIPIENT_GLOBAL_DAILY_CAP times a day (the lane-only
//     cross-tenant key).
//
// Every tenant send, lane or not, also counts against one all-purpose
// cross-tenant key per recipient (EMAIL_TENANT_RECIPIENT_GLOBAL_ALL_DAILY_CAP,
// default 50). Without it, N sybil businesses could each mail one victim
// EMAIL_TENANT_RECIPIENT_DAILY_CAP times a day through staff invites or
// receipts. The default sits well above the per-business recipient cap, so a
// receipt or a staff invite is refused only when one address is being mailed
// by many venues on the same day. Receipts (MailPurposeReceipt) use a separate
// key with the same cap, so other tenant mail cannot crowd them out.
func isGuestBookingMailPurpose(purpose string) bool {
	switch purpose {
	case MailPurposeReservationRequest, MailPurposeGuestBookingFollowUp, MailPurposeGuestDeliveryRequest:
		return true
	default:
		return false
	}
}

// TenantMailTier selects which per-business daily cap applies.
type TenantMailTier string

const (
	TenantMailTierStandard TenantMailTier = "standard"
	TenantMailTierDemo     TenantMailTier = "demo"
)

// Ledger kinds stored in auth_attempts.kind. Principals never contain a raw
// address: recipients are reduced to a truncated SHA-256 of the lowercased
// address.
const (
	tenantMailKindBusinessDay                 = "tenant_mail_business_day"
	tenantMailKindRecipientDay                = "tenant_mail_recipient_day"
	tenantMailKindRecipientGlobalDay          = "tenant_mail_recipient_global_day"
	tenantMailKindLaneRecipientGlobalDay      = "tenant_mail_lane_recipient_global_day"
	tenantMailKindReceiptRecipientGlobalDay   = "tenant_mail_receipt_recipient_global_day"
	tenantMailKindReservationRequestDay       = "tenant_mail_reservation_request_day"
	tenantMailKindReservationRecipientDay     = "tenant_mail_reservation_request_recipient_day"
	tenantMailKindDedupe                      = "tenant_mail_dedupe"
	tenantMailDailyWindow                     = 24 * time.Hour
	tenantMailPurgeHorizon                    = 48 * time.Hour
	tenantMailMaxDedupeWindow                 = 24 * time.Hour
	defaultTenantMailBusinessDailyCap         = 1000
	defaultTenantMailDemoDailyCap             = 20
	defaultTenantMailRecipientDailyCap        = 20
	defaultTenantMailRecipientGlobalDailyCap  = 10
	defaultTenantMailRecipientGlobalAllCap    = 50
	defaultTenantMailReservationDailyCap      = 200
	defaultTenantMailReservationRecipientCap  = 3
	defaultTenantMailDedupeWindow             = 10 * time.Minute
	tenantMailHashHexLength                   = 32
	tenantMailGuestLaneShareDivisor           = 2
	tenantMailUnlimited                       = -1
	envTenantMailBusinessDailyCap             = "EMAIL_TENANT_DAILY_CAP"
	envTenantMailDemoDailyCap                 = "EMAIL_TENANT_DEMO_DAILY_CAP"
	envTenantMailRecipientDailyCap            = "EMAIL_TENANT_RECIPIENT_DAILY_CAP"
	envTenantMailRecipientGlobalDailyCap      = "EMAIL_TENANT_RECIPIENT_GLOBAL_DAILY_CAP"
	envTenantMailRecipientGlobalAllDailyCap   = "EMAIL_TENANT_RECIPIENT_GLOBAL_ALL_DAILY_CAP"
	envTenantMailReservationDailyCap          = "EMAIL_TENANT_RESERVATION_DAILY_CAP"
	envTenantMailReservationRecipientDailyCap = "EMAIL_TENANT_RESERVATION_RECIPIENT_DAILY_CAP"
	envTenantMailDedupeMinutes                = "EMAIL_TENANT_DEDUPE_MINUTES"
)

var tenantMailLedgerKinds = []string{
	tenantMailKindBusinessDay,
	tenantMailKindRecipientDay,
	tenantMailKindRecipientGlobalDay,
	tenantMailKindLaneRecipientGlobalDay,
	tenantMailKindReceiptRecipientGlobalDay,
	tenantMailKindReservationRequestDay,
	tenantMailKindReservationRecipientDay,
	tenantMailKindDedupe,
}

// TenantMailBudgetConfig holds the caps. For every cap: a positive value is the
// limit per 24h window, 0 blocks the scope entirely (a kill switch), and a
// negative value disables that cap. Each counter's window opens with its first
// counted send and resets 24 hours later (bumpTenantMailCounter); it is not a
// UTC calendar day and not a sliding window.
type TenantMailBudgetConfig struct {
	// BusinessDailyCap applies to every real (non-demo) business.
	BusinessDailyCap int
	// DemoBusinessDailyCap applies to is_demo or kind in (demo, test).
	DemoBusinessDailyCap int
	// RecipientDailyCap bounds one business mailing one address (any purpose).
	RecipientDailyCap int
	// RecipientGlobalDailyCap bounds guest-booking lane mail (public booking
	// form and its follow-ups) to one address from all tenants together. Only
	// lane mail is counted here; operator and receipt mail is bounded across
	// tenants only by the looser RecipientGlobalAllDailyCap.
	//
	// Accepted trade-off (review round 2, L1): the key is shared across
	// tenants on purpose. It is the platform-wide ceiling on anonymous
	// booking mail to one address, so booking a victim's address at N venues
	// cannot multiply it by N. The cost is that such bookings can use up the
	// address's allowance, and a later genuine booking-form auto-reply to it
	// is refused until the window resets. Only the email is skipped: the
	// booking stands and the venue still sees it. Public booking creation is
	// also per-IP limited (publicReservationLimiter in main.go).
	RecipientGlobalDailyCap int
	// RecipientGlobalAllDailyCap bounds every tenant send (any purpose) to one
	// address from all tenants together. It closes the sybil gap the
	// lane-only key leaves open: many businesses each mailing one victim up to
	// RecipientDailyCap times a day through staff invites or receipts.
	RecipientGlobalAllDailyCap int
	// ReservationRequestDailyCap bounds guest-booking lane mail per business.
	// The effective cap is also clamped to 1/tenantMailGuestLaneShareDivisor of
	// the business's tier cap, so the lane can never starve operator mail.
	ReservationRequestDailyCap int
	// ReservationRequestRecipientDailyCap bounds guest-booking lane mail from
	// one business to one address.
	ReservationRequestRecipientDailyCap int
	// DedupeWindow drops an identical message (same business, recipients and
	// rendered content) re-sent within the window. 0 disables dedupe.
	DedupeWindow time.Duration
}

// DefaultTenantMailBudgetConfig returns the built-in defaults.
func DefaultTenantMailBudgetConfig() TenantMailBudgetConfig {
	return TenantMailBudgetConfig{
		BusinessDailyCap:                    defaultTenantMailBusinessDailyCap,
		DemoBusinessDailyCap:                defaultTenantMailDemoDailyCap,
		RecipientDailyCap:                   defaultTenantMailRecipientDailyCap,
		RecipientGlobalDailyCap:             defaultTenantMailRecipientGlobalDailyCap,
		RecipientGlobalAllDailyCap:          defaultTenantMailRecipientGlobalAllCap,
		ReservationRequestDailyCap:          defaultTenantMailReservationDailyCap,
		ReservationRequestRecipientDailyCap: defaultTenantMailReservationRecipientCap,
		DedupeWindow:                        defaultTenantMailDedupeWindow,
	}
}

// TenantMailBudgetConfigFromEnv reads the EMAIL_TENANT_* variables over the
// defaults. A value that is not an integer keeps the default and logs a
// warning, so a typo can never silently lift a cap.
func TenantMailBudgetConfigFromEnv() TenantMailBudgetConfig {
	cfg := DefaultTenantMailBudgetConfig()
	cfg.BusinessDailyCap = tenantMailCapFromEnv(envTenantMailBusinessDailyCap, cfg.BusinessDailyCap)
	cfg.DemoBusinessDailyCap = tenantMailCapFromEnv(envTenantMailDemoDailyCap, cfg.DemoBusinessDailyCap)
	cfg.RecipientDailyCap = tenantMailCapFromEnv(envTenantMailRecipientDailyCap, cfg.RecipientDailyCap)
	cfg.RecipientGlobalDailyCap = tenantMailCapFromEnv(envTenantMailRecipientGlobalDailyCap, cfg.RecipientGlobalDailyCap)
	cfg.RecipientGlobalAllDailyCap = tenantMailCapFromEnv(envTenantMailRecipientGlobalAllDailyCap, cfg.RecipientGlobalAllDailyCap)
	cfg.ReservationRequestDailyCap = tenantMailCapFromEnv(envTenantMailReservationDailyCap, cfg.ReservationRequestDailyCap)
	cfg.ReservationRequestRecipientDailyCap = tenantMailCapFromEnv(envTenantMailReservationRecipientDailyCap, cfg.ReservationRequestRecipientDailyCap)
	minutes := tenantMailCapFromEnv(envTenantMailDedupeMinutes, int(defaultTenantMailDedupeWindow/time.Minute))
	if minutes <= 0 {
		cfg.DedupeWindow = 0
	} else {
		cfg.DedupeWindow = time.Duration(minutes) * time.Minute
	}
	return cfg
}

func tenantMailCapFromEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		logrus.WithField("env", key).Warn("invalid tenant email budget value; keeping the default")
		return fallback
	}
	if value < 0 {
		return tenantMailUnlimited
	}
	return value
}

// TenantMailClaimRequest describes one tenant-triggered send.
type TenantMailClaimRequest struct {
	BusinessID uint
	Purpose    string
	Recipients []string
	// ContentHash identifies the rendered message for the dedupe window. Empty
	// disables dedupe for this send.
	ContentHash string
}

// TenantMailClaim is the result of a successful Claim. Duplicate means the
// identical message was already sent inside the dedupe window: the caller must
// skip the send and report success.
type TenantMailClaim struct {
	Duplicate bool
	Tier      TenantMailTier
	counters  []tenantMailKey
	dedupe    *tenantMailKey
}

type tenantMailKey struct {
	principal string
	kind      string
}

// TenantMailBudget is the contract dispatch depends on.
type TenantMailBudget interface {
	Claim(ctx context.Context, req TenantMailClaimRequest) (TenantMailClaim, error)
	Release(ctx context.Context, claim TenantMailClaim) error
}

// GormTenantMailBudget stores the budget in auth_attempts.
type GormTenantMailBudget struct {
	db  *gorm.DB
	cfg TenantMailBudgetConfig
	now func() time.Time
	// tierOf resolves the business tier; overridable in tests.
	tierOf func(ctx context.Context, tx *gorm.DB, businessID uint) (TenantMailTier, error)
}

// NewGormTenantMailBudget binds the budget to db with cfg.
func NewGormTenantMailBudget(db *gorm.DB, cfg TenantMailBudgetConfig) *GormTenantMailBudget {
	if cfg.DedupeWindow > tenantMailMaxDedupeWindow {
		cfg.DedupeWindow = tenantMailMaxDedupeWindow
	}
	return &GormTenantMailBudget{
		db:     db,
		cfg:    cfg,
		now:    func() time.Time { return time.Now().UTC() },
		tierOf: lookupTenantMailTier,
	}
}

// Config returns the effective configuration.
func (b *GormTenantMailBudget) Config() TenantMailBudgetConfig { return b.cfg }

// lookupTenantMailTier reads the two columns that decide the tier. A missing
// business fails closed: a send that names a business that does not exist is a
// bug or an attack, never something to wave through uncounted.
func lookupTenantMailTier(ctx context.Context, tx *gorm.DB, businessID uint) (TenantMailTier, error) {
	var row struct {
		IsDemo bool
		Kind   string
	}
	err := tx.WithContext(ctx).Table("businesses").
		Select("is_demo, kind").
		Where("id = ?", businessID).
		Take(&row).Error
	if err != nil {
		return "", fmt.Errorf("tenant email budget: business %d lookup failed: %w", businessID, err)
	}
	return classifyTenantMailTier(row.IsDemo, row.Kind), nil
}

func classifyTenantMailTier(isDemo bool, kind string) TenantMailTier {
	switch database.BusinessKind(strings.TrimSpace(kind)) {
	case database.BusinessKindDemo, database.BusinessKindTest:
		return TenantMailTierDemo
	}
	if isDemo {
		return TenantMailTierDemo
	}
	return TenantMailTierStandard
}

// guestLaneCap is the effective per-business guest-booking lane cap: the
// configured cap clamped to a share of the tier cap. A negative configured cap
// only lifts the configured limit; the share still applies while the tier is
// capped. When the tier cap is unlimited the configured cap is used as is.
func (b *GormTenantMailBudget) guestLaneCap(tierCap int) int {
	laneCap := b.cfg.ReservationRequestDailyCap
	if tierCap < 0 {
		return laneCap
	}
	share := tierCap / tenantMailGuestLaneShareDivisor
	if laneCap < 0 || laneCap > share {
		return share
	}
	return laneCap
}

func (b *GormTenantMailBudget) businessCap(tier TenantMailTier) int {
	switch tier {
	case TenantMailTierDemo:
		return b.cfg.DemoBusinessDailyCap
	default:
		return b.cfg.BusinessDailyCap
	}
}

// Claim counts one tenant-triggered send against every applicable cap inside a
// single transaction. Any cap that would be exceeded rolls the whole claim
// back, so a refused send never consumes budget in another scope.
func (b *GormTenantMailBudget) Claim(ctx context.Context, req TenantMailClaimRequest) (TenantMailClaim, error) {
	if req.BusinessID == 0 {
		return TenantMailClaim{}, nil
	}
	if b == nil || b.db == nil {
		return TenantMailClaim{}, errors.New("tenant email budget: store not configured")
	}
	recipients := normalizedTenantMailRecipients(req.Recipients)
	now := b.now().UTC()
	biz := fmt.Sprintf("biz:%d", req.BusinessID)

	var claim TenantMailClaim
	err := b.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		claim = TenantMailClaim{}
		if b.cfg.DedupeWindow > 0 && strings.TrimSpace(req.ContentHash) != "" && len(recipients) > 0 {
			key := tenantMailKey{
				principal: biz + ":msg:" + tenantMailHash(strings.Join(recipients, "\n")+"\x00"+req.ContentHash),
				kind:      tenantMailKindDedupe,
			}
			count, err := bumpTenantMailCounter(tx, key, now, b.cfg.DedupeWindow)
			if err != nil {
				return err
			}
			if count > 1 {
				claim.Duplicate = true
				return nil
			}
			claim.dedupe = &key
		}

		tier, err := b.tierOf(ctx, tx, req.BusinessID)
		if err != nil {
			return err
		}
		claim.Tier = tier

		bump := func(key tenantMailKey, limit int, scope string) error {
			if limit < 0 {
				return nil
			}
			if limit == 0 {
				return fmt.Errorf("%w: %s", ErrTenantMailBudgetExceeded, scope)
			}
			count, err := bumpTenantMailCounter(tx, key, now, tenantMailDailyWindow)
			if err != nil {
				return err
			}
			if count > int64(limit) {
				return fmt.Errorf("%w: %s", ErrTenantMailBudgetExceeded, scope)
			}
			claim.counters = append(claim.counters, key)
			return nil
		}

		// Fixed order: business-wide first, recipients sorted. Every claim
		// locks rows in the same order, so concurrent claims cannot deadlock.
		// The only rows two businesses can share are the per-recipient
		// cross-tenant keys, taken in sorted recipient order, the all-purpose
		// key before the lane-only key.
		tierCap := b.businessCap(tier)
		if err := bump(tenantMailKey{biz, tenantMailKindBusinessDay}, tierCap, "business_daily_"+string(tier)); err != nil {
			return err
		}
		guestLane := isGuestBookingMailPurpose(req.Purpose)
		if guestLane {
			if err := bump(tenantMailKey{biz, tenantMailKindReservationRequestDay}, b.guestLaneCap(tierCap), "reservation_request_business_daily"); err != nil {
				return err
			}
		}
		for _, recipient := range recipients {
			to := "to:" + tenantMailHash(recipient)
			globalKind, globalScope := tenantMailKindRecipientGlobalDay, "recipient_global_all_daily"
			if req.Purpose == MailPurposeReceipt {
				globalKind, globalScope = tenantMailKindReceiptRecipientGlobalDay, "recipient_global_receipt_daily"
			}
			if err := bump(tenantMailKey{to, globalKind}, b.cfg.RecipientGlobalAllDailyCap, globalScope); err != nil {
				return err
			}
			if guestLane {
				if err := bump(tenantMailKey{to, tenantMailKindLaneRecipientGlobalDay}, b.cfg.RecipientGlobalDailyCap, "recipient_global_daily"); err != nil {
					return err
				}
			}
			if err := bump(tenantMailKey{biz + ":" + to, tenantMailKindRecipientDay}, b.cfg.RecipientDailyCap, "recipient_business_daily"); err != nil {
				return err
			}
			if guestLane {
				if err := bump(tenantMailKey{biz + ":" + to, tenantMailKindReservationRecipientDay}, b.cfg.ReservationRequestRecipientDailyCap, "reservation_request_recipient_daily"); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return TenantMailClaim{}, err
	}
	return claim, nil
}

// Release gives a claim back after the send it paid for failed, so a provider
// outage does not burn a tenant's daily budget.
func (b *GormTenantMailBudget) Release(ctx context.Context, claim TenantMailClaim) error {
	if b == nil || b.db == nil || claim.Duplicate || (len(claim.counters) == 0 && claim.dedupe == nil) {
		return nil
	}
	now := b.now().UTC()
	return b.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, key := range claim.counters {
			if err := tx.Exec(
				`UPDATE auth_attempts SET count = count - 1, updated_at = ? WHERE principal = ? AND kind = ? AND count > 0`,
				now, key.principal, key.kind,
			).Error; err != nil {
				return err
			}
		}
		if claim.dedupe != nil {
			if err := tx.Exec(
				`DELETE FROM auth_attempts WHERE principal = ? AND kind = ?`,
				claim.dedupe.principal, claim.dedupe.kind,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// PurgeExpired deletes ledger rows whose window ended long ago. auth_attempts
// has no janitor of its own; without this, every distinct recipient would
// leave a row behind forever.
func (b *GormTenantMailBudget) PurgeExpired(ctx context.Context) (int64, error) {
	if b == nil || b.db == nil {
		return 0, nil
	}
	cutoff := b.now().UTC().Add(-tenantMailPurgeHorizon)
	result := b.db.WithContext(ctx).Exec(
		`DELETE FROM auth_attempts WHERE kind IN ? AND window_started_at < ?`,
		tenantMailLedgerKinds, cutoff,
	)
	return result.RowsAffected, result.Error
}

// StartTenantMailBudgetJanitor purges expired ledger rows every interval until
// ctx is cancelled.
func StartTenantMailBudgetJanitor(ctx context.Context, budget *GormTenantMailBudget, interval time.Duration) {
	if budget == nil || interval <= 0 {
		return
	}
	logger.SafeGoNamed("tenant-mail-budget-janitor", func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := budget.PurgeExpired(ctx); err != nil {
					logrus.WithError(err).Warn("tenant email budget purge failed")
				} else if n > 0 {
					logrus.WithField("rows", n).Debug("tenant email budget purged expired rows")
				}
			}
		}
	})
}

// bumpTenantMailCounter increments (principal, kind) atomically, restarting
// the window when the stored one is older than window, and returns the new
// count.
func bumpTenantMailCounter(tx *gorm.DB, key tenantMailKey, now time.Time, window time.Duration) (int64, error) {
	cutoff := now.Add(-window)
	var count int64
	err := tx.Raw(`INSERT INTO auth_attempts (principal, kind, count, window_started_at, created_at, updated_at)
VALUES (?, ?, 1, ?, ?, ?)
ON CONFLICT (principal, kind) DO UPDATE SET
  count = CASE WHEN auth_attempts.window_started_at <= ? THEN 1 ELSE auth_attempts.count + 1 END,
  window_started_at = CASE WHEN auth_attempts.window_started_at <= ? THEN ? ELSE auth_attempts.window_started_at END,
  updated_at = ?
RETURNING count`,
		key.principal, key.kind, now, now, now,
		cutoff,
		cutoff, now,
		now,
	).Scan(&count).Error
	if err != nil {
		return 0, fmt.Errorf("tenant email budget: counter update failed: %w", err)
	}
	return count, nil
}

func normalizedTenantMailRecipients(recipients []string) []string {
	seen := make(map[string]struct{}, len(recipients))
	out := make([]string, 0, len(recipients))
	for _, raw := range recipients {
		recipient := strings.ToLower(strings.TrimSpace(raw))
		if recipient == "" {
			continue
		}
		if _, ok := seen[recipient]; ok {
			continue
		}
		seen[recipient] = struct{}{}
		out = append(out, recipient)
	}
	sort.Strings(out)
	return out
}

func tenantMailHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:tenantMailHashHexLength]
}

// tenantMailContentHash fingerprints the rendered message for dedupe. The
// idempotency key is deliberately excluded: it is time-based and differs on
// every call, which is exactly the repeat dedupe must catch.
func tenantMailContentHash(msg EmailMessage) string {
	h := sha256.New()
	write := func(s string) {
		_, _ = h.Write([]byte(s))
		_, _ = h.Write([]byte{0})
	}
	write(msg.TemplateName)
	write(msg.Subject)
	write(msg.HTMLBody)
	write(msg.TextBody)
	for _, attachment := range msg.Attachments {
		write(attachment.Filename)
		_, _ = h.Write(attachment.Content)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// SetTenantMailBudget enables the tenant outbound budget at the dispatch choke
// point. Called once at startup after migrations; nil disables it.
func (e *EmailServer) SetTenantMailBudget(budget TenantMailBudget) {
	e.tenantBudget = budget
}

// ForBusiness stamps a send triggered by a tenant action that has no narrower
// entity (staff invitations).
func (e *EmailServer) ForBusiness(businessID uint) *EmailServer {
	return e.WithOrigin(EmailOrigin{BusinessID: businessID})
}

// ForReservationRequest stamps the confirmation / pending mail sent to the
// address a public guest typed into the booking form. It is attributed like
// ForReservation and additionally counted against the reservation-request caps.
func (e *EmailServer) ForReservationRequest(businessID, reservationID uint) *EmailServer {
	return e.WithOrigin(EmailOrigin{BusinessID: businessID, ReservationID: reservationID, Purpose: MailPurposeReservationRequest})
}

// ReportingDuplicates makes dispatch return ErrTenantMailDuplicate instead of
// nil when the dedupe window swallows the send, so a user-initiated "send it
// again" endpoint can tell the person nothing new was mailed. The receiver's
// origin (business, entity, purpose) is kept.
func (e *EmailServer) ReportingDuplicates() *EmailServer {
	if e == nil {
		return nil
	}
	origin := e.origin
	origin.ReportDuplicate = true
	return e.WithOrigin(origin)
}

// ForReservationFollowUp stamps scheduler mail about an existing booking
// (reminder, approval-timeout decline, automatic no-show). A booking an
// anonymous guest made through the public form keeps its follow-ups in the
// guest-booking lane; a booking staff entered is operator mail.
func (e *EmailServer) ForReservationFollowUp(businessID, reservationID uint, createdBy string) *EmailServer {
	if ReservationMadeByGuest(createdBy) {
		return e.WithOrigin(EmailOrigin{BusinessID: businessID, ReservationID: reservationID, Purpose: MailPurposeGuestBookingFollowUp})
	}
	return e.ForReservation(businessID, reservationID)
}

// ReservationMadeByGuest reports whether table_reservations.created_by names
// the public booking form. Staff bookings record the actor ("staff:N",
// "owner:<addr>", a token type or "system"); the public form records
// "customer". Older rows ("guest", or empty) are treated as guest-made so they
// stay in the stricter lane.
func ReservationMadeByGuest(createdBy string) bool {
	switch strings.ToLower(strings.TrimSpace(createdBy)) {
	case "", "customer", "guest":
		return true
	default:
		return false
	}
}

// claimTenantMail is the dispatch hook. Unattributed (system) mail and a
// server without a budget pass straight through.
func (e *EmailServer) claimTenantMail(ctx context.Context, msg EmailMessage) (TenantMailClaim, error) {
	if e.tenantBudget == nil || e.origin.BusinessID == 0 {
		return TenantMailClaim{}, nil
	}
	claim, err := e.tenantBudget.Claim(ctx, TenantMailClaimRequest{
		BusinessID:  e.origin.BusinessID,
		Purpose:     e.origin.Purpose,
		Recipients:  msg.To,
		ContentHash: tenantMailContentHash(msg),
	})
	fields := e.origin.logFields()
	fields["template_name"] = msg.TemplateName
	if e.origin.Purpose != "" {
		fields["purpose"] = e.origin.Purpose
	}
	switch {
	case errors.Is(err, ErrTenantMailBudgetExceeded):
		logrus.WithFields(fields).WithError(err).Warn("tenant email budget exceeded; send refused")
	case err != nil:
		logrus.WithFields(fields).WithError(err).Error("tenant email budget check failed; send refused")
	case claim.Duplicate:
		logrus.WithFields(fields).Info("identical tenant email inside the dedupe window; send skipped")
	}
	return claim, err
}

// releaseTenantMail returns the claim after a failed send. Best-effort: a
// failed release only means the tenant lost one unit of budget.
func (e *EmailServer) releaseTenantMail(claim TenantMailClaim) {
	if e.tenantBudget == nil || claim.Duplicate || (len(claim.counters) == 0 && claim.dedupe == nil) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.tenantBudget.Release(ctx, claim); err != nil {
		logrus.WithFields(e.origin.logFields()).WithError(err).Warn("tenant email budget release failed")
	}
}
