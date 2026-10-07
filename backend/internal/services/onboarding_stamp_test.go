package services

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedReadyBusiness(t *testing.T, db *gorm.DB, id uint) database.Business {
	t.Helper()
	biz := database.Business{
		ID: id, BusinessId: "stamp-biz", Name: "Stamp Biz", OwnerName: "Owner",
		Email: "owner@example.com", DefaultCurrency: "USD",
	}
	require.NoError(t, db.Create(&biz).Error)
	require.NoError(t, db.Create(&database.Table{BusinessID: id, Name: "T1", TableCode: "STAMP01", IsActive: true}).Error)
	require.NoError(t, db.Create(&database.Menu{
		BusinessID: id, IsActive: true,
		Categories: `[{"id":"c1","name":"Mains","items":[{"id":"i1","name":"Dish","price":10}]}]`,
	}).Error)
	return biz
}

// Completion must not depend on a browser visiting the Overview tab. The
// stamp is a one-shot conditional update; migration-owned activation triggers
// observe the persisted transition.
func TestStampOnboardingCompletedIfReady(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&database.Table{}, &database.Menu{}, &database.Staff{},
		&database.Plugin{}, &database.BusinessPlugin{},
	))

	seedReadyBusiness(t, db, 1)

	stamped, err := StampOnboardingCompletedIfReady(1)
	require.NoError(t, err)
	assert.True(t, stamped, "required steps done + NULL stamp → must claim")

	var after database.Business
	require.NoError(t, db.First(&after, 1).Error)
	require.NotNil(t, after.OnboardingCompletedAt)

	// Second call is an idempotent no-op.
	stamped, err = StampOnboardingCompletedIfReady(1)
	require.NoError(t, err)
	assert.False(t, stamped)
}

func TestStampOnboardingCompletedIfReady_SkipsWhenRequiredStepsIncomplete(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&database.Table{}, &database.Menu{}, &database.Staff{},
		&database.Plugin{}, &database.BusinessPlugin{},
	))

	// Menu missing → RequiredDone false → no stamp.
	biz := database.Business{ID: 2, BusinessId: "stamp-biz-2", Name: "No Menu", DefaultCurrency: "USD"}
	require.NoError(t, db.Create(&biz).Error)
	require.NoError(t, db.Create(&database.Table{BusinessID: 2, Name: "T1", TableCode: "STAMP02", IsActive: true}).Error)

	stamped, err := StampOnboardingCompletedIfReady(2)
	require.NoError(t, err)
	assert.False(t, stamped)
	var after database.Business
	require.NoError(t, db.First(&after, 2).Error)
	assert.Nil(t, after.OnboardingCompletedAt)
}
