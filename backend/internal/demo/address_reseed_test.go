package demo

import (
	"context"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// expectedDemoStreet maps a seeded demo business to its canonical per-venue
// street (venueIdentity): San Telmo for the core bodegón, Palermo for the
// ai_pro parrilla. Keyed on the stable internal BusinessId (not the address
// columns this test corrupts and heals).
func expectedDemoStreet(t *testing.T, b database.Business) (street, postal string) {
	t.Helper()
	if strings.HasSuffix(b.BusinessId, "-secondary") {
		return "Costa Rica 9602, Palermo", "C1414"
	}
	return "Defensa 9148, San Telmo", "C1065"
}

// expectedDemoOwner maps a seeded demo business to its canonical per-venue
// owner name (venueIdentity), keyed on the stable internal BusinessId.
func expectedDemoOwner(b database.Business) string {
	if strings.HasSuffix(b.BusinessId, "-secondary") {
		return "Ernesto Villalba"
	}
	return "Rosa Beltrán"
}

// Live issue #823: the storefront /b/<slug> printed one street while the table
// home /t/<code> printed another. Both public serializers read the SAME
// embedded businesses.street/city/... columns, so the code paths cannot
// diverge for one row — the drift is stale seed data: demo rows created by an
// older generator kept their original address forever because ensureBusiness's
// OnConflict DoUpdates never refreshed the address columns. A same-seed-version
// ensure (no wipe) must repair the canonical demo address, like it already
// repairs owner_name and dishonest feature copy.
func TestEnsureRepairsStaleDemoBusinessAddress(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-address@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed-v1", BaselineDays: 3})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var seeded []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Find(&seeded).Error)
	require.NotEmpty(t, seeded)
	streets := map[string]bool{}
	for _, b := range seeded {
		street, _ := expectedDemoStreet(t, b)
		require.Equal(t, street, b.Address.Street,
			"fresh seed must write the canonical demo address")
		streets[b.Address.Street] = true
	}
	require.Len(t, streets, 2, "the two demo venues must carry distinct addresses")

	// Simulate legacy prod rows behind #823: an older generator seeded the NY
	// showroom address and the row survived every ensure since.
	require.NoError(t, db.Model(&database.Business{}).
		Where("demo_owner_user_id = ?", admin.ID).
		Updates(map[string]interface{}{
			"street":      "16 Demo Market St",
			"city":        "New York",
			"state":       "NY",
			"postal_code": "10001",
			"country":     "US",
		}).Error)

	// Same seed version → ensure path, NOT a wipe+reseed. The stale address
	// must still self-heal so /b and /t agree again.
	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var after []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Find(&after).Error)
	require.NotEmpty(t, after)
	for _, b := range after {
		street, postal := expectedDemoStreet(t, b)
		require.Equalf(t, street, b.Address.Street,
			"ensure must repair a stale demo street on business %d (%s); storefront and table home render this same column", b.ID, b.CustomURL)
		require.Equalf(t, postal, b.Address.PostalCode,
			"ensure must repair a stale demo postal code on business %d (%s)", b.ID, b.CustomURL)
		require.Equal(t, "Buenos Aires", b.Address.City)
		require.Equal(t, "CABA", b.Address.State)
		require.Equal(t, "AR", b.Address.Country)
	}
}
