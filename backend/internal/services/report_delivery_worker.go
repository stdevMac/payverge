package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	operationalalerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
)

const (
	defaultReportLeaseDuration = 5 * time.Minute
	defaultReportMaxAttempts   = 5
	defaultReportRetryBase     = time.Minute
	defaultReportRetryMax      = time.Hour
	// reportScheduleProcessTimeout is the floor for one due schedule so a stuck
	// send cannot hold the minute tick open. runOnce raises it to the
	// scheduler's lease when that lease is longer, so the default
	// defaultReportLeaseDuration is not cut short. The lease default itself
	// is unchanged.
	reportScheduleProcessTimeout = 2 * time.Minute
)

// ErrReportWorkerCrash is used by deterministic failure-injection tests to
// model process death after provider acceptance and before database ack.
var ErrReportWorkerCrash = errors.New("simulated report worker crash")

type permanentReportDeliveryError struct{ err error }

func (e permanentReportDeliveryError) Error() string { return e.err.Error() }
func (e permanentReportDeliveryError) Unwrap() error { return e.err }

// PermanentReportDeliveryError marks configuration or recipient failures that
// cannot recover through retrying the same delivery.
func PermanentReportDeliveryError(err error) error {
	if err == nil {
		return nil
	}
	return permanentReportDeliveryError{err: err}
}

func isPermanentReportDeliveryError(err error) bool {
	var target permanentReportDeliveryError
	return errors.As(err, &target)
}

// ReportEmail is the rendered, provider-neutral scheduled-report message.
type ReportEmail struct {
	Frequency    database.ReportFrequency
	Recipients   []string
	OwnerName    string
	TotalOrders  string
	TotalRevenue string
	AverageBill  string
	TopItems     string
	DashboardURL string
	Language     string
	WindowStart  time.Time
	WindowEnd    time.Time
}

type ReportRenderer interface {
	Render(context.Context, database.ReportSchedule, database.ReportDelivery) (ReportEmail, error)
}

type ReportSender interface {
	Send(context.Context, ReportEmail, string) error
}

type reportAnalyticsRenderer struct {
	db        *database.DB
	analytics *analytics.AnalyticsService
}

func (r reportAnalyticsRenderer) Render(_ context.Context, schedule database.ReportSchedule, delivery database.ReportDelivery) (ReportEmail, error) {
	if r.db == nil || r.analytics == nil {
		return ReportEmail{}, errors.New("report analytics is unavailable")
	}
	business, err := r.db.GetBusinessByID(schedule.BusinessID)
	if err != nil {
		return ReportEmail{}, fmt.Errorf("get report business: %w", err)
	}
	if business.Email == "" {
		return ReportEmail{}, PermanentReportDeliveryError(errors.New("business has no report email address"))
	}
	owner := business.OwnerName
	if owner == "" {
		owner = "Business Owner"
	}
	message := ReportEmail{
		Frequency:    schedule.Frequency,
		Recipients:   []string{business.Email},
		OwnerName:    owner,
		DashboardURL: BuildBusinessDashboardURL(business),
		Language:     DetermineBusinessOwnerLanguage(business),
		WindowStart:  delivery.WindowStart,
		WindowEnd:    delivery.WindowEnd,
	}
	currency := resolveReportCurrency(business)

	switch schedule.Frequency {
	case database.ReportFrequencyDaily:
		report, reportErr := r.analytics.GetDailySales(schedule.BusinessID, delivery.WindowStart.In(database.ResolveLocation(schedule.Timezone)))
		if reportErr != nil {
			return ReportEmail{}, fmt.Errorf("render daily analytics: %w", reportErr)
		}
		message.TotalOrders = fmt.Sprintf("%d", report.TransactionCount)
		message.TotalRevenue = formatReportMoney(report.TotalRevenue, currency)
		message.AverageBill = formatReportMoney(report.AverageTicket, currency)
		if _, enqueueErr := EnqueueTelegramDailySummary(PluginDailySummary{
			BusinessID:   schedule.BusinessID,
			Date:         delivery.WindowStart,
			RevenueCents: int64(math.Round(report.TotalRevenue * 100)),
			OrderCount:   report.TransactionCount,
			PaymentCount: report.TransactionCount,
			Currency:     currency,
			GeneratedAt:  time.Now().UTC(),
		}); enqueueErr != nil {
			log.Printf("Error enqueueing Telegram daily summary for business ID %d: %v", schedule.BusinessID, enqueueErr)
		}
	case database.ReportFrequencyWeekly:
		summary, reportErr := r.analytics.GetPaymentWindowSummary(schedule.BusinessID, delivery.WindowStart, delivery.WindowEnd)
		if reportErr != nil {
			return ReportEmail{}, fmt.Errorf("render weekly analytics: %w", reportErr)
		}
		items, itemsErr := r.analytics.GetPopularItemsInWindow(schedule.BusinessID, 3, delivery.WindowStart, delivery.WindowEnd)
		if itemsErr != nil {
			return ReportEmail{}, fmt.Errorf("render weekly popular items: %w", itemsErr)
		}
		message.TotalOrders = fmt.Sprintf("%d", summary.TransactionCount)
		message.TotalRevenue = formatReportMoney(summary.TotalRevenue, currency)
		message.AverageBill = formatReportMoney(summary.AverageTicket, currency)
		message.TopItems = formatWeeklyTopItems(items, message.Language)
	default:
		return ReportEmail{}, PermanentReportDeliveryError(fmt.Errorf("unsupported report frequency %q", schedule.Frequency))
	}
	return message, nil
}

type emailReportSender struct{ server *emails.EmailServer }

func (s emailReportSender) Send(_ context.Context, message ReportEmail, idempotencyKey string) error {
	if s.server == nil {
		return PermanentReportDeliveryError(errors.New("email provider is disabled"))
	}
	switch message.Frequency {
	case database.ReportFrequencyDaily:
		return s.server.SendDailySummaryEmailIdempotent(
			message.Recipients, message.OwnerName, message.TotalOrders, message.TotalRevenue,
			message.AverageBill, message.DashboardURL, message.Language, idempotencyKey,
		)
	case database.ReportFrequencyWeekly:
		return s.server.SendWeeklyAnalyticsEmailIdempotent(
			message.Recipients, message.OwnerName, message.TotalOrders, message.TotalRevenue,
			message.AverageBill, message.TopItems, message.DashboardURL, message.Language, idempotencyKey,
		)
	default:
		return PermanentReportDeliveryError(fmt.Errorf("unsupported report frequency %q", message.Frequency))
	}
}

// DisabledReportSender gives callers and tests an explicit fail-closed provider
// rather than allowing a nil pointer panic.
type DisabledReportSender struct{}

func (DisabledReportSender) Send(context.Context, ReportEmail, string) error {
	return PermanentReportDeliveryError(errors.New("email provider is disabled"))
}

func (rs *ReportScheduler) runOnce(now time.Time) {
	if rs == nil || rs.db == nil {
		return
	}
	schedules, err := rs.db.GetDueReportSchedules(now)
	if err != nil {
		log.Printf("Error fetching due report schedules: %v", err)
		return
	}
	for i := range schedules {
		// Per-schedule deadline. At least the lease this scheduler was given,
		// so the default 5-minute lease is not cut short by the 2-minute floor.
		timeout := reportScheduleProcessTimeout
		if rs.leaseDuration > timeout {
			timeout = rs.leaseDuration
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		rs.processDueSchedule(ctx, schedules[i], now)
		cancel()
	}
}

func (rs *ReportScheduler) processDueSchedule(ctx context.Context, schedule database.ReportSchedule, now time.Time) {
	enabled, err := rs.pluginEnabled(schedule)
	if err != nil {
		log.Printf("Error checking report provider for schedule %d: %v", schedule.ID, err)
		return
	}
	var providerDisabledErr error
	if !enabled {
		schedule.IsActive = false
		if err := rs.db.UpdateReportSchedule(&schedule); err != nil {
			log.Printf("Failed to deactivate orphaned report schedule %d: %v", schedule.ID, err)
		}
		// Preserve an auditable terminal delivery and operator alert for the due
		// report. Silently deactivating the schedule makes a promised email vanish
		// with no recovery state in the dashboard.
		providerDisabledErr = PermanentReportDeliveryError(errors.New("email provider is disabled"))
	}

	delivery, _, err := rs.db.EnsureReportDelivery(&schedule, schedule.NextSendAt)
	if err != nil {
		log.Printf("Error ensuring report delivery for schedule %d: %v", schedule.ID, err)
		return
	}
	token, err := newReportLeaseToken()
	if err != nil {
		log.Printf("Error generating report lease token: %v", err)
		return
	}
	claimed, err := rs.db.ClaimReportDelivery(delivery.ID, token, now, rs.leaseDuration, rs.maxAttempts)
	if err != nil {
		log.Printf("Error claiming report delivery %d: %v", delivery.ID, err)
		return
	}
	if claimed == nil {
		rs.deadLetterExpiredFinalLease(ctx, delivery.ID, now)
		return
	}
	if providerDisabledErr != nil {
		rs.failClaimedDelivery(ctx, *claimed, now, providerDisabledErr)
		return
	}
	rs.processClaimedDelivery(ctx, schedule, *claimed, now)
}

func (rs *ReportScheduler) deadLetterExpiredFinalLease(ctx context.Context, deliveryID uint, now time.Time) {
	const outcomeUnknown = "report worker lease expired after final attempt; provider delivery outcome is unknown"
	won, err := rs.db.DeadLetterExpiredReportDelivery(deliveryID, now, rs.maxAttempts, outcomeUnknown)
	if err != nil {
		log.Printf("Error closing expired final report lease %d: %v", deliveryID, err)
		return
	}
	if !won || rs.alertOperator == nil {
		return
	}
	fresh, err := rs.db.GetReportDeliveryByID(deliveryID)
	if err == nil {
		if alertErr := rs.alertOperator(ctx, *fresh); alertErr != nil {
			log.Printf("Error alerting operator for expired report lease %d: %v", deliveryID, alertErr)
		}
	}
}

func (rs *ReportScheduler) processClaimedDelivery(ctx context.Context, schedule database.ReportSchedule, delivery database.ReportDelivery, now time.Time) {
	message, err := rs.renderer.Render(ctx, schedule, delivery)
	if err == nil {
		err = rs.sender.Send(ctx, message, delivery.IdempotencyKey)
	}
	if err == nil && rs.afterTransport != nil {
		if crashErr := rs.afterTransport(delivery); crashErr != nil {
			// Model a process disappearing: do not release, fail, or acknowledge the
			// lease. A restarted replica recovers it after expiry.
			return
		}
	}
	if err != nil {
		rs.failClaimedDelivery(ctx, delivery, now, err)
		return
	}

	next := database.CalculateNextSendTimeFrom(&schedule, schedule.NextSendAt.Add(time.Nanosecond))
	won, ackErr := rs.db.AcknowledgeReportDelivery(delivery.ID, delivery.LeaseToken, now, next)
	if ackErr != nil {
		log.Printf("Error acknowledging report delivery %d: %v", delivery.ID, ackErr)
		return
	}
	if won {
		log.Printf("Successfully sent %s report for business ID %d", schedule.Frequency, schedule.BusinessID)
	}
}

func (rs *ReportScheduler) failClaimedDelivery(ctx context.Context, delivery database.ReportDelivery, now time.Time, cause error) {
	permanent := isPermanentReportDeliveryError(cause)
	exhausted := delivery.AttemptCount >= rs.maxAttempts
	if permanent || exhausted {
		won, err := rs.db.DeadLetterReportDelivery(delivery.ID, delivery.LeaseToken, now, cause.Error())
		if err != nil {
			log.Printf("Error dead-lettering report delivery %d: %v", delivery.ID, err)
			return
		}
		if won && rs.alertOperator != nil {
			fresh, getErr := rs.db.GetReportDeliveryByID(delivery.ID)
			if getErr == nil {
				if alertErr := rs.alertOperator(ctx, *fresh); alertErr != nil {
					log.Printf("Error alerting operator for report delivery %d: %v", delivery.ID, alertErr)
				}
			}
		}
		return
	}
	nextAttempt := now.Add(rs.retryDelay(delivery.AttemptCount))
	if _, err := rs.db.RetryReportDelivery(delivery.ID, delivery.LeaseToken, nextAttempt, cause.Error()); err != nil {
		log.Printf("Error scheduling report delivery %d retry: %v", delivery.ID, err)
	}
}

func (rs *ReportScheduler) retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := rs.retryBase
	for i := 1; i < attempt && delay < rs.retryMax; i++ {
		delay *= 2
		if delay > rs.retryMax {
			delay = rs.retryMax
		}
	}
	return delay
}

func newReportLeaseToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func defaultReportPluginEnabled(schedule database.ReportSchedule) (bool, error) {
	pluginName := reportPluginNameForFrequency(schedule.Frequency)
	if pluginName == "" {
		return false, nil
	}
	return database.IsPluginEnabledForBusiness(schedule.BusinessID, pluginName)
}

func defaultReportDeliveryAlert(db *database.DB) func(context.Context, database.ReportDelivery) error {
	return func(ctx context.Context, delivery database.ReportDelivery) error {
		if db == nil || db.GetDB() == nil {
			return errors.New("operational alert database unavailable")
		}
		_, err := operationalalerts.NewService(db.GetDB()).UpsertAlert(ctx, operationalalerts.UpsertAlertInput{
			BusinessID:   delivery.BusinessID,
			AlertType:    database.OperationalAlertType("report_delivery_failed"),
			ResourceType: database.OperationalAlertResourceType("report_delivery"),
			ResourceID:   delivery.ID,
			Priority:     database.OperationalAlertPriorityHigh,
			Title:        "Scheduled report delivery failed",
			Body:         "A scheduled report exhausted delivery attempts and needs review.",
			Metadata: map[string]any{
				"schedule_id": delivery.ScheduleID,
				"attempts":    delivery.AttemptCount,
				"state":       delivery.State,
			},
		})
		return err
	}
}
