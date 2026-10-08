package demo

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// featureIconCatalog mirrors FEATURE_ICON_KEYS in
// frontend/src/components/business-page/featureIcons.tsx. Anything outside this
// set renders as the generic fallback glyph, so the demo must not invent keys.
var featureIconCatalog = map[string]bool{
	"wifi": true, "car": true, "credit-card": true, "coffee": true,
	"music": true, "utensils": true, "shield": true, "heart": true,
	"star": true, "gift": true, "zap": true, "users": true,
	"clock": true, "map-pin": true, "phone": true, "award": true,
}

// unbackedClaimSubstrings are rails the demo venues cannot actually take: every
// crypto plugin ships disabled and the settlement wallet is wiped (#935).
var unbackedClaimSubstrings = []string{
	"cripto", "crypto", "on-chain", "onchain", "usdc", "blockchain",
	"wallet", "billetera", "zero-fee", "zero fee", "sin comisi",
}

func activeFeatures(t *testing.T, db *gorm.DB, businessID uint) []database.BusinessSpecialFeature {
	t.Helper()
	var rows []database.BusinessSpecialFeature
	require.NoError(t, db.Where("business_id = ? AND is_active = ?", businessID, true).
		Order("display_order ASC, id ASC").Find(&rows).Error)
	return rows
}

// #935: the storefront "about" tab sold crypto rails on both demo venues, and
// the Core venue also advertised AI it does not have.
func TestDemoSpecialFeaturesOnlySellBackedRails(t *testing.T) {
	for _, p := range profiles() {
		t.Run(p.Key, func(t *testing.T) {
			db := newDemoServiceTestDB(t)
			business := database.Business{Name: "Claim-safe demo"}
			require.NoError(t, db.Create(&business).Error)

			svc := NewService(db, Options{})
			require.NoError(t, svc.ensureBusinessPageData(context.Background(), db, business.ID, p))

			rows := activeFeatures(t, db, business.ID)
			require.Len(t, rows, 3, "every demo venue ships exactly three storefront claims")

			sawAI := false
			for _, row := range rows {
				blob := strings.ToLower(row.Title + " " + row.Description)
				for _, bad := range unbackedClaimSubstrings {
					require.NotContains(t, blob, bad,
						"feature %q sells a rail the venue cannot take", row.Title)
				}
				require.True(t, featureIconCatalog[row.Icon],
					"icon %q for feature %q is not in the frontend icon catalog", row.Icon, row.Title)
				if aiClaimPattern.MatchString(blob) {
					sawAI = true
				}
			}
			require.Equal(t, p.AIEnabled, sawAI,
				"AI claims must exist only on the AI-Pro venue (ai_enabled=%v)", p.AIEnabled)

			// The payments claim names the rails the demo actually settles on.
			var payments database.BusinessSpecialFeature
			require.NoError(t, db.Where("business_id = ? AND display_order = ?", business.ID, 2).
				First(&payments).Error)
			copy := strings.ToLower(payments.Title + " " + payments.Description)
			require.Contains(t, copy, "mercado pago")
			require.Contains(t, copy, "tarjeta")
			require.Contains(t, copy, "efectivo")
		})
	}
}

// PV-LIVE-20260720-003 healed only "zero-fee" wording; #935 needs the whole
// dishonest row retired, on venues that have been live for months.
func TestDemoSpecialFeaturesRepairStaleCryptoAndAIRows(t *testing.T) {
	db := newDemoServiceTestDB(t)
	business := database.Business{Name: "Stale claim demo"}
	require.NoError(t, db.Create(&business).Error)

	legacy := []database.BusinessSpecialFeature{
		{BusinessID: business.ID, Title: "Pedidos con QR", Description: "Cada mesa pide desde el celular.", Icon: "QrCode", DisplayOrder: 1, IsActive: true},
		{BusinessID: business.ID, Title: "Crypto payments", Description: "Zero-fee on-chain payments are enabled.", Icon: "Wallet", DisplayOrder: 2, IsActive: true},
		{BusinessID: business.ID, Title: "Atención con IA", Description: "Mozo, el asistente de la casa, responde preguntas.", Icon: "Sparkles", DisplayOrder: 3, IsActive: true},
		{BusinessID: business.ID, Title: "Pagos con USDC", Description: "Aceptamos USDC en la mesa.", Icon: "Wallet", DisplayOrder: 4, IsActive: true},
	}
	require.NoError(t, db.Create(&legacy).Error)

	svc := NewService(db, Options{})
	// Core profile: ai_enabled=false, so the AI claim must go too.
	require.NoError(t, svc.ensureBusinessPageData(context.Background(), db, business.ID, profiles()[0]))

	rows := activeFeatures(t, db, business.ID)
	require.Len(t, rows, 3)
	for _, row := range rows {
		blob := strings.ToLower(row.Title + " " + row.Description)
		for _, bad := range unbackedClaimSubstrings {
			require.NotContains(t, blob, bad)
		}
		require.False(t, aiClaimPattern.MatchString(blob),
			"Core venue must not advertise AI: %q", row.Title)
		require.True(t, featureIconCatalog[row.Icon], "icon %q not in catalog", row.Icon)
	}

	// The unbacked overflow row is retired, not deleted — operators can still
	// see what was there.
	var retired database.BusinessSpecialFeature
	require.NoError(t, db.Where("business_id = ? AND display_order = ?", business.ID, 4).First(&retired).Error)
	require.False(t, retired.IsActive)
	require.Equal(t, "Pagos con USDC", retired.Title)
}

func TestDemoSpecialFeaturesEnsureIsIdempotent(t *testing.T) {
	db := newDemoServiceTestDB(t)
	business := database.Business{Name: "Idempotent claims demo"}
	require.NoError(t, db.Create(&business).Error)

	svc := NewService(db, Options{})
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		require.NoError(t, svc.ensureBusinessPageData(ctx, db, business.ID, profiles()[1]))
	}

	var total int64
	require.NoError(t, db.Model(&database.BusinessSpecialFeature{}).
		Where("business_id = ?", business.ID).Count(&total).Error)
	require.EqualValues(t, 3, total, "re-ensure must not append duplicate feature rows")

	rows := activeFeatures(t, db, business.ID)
	require.Len(t, rows, 3)
	require.Equal(t, []int{1, 2, 3}, []int{rows[0].DisplayOrder, rows[1].DisplayOrder, rows[2].DisplayOrder})
}
