package metrics

import (
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"
)

// TenantAuthorizationMismatches counts canonical business-boundary denials.
// boundary is a closed, code-selected label; it must never contain tenant or
// actor identifiers.
var TenantAuthorizationMismatches = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "payverge_tenant_authorization_mismatches_total",
		Help: "Canonical tenant authorization mismatches by bounded enforcement boundary.",
	},
	[]string{"boundary"},
)

// OwnerlessBusinesses is refreshed from authoritative database state. A value
// above zero means businesses with no owner at all, which need audited
// reconciliation.
var OwnerlessBusinesses = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "payverge_ownerless_businesses",
		Help: "Current businesses with neither a non-whitespace wallet owner nor a user owner.",
	},
)

// PendingEmailRegistrations is the current count of unverified email auth rows,
// including expired reservations until the normal cleanup/re-registration path
// safely releases them.
var PendingEmailRegistrations = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "payverge_pending_email_registrations",
		Help: "Current unverified email registrations awaiting verification or expiry cleanup.",
	},
)

var EmailVerificationLatency = prometheus.NewHistogram(
	prometheus.HistogramOpts{
		Name: "payverge_email_verification_latency_seconds",
		Help: "Elapsed seconds from email registration record creation to successful verification.",
		Buckets: []float64{
			60, 5 * 60, 15 * 60, 30 * 60, 60 * 60,
			3 * 60 * 60, 6 * 60 * 60, 12 * 60 * 60, 24 * 60 * 60,
		},
	},
)

type businessOwnerState struct {
	OwnerAddress string
	UserID       *uint
}

// RefreshLaunchReadinessState refreshes low-cardinality gauges from database
// truth. It computes both values before publishing either, so a failed query
// cannot expose a mixed-generation snapshot.
func RefreshLaunchReadinessState(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("launch readiness metrics database is nil")
	}

	var businesses []businessOwnerState
	if err := db.Table("businesses").Select("owner_address", "user_id").Where("user_id IS NULL").Scan(&businesses).Error; err != nil {
		return fmt.Errorf("count ownerless businesses: %w", err)
	}
	ownerless := 0
	for _, business := range businesses {
		if business.UserID == nil && strings.TrimSpace(business.OwnerAddress) == "" {
			ownerless++
		}
	}

	pending, err := countPendingEmailRegistrations(db)
	if err != nil {
		return err
	}

	OwnerlessBusinesses.Set(float64(ownerless))
	PendingEmailRegistrations.Set(float64(pending))
	return nil
}

// RefreshPendingEmailRegistrationState refreshes the auth gauge after a
// registration or verification transition without rescanning businesses.
func RefreshPendingEmailRegistrationState(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("pending registration metrics database is nil")
	}
	pending, err := countPendingEmailRegistrations(db)
	if err != nil {
		return err
	}
	PendingEmailRegistrations.Set(float64(pending))
	return nil
}

func countPendingEmailRegistrations(db *gorm.DB) (int64, error) {
	var pending int64
	if err := db.Table("user_auths").
		Where("provider = ? AND email_verified = ?", "email", false).
		Count(&pending).Error; err != nil {
		return 0, fmt.Errorf("count pending email registrations: %w", err)
	}
	return pending, nil
}
