package database

import (
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RevenueMilestoneDefinition struct {
	AmountCents int64
	DisplayText string
}

var revenueMilestoneDefinitions = []RevenueMilestoneDefinition{
	{AmountCents: 100000, DisplayText: "$1,000"},
	{AmountCents: 1000000, DisplayText: "$10,000"},
	{AmountCents: 2500000, DisplayText: "$25,000"},
	{AmountCents: 5000000, DisplayText: "$50,000"},
	{AmountCents: 10000000, DisplayText: "$100,000"},
	{AmountCents: 25000000, DisplayText: "$250,000"},
	{AmountCents: 50000000, DisplayText: "$500,000"},
	{AmountCents: 100000000, DisplayText: "$1,000,000"},
}

const businessMilestoneProcessingTimeout = 15 * time.Minute

func RecordPaymentMilestonesTx(tx *gorm.DB, businessID uint, billID uint, paymentAmountCents int64, tipAmountCents int64, recognizedBillDelta int64, billStatus BillStatus) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}

	if err := lockBusinessMilestoneAggregationTx(tx, businessID); err != nil {
		return err
	}

	if paymentAmountCents != 0 || tipAmountCents != 0 || recognizedBillDelta != 0 {
		previousRevenueCents, totalRevenueCents, err := incrementBusinessRevenueAggregateTx(tx, businessID, paymentAmountCents, tipAmountCents, recognizedBillDelta)
		if err != nil {
			return err
		}
		if paymentAmountCents > 0 {
			if err := recordRevenueMilestonesTx(tx, businessID, billID, previousRevenueCents, totalRevenueCents); err != nil {
				return err
			}
		}
	}

	if billStatus == BillStatusPaid {
		if err := recordFirstOrderMilestoneTx(tx, businessID, billID); err != nil {
			return err
		}
	}
	return nil
}

func ClaimPendingBusinessMilestoneEvents(businessID uint, limit int) ([]BusinessMilestoneEvent, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	now := time.Now()
	processingToken := uuid.NewString()
	var events []BusinessMilestoneEvent
	err := db.Transaction(func(tx *gorm.DB) error {
		query := claimableBusinessMilestoneEventsQuery(tx, businessID).
			Order("created_at ASC, id ASC").
			Limit(limit)
		if tx.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}

		var eventIDs []uint
		if err := query.Pluck("id", &eventIDs).Error; err != nil {
			return err
		}
		if len(eventIDs) == 0 {
			return nil
		}

		if err := tx.Model(&BusinessMilestoneEvent{}).
			Where("id IN ?", eventIDs).
			Updates(map[string]interface{}{
				"status":           BusinessMilestoneStatusProcessing,
				"processing_token": processingToken,
				"last_error":       "",
				"updated_at":       now,
			}).Error; err != nil {
			return err
		}

		return tx.Where("id IN ? AND processing_token = ?", eventIDs, processingToken).
			Order("created_at ASC, id ASC").
			Find(&events).Error
	})
	return events, err
}

func ListBusinessesWithPendingMilestoneEvents(limit int) ([]uint, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	var businessIDs []uint
	err := claimableBusinessMilestoneEventsQuery(db, 0).
		Group("business_id").
		Order("MIN(created_at) ASC").
		Limit(limit).
		Pluck("business_id", &businessIDs).Error
	return businessIDs, err
}

func MarkBusinessMilestoneEventSent(eventID uint, processingToken string) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}

	now := time.Now()
	return db.Model(&BusinessMilestoneEvent{}).
		Where("id = ? AND status = ? AND processing_token = ?", eventID, BusinessMilestoneStatusProcessing, processingToken).
		Updates(map[string]interface{}{
			"status":           BusinessMilestoneStatusSent,
			"sent_at":          &now,
			"processing_token": "",
			"last_error":       "",
			"updated_at":       now,
		}).Error
}

func MarkBusinessMilestoneEventFailed(eventID uint, processingToken string, err error) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}

	message := ""
	if err != nil {
		message = err.Error()
	}
	if len(message) > 500 {
		message = message[:500]
	}
	return db.Model(&BusinessMilestoneEvent{}).
		Where("id = ? AND status = ? AND processing_token = ?", eventID, BusinessMilestoneStatusProcessing, processingToken).
		Updates(map[string]interface{}{
			"status":           BusinessMilestoneStatusFailed,
			"processing_token": "",
			"last_error":       message,
			"updated_at":       time.Now(),
		}).Error
}

func lockBusinessMilestoneAggregationTx(tx *gorm.DB, businessID uint) error {
	var business Business
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id").
		First(&business, businessID).Error; err != nil {
		return fmt.Errorf("failed to lock milestone business: %w", err)
	}
	return nil
}

func incrementBusinessRevenueAggregateTx(tx *gorm.DB, businessID uint, revenueDeltaCents int64, tipDeltaCents int64, recognizedBillDelta int64) (int64, int64, error) {
	grossRevenueDeltaCents, grossTipDeltaCents, positiveEventCountDelta := positiveRevenueAggregateDeltas(revenueDeltaCents, tipDeltaCents)
	var aggregate BusinessRevenueAggregate
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("business_id = ?", businessID).
		First(&aggregate).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		seedTotals, err := recognizedPaymentAggregateTotalsTx(tx, businessID)
		if err != nil {
			return 0, 0, err
		}
		aggregate = BusinessRevenueAggregate{
			BusinessID:          businessID,
			NetRevenueCents:     clampNonNegativeCents(seedTotals.NetRevenueCents),
			NetTipCents:         clampNonNegativeCents(seedTotals.NetTipCents),
			GrossRevenueCents:   clampNonNegativeCents(seedTotals.GrossRevenueCents),
			GrossTipCents:       clampNonNegativeCents(seedTotals.GrossTipCents),
			PositiveEventCount:  seedTotals.PositiveEventCount,
			RecognizedBillCount: seedTotals.RecognizedBillCount,
		}
		if err := tx.Create(&aggregate).Error; err != nil {
			return 0, 0, fmt.Errorf("failed to initialize milestone revenue aggregate: %w", err)
		}
		return seedTotals.NetRevenueCents - revenueDeltaCents, aggregate.NetRevenueCents, nil
	} else if err != nil {
		return 0, 0, fmt.Errorf("failed to load milestone revenue aggregate: %w", err)
	}

	previousRevenueCents := aggregate.NetRevenueCents
	nextRevenueCents := clampNonNegativeCents(aggregate.NetRevenueCents + revenueDeltaCents)
	nextTipCents := clampNonNegativeCents(aggregate.NetTipCents + tipDeltaCents)
	nextGrossRevenueCents := clampNonNegativeCents(aggregate.GrossRevenueCents + grossRevenueDeltaCents)
	nextGrossTipCents := clampNonNegativeCents(aggregate.GrossTipCents + grossTipDeltaCents)
	nextPositiveEventCount := aggregate.PositiveEventCount + positiveEventCountDelta
	if nextPositiveEventCount < 0 {
		nextPositiveEventCount = 0
	}
	nextRecognizedBillCount := aggregate.RecognizedBillCount + recognizedBillDelta
	if nextRecognizedBillCount < 0 {
		nextRecognizedBillCount = 0
	}

	if err := tx.Model(&BusinessRevenueAggregate{}).
		Where("id = ?", aggregate.ID).
		Updates(map[string]interface{}{
			"net_revenue_cents":     nextRevenueCents,
			"net_tip_cents":         nextTipCents,
			"gross_revenue_cents":   nextGrossRevenueCents,
			"gross_tip_cents":       nextGrossTipCents,
			"positive_event_count":  nextPositiveEventCount,
			"recognized_bill_count": nextRecognizedBillCount,
		}).Error; err != nil {
		return 0, 0, fmt.Errorf("failed to update milestone revenue aggregate: %w", err)
	}

	return previousRevenueCents, nextRevenueCents, nil
}

func positiveRevenueAggregateDeltas(revenueDeltaCents int64, tipDeltaCents int64) (int64, int64, int64) {
	if revenueDeltaCents <= 0 && tipDeltaCents <= 0 {
		return 0, 0, 0
	}
	return revenueDeltaCents, tipDeltaCents, 1
}

func clampNonNegativeCents(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

type recognizedPaymentAggregateTotals struct {
	NetRevenueCents     int64 `gorm:"column:net_revenue_cents"`
	NetTipCents         int64 `gorm:"column:net_tip_cents"`
	GrossRevenueCents   int64 `gorm:"column:gross_revenue_cents"`
	GrossTipCents       int64 `gorm:"column:gross_tip_cents"`
	PositiveEventCount  int64 `gorm:"column:positive_event_count"`
	RecognizedBillCount int64 `gorm:"column:recognized_bill_count"`
}

func recognizedPaymentAggregateTotalsTx(tx *gorm.DB, businessID uint) (recognizedPaymentAggregateTotals, error) {
	altPaymentUnion := ""
	altPaymentFallbackExclusion := ""
	args := []interface{}{
		businessID, RecognizedPaymentStatuses, PaymentStatusConfirmed, ReversedOrRefundedPaymentStatuses,
		businessID, ReversedOrRefundedPaymentStatuses,
	}
	if tx.Migrator().HasTable(&AlternativePayment{}) {
		altPaymentUnion = `
			UNION ALL
			SELECT COALESCE(alternative_payments.amount, 0) AS amount_cents,
				COALESCE(alternative_payments.tip_amount_cents, 0) AS tip_cents,
				alternative_payments.bill_id AS bill_id
			FROM alternative_payments
			JOIN bills ON alternative_payments.bill_id = bills.id
			WHERE bills.business_id = ?
				AND alternative_payments.status IN ?
				AND alternative_payments.payment_method IN ?
			UNION ALL
			SELECT (-COALESCE(alternative_payments.amount, 0)) AS amount_cents,
				(-COALESCE(alternative_payments.tip_amount_cents, 0)) AS tip_cents,
				alternative_payments.bill_id AS bill_id
			FROM alternative_payments
			JOIN bills ON alternative_payments.bill_id = bills.id
			WHERE bills.business_id = ?
				AND alternative_payments.status = ?
				AND alternative_payments.payment_method IN ?
		`
		altPaymentFallbackExclusion = `
				AND NOT EXISTS (
					SELECT 1
					FROM alternative_payments recognized_alternative_payments
					WHERE recognized_alternative_payments.bill_id = bills.id
						AND recognized_alternative_payments.status IN ?
						AND recognized_alternative_payments.payment_method IN ?
				)
		`
		args = append(args, businessID, RecognizedAltPaymentStatuses, billManagedAlternativePaymentMethodStrings, businessID, AltPaymentStatusRefunded, billManagedAlternativePaymentMethodStrings)
	}
	args = append(args,
		businessID, 0, BillStatusPaid, 0, RecognizedPaymentStatuses,
	)
	if altPaymentFallbackExclusion != "" {
		args = append(args, RecognizedAltPaymentStatuses, billManagedAlternativePaymentMethodStrings)
	}

	var total recognizedPaymentAggregateTotals
	err := tx.Raw(fmt.Sprintf(`
		WITH recognized_events AS (
			SELECT COALESCE(payments.amount, 0) AS amount_cents,
				COALESCE(payments.tip_amount, 0) AS tip_cents,
				payments.bill_id AS bill_id
			FROM payments
			JOIN bills ON payments.bill_id = bills.id
			WHERE bills.business_id = ?
				AND (
					(payments.status IN ? AND payments.confirmed_at IS NOT NULL)
					OR (payments.status = ? AND payments.confirmed_at IS NULL)
					OR (payments.status IN ? AND payments.confirmed_at IS NULL)
				)
			UNION ALL
			SELECT (-COALESCE(payments.amount, 0)) AS amount_cents,
				(-COALESCE(payments.tip_amount, 0)) AS tip_cents,
				payments.bill_id AS bill_id
			FROM payments
			JOIN bills ON payments.bill_id = bills.id
			WHERE bills.business_id = ?
				AND payments.status IN ?
			%s
			UNION ALL
			SELECT CASE
				WHEN COALESCE(bills.paid_amount, 0) > 0 THEN COALESCE(bills.paid_amount, 0)
				ELSE COALESCE(bills.total_amount, 0)
			END AS amount_cents,
				COALESCE(bills.tip_amount, 0) AS tip_cents,
				bills.id AS bill_id
			FROM bills
			WHERE bills.business_id = ?
				AND (bills.paid_amount > ? OR (bills.status = ? AND bills.total_amount > ?))
				AND NOT EXISTS (
					SELECT 1
					FROM payments recognized_payments
					WHERE recognized_payments.bill_id = bills.id
						AND recognized_payments.status IN ?
				)
				%s
		)
		SELECT
			COALESCE(SUM(amount_cents), 0) AS net_revenue_cents,
			COALESCE(SUM(tip_cents), 0) AS net_tip_cents,
			COALESCE(SUM(CASE WHEN amount_cents > 0 OR tip_cents > 0 THEN amount_cents ELSE 0 END), 0) AS gross_revenue_cents,
			COALESCE(SUM(CASE WHEN amount_cents > 0 OR tip_cents > 0 THEN tip_cents ELSE 0 END), 0) AS gross_tip_cents,
			COALESCE(SUM(CASE WHEN amount_cents > 0 OR tip_cents > 0 THEN 1 ELSE 0 END), 0) AS positive_event_count,
			(
				SELECT COUNT(*)
				FROM (
					SELECT bill_id
					FROM recognized_events
					GROUP BY bill_id
					HAVING SUM(amount_cents) > 0
				) recognized_bills
			) AS recognized_bill_count
		FROM recognized_events
	`, altPaymentUnion, altPaymentFallbackExclusion), args...).Scan(&total).Error
	if err != nil {
		return recognizedPaymentAggregateTotals{}, fmt.Errorf("failed to calculate milestone revenue: %w", err)
	}
	return total, nil
}

// latestUSDToCurrencyRate resolves USD→currency from the stored exchange-rate
// observations, mirroring ExchangeRateService.resolveExchangeRate's cross-rate
// idiom (Coinbase stores USDC legs): direct USD→cur row if present, else
// (USDC→cur)/(USDC→USD). Returns (1, false) when unresolvable — callers fall
// back to 1:1 so a rate outage can never kill milestone recording (P2-9).
func latestUSDToCurrencyRate(tx *gorm.DB, currency string) (float64, bool) {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if cur == "" || cur == "USD" {
		return 1, true
	}
	var direct ExchangeRate
	if err := tx.Where("from_currency = ? AND to_currency = ?", "USD", cur).
		Order("fetched_at DESC").First(&direct).Error; err == nil && direct.Rate > 0 {
		return direct.Rate, true
	}
	var usdcToTarget, usdcToUSD ExchangeRate
	if err := tx.Where("from_currency = ? AND to_currency = ?", "USDC", cur).
		Order("fetched_at DESC").First(&usdcToTarget).Error; err != nil || usdcToTarget.Rate <= 0 {
		return 1, false
	}
	if err := tx.Where("from_currency = ? AND to_currency = ?", "USDC", "USD").
		Order("fetched_at DESC").First(&usdcToUSD).Error; err != nil || usdcToUSD.Rate <= 0 {
		return 1, false
	}
	return usdcToTarget.Rate / usdcToUSD.Rate, true
}

// businessMilestoneCurrencyTx reads only the pricing currency of a business.
func businessMilestoneCurrencyTx(tx *gorm.DB, businessID uint) string {
	var cur string
	if err := tx.Model(&Business{}).Select("default_currency").
		Where("id = ?", businessID).Scan(&cur).Error; err != nil {
		return ""
	}
	return cur
}

// milestoneLocalThresholdCents converts a USD ladder rung into the business
// pricing currency. Comparison happens in local cents; threshold_cents on the
// emitted event keeps the USD rung (stable dedupe key across rate movement).
func milestoneLocalThresholdCents(usdCents int64, rate float64) int64 {
	local := int64(math.Round(float64(usdCents) * rate))
	if local < 1 {
		local = 1
	}
	return local
}

// milestoneDisplayText renders the owner-facing amount: unchanged USD text for
// USD/unconverted businesses; otherwise the local equivalent with the US$
// context — the $1,000 rung at 1450 ARS/USD renders "ARS 1,450,000 (≈ US$1,000)".
func milestoneDisplayText(def RevenueMilestoneDefinition, currency string, localCents int64, converted bool) string {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if !converted || cur == "" || cur == "USD" {
		return def.DisplayText
	}
	return fmt.Sprintf("%s %s (≈ US%s)", cur, formatWholeUnitsWithCommas(localCents/100), def.DisplayText)
}

// formatWholeUnitsWithCommas renders whole currency units with comma
// thousands separators (display only — money math stays integer cents).
func formatWholeUnitsWithCommas(units int64) string {
	s := fmt.Sprintf("%d", units)
	if len(s) <= 3 {
		return s
	}
	var out strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		out.WriteString(s[:lead])
		out.WriteString(",")
	}
	for i := lead; i < len(s); i += 3 {
		out.WriteString(s[i : i+3])
		if i+3 < len(s) {
			out.WriteString(",")
		}
	}
	return out.String()
}

func recordRevenueMilestonesTx(tx *gorm.DB, businessID uint, billID uint, previousRevenueCents int64, totalRevenueCents int64) error {
	if totalRevenueCents <= 0 {
		return nil
	}
	if previousRevenueCents < 0 {
		previousRevenueCents = 0
	}

	// P2-9: revenue aggregates are in BUSINESS-currency cents; the ladder is
	// USD. Convert each rung before comparing; store the USD rung as the
	// dedupe key; render the local amount in the payload.
	currency := businessMilestoneCurrencyTx(tx, businessID)
	rate, converted := latestUSDToCurrencyRate(tx, currency)
	if !converted {
		log.Printf("milestone: no USD→%s rate for business %d; falling back to 1:1 thresholds", currency, businessID)
	}

	for _, milestone := range revenueMilestoneDefinitions {
		localThreshold := milestoneLocalThresholdCents(milestone.AmountCents, rate)
		if previousRevenueCents < localThreshold && totalRevenueCents >= localThreshold {
			event := BusinessMilestoneEvent{
				BusinessID:     businessID,
				MilestoneType:  BusinessMilestoneTypeRevenue,
				ThresholdCents: milestone.AmountCents, // USD rung = stable dedupe key
				Status:         BusinessMilestoneStatusPending,
				Payload: map[string]interface{}{
					"display_text": milestoneDisplayText(milestone, currency, localThreshold, converted),
				},
			}
			if billID > 0 {
				event.BillID = &billID
			}
			if err := insertBusinessMilestoneEventTx(tx, &event); err != nil {
				return err
			}
		}
	}
	return nil
}

func recordFirstOrderMilestoneTx(tx *gorm.DB, businessID uint, billID uint) error {
	var existingCount int64
	if err := tx.Model(&BusinessMilestoneEvent{}).
		Where("business_id = ? AND milestone_type = ? AND threshold_cents = ?", businessID, BusinessMilestoneTypeFirstOrder, 0).
		Count(&existingCount).Error; err != nil {
		return fmt.Errorf("failed to check first-order milestone: %w", err)
	}
	if existingCount > 0 {
		return nil
	}

	var paidBillCount int64
	if err := tx.Model(&Bill{}).
		Where("business_id = ? AND status = ?", businessID, BillStatusPaid).
		Count(&paidBillCount).Error; err != nil {
		return fmt.Errorf("failed to count paid bills for first-order milestone: %w", err)
	}
	if paidBillCount != 1 {
		return nil
	}

	event := BusinessMilestoneEvent{
		BusinessID:     businessID,
		MilestoneType:  BusinessMilestoneTypeFirstOrder,
		ThresholdCents: 0,
		Status:         BusinessMilestoneStatusPending,
	}
	if billID > 0 {
		event.BillID = &billID
	}
	return insertBusinessMilestoneEventTx(tx, &event)
}

func insertBusinessMilestoneEventTx(tx *gorm.DB, event *BusinessMilestoneEvent) error {
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "business_id"},
			{Name: "milestone_type"},
			{Name: "threshold_cents"},
		},
		DoNothing: true,
	}).Create(event).Error
}

func claimableBusinessMilestoneEventsQuery(tx *gorm.DB, businessID uint) *gorm.DB {
	staleBefore := time.Now().Add(-businessMilestoneProcessingTimeout)
	query := tx.Model(&BusinessMilestoneEvent{}).
		Where("(status IN ? OR (status = ? AND updated_at < ?))",
			[]BusinessMilestoneStatus{BusinessMilestoneStatusPending, BusinessMilestoneStatusFailed},
			BusinessMilestoneStatusProcessing,
			staleBefore,
		)
	if businessID != 0 {
		query = query.Where("business_id = ?", businessID)
	}
	return query
}

func RevenueMilestoneDisplayText(thresholdCents int64) string {
	for _, milestone := range revenueMilestoneDefinitions {
		if milestone.AmountCents == thresholdCents {
			return milestone.DisplayText
		}
	}
	if thresholdCents <= 0 {
		return "$0"
	}
	return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("$%.2f", float64(thresholdCents)/100.0), "0"), ".")
}
