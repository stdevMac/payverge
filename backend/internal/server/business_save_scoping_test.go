package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// UpdateBusinessExceptDesign must write every non-design column from the
// in-memory struct but leave the design_* columns untouched, so a concurrent
// design-settings save is never clobbered.
func TestUpdateBusinessExceptDesign_OmitsDesignColumns(t *testing.T) {
	setupStaffHandlerTestDB(t)
	business := createBusinessHandlerTestBusiness(t, "0xOwnerScopeA", "scope-omit")

	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Update("design_primary_color", "#abcabc").Error)

	loaded, err := database.GetBusinessByID(business.ID)
	require.NoError(t, err)
	loaded.Description = "NEWDESC"
	loaded.DesignSettings.PrimaryColor = "#999999" // must NOT be persisted by this helper

	require.NoError(t, database.UpdateBusinessExceptDesign(loaded))

	reloaded, err := database.GetBusinessByID(business.ID)
	require.NoError(t, err)
	assert.Equal(t, "NEWDESC", reloaded.Description, "non-design column should be written")
	assert.Equal(t, "#abcabc", reloaded.DesignSettings.PrimaryColor, "design columns must be omitted")
}

// UpdateBusinessDesignOnly must write only the design_* columns and leave every
// other column untouched.
func TestUpdateBusinessDesignOnly_WritesOnlyDesignColumns(t *testing.T) {
	setupStaffHandlerTestDB(t)
	business := createBusinessHandlerTestBusiness(t, "0xOwnerScopeB", "scope-design")

	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Update("description", "KEEPME").Error)

	loaded, err := database.GetBusinessByID(business.ID)
	require.NoError(t, err)
	loaded.Description = "SHOULD_NOT_PERSIST"
	loaded.DesignSettings.PrimaryColor = "#123456"

	require.NoError(t, database.UpdateBusinessDesignOnly(loaded))

	reloaded, err := database.GetBusinessByID(business.ID)
	require.NoError(t, err)
	assert.Equal(t, "KEEPME", reloaded.Description, "non-design column must be left untouched")
	assert.Equal(t, "#123456", reloaded.DesignSettings.PrimaryColor)
}

// Zero values (false bool, empty string) on selected columns must persist.
func TestUpdateBusinessExceptDesign_WritesZeroValues(t *testing.T) {
	setupStaffHandlerTestDB(t)
	business := createBusinessHandlerTestBusiness(t, "0xOwnerScopeC", "scope-zero")

	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Updates(map[string]any{"show_reviews": true, "description": "X"}).Error)

	loaded, err := database.GetBusinessByID(business.ID)
	require.NoError(t, err)
	loaded.ShowReviews = false
	loaded.Description = ""

	require.NoError(t, database.UpdateBusinessExceptDesign(loaded))

	reloaded, err := database.GetBusinessByID(business.ID)
	require.NoError(t, err)
	assert.False(t, reloaded.ShowReviews, "false must be persisted")
	assert.Equal(t, "", reloaded.Description, "empty string must be persisted")
}

// The editor's save batch fires updateBusiness + updateBusinessDesignSettings
// concurrently; both handlers load the full row then persist. With full-row
// db.Save the second writer clobbers the first. This simulates the race
// deterministically: two independent loads, each persisting a disjoint column
// via the scoped helpers — both edits must survive.
func TestSaveBatch_DesignAndBusinessEditsDoNotClobber(t *testing.T) {
	setupStaffHandlerTestDB(t)
	business := createBusinessHandlerTestBusiness(t, "0xOwnerRace", "race-batch")

	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Updates(map[string]any{"description": "ORIGINAL", "design_primary_color": "#111111"}).Error)

	bizLeg, err := database.GetBusinessByID(business.ID)
	require.NoError(t, err)
	designLeg, err := database.GetBusinessByID(business.ID) // stale Description = "ORIGINAL"
	require.NoError(t, err)

	bizLeg.Description = "EDITED"
	require.NoError(t, database.UpdateBusinessExceptDesign(bizLeg))

	designLeg.DesignSettings.PrimaryColor = "#222222"
	require.NoError(t, database.UpdateBusinessDesignOnly(designLeg))

	reloaded, err := database.GetBusinessByID(business.ID)
	require.NoError(t, err)
	assert.Equal(t, "EDITED", reloaded.Description, "design save must not clobber the concurrent description edit")
	assert.Equal(t, "#222222", reloaded.DesignSettings.PrimaryColor)
}

func BenchmarkUpdateBusinessExceptDesign(b *testing.B) {
	gormDB := setupStaffHandlerTestDBForTB(b)
	// setupStaffHandlerTestDBForTB's *testing.B path does not auto-migrate any
	// tables (it only registers the DB + RBAC), so create the businesses table
	// here — mirrors setupPaymentHistoryPerfDB's AutoMigrate pattern.
	require.NoError(b, gormDB.AutoMigrate(&database.Business{}))
	biz := &database.Business{
		OwnerAddress:   "0xBench",
		Name:           "Bench Co",
		SettlementAddr: "0xSettle",
		TippingAddr:    "0xTip",
	}
	require.NoError(b, database.GetDB().Create(biz).Error)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		biz.Description = "desc"
		if err := database.UpdateBusinessExceptDesign(biz); err != nil {
			b.Fatal(err)
		}
	}
}
