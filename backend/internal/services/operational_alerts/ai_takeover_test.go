package operational_alerts

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestCreateAITakeoverAlertDedupsPerConversation(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateAITakeoverAlert(ctx, business.ID, 42))

	var alerts []database.OperationalAlert
	require.NoError(t, db.Where("business_id = ? AND alert_type = ?", business.ID, database.OperationalAlertTypeAITakeover).Find(&alerts).Error)
	require.Len(t, alerts, 1)
	first := alerts[0]
	require.Equal(t, database.OperationalAlertStatusOpen, first.Status)
	require.Equal(t, database.OperationalAlertResourceTypeAIConversation, first.ResourceType)
	require.Equal(t, int64(42), first.ResourceID)
	require.Equal(t, database.OperationalAlertPriorityUrgent, first.Priority)
	require.Contains(t, string(first.Metadata), `"conversation_id":42`)

	// Second guest message into the same paused conversation must upsert the
	// SAME open alert, not pile up duplicates.
	require.NoError(t, svc.CreateAITakeoverAlert(ctx, business.ID, 42))

	require.NoError(t, db.Where("business_id = ? AND alert_type = ?", business.ID, database.OperationalAlertTypeAITakeover).Find(&alerts).Error)
	require.Len(t, alerts, 1)
}

func TestResolveAITakeoverAlertOnClaim(t *testing.T) {
	db, business, staff := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateAITakeoverAlert(ctx, business.ID, 7))

	require.NoError(t, svc.ResolveAITakeoverAlert(ctx, business.ID, 7, Actor{StaffID: &staff.ID, Name: staff.Name}))

	var alert database.OperationalAlert
	require.NoError(t, db.Where("business_id = ? AND alert_type = ? AND resource_id = ?",
		business.ID, database.OperationalAlertTypeAITakeover, 7).First(&alert).Error)
	require.Equal(t, database.OperationalAlertStatusResolved, alert.Status)
	require.NotNil(t, alert.ResolvedAt)
}

func TestResolveAITakeoverAlertMissingAlertIsNotAnError(t *testing.T) {
	db, business, staff := setupServiceTestDB(t)
	svc := NewService(db)

	require.NoError(t, svc.ResolveAITakeoverAlert(context.Background(), business.ID, 999, Actor{StaffID: &staff.ID, Name: staff.Name}))
}

func TestAITakeoverSettingsSelfHeal(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Simulate a settings row created before ai_takeover existed: everything
	// else configured, ai_takeover left zero-value.
	stale := database.DefaultBusinessAlertSettings(business.ID)
	stale.EventSettings.AITakeover = database.AlertTypeSetting{}
	require.NoError(t, db.Create(&stale).Error)

	settings, err := svc.GetSettings(ctx, business.ID)
	require.NoError(t, err)
	require.True(t, settings.EventSettings.AITakeover.Enabled)
	require.True(t, settings.EventSettings.AITakeover.Repeating)
	require.Equal(t, database.OperationalAlertPriorityUrgent, settings.EventSettings.AITakeover.Priority)
}
