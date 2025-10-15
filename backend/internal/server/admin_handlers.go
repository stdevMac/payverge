package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"payverge/internal/database"
)

// AdminStats represents comprehensive admin dashboard statistics
type AdminStats struct {
	// Business metrics
	TotalBusinesses    int64                    `json:"total_businesses"`
	ActiveBusinesses   int64                    `json:"active_businesses"`
	InactiveBusinesses int64                    `json:"inactive_businesses"`
	BusinessGrowth     []MonthlyGrowth          `json:"business_growth"`
	BusinessesByReferral map[string]int64       `json:"businesses_by_referral"`

	// User metrics
	TotalUsers     int64           `json:"total_users"`
	UsersByRole    map[string]int64 `json:"users_by_role"`
	UserGrowth     []MonthlyGrowth `json:"user_growth"`

	// Payment metrics
	TotalPaymentVolume    float64         `json:"total_payment_volume"`
	TotalCryptoPayments   float64         `json:"total_crypto_payments"`
	TotalAlternativePayments float64      `json:"total_alternative_payments"`
	PaymentVolumeGrowth   []MonthlyGrowth `json:"payment_volume_growth"`
	AverageTransactionSize float64        `json:"average_transaction_size"`

	// Bill metrics
	TotalBills     int64           `json:"total_bills"`
	BillsByStatus  map[string]int64 `json:"bills_by_status"`
	BillGrowth     []MonthlyGrowth `json:"bill_growth"`

	// Subscription metrics
	TotalSubscribers   int64           `json:"total_subscribers"`
	SubscriberGrowth   []MonthlyGrowth `json:"subscriber_growth"`

	// Referral metrics (from database)
	TotalReferrers        int64                    `json:"total_referrers"`
	ReferrersByTier       map[string]int64         `json:"referrers_by_tier"`
	TotalCommissionsPaid  float64                  `json:"total_commissions_paid"`
	TopReferrers          []TopReferrer            `json:"top_referrers"`

	// Revenue metrics
	TotalRevenue          float64         `json:"total_revenue"`
	RevenueGrowth         []MonthlyGrowth `json:"revenue_growth"`
}

// MonthlyGrowth represents growth data for a specific month
type MonthlyGrowth struct {
	Month string  `json:"month"`
	Count int64   `json:"count"`
	Value float64 `json:"value,omitempty"` // For revenue/volume data
}

// TopReferrer represents top performing referrers
type TopReferrer struct {
	WalletAddress     string  `json:"wallet_address"`
	ReferralCode      string  `json:"referral_code"`
	Tier              string  `json:"tier"`
	TotalReferrals    int64   `json:"total_referrals"`
	TotalCommissions  float64 `json:"total_commissions"`
}

// GetAdminStats returns comprehensive admin dashboard statistics
func GetAdminStats(c *gin.Context) {
	stats := AdminStats{}

	// Get business metrics
	if err := getBusinessMetrics(&stats); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get business metrics"})
		return
	}

	// Get user metrics
	if err := getUserMetrics(&stats); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user metrics"})
		return
	}

	// Get payment metrics
	if err := getPaymentMetrics(&stats); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get payment metrics"})
		return
	}

	// Get bill metrics
	if err := getBillMetrics(&stats); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get bill metrics"})
		return
	}

	// Get subscriber metrics
	if err := getSubscriberMetrics(&stats); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get subscriber metrics"})
		return
	}

	// Get referral metrics
	if err := getReferralMetrics(&stats); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get referral metrics"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// getBusinessMetrics calculates business-related statistics
func getBusinessMetrics(stats *AdminStats) error {
	db := database.GetDB()

	// Total businesses
	if err := db.Model(&database.Business{}).Count(&stats.TotalBusinesses).Error; err != nil {
		return err
	}

	// Active vs inactive businesses
	if err := db.Model(&database.Business{}).Where("is_active = ?", true).Count(&stats.ActiveBusinesses).Error; err != nil {
		return err
	}
	stats.InactiveBusinesses = stats.TotalBusinesses - stats.ActiveBusinesses

	// Business growth by month (last 12 months)
	stats.BusinessGrowth = make([]MonthlyGrowth, 0)
	for i := 11; i >= 0; i-- {
		monthStart := time.Now().AddDate(0, -i, 0).Truncate(24 * time.Hour)
		monthStart = time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, monthStart.Location())
		monthEnd := monthStart.AddDate(0, 1, 0).Add(-time.Second)

		var count int64
		if err := db.Model(&database.Business{}).
			Where("created_at BETWEEN ? AND ?", monthStart, monthEnd).
			Count(&count).Error; err != nil {
			return err
		}

		stats.BusinessGrowth = append(stats.BusinessGrowth, MonthlyGrowth{
			Month: monthStart.Format("Jan 2006"),
			Count: count,
		})
	}

	// Businesses by referral source
	stats.BusinessesByReferral = make(map[string]int64)
	var referralData []struct {
		ReferredByCode string `json:"referred_by_code"`
		Count          int64  `json:"count"`
	}
	
	if err := db.Model(&database.Business{}).
		Select("referred_by_code, COUNT(*) as count").
		Group("referred_by_code").
		Scan(&referralData).Error; err != nil {
		return err
	}

	for _, data := range referralData {
		if data.ReferredByCode == "" {
			stats.BusinessesByReferral["direct"] = data.Count
		} else {
			stats.BusinessesByReferral["referral"] = stats.BusinessesByReferral["referral"] + data.Count
		}
	}

	return nil
}

// getUserMetrics calculates user-related statistics
func getUserMetrics(stats *AdminStats) error {
	db := database.GetDB()

	// Total users
	if err := db.Model(&database.User{}).Count(&stats.TotalUsers).Error; err != nil {
		return err
	}

	// Users by role
	stats.UsersByRole = make(map[string]int64)
	var roleData []struct {
		Role  string `json:"role"`
		Count int64  `json:"count"`
	}
	
	if err := db.Model(&database.User{}).
		Select("role, COUNT(*) as count").
		Group("role").
		Scan(&roleData).Error; err != nil {
		return err
	}

	for _, data := range roleData {
		stats.UsersByRole[data.Role] = data.Count
	}

	// User growth by month (last 12 months)
	stats.UserGrowth = make([]MonthlyGrowth, 0)
	for i := 11; i >= 0; i-- {
		monthStart := time.Now().AddDate(0, -i, 0).Truncate(24 * time.Hour)
		monthStart = time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, monthStart.Location())
		monthEnd := monthStart.AddDate(0, 1, 0).Add(-time.Second)

		var count int64
		if err := db.Model(&database.User{}).
			Where("created_at BETWEEN ? AND ?", monthStart, monthEnd).
			Count(&count).Error; err != nil {
			return err
		}

		stats.UserGrowth = append(stats.UserGrowth, MonthlyGrowth{
			Month: monthStart.Format("Jan 2006"),
			Count: count,
		})
	}

	return nil
}

// getPaymentMetrics calculates payment-related statistics
func getPaymentMetrics(stats *AdminStats) error {
	db := database.GetDB()

	// Total crypto payment volume
	var cryptoVolume struct {
		Total float64 `json:"total"`
	}
	if err := db.Model(&database.Payment{}).
		Select("COALESCE(SUM(amount + tip_amount), 0) as total").
		Where("status = ?", "confirmed").
		Scan(&cryptoVolume).Error; err != nil {
		return err
	}
	stats.TotalCryptoPayments = cryptoVolume.Total

	// Total alternative payment volume
	var altVolume struct {
		Total float64 `json:"total"`
	}
	if err := db.Model(&database.AlternativePayment{}).
		Select("COALESCE(SUM(amount), 0) as total").
		Where("status = ?", "confirmed").
		Scan(&altVolume).Error; err != nil {
		return err
	}
	stats.TotalAlternativePayments = altVolume.Total

	stats.TotalPaymentVolume = stats.TotalCryptoPayments + stats.TotalAlternativePayments

	// Average transaction size
	var avgTransaction struct {
		Average float64 `json:"average"`
	}
	if err := db.Model(&database.Payment{}).
		Select("COALESCE(AVG(amount + tip_amount), 0) as average").
		Where("status = ?", "confirmed").
		Scan(&avgTransaction).Error; err != nil {
		return err
	}
	stats.AverageTransactionSize = avgTransaction.Average

	// Payment volume growth by month (last 12 months)
	stats.PaymentVolumeGrowth = make([]MonthlyGrowth, 0)
	for i := 11; i >= 0; i-- {
		monthStart := time.Now().AddDate(0, -i, 0).Truncate(24 * time.Hour)
		monthStart = time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, monthStart.Location())
		monthEnd := monthStart.AddDate(0, 1, 0).Add(-time.Second)

		var cryptoSum struct {
			Total float64 `json:"total"`
		}
		if err := db.Model(&database.Payment{}).
			Select("COALESCE(SUM(amount + tip_amount), 0) as total").
			Where("status = ? AND created_at BETWEEN ? AND ?", "confirmed", monthStart, monthEnd).
			Scan(&cryptoSum).Error; err != nil {
			return err
		}

		var altSum struct {
			Total float64 `json:"total"`
		}
		if err := db.Model(&database.AlternativePayment{}).
			Select("COALESCE(SUM(amount), 0) as total").
			Where("status = ? AND created_at BETWEEN ? AND ?", "confirmed", monthStart, monthEnd).
			Scan(&altSum).Error; err != nil {
			return err
		}

		stats.PaymentVolumeGrowth = append(stats.PaymentVolumeGrowth, MonthlyGrowth{
			Month: monthStart.Format("Jan 2006"),
			Value: cryptoSum.Total + altSum.Total,
		})
	}

	return nil
}

// getBillMetrics calculates bill-related statistics
func getBillMetrics(stats *AdminStats) error {
	db := database.GetDB()

	// Total bills
	if err := db.Model(&database.Bill{}).Count(&stats.TotalBills).Error; err != nil {
		return err
	}

	// Bills by status
	stats.BillsByStatus = make(map[string]int64)
	var statusData []struct {
		Status string `json:"status"`
		Count  int64  `json:"count"`
	}
	
	if err := db.Model(&database.Bill{}).
		Select("status, COUNT(*) as count").
		Group("status").
		Scan(&statusData).Error; err != nil {
		return err
	}

	for _, data := range statusData {
		stats.BillsByStatus[data.Status] = data.Count
	}

	// Bill growth by month (last 12 months)
	stats.BillGrowth = make([]MonthlyGrowth, 0)
	for i := 11; i >= 0; i-- {
		monthStart := time.Now().AddDate(0, -i, 0).Truncate(24 * time.Hour)
		monthStart = time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, monthStart.Location())
		monthEnd := monthStart.AddDate(0, 1, 0).Add(-time.Second)

		var count int64
		if err := db.Model(&database.Bill{}).
			Where("created_at BETWEEN ? AND ?", monthStart, monthEnd).
			Count(&count).Error; err != nil {
			return err
		}

		stats.BillGrowth = append(stats.BillGrowth, MonthlyGrowth{
			Month: monthStart.Format("Jan 2006"),
			Count: count,
		})
	}

	return nil
}

// getSubscriberMetrics calculates subscriber-related statistics
func getSubscriberMetrics(stats *AdminStats) error {
	db := database.GetDB()

	// Total subscribers
	if err := db.Model(&database.Subscriber{}).Count(&stats.TotalSubscribers).Error; err != nil {
		return err
	}

	// Subscriber growth by month (last 12 months)
	stats.SubscriberGrowth = make([]MonthlyGrowth, 0)
	for i := 11; i >= 0; i-- {
		monthStart := time.Now().AddDate(0, -i, 0).Truncate(24 * time.Hour)
		monthStart = time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, monthStart.Location())
		monthEnd := monthStart.AddDate(0, 1, 0).Add(-time.Second)

		var count int64
		if err := db.Model(&database.Subscriber{}).
			Where("created_at BETWEEN ? AND ?", monthStart, monthEnd).
			Count(&count).Error; err != nil {
			return err
		}

		stats.SubscriberGrowth = append(stats.SubscriberGrowth, MonthlyGrowth{
			Month: monthStart.Format("Jan 2006"),
			Count: count,
		})
	}

	return nil
}

// getReferralMetrics calculates referral-related statistics from database
func getReferralMetrics(stats *AdminStats) error {
	db := database.GetDB()

	// Check if referrers table exists
	var tableExists bool
	if err := db.Raw("SELECT name FROM sqlite_master WHERE type='table' AND name='referrers'").Scan(&tableExists).Error; err != nil {
		// If table doesn't exist, set default values
		stats.TotalReferrers = 0
		stats.ReferrersByTier = make(map[string]int64)
		stats.TotalCommissionsPaid = 0
		stats.TopReferrers = make([]TopReferrer, 0)
		return nil
	}

	// Total referrers
	if err := db.Table("referrers").Count(&stats.TotalReferrers).Error; err != nil {
		return err
	}

	// Referrers by tier
	stats.ReferrersByTier = make(map[string]int64)
	var tierData []struct {
		Tier  string `json:"tier"`
		Count int64  `json:"count"`
	}
	
	if err := db.Table("referrers").
		Select("tier, COUNT(*) as count").
		Group("tier").
		Scan(&tierData).Error; err != nil {
		return err
	}

	for _, data := range tierData {
		stats.ReferrersByTier[data.Tier] = data.Count
	}

	// Total commissions paid
	var commissionsSum struct {
		Total float64 `json:"total"`
	}
	if err := db.Table("referral_commission_claims").
		Select("COALESCE(SUM(CAST(amount AS REAL)), 0) as total").
		Where("status = ?", "completed").
		Scan(&commissionsSum).Error; err != nil {
		stats.TotalCommissionsPaid = 0
	} else {
		stats.TotalCommissionsPaid = commissionsSum.Total
	}

	// Top referrers
	stats.TopReferrers = make([]TopReferrer, 0)
	var topReferrersData []struct {
		WalletAddress    string  `json:"wallet_address"`
		ReferralCode     string  `json:"referral_code"`
		Tier             string  `json:"tier"`
		TotalReferrals   int64   `json:"total_referrals"`
		TotalCommissions float64 `json:"total_commissions"`
	}
	
	if err := db.Table("referrers").
		Select("wallet_address, referral_code, tier, total_referrals, CAST(total_commissions AS REAL) as total_commissions").
		Order("total_referrals DESC").
		Limit(10).
		Scan(&topReferrersData).Error; err == nil {
		
		for _, data := range topReferrersData {
			stats.TopReferrers = append(stats.TopReferrers, TopReferrer{
				WalletAddress:    data.WalletAddress,
				ReferralCode:     data.ReferralCode,
				Tier:             data.Tier,
				TotalReferrals:   data.TotalReferrals,
				TotalCommissions: data.TotalCommissions,
			})
		}
	}

	return nil
}

// GetBusinessList returns a paginated list of businesses for admin management
func GetBusinessList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	search := c.Query("search")
	
	offset := (page - 1) * limit

	db := database.GetDB()
	query := db.Model(&database.Business{})

	// Apply search filter
	if search != "" {
		query = query.Where("name ILIKE ? OR owner_address ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	var total int64
	query.Count(&total)

	var businesses []database.Business
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&businesses).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get businesses"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"businesses": businesses,
		"total":      total,
		"page":       page,
		"limit":      limit,
	})
}

// GetUserList returns a paginated list of users for admin management
func GetUserList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	search := c.Query("search")
	role := c.Query("role")
	
	offset := (page - 1) * limit

	db := database.GetDB()
	query := db.Model(&database.User{})

	// Apply filters
	if search != "" {
		query = query.Where("address ILIKE ?", "%"+search+"%")
	}
	if role != "" {
		query = query.Where("role = ?", role)
	}

	var total int64
	query.Count(&total)

	var users []database.User
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get users"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"users": users,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}
