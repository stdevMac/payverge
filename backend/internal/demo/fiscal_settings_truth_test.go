package demo

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

var cuitShape = regexp.MustCompile(`^\d{2}-\d{8}-\d$`)

// requireValidCUIT applies the AFIP mod-11 check digit rule the fiscal receiver
// enforces, so a seeded tax id that would be rejected at issuance fails here.
func requireValidCUIT(t *testing.T, cuit string) {
	t.Helper()
	require.Regexp(t, cuitShape, cuit, "CUIT must be rendered NN-NNNNNNNN-N")
	digits := strings.ReplaceAll(cuit, "-", "")
	weights := []int{5, 4, 3, 2, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i, w := range weights {
		sum += int(digits[i]-'0') * w
	}
	check := 11 - sum%11
	if check == 11 {
		check = 0
	}
	require.NotEqual(t, 10, check, "CUIT lands in the never-issued check-10 class")
	require.EqualValues(t, check, int(digits[10]-'0'), "CUIT check digit fails AFIP mod-11")
}

func demoFiscalRows(t *testing.T, db *gorm.DB, businessID uint) []database.BusinessFiscalSettings {
	t.Helper()
	var rows []database.BusinessFiscalSettings
	require.NoError(t, db.Where("business_id = ?", businessID).Order("id ASC").Find(&rows).Error)
	return rows
}

// #936: the AR showroom shipped one shared placeholder CUIT, a null point of
// sale, the Core annual fee on both plans, and a "D" counter prefix — a fiscal
// settings screen a prospect cannot believe.
func TestDemoFiscalAndPlanSettingsAreVenueTruthful(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-fiscal@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed-v1", BaselineDays: 2})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var seeded []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Order("id ASC").Find(&seeded).Error)
	require.Len(t, seeded, 2)

	taxIDs := map[string]bool{}
	for _, b := range seeded {
		rows := demoFiscalRows(t, db, b.ID)
		require.Len(t, rows, 1, "business %d must carry exactly one fiscal settings row", b.ID)
		fiscal := rows[0]

		require.Equal(t, "AR", fiscal.Country)
		require.Equal(t, "responsable_inscripto", fiscal.TaxCondition)
		require.NotEqual(t, "30-71000000-6", fiscal.TaxID, "shared placeholder CUIT")
		requireValidCUIT(t, fiscal.TaxID)
		taxIDs[fiscal.TaxID] = true
		require.NotNil(t, fiscal.PointOfSale, "AFIP issuance needs a punto de venta")
		require.Greater(t, *fiscal.PointOfSale, 0)

		require.True(t, b.TaxInclusive, "AR carta prices are IVA-final, so the check must say so")
		require.EqualValues(t, 0, b.TaxRate, "IVA-final prices must not add a tax line")
		require.NotEqual(t, "D", b.CounterPrefix)
	}
	require.Len(t, taxIDs, 2, "the two demo venues must file under distinct CUITs")

	// Counter tiles follow the prefix, so the placeholder must be gone there too.
	var counters []database.Counter
	require.NoError(t, db.Where("business_id = ?", seeded[0].ID).Order("counter_number ASC").Find(&counters).Error)
	require.NotEmpty(t, counters)
	for _, c := range counters {
		require.False(t, strings.HasPrefix(c.Name, "D1") || strings.HasPrefix(c.Name, "D2"),
			"counter %q still carries the placeholder prefix", c.Name)
		require.True(t, strings.HasPrefix(c.Name, seeded[0].CounterPrefix))
	}
}

// Long-lived demos never had their fiscal row rewritten: Attrs+FirstOrCreate
// only fills a row it creates. A same-seed-version ensure must repair it.
func TestEnsureRepairsStaleDemoFiscalSettings(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-fiscal-heal@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed-v1", BaselineDays: 2})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var seeded []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Order("id ASC").Find(&seeded).Error)
	require.Len(t, seeded, 2)

	// Recreate the live rows: shared placeholder CUIT, no point of sale, a
	// country the AR lookup key never matches.
	require.NoError(t, db.Model(&database.BusinessFiscalSettings{}).
		Where("business_id IN ?", []uint{seeded[0].ID, seeded[1].ID}).
		Updates(map[string]interface{}{
			"tax_id":        "30-71000000-6",
			"point_of_sale": nil,
			"country":       "",
		}).Error)
	require.NoError(t, db.Model(&database.Business{}).
		Where("demo_owner_user_id = ?", admin.ID).
		Updates(map[string]interface{}{
			"tax_inclusive":  false,
			"counter_prefix": "D",
		}).Error)

	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var after []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Order("id ASC").Find(&after).Error)
	taxIDs := map[string]bool{}
	for _, b := range after {
		rows := demoFiscalRows(t, db, b.ID)
		require.Len(t, rows, 1, "repair must rewrite the stale row, not add a second one")
		fiscal := rows[0]
		require.Equal(t, "AR", fiscal.Country)
		require.NotEqual(t, "30-71000000-6", fiscal.TaxID)
		requireValidCUIT(t, fiscal.TaxID)
		require.NotNil(t, fiscal.PointOfSale)
		taxIDs[fiscal.TaxID] = true

		require.True(t, b.TaxInclusive)
		require.NotEqual(t, "D", b.CounterPrefix)
	}
	require.Len(t, taxIDs, 2)

	var counters []database.Counter
	require.NoError(t, db.Where("business_id = ?", after[0].ID).Find(&counters).Error)
	require.NotEmpty(t, counters)
	for _, c := range counters {
		require.True(t, strings.HasPrefix(c.Name, after[0].CounterPrefix),
			"counter %q must follow the repaired prefix %q", c.Name, after[0].CounterPrefix)
	}
}
