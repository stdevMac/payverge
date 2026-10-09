package demo

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// shippedBrandSecondaries mirrors frontend/src/lib/contrast.ts SHIPPED_BRAND_PRESETS
// secondary values. Demo seed secondary must be one of these so the guest
// storefront never fails the app's own AA validator (L6-28 / A-10).
// Ideal long-term: one shared constant list between Go seed and TS — do not
// hand-fork this map without updating contrast.ts.
var shippedBrandSecondaries = map[string]struct{}{
	"#2563eb": {}, // classicGray
	"#b45309": {}, // warmBrown
	"#15803d": {}, // forestGreen
	"#1d4ed8": {}, // oceanBlue
	"#7e22ce": {}, // purplePassion
	"#c2410c": {}, // sunsetOrange
	"#be185d": {}, // roseGold
	"#475569": {}, // midnight
}

func TestDemoSecondaryBrandColorIsShippedAAPreset(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "brand-color-admin@example.com")

	now := fixedNow()
	svc := NewService(db, Options{
		Now:          func() time.Time { return now },
		SeedVersion:  "brand-color-seed",
		BaselineDays: 1,
	})
	instance, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)
	require.NotNil(t, instance.PrimaryBusinessID)

	var business database.Business
	require.NoError(t, db.First(&business, *instance.PrimaryBusinessID).Error)

	secondary := strings.ToLower(strings.TrimSpace(business.DesignSettings.SecondaryColor))
	require.NotEqual(t, "#f59e0b", secondary,
		"amber #f59e0b fails WCAG AA secondary-on-white (contrast.ts)")
	_, ok := shippedBrandSecondaries[secondary]
	require.True(t, ok,
		"demo SecondaryColor %q must be a SHIPPED_BRAND_PRESETS secondary", secondary)
}
