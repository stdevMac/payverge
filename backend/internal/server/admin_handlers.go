package server

import (
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
)

// AdminStats represents comprehensive admin dashboard statistics
type AdminStats struct {
	// Business metrics
	TotalBusinesses    int64           `json:"total_businesses"`
	ActiveBusinesses   int64           `json:"active_businesses"`
	InactiveBusinesses int64           `json:"inactive_businesses"`
	BusinessGrowth     []MonthlyGrowth `json:"business_growth"`

	// User metrics
	TotalUsers  int64            `json:"total_users"`
	UsersByRole map[string]int64 `json:"users_by_role"`
	UserGrowth  []MonthlyGrowth  `json:"user_growth"`

	// Payment metrics
	TotalPaymentVolume     float64         `json:"total_payment_volume"`
	PaymentVolumeGrowth    []MonthlyGrowth `json:"payment_volume_growth"`
	AverageTransactionSize float64         `json:"average_transaction_size"`
	// PaymentVolumeCurrency is the ISO code the volume figures are in when
	// every venue with bills uses one currency; empty when they differ (the
	// sums are then not converted) or when there are no bills.
	// PaymentVolumeCurrencies lists every currency that contributed.
	PaymentVolumeCurrency   string   `json:"payment_volume_currency"`
	PaymentVolumeCurrencies []string `json:"payment_volume_currencies"`

	// Bill metrics
	TotalBills      int64            `json:"total_bills"`
	RecognizedBills int64            `json:"recognized_bills"`
	BillsByStatus   map[string]int64 `json:"bills_by_status"`
	BillGrowth      []MonthlyGrowth  `json:"bill_growth"`

	// Operational metrics
	RecentAdminActions  []database.AdminAction `json:"recent_admin_actions"`
	RecentErrors        []database.ErrorLog    `json:"recent_errors"`
	FailedWebhooksCount int                    `json:"failed_webhooks_count"`

	// Merchandise volume (restaurant guest takings processed through the
	// platform — NOT Payverge revenue). Formerly mislabeled total_revenue.
	GrossMerchandiseVolume float64         `json:"gross_merchandise_volume"`
	RevenueGrowth          []MonthlyGrowth `json:"revenue_growth"`
}

// MonthlyGrowth represents growth data for a specific month
type MonthlyGrowth struct {
	Month string  `json:"month"`
	Count int64   `json:"count"`
	Value float64 `json:"value,omitempty"` // For revenue/volume data
}

var adminStatsNow = time.Now

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

	// Get operational metrics
	if err := getOperationalMetrics(&stats); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get operational metrics"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// getBusinessMetrics calculates business-related statistics
func getBusinessMetrics(stats *AdminStats) error {
	// Single registry entry point: kind=real by default (Task 12).
	realFilter := database.AdminBusinessFilter{Kind: string(database.BusinessKindReal)}
	total, err := database.CountBusinessesForAdmin(realFilter)
	if err != nil {
		return err
	}
	stats.TotalBusinesses = total

	db := database.GetDB()
	var activeCount int64
	if err := database.ApplyAdminBusinessFilter(db.Model(&database.Business{}), realFilter).
		Where("businesses.is_active = ?", true).
		Count(&activeCount).Error; err != nil {
		return err
	}
	stats.ActiveBusinesses = activeCount
	stats.InactiveBusinesses = stats.TotalBusinesses - stats.ActiveBusinesses

	// Business growth by month (last 12 months) — same kind=real scope.
	stats.BusinessGrowth = make([]MonthlyGrowth, 0)
	for i := 11; i >= 0; i-- {
		monthStart := time.Now().AddDate(0, -i, 0).Truncate(24 * time.Hour)
		monthStart = time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, monthStart.Location())
		monthEnd := monthStart.AddDate(0, 1, 0).Add(-time.Second)

		var count int64
		if err := database.ApplyAdminBusinessFilter(db.Model(&database.Business{}), realFilter).
			Where("businesses.created_at BETWEEN ? AND ?", monthStart, monthEnd).
			Count(&count).Error; err != nil {
			return err
		}

		stats.BusinessGrowth = append(stats.BusinessGrowth, MonthlyGrowth{
			Month: monthStart.Format("Jan 2006"),
			Count: count,
		})
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
	now := adminStatsNow()
	// Single source: the recognized payment ledger. Do not branch on a business
	// count vs aggregate mismatch and recompute (Task 12) — that dual-path
	// papered over registry drift instead of fixing it.
	summary, err := database.GetRecognizedPaymentSummaryForAllBusinesses(time.Time{}, now)
	if err != nil {
		return err
	}

	netRevenueCents := summary.TotalRevenueCents
	netTipCents := summary.TotalTipCents
	grossRevenueCents := summary.GrossRevenueCents
	grossTipCents := summary.GrossTipCents
	positiveEventCount := summary.PositiveEventCount

	totalVolumeCents := netRevenueCents + netTipCents
	// Gross merchandise volume = restaurant net recognized takings (ex-tips).
	// This is GMV processed by Payverge, not platform subscription revenue.
	stats.GrossMerchandiseVolume = float64(netRevenueCents) / 100.0
	stats.TotalPaymentVolume = float64(totalVolumeCents) / 100.0

	// Average transaction size
	grossVolumeCents := grossRevenueCents + grossTipCents
	if positiveEventCount > 0 {
		stats.AverageTransactionSize = float64(grossVolumeCents) / float64(positiveEventCount) / 100.0
	}

	// The ledger sums cents across venues without converting. Name the
	// currency so the dashboard does not label peso sums as dollars.
	currencies, err := paymentVolumeCurrencies()
	if err != nil {
		return err
	}
	stats.PaymentVolumeCurrencies = currencies
	if len(currencies) == 1 {
		stats.PaymentVolumeCurrency = currencies[0]
	}

	// Payment volume growth by month (last 12 months)
	stats.PaymentVolumeGrowth = make([]MonthlyGrowth, 0)
	stats.RevenueGrowth = make([]MonthlyGrowth, 0)
	firstMonthStart := now.AddDate(0, -11, 0).Truncate(24 * time.Hour)
	firstMonthStart = time.Date(firstMonthStart.Year(), firstMonthStart.Month(), 1, 0, 0, 0, 0, firstMonthStart.Location())
	lastMonthEnd := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, 1, 0)
	monthlySummaries, err := database.GetRecognizedPaymentMonthlySummaryForAllBusinesses(firstMonthStart, lastMonthEnd)
	if err != nil {
		return err
	}
	for i := 11; i >= 0; i-- {
		monthStart := now.AddDate(0, -i, 0).Truncate(24 * time.Hour)
		monthStart = time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, monthStart.Location())
		monthSummary := monthlySummaries[monthStart.Format("2006-01")]
		monthVolumeCents := monthSummary.TotalRevenueCents + monthSummary.TotalTipCents

		stats.PaymentVolumeGrowth = append(stats.PaymentVolumeGrowth, MonthlyGrowth{
			Month: monthStart.Format("Jan 2006"),
			Count: monthSummary.PositiveEventCount,
			Value: float64(monthVolumeCents) / 100.0,
		})
		stats.RevenueGrowth = append(stats.RevenueGrowth, MonthlyGrowth{
			Month: monthStart.Format("Jan 2006"),
			Count: monthSummary.PositiveEventCount,
			Value: float64(monthSummary.TotalRevenueCents) / 100.0,
		})
	}

	return nil
}

// paymentVolumeCurrencies returns the distinct resolved currencies (display,
// then default, then USD) of venues that have at least one bill, sorted.
func paymentVolumeCurrencies() ([]string, error) {
	db := database.GetDB()
	var rows []database.Business
	if err := db.Model(&database.Business{}).
		Select("display_currency", "default_currency").
		Where("id IN (?)", db.Model(&database.Bill{}).Distinct("business_id")).
		Distinct().
		Find(&rows).Error; err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(rows))
	out := make([]string, 0, len(rows))
	for i := range rows {
		code := strings.ToUpper(strings.TrimSpace(database.BusinessDisplayCurrency(&rows[i])))
		if _, dup := seen[code]; dup {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	sort.Strings(out)
	return out, nil
}

// getBillMetrics calculates bill-related statistics
func getBillMetrics(stats *AdminStats) error {
	db := database.GetDB()
	now := adminStatsNow()

	// Total bills
	if err := db.Model(&database.Bill{}).Count(&stats.TotalBills).Error; err != nil {
		return err
	}
	// Single source for recognized bills — live ledger, no dual-count reconcile (Task 12).
	recognizedBills, err := database.GetRecognizedPaymentBillCountForAllBusinesses(time.Time{}, now)
	if err != nil {
		return err
	}
	stats.RecognizedBills = recognizedBills

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
		monthStart := now.AddDate(0, -i, 0).Truncate(24 * time.Hour)
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

// getOperationalMetrics retrieves recent admin actions, error logs, and failed webhooks
func getOperationalMetrics(stats *AdminStats) error {
	db := database.GetDB()

	// Recent admin actions (last 10)
	stats.RecentAdminActions = make([]database.AdminAction, 0)
	if db.Migrator().HasTable("admin_actions") {
		if err := db.Model(&database.AdminAction{}).
			Order("created_at DESC").
			Limit(10).
			Find(&stats.RecentAdminActions).Error; err != nil {
			return err
		}
	}

	// Recent errors (last 5). Soft-fail: scrubbed clones historically set
	// additional_info (serializer:json) to the non-JSON string "[redacted]",
	// which made Find fail and 500'd the entire admin stats poll.
	stats.RecentErrors = make([]database.ErrorLog, 0)
	if db.Migrator().HasTable("error_logs") {
		var errors []database.ErrorLog
		if err := db.Model(&database.ErrorLog{}).
			Order("created_at DESC").
			Limit(5).
			Find(&errors).Error; err != nil {
			log.Printf("getOperationalMetrics: recent error_logs (non-fatal): %v", err)
		} else {
			stats.RecentErrors = errors
		}
	}

	// Failed webhooks count — the dead-letter queue only. Previously this also
	// counted 'processing' (in-flight retries), overstating "needs attention"
	// with transient mid-flight events. Count terminal failures only. The
	// status column is indexed so this admin-poll query
	// stays an index lookup, not a full scan. (audit F2)
	var failedWebhooks int64
	if db.Migrator().HasTable("webhook_events") {
		if err := db.Model(&database.WebhookEvent{}).
			Where("status = ?", "failed").
			Count(&failedWebhooks).Error; err != nil {
			return err
		}
	}
	stats.FailedWebhooksCount = int(failedWebhooks)

	return nil
}

// AdminBusinessListItem is the enriched response type for the admin business list.
type AdminBusinessListItem struct {
	ID           uint       `json:"id"`
	Name         string     `json:"name"`
	OwnerName    string     `json:"owner_name"`
	OwnerEmail   string     `json:"owner_email"`
	Status       string     `json:"status"`
	Kind         string     `json:"kind"`
	CreatedAt    time.Time  `json:"created_at"`
	LastActiveAt *time.Time `json:"last_active_at"`
}

// GetBusinessList returns a paginated list of businesses for admin management.
// Defaults to kind=real via database.ListBusinessesForAdmin (Task 12).
// Query: kind=real|demo|test|all.
func GetBusinessList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	search := c.Query("search")
	status := c.Query("status")

	kind := strings.TrimSpace(c.Query("kind"))
	if kind == "" {
		kind = string(database.BusinessKindReal)
	}

	businesses, total, err := database.ListBusinessesForAdmin(database.AdminBusinessFilter{
		Kind:   kind,
		Status: status,
		Search: search,
		Page:   page,
		Limit:  limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get businesses"})
		return
	}

	db := database.GetDB()

	// Collect user IDs to batch-load owner emails
	userIDs := make([]uint, 0, len(businesses))
	for _, b := range businesses {
		if b.UserID != nil {
			userIDs = append(userIDs, *b.UserID)
		}
	}

	emailMap := make(map[uint]string)
	if len(userIDs) > 0 {
		var users []database.User
		if err := db.Select("id, email").Where("id IN ?", userIDs).Find(&users).Error; err == nil {
			for _, u := range users {
				emailMap[u.ID] = u.Email
			}
		}
	}

	// Build enriched response
	items := make([]AdminBusinessListItem, 0, len(businesses))
	for _, b := range businesses {
		lastActiveAt := b.UpdatedAt
		item := AdminBusinessListItem{
			ID:           b.ID,
			Name:         b.Name,
			OwnerName:    b.OwnerName,
			Status:       database.AdminBusinessStatus(&b),
			Kind:         string(b.Kind),
			CreatedAt:    b.CreatedAt,
			LastActiveAt: &lastActiveAt,
		}
		if b.UserID != nil {
			if email, ok := emailMap[*b.UserID]; ok {
				item.OwnerEmail = email
			}
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"businesses": items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"kind":       kind,
	})
}

// AdminUserListItem is the enriched response type for the admin user list.
type AdminUserListItem struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	BusinessName string `json:"business_name"`
	Status       string `json:"status"`
	JoinedAt     string `json:"joined_at"`
	// Merge affordance for case-colliding legacy duplicates (Task 22).
	// CanMerge is true when another live account shares lower(email); MergeWith
	// lists the sibling user ids (primary survivor first when this row is not it).
	CanMerge  bool   `json:"can_merge,omitempty"`
	MergeWith []uint `json:"merge_with,omitempty"`
}

// GetUserList returns a paginated list of users for admin management
func GetUserList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	search := c.Query("search")
	role := c.Query("role")
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))

	offset := (page - 1) * limit

	db := database.GetDB()
	query := db.Model(&database.User{})

	joinBusinesses := search != "" || status != ""
	if joinBusinesses {
		query = query.Joins("LEFT JOIN businesses ON businesses.user_id = users.id")
	}

	// Apply filters on user and business fields
	if search != "" {
		searchTerm := "%" + strings.ToLower(search) + "%"
		query = query.Where(
			"LOWER(users.name) LIKE ? OR LOWER(users.email) LIKE ? OR LOWER(users.address) LIKE ? OR LOWER(businesses.name) LIKE ?",
			searchTerm,
			searchTerm,
			searchTerm,
			searchTerm,
		)
	}
	if role != "" {
		query = query.Where("users.role = ?", role)
	}

	switch status {
	case database.BusinessStatusActive:
		query = query.Where("businesses.is_active = ? AND businesses.closed_at IS NULL", true)
	case database.BusinessStatusSuspended:
		query = query.Where("businesses.is_active = ? AND businesses.closed_at IS NULL", false)
	case database.BusinessStatusClosed:
		query = query.Where("businesses.closed_at IS NOT NULL")
	}

	// When joining businesses, a single user can match multiple business rows
	// (multi-business owners) and inflate both Count and Find. Count distinct
	// users instead, and Group by users.id on the list query.
	var total int64
	countQuery := query
	if joinBusinesses {
		countQuery = query.Distinct("users.id")
	}
	if err := countQuery.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count users"})
		return
	}

	// Fetch users
	var users []database.User
	listQuery := query
	if joinBusinesses {
		listQuery = listQuery.Group("users.id")
	}
	if err := listQuery.Offset(offset).Limit(limit).Order("users.created_at DESC").Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get users"})
		return
	}

	// Collect user IDs to batch-load businesses
	userIDs := make([]uint, len(users))
	for i, u := range users {
		userIDs[i] = u.ID
	}

	// Batch-load businesses by user_id
	bizMap := make(map[uint]database.Business)
	if len(userIDs) > 0 {
		var businesses []database.Business
		if err := db.Where("user_id IN ?", userIDs).Find(&businesses).Error; err == nil {
			for _, b := range businesses {
				if b.UserID != nil {
					bizMap[*b.UserID] = b
				}
			}
		}
	}

	// Index duplicate-email groups so the list can surface a merge affordance
	// on every sibling of a case-colliding mailbox (Task 22).
	dupByUserID := map[uint]database.EmailDuplicateGroup{}
	if groups, err := database.ListEmailDuplicateGroups(); err == nil {
		for _, g := range groups {
			for _, id := range g.UserIDs {
				dupByUserID[id] = g
			}
		}
	}

	// Build enriched response
	items := make([]AdminUserListItem, 0, len(users))
	for _, u := range users {
		item := AdminUserListItem{
			ID:       u.ID,
			Name:     u.Name,
			Email:    u.Email,
			JoinedAt: u.CreatedAt.Format(time.RFC3339),
		}
		if biz, ok := bizMap[u.ID]; ok {
			item.BusinessName = biz.Name
			item.Status = database.AdminBusinessStatus(&biz)
		}
		if g, ok := dupByUserID[u.ID]; ok {
			item.CanMerge = g.CanMerge
			// Sibling ids excluding self; primary survivor first.
			siblings := make([]uint, 0, len(g.UserIDs))
			if g.PrimaryUserID != u.ID {
				siblings = append(siblings, g.PrimaryUserID)
			}
			for _, id := range g.UserIDs {
				if id == u.ID || id == g.PrimaryUserID {
					continue
				}
				siblings = append(siblings, id)
			}
			item.MergeWith = siblings
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"users": items,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}
