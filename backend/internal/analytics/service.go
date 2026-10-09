package analytics

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/reporting"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"gorm.io/gorm"
)

type AnalyticsService struct {
	db  *database.DB
	now func() time.Time
}

func NewAnalyticsService(db *database.DB) *AnalyticsService {
	return &AnalyticsService{db: db, now: time.Now}
}

// WithClock overrides the time source used by day-bucket classification.
// Tests use this to pin "now" so the pass/fail outcome doesn't race against
// midnight boundaries between fixture setup and assertion.
func (s *AnalyticsService) WithClock(now func() time.Time) *AnalyticsService {
	s.now = now
	return s
}

// SalesReport represents daily sales data
type SalesReport struct {
	Date             time.Time      `json:"date"`
	BusinessID       uint           `json:"business_id"`
	TotalRevenue     float64        `json:"total_revenue"`
	TotalTips        float64        `json:"total_tips"`
	TransactionCount int            `json:"transaction_count"`
	BillCount        int            `json:"bill_count"`
	AverageTicket    float64        `json:"average_ticket"`
	PaymentMethods   map[string]int `json:"payment_methods"`
	HourlyBreakdown  []HourlySales  `json:"hourly_breakdown"`
}

type HourlySales struct {
	Hour      int     `json:"hour"`
	Revenue   float64 `json:"revenue"`
	Tips      float64 `json:"tips"`
	BillCount int     `json:"bill_count"`
}

// ItemStats represents menu item performance
type ItemStats struct {
	ItemID    string `json:"item_id"`
	ItemName  string `json:"item_name"`
	Category  string `json:"category"`
	TotalSold int    `json:"total_sold"`
	// RecognizedQuantity is quantity * payment recognition ratio, unrounded.
	// Zero-revenue lines (bundle components priced inside a combo) still carry
	// their quantity; COGS and theoretical usage must use this, not TotalSold.
	RecognizedQuantity float64 `json:"recognized_quantity"`
	Revenue            float64 `json:"revenue"`
	AveragePrice       float64 `json:"avg_price"`
	BillsFeatured      int     `json:"bills_featured"`  // number of bills containing this item
	Popularity         float64 `json:"popularity_rank"` // percentage of bills containing this item
}

// TipReport represents tip analytics
type TipReport struct {
	BusinessID      uint               `json:"business_id"`
	Period          string             `json:"period"`
	TotalTips       float64            `json:"total_tips"`
	TipCount        int                `json:"tip_count"`
	AverageTip      float64            `json:"average_tip"`
	AverageTipRate  float64            `json:"average_tip_rate"` // percentage of bill
	TipDistribution map[string]int     `json:"tip_distribution"` // tip ranges
	TopTippers      []TipperInfo       `json:"top_tippers"`
	HourlyTips      map[string]float64 `json:"hourly_tips"`
	DailyComparison DailyTipComparison `json:"daily_comparison"`
}

type TipperInfo struct {
	PayerAddress string `json:"payer_address"`
	// GuestName is the CRM customer display name when the payer wallet is
	// linked to a guest for this business. Staff-facing tips UI must prefer
	// this over the raw wallet address.
	GuestName  string  `json:"guest_name,omitempty"`
	TotalTips  float64 `json:"total_tips"`
	TipCount   int     `json:"tip_count"`
	AverageTip float64 `json:"average_tip"`
}

type DailyTipComparison struct {
	Today            float64 `json:"today"`
	Yesterday        float64 `json:"yesterday"`
	ChangePercentage float64 `json:"change_percentage"`
}

// PeriodReport represents analytics for a specific time period
type PeriodReport struct {
	StartDate        time.Time             `json:"start_date"`
	EndDate          time.Time             `json:"end_date"`
	TotalRevenue     float64               `json:"total_revenue"`
	TotalTips        float64               `json:"total_tips"`
	TransactionCount int                   `json:"transaction_count"`
	BillCount        int                   `json:"bill_count"`
	UniqueCustomers  int                   `json:"unique_customers"`
	AverageTicket    float64               `json:"average_ticket"`
	GrowthRate       *float64              `json:"growth_rate,omitempty"` // prior calendar period; nil = no baseline
	PaymentMethods   map[string]int        `json:"payment_methods"`
	HourlyBreakdown  map[string]HourlyData `json:"hourly_breakdown"`
	// CollectedRevenue and FloorRemaining are populated for today/day only.
	// TotalRevenue stays collected; remaining is labeled separately (#703).
	CollectedRevenue float64 `json:"collected_revenue,omitempty"`
	FloorRemaining   float64 `json:"floor_remaining,omitempty"`
}

// PaymentWindowSummary is the compact ledger-derived aggregate used by summary
// surfaces that do not need full hourly/payment-method breakdowns.
type PaymentWindowSummary struct {
	StartDate        time.Time `json:"start_date"`
	EndDate          time.Time `json:"end_date"`
	TotalRevenue     float64   `json:"total_revenue"`
	TotalTips        float64   `json:"total_tips"`
	TransactionCount int       `json:"transaction_count"`
	BillCount        int       `json:"bill_count"`
	UniqueCustomers  int       `json:"unique_customers"`
	AverageTicket    float64   `json:"average_ticket"`
}

// HourlyData represents aggregated hourly data
type HourlyData struct {
	Revenue          float64 `json:"revenue"`
	BillCount        int     `json:"bill_count"`
	TransactionCount int     `json:"transaction_count"`
}

type recognizedPaymentHourlyAggregate struct {
	Hour             int   `gorm:"column:hour"`
	RevenueCents     int64 `gorm:"column:revenue_cents"`
	TipCents         int64 `gorm:"column:tip_cents"`
	BillCount        int   `gorm:"column:bill_count"`
	TransactionCount int   `gorm:"column:transaction_count"`
}

type paymentWindowSummaryRow struct {
	TotalRevenueCents int64 `gorm:"column:total_revenue_cents"`
	TotalTipCents     int64 `gorm:"column:total_tip_cents"`
	TransactionCount  int   `gorm:"column:transaction_count"`
	BillCount         int   `gorm:"column:bill_count"`
	UniqueCustomers   int   `gorm:"column:unique_customers"`
}

type recognizedPaymentPeriodMetricsRow struct {
	RowKind                string `gorm:"column:row_kind"`
	Hour                   *int   `gorm:"column:hour"`
	PaymentMethod          string `gorm:"column:payment_method"`
	RevenueCents           int64  `gorm:"column:revenue_cents"`
	TipCents               int64  `gorm:"column:tip_cents"`
	BillCount              int    `gorm:"column:bill_count"`
	TransactionCount       int    `gorm:"column:transaction_count"`
	TotalRevenueCents      int64  `gorm:"column:total_revenue_cents"`
	TotalTipCents          int64  `gorm:"column:total_tip_cents"`
	PeriodTransactionCount int    `gorm:"column:period_transaction_count"`
	PeriodBillCount        int    `gorm:"column:period_bill_count"`
	UniqueCustomers        int    `gorm:"column:unique_customers"`
}

type recognizedTipMetricsRow struct {
	RowKind             string `gorm:"column:row_kind"`
	Hour                *int   `gorm:"column:hour"`
	Bucket              string `gorm:"column:bucket"`
	PayerAddr           string `gorm:"column:payer_addr"`
	TotalTipCents       int64  `gorm:"column:total_tip_cents"`
	PositiveTipCents    int64  `gorm:"column:positive_tip_cents"`
	PositiveAmountCents int64  `gorm:"column:positive_amount_cents"`
	PositiveTipCount    int    `gorm:"column:positive_tip_count"`
	TipCents            int64  `gorm:"column:tip_cents"`
	TipCount            int    `gorm:"column:tip_count"`
	TodayTipCents       int64  `gorm:"column:today_tip_cents"`
	YesterdayTipCents   int64  `gorm:"column:yesterday_tip_cents"`
}

var confirmedAlternativePaymentMethods = []database.AlternativePaymentMethod{
	database.PaymentMethodCash,
	database.PaymentMethodCard,
	database.PaymentMethodVenmo,
	database.PaymentMethodOther,
}

// GetDailySales returns sales data for a specific date
func (s *AnalyticsService) GetDailySales(businessID uint, date time.Time) (*SalesReport, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.AddDate(0, 0, 1) // next local midnight (DST-safe)

	events, err := database.GetRecognizedPaymentEvents(businessID, startOfDay, endOfDay)
	if err != nil {
		return nil, fmt.Errorf("failed to get recognized payment events: %w", err)
	}

	report := &SalesReport{
		Date:            date,
		BusinessID:      businessID,
		PaymentMethods:  make(map[string]int),
		HourlyBreakdown: make([]HourlySales, 24),
	}

	// Initialize hourly breakdown
	for i := 0; i < 24; i++ {
		report.HourlyBreakdown[i] = HourlySales{Hour: i}
	}

	// Count a bill only if its NET recognized amount is positive, matching
	// GetPaymentWindowSummary's `HAVING SUM(amount_cents) > 0`. A same-day
	// fully-refunded bill emits a +X and a dated -X event; counting it (as the
	// old raw event-set did) inflated BillCount and halved AverageTicket relative
	// to the dashboard's GetPeriodReport for the identical day.
	billNet := make(map[uint]int64)
	hourlyBillEvents := make(map[int]map[uint]struct{})
	for _, event := range events {
		report.TotalRevenue += centsToDollars(event.AmountCents)
		report.TotalTips += centsToDollars(event.TipCents)
		report.TransactionCount++
		report.PaymentMethods[event.PaymentMethod]++
		billNet[event.BillID] += event.AmountCents

		// Bucket by the business-local hour (the window's location), not the
		// stored UTC hour, so a 23:00-local sale isn't filed under a UTC hour
		// that belongs to a different calendar day.
		hour := event.RecognizedAt.In(date.Location()).Hour()
		if hour < 0 || hour >= 24 {
			continue
		}
		report.HourlyBreakdown[hour].Revenue += centsToDollars(event.AmountCents)
		report.HourlyBreakdown[hour].Tips += centsToDollars(event.TipCents)
		if _, exists := hourlyBillEvents[hour]; !exists {
			hourlyBillEvents[hour] = make(map[uint]struct{})
		}
		hourlyBillEvents[hour][event.BillID] = struct{}{}
	}

	positiveBills := make(map[uint]struct{}, len(billNet))
	for billID, net := range billNet {
		if net > 0 {
			positiveBills[billID] = struct{}{}
		}
	}
	report.BillCount = len(positiveBills)
	for hour, ids := range hourlyBillEvents {
		count := 0
		for billID := range ids {
			if _, ok := positiveBills[billID]; ok {
				count++
			}
		}
		report.HourlyBreakdown[hour].BillCount = count
	}

	// Calculate average ticket
	if report.BillCount > 0 {
		report.AverageTicket = report.TotalRevenue / float64(report.BillCount)
	}

	return report, nil
}

func (s *AnalyticsService) GetPaymentPeriodSummary(businessID uint, period string, loc *time.Location) (*PaymentWindowSummary, error) {
	startDate, endDate, err := s.parsePeriod(period, loc)
	if err != nil {
		return nil, fmt.Errorf("invalid period: %w", err)
	}
	return s.GetPaymentWindowSummary(businessID, startDate, endDate)
}

func (s *AnalyticsService) GetPaymentWindowSummary(businessID uint, startDate, endDate time.Time) (*PaymentWindowSummary, error) {
	recognizedSQL, args := s.recognizedPaymentBillAmountsSubquerySQL(businessID, startDate, endDate)
	recognizedCTE := s.recognizedEventsCTE(recognizedSQL)

	var row paymentWindowSummaryRow
	if err := s.db.GetGorm().Raw(fmt.Sprintf(`
		WITH %s
		SELECT
			COALESCE(SUM(recognized_events.amount_cents), 0) AS total_revenue_cents,
			COALESCE(SUM(recognized_events.tip_cents), 0) AS total_tip_cents,
			COUNT(*) AS transaction_count,
			(
				SELECT COUNT(*)
				FROM (
					SELECT bill_id
					FROM recognized_events
					GROUP BY bill_id
					HAVING SUM(amount_cents) > 0
				) AS recognized_positive_bills
			) AS bill_count,
			COUNT(DISTINCT CASE WHEN recognized_events.customer_id <> '' THEN recognized_events.customer_id END) AS unique_customers
		FROM recognized_events
	`, recognizedCTE), args...).Scan(&row).Error; err != nil {
		return nil, err
	}

	summary := &PaymentWindowSummary{
		StartDate:        startDate,
		EndDate:          endDate,
		TotalRevenue:     centsToDollars(row.TotalRevenueCents),
		TotalTips:        centsToDollars(row.TotalTipCents),
		TransactionCount: row.TransactionCount,
		BillCount:        row.BillCount,
		UniqueCustomers:  row.UniqueCustomers,
	}
	if summary.BillCount > 0 {
		summary.AverageTicket = summary.TotalRevenue / float64(summary.BillCount)
	}

	return summary, nil
}

// LiveOpenCheckRemaining is remaining due on every open/partial check, not
// just bills opened today. Callers must label this remaining — never fold it
// into ventas / ingresos cobrado (#703).
func (s *AnalyticsService) LiveOpenCheckRemaining(businessID uint) (float64, error) {
	return s.openCheckRemaining(businessID, time.Time{}, time.Time{})
}

func (s *AnalyticsService) openCheckRemaining(businessID uint, start, end time.Time) (float64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	q := s.db.GetGorm().Table("bills").
		Select("COALESCE(SUM(total_amount - paid_amount), 0)").
		Where("business_id = ? AND status IN ? AND total_amount > paid_amount",
			businessID,
			[]database.BillStatus{database.BillStatusOpen, database.BillStatusPartial})
	if !start.IsZero() && !end.IsZero() {
		q = q.Where("created_at >= ? AND created_at < ?", start, end)
	}
	var remainingCents int64
	if err := q.Scan(&remainingCents).Error; err != nil {
		return 0, err
	}
	return centsToDollars(remainingCents), nil
}

// LiveKitchenTicketCount is tonight's live kitchen work: pending / approved /
// in_kitchen tickets regardless of created_at. Overnight tickets still on the
// board must count as today's orders (#702 still-broken 2026-08-20).
func (s *AnalyticsService) LiveKitchenTicketCount(businessID uint) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	var n int64
	err := s.db.GetGorm().Model(&database.Order{}).
		Where("business_id = ? AND status IN ?",
			businessID,
			[]database.OrderStatus{
				database.OrderStatusPending,
				database.OrderStatusApproved,
				database.OrderStatusInKitchen,
			}).
		Count(&n).Error
	if err != nil {
		return 0, err
	}
	return n, nil
}

// MaxPopularItemsLimit is the hard cap for popular-item aggregates. A larger
// limit is clamped so a query parameter cannot reach SQL. Zero and negative
// limits stay unbounded for callers that genuinely need every item.
const MaxPopularItemsLimit = 100

// GetPopularItems returns item performance statistics ordered by revenue
// descending. limit controls how many rows are returned; a value of 0 (or
// negative) means "no limit" and preserves the previous unbounded behaviour.
// Values above MaxPopularItemsLimit are clamped. Dashboard callers should
// pass limit=5; callers that need all items pass 0.
func (s *AnalyticsService) GetPopularItems(businessID uint, limit int, period string, loc *time.Location) ([]ItemStats, error) {
	if limit > MaxPopularItemsLimit {
		limit = MaxPopularItemsLimit
	}
	startDate, endDate, err := s.parsePeriod(period, loc)
	if err != nil {
		return nil, fmt.Errorf("invalid period: %w", err)
	}
	return s.GetPopularItemsInWindow(businessID, limit, startDate, endDate)
}

// GetPopularItemsInWindow is the date-bounded popular-items aggregate used by
// food-cost / menu-engineering when callers pass an explicit [start, end)
// reporting window (accounting Overview date range). Same SQL shape as the
// period presets — no full-table scan beyond the recognized-payment window.
func (s *AnalyticsService) GetPopularItemsInWindow(businessID uint, limit int, startDate, endDate time.Time) ([]ItemStats, error) {
	var stats []struct {
		ItemID        string  `gorm:"column:item_id"`
		Name          string  `gorm:"column:name"`
		TotalQuantity float64 `gorm:"column:total_quantity"`
		Revenue       float64 `gorm:"column:revenue"`
		BillsFeatured int     `gorm:"column:bills_featured"`
		TotalBills    int64   `gorm:"column:total_bills"`
	}

	// Build an optional LIMIT clause applied inside the SQL so only the top-N
	// rows are fetched and processed, not the whole item set.
	limitClause := ""
	limitArgs := []interface{}{}
	if limit > 0 {
		limitClause = "\n\t\tLIMIT ?"
		limitArgs = append(limitArgs, limit)
	}

	recognizedSQL, recognizedArgs := s.recognizedPaymentBillAmountsSubquerySQL(businessID, startDate, endDate)
	recognizedCTE := s.recognizedEventsCTE(recognizedSQL)
	queryArgs := append(recognizedArgs, businessID)
	queryArgs = append(queryArgs, limitArgs...)
	err := s.db.GetGorm().Raw(fmt.Sprintf(`
		WITH %s,
		recognized_bills AS (
			SELECT
				recognized_amounts.bill_id,
				CASE
					WHEN bills.total_amount > 0 THEN CAST(SUM(recognized_amounts.amount_cents) AS REAL) / bills.total_amount
					ELSE 0
				END AS recognition_ratio
			FROM recognized_events AS recognized_amounts
			JOIN bills ON recognized_amounts.bill_id = bills.id
			WHERE bills.business_id = ?
				AND EXISTS (
					SELECT 1 FROM bill_items
					WHERE bill_items.bill_id = recognized_amounts.bill_id
				)
			GROUP BY recognized_amounts.bill_id, bills.total_amount
			HAVING SUM(recognized_amounts.amount_cents) > 0
		),
		eligible_bill_count AS (
			SELECT COUNT(*) AS total_bills
			FROM recognized_bills
		)
		SELECT
			CASE
				WHEN TRIM(COALESCE(bill_items.menu_item_id, '')) <> '' THEN bill_items.menu_item_id
				ELSE bill_items.name
			END AS item_id,
			MAX(bill_items.name) AS name,
			COALESCE(SUM(bill_items.quantity * recognized_bills.recognition_ratio), 0) AS total_quantity,
			COALESCE(SUM(bill_items.subtotal * recognized_bills.recognition_ratio), 0) AS revenue,
			COUNT(DISTINCT bill_items.bill_id) AS bills_featured,
			eligible_bill_count.total_bills AS total_bills
		FROM bill_items
		JOIN recognized_bills ON recognized_bills.bill_id = bill_items.bill_id
		JOIN eligible_bill_count ON 1 = 1
		GROUP BY
			CASE
				WHEN TRIM(COALESCE(bill_items.menu_item_id, '')) <> '' THEN bill_items.menu_item_id
				ELSE bill_items.name
			END,
			eligible_bill_count.total_bills
		ORDER BY revenue DESC%s
	`, recognizedCTE, limitClause), queryArgs...).Scan(&stats).Error

	if err != nil {
		return nil, fmt.Errorf("failed to aggregate bill items: %w", err)
	}

	// Get menu to map item names to categories and translated names
	itemCategoryMap, itemNameMap, err := s.getItemCategoryMapping(businessID)
	if err != nil {
		itemCategoryMap = make(map[string]string)
		itemNameMap = make(map[string]string)
	}

	totalBills := int64(0)
	if len(stats) > 0 {
		totalBills = stats[0].TotalBills
	}

	var result []ItemStats
	for _, s := range stats {
		category := "General"
		if cat, found := itemCategoryMap[s.Name]; found {
			category = cat
		}

		displayName := s.Name
		if trans, found := itemNameMap[s.Name]; found {
			displayName = trans
		}

		itemStat := ItemStats{
			ItemID:             s.ItemID,
			ItemName:           displayName,
			Category:           category,
			TotalSold:          recognizedQuantityToInt(s.TotalQuantity, s.Revenue),
			RecognizedQuantity: s.TotalQuantity,
			Revenue:            s.Revenue,
			BillsFeatured:      s.BillsFeatured,
		}

		// Average price divides recognized revenue by the RECOGNIZED (float)
		// quantity, not the rounded display count — a half-recognized bill
		// (partial payment) contributes 0.5 qty and 0.5×price revenue, and
		// dividing by the rounded "1" halves the item's apparent unit price.
		if s.TotalQuantity > 0 {
			itemStat.AveragePrice = s.Revenue / s.TotalQuantity
		} else if itemStat.TotalSold > 0 {
			itemStat.AveragePrice = s.Revenue / float64(itemStat.TotalSold)
		}

		if totalBills > 0 {
			itemStat.Popularity = (float64(s.BillsFeatured) / float64(totalBills)) * 100
		}

		result = append(result, itemStat)
	}

	return result, nil
}

// GetTipAnalytics returns tip statistics for a period
func (s *AnalyticsService) GetTipAnalytics(businessID uint, period string, loc *time.Location) (*TipReport, error) {
	loc = locOrUTC(loc)
	startDate, endDate, err := s.parsePeriod(period, loc)
	if err != nil {
		return nil, fmt.Errorf("invalid period: %w", err)
	}

	report := &TipReport{
		BusinessID:      businessID,
		Period:          period,
		TipDistribution: make(map[string]int),
		HourlyTips:      make(map[string]float64),
	}

	now := s.now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	yesterdayStart := todayStart.AddDate(0, 0, -1) // prior local midnight (DST-safe)

	tipRows, err := s.aggregateRecognizedTipMetrics(businessID, startDate, endDate, todayStart, yesterdayStart)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate recognized tip metrics: %w", err)
	}

	tipperMap := make(map[string]*TipperInfo)
	for _, row := range tipRows {
		switch row.RowKind {
		case "totals":
			report.TotalTips = centsToDollars(row.TotalTipCents)
			report.TipCount = row.PositiveTipCount
			if row.PositiveTipCount > 0 {
				report.AverageTip = centsToDollars(row.PositiveTipCents) / float64(row.PositiveTipCount)
			}
			if row.PositiveAmountCents > 0 {
				report.AverageTipRate = (float64(row.PositiveTipCents) / float64(row.PositiveAmountCents)) * 100
			}
			report.DailyComparison.Today = centsToDollars(row.TodayTipCents)
			report.DailyComparison.Yesterday = centsToDollars(row.YesterdayTipCents)
		case "hour":
			if row.Hour != nil {
				report.HourlyTips[fmt.Sprintf("%d", *row.Hour)] = centsToDollars(row.TipCents)
			}
		case "distribution":
			if row.Bucket != "" {
				report.TipDistribution[row.Bucket] = row.TipCount
			}
		case "tipper":
			if row.PayerAddr == "" || row.TipCents <= 0 || row.TipCount == 0 {
				continue
			}
			if _, exists := tipperMap[row.PayerAddr]; !exists {
				tipperMap[row.PayerAddr] = &TipperInfo{
					PayerAddress: row.PayerAddr,
				}
			}
			tipper := tipperMap[row.PayerAddr]
			tipper.TotalTips += centsToDollars(row.TipCents)
			tipper.TipCount += row.TipCount
		}
	}

	// Calculate average tips for each tipper and sort
	for _, tipper := range tipperMap {
		if tipper.TipCount == 0 || tipper.TotalTips <= 0 {
			continue
		}
		tipper.AverageTip = tipper.TotalTips / float64(tipper.TipCount)
		report.TopTippers = append(report.TopTippers, *tipper)
	}

	sort.Slice(report.TopTippers, func(i, j int) bool {
		if report.TopTippers[i].TotalTips == report.TopTippers[j].TotalTips {
			return report.TopTippers[i].TipCount > report.TopTippers[j].TipCount
		}
		return report.TopTippers[i].TotalTips > report.TopTippers[j].TotalTips
	})

	switch {
	case report.DailyComparison.Yesterday > 0:
		report.DailyComparison.ChangePercentage = ((report.DailyComparison.Today - report.DailyComparison.Yesterday) / report.DailyComparison.Yesterday) * 100
	case report.DailyComparison.Today > 0:
		report.DailyComparison.ChangePercentage = 100
	}

	s.attachTipperGuestNames(businessID, report.TopTippers)

	return report, nil
}

// attachTipperGuestNames fills GuestName from CRM customers linked to this
// business whose wallet matches the tip payer address (case-insensitive).
// Unmatched tippers keep an empty GuestName so the UI can anonymize rather
// than leak raw wallet hex on a staff-facing screen.
func (s *AnalyticsService) attachTipperGuestNames(businessID uint, tippers []TipperInfo) {
	if len(tippers) == 0 {
		return
	}
	addrs := make([]string, 0, len(tippers))
	seen := make(map[string]struct{}, len(tippers))
	for _, tipper := range tippers {
		addr := strings.ToLower(strings.TrimSpace(tipper.PayerAddress))
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		addrs = append(addrs, addr)
	}
	if len(addrs) == 0 {
		return
	}

	type nameRow struct {
		Wallet string `gorm:"column:wallet"`
		Name   string `gorm:"column:name"`
	}
	var rows []nameRow
	err := s.db.GetGorm().Raw(`
		SELECT LOWER(TRIM(c.wallet_address)) AS wallet, TRIM(c.name) AS name
		FROM customers c
		INNER JOIN customer_businesses cb ON cb.customer_id = c.id
		WHERE cb.business_id = ?
			AND c.wallet_address <> ''
			AND LOWER(TRIM(c.wallet_address)) IN ?
			AND TRIM(c.name) <> ''
	`, businessID, addrs).Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return
	}

	byWallet := make(map[string]string, len(rows))
	for _, row := range rows {
		if row.Wallet == "" || row.Name == "" {
			continue
		}
		byWallet[row.Wallet] = row.Name
	}
	for i := range tippers {
		addr := strings.ToLower(strings.TrimSpace(tippers[i].PayerAddress))
		if name, ok := byWallet[addr]; ok {
			tippers[i].GuestName = name
		}
	}
}

// GetPeriodReport returns comprehensive analytics for a preset time period.
func (s *AnalyticsService) GetPeriodReport(businessID uint, period string, loc *time.Location) (*PeriodReport, error) {
	return s.GetPeriodReportRange(businessID, period, nil, nil, loc)
}

// GetPeriodReportRange is GetPeriodReport with an optional explicit
// [start, end) window. A non-nil pair wins over the preset (reporting's
// documented precedence) and an inverted or one-sided pair is an error, so a
// hand-built range can no longer be dropped in favour of the default period
// (#925). Both nil keeps the preset behaviour byte-for-byte.
func (s *AnalyticsService) GetPeriodReportRange(businessID uint, period string, start, end *time.Time, loc *time.Location) (*PeriodReport, error) {
	startDate, endDate, err := s.resolveWindow(period, start, end, loc)
	if err != nil {
		return nil, fmt.Errorf("invalid period: %w", err)
	}

	windowSummary, hourlyRows, paymentMethods, err := s.aggregateRecognizedPaymentPeriodMetrics(businessID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate recognized payment period metrics: %w", err)
	}

	report := &PeriodReport{
		StartDate:       startDate,
		EndDate:         endDate,
		PaymentMethods:  make(map[string]int),
		HourlyBreakdown: make(map[string]HourlyData),
	}

	// Initialize hourly breakdown for all hours
	for i := 0; i < 24; i++ {
		hourKey := fmt.Sprintf("%d", i)
		report.HourlyBreakdown[hourKey] = HourlyData{}
	}

	report.TotalRevenue = windowSummary.TotalRevenue
	report.TotalTips = windowSummary.TotalTips
	report.TransactionCount = windowSummary.TransactionCount
	report.BillCount = windowSummary.BillCount
	report.UniqueCustomers = windowSummary.UniqueCustomers
	report.PaymentMethods = paymentMethods
	// The live floor overlay only makes sense for the service day itself; an
	// explicit custom range is a historical window, so today's open checks must
	// not be grafted onto it.
	if start == nil && end == nil && (period == "today" || period == "day") {
		report.CollectedRevenue = windowSummary.TotalRevenue
		if openRemaining, openErr := s.LiveOpenCheckRemaining(businessID); openErr == nil {
			report.FloorRemaining = openRemaining
		}
	}

	for _, row := range hourlyRows {
		hourKey := fmt.Sprintf("%d", row.Hour)
		if hourData, exists := report.HourlyBreakdown[hourKey]; exists {
			hourData.Revenue += centsToDollars(row.RevenueCents)
			hourData.BillCount += row.BillCount
			hourData.TransactionCount += row.TransactionCount
			report.HourlyBreakdown[hourKey] = hourData
		}
	}

	// Average ticket is collected revenue / closed-or-net-positive bills.
	// Live floor remaining is not sold as today's sales (#703).
	if report.BillCount > 0 {
		report.AverageTicket = report.TotalRevenue / float64(report.BillCount)
	}

	// Growth vs prior calendar period of the same shape (L6-10). Duration
	// shift would mis-label "vs año/mes previo"; zero prior revenue is no
	// baseline (omit growth_rate) rather than a fabricated 100%.
	win := reporting.Window{
		Start: startDate.UTC(),
		End:   endDate.UTC(),
		Label: period,
		Loc:   locOrUTC(loc),
	}
	prior := reporting.PriorWindow(win)
	prevSummary, err := database.GetRecognizedPaymentSummary(businessID, prior.Start, prior.End)
	if err == nil {
		prevRevenue := centsToDollars(prevSummary.TotalRevenueCents)
		if prevRevenue > 0 {
			rate := ((report.TotalRevenue - prevRevenue) / prevRevenue) * 100
			report.GrowthRate = &rate
		}
		// prevRevenue == 0 → leave GrowthRate nil (no baseline)
	}

	return report, nil
}

func (s *AnalyticsService) aggregateRecognizedPaymentPeriodMetrics(
	businessID uint,
	startDate, endDate time.Time,
) (*PaymentWindowSummary, []recognizedPaymentHourlyAggregate, map[string]int, error) {
	recognizedSQL, args := s.recognizedPaymentBillAmountsSubquerySQL(businessID, startDate, endDate)
	recognizedCTE := s.recognizedEventsCTE(recognizedSQL)
	hourExpr := s.hourExpression("recognized_events.recognized_at", startDate.Location())

	var rows []recognizedPaymentPeriodMetricsRow
	if err := s.db.GetGorm().Raw(fmt.Sprintf(`
		WITH %s,
		period_totals AS (
			SELECT
				COALESCE(SUM(recognized_events.amount_cents), 0) AS total_revenue_cents,
				COALESCE(SUM(recognized_events.tip_cents), 0) AS total_tip_cents,
				COUNT(*) AS transaction_count,
				(
					SELECT COUNT(*)
					FROM (
						SELECT bill_id
						FROM recognized_events
						GROUP BY bill_id
						HAVING SUM(amount_cents) > 0
					) AS recognized_positive_bills
				) AS bill_count,
				COUNT(DISTINCT CASE WHEN recognized_events.customer_id <> '' THEN recognized_events.customer_id END) AS unique_customers
			FROM recognized_events
		),
		hour_totals AS (
			SELECT
				hour_bill_totals.hour AS hour,
				COALESCE(SUM(hour_bill_totals.revenue_cents), 0) AS revenue_cents,
				COALESCE(SUM(hour_bill_totals.tip_cents), 0) AS tip_cents,
				COALESCE(SUM(CASE WHEN hour_bill_totals.revenue_cents > 0 THEN 1 ELSE 0 END), 0) AS bill_count,
				COALESCE(SUM(hour_bill_totals.transaction_count), 0) AS transaction_count
			FROM (
				SELECT
					%s AS hour,
					recognized_events.bill_id AS bill_id,
					COALESCE(SUM(recognized_events.amount_cents), 0) AS revenue_cents,
					COALESCE(SUM(recognized_events.tip_cents), 0) AS tip_cents,
					COUNT(*) AS transaction_count
				FROM recognized_events
				GROUP BY %s, recognized_events.bill_id
			) AS hour_bill_totals
			GROUP BY hour_bill_totals.hour
		),
		method_totals AS (
			SELECT
				recognized_events.payment_method AS payment_method,
				COUNT(*) AS transaction_count
			FROM recognized_events
			GROUP BY recognized_events.payment_method
		)
		SELECT
			'hour' AS row_kind,
			hour_totals.hour AS hour,
			'' AS payment_method,
			COALESCE(hour_totals.revenue_cents, 0) AS revenue_cents,
			COALESCE(hour_totals.tip_cents, 0) AS tip_cents,
			COALESCE(hour_totals.bill_count, 0) AS bill_count,
			COALESCE(hour_totals.transaction_count, 0) AS transaction_count,
			period_totals.total_revenue_cents AS total_revenue_cents,
			period_totals.total_tip_cents AS total_tip_cents,
			period_totals.transaction_count AS period_transaction_count,
			period_totals.bill_count AS period_bill_count,
			period_totals.unique_customers AS unique_customers
		FROM period_totals
		LEFT JOIN hour_totals ON 1 = 1
		UNION ALL
		SELECT
			'method' AS row_kind,
			NULL AS hour,
			method_totals.payment_method AS payment_method,
			0 AS revenue_cents,
			0 AS tip_cents,
			0 AS bill_count,
			method_totals.transaction_count AS transaction_count,
			period_totals.total_revenue_cents AS total_revenue_cents,
			period_totals.total_tip_cents AS total_tip_cents,
			period_totals.transaction_count AS period_transaction_count,
			period_totals.bill_count AS period_bill_count,
			period_totals.unique_customers AS unique_customers
		FROM period_totals
		JOIN method_totals ON 1 = 1
	`, recognizedCTE, hourExpr, hourExpr), args...).Scan(&rows).Error; err != nil {
		return nil, nil, nil, err
	}

	summary := &PaymentWindowSummary{StartDate: startDate, EndDate: endDate}
	hourlyRows := make([]recognizedPaymentHourlyAggregate, 0)
	paymentMethods := make(map[string]int)

	if len(rows) > 0 {
		first := rows[0]
		summary.TotalRevenue = centsToDollars(first.TotalRevenueCents)
		summary.TotalTips = centsToDollars(first.TotalTipCents)
		summary.TransactionCount = first.PeriodTransactionCount
		summary.BillCount = first.PeriodBillCount
		summary.UniqueCustomers = first.UniqueCustomers
		if summary.BillCount > 0 {
			summary.AverageTicket = summary.TotalRevenue / float64(summary.BillCount)
		}
	}

	for _, row := range rows {
		switch row.RowKind {
		case "hour":
			if row.Hour == nil {
				continue
			}
			hourlyRows = append(hourlyRows, recognizedPaymentHourlyAggregate{
				Hour:             *row.Hour,
				RevenueCents:     row.RevenueCents,
				TipCents:         row.TipCents,
				BillCount:        row.BillCount,
				TransactionCount: row.TransactionCount,
			})
		case "method":
			method := strings.TrimSpace(row.PaymentMethod)
			if method == "" {
				method = "crypto"
			}
			paymentMethods[method] += row.TransactionCount
		}
	}

	return summary, hourlyRows, paymentMethods, nil
}

func (s *AnalyticsService) aggregateRecognizedTipMetrics(
	businessID uint,
	startDate, endDate time.Time,
	todayStart, yesterdayStart time.Time,
) ([]recognizedTipMetricsRow, error) {
	recognizedSQL, recognizedArgs := s.recognizedPaymentBillAmountsSubquerySQL(businessID, startDate, endDate)
	recognizedCTE := s.recognizedEventsCTE(recognizedSQL)
	hourExpr := s.hourExpression("recognized_events.recognized_at", startDate.Location())
	args := append(append([]interface{}{}, recognizedArgs...), todayStart, yesterdayStart, todayStart)

	var rows []recognizedTipMetricsRow
	if err := s.db.GetGorm().Raw(fmt.Sprintf(`
		WITH %s,
		tip_totals AS (
			SELECT
				COALESCE(SUM(recognized_events.tip_cents), 0) AS total_tip_cents,
				COALESCE(SUM(CASE WHEN recognized_events.tip_cents > 0 THEN recognized_events.tip_cents ELSE 0 END), 0) AS positive_tip_cents,
				COALESCE(SUM(CASE WHEN recognized_events.tip_cents > 0 AND recognized_events.amount_cents > 0 THEN recognized_events.amount_cents ELSE 0 END), 0) AS positive_amount_cents,
				COALESCE(SUM(CASE WHEN recognized_events.tip_cents > 0 THEN 1 ELSE 0 END), 0) AS positive_tip_count,
				COALESCE(SUM(CASE WHEN recognized_events.recognized_at >= ? THEN recognized_events.tip_cents ELSE 0 END), 0) AS today_tip_cents,
				COALESCE(SUM(CASE WHEN recognized_events.recognized_at >= ? AND recognized_events.recognized_at < ? THEN recognized_events.tip_cents ELSE 0 END), 0) AS yesterday_tip_cents
			FROM recognized_events
			WHERE recognized_events.tip_cents <> 0
		),
		hourly_tips AS (
			SELECT
				%s AS hour,
				COALESCE(SUM(recognized_events.tip_cents), 0) AS tip_cents
			FROM recognized_events
			WHERE recognized_events.tip_cents <> 0
			GROUP BY %s
		),
		tip_distribution AS (
			SELECT
				CASE
					WHEN recognized_events.tip_cents < 500 THEN '0_5'
					WHEN recognized_events.tip_cents < 1000 THEN '5_10'
					WHEN recognized_events.tip_cents < 2000 THEN '10_20'
					WHEN recognized_events.tip_cents < 5000 THEN '20_50'
					ELSE '50_plus'
				END AS bucket,
				COUNT(*) AS tip_count
			FROM recognized_events
			WHERE recognized_events.tip_cents > 0
			GROUP BY bucket
		),
		top_tippers AS (
			SELECT
				recognized_events.customer_id AS payer_addr,
				COALESCE(SUM(recognized_events.tip_cents), 0) AS tip_cents,
				COUNT(*) AS tip_count
			FROM recognized_events
			WHERE recognized_events.source_table = 'payments'
				AND recognized_events.customer_id <> ''
				AND recognized_events.tip_cents > 0
			GROUP BY recognized_events.customer_id
			HAVING SUM(recognized_events.tip_cents) > 0
			ORDER BY tip_cents DESC, tip_count DESC
			LIMIT 10
		)
		SELECT
			'totals' AS row_kind,
			NULL AS hour,
			'' AS bucket,
			'' AS payer_addr,
			tip_totals.total_tip_cents AS total_tip_cents,
			tip_totals.positive_tip_cents AS positive_tip_cents,
			tip_totals.positive_amount_cents AS positive_amount_cents,
			tip_totals.positive_tip_count AS positive_tip_count,
			0 AS tip_cents,
			0 AS tip_count,
			tip_totals.today_tip_cents AS today_tip_cents,
			tip_totals.yesterday_tip_cents AS yesterday_tip_cents
		FROM tip_totals
		UNION ALL
		SELECT
			'hour' AS row_kind,
			hourly_tips.hour AS hour,
			'' AS bucket,
			'' AS payer_addr,
			0 AS total_tip_cents,
			0 AS positive_tip_cents,
			0 AS positive_amount_cents,
			0 AS positive_tip_count,
			hourly_tips.tip_cents AS tip_cents,
			0 AS tip_count,
			0 AS today_tip_cents,
			0 AS yesterday_tip_cents
		FROM hourly_tips
		UNION ALL
		SELECT
			'distribution' AS row_kind,
			NULL AS hour,
			tip_distribution.bucket AS bucket,
			'' AS payer_addr,
			0 AS total_tip_cents,
			0 AS positive_tip_cents,
			0 AS positive_amount_cents,
			0 AS positive_tip_count,
			0 AS tip_cents,
			tip_distribution.tip_count AS tip_count,
			0 AS today_tip_cents,
			0 AS yesterday_tip_cents
		FROM tip_distribution
		UNION ALL
		SELECT
			'tipper' AS row_kind,
			NULL AS hour,
			'' AS bucket,
			top_tippers.payer_addr AS payer_addr,
			0 AS total_tip_cents,
			0 AS positive_tip_cents,
			0 AS positive_amount_cents,
			0 AS positive_tip_count,
			top_tippers.tip_cents AS tip_cents,
			top_tippers.tip_count AS tip_count,
			0 AS today_tip_cents,
			0 AS yesterday_tip_cents
		FROM top_tippers
	`, recognizedCTE, hourExpr, hourExpr), args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *AnalyticsService) recognizedPaymentBillAmountsSubquerySQL(businessID uint, startDate, endDate time.Time) (string, []interface{}) {
	return s.recognizedPaymentRowsSubquerySQL(businessID, startDate, endDate)
}

func (s *AnalyticsService) recognizedEventsCTE(recognizedSQL string) string {
	if s.db.GetGorm().Name() == "postgres" {
		return fmt.Sprintf("recognized_events AS MATERIALIZED (%s)", recognizedSQL)
	}
	return fmt.Sprintf("recognized_events AS (%s)", recognizedSQL)
}

func analyticsPaymentMethodExpression() string {
	return `
		CASE
			WHEN LOWER(TRIM(COALESCE(payments.payment_method, ''))) = 'plugin'
				OR LOWER(TRIM(COALESCE(payments.payer_addr, ''))) = 'plugin'
				OR SUBSTR(LOWER(TRIM(COALESCE(payments.tx_hash, ''))), 1, 7) = 'plugin_'
				THEN 'plugin'
			WHEN TRIM(COALESCE(payments.payment_method, '')) = '' THEN 'crypto'
			ELSE LOWER(TRIM(COALESCE(payments.payment_method, '')))
		END
	`
}

func analyticsAlternativePaymentMethodExpression() string {
	return "LOWER(TRIM(COALESCE(alternative_payments.payment_method, '')))"
}

func (s *AnalyticsService) recognizedPaymentRowsSubquerySQL(businessID uint, startDate, endDate time.Time) (string, []interface{}) {
	// Refunded payments/alt-payments are recognized exactly like reversed ones
	// (original positive recognition + dated negative event), so AN-MON-1 refunds
	// are netted in the right periods instead of silently vanishing. The status
	// sets are the shared source of truth in the database package.
	recognizedPaymentStatuses := database.RecognizedPaymentStatuses
	reversedOrRefundedStatuses := database.ReversedOrRefundedPaymentStatuses
	recognizedAltStatuses := database.RecognizedAltPaymentStatuses
	paymentMethodExpr := analyticsPaymentMethodExpression()
	alternativePaymentMethodExpr := analyticsAlternativePaymentMethodExpression()
	billSettlementTimeExpr := database.RecognizedBillSettlementTimeSQL()

	branches := []string{
		fmt.Sprintf(`
			SELECT
				payments.bill_id AS bill_id,
				COALESCE(payments.amount, 0) AS amount_cents,
				COALESCE(payments.tip_amount, 0) AS tip_cents,
				payments.confirmed_at AS recognized_at,
				COALESCE(payments.payer_addr, '') AS customer_id,
				%s AS payment_method,
				'payments' AS source_table
			FROM payments
			JOIN bills ON payments.bill_id = bills.id
			WHERE bills.business_id = ?
				AND payments.status IN ?
				AND payments.confirmed_at IS NOT NULL
				AND payments.confirmed_at >= ? AND payments.confirmed_at < ?
		`, paymentMethodExpr),
		fmt.Sprintf(`
			SELECT
				payments.bill_id AS bill_id,
				COALESCE(payments.amount, 0) AS amount_cents,
				COALESCE(payments.tip_amount, 0) AS tip_cents,
				payments.updated_at AS recognized_at,
				COALESCE(payments.payer_addr, '') AS customer_id,
				%s AS payment_method,
				'payments' AS source_table
			FROM payments
			JOIN bills ON payments.bill_id = bills.id
			WHERE bills.business_id = ?
				AND payments.status = ?
				AND payments.confirmed_at IS NULL
				AND payments.updated_at >= ? AND payments.updated_at < ?
		`, paymentMethodExpr),
		fmt.Sprintf(`
			SELECT
				payments.bill_id AS bill_id,
				COALESCE(payments.amount, 0) AS amount_cents,
				COALESCE(payments.tip_amount, 0) AS tip_cents,
				payments.created_at AS recognized_at,
				COALESCE(payments.payer_addr, '') AS customer_id,
				%s AS payment_method,
				'payments' AS source_table
			FROM payments
			JOIN bills ON payments.bill_id = bills.id
			WHERE bills.business_id = ?
				AND payments.status IN ?
				AND payments.confirmed_at IS NULL
				AND payments.created_at >= ? AND payments.created_at < ?
		`, paymentMethodExpr),
		fmt.Sprintf(`
			SELECT
				payments.bill_id AS bill_id,
				(-COALESCE(payments.amount, 0)) AS amount_cents,
				(-COALESCE(payments.tip_amount, 0)) AS tip_cents,
				payments.reversed_at AS recognized_at,
				COALESCE(payments.payer_addr, '') AS customer_id,
				%s AS payment_method,
				'payments' AS source_table
			FROM payments
			JOIN bills ON payments.bill_id = bills.id
			WHERE bills.business_id = ?
				AND payments.status IN ?
				AND payments.reversed_at IS NOT NULL
				AND payments.reversed_at >= ? AND payments.reversed_at < ?
		`, paymentMethodExpr),
		fmt.Sprintf(`
			SELECT
				payments.bill_id AS bill_id,
				(-COALESCE(payments.amount, 0)) AS amount_cents,
				(-COALESCE(payments.tip_amount, 0)) AS tip_cents,
				payments.updated_at AS recognized_at,
				COALESCE(payments.payer_addr, '') AS customer_id,
				%s AS payment_method,
				'payments' AS source_table
			FROM payments
			JOIN bills ON payments.bill_id = bills.id
			WHERE bills.business_id = ?
				AND payments.status IN ?
				AND payments.reversed_at IS NULL
				AND payments.updated_at >= ? AND payments.updated_at < ?
		`, paymentMethodExpr),
		fmt.Sprintf(`
			SELECT
				alternative_payments.bill_id AS bill_id,
				COALESCE(alternative_payments.amount, 0) AS amount_cents,
				COALESCE(alternative_payments.tip_amount_cents, 0) AS tip_cents,
				alternative_payments.confirmed_at AS recognized_at,
				COALESCE(alternative_payments.participant_addr, '') AS customer_id,
				%s AS payment_method,
				'alternative_payments' AS source_table
			FROM alternative_payments
			JOIN bills ON alternative_payments.bill_id = bills.id
			WHERE bills.business_id = ?
				AND alternative_payments.status IN ?
				AND alternative_payments.payment_method IN ?
				AND alternative_payments.confirmed_at IS NOT NULL
				AND alternative_payments.confirmed_at >= ? AND alternative_payments.confirmed_at < ?
		`, alternativePaymentMethodExpr),
		fmt.Sprintf(`
			SELECT
				alternative_payments.bill_id AS bill_id,
				COALESCE(alternative_payments.amount, 0) AS amount_cents,
				COALESCE(alternative_payments.tip_amount_cents, 0) AS tip_cents,
				alternative_payments.created_at AS recognized_at,
				COALESCE(alternative_payments.participant_addr, '') AS customer_id,
				%s AS payment_method,
				'alternative_payments' AS source_table
			FROM alternative_payments
			JOIN bills ON alternative_payments.bill_id = bills.id
			WHERE bills.business_id = ?
				AND alternative_payments.status IN ?
				AND alternative_payments.payment_method IN ?
				AND alternative_payments.confirmed_at IS NULL
				AND alternative_payments.created_at >= ? AND alternative_payments.created_at < ?
		`, alternativePaymentMethodExpr),
		fmt.Sprintf(`
			SELECT
				alternative_payments.bill_id AS bill_id,
				(-COALESCE(alternative_payments.amount, 0)) AS amount_cents,
				(-COALESCE(alternative_payments.tip_amount_cents, 0)) AS tip_cents,
				alternative_payments.updated_at AS recognized_at,
				COALESCE(alternative_payments.participant_addr, '') AS customer_id,
				%s AS payment_method,
				'alternative_payments' AS source_table
			FROM alternative_payments
			JOIN bills ON alternative_payments.bill_id = bills.id
			WHERE bills.business_id = ?
				AND alternative_payments.status = ?
				AND alternative_payments.payment_method IN ?
				AND alternative_payments.updated_at >= ? AND alternative_payments.updated_at < ?
		`, alternativePaymentMethodExpr),
		fmt.Sprintf(`
			SELECT
				bills.id AS bill_id,
				CASE
					WHEN COALESCE(bills.paid_amount, 0) > 0 THEN COALESCE(bills.paid_amount, 0)
					ELSE COALESCE(bills.total_amount, 0)
				END AS amount_cents,
				COALESCE(bills.tip_amount, 0) AS tip_cents,
				%s AS recognized_at,
				'' AS customer_id,
				'legacy_bill' AS payment_method,
				'bills' AS source_table
			FROM bills
			WHERE bills.business_id = ?
				AND (bills.paid_amount > ? OR (bills.status = ? AND bills.total_amount > ?))
				AND NOT EXISTS (
					SELECT 1 FROM payments recognized_payments
					WHERE recognized_payments.bill_id = bills.id
						AND recognized_payments.status IN ?
				)
				AND NOT EXISTS (
					SELECT 1 FROM alternative_payments recognized_alternative_payments
					WHERE recognized_alternative_payments.bill_id = bills.id
						AND recognized_alternative_payments.status IN ?
						AND recognized_alternative_payments.payment_method IN ?
				)
				AND %s >= ? AND %s < ?
		`, billSettlementTimeExpr, billSettlementTimeExpr, billSettlementTimeExpr),
	}

	args := []interface{}{
		businessID, recognizedPaymentStatuses, startDate, endDate,
		businessID, database.PaymentStatusConfirmed, startDate, endDate,
		businessID, reversedOrRefundedStatuses, startDate, endDate,
		businessID, reversedOrRefundedStatuses, startDate, endDate,
		businessID, reversedOrRefundedStatuses, startDate, endDate,
		businessID, recognizedAltStatuses, confirmedAlternativePaymentMethods, startDate, endDate,
		businessID, recognizedAltStatuses, confirmedAlternativePaymentMethods, startDate, endDate,
		businessID, database.AltPaymentStatusRefunded, confirmedAlternativePaymentMethods, startDate, endDate,
		businessID, 0, database.BillStatusPaid, 0, recognizedPaymentStatuses, recognizedAltStatuses, confirmedAlternativePaymentMethods, startDate, endDate,
	}

	return strings.Join(branches, "\nUNION ALL\n"), args
}

func (s *AnalyticsService) hourExpression(column string, loc *time.Location) string {
	return hourExpressionForDialect(s.db.GetGorm().Name(), column, loc)
}

func hourExpressionForDialect(dialectName, column string, loc *time.Location) string {
	col := localizedTimestampExpr(dialectName, column, loc)
	if dialectName == "sqlite" {
		return fmt.Sprintf("CAST(strftime('%%H', %s) AS INTEGER)", col)
	}
	return fmt.Sprintf("CAST(EXTRACT(HOUR FROM %s) AS INTEGER)", col)
}

// localizedTimestampExpr wraps a timestamp column so date/hour extraction buckets
// in the business's local calendar. Delegates to database.LocalizedTimestampExpr
// (decision-14 cache) so analytics and AI insights share one LoadLocation + SQL
// fragment cache. loc==UTC (or nil) returns the column unchanged.
func localizedTimestampExpr(dialectName, column string, loc *time.Location) string {
	return database.LocalizedTimestampExpr(dialectName, column, loc)
}

func centsToDollars(cents int64) float64 {
	return float64(cents) / 100.0
}

// recognizedQuantityToInt is the display count for ItemStats.TotalSold.
// Non-positive quantity displays as 0. Otherwise the recognized quantity is
// rounded to the nearest integer. A paid line whose recognized quantity rounds
// below 1 still displays as 1, so a half-paid single unit does not disappear.
// Zero-revenue lines (bundle components) display the rounded quantity and are
// not forced to 0.
func recognizedQuantityToInt(quantity, revenue float64) int {
	if quantity <= 0 {
		return 0
	}

	sold := int(math.Round(quantity))
	if sold < 1 && revenue > 0 {
		return 1
	}
	return sold
}

// ExportSalesData exports sales data for a preset period in the specified
// format. lang selects CSV header locale ("en" / "es*" → Spanish).
func (s *AnalyticsService) ExportSalesData(businessID uint, period string, format string, loc *time.Location, lang string) ([]byte, error) {
	return s.ExportSalesDataRange(businessID, period, nil, nil, format, loc, lang)
}

// ExportSalesDataRange is ExportSalesData with an optional explicit
// [start, end) window that wins over the preset. An inverted or one-sided pair
// is an error rather than a silent fallback to the default period (#925).
func (s *AnalyticsService) ExportSalesDataRange(businessID uint, period string, start, end *time.Time, format string, loc *time.Location, lang string) ([]byte, error) {
	startDate, endDate, err := s.resolveWindow(period, start, end, loc)
	if err != nil {
		return nil, fmt.Errorf("invalid period: %w", err)
	}

	events, err := database.GetRecognizedPaymentEvents(businessID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get recognized payment events: %w", err)
	}

	switch format {
	case "csv":
		return s.exportEventsToCSV(events, lang, s.businessCurrency(businessID))
	case "json":
		return s.exportEventsToJSON(events)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// businessCurrency reads the venue's currency columns with a narrow projection
// (no SELECT *, one query per export) and resolves them through the same
// display → default → USD order every other money surface uses, so the sales
// CSV can name the currency its amounts are denominated in (#926).
func (s *AnalyticsService) businessCurrency(businessID uint) string {
	var row struct {
		DisplayCurrency string
		DefaultCurrency string
	}
	if err := s.db.GetGorm().
		Table("businesses").
		Select("display_currency", "default_currency").
		Where("id = ?", businessID).
		Limit(1).
		Scan(&row).Error; err != nil {
		return database.Business{}.ResolvedCurrency()
	}
	return database.Business{
		DisplayCurrency: row.DisplayCurrency,
		DefaultCurrency: row.DefaultCurrency,
	}.ResolvedCurrency()
}

// locOrUTC normalizes a possibly-nil *time.Location to time.UTC so any caller
// that has not yet been updated to thread a business location is
// behavior-preserving (UTC == the legacy server-clock behavior).
func locOrUTC(loc *time.Location) *time.Location {
	if loc == nil {
		return time.UTC
	}
	return loc
}

// ParsePeriodWindow exposes the analytics period→[start,end) resolution (in the
// business location) so other read-only services (e.g. wastevariance) can scope
// ledger queries to the SAME window the sales aggregates use. Pass "today" for a
// single-day window. Half-open: start inclusive, end exclusive.
func (s *AnalyticsService) ParsePeriodWindow(period string, loc *time.Location) (time.Time, time.Time, error) {
	return s.parsePeriod(period, loc)
}

// parsePeriod converts a period string to a [start, end) window in the
// business's timezone (loc). Delegates to reporting.ResolveWindowAt so
// analytics and accounting share one calendar-window implementation (L6-10).
// Returned start/end are UTC instants; use loc for zone-aware bucketing.
func (s *AnalyticsService) parsePeriod(period string, loc *time.Location) (time.Time, time.Time, error) {
	return s.resolveWindow(period, nil, nil, loc)
}

// resolveWindow is parsePeriod plus the optional custom [start, end) override.
// It keeps ONE window implementation for analytics: reporting.ResolveWindowAt
// already gives a non-nil pair precedence over the preset and rejects inverted
// / one-sided ranges, so callers get a real error instead of the default period
// whenever an operator hands in a bad range (#925).
func (s *AnalyticsService) resolveWindow(period string, start, end *time.Time, loc *time.Location) (time.Time, time.Time, error) {
	loc = locOrUTC(loc)
	win, err := reporting.ResolveWindowAt(period, start, end, loc, s.now())
	if err != nil {
		if start != nil || end != nil {
			return time.Time{}, time.Time{}, err
		}
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported period: %s", period)
	}
	// Re-attach business location so hour-bucketing callers that use
	// startDate.Location() stay in the business zone (UTC instants unchanged).
	return win.Start.In(loc), win.End.In(loc), nil
}

// exportEventsToCSV writes the recognized-payment rows. currency names the
// venue's own currency and trails every money column, matching the payments
// and accounting exports — without it an ARS carta downloads bare numbers and
// the operator has to assume dollars (#926).
func (s *AnalyticsService) exportEventsToCSV(events []database.RecognizedPaymentEvent, lang string, currency string) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	headers := []string{"Recognized At", "Bill Number", "Bill ID", "Source", "Payment Method", "Amount", "Tip", "Bill Status", "Bill Total", "Currency"}
	if utils.NormalizeOperatorLang(lang) == "es" {
		headers = []string{"Reconocido el", "Número de cuenta", "ID de cuenta", "Fuente", "Método de pago", "Monto", "Propina", "Estado de cuenta", "Total de cuenta", "Moneda"}
	}
	if err := writer.Write(headers); err != nil {
		return nil, err
	}
	for _, event := range events {
		// Source/PaymentMethod can carry plugin-provided strings; sanitize the
		// free-text columns against CSV formula injection (CWE-1236). Generated
		// bill numbers, enums, and code-formatted numerics are left as-is.
		if err := writer.Write([]string{
			event.RecognizedAt.Format("2006-01-02 15:04:05"),
			event.BillNumber,
			fmt.Sprintf("%d", event.BillID),
			utils.SanitizeCSVField(event.Source),
			utils.SanitizeCSVField(event.PaymentMethod),
			fmt.Sprintf("%.2f", centsToDollars(event.AmountCents)),
			fmt.Sprintf("%.2f", centsToDollars(event.TipCents)),
			string(event.BillStatus),
			fmt.Sprintf("%.2f", centsToDollars(event.BillTotalCents)),
			currency,
		}); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	return buf.Bytes(), writer.Error()
}

func (s *AnalyticsService) exportEventsToJSON(events []database.RecognizedPaymentEvent) ([]byte, error) {
	return json.Marshal(struct {
		Events []database.RecognizedPaymentEvent `json:"events"`
	}{Events: events})
}

// itemCategoryCacheTTL bounds how long a cached item→category map may serve
// translation edits, which do not bump the menu version.
const itemCategoryCacheTTL = 5 * time.Minute

// itemCategoryCacheEntry is the item→category/name map built from one menu
// version. One entry per business: a newer menu version replaces it.
type itemCategoryCacheEntry struct {
	conn       *gorm.DB
	menuID     uint
	version    uint
	updatedAt  time.Time
	language   string
	loadedAt   time.Time
	categories map[string]string
	names      map[string]string
}

// itemCategoryCache maps businessID → *itemCategoryCacheEntry. Package level
// because handlers build a fresh AnalyticsService per request.
var itemCategoryCache sync.Map

type menuVersionHead struct {
	ID        uint
	Version   uint
	UpdatedAt time.Time
}

func (e *itemCategoryCacheEntry) matches(conn *gorm.DB, head menuVersionHead, language string, now time.Time) bool {
	return e.conn == conn && e.menuID == head.ID && e.version == head.Version &&
		e.updatedAt.Equal(head.UpdatedAt) && e.language == language &&
		now.Sub(e.loadedAt) < itemCategoryCacheTTL
}

// getItemCategoryMapping creates a map from item name to category by parsing the menu.
// The result is cached per (business, menu id, menu version, language): the
// dashboard pulse calls this on every refresh, and the menu JSON blob plus two
// translation reads dominated its cost. A narrow id/version probe decides
// whether the cached map is still current. Callers must not mutate the maps.
func (s *AnalyticsService) getItemCategoryMapping(businessID uint) (map[string]string, map[string]string, error) {
	// Get the business default language
	_, defaultLanguage, err := s.db.GetBusinessDefaults(businessID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get business defaults: %w", err)
	}

	conn := s.db.GetGorm()
	var head menuVersionHead
	headErr := conn.Model(&database.Menu{}).
		Select("id, version, updated_at").
		Where("business_id = ? AND is_active = ?", businessID, true).
		Order("id").
		Limit(1).
		Take(&head).Error
	if headErr == nil {
		if cached, ok := itemCategoryCache.Load(businessID); ok {
			entry := cached.(*itemCategoryCacheEntry)
			if entry.matches(conn, head, defaultLanguage, time.Now()) {
				return entry.categories, entry.names, nil
			}
		}
	}

	// Get the menu for this business
	menu, categories, err := s.db.MenuService.GetByBusinessID(businessID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get menu: %w", err)
	}

	itemCategoryMap := make(map[string]string)
	itemNameMap := make(map[string]string) // Map original item names to translated names
	categoryIDs := make([]uint, 0, len(categories))
	itemIDs := make([]uint, 0)
	for i, category := range categories {
		categoryIDs = append(categoryIDs, uint(i))
		for j := range category.Items {
			itemIDs = append(itemIDs, uint(i*1000+j))
		}
	}

	translations, err := s.loadMenuNameTranslations(businessID, defaultLanguage, categoryIDs, itemIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get menu translations: %w", err)
	}

	// Build the mapping from ORIGINAL item names to TRANSLATED category names AND translated item names
	for i, category := range categories {
		// Get translated category name
		translatedCategoryName := category.Name
		if translated, found := translations.categoryNames[uint(i)]; found {
			translatedCategoryName = translated
		}

		// Map ORIGINAL item names to translated category name AND get translated item names
		for j, item := range category.Items {
			// Use original item name as key (this is what's stored in bills)
			itemCategoryMap[item.Name] = translatedCategoryName

			// Get translated item name - EXACT same logic as business handlers
			translatedItemName := item.Name
			// Use position-based ID: categoryIndex * 1000 + itemIndex - EXACT same as business handlers.
			entityID := uint(i*1000 + j)
			if translated, found := translations.itemNames[entityID]; found {
				translatedItemName = translated
			}

			// Map original item name to translated item name
			itemNameMap[item.Name] = translatedItemName
		}
	}

	// Key on the menu row actually read, so a write racing the probe caches
	// under the version that produced these maps.
	itemCategoryCache.Store(businessID, &itemCategoryCacheEntry{
		conn:       conn,
		menuID:     menu.ID,
		version:    menu.Version,
		updatedAt:  menu.UpdatedAt,
		language:   defaultLanguage,
		loadedAt:   time.Now(),
		categories: itemCategoryMap,
		names:      itemNameMap,
	})
	return itemCategoryMap, itemNameMap, nil
}

type menuNameTranslations struct {
	categoryNames map[uint]string
	itemNames     map[uint]string
}

func (s *AnalyticsService) loadMenuNameTranslations(businessID uint, languageCode string, categoryIDs, itemIDs []uint) (menuNameTranslations, error) {
	result := menuNameTranslations{
		categoryNames: make(map[uint]string),
		itemNames:     make(map[uint]string),
	}

	languageCode = strings.TrimSpace(languageCode)
	if languageCode == "" || strings.EqualFold(languageCode, "es") {
		return result, nil
	}

	if err := s.loadLatestNameTranslations(businessID, "category", languageCode, categoryIDs, result.categoryNames); err != nil {
		return menuNameTranslations{}, err
	}
	if err := s.loadLatestNameTranslations(businessID, "menu_item", languageCode, itemIDs, result.itemNames); err != nil {
		return menuNameTranslations{}, err
	}

	return result, nil
}

func (s *AnalyticsService) loadLatestNameTranslations(businessID uint, entityType, languageCode string, entityIDs []uint, target map[uint]string) error {
	if len(entityIDs) == 0 {
		return nil
	}

	var translations []database.Translation
	if err := s.db.GetGorm().
		Where("business_id = ? AND entity_type = ? AND entity_id IN ? AND field_name = ? AND language_code = ?",
			businessID, entityType, entityIDs, "name", languageCode).
		Order("id DESC").
		Find(&translations).Error; err != nil {
		return err
	}

	for _, translation := range translations {
		if _, exists := target[translation.EntityID]; !exists {
			target[translation.EntityID] = translation.TranslatedText
		}
	}

	return nil
}
