package demo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// #856: the guest check carried a tipping_address — a synthetic 0x…dE01 wallet
// — on venues that cannot take a single on-chain payment (settlement wallet
// wiped, every crypto plugin disabled). A guest reading their bill was offered
// a crypto tip rail that goes nowhere. Empty is a first-class production state:
// the bill serializer omits the field entirely when it is blank.
func TestDemoVenuesOfferNoCryptoTippingWallet(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-tipping-wallet@example.com")
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "tipping-seed", BaselineDays: 4})
	instance, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	businessIDs := compactBusinessIDs(instance.PrimaryBusinessID, instance.SecondaryBusinessID)
	require.NotEmpty(t, businessIDs)

	for _, businessID := range businessIDs {
		var biz database.Business
		require.NoError(t, db.Select("id", "settlement_addr", "tipping_addr").First(&biz, businessID).Error)
		require.Empty(t, biz.SettlementAddr, "demo business %d must not seed a settlement wallet", businessID)
		require.Emptyf(t, biz.TippingAddr,
			"demo business %d offers tipping wallet %q it cannot receive on", businessID, biz.TippingAddr)

		var bills []database.Bill
		require.NoError(t, db.Where("business_id = ?", businessID).Find(&bills).Error)
		require.NotEmpty(t, bills, "demo business %d generated no bills to check", businessID)
		for _, bill := range bills {
			require.Emptyf(t, bill.TippingAddr,
				"bill %s shows guests a tipping wallet the venue cannot receive on", bill.BillNumber)
			require.Empty(t, bill.SettlementAddr, "bill %s must not carry a settlement wallet", bill.BillNumber)
		}
	}
}

// Demos seeded before this kept the wallet on the business row and on every
// historical bill; the same-seed-version ensure has to wipe them.
func TestEnsureWipesLegacyDemoTippingWallets(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-tipping-heal@example.com")
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "tipping-seed", BaselineDays: 4})
	instance, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	businessIDs := compactBusinessIDs(instance.PrimaryBusinessID, instance.SecondaryBusinessID)
	require.NotEmpty(t, businessIDs)

	legacyWallet := "0x000000000000000000000000000000000000dE01"
	require.NoError(t, db.Model(&database.Business{}).
		Where("id IN ?", businessIDs).Update("tipping_addr", legacyWallet).Error)
	require.NoError(t, db.Model(&database.Bill{}).
		Where("business_id IN ?", businessIDs).Update("tipping_addr", legacyWallet).Error)

	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	for _, businessID := range businessIDs {
		var biz database.Business
		require.NoError(t, db.Select("id", "tipping_addr").First(&biz, businessID).Error)
		require.Empty(t, biz.TippingAddr, "business %d kept a legacy tipping wallet", businessID)

		var stale int64
		require.NoError(t, db.Model(&database.Bill{}).
			Where("business_id = ? AND tipping_addr <> ?", businessID, "").Count(&stale).Error)
		require.Zerof(t, stale, "business %d kept %d bills with a legacy tipping wallet", businessID, stale)
	}
}
