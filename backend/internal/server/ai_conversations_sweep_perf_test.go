package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
)

// fullActiveAlertListCount counts operational_alerts reads that list the
// business's whole active-alert set (status filter, no resource key) — the
// shape the stale-claim sweep used to issue once per released conversation.
func (r *billAccessSQLRecorder) fullActiveAlertListCount() int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select ") || !strings.Contains(normalized, "operational_alerts") {
			continue
		}
		if strings.Contains(normalized, "status in") && !strings.Contains(normalized, "resource_id") {
			count++
		}
	}
	return count
}

type staleClaimFixture struct {
	db       *gorm.DB
	business *database.Business
	staff    *database.Staff
	stale    []uint
	fresh    []uint
}

func seedStaleClaimFixture(t testing.TB, rec logger.Interface, staleN, freshN int) staleClaimFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDBWithLogger(t, rec)
	require.NoError(t, db.AutoMigrate(
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
		&database.BusinessAlertSettings{},
	))
	business := createAIWaiterBusiness(t, "sweep-batch", true)
	staff := createAIWaiterStaff(t, business.ID, database.StaffRoleManager, "sweep-batch@example.com")
	svc := operational_alerts.NewService(db)
	fx := staleClaimFixture{db: db, business: business, staff: staff}
	seed := func(i int, claimedAt time.Time) uint {
		conv := createAIWaiterConversation(t, business.ID, fmt.Sprintf("sweep-batch-%d", i))
		require.NoError(t, db.Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).
			Updates(map[string]interface{}{
				"table_code":          fmt.Sprintf("T-%d", i),
				"is_paused":           true,
				"claimed_by_staff_id": staff.ID,
				"claimed_by_name":     "Gone",
				"claimed_by_role":     "manager",
				"claimed_at":          claimedAt,
			}).Error)
		require.NoError(t, svc.CreateAITakeoverAlert(context.Background(), business.ID, int64(conv.ID)))
		return conv.ID
	}
	staleAt := time.Now().Add(-aiClaimIdleTTL - time.Minute)
	for i := 0; i < staleN; i++ {
		fx.stale = append(fx.stale, seed(i, staleAt))
	}
	for i := 0; i < freshN; i++ {
		fx.fresh = append(fx.fresh, seed(staleN+i, time.Now()))
	}
	return fx
}

func alertStatusFor(t testing.TB, db *gorm.DB, businessID, convID uint) database.OperationalAlertStatus {
	t.Helper()
	var alert database.OperationalAlert
	require.NoError(t, db.Where("business_id = ? AND resource_type = ? AND resource_id = ?",
		businessID, database.OperationalAlertResourceTypeAIConversation, convID).First(&alert).Error)
	return alert.Status
}

// The stale-claim sweep on the conversation list poll must resolve the
// orphaned takeover alerts with one keyed lookup, not one full active-alert
// list per released conversation, and must leave fresh claims alone.
func TestGetAiConversations_StaleSweepBatchesAlertResolution(t *testing.T) {
	rec := &billAccessSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	fx := seedStaleClaimFixture(t, rec, 8, 4)

	rec.statements = nil
	router := aiStaffRouter(fx.staff.ID, database.StaffRoleManager, fx.business.ID)
	w := performAIWaiterRequest(t, router, http.MethodGet,
		fmt.Sprintf("/businesses/%d/ai/conversations", fx.business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	require.Zero(t, rec.fullActiveAlertListCount(),
		"stale sweep must not list every active alert per released conversation")

	for _, id := range fx.stale {
		require.Equal(t, database.OperationalAlertStatusResolved, alertStatusFor(t, fx.db, fx.business.ID, id))
		var conv database.AiWaiterConversation
		require.NoError(t, fx.db.First(&conv, id).Error)
		require.Nil(t, conv.ClaimedAt)
		require.False(t, conv.IsPaused)
	}
	for _, id := range fx.fresh {
		require.Equal(t, database.OperationalAlertStatusOpen, alertStatusFor(t, fx.db, fx.business.ID, id))
		var conv database.AiWaiterConversation
		require.NoError(t, fx.db.First(&conv, id).Error)
		require.NotNil(t, conv.ClaimedAt)
		require.True(t, conv.IsPaused)
	}
}

func BenchmarkGetAiConversationsStaleSweepSQLite(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		fx := seedStaleClaimFixture(b, nil, 20, 20)
		router := aiStaffRouter(fx.staff.ID, database.StaffRoleManager, fx.business.ID)
		path := fmt.Sprintf("/businesses/%d/ai/conversations", fx.business.ID)
		b.StartTimer()
		w := performAIWaiterRequest(b, router, http.MethodGet, path, nil)
		if w.Code != http.StatusOK {
			b.Fatalf("status %d", w.Code)
		}
		b.StopTimer()
		if sqlDB, err := fx.db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		b.StartTimer()
	}
}

// The background sweep releases stale claims across every business without a
// dashboard poll, so the list GET normally finds nothing to write.
func TestReleaseStaleAiClaims_BackgroundSweepAcrossBusinesses(t *testing.T) {
	fx := seedStaleClaimFixture(t, nil, 3, 2)

	released, err := ReleaseStaleAiClaims(context.Background(), 0, time.Now())
	require.NoError(t, err)
	require.Equal(t, 3, released)

	for _, id := range fx.stale {
		require.Equal(t, database.OperationalAlertStatusResolved, alertStatusFor(t, fx.db, fx.business.ID, id))
	}
	for _, id := range fx.fresh {
		require.Equal(t, database.OperationalAlertStatusOpen, alertStatusFor(t, fx.db, fx.business.ID, id))
	}

	again, err := ReleaseStaleAiClaims(context.Background(), 0, time.Now())
	require.NoError(t, err)
	require.Zero(t, again, "a second sweep has nothing left to release")
}
