package analytics

import (
	"fmt"
	"time"

	"payverge/internal/database"
)

type AnalyticsService struct {
	db *database.DB
}

func NewAnalyticsService(db *database.DB) *AnalyticsService {
	return &AnalyticsService{db: db}
}

// SalesReport represents daily sales data
type SalesReport struct {
	Date            time.Time `json:"date"`
	BusinessID      uint      `json:"business_id"`
	TotalRevenue    float64   `json:"total_revenue"`
	TotalTips       float64   `json:"total_tips"`
	TransactionCount int      `json:"transaction_count"`
	BillCount       int       `json:"bill_count"`
	AverageTicket   float64   `json:"average_ticket"`
	PaymentMethods  map[string]int `json:"payment_methods"`
	HourlyBreakdown []HourlySales `json:"hourly_breakdown"`
}

type HourlySales struct {
	Hour     int     `json:"hour"`
	Revenue  float64 `json:"revenue"`
	Tips     float64 `json:"tips"`
	BillCount int    `json:"bill_count"`
}

// ItemStats represents menu item performance
type ItemStats struct {
	ItemID       string  `json:"item_id"`
	ItemName     string  `json:"item_name"`
	Category     string  `json:"category"`
	TotalSold    int     `json:"total_sold"`
	Revenue      float64 `json:"revenue"`
	AveragePrice float64 `json:"average_price"`
	Popularity   float64 `json:"popularity"` // percentage of bills containing this item
}

// TipReport represents tip analytics
type TipReport struct {
	BusinessID      uint    `json:"business_id"`
	Period          string  `json:"period"`
	TotalTips       float64 `json:"total_tips"`
	AverageTip      float64 `json:"average_tip"`
	AverageTipRate  float64 `json:"average_tip_rate"` // percentage of bill
	TipDistribution map[string]int `json:"tip_distribution"` // tip ranges
	TopTippers      []TipperInfo `json:"top_tippers"`
}

type TipperInfo struct {
	PayerAddress string  `json:"payer_address"`
	TotalTips    float64 `json:"total_tips"`
	TipCount     int     `json:"tip_count"`
	AverageTip   float64 `json:"average_tip"`
}

// PeriodReport represents analytics for a specific time period
type PeriodReport struct {
	StartDate       time.Time `json:"start_date"`
	EndDate         time.Time `json:"end_date"`
	TotalRevenue    float64   `json:"total_revenue"`
	TotalTips       float64   `json:"total_tips"`
	TransactionCount int      `json:"transaction_count"`
	BillCount       int       `json:"bill_count"`
	UniqueCustomers int       `json:"unique_customers"`
	AverageTicket   float64   `json:"average_ticket"`
	GrowthRate      float64   `json:"growth_rate"` // compared to previous period
}

// GetDailySales returns sales data for a specific date
func (s *AnalyticsService) GetDailySales(businessID uint, date time.Time) (*SalesReport, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	// Get bills for the day
	bills, err := s.db.GetBillsByDateRange(businessID, startOfDay, endOfDay)
	if err != nil {
		return nil, fmt.Errorf("failed to get bills: %w", err)
	}

	// Get payments for the day
	payments, err := s.db.GetPaymentsByDateRange(businessID, startOfDay, endOfDay)
	if err != nil {
		return nil, fmt.Errorf("failed to get payments: %w", err)
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

	// Calculate totals from bills
	for _, bill := range bills {
		report.TotalRevenue += bill.TotalAmount
		report.TotalTips += bill.TipAmount
		report.BillCount++

		// Add to hourly breakdown
		hour := bill.CreatedAt.Hour()
		if hour >= 0 && hour < 24 {
			report.HourlyBreakdown[hour].Revenue += bill.TotalAmount
			report.HourlyBreakdown[hour].Tips += bill.TipAmount
			report.HourlyBreakdown[hour].BillCount++
		}
	}

	// Count payment methods and transactions
	for range payments {
		report.TransactionCount++
		// Assume all payments are crypto for now
		report.PaymentMethods["crypto"]++
	}

	// Calculate average ticket
	if report.BillCount > 0 {
		report.AverageTicket = report.TotalRevenue / float64(report.BillCount)
	}

	return report, nil
}

// GetPopularItems returns item performance statistics
func (s *AnalyticsService) GetPopularItems(businessID uint, period string) ([]ItemStats, error) {
	startDate, endDate, err := s.parsePeriod(period)
	if err != nil {
		return nil, fmt.Errorf("invalid period: %w", err)
	}

	bills, err := s.db.GetBillsByDateRange(businessID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get bills: %w", err)
	}

	itemStats := make(map[string]*ItemStats)
	totalBills := len(bills)

	// Process each bill's items
	for _, bill := range bills {
		billItems, err := s.db.GetBillItems(bill.ID)
		if err != nil {
			continue // Skip bills with item retrieval errors
		}

		billItemMap := make(map[string]bool) // Track unique items per bill

		for _, item := range billItems {
			itemKey := fmt.Sprintf("%s", item.Name)
			
			if _, exists := itemStats[itemKey]; !exists {
				itemStats[itemKey] = &ItemStats{
					ItemID:   item.ID,
					ItemName: item.Name,
					Category: "General", // Default category since BillItem doesn't have Category field
				}
			}

			stats := itemStats[itemKey]
			stats.TotalSold += item.Quantity
			stats.Revenue += item.Subtotal

			// Mark this item as present in this bill
			billItemMap[itemKey] = true
		}

		// Update popularity count (bills containing each item)
		for itemKey := range billItemMap {
			if stats, exists := itemStats[itemKey]; exists {
				// We'll calculate popularity after processing all bills
				stats.AveragePrice = stats.Revenue / float64(stats.TotalSold)
			}
		}
	}

	// Calculate popularity percentages and convert to slice
	var result []ItemStats
	for _, stats := range itemStats {
		if totalBills > 0 {
			// Count bills containing this item
			billsWithItem := 0
			for _, bill := range bills {
				billItems, err := s.db.GetBillItems(bill.ID)
				if err != nil {
					continue
				}
				for _, item := range billItems {
					if item.Name == stats.ItemName {
						billsWithItem++
						break
					}
				}
			}
			stats.Popularity = (float64(billsWithItem) / float64(totalBills)) * 100
		}
		result = append(result, *stats)
	}

	return result, nil
}

// GetTipAnalytics returns tip statistics for a period
func (s *AnalyticsService) GetTipAnalytics(businessID uint, period string) (*TipReport, error) {
	startDate, endDate, err := s.parsePeriod(period)
	if err != nil {
		return nil, fmt.Errorf("invalid period: %w", err)
	}

	payments, err := s.db.GetPaymentsByDateRange(businessID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get payments: %w", err)
	}

	report := &TipReport{
		BusinessID:      businessID,
		Period:          period,
		TipDistribution: make(map[string]int),
	}

	tipperMap := make(map[string]*TipperInfo)
	var totalTipRate float64
	var tipCount int

	for _, payment := range payments {
		if payment.TipAmount > 0 {
			report.TotalTips += float64(payment.TipAmount)
			tipCount++

			// Calculate tip rate
			if payment.Amount > 0 {
				tipRate := (float64(payment.TipAmount) / float64(payment.Amount)) * 100
				totalTipRate += tipRate
			}

			// Track tipper info
			if payment.PayerAddr != "" {
				if _, exists := tipperMap[payment.PayerAddr]; !exists {
					tipperMap[payment.PayerAddr] = &TipperInfo{
						PayerAddress: payment.PayerAddr,
					}
				}
				tipper := tipperMap[payment.PayerAddr]
				tipper.TotalTips += float64(payment.TipAmount)
				tipper.TipCount++
			}

			// Categorize tip amounts
			tipAmount := float64(payment.TipAmount)
			switch {
			case tipAmount < 5:
				report.TipDistribution["$0-5"]++
			case tipAmount < 10:
				report.TipDistribution["$5-10"]++
			case tipAmount < 20:
				report.TipDistribution["$10-20"]++
			case tipAmount < 50:
				report.TipDistribution["$20-50"]++
			default:
				report.TipDistribution["$50+"]++
			}
		}
	}

	// Calculate averages
	if tipCount > 0 {
		report.AverageTip = report.TotalTips / float64(tipCount)
		report.AverageTipRate = totalTipRate / float64(tipCount)
	}

	// Calculate average tips for each tipper and sort
	for _, tipper := range tipperMap {
		if tipper.TipCount > 0 {
			tipper.AverageTip = tipper.TotalTips / float64(tipper.TipCount)
		}
		report.TopTippers = append(report.TopTippers, *tipper)
	}

	return report, nil
}

// GetPeriodReport returns comprehensive analytics for a time period
func (s *AnalyticsService) GetPeriodReport(businessID uint, period string) (*PeriodReport, error) {
	startDate, endDate, err := s.parsePeriod(period)
	if err != nil {
		return nil, fmt.Errorf("invalid period: %w", err)
	}

	bills, err := s.db.GetBillsByDateRange(businessID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get bills: %w", err)
	}

	payments, err := s.db.GetPaymentsByDateRange(businessID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get payments: %w", err)
	}

	report := &PeriodReport{
		StartDate: startDate,
		EndDate:   endDate,
	}

	// Calculate totals from bills
	uniqueCustomers := make(map[string]bool)
	for _, bill := range bills {
		report.TotalRevenue += bill.TotalAmount
		report.TotalTips += bill.TipAmount
		report.BillCount++
	}

	// Count transactions and unique customers
	for _, p := range payments {
		// Count unique payers for customer analytics
		uniqueCustomers[p.PayerAddr] = true
	}

	report.UniqueCustomers = len(uniqueCustomers)

	// Calculate average ticket
	if report.BillCount > 0 {
		report.AverageTicket = report.TotalRevenue / float64(report.BillCount)
	}

	// TODO: Calculate growth rate compared to previous period
	// This would require getting data for the previous period and comparing

	return report, nil
}

// ExportSalesData exports sales data in the specified format
func (s *AnalyticsService) ExportSalesData(businessID uint, period string, format string) ([]byte, error) {
	startDate, endDate, err := s.parsePeriod(period)
	if err != nil {
		return nil, fmt.Errorf("invalid period: %w", err)
	}

	bills, err := s.db.GetBillsByDateRange(businessID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get bills: %w", err)
	}

	switch format {
	case "csv":
		return s.exportToCSV(bills)
	case "json":
		return s.exportToJSON(bills)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// parsePeriod converts period string to start and end dates
func (s *AnalyticsService) parsePeriod(period string) (time.Time, time.Time, error) {
	now := time.Now()
	
	switch period {
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		end := start.Add(24 * time.Hour)
		return start, end, nil
	case "yesterday":
		yesterday := now.AddDate(0, 0, -1)
		start := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, yesterday.Location())
		end := start.Add(24 * time.Hour)
		return start, end, nil
	case "week":
		start := now.AddDate(0, 0, -7)
		return start, now, nil
	case "month":
		start := now.AddDate(0, -1, 0)
		return start, now, nil
	case "quarter":
		start := now.AddDate(0, -3, 0)
		return start, now, nil
	case "year":
		start := now.AddDate(-1, 0, 0)
		return start, now, nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported period: %s", period)
	}
}

// exportToCSV converts bills data to CSV format
func (s *AnalyticsService) exportToCSV(bills []database.Bill) ([]byte, error) {
	csv := "Date,Bill Number,Total Amount,Tip Amount,Status,Items\n"
	
	for _, bill := range bills {
		csv += fmt.Sprintf("%s,%s,%.2f,%.2f,%s,%s\n",
			bill.CreatedAt.Format("2006-01-02 15:04:05"),
			bill.BillNumber,
			bill.TotalAmount,
			bill.TipAmount,
			bill.Status,
			bill.Items, // This is a JSON string of items
		)
	}
	
	return []byte(csv), nil
}

// exportToJSON converts bills data to JSON format
func (s *AnalyticsService) exportToJSON(bills []database.Bill) ([]byte, error) {
	// This would use json.Marshal to convert bills to JSON
	// For now, return a simple JSON structure
	json := `{"bills": [`
	
	for i, bill := range bills {
		if i > 0 {
			json += ","
		}
		json += fmt.Sprintf(`{
			"date": "%s",
			"bill_number": "%s",
			"total_amount": %.2f,
			"tip_amount": %.2f,
			"status": "%s"
		}`, 
			bill.CreatedAt.Format("2006-01-02T15:04:05Z"),
			bill.BillNumber,
			bill.TotalAmount,
			bill.TipAmount,
			bill.Status,
		)
	}
	
	json += "]}"
	return []byte(json), nil
}
