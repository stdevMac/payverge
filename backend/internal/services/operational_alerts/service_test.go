package operational_alerts

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupServiceTestDB(t *testing.T) (*gorm.DB, *database.Business, *database.Staff) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.User{},
		&database.Staff{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
		&database.BusinessAlertSettings{},
	))
	business := &database.Business{
		BusinessId:     "ops-alert-service",
		Name:           "Ops Alert Service",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
	}
	require.NoError(t, db.Create(business).Error)
	staff := &database.Staff{
		BusinessID: business.ID,
		Name:       "Sofia",
		Email:      "sofia@example.com",
		Role:       database.StaffRoleServer,
		IsActive:   true,
	}
	require.NoError(t, db.Create(staff).Error)
	return db, business, staff
}

func TestUpsertAlertCreatesThenUpdatesActiveAlert(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)

	first, err := svc.UpsertAlert(context.Background(), UpsertAlertInput{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeOrderNew,
		ResourceType: database.OperationalAlertResourceTypeOrder,
		ResourceID:   44,
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        "New order #44",
		Body:         "First body",
		Metadata:     map[string]any{"order_number": "44"},
	})
	require.NoError(t, err)
	require.Equal(t, database.OperationalAlertStatusOpen, first.Status)

	second, err := svc.UpsertAlert(context.Background(), UpsertAlertInput{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeOrderNew,
		ResourceType: database.OperationalAlertResourceTypeOrder,
		ResourceID:   44,
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        "New order #44",
		Body:         "Updated body",
		Metadata:     map[string]any{"bill_id": 9},
	})
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, "Updated body", second.Body)

	var count int64
	require.NoError(t, db.Model(&database.OperationalAlert{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestClaimAlertAttributesStaffAndStopsOpenState(t *testing.T) {
	db, business, staff := setupServiceTestDB(t)
	svc := NewService(db)
	alert, err := svc.UpsertAlert(context.Background(), UpsertAlertInput{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeReservationNew,
		ResourceType: database.OperationalAlertResourceTypeReservation,
		ResourceID:   12,
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        "New reservation",
	})
	require.NoError(t, err)

	claimed, err := svc.ClaimAlertForBusiness(context.Background(), alert.BusinessID, alert.ID, Actor{
		StaffID: &staff.ID,
		Name:    staff.Name,
	}, "row_click")
	require.NoError(t, err)
	require.Equal(t, database.OperationalAlertStatusClaimed, claimed.Status)
	require.Equal(t, "Sofia", claimed.ClaimedByName)
	require.NotNil(t, claimed.ClaimedAt)

	var events []database.OperationalAlertEvent
	require.NoError(t, db.Where("alert_id = ?", alert.ID).Order("id ASC").Find(&events).Error)
	require.Len(t, events, 2)
	require.Equal(t, database.OperationalAlertEventTypeCreated, events[0].EventType)
	require.Equal(t, database.OperationalAlertEventTypeClaimed, events[1].EventType)
	require.Equal(t, "Sofia", events[1].ActorName)
}

func TestResolveAlertSetsResolvedAtAndRecordsEvent(t *testing.T) {
	db, business, staff := setupServiceTestDB(t)
	svc := NewService(db)
	alert, err := svc.UpsertAlert(context.Background(), UpsertAlertInput{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeDeliveryNew,
		ResourceType: database.OperationalAlertResourceTypeDelivery,
		ResourceID:   88,
		Title:        "New delivery",
	})
	require.NoError(t, err)

	resolved, err := svc.ResolveAlert(context.Background(), alert.ID, Actor{
		StaffID: &staff.ID,
		Name:    staff.Name,
	}, "assigned")
	require.NoError(t, err)
	require.Equal(t, database.OperationalAlertStatusResolved, resolved.Status)
	require.NotNil(t, resolved.ResolvedAt)
}

func TestSnoozeAlertKeepsOpenButSetsSnoozedUntil(t *testing.T) {
	db, business, staff := setupServiceTestDB(t)
	svc := NewService(db)
	alert, err := svc.UpsertAlert(context.Background(), UpsertAlertInput{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeOrderNew,
		ResourceType: database.OperationalAlertResourceTypeOrder,
		ResourceID:   99,
		Title:        "New order",
	})
	require.NoError(t, err)

	until := time.Now().Add(5 * time.Minute)
	snoozed, err := svc.SnoozeAlertForBusiness(context.Background(), alert.BusinessID, alert.ID, Actor{
		StaffID: &staff.ID,
		Name:    staff.Name,
	}, until)
	require.NoError(t, err)
	require.Equal(t, database.OperationalAlertStatusOpen, snoozed.Status)
	require.NotNil(t, snoozed.SnoozedUntil)
	require.True(t, snoozed.SnoozedUntil.After(time.Now()))
}

func TestGetSettingsCreatesDefaults(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)

	settings, err := svc.GetSettings(context.Background(), business.ID)
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.False(t, settings.SoundEnabled)
	require.Equal(t, database.DefaultBusinessAlertRepeatIntervalSeconds, settings.RepeatIntervalSeconds)

	settingsAgain, err := svc.GetSettings(context.Background(), business.ID)
	require.NoError(t, err)
	require.Equal(t, settings.ID, settingsAgain.ID)
}

func TestGetSettingsClampsLegacyEightSecondInterval(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	stale := database.DefaultBusinessAlertSettings(business.ID)
	stale.SoundEnabled = true
	stale.RepeatIntervalSeconds = 8
	require.NoError(t, db.Create(&stale).Error)

	settings, err := svc.GetSettings(ctx, business.ID)
	require.NoError(t, err)
	require.Equal(t, database.MinBusinessAlertRepeatIntervalSeconds, settings.RepeatIntervalSeconds)
	require.GreaterOrEqual(t, settings.RepeatIntervalSeconds, 30)
}

func TestUpdateSettingsRejectsSubThirtyRepeat(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	_, err := svc.GetSettings(ctx, business.ID)
	require.NoError(t, err)

	next := database.DefaultBusinessAlertSettings(business.ID)
	next.SoundEnabled = true
	next.RepeatIntervalSeconds = 8
	_, err = svc.UpdateSettings(ctx, business.ID, next, Actor{})
	require.ErrorIs(t, err, ErrInvalidAlertSetting)

	next.RepeatIntervalSeconds = 12
	_, err = svc.UpdateSettings(ctx, business.ID, next, Actor{})
	require.ErrorIs(t, err, ErrInvalidAlertSetting)
}

func TestCreateReservationAlertRoutesTypeAndPriority(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)

	approval := database.TableReservation{CustomerName: "Ana"}
	approval.ID = 21
	approval.BusinessID = business.ID
	require.NoError(t, svc.CreateReservationAlert(
		context.Background(), approval,
		database.OperationalAlertTypeReservationApproval,
		database.OperationalAlertPriorityUrgent,
	))

	confirmed := database.TableReservation{CustomerName: "Bruno"}
	confirmed.ID = 22
	confirmed.BusinessID = business.ID
	require.NoError(t, svc.CreateReservationAlert(
		context.Background(), confirmed,
		database.OperationalAlertTypeReservationNew,
		database.OperationalAlertPriorityNormal,
	))

	var approvalAlert database.OperationalAlert
	require.NoError(t, db.Where("alert_type = ?", database.OperationalAlertTypeReservationApproval).First(&approvalAlert).Error)
	require.Equal(t, database.OperationalAlertPriorityUrgent, approvalAlert.Priority)
	require.Equal(t, "Reservation needs approval", approvalAlert.Title)
	require.Equal(t, database.OperationalAlertResourceTypeReservation, approvalAlert.ResourceType)
	require.Equal(t, int64(21), approvalAlert.ResourceID)

	var newAlert database.OperationalAlert
	require.NoError(t, db.Where("alert_type = ?", database.OperationalAlertTypeReservationNew).First(&newAlert).Error)
	require.Equal(t, database.OperationalAlertPriorityNormal, newAlert.Priority)
	require.Equal(t, "New reservation", newAlert.Title)
	require.Equal(t, int64(22), newAlert.ResourceID)
}

func TestResolveAlertForResourceResolvesBothReservationAlertTypes(t *testing.T) {
	db, business, staff := setupServiceTestDB(t)
	svc := NewService(db)

	res := database.TableReservation{CustomerName: "Carla"}
	res.ID = 33
	res.BusinessID = business.ID
	require.NoError(t, svc.CreateReservationAlert(
		context.Background(), res,
		database.OperationalAlertTypeReservationApproval,
		database.OperationalAlertPriorityUrgent,
	))

	require.NoError(t, svc.ResolveAlertForResource(
		context.Background(), business.ID,
		database.OperationalAlertResourceTypeReservation, res.ID,
		Actor{StaffID: &staff.ID, Name: staff.Name}, "confirmed",
	))

	var open int64
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("status IN ?", []database.OperationalAlertStatus{
			database.OperationalAlertStatusOpen,
			database.OperationalAlertStatusClaimed,
		}).Count(&open).Error)
	require.Equal(t, int64(0), open)
}

func TestGetSettingsDefaultsIncludeReservationApprovalAndSelfHealOldRows(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)

	// Fresh rows include the new type by default.
	settings, err := svc.GetSettings(context.Background(), business.ID)
	require.NoError(t, err)
	require.True(t, settings.EventSettings.ReservationApproval.Enabled)
	require.Equal(t, database.OperationalAlertPriorityUrgent, settings.EventSettings.ReservationApproval.Priority)

	// Simulate a pre-existing row stored before reservation_approval existed:
	// zero out the field and persist.
	settings.EventSettings.ReservationApproval = database.AlertTypeSetting{}
	settings.EventSettings.ReservationNew = database.AlertTypeSetting{
		Enabled:   false,
		Repeating: false,
		Priority:  database.OperationalAlertPriorityNormal,
	}
	rawEventSettings, err := json.Marshal(settings.EventSettings)
	require.NoError(t, err)
	require.NoError(t, db.Model(&database.BusinessAlertSettings{}).
		Where("id = ?", settings.ID).
		Update("event_settings", string(rawEventSettings)).Error)

	// Read path self-heals: the unconfigured type inherits reservation_new.
	healed, err := svc.GetSettings(context.Background(), business.ID)
	require.NoError(t, err)
	require.Equal(t, healed.EventSettings.ReservationNew, healed.EventSettings.ReservationApproval)
	require.False(t, healed.EventSettings.ReservationApproval.Enabled)
}

func TestCreateCryptoSettlementReviewAlert(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)

	// setupServiceTestDB does not migrate bills; the alert path only needs the
	// bill scalar fields (id/business_id/bill_number) for UpsertAlert metadata.
	bill := database.Bill{ID: 77, BusinessID: business.ID, BillNumber: "B-77", TotalAmount: 5000}

	err := svc.CreateCryptoSettlementReviewAlert(context.Background(), bill, "0xdeadbeef", map[string]any{
		"amount_cents": 5000,
		"tip_cents":    0,
	})
	require.NoError(t, err)

	var alert database.OperationalAlert
	require.NoError(t, db.Where("business_id = ? AND resource_id = ?", bill.BusinessID, bill.ID).First(&alert).Error)
	require.Equal(t, database.OperationalAlertTypePaymentRefundReview, alert.AlertType)
	require.Equal(t, database.OperationalAlertPriorityUrgent, alert.Priority)
	require.Contains(t, string(alert.Metadata), "0xdeadbeef")
}

func TestCreateCryptoTxHashConflictAlert(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	bill := database.Bill{ID: 78, BusinessID: business.ID, BillNumber: "B-78", TotalAmount: 5000}
	hash := "0x" + strings.Repeat("ab", 32)

	require.NoError(t, svc.CreateCryptoTxHashConflictAlert(context.Background(), bill, hash, nil, map[string]any{"quote_id": 9}))
	// A repeated attempt refreshes the same row instead of piling up alerts.
	require.NoError(t, svc.CreateCryptoTxHashConflictAlert(context.Background(), bill, hash, nil, nil))

	var alerts []database.OperationalAlert
	require.NoError(t, db.Where("business_id = ? AND resource_id = ?", bill.BusinessID, bill.ID).Find(&alerts).Error)
	require.Len(t, alerts, 1)
	require.Equal(t, database.OperationalAlertTypePaymentRefundReview, alerts[0].AlertType)
	require.Equal(t, database.OperationalAlertResourceTypeBill, alerts[0].ResourceType)
	require.Equal(t, database.OperationalAlertPriorityHigh, alerts[0].Priority)
	require.Contains(t, string(alerts[0].Metadata), "tx_hash_conflict")
	require.Contains(t, string(alerts[0].Metadata), hash)
	meta := decodeTestAlertMetadata(t, alerts[0])
	require.NotContains(t, meta, followUpEventsMetadataKey, "a repeat of the same conflict must not grow a follow-up trail")
}

// Review c1 R2-M1: the conflict alert names the bill that already records the
// replayed transfer (same business only) and never invites a manual settle or
// refund on the strength of it.
func TestCreateCryptoTxHashConflictAlertNamesTheRecordedBill(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	t.Run("another bill of the same business", func(t *testing.T) {
		bill := database.Bill{ID: 90, BusinessID: business.ID, BillNumber: "B-90"}
		hash := "0x" + strings.Repeat("9a", 32)
		holder := &database.PaymentTxHashHolder{PaymentID: 501, BillID: 89, BusinessID: business.ID, BillNumber: "B-89"}
		require.NoError(t, svc.CreateCryptoTxHashConflictAlert(ctx, bill, hash, holder, map[string]any{"quote_id": 4}))
		alert, meta := requireSingleBillReviewAlert(t, db, bill)
		require.Equal(t, "Crypto payment reused on another bill", alert.Title)
		require.Contains(t, alert.Body, "already paid bill B-89")
		require.Contains(t, alert.Body, "Do not settle or refund anything with this transfer")
		require.Equal(t, CryptoReviewReasonTxHashConflict, meta["reason"])
		require.EqualValues(t, 89, meta["recorded_bill_id"])
		require.Equal(t, "B-89", meta["recorded_bill_number"])
		require.EqualValues(t, 501, meta["recorded_payment_id"])
	})

	t.Run("the bill it already paid", func(t *testing.T) {
		bill := database.Bill{ID: 91, BusinessID: business.ID, BillNumber: "B-91"}
		hash := "0x" + strings.Repeat("9b", 32)
		holder := &database.PaymentTxHashHolder{PaymentID: 502, BillID: 91, BusinessID: business.ID, BillNumber: "B-91"}
		require.NoError(t, svc.CreateCryptoTxHashConflictAlert(ctx, bill, hash, holder, nil))
		alert, meta := requireSingleBillReviewAlert(t, db, bill)
		require.Equal(t, "Crypto payment presented twice", alert.Title)
		require.Contains(t, alert.Body, "already recorded on this bill")
		require.EqualValues(t, 91, meta["recorded_bill_id"])
	})

	t.Run("another business never leaks its bill", func(t *testing.T) {
		bill := database.Bill{ID: 92, BusinessID: business.ID, BillNumber: "B-92"}
		hash := "0x" + strings.Repeat("9c", 32)
		holder := &database.PaymentTxHashHolder{PaymentID: 503, BillID: 4000, BusinessID: business.ID + 1000, BillNumber: "OTHER-VENUE-1"}
		require.NoError(t, svc.CreateCryptoTxHashConflictAlert(ctx, bill, hash, holder, nil))
		alert, meta := requireSingleBillReviewAlert(t, db, bill)
		require.Equal(t, "Crypto payment reused on another bill", alert.Title)
		require.NotContains(t, alert.Body, "OTHER-VENUE-1")
		require.NotContains(t, string(alert.Metadata), "OTHER-VENUE-1")
		require.NotContains(t, meta, "recorded_bill_id")
		require.NotContains(t, meta, "recorded_payment_id")
		require.Contains(t, alert.Body, "Do not settle or refund anything by hand")
	})
}

func decodeTestAlertMetadata(t *testing.T, alert database.OperationalAlert) map[string]any {
	t.Helper()
	meta := map[string]any{}
	require.NoError(t, json.Unmarshal(alert.Metadata, &meta))
	return meta
}

func requireSingleBillReviewAlert(t *testing.T, db *gorm.DB, bill database.Bill) (database.OperationalAlert, map[string]any) {
	t.Helper()
	var alerts []database.OperationalAlert
	require.NoError(t, db.Where("business_id = ? AND resource_id = ?", bill.BusinessID, bill.ID).Find(&alerts).Error)
	require.Len(t, alerts, 1)
	return alerts[0], decodeTestAlertMetadata(t, alerts[0])
}

func followUpTxHashes(t *testing.T, meta map[string]any) []string {
	t.Helper()
	raw, ok := meta[followUpEventsMetadataKey]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	require.True(t, ok, "followup_events must be a list")
	out := make([]string, 0, len(list))
	for _, item := range list {
		event, ok := item.(map[string]any)
		require.True(t, ok)
		hash, _ := event["transaction_hash"].(string)
		out = append(out, hash)
	}
	return out
}

// Review c1 L2: the tx-hash conflict alert shares the bill-scoped
// payment_refund_review key with the Urgent settlement-review alert. A later
// conflict must not downgrade the Urgent alert or replace its evidence.
func TestCryptoTxHashConflictAlertDoesNotDowngradeSettlementReview(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	bill := database.Bill{ID: 81, BusinessID: business.ID, BillNumber: "B-81", TotalAmount: 5000}
	settledHash := "0x" + strings.Repeat("11", 32)
	replayHash := "0x" + strings.Repeat("22", 32)

	require.NoError(t, svc.CreateCryptoSettlementReviewAlert(context.Background(), bill, settledHash, nil))
	require.NoError(t, svc.CreateCryptoTxHashConflictAlert(context.Background(), bill, replayHash, nil, map[string]any{"quote_id": 3}))

	alert, meta := requireSingleBillReviewAlert(t, db, bill)
	require.Equal(t, database.OperationalAlertPriorityUrgent, alert.Priority)
	require.Equal(t, "Verified crypto payment failed to settle", alert.Title)
	require.Contains(t, alert.Body, settledHash)
	require.Equal(t, CryptoReviewReasonSettlementFailed, meta["reason"])
	require.Equal(t, settledHash, meta["transaction_hash"])
	require.Equal(t, []string{replayHash}, followUpTxHashes(t, meta))

	// Repeating the same conflict refreshes the follow-up, not a second entry.
	require.NoError(t, svc.CreateCryptoTxHashConflictAlert(context.Background(), bill, replayHash, nil, nil))
	alert, meta = requireSingleBillReviewAlert(t, db, bill)
	require.Equal(t, database.OperationalAlertPriorityUrgent, alert.Priority)
	require.Equal(t, []string{replayHash}, followUpTxHashes(t, meta))
}

// The reverse order: an Urgent settlement failure arriving after a High
// conflict takes the headline, and the conflict stays on the trail.
func TestCryptoSettlementReviewOutranksOpenConflictAndKeepsIt(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	bill := database.Bill{ID: 82, BusinessID: business.ID, BillNumber: "B-82", TotalAmount: 5000}
	replayHash := "0x" + strings.Repeat("33", 32)
	settledHash := "0x" + strings.Repeat("44", 32)

	require.NoError(t, svc.CreateCryptoTxHashConflictAlert(context.Background(), bill, replayHash, nil, nil))
	require.NoError(t, svc.CreateCryptoSettlementReviewAlert(context.Background(), bill, settledHash, nil))

	alert, meta := requireSingleBillReviewAlert(t, db, bill)
	require.Equal(t, database.OperationalAlertPriorityUrgent, alert.Priority)
	require.Equal(t, "Verified crypto payment failed to settle", alert.Title)
	require.Equal(t, settledHash, meta["transaction_hash"])
	require.Equal(t, []string{replayHash}, followUpTxHashes(t, meta))
}

// Review c1 L1: a refused wrong-amount transfer leaves a staff-side record.
// Two different refused transfers on one bill both stay visible, and amounts
// never enter alert metadata.
func TestCreateCryptoAmountMismatchAlertKeepsEveryRefusedTransfer(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	bill := database.Bill{ID: 83, BusinessID: business.ID, BillNumber: "B-83", TotalAmount: 5000}
	firstHash := "0x" + strings.Repeat("55", 32)
	secondHash := "0x" + strings.Repeat("66", 32)

	require.NoError(t, svc.CreateCryptoAmountMismatchAlert(context.Background(), bill, firstHash, map[string]any{
		"quote_id":     5,
		"amount_cents": 5000,
	}))
	alert, meta := requireSingleBillReviewAlert(t, db, bill)
	require.Equal(t, database.OperationalAlertTypePaymentRefundReview, alert.AlertType)
	require.Equal(t, database.OperationalAlertResourceTypeBill, alert.ResourceType)
	require.Equal(t, database.OperationalAlertPriorityHigh, alert.Priority)
	require.Equal(t, CryptoReviewReasonAmountMismatch, meta["reason"])
	require.Equal(t, firstHash, meta["transaction_hash"])
	require.Equal(t, "B-83", meta["bill_number"])
	require.NotContains(t, meta, "amount_cents")
	// R2-M1: the transfer may be someone else's (even one that later settles
	// another bill), so the copy must not invite a manual settle or refund.
	require.Contains(t, alert.Body, "It may belong to another guest")
	require.Contains(t, alert.Body, "no bill has since recorded it")
	require.NotContains(t, alert.Body, "If the guest says they paid")

	require.NoError(t, svc.CreateCryptoAmountMismatchAlert(context.Background(), bill, secondHash, nil))
	_, meta = requireSingleBillReviewAlert(t, db, bill)
	require.Equal(t, firstHash, meta["transaction_hash"])
	require.Equal(t, []string{secondHash}, followUpTxHashes(t, meta))

	// A later settlement failure on the same bill outranks both and keeps them.
	settledHash := "0x" + strings.Repeat("77", 32)
	require.NoError(t, svc.CreateCryptoSettlementReviewAlert(context.Background(), bill, settledHash, nil))
	alert, meta = requireSingleBillReviewAlert(t, db, bill)
	require.Equal(t, database.OperationalAlertPriorityUrgent, alert.Priority)
	require.ElementsMatch(t, []string{firstHash, secondHash}, followUpTxHashes(t, meta))
}

// A MergeKey upsert into a row raised by a producer that does not use merge
// keys (the in-transaction overpaid-cancel alert) keeps that row's headline.
func TestMergeKeyUpsertKeepsForeignHeadline(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	bill := database.Bill{ID: 84, BusinessID: business.ID, BillNumber: "B-84", TotalAmount: 5000}
	_, err := svc.UpsertAlert(context.Background(), UpsertAlertInput{
		BusinessID:   bill.BusinessID,
		AlertType:    database.OperationalAlertTypePaymentRefundReview,
		ResourceType: database.OperationalAlertResourceTypeBill,
		ResourceID:   bill.ID,
		Priority:     database.OperationalAlertPriorityHigh,
		Title:        "Cancelled order left the bill overpaid",
		Body:         "Refund the guest.",
		Metadata:     map[string]any{"order_id": 9},
	})
	require.NoError(t, err)
	replayHash := "0x" + strings.Repeat("88", 32)
	require.NoError(t, svc.CreateCryptoTxHashConflictAlert(context.Background(), bill, replayHash, nil, nil))

	alert, meta := requireSingleBillReviewAlert(t, db, bill)
	require.Equal(t, "Cancelled order left the bill overpaid", alert.Title)
	require.Equal(t, "Refund the guest.", alert.Body)
	require.EqualValues(t, 9, meta["order_id"])
	require.Equal(t, []string{replayHash}, followUpTxHashes(t, meta))
}

func TestAppendFollowUpEventIsBounded(t *testing.T) {
	var trail []any
	for i := 0; i < maxFollowUpEvents+5; i++ {
		trail = appendFollowUpEvent(trail, map[string]any{mergeKeyMetadataKey: strings.Repeat("k", i+1)})
	}
	require.Len(t, trail, maxFollowUpEvents)
	last, _ := trail[len(trail)-1].(map[string]any)
	require.Equal(t, strings.Repeat("k", maxFollowUpEvents+5), last[mergeKeyMetadataKey])
}
