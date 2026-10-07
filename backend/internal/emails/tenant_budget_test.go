package emails

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// tenantBudgetTestDB is a sqlite fixture with the two tables the budget reads:
// auth_attempts (the ledger, created from the model so the unique
// (principal, kind) index the upsert relies on exists) and a minimal
// businesses table holding only the tier columns.
func tenantBudgetTestDB(t testing.TB) *gorm.DB {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.Migrator().CreateTable(&database.AuthAttempt{}))
	require.NoError(t, db.Exec(`CREATE TABLE businesses (
		id INTEGER PRIMARY KEY,
		is_demo BOOLEAN NOT NULL DEFAULT 0,
		kind TEXT NOT NULL DEFAULT 'real'
	)`).Error)
	return db
}

func seedBudgetBusiness(t testing.TB, db *gorm.DB, id uint, isDemo bool, kind string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO businesses (id, is_demo, kind) VALUES (?, ?, ?)`,
		id, isDemo, kind,
	).Error)
}

// unlimitedBudgetConfig disables every cap and dedupe so a test can switch on
// exactly the one it exercises.
func unlimitedBudgetConfig() TenantMailBudgetConfig {
	return TenantMailBudgetConfig{
		BusinessDailyCap:                    -1,
		DemoBusinessDailyCap:                -1,
		RecipientDailyCap:                   -1,
		RecipientGlobalDailyCap:             -1,
		RecipientGlobalAllDailyCap:          -1,
		ReservationRequestDailyCap:          -1,
		ReservationRequestRecipientDailyCap: -1,
	}
}

type budgetClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *budgetClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *budgetClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestBudget(t testing.TB, db *gorm.DB, cfg TenantMailBudgetConfig) (*GormTenantMailBudget, *budgetClock) {
	t.Helper()
	clock := &budgetClock{now: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
	budget := NewGormTenantMailBudget(db, cfg)
	budget.now = clock.Now
	return budget, clock
}

func claimTo(budget *GormTenantMailBudget, businessID uint, purpose string, to ...string) (TenantMailClaim, error) {
	return budget.Claim(context.Background(), TenantMailClaimRequest{
		BusinessID: businessID,
		Purpose:    purpose,
		Recipients: to,
	})
}

func ledgerCount(t testing.TB, db *gorm.DB, kind string) int64 {
	t.Helper()
	var total int64
	require.NoError(t, db.Raw(`SELECT COALESCE(SUM(count), 0) FROM auth_attempts WHERE kind = ?`, kind).Scan(&total).Error)
	return total
}

func requireBudgetExceeded(t *testing.T, err error, scope string) {
	t.Helper()
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrTenantMailBudgetExceeded), "want ErrTenantMailBudgetExceeded, got %v", err)
	require.Contains(t, err.Error(), scope)
}

func TestTenantMailBudget_BusinessDailyCapBoundaryPerTier(t *testing.T) {
	cases := []struct {
		name   string
		isDemo bool
		kind   string
		tier   TenantMailTier
		cap    int
	}{
		{"real business uses the standard cap", false, "real", TenantMailTierStandard, 5},
		{"legacy is_demo flag uses the demo cap", true, "real", TenantMailTierDemo, 2},
		{"kind=demo uses the demo cap", false, "demo", TenantMailTierDemo, 2},
		{"kind=test uses the demo cap", false, "test", TenantMailTierDemo, 2},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := tenantBudgetTestDB(t)
			businessID := uint(100 + i)
			seedBudgetBusiness(t, db, businessID, tc.isDemo, tc.kind)
			cfg := unlimitedBudgetConfig()
			cfg.BusinessDailyCap = 5
			cfg.DemoBusinessDailyCap = 2
			budget, _ := newTestBudget(t, db, cfg)

			for n := 1; n <= tc.cap; n++ {
				claim, err := claimTo(budget, businessID, "", fmt.Sprintf("guest%d@example.test", n))
				require.NoError(t, err, "send %d of %d must fit the cap", n, tc.cap)
				require.Equal(t, tc.tier, claim.Tier)
			}
			_, err := claimTo(budget, businessID, "", "one-too-many@example.test")
			requireBudgetExceeded(t, err, "business_daily_"+string(tc.tier))
			require.EqualValues(t, tc.cap, ledgerCount(t, db, tenantMailKindBusinessDay),
				"the refused send must not stay counted")
		})
	}
}

func TestClassifyTenantMailTier(t *testing.T) {
	require.Equal(t, TenantMailTierStandard, classifyTenantMailTier(false, "real"))
	require.Equal(t, TenantMailTierDemo, classifyTenantMailTier(false, "demo"))
	require.Equal(t, TenantMailTierDemo, classifyTenantMailTier(false, "test"))
	require.Equal(t, TenantMailTierDemo, classifyTenantMailTier(true, "real"))
}

func TestTenantMailBudget_RecipientPerBusinessCapBoundary(t *testing.T) {
	db := tenantBudgetTestDB(t)
	seedBudgetBusiness(t, db, 1, false, "real")
	cfg := unlimitedBudgetConfig()
	cfg.RecipientDailyCap = 2
	budget, _ := newTestBudget(t, db, cfg)

	_, err := claimTo(budget, 1, "", "victim@example.test")
	require.NoError(t, err)
	_, err = claimTo(budget, 1, "", "  VICTIM@Example.test ")
	require.NoError(t, err, "second send to the same address (any casing) is still inside the cap")
	_, err = claimTo(budget, 1, "", "Victim@example.TEST")
	requireBudgetExceeded(t, err, "recipient_business_daily")

	_, err = claimTo(budget, 1, "", "someone-else@example.test")
	require.NoError(t, err, "a different recipient has its own counter")
}

// The global recipient key bounds guest-booking lane mail across tenants. It
// is the only cross-tenant key, and operator / receipt mail never touches it
// (review F2): a guest's receipt at one venue is never refused because other
// venues mailed the same address.
func TestTenantMailBudget_RecipientGlobalCapIsGuestLaneOnly(t *testing.T) {
	db := tenantBudgetTestDB(t)
	for id := uint(1); id <= 4; id++ {
		seedBudgetBusiness(t, db, id, false, "real")
	}
	cfg := unlimitedBudgetConfig()
	cfg.RecipientGlobalDailyCap = 2
	budget, _ := newTestBudget(t, db, cfg)

	_, err := claimTo(budget, 1, MailPurposeReservationRequest, "victim@example.test")
	require.NoError(t, err)
	_, err = claimTo(budget, 2, MailPurposeGuestBookingFollowUp, "victim@example.test")
	require.NoError(t, err, "follow-ups of anonymous bookings share the lane's global key")
	_, err = claimTo(budget, 3, MailPurposeReservationRequest, "victim@example.test")
	requireBudgetExceeded(t, err, "recipient_global_daily")

	for n := 0; n < 5; n++ {
		_, err = claimTo(budget, 4, "", "victim@example.test")
		require.NoError(t, err, "operator and receipt mail is never refused by other tenants' activity")
	}
	require.EqualValues(t, 2, ledgerCount(t, db, tenantMailKindLaneRecipientGlobalDay),
		"only guest-lane mail is written to the lane-only cross-tenant key")
}

// Follow-up F-1: every tenant send, not only guest-lane mail, counts against
// one all-purpose cross-tenant key per recipient, so sybil businesses cannot
// multiply the per-business recipient cap with staff invites or receipts.
func TestTenantMailBudget_RecipientGlobalAllCapBoundsSybilBusinesses(t *testing.T) {
	db := tenantBudgetTestDB(t)
	for id := uint(1); id <= 3; id++ {
		seedBudgetBusiness(t, db, id, false, "real")
	}
	def := DefaultTenantMailBudgetConfig()
	cfg := unlimitedBudgetConfig()
	cfg.RecipientDailyCap = def.RecipientDailyCap
	cfg.RecipientGlobalAllDailyCap = def.RecipientGlobalAllDailyCap
	budget, _ := newTestBudget(t, db, cfg)

	sent := 0
	var lastErr error
	for id := uint(1); id <= 3 && lastErr == nil; id++ {
		for n := 0; n < def.RecipientDailyCap; n++ {
			if _, err := claimTo(budget, id, "", "victim@example.test"); err != nil {
				lastErr = err
				break
			}
			sent++
		}
	}
	requireBudgetExceeded(t, lastErr, "recipient_global_all_daily")
	require.Equal(t, def.RecipientGlobalAllDailyCap, sent,
		"3 businesses x staff invites stop at the all-purpose cap, not at 3 x the per-business cap")
	require.EqualValues(t, def.RecipientGlobalAllDailyCap, ledgerCount(t, db, tenantMailKindRecipientGlobalDay))
	require.EqualValues(t, 0, ledgerCount(t, db, tenantMailKindLaneRecipientGlobalDay),
		"operator mail never touches the lane-only key")

	// Other addresses are unaffected.
	_, err := claimTo(budget, 1, "", "someone-else@example.test")
	require.NoError(t, err)
}

// Lane mail can never starve operator mail to the same address below the
// all-purpose cap: the lane-only key stops anonymous mail first.
func TestTenantMailBudget_LaneCannotExhaustAllPurposeRecipientCap(t *testing.T) {
	db := tenantBudgetTestDB(t)
	for id := uint(1); id <= 20; id++ {
		seedBudgetBusiness(t, db, id, false, "real")
	}
	def := DefaultTenantMailBudgetConfig()
	budget, _ := newTestBudget(t, db, def)

	lane := 0
	for id := uint(1); id <= 20; id++ {
		if _, err := claimTo(budget, id, MailPurposeReservationRequest, "guest@example.test"); err != nil {
			requireBudgetExceeded(t, err, "recipient_global_daily")
			break
		}
		lane++
	}
	require.Equal(t, def.RecipientGlobalDailyCap, lane)
	require.Less(t, def.RecipientGlobalDailyCap, def.RecipientGlobalAllDailyCap)

	// Operator mail to the same address (from a venue that sent no lane mail)
	// still has the rest of the all-purpose cap.
	for n := 0; n < def.RecipientDailyCap; n++ {
		_, err := claimTo(budget, 20, "", "guest@example.test")
		require.NoError(t, err)
	}
}

// Sybil businesses that use up an address's all-purpose cap with staff
// invites cannot block the receipts that address is owed, and receipts count
// against their own key with the same cap.
func TestTenantMailBudget_InviteFloodCannotStarveReceipts(t *testing.T) {
	db := tenantBudgetTestDB(t)
	for id := uint(1); id <= 4; id++ {
		seedBudgetBusiness(t, db, id, false, "real")
	}
	cfg := unlimitedBudgetConfig()
	cfg.RecipientGlobalAllDailyCap = 3
	budget, _ := newTestBudget(t, db, cfg)

	for id := uint(1); id <= 3; id++ {
		_, err := claimTo(budget, id, "", "guest@example.test")
		require.NoError(t, err)
	}
	_, err := claimTo(budget, 4, "", "guest@example.test")
	requireBudgetExceeded(t, err, "recipient_global_all_daily")

	for n := 0; n < 3; n++ {
		_, err := claimTo(budget, 4, MailPurposeReceipt, "guest@example.test")
		require.NoError(t, err, "receipt %d must not be refused by the invite flood", n+1)
	}
	_, err = claimTo(budget, 4, MailPurposeReceipt, "guest@example.test")
	requireBudgetExceeded(t, err, "recipient_global_receipt_daily")
	require.EqualValues(t, 3, ledgerCount(t, db, tenantMailKindReceiptRecipientGlobalDay))

	server := &EmailServer{}
	require.Equal(t, EmailOrigin{BusinessID: 4, BillID: 7, Purpose: MailPurposeReceipt}, server.ForReceipt(4, 7).origin)
}

func TestTenantMailBudget_ReservationRequestCaps(t *testing.T) {
	t.Run("per recipient is per business and includes follow-ups", func(t *testing.T) {
		db := tenantBudgetTestDB(t)
		for id := uint(1); id <= 2; id++ {
			seedBudgetBusiness(t, db, id, false, "real")
		}
		cfg := unlimitedBudgetConfig()
		cfg.ReservationRequestRecipientDailyCap = 3
		budget, _ := newTestBudget(t, db, cfg)

		_, err := claimTo(budget, 1, MailPurposeReservationRequest, "victim@example.test")
		require.NoError(t, err)
		_, err = claimTo(budget, 1, MailPurposeReservationRequest, "victim@example.test")
		require.NoError(t, err)
		_, err = claimTo(budget, 1, MailPurposeGuestBookingFollowUp, "victim@example.test")
		require.NoError(t, err, "the third lane mail (a follow-up) still fits the 3/day cap")
		_, err = claimTo(budget, 1, MailPurposeGuestBookingFollowUp, "victim@example.test")
		requireBudgetExceeded(t, err, "reservation_request_recipient_daily")
		_, err = claimTo(budget, 1, MailPurposeReservationRequest, "victim@example.test")
		requireBudgetExceeded(t, err, "reservation_request_recipient_daily")

		_, err = claimTo(budget, 2, MailPurposeReservationRequest, "victim@example.test")
		require.NoError(t, err, "venue B's booking confirmation is not refused because of venue A")
		_, err = claimTo(budget, 1, "", "victim@example.test")
		require.NoError(t, err, "operator mail is not counted against the booking-form cap")
	})

	t.Run("per business", func(t *testing.T) {
		db := tenantBudgetTestDB(t)
		seedBudgetBusiness(t, db, 1, false, "real")
		cfg := unlimitedBudgetConfig()
		cfg.ReservationRequestDailyCap = 2
		budget, _ := newTestBudget(t, db, cfg)

		_, err := claimTo(budget, 1, MailPurposeReservationRequest, "a@example.test")
		require.NoError(t, err)
		_, err = claimTo(budget, 1, MailPurposeReservationRequest, "b@example.test")
		require.NoError(t, err)
		_, err = claimTo(budget, 1, MailPurposeReservationRequest, "c@example.test")
		requireBudgetExceeded(t, err, "reservation_request_business_daily")

		_, err = claimTo(budget, 1, "", "c@example.test")
		require.NoError(t, err, "lifecycle mail for the same business is not blocked by the booking-form cap")
		require.EqualValues(t, 2, ledgerCount(t, db, tenantMailKindReservationRequestDay))
	})
}

func TestTenantMailBudget_ZeroBlocksAndNegativeIsUnlimited(t *testing.T) {
	db := tenantBudgetTestDB(t)
	seedBudgetBusiness(t, db, 1, false, "real")
	seedBudgetBusiness(t, db, 2, true, "demo")
	cfg := unlimitedBudgetConfig()
	cfg.DemoBusinessDailyCap = 0
	budget, _ := newTestBudget(t, db, cfg)

	_, err := claimTo(budget, 2, "", "guest@example.test")
	requireBudgetExceeded(t, err, "business_daily_demo")
	require.Zero(t, ledgerCount(t, db, tenantMailKindBusinessDay), "a zero cap refuses without writing")

	for n := 0; n < 50; n++ {
		_, err := claimTo(budget, 1, MailPurposeReservationRequest, "guest@example.test")
		require.NoError(t, err)
	}
	require.Zero(t, ledgerCount(t, db, tenantMailKindBusinessDay), "a disabled cap writes no ledger rows")
}

func TestTenantMailBudget_RefusalRollsBackEveryScope(t *testing.T) {
	db := tenantBudgetTestDB(t)
	seedBudgetBusiness(t, db, 1, false, "real")
	cfg := unlimitedBudgetConfig()
	cfg.BusinessDailyCap = 100
	cfg.ReservationRequestDailyCap = 100
	cfg.RecipientGlobalDailyCap = 100
	cfg.RecipientDailyCap = 1
	budget, _ := newTestBudget(t, db, cfg)

	_, err := claimTo(budget, 1, MailPurposeReservationRequest, "victim@example.test")
	require.NoError(t, err)
	_, err = claimTo(budget, 1, MailPurposeReservationRequest, "victim@example.test")
	requireBudgetExceeded(t, err, "recipient_business_daily")

	require.EqualValues(t, 1, ledgerCount(t, db, tenantMailKindBusinessDay),
		"the business counter bumped earlier in the refused claim must roll back")
	require.EqualValues(t, 1, ledgerCount(t, db, tenantMailKindReservationRequestDay),
		"the lane counter bumped earlier in the refused claim must roll back")
	require.EqualValues(t, 1, ledgerCount(t, db, tenantMailKindLaneRecipientGlobalDay),
		"the cross-tenant key bumped earlier in the refused claim must roll back")
}

// Review F1: anonymous booking-form mail and the follow-ups it schedules must
// never use up the budget the venue needs for its own mail. With the shipped
// defaults, flood the guest-booking lane of a standard and a demo
// venue until it is refused; operator mail must still have at least half the
// tier cap left.
func TestTenantMailBudget_GuestLaneCannotStarveOperatorMail(t *testing.T) {
	def := DefaultTenantMailBudgetConfig()
	cases := []struct {
		name     string
		isDemo   bool
		kind     string
		tier     TenantMailTier
		tierCap  int
		laneWant int
	}{
		{"standard", false, "real", TenantMailTierStandard, def.BusinessDailyCap, def.ReservationRequestDailyCap},
		{"demo", true, "demo", TenantMailTierDemo, def.DemoBusinessDailyCap, def.DemoBusinessDailyCap / tenantMailGuestLaneShareDivisor},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := tenantBudgetTestDB(t)
			businessID := uint(10 + i)
			seedBudgetBusiness(t, db, businessID, tc.isDemo, tc.kind)
			budget, _ := newTestBudget(t, db, def)

			lane := 0
			for ; lane <= tc.tierCap; lane++ {
				purpose := MailPurposeReservationRequest
				if lane%2 == 1 {
					purpose = MailPurposeGuestBookingFollowUp
				}
				_, err := claimTo(budget, businessID, purpose, fmt.Sprintf("anon%d@example.test", lane))
				if err != nil {
					requireBudgetExceeded(t, err, "reservation_request_business_daily")
					break
				}
			}
			require.Equal(t, tc.laneWant, lane, "the lane stops at its share, not at the tier cap")
			require.LessOrEqual(t, lane*tenantMailGuestLaneShareDivisor, tc.tierCap)

			// Operator mail (staff invite, receipt) still has the rest.
			operator := 0
			for ; operator <= tc.tierCap; operator++ {
				_, err := claimTo(budget, businessID, "", fmt.Sprintf("staff%d@example.test", operator))
				if err != nil {
					requireBudgetExceeded(t, err, "business_daily_"+string(tc.tier))
					break
				}
			}
			require.Equal(t, tc.tierCap-tc.laneWant, operator)
			require.GreaterOrEqual(t, operator, tc.tierCap/tenantMailGuestLaneShareDivisor)
		})
	}
}

func TestTenantMailBudget_GuestLaneCapClamp(t *testing.T) {
	budget := NewGormTenantMailBudget(nil, TenantMailBudgetConfig{ReservationRequestDailyCap: 200})
	require.Equal(t, 200, budget.guestLaneCap(1000), "the configured cap wins when it is under the share")
	require.Equal(t, 100, budget.guestLaneCap(200), "the share wins when it is under the configured cap")
	require.Equal(t, 0, budget.guestLaneCap(1), "a tiny tier cap closes the lane rather than handing it all")
	require.Equal(t, 200, budget.guestLaneCap(-1), "an unlimited tier keeps the configured cap")

	budget = NewGormTenantMailBudget(nil, TenantMailBudgetConfig{ReservationRequestDailyCap: -1})
	require.Equal(t, 500, budget.guestLaneCap(1000), "an unlimited lane cap is still clamped to the share")
	require.Equal(t, -1, budget.guestLaneCap(-1))
}

func TestTenantMailBudget_MultiRecipientChecksEveryAddress(t *testing.T) {
	db := tenantBudgetTestDB(t)
	seedBudgetBusiness(t, db, 1, false, "real")
	cfg := unlimitedBudgetConfig()
	cfg.RecipientDailyCap = 1
	budget, _ := newTestBudget(t, db, cfg)

	_, err := claimTo(budget, 1, "", "b@example.test")
	require.NoError(t, err)
	_, err = claimTo(budget, 1, "", "a@example.test", "b@example.test")
	requireBudgetExceeded(t, err, "recipient_business_daily")
	_, err = claimTo(budget, 1, "", "a@example.test")
	require.NoError(t, err, "the refused multi-recipient claim must not have counted a@")
}

func TestTenantMailBudget_WindowResetsAfterADay(t *testing.T) {
	db := tenantBudgetTestDB(t)
	seedBudgetBusiness(t, db, 1, false, "real")
	cfg := unlimitedBudgetConfig()
	cfg.BusinessDailyCap = 1
	budget, clock := newTestBudget(t, db, cfg)

	_, err := claimTo(budget, 1, "", "a@example.test")
	require.NoError(t, err)
	clock.Advance(23 * time.Hour)
	_, err = claimTo(budget, 1, "", "b@example.test")
	requireBudgetExceeded(t, err, "business_daily_standard")

	clock.Advance(time.Hour)
	_, err = claimTo(budget, 1, "", "c@example.test")
	require.NoError(t, err, "a full day after the window opened, the counter restarts")
}

// Review round 2, L2: the window opens with the first counted send. Crossing
// UTC midnight does not reset it, which is what .env.example now says.
func TestTenantMailBudget_WindowIsNotACalendarDay(t *testing.T) {
	db := tenantBudgetTestDB(t)
	seedBudgetBusiness(t, db, 1, false, "real")
	cfg := unlimitedBudgetConfig()
	cfg.BusinessDailyCap = 1
	budget, clock := newTestBudget(t, db, cfg)
	clock.now = time.Date(2026, 10, 3, 23, 30, 0, 0, time.UTC)

	_, err := claimTo(budget, 1, "", "a@example.test")
	require.NoError(t, err)
	clock.Advance(time.Hour) // 00:30 UTC the next calendar day
	_, err = claimTo(budget, 1, "", "b@example.test")
	requireBudgetExceeded(t, err, "business_daily_standard")
}

func TestTenantMailBudget_DedupeWindow(t *testing.T) {
	db := tenantBudgetTestDB(t)
	seedBudgetBusiness(t, db, 1, false, "real")
	cfg := unlimitedBudgetConfig()
	cfg.BusinessDailyCap = 100
	cfg.DedupeWindow = 10 * time.Minute
	budget, clock := newTestBudget(t, db, cfg)

	req := TenantMailClaimRequest{BusinessID: 1, Recipients: []string{"Guest@example.test"}, ContentHash: "hash-a"}
	first, err := budget.Claim(context.Background(), req)
	require.NoError(t, err)
	require.False(t, first.Duplicate)

	req.Recipients = []string{"guest@example.test"}
	second, err := budget.Claim(context.Background(), req)
	require.NoError(t, err)
	require.True(t, second.Duplicate, "same business, recipient and content inside the window is a duplicate")
	require.EqualValues(t, 1, ledgerCount(t, db, tenantMailKindBusinessDay), "a duplicate consumes no budget")

	other := req
	other.ContentHash = "hash-b"
	third, err := budget.Claim(context.Background(), other)
	require.NoError(t, err)
	require.False(t, third.Duplicate, "different content is not a duplicate")

	clock.Advance(10*time.Minute + time.Second)
	fourth, err := budget.Claim(context.Background(), req)
	require.NoError(t, err)
	require.False(t, fourth.Duplicate, "after the window the same message may be sent again")
}

func TestTenantMailBudget_ReleaseGivesBudgetBack(t *testing.T) {
	db := tenantBudgetTestDB(t)
	seedBudgetBusiness(t, db, 1, false, "real")
	cfg := unlimitedBudgetConfig()
	cfg.BusinessDailyCap = 1
	cfg.RecipientDailyCap = 1
	cfg.DedupeWindow = 10 * time.Minute
	budget, _ := newTestBudget(t, db, cfg)

	req := TenantMailClaimRequest{BusinessID: 1, Recipients: []string{"guest@example.test"}, ContentHash: "hash-a"}
	claim, err := budget.Claim(context.Background(), req)
	require.NoError(t, err)
	require.NoError(t, budget.Release(context.Background(), claim))
	require.Zero(t, ledgerCount(t, db, tenantMailKindBusinessDay))
	require.Zero(t, ledgerCount(t, db, tenantMailKindDedupe), "the dedupe marker is dropped so the retry is not swallowed")

	again, err := budget.Claim(context.Background(), req)
	require.NoError(t, err, "the released unit is available again")
	require.False(t, again.Duplicate)
}

func TestTenantMailBudget_UnknownBusinessFailsClosed(t *testing.T) {
	db := tenantBudgetTestDB(t)
	budget, _ := newTestBudget(t, db, DefaultTenantMailBudgetConfig())

	_, err := claimTo(budget, 404, "", "guest@example.test")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrTenantMailBudgetExceeded))
	require.Contains(t, err.Error(), "business 404 lookup failed")
}

func TestTenantMailBudget_UnattributedClaimIsFree(t *testing.T) {
	db := tenantBudgetTestDB(t)
	cfg := unlimitedBudgetConfig()
	cfg.BusinessDailyCap = 0
	budget, _ := newTestBudget(t, db, cfg)

	_, err := claimTo(budget, 0, "", "guest@example.test")
	require.NoError(t, err)
}

func TestTenantMailBudget_PurgeExpiredKeepsLiveAndForeignRows(t *testing.T) {
	db := tenantBudgetTestDB(t)
	seedBudgetBusiness(t, db, 1, false, "real")
	cfg := unlimitedBudgetConfig()
	cfg.BusinessDailyCap = 100
	budget, clock := newTestBudget(t, db, cfg)

	_, err := claimTo(budget, 1, "", "old@example.test")
	require.NoError(t, err)
	// An auth throttle row from the same era must survive the purge.
	require.NoError(t, db.Create(&database.AuthAttempt{
		Principal: "someone@example.test", Kind: "login_password", Count: 2, WindowStartedAt: clock.Now(),
	}).Error)

	clock.Advance(49 * time.Hour)
	_, err = claimTo(budget, 1, "", "new@example.test")
	require.NoError(t, err)

	purged, err := budget.PurgeExpired(context.Background())
	require.NoError(t, err)
	require.Zero(t, purged, "the business row was re-windowed by the new claim, nothing expired yet")

	clock.Advance(49 * time.Hour)
	purged, err = budget.PurgeExpired(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 1, purged)

	var foreign int64
	require.NoError(t, db.Model(&database.AuthAttempt{}).Where("kind = ?", "login_password").Count(&foreign).Error)
	require.EqualValues(t, 1, foreign, "purge must only touch tenant mail ledger kinds")
}

func TestTenantMailBudgetConfigFromEnv(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		for _, key := range []string{
			envTenantMailBusinessDailyCap, envTenantMailDemoDailyCap,
			envTenantMailRecipientDailyCap, envTenantMailRecipientGlobalDailyCap, envTenantMailRecipientGlobalAllDailyCap,
			envTenantMailReservationDailyCap, envTenantMailReservationRecipientDailyCap, envTenantMailDedupeMinutes,
		} {
			t.Setenv(key, "")
		}
		require.Equal(t, DefaultTenantMailBudgetConfig(), TenantMailBudgetConfigFromEnv())
		def := DefaultTenantMailBudgetConfig()
		require.Less(t, def.DemoBusinessDailyCap, def.BusinessDailyCap, "demo tenants must be stricter")
		require.Equal(t, 3, def.ReservationRequestRecipientDailyCap)
		require.Equal(t, 10, def.RecipientGlobalDailyCap)
		require.Equal(t, 50, def.RecipientGlobalAllDailyCap)
		require.Greater(t, def.RecipientGlobalAllDailyCap, def.RecipientDailyCap,
			"the all-purpose cross-tenant cap must sit above one venue's per-recipient cap")
		require.LessOrEqual(t, def.ReservationRequestDailyCap*tenantMailGuestLaneShareDivisor, def.BusinessDailyCap,
			"the default lane cap must leave operator mail at least its share")
	})

	t.Run("zero blocks the scope", func(t *testing.T) {
		t.Setenv(envTenantMailDemoDailyCap, "0")
		require.Equal(t, 0, TenantMailBudgetConfigFromEnv().DemoBusinessDailyCap, "0 is the explicit block-all value")
	})

	t.Run("overrides, invalid values and kill switches", func(t *testing.T) {
		t.Setenv(envTenantMailBusinessDailyCap, "250")
		t.Setenv(envTenantMailDemoDailyCap, "not-a-number")
		t.Setenv(envTenantMailRecipientDailyCap, "-5")
		t.Setenv(envTenantMailRecipientGlobalDailyCap, " 7 ")
		t.Setenv(envTenantMailRecipientGlobalAllDailyCap, "75")
		t.Setenv(envTenantMailReservationDailyCap, "")
		t.Setenv(envTenantMailReservationRecipientDailyCap, "1")
		t.Setenv(envTenantMailDedupeMinutes, "0")

		cfg := TenantMailBudgetConfigFromEnv()
		require.Equal(t, 250, cfg.BusinessDailyCap)
		require.Equal(t, defaultTenantMailDemoDailyCap, cfg.DemoBusinessDailyCap, "a typo keeps the default, never lifts the cap")
		require.Equal(t, tenantMailUnlimited, cfg.RecipientDailyCap)
		require.Equal(t, 7, cfg.RecipientGlobalDailyCap)
		require.Equal(t, 75, cfg.RecipientGlobalAllDailyCap)
		require.Equal(t, defaultTenantMailReservationDailyCap, cfg.ReservationRequestDailyCap)
		require.Equal(t, 1, cfg.ReservationRequestRecipientDailyCap)
		require.Zero(t, cfg.DedupeWindow)
	})

	t.Run("dedupe window is clamped", func(t *testing.T) {
		cfg := unlimitedBudgetConfig()
		cfg.DedupeWindow = 72 * time.Hour
		budget := NewGormTenantMailBudget(nil, cfg)
		require.Equal(t, tenantMailMaxDedupeWindow, budget.Config().DedupeWindow)
	})
}

// --- dispatch choke point -------------------------------------------------

type budgetProviderStub struct {
	mu    sync.Mutex
	fail  error
	calls int
	to    [][]string
}

func (p *budgetProviderStub) Send(_ context.Context, msg EmailMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.to = append(p.to, msg.To)
	return p.fail
}

func newBudgetTestServer(t testing.TB, provider EmailProvider, budget TenantMailBudget) *EmailServer {
	t.Helper()
	prev := EmailServerInstance
	t.Cleanup(func() { EmailServerInstance = prev })
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	templatesRoot := filepath.Join(filepath.Dir(currentFile), "..", "..", "email", "templates")
	server, err := NewEmailServer(provider, "noreply@payverge.io", "updates@payverge.io", templatesRoot)
	require.NoError(t, err)
	server.retryBackoff = 0
	server.maxSendAttempts = 1
	if budget != nil {
		server.SetTenantMailBudget(budget)
	}
	return server
}

func TestDispatchTenantBudget_CountsOnlyOriginStampedMail(t *testing.T) {
	db := tenantBudgetTestDB(t)
	seedBudgetBusiness(t, db, 7, false, "real")
	cfg := unlimitedBudgetConfig()
	cfg.BusinessDailyCap = 1
	budget, _ := newTestBudget(t, db, cfg)
	provider := &budgetProviderStub{}
	server := newBudgetTestServer(t, provider, budget)

	require.NoError(t, server.ForBusiness(7).SendCustomEmail([]string{"a@example.test"}, "Hello", "<p>one</p>", "one"))
	err := server.ForBill(7, 99).SendCustomEmail([]string{"b@example.test"}, "Hello", "<p>two</p>", "two")
	requireBudgetExceeded(t, err, "business_daily_standard")
	require.Equal(t, 1, provider.calls, "a refused send never reaches the provider")

	// System mail carries no origin and keeps its own limits.
	require.NoError(t, server.SendPasswordResetEmail([]string{"owner@example.test"}, "https://example.test/reset", "en"))
	require.Equal(t, 2, provider.calls)
	require.EqualValues(t, 1, ledgerCount(t, db, tenantMailKindBusinessDay))
}

func TestDispatchTenantBudget_DuplicateIsASilentNoOp(t *testing.T) {
	db := tenantBudgetTestDB(t)
	seedBudgetBusiness(t, db, 7, false, "real")
	cfg := unlimitedBudgetConfig()
	cfg.DedupeWindow = 10 * time.Minute
	budget, _ := newTestBudget(t, db, cfg)
	provider := &budgetProviderStub{}
	server := newBudgetTestServer(t, provider, budget)

	send := func() error {
		return server.ForBill(7, 1).SendCustomEmail([]string{"guest@example.test"}, "Your receipt", "<p>same</p>", "same")
	}
	require.NoError(t, send())
	require.NoError(t, send(), "a duplicate inside the window reports success")
	require.Equal(t, 1, provider.calls, "but is not delivered twice")
}

// A user-initiated "send it again" opts in with ReportingDuplicates and gets a
// distinguishable error instead of a silent success, while keeping the origin
// it was stamped with. Scheduler callers keep the silent no-op above.
func TestDispatchTenantBudget_ReportingDuplicatesSurfacesTheSkip(t *testing.T) {
	db := tenantBudgetTestDB(t)
	seedBudgetBusiness(t, db, 7, false, "real")
	cfg := unlimitedBudgetConfig()
	cfg.DedupeWindow = 10 * time.Minute
	budget, clock := newTestBudget(t, db, cfg)
	provider := &budgetProviderStub{}
	server := newBudgetTestServer(t, provider, budget)

	reporting := server.ForBill(7, 1).ReportingDuplicates()
	require.Equal(t, uint(7), reporting.origin.BusinessID, "the opt-in keeps the origin")
	require.Equal(t, uint(1), reporting.origin.BillID)
	require.False(t, server.ForBill(7, 1).origin.ReportDuplicate, "the opt-in does not leak into other sends")

	send := func() error {
		return reporting.SendCustomEmail([]string{"guest@example.test"}, "Your receipt", "<p>same</p>", "same")
	}
	require.NoError(t, send())
	err := send()
	require.ErrorIs(t, err, ErrTenantMailDuplicate)
	require.NotErrorIs(t, err, ErrTenantMailBudgetExceeded)
	require.Equal(t, 1, provider.calls, "the duplicate is still not delivered")

	// Different content is a new message, not a duplicate.
	require.NoError(t, reporting.SendCustomEmail([]string{"guest@example.test"}, "Your receipt", "<p>other</p>", "other"))
	require.Equal(t, 2, provider.calls)

	// Once the window has passed the same message goes out again.
	clock.Advance(cfg.DedupeWindow + time.Second)
	require.NoError(t, send())
	require.Equal(t, 3, provider.calls)

	var nilServer *EmailServer
	require.Nil(t, nilServer.ReportingDuplicates())
}

func TestDispatchTenantBudget_FailedSendReleasesClaim(t *testing.T) {
	db := tenantBudgetTestDB(t)
	seedBudgetBusiness(t, db, 7, false, "real")
	cfg := unlimitedBudgetConfig()
	cfg.BusinessDailyCap = 1
	cfg.DedupeWindow = 10 * time.Minute
	budget, _ := newTestBudget(t, db, cfg)
	provider := &budgetProviderStub{fail: errors.New("provider down")}
	server := newBudgetTestServer(t, provider, budget)

	send := func() error {
		return server.ForBill(7, 1).SendCustomEmail([]string{"guest@example.test"}, "Your receipt", "<p>r</p>", "r")
	}
	require.Error(t, send())
	require.Zero(t, ledgerCount(t, db, tenantMailKindBusinessDay), "a failed send must give its budget back")

	provider.fail = nil
	require.NoError(t, send(), "the retry is neither over budget nor swallowed as a duplicate")
	require.Equal(t, 2, provider.calls)
}

func TestDispatchTenantBudget_OutboxEnqueueCounts(t *testing.T) {
	db := tenantBudgetTestDB(t)
	require.NoError(t, db.Migrator().CreateTable(&EmailOutbox{}, &EmailOutboundSend{}))
	seedBudgetBusiness(t, db, 7, false, "real")
	cfg := unlimitedBudgetConfig()
	cfg.BusinessDailyCap = 1
	budget, _ := newTestBudget(t, db, cfg)
	provider := &budgetProviderStub{}
	server := newBudgetTestServer(t, provider, budget)
	server.SetOutboxStore(NewGormOutboxStore(db))

	require.NoError(t, server.ForBusiness(7).SendCustomEmail([]string{"a@example.test"}, "Hi", "<p>1</p>", "1"))
	err := server.ForBusiness(7).SendCustomEmail([]string{"b@example.test"}, "Hi", "<p>2</p>", "2")
	requireBudgetExceeded(t, err, "business_daily_standard")

	var queued int64
	require.NoError(t, db.Model(&EmailOutbox{}).Count(&queued).Error)
	require.EqualValues(t, 1, queued, "the refused message is never queued")
}

func TestForReservationRequestStampsPurposeAndKeepsBudget(t *testing.T) {
	budget := NewGormTenantMailBudget(nil, unlimitedBudgetConfig())
	server := &EmailServer{}
	server.SetTenantMailBudget(budget)

	scoped := server.ForReservationRequest(3, 9)
	require.Equal(t, EmailOrigin{BusinessID: 3, ReservationID: 9, Purpose: MailPurposeReservationRequest}, scoped.origin)
	require.Equal(t, budget, scoped.tenantBudget, "the per-call clone must keep the shared budget")
	require.Equal(t, EmailOrigin{BusinessID: 3}, server.ForBusiness(3).origin)
	require.Equal(t, EmailOrigin{}, server.origin, "the shared singleton is never mutated")
}

func TestForReservationFollowUpLane(t *testing.T) {
	server := &EmailServer{}
	require.Equal(t,
		EmailOrigin{BusinessID: 3, ReservationID: 9, Purpose: MailPurposeGuestBookingFollowUp},
		server.ForReservationFollowUp(3, 9, "customer").origin,
		"a public-form booking keeps its follow-ups in the guest-booking lane")
	require.Equal(t, EmailOrigin{BusinessID: 3, ReservationID: 9}, server.ForReservationFollowUp(3, 9, "staff:4").origin,
		"a staff-entered booking's follow-ups are operator mail")

	for _, createdBy := range []string{"customer", " Customer ", "guest", ""} {
		require.True(t, ReservationMadeByGuest(createdBy), createdBy)
	}
	for _, createdBy := range []string{"staff:4", "owner:0xabc", "system", "access"} {
		require.False(t, ReservationMadeByGuest(createdBy), createdBy)
	}
}

// Review F1 at the choke point: N anonymous bookings plus their follow-ups,
// then a staff invite and a receipt for the same venue still go out.
func TestDispatchTenantBudget_AnonymousBookingsLeaveOperatorMail(t *testing.T) {
	db := tenantBudgetTestDB(t)
	seedBudgetBusiness(t, db, 7, false, "real")
	cfg := DefaultTenantMailBudgetConfig()
	cfg.BusinessDailyCap = 10
	budget, _ := newTestBudget(t, db, cfg)
	provider := &budgetProviderStub{}
	server := newBudgetTestServer(t, provider, budget)

	refused := 0
	for n := uint(1); n <= 10; n++ {
		to := []string{fmt.Sprintf("anon%d@example.test", n)}
		if err := server.ForReservationRequest(7, n).SendCustomEmail(to, "Booking", "<p>booked</p>", "booked"); err != nil {
			requireBudgetExceeded(t, err, "reservation_request_business_daily")
			refused++
		}
		if err := server.ForReservationFollowUp(7, n, "customer").SendCustomEmail(to, "Reminder", "<p>soon</p>", "soon"); err != nil {
			requireBudgetExceeded(t, err, "reservation_request_business_daily")
			refused++
		}
	}
	require.Equal(t, 15, refused, "the lane gets half of the business cap of 10")

	require.NoError(t, server.ForBusiness(7).SendCustomEmail([]string{"new-staff@example.test"}, "Invite", "<p>join</p>", "join"),
		"the staff invite still goes out")
	require.NoError(t, server.ForBill(7, 1).SendCustomEmail([]string{"payer@example.test"}, "Receipt", "<p>paid</p>", "paid"),
		"the receipt still goes out")
}

// BenchmarkDispatchTenantBudget measures the cost the choke point adds to one
// tenant-stamped synchronous send (provider stubbed). "without" is the
// pre-change baseline shape: the same receipt send with no budget configured.
// "with" is an operator/receipt send (ForBill): dedupe, tier select, business
// day and per-business recipient. "with-booking-lane" is a public booking-form
// send (ForReservationRequest), which also takes the lane day, the cross-tenant
// recipient key and the lane recipient key.
func BenchmarkDispatchTenantBudget(b *testing.B) {
	for _, mode := range []string{"without", "with", "with-booking-lane"} {
		b.Run(mode, func(b *testing.B) {
			db := tenantBudgetTestDB(b)
			seedBudgetBusiness(b, db, 7, false, "real")
			var budget TenantMailBudget
			if mode != "without" {
				// Real caps high enough never to trip, so every ledger write
				// for the send's purpose happens on each iteration.
				cfg := DefaultTenantMailBudgetConfig()
				cfg.BusinessDailyCap = 1 << 30
				cfg.RecipientDailyCap = 1 << 30
				cfg.RecipientGlobalDailyCap = 1 << 30
				cfg.RecipientGlobalAllDailyCap = 1 << 30
				cfg.ReservationRequestDailyCap = 1 << 30
				cfg.ReservationRequestRecipientDailyCap = 1 << 30
				budget = NewGormTenantMailBudget(db, cfg)
			}
			server := newBudgetTestServer(b, &budgetProviderStub{}, budget)
			scoped := server.ForBill(7, 1)
			if mode == "with-booking-lane" {
				scoped = server.ForReservationRequest(7, 1)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				to := []string{fmt.Sprintf("guest%d@example.test", i)}
				if err := scoped.SendCustomEmail(to, "Your receipt", "<p>receipt body</p>", "receipt body"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
