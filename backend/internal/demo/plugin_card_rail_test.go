package demo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Demo venues must ship a configured card/local processor (Mercado Pago) as an
// enabled payment rail. USDC may stay available but must not be the only
// enabled payment category (#202).
func TestDemoEnablesMercadoPagoCardRail(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-plugins-mp@example.com")
	svc := NewService(db, Options{Now: func() time.Time { return fixedNow() }, SeedVersion: "plugin-mp-seed", BaselineDays: 3})
	instance, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	businessIDs := compactBusinessIDs(instance.PrimaryBusinessID, instance.SecondaryBusinessID)
	require.NotEmpty(t, businessIDs)

	for _, businessID := range businessIDs {
		var rows []database.BusinessPlugin
		require.NoError(t, db.Preload("Plugin").Where("business_id = ?", businessID).Find(&rows).Error)

		enabledPayment := map[string]bool{}
		for _, row := range rows {
			if row.Plugin.Category != database.PluginCategoryPayment {
				continue
			}
			if row.IsEnabled {
				enabledPayment[row.Plugin.Name] = true
			}
		}
		require.False(t, enabledPayment["mercadopago"],
			"business %d must not mark Mercado Pago Enabled without credential-shaped tokens", businessID)
		require.False(t, enabledPayment["usdc_payment"],
			"USDC must not be an enabled dinner rail on showcase demo")
		require.False(t, enabledPayment["cross_chain_payment"],
			"cross-chain must not be an enabled dinner rail on showcase demo")
		require.False(t, len(enabledPayment) == 1 && enabledPayment["usdc_payment"],
			"USDC must not be the sole enabled payment plugin")

		var biz database.Business
		require.NoError(t, db.Select("id", "settlement_addr", "is_demo").First(&biz, businessID).Error)
		require.True(t, biz.IsDemo)
		require.Empty(t, biz.SettlementAddr,
			"demo seed must not write a placeholder settlement address")
		if enabledPayment["usdc_payment"] || enabledPayment["cross_chain_payment"] {
			t.Fatalf("crypto plugin enabled on demo business %d with settlement %q", businessID, biz.SettlementAddr)
		}

		var names []string
		for _, row := range rows {
			if row.Plugin.Category == database.PluginCategoryPayment {
				names = append(names, row.Plugin.Name)
			}
		}
		require.Contains(t, names, "mercadopago", "Mercado Pago must remain listed and enableable")
	}
}
