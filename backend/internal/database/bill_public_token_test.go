package database

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillCreate_GeneratesPublicToken(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 10, 5)
	bill := helperBill(t, biz, nil, 0)

	var saved Bill
	require.NoError(t, db.First(&saved, bill.ID).Error)
	require.NotEmpty(t, saved.PublicToken)
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{32}$`), saved.PublicToken, "token is 16 crypto/rand bytes hex-encoded")

	bill2 := helperBill(t, biz, nil, 0)
	var saved2 Bill
	require.NoError(t, db.First(&saved2, bill2.ID).Error)
	assert.NotEqual(t, saved.PublicToken, saved2.PublicToken)
}

func TestCreateBillTx_GeneratesPublicTokenWhenHooksAreDisabled(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 10, 5)
	bill := &Bill{
		BusinessID:     biz.ID,
		SettlementAddr: "0xSettle",
		TippingAddr:    "0xTip",
		Status:         BillStatusOpen,
	}

	// createBillTx is the canonical application path. It must not depend solely
	// on an ORM callback for this security capability: maintenance/import code
	// can intentionally disable callbacks while still using the helper.
	require.NoError(t, createBillTx(db.Session(&gorm.Session{SkipHooks: true}), bill, nil))
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{32}$`), bill.PublicToken)
}

func TestCreateBillTx_GeneratesUUIDBillNumberWhenEmpty(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 10, 5)

	bill := &Bill{
		BusinessID:     biz.ID,
		SettlementAddr: "0xSettle",
		TippingAddr:    "0xTip",
		Status:         BillStatusOpen,
	}
	require.NoError(t, CreateBill(bill, nil))
	require.True(t, strings.HasPrefix(bill.BillNumber, "B"), "bill number keeps operator-facing B{businessID}- prefix")
	// The UUID generator emits B{businessID}-{uuid[:12]}; a pure-unix-timestamp
	// suffix (10 digits) must never appear again.
	assert.NotRegexp(t, regexp.MustCompile(`^B\d+-\d{10}$`), bill.BillNumber)
}

func TestGuestResolvers_AcceptTokenRejectBillNumber(t *testing.T) {
	setupOrderTestDB(t)
	// GetBillByID preloads Payments/AlternativePayments; the shared order
	// harness omits those tables, so migrate them here (matches the pattern in
	// guest_plugin_checkout_access_test.go).
	require.NoError(t, db.AutoMigrate(&Payment{}, &AlternativePayment{}))
	biz := helperBusiness(t, 10, 5)
	bill := helperBill(t, biz, nil, 0)

	var saved Bill
	require.NoError(t, db.First(&saved, bill.ID).Error)
	require.NotEmpty(t, saved.PublicToken)

	// Token-only resolution on every guest resolver.
	byToken, _, err := GetPublicBillByToken(saved.PublicToken)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, byToken.ID)
	assert.Equal(t, saved.PublicToken, byToken.PublicToken, "public projection must include the token so guests can hold it")

	idByToken, err := GetGuestBillIDByToken(saved.PublicToken)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, idByToken)

	id, bizID, err := GetGuestBillScopeByToken(saved.PublicToken)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, id)
	assert.Equal(t, biz.ID, bizID)

	// Operator detail load is by bill id (GetBillByID), not a guest capability path.
	fullByID, _, err := GetBillByID(saved.ID)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, fullByID.ID)
	assert.Equal(t, saved.BillNumber, fullByID.BillNumber)

	// Guessable bill_number must NOT resolve on guest resolvers.
	_, _, err = GetPublicBillByToken(saved.BillNumber)
	require.ErrorIs(t, err, ErrPublicGuestBillNotFound)

	_, err = GetGuestBillIDByToken(saved.BillNumber)
	require.Error(t, err)

	_, _, err = GetGuestBillScopeByToken(saved.BillNumber)
	require.Error(t, err)
}

func TestGuestResolvers_UnknownIdentifierStillNotFound(t *testing.T) {
	setupOrderTestDB(t)
	_, _, err := GetPublicBillByToken("ffffffffffffffffffffffffffffffff")
	require.ErrorIs(t, err, ErrPublicGuestBillNotFound)
}

func TestGuestResolvers_EmptyTokenNotFound(t *testing.T) {
	setupOrderTestDB(t)
	_, _, err := GetPublicBillByToken("   ")
	require.ErrorIs(t, err, ErrPublicGuestBillNotFound)
	_, err = GetGuestBillIDByToken("")
	require.Error(t, err)
	_, _, err = GetGuestBillScopeByToken("\t")
	require.Error(t, err)
	_, _, err = GetGuestPluginCheckoutBillByToken("")
	require.Error(t, err)
}

// Split + plugin-checkout guest resolvers accept public_token only.
func TestSplitAndPluginCheckoutResolvers_AcceptTokenRejectBillNumber(t *testing.T) {
	dbw := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, dbw, "token-split-bill", 10000)

	var saved Bill
	require.NoError(t, db.First(&saved, bill.ID).Error)
	require.NotEmpty(t, saved.PublicToken)

	_, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         saved.ID,
		GuestSessionID: "guest-token-1",
		Mode:           BillSplitModeCustom,
		AmountCents:    2500,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)

	// split/state, split/events, split/execute resolver — token only.
	stateByToken, err := GetBillSplitStateByNumber(saved.PublicToken, now)
	require.NoError(t, err)
	assert.Equal(t, int64(2500), stateByToken.HeldCents)
	_, err = GetBillSplitStateByNumber(saved.BillNumber, now)
	require.Error(t, err, "bill_number must not resolve guest split state")

	// Internal bill-id path still works after server-side resolution.
	stateByID, err := GetBillSplitStateByBillID(saved.ID, now)
	require.NoError(t, err)
	assert.Equal(t, int64(2500), stateByID.HeldCents)

	// split/my-shares resolver.
	sharesByToken, err := GetGuestBillSplitSharesByNumber(saved.PublicToken, "guest-token-1", now)
	require.NoError(t, err)
	require.Len(t, sharesByToken, 1)
	_, err = GetGuestBillSplitSharesByNumber(saved.BillNumber, "guest-token-1", now)
	require.Error(t, err)

	// plugin-payment checkout resolver (TableID==0 skips the table read).
	pluginByToken, _, err := GetGuestPluginCheckoutBillByToken(saved.PublicToken)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, pluginByToken.ID)
	_, _, err = GetGuestPluginCheckoutBillByToken(saved.BillNumber)
	require.Error(t, err)

	// Unknown identifiers stay not-found on every resolver.
	const unknown = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	_, err = GetBillSplitStateByNumber(unknown, now)
	require.Error(t, err)
	_, err = GetGuestBillSplitSharesByNumber(unknown, "guest-token-1", now)
	require.Error(t, err)
	_, _, err = GetGuestPluginCheckoutBillByToken(unknown)
	require.Error(t, err)
}
