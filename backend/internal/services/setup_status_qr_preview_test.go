package services

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

func firstValueSetupInput() SetupStatusInput {
	return SetupStatusInput{
		BusinessName:    "First Value Cafe",
		DefaultCurrency: "AED",
		TableCount:      1,
		MenuCategories:  1,
		MenuItems:       1,
	}
}

func TestSetupStatusQRPreviewIsServerAuthoritative(t *testing.T) {
	before := ComputeSetupStatusFromInput(firstValueSetupInput())
	require.False(t, before.QRPreviewed, "menu + table never imply a QR preview")
	require.True(t, before.RequiredDone)

	in := firstValueSetupInput()
	in.QRPreviewed = true
	require.True(t, ComputeSetupStatusFromInput(in).QRPreviewed)
}

func TestSetupStatusQRPreviewDoesNotBypassMissingFirstValueSteps(t *testing.T) {
	incomplete := firstValueSetupInput()
	incomplete.MenuCategories = 0
	incomplete.MenuItems = 0
	incomplete.QRPreviewed = true

	status := ComputeSetupStatusFromInput(incomplete)
	require.True(t, status.QRPreviewed)
	require.False(t, status.RequiredDone,
		"a QR preview cannot complete setup while the first menu item is missing")
}

func TestComputeSetupStatusLoadsPersistedQRPreviewMilestone(t *testing.T) {
	db := setupTestDB(t)
	migrateSetupStatusTables(t, db)
	previewedAt := time.Now().UTC()
	business := &database.Business{
		BusinessId:      "db-backed-qr-status",
		Name:            "First Value Cafe",
		OwnerAddress:    "0xOwner",
		DefaultCurrency: "AED",
		QRPreviewedAt:   &previewedAt,
		IsActive:        true,
	}
	require.NoError(t, db.Create(business).Error)
	require.NoError(t, db.Create(&database.Table{
		BusinessID: business.ID,
		TableCode:  "db-backed-table",
		Name:       "Table 1",
		IsActive:   true,
	}).Error)
	categories, err := json.Marshal([]database.MenuCategory{{
		ID: "main", Name: "Main",
		Items: []database.MenuItem{{ID: "coffee", Name: "Coffee", IsAvailable: true}},
	}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{
		BusinessID: business.ID,
		Categories: string(categories),
		IsActive:   true,
	}).Error)

	status, err := ComputeSetupStatus(business.ID)
	require.NoError(t, err)
	require.True(t, status.QRPreviewed)
	require.True(t, status.RequiredDone)
}
