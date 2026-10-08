package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/money"
)

// ReportScheduler manages the scheduling and sending of email reports
type ReportScheduler struct {
	db             *database.DB
	emailServer    *emails.EmailServer
	analyticsServ  *analytics.AnalyticsService
	renderer       ReportRenderer
	sender         ReportSender
	pluginEnabled  func(database.ReportSchedule) (bool, error)
	alertOperator  func(context.Context, database.ReportDelivery) error
	afterTransport func(database.ReportDelivery) error
	leaseDuration  time.Duration
	maxAttempts    int
	retryBase      time.Duration
	retryMax       time.Duration
	stopChan       chan struct{}
	wg             sync.WaitGroup
	mu             sync.Mutex
	isRunning      bool
}

// NewReportScheduler creates a new report scheduler instance
func NewReportScheduler(db *database.DB, emailServer *emails.EmailServer, analyticsServ *analytics.AnalyticsService) *ReportScheduler {
	rs := &ReportScheduler{
		db:            db,
		emailServer:   emailServer,
		analyticsServ: analyticsServ,
		renderer:      reportAnalyticsRenderer{db: db, analytics: analyticsServ},
		sender:        emailReportSender{server: emailServer},
		pluginEnabled: defaultReportPluginEnabled,
		leaseDuration: defaultReportLeaseDuration,
		maxAttempts:   defaultReportMaxAttempts,
		retryBase:     defaultReportRetryBase,
		retryMax:      defaultReportRetryMax,
		stopChan:      make(chan struct{}),
	}
	rs.alertOperator = defaultReportDeliveryAlert(db)
	return rs
}

// Start begins the report scheduling service
func (rs *ReportScheduler) Start() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.isRunning {
		return fmt.Errorf("report scheduler is already running")
	}

	rs.isRunning = true
	rs.stopChan = make(chan struct{}) // re-create so a Stop→Start cycle is safe
	log.Println("Starting report scheduler service...")

	// Start the main scheduler loop
	rs.wg.Add(1)
	go rs.schedulerLoop()

	log.Println("Report scheduler service started successfully")
	return nil
}

// Stop gracefully stops the report scheduling service
func (rs *ReportScheduler) Stop() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if !rs.isRunning {
		return fmt.Errorf("report scheduler is not running")
	}

	log.Println("Stopping report scheduler service...")
	close(rs.stopChan)
	rs.wg.Wait()
	rs.isRunning = false
	log.Println("Report scheduler service stopped")
	return nil
}

// schedulerLoop is the main loop that checks for due reports
func (rs *ReportScheduler) schedulerLoop() {
	defer rs.wg.Done()

	// Check every minute for due reports
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Run initial check immediately
	logger.SafeTick("report-scheduler", rs.checkAndSendDueReports)

	for {
		select {
		case <-ticker.C:
			logger.SafeTick("report-scheduler", rs.checkAndSendDueReports)
		case <-rs.stopChan:
			return
		}
	}
}

// checkAndSendDueReports checks for reports that are due and sends them
func (rs *ReportScheduler) checkAndSendDueReports() {
	rs.runOnce(time.Now())
}

// resolveReportCurrency returns the ISO 4217 code report money should be
// rendered in: the business display currency, then the default currency, then
// USD as a last resort.
func resolveReportCurrency(business *database.Business) string {
	currency := strings.TrimSpace(business.DisplayCurrency)
	if currency == "" {
		currency = strings.TrimSpace(business.DefaultCurrency)
	}
	if currency == "" {
		currency = "USD"
	}
	return currency
}

// formatReportMoney renders a major-unit dollar amount (analytics reports emit
// float64 dollars) as "<CODE> <amount>" with the currency's real decimal count,
// matching the operator-facing Telegram money format. The float is converted to
// stored cents first so money.MajorUnitString applies the zero-/three-decimal
// rules (JPY -> "JPY 1000", USD -> "USD 10.00", BHD -> "BHD 10.000").
func formatReportMoney(majorUnits float64, currency string) string {
	currency = strings.TrimSpace(currency)
	if currency == "" {
		currency = "USD"
	}
	cents := int64(math.Round(majorUnits * 100))
	return currency + " " + money.MajorUnitString(cents, currency)
}

// reportPluginNameForFrequency maps a schedule frequency to the catalog plugin
// name that owns it, so the worker can verify the plugin is still enabled.
func reportPluginNameForFrequency(freq database.ReportFrequency) string {
	switch freq {
	case database.ReportFrequencyDaily:
		return "daily_email_report"
	case database.ReportFrequencyWeekly:
		return "weekly_email_report"
	default:
		return ""
	}
}

func configInt(config map[string]interface{}, key string, fallback int) int {
	switch val := config[key].(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case json.Number:
		if parsed, err := val.Int64(); err == nil {
			return int(parsed)
		}
	case string:
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return fallback
}

func configBool(config map[string]interface{}, key string, fallback bool) bool {
	switch val := config[key].(type) {
	case bool:
		return val
	case string:
		if parsed, err := strconv.ParseBool(val); err == nil {
			return parsed
		}
	}
	return fallback
}

func configString(config map[string]interface{}, key string, fallback string) string {
	if val, ok := config[key].(string); ok && val != "" {
		return val
	}
	return fallback
}

func validateReportScheduleConfig(hour, dayOfWeek int, timezone string, weekly bool) error {
	if hour < 0 || hour > 23 {
		return fmt.Errorf("hour must be between 0 and 23")
	}
	if weekly && (dayOfWeek < 0 || dayOfWeek > 6) {
		return fmt.Errorf("day_of_week must be between 0 and 6")
	}
	if timezone == "" {
		return fmt.Errorf("timezone is required")
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return fmt.Errorf("timezone is invalid: %w", err)
	}
	return nil
}

// SyncWeeklySchedule creates or updates a weekly report schedule
// This should be called when the weekly report plugin is enabled/configured
func (rs *ReportScheduler) SyncWeeklySchedule(businessID uint, config map[string]interface{}) error {
	// Extract config values
	dayOfWeek := configInt(config, "day_of_week", 1)
	hour := configInt(config, "hour", 9)
	timezone := configString(config, "timezone", "UTC")
	enabled := configBool(config, "enabled", true)
	if err := validateReportScheduleConfig(hour, dayOfWeek, timezone, true); err != nil {
		return err
	}

	// Get existing schedule or create new
	schedule, err := rs.db.GetReportScheduleByBusinessAndFrequency(businessID, database.ReportFrequencyWeekly)
	if err != nil {
		return err
	}
	if schedule == nil {
		// Create new schedule
		schedule = &database.ReportSchedule{
			BusinessID: businessID,
			Frequency:  database.ReportFrequencyWeekly,
			DayOfWeek:  dayOfWeek,
			Hour:       hour,
			Minute:     0,
			Timezone:   timezone,
			IsActive:   enabled,
		}
		schedule.NextSendAt = database.CalculateNextSendTime(schedule)
		return rs.db.CreateReportSchedule(schedule)
	}

	// Update existing schedule
	schedule.DayOfWeek = dayOfWeek
	schedule.Hour = hour
	schedule.Timezone = timezone
	schedule.IsActive = enabled
	schedule.NextSendAt = database.CalculateNextSendTime(schedule)
	return rs.db.UpdateReportSchedule(schedule)
}

// SyncDailySchedule creates or updates a daily report schedule
// This should be called when the daily report plugin is enabled/configured
func (rs *ReportScheduler) SyncDailySchedule(businessID uint, config map[string]interface{}) error {
	// Extract config values
	hour := configInt(config, "hour", 8)
	timezone := configString(config, "timezone", "UTC")
	enabled := configBool(config, "enabled", true)
	if err := validateReportScheduleConfig(hour, 0, timezone, false); err != nil {
		return err
	}

	// Get existing schedule or create new
	schedule, err := rs.db.GetReportScheduleByBusinessAndFrequency(businessID, database.ReportFrequencyDaily)
	if err != nil {
		return err
	}
	if schedule == nil {
		// Create new schedule
		schedule = &database.ReportSchedule{
			BusinessID: businessID,
			Frequency:  database.ReportFrequencyDaily,
			Hour:       hour,
			Minute:     0,
			Timezone:   timezone,
			IsActive:   enabled,
		}
		schedule.NextSendAt = database.CalculateNextSendTime(schedule)
		return rs.db.CreateReportSchedule(schedule)
	}

	// Update existing schedule
	schedule.Hour = hour
	schedule.Timezone = timezone
	schedule.IsActive = enabled
	schedule.NextSendAt = database.CalculateNextSendTime(schedule)
	return rs.db.UpdateReportSchedule(schedule)
}

// DisableWeeklySchedule deactivates a weekly report schedule without deleting user configuration.
func (rs *ReportScheduler) DisableWeeklySchedule(businessID uint) error {
	return rs.disableSchedule(businessID, database.ReportFrequencyWeekly)
}

// DisableDailySchedule deactivates a daily report schedule without deleting user configuration.
func (rs *ReportScheduler) DisableDailySchedule(businessID uint) error {
	return rs.disableSchedule(businessID, database.ReportFrequencyDaily)
}

func (rs *ReportScheduler) disableSchedule(businessID uint, frequency database.ReportFrequency) error {
	schedule, err := rs.db.GetReportScheduleByBusinessAndFrequency(businessID, frequency)
	if err != nil || schedule == nil {
		return err
	}

	schedule.IsActive = false
	return rs.db.UpdateReportSchedule(schedule)
}

// formatWeeklyTopItems builds the weekly-email top-items fragment in the
// operator's email template family (en / es / es_ar).
func formatWeeklyTopItems(itemStats []analytics.ItemStats, language string) string {
	family := weeklyDigestLanguageFamily(language)
	if len(itemStats) == 0 {
		switch family {
		case "es", "es_ar":
			return "No se vendieron artículos esta semana"
		default:
			return "No items sold this week"
		}
	}
	var b strings.Builder
	for i, item := range itemStats {
		switch family {
		case "es", "es_ar":
			fmt.Fprintf(&b, "%d. %s - %d vendidos\n", i+1, item.ItemName, item.TotalSold)
		default:
			fmt.Fprintf(&b, "%d. %s - %d sold\n", i+1, item.ItemName, item.TotalSold)
		}
	}
	return b.String()
}

func weeklyDigestLanguageFamily(language string) string {
	l := strings.ToLower(strings.TrimSpace(language))
	l = strings.ReplaceAll(l, "_", "-")
	switch {
	case l == "es-ar" || strings.HasPrefix(l, "es-ar"):
		return "es_ar"
	case l == "es" || strings.HasPrefix(l, "es"):
		return "es"
	default:
		return "en"
	}
}
