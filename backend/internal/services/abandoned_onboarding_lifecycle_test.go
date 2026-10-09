package services

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAbandonedOnboardingLifecycle_ReportsIncompleteSetup(t *testing.T) {
	db := setupTestDB(t)
	migrateSetupStatusTables(t, db)

	stalled := database.Business{
		ID: 901, BusinessId: "abandoned-onboarding", Name: "Stalled setup",
		Email: "stalled@example.test", DefaultCurrency: "USD",
	}
	require.NoError(t, db.Create(&stalled).Error)

	status, err := ComputeSetupStatus(stalled.ID)
	require.NoError(t, err)
	assert.False(t, status.RequiredDone)
	assert.NotEmpty(t, status.FirstMissingRequiredStep())
}
