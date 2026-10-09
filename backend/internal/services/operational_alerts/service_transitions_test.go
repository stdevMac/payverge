package operational_alerts

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
)

func upsertTestAlert(t *testing.T, svc *Service, businessID uint, resourceID uint) *database.OperationalAlert {
	t.Helper()
	alert, err := svc.UpsertAlert(context.Background(), UpsertAlertInput{
		BusinessID:   businessID,
		AlertType:    database.OperationalAlertTypeOrderNew,
		ResourceType: database.OperationalAlertResourceTypeOrder,
		ResourceID:   resourceID,
		Title:        "New order",
	})
	require.NoError(t, err)
	return alert
}

// A payment reorg alert and a refund reorg alert that share the same numeric id
// in one business must NOT collide on the dedup key (review finding #2/#4): the
// distinct resource types (payment vs payment_refund) keep them as two separate
// rows, so one orphaned record's alert can never mask the other's.
func TestCreateReorgSuspectedAlert_PaymentAndRefundSameIDDoNotCollide(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateReorgSuspectedAlert(ctx, business.ID, 15, "payment", map[string]any{"tx_hash": "0xpay"}))
	require.NoError(t, svc.CreateReorgSuspectedAlert(ctx, business.ID, 15, "refund", map[string]any{"tx_hash": "0xref"}))

	var alerts []database.OperationalAlert
	require.NoError(t, db.Where("business_id = ? AND alert_type = ?",
		business.ID, database.OperationalAlertTypePaymentReorgSuspected).Find(&alerts).Error)
	require.Len(t, alerts, 2, "payment id=15 and refund id=15 must be two distinct alerts, not one merged row")

	types := map[database.OperationalAlertResourceType]bool{}
	for _, a := range alerts {
		types[a.ResourceType] = true
	}
	require.True(t, types[database.OperationalAlertResourceTypePayment], "payment reorg alert scoped to resource_type=payment")
	require.True(t, types[database.OperationalAlertResourceTypePaymentRefund], "refund reorg alert scoped to resource_type=payment_refund")
}

// Fix 1: claims are guarded — staff B cannot silently steal staff A's claim.
func TestClaimAlertConflictsWhenClaimedByAnotherActor(t *testing.T) {
	db, business, staff := setupServiceTestDB(t)
	svc := NewService(db)
	alert := upsertTestAlert(t, svc, business.ID, 501)

	other := &database.Staff{
		BusinessID: business.ID,
		Name:       "Bruno",
		Email:      "bruno@example.com",
		Role:       database.StaffRoleServer,
		IsActive:   true,
	}
	require.NoError(t, db.Create(other).Error)

	_, err := svc.ClaimAlertForBusiness(context.Background(), alert.BusinessID, alert.ID, Actor{StaffID: &staff.ID, Name: staff.Name}, "row_click")
	require.NoError(t, err)

	// Staff B tries to steal → conflict, claim attribution unchanged.
	_, err = svc.ClaimAlertForBusiness(context.Background(), alert.BusinessID, alert.ID, Actor{StaffID: &other.ID, Name: other.Name}, "row_click")
	require.ErrorIs(t, err, ErrAlertConflict)
	var conflict *AlertConflictError
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, "Sofia", conflict.Alert.ClaimedByName)

	var reloaded database.OperationalAlert
	require.NoError(t, db.First(&reloaded, alert.ID).Error)
	require.Equal(t, "Sofia", reloaded.ClaimedByName)
	require.Equal(t, &staff.ID, reloaded.ClaimedByStaffID)
}

// Fix 1: re-claiming an alert you already hold is an idempotent success and
// does not append a duplicate claimed event.
func TestClaimAlertSelfReclaimIsIdempotent(t *testing.T) {
	db, business, staff := setupServiceTestDB(t)
	svc := NewService(db)
	alert := upsertTestAlert(t, svc, business.ID, 502)
	actor := Actor{StaffID: &staff.ID, Name: staff.Name}

	first, err := svc.ClaimAlertForBusiness(context.Background(), alert.BusinessID, alert.ID, actor, "row_click")
	require.NoError(t, err)

	second, err := svc.ClaimAlertForBusiness(context.Background(), alert.BusinessID, alert.ID, actor, "row_click")
	require.NoError(t, err)
	require.Equal(t, database.OperationalAlertStatusClaimed, second.Status)
	require.Equal(t, first.ClaimedAt.Unix(), second.ClaimedAt.Unix())

	var claimedEvents int64
	require.NoError(t, db.Model(&database.OperationalAlertEvent{}).
		Where("alert_id = ? AND event_type = ?", alert.ID, database.OperationalAlertEventTypeClaimed).
		Count(&claimedEvents).Error)
	require.Equal(t, int64(1), claimedEvents)
}

// Fix 1: a resolved alert cannot be resurrected by a claim — that could even
// collide with a newer active alert on idx_operational_alerts_active_unique.
func TestClaimResolvedAlertConflicts(t *testing.T) {
	db, business, staff := setupServiceTestDB(t)
	svc := NewService(db)
	alert := upsertTestAlert(t, svc, business.ID, 503)
	actor := Actor{StaffID: &staff.ID, Name: staff.Name}

	_, err := svc.ResolveAlert(context.Background(), alert.ID, actor, "done")
	require.NoError(t, err)

	_, err = svc.ClaimAlertForBusiness(context.Background(), alert.BusinessID, alert.ID, actor, "row_click")
	require.ErrorIs(t, err, ErrAlertConflict)

	var reloaded database.OperationalAlert
	require.NoError(t, db.First(&reloaded, alert.ID).Error)
	require.Equal(t, database.OperationalAlertStatusResolved, reloaded.Status)
}

// Fix 1: resolving twice is an idempotent success — no duplicate event, no
// clobbered resolved_at.
func TestResolveAlertTwiceIsIdempotent(t *testing.T) {
	db, business, staff := setupServiceTestDB(t)
	svc := NewService(db)
	alert := upsertTestAlert(t, svc, business.ID, 504)
	actor := Actor{StaffID: &staff.ID, Name: staff.Name}

	first, err := svc.ResolveAlert(context.Background(), alert.ID, actor, "done")
	require.NoError(t, err)
	require.NotNil(t, first.ResolvedAt)

	second, err := svc.ResolveAlert(context.Background(), alert.ID, actor, "done again")
	require.NoError(t, err)
	require.Equal(t, database.OperationalAlertStatusResolved, second.Status)
	require.Equal(t, first.ResolvedAt.Unix(), second.ResolvedAt.Unix())

	var resolvedEvents int64
	require.NoError(t, db.Model(&database.OperationalAlertEvent{}).
		Where("alert_id = ? AND event_type = ?", alert.ID, database.OperationalAlertEventTypeResolved).
		Count(&resolvedEvents).Error)
	require.Equal(t, int64(1), resolvedEvents)
}

// Fix 1: a resolver who differs from the claimer must not overwrite the
// original claim attribution — the resolver is recorded on the event row.
func TestResolveKeepsOriginalClaimAttribution(t *testing.T) {
	db, business, staff := setupServiceTestDB(t)
	svc := NewService(db)
	alert := upsertTestAlert(t, svc, business.ID, 505)

	_, err := svc.ClaimAlertForBusiness(context.Background(), alert.BusinessID, alert.ID, Actor{StaffID: &staff.ID, Name: staff.Name}, "row_click")
	require.NoError(t, err)

	managerID := uint(9911)
	resolved, err := svc.ResolveAlert(context.Background(), alert.ID, Actor{UserID: &managerID, Name: "Manager Mia"}, "handled")
	require.NoError(t, err)
	require.Equal(t, database.OperationalAlertStatusResolved, resolved.Status)
	require.Equal(t, "Sofia", resolved.ClaimedByName)
	require.Equal(t, &staff.ID, resolved.ClaimedByStaffID)
	require.Nil(t, resolved.ClaimedByUserID)

	var event database.OperationalAlertEvent
	require.NoError(t, db.Where("alert_id = ? AND event_type = ?", alert.ID, database.OperationalAlertEventTypeResolved).First(&event).Error)
	require.Equal(t, "Manager Mia", event.ActorName)
}

// Fix 6: snoozing a resolved (or dismissed) alert is rejected.
func TestSnoozeResolvedAlertConflicts(t *testing.T) {
	db, business, staff := setupServiceTestDB(t)
	svc := NewService(db)
	alert := upsertTestAlert(t, svc, business.ID, 506)
	actor := Actor{StaffID: &staff.ID, Name: staff.Name}

	_, err := svc.ResolveAlert(context.Background(), alert.ID, actor, "done")
	require.NoError(t, err)

	_, err = svc.SnoozeAlertForBusiness(context.Background(), alert.BusinessID, alert.ID, actor, time.Now().Add(5*time.Minute))
	require.ErrorIs(t, err, ErrAlertConflict)
}

// Fix 2: money fields must never reach alerts:read holders — UpsertAlert
// strips the sensitive-key denylist before persisting AND before publishing.
func TestUpsertAlertStripsSensitiveMetadata(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	alert, err := svc.UpsertAlert(context.Background(), UpsertAlertInput{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypePaymentReceived,
		ResourceType: database.OperationalAlertResourceTypePayment,
		ResourceID:   77,
		Title:        "Payment received",
		Metadata: map[string]any{
			"amount_cents":      12345,
			"tip_cents":         678,
			"tip_amount_cents":  678,
			"settlement_source": "payment_webhook",
			"method":            "crypto",
			"payment_method":    "cash",
			"customer_name":     "Ana",
			"bill_number":       "B-1",
		},
	})
	require.NoError(t, err)

	var persisted map[string]any
	require.NoError(t, json.Unmarshal([]byte(alert.Metadata), &persisted))
	for _, key := range []string{"amount_cents", "tip_cents", "tip_amount_cents", "settlement_source", "method", "payment_method"} {
		require.NotContains(t, persisted, key)
	}
	require.Equal(t, "Ana", persisted["customer_name"])
	require.Equal(t, "B-1", persisted["bill_number"])

	select {
	case frame := <-ch:
		require.Equal(t, "alert.created", frame.Type)
		require.NotContains(t, string(frame.Data), "amount_cents")
		require.NotContains(t, string(frame.Data), "settlement_source")
	case <-time.After(2 * time.Second):
		t.Fatal("expected alert.created frame on the hub")
	}
}

// Fix 3: service_call metadata must carry table_name so localized FE copy can
// still say WHICH table raised its hand.
func TestServiceCallAlertMetadataIncludesTableName(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)

	require.NoError(t, svc.CreateServiceCallAlert(context.Background(), business.ID, 14, "Mesa 14", "check"))

	var alert database.OperationalAlert
	require.NoError(t, db.Where("alert_type = ?", database.OperationalAlertTypeServiceCall).First(&alert).Error)
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(alert.Metadata), &metadata))
	require.Equal(t, "Mesa 14", metadata["table_name"])
	require.Equal(t, "check", metadata["reason"])
}

// Fix 6: two callers racing UpsertAlert for the same resource — the loser's
// INSERT hits idx_operational_alerts_active_unique. It must recover by
// updating the winner's row instead of surfacing a 500.
func TestUpsertAlertRecoversFromLostCreateRace(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	// Mirror the Postgres partial unique index (sqlite supports the syntax).
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX idx_operational_alerts_active_unique
		 ON operational_alerts (business_id, alert_type, resource_type, resource_id)
		 WHERE status IN ('open', 'claimed')`,
	).Error)
	svc := NewService(db)

	// Simulate the lost race: between the find (no active row) and the INSERT,
	// a competing caller creates the active row. A before-create callback that
	// raw-inserts the conflicting row deterministically reproduces this.
	raced := false
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:upsert_race", func(tx *gorm.DB) {
		if raced || tx.Statement == nil || tx.Statement.Table != "operational_alerts" {
			return
		}
		raced = true
		if err := tx.Session(&gorm.Session{NewDB: true, SkipHooks: true}).Exec(
			`INSERT INTO operational_alerts
			   (business_id, alert_type, resource_type, resource_id, status, priority, title, body, claimed_by_name, last_event_at, metadata, created_at, updated_at)
			 VALUES (?, ?, ?, ?, 'open', 'normal', 'Winner', 'Winner body', '', CURRENT_TIMESTAMP, '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			business.ID, string(database.OperationalAlertTypeOrderNew), string(database.OperationalAlertResourceTypeOrder), 900,
		).Error; err != nil {
			tx.AddError(err)
		}
	}))
	defer func() { _ = db.Callback().Create().Remove("test:upsert_race") }()

	alert, err := svc.UpsertAlert(context.Background(), UpsertAlertInput{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeOrderNew,
		ResourceType: database.OperationalAlertResourceTypeOrder,
		ResourceID:   900,
		Title:        "Loser title",
		Body:         "Loser body",
	})
	require.NoError(t, err, "losing caller must recover as an update, not 500")
	require.Equal(t, "Loser body", alert.Body, "loser's payload should refresh the winner's row")

	var count int64
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("business_id = ? AND resource_id = ? AND status IN ?", business.ID, 900,
			[]database.OperationalAlertStatus{database.OperationalAlertStatusOpen, database.OperationalAlertStatusClaimed}).
		Count(&count).Error)
	require.Equal(t, int64(1), count, "exactly one active row must survive the race")
	require.False(t, errors.Is(err, ErrAlertConflict))
}
