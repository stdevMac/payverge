package demo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/reporting"
)

// A demo venue seeds no settlement wallet and no enabled payment plugin, so
// requireGuestCryptoPlugin rejects guest crypto with 422 plugin_unavailable and
// the public picker returns zero rails. Its own books must not then claim
// on-chain tenders it could never have taken (#795): every seeded payment
// settles on the counter card rail.
func TestDemoBillsNeverSettleOnARailTheVenueCannotHonor(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-tender-rail@example.com")
	svc := NewService(db, Options{Now: func() time.Time { return fixedNow() }, SeedVersion: "tender-rail-seed", BaselineDays: 6})
	instance, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	businessIDs := compactBusinessIDs(instance.PrimaryBusinessID, instance.SecondaryBusinessID)
	require.NotEmpty(t, businessIDs)

	for _, businessID := range businessIDs {
		// The premise: this venue cannot settle crypto.
		var biz database.Business
		require.NoError(t, db.Select("id", "settlement_addr").First(&biz, businessID).Error)
		require.Empty(t, biz.SettlementAddr, "demo business %d must not seed a settlement wallet", businessID)

		var cryptoRails int64
		require.NoError(t, db.Model(&database.BusinessPlugin{}).
			Joins("JOIN plugins ON plugins.id = business_plugins.plugin_id").
			Where("business_plugins.business_id = ? AND business_plugins.is_enabled = ? AND plugins.name IN ?",
				businessID, true, []string{"usdc_payment", "cross_chain_payment"}).
			Count(&cryptoRails).Error)
		require.Zero(t, cryptoRails, "demo business %d must not enable a crypto rail", businessID)

		// The books must agree with it.
		var payments []database.Payment
		require.NoError(t, db.
			Joins("JOIN bills ON bills.id = payments.bill_id").
			Where("bills.business_id = ?", businessID).
			Find(&payments).Error)
		require.NotEmpty(t, payments, "demo business %d generated no payments to check", businessID)

		for _, p := range payments {
			method := reporting.Canonicalize("payments", p.PaymentMethod)
			require.NotEqual(t, reporting.MethodCrypto, method,
				"payment %d on business %d claims a crypto tender the venue cannot take", p.ID, businessID)
			require.NotEqual(t, reporting.MethodCrossChain, method,
				"payment %d on business %d claims a cross-chain tender the venue cannot take", p.ID, businessID)
			require.Equal(t, reporting.MethodCard, method,
				"payment %d on business %d must settle on the counter card rail", p.ID, businessID)
			require.Empty(t, p.SettlementChain,
				"payment %d on business %d is not on-chain and must not carry a settlement chain", p.ID, businessID)
			// Empty payment_method canonicalizes to crypto for the payments
			// table — the rail has to be written, not left to the default.
			require.NotEmpty(t, p.PaymentMethod, "payment %d must name its rail explicitly", p.ID)
		}
	}
}
