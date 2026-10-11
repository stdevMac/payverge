package metrics

import (
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"gorm.io/gorm"
)

// AlternativePaymentRequestOutcome is deliberately closed so request or
// tenant identifiers can never become Prometheus labels.
type AlternativePaymentRequestOutcome string

const (
	AlternativePaymentRequestCreated     AlternativePaymentRequestOutcome = "created"
	AlternativePaymentRequestReplayed    AlternativePaymentRequestOutcome = "replayed"
	AlternativePaymentRequestConflict    AlternativePaymentRequestOutcome = "conflict"
	AlternativePaymentRequestRateLimited AlternativePaymentRequestOutcome = "rate_limited"
	AlternativePaymentRequestConfirmed   AlternativePaymentRequestOutcome = "confirmed"
	AlternativePaymentRequestCancelled   AlternativePaymentRequestOutcome = "cancelled"
	AlternativePaymentRequestRejected    AlternativePaymentRequestOutcome = "rejected"
	AlternativePaymentRequestAlertFailed AlternativePaymentRequestOutcome = "alert_failed"
)

var alternativePaymentRequestOutcomeValues = []AlternativePaymentRequestOutcome{
	AlternativePaymentRequestCreated,
	AlternativePaymentRequestReplayed,
	AlternativePaymentRequestConflict,
	AlternativePaymentRequestRateLimited,
	AlternativePaymentRequestConfirmed,
	AlternativePaymentRequestCancelled,
	AlternativePaymentRequestRejected,
	AlternativePaymentRequestAlertFailed,
}

var alternativePaymentRequestOutcomes = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "payverge_alternative_payment_request_outcomes_total",
		Help: "Alternative-payment request lifecycle outcomes using a closed, low-cardinality vocabulary.",
	},
	[]string{"outcome"},
)

var AlternativePaymentRequestOldestPendingSeconds = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "payverge_alternative_payment_request_oldest_pending_seconds",
		Help: "Age in seconds of the oldest alternative-payment request still marked pending.",
	},
)

func AlternativePaymentRequestOutcomeValues() []AlternativePaymentRequestOutcome {
	values := make([]AlternativePaymentRequestOutcome, len(alternativePaymentRequestOutcomeValues))
	copy(values, alternativePaymentRequestOutcomeValues)
	return values
}

func validAlternativePaymentRequestOutcome(want AlternativePaymentRequestOutcome) bool {
	for _, outcome := range alternativePaymentRequestOutcomeValues {
		if want == outcome {
			return true
		}
	}
	return false
}

func RecordAlternativePaymentRequestOutcome(outcome AlternativePaymentRequestOutcome) {
	if !validAlternativePaymentRequestOutcome(outcome) {
		panic(fmt.Sprintf("invalid alternative payment request outcome %q", outcome))
	}
	alternativePaymentRequestOutcomes.WithLabelValues(string(outcome)).Inc()
}

// CurrentAlternativePaymentRequestOutcome is intended for deterministic tests
// and diagnostics; production callers should only record typed outcomes.
func CurrentAlternativePaymentRequestOutcome(outcome AlternativePaymentRequestOutcome) float64 {
	if !validAlternativePaymentRequestOutcome(outcome) {
		panic(fmt.Sprintf("invalid alternative payment request outcome %q", outcome))
	}
	counter := alternativePaymentRequestOutcomes.WithLabelValues(string(outcome))
	metric := &dto.Metric{}
	if err := counter.Write(metric); err != nil {
		panic(fmt.Sprintf("metrics: read alternative payment request outcome: %v", err))
	}
	return metric.GetCounter().GetValue()
}

// RefreshAlternativePaymentRequestState publishes database truth for pending
// age. Rows that remain pending beyond their normal TTL are intentionally
// included: the gauge is also the detector for a stuck expiry path.
func RefreshAlternativePaymentRequestState(db *gorm.DB, now time.Time) error {
	if db == nil {
		return fmt.Errorf("alternative payment request metrics database is nil")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	var result struct {
		CreatedAt time.Time `gorm:"column:created_at"`
	}
	err := db.Table("alternative_payments").
		Select("created_at").
		Where("status = ?", "pending").
		Order("created_at ASC").
		Take(&result).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("load oldest pending alternative payment request: %w", err)
	}

	age := 0.0
	if err == nil {
		age = now.Sub(result.CreatedAt.UTC()).Seconds()
		if age < 0 {
			age = 0
		}
	}
	AlternativePaymentRequestOldestPendingSeconds.Set(age)
	return nil
}
