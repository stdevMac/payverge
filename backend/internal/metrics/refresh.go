package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// RefreshRealm is a closed, low-cardinality refresh endpoint namespace.
type RefreshRealm string

const (
	RefreshRealmOperator RefreshRealm = "operator"
	RefreshRealmCustomer RefreshRealm = "customer"
)

// RefreshOutcome is a closed terminal result for one refresh request.
type RefreshOutcome string

const (
	RefreshOutcomeSuccess    RefreshOutcome = "success"
	RefreshOutcomeRotated    RefreshOutcome = "rotated"
	RefreshOutcomeReuse      RefreshOutcome = "reuse"
	RefreshOutcomeInvalid    RefreshOutcome = "invalid"
	RefreshOutcomeStoreError RefreshOutcome = "store_error"
)

var refreshOutcomes = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "payverge_auth_refresh_outcomes_total",
		Help: "Total refresh requests by credential realm and terminal outcome.",
	},
	[]string{"realm", "outcome"},
)

// RefreshRealms returns the complete realm vocabulary. The returned slice is a
// copy so callers cannot expand or mutate the collector's label contract.
func RefreshRealms() []RefreshRealm {
	return []RefreshRealm{RefreshRealmOperator, RefreshRealmCustomer}
}

// RefreshOutcomeValues returns the complete terminal-outcome vocabulary.
func RefreshOutcomeValues() []RefreshOutcome {
	return []RefreshOutcome{
		RefreshOutcomeSuccess,
		RefreshOutcomeRotated,
		RefreshOutcomeReuse,
		RefreshOutcomeInvalid,
		RefreshOutcomeStoreError,
	}
}

// RecordRefreshOutcome rejects programmer-supplied values outside the closed
// vocabulary before they can create an unbounded Prometheus time series.
func RecordRefreshOutcome(realm RefreshRealm, outcome RefreshOutcome) {
	validateRefreshLabels(realm, outcome)
	refreshOutcomes.WithLabelValues(string(realm), string(outcome)).Inc()
}

// CurrentRefreshOutcome returns a read-only point-in-time counter value for a
// validated realm/outcome pair. It exists for deterministic delta assertions
// without exposing the underlying CounterVec or permitting arbitrary labels.
func CurrentRefreshOutcome(realm RefreshRealm, outcome RefreshOutcome) float64 {
	validateRefreshLabels(realm, outcome)
	counter := refreshOutcomes.WithLabelValues(string(realm), string(outcome))
	metric := &dto.Metric{}
	if err := counter.Write(metric); err != nil {
		panic(fmt.Sprintf("metrics: read refresh outcome: %v", err))
	}
	return metric.GetCounter().GetValue()
}

func validateRefreshLabels(realm RefreshRealm, outcome RefreshOutcome) {
	if !validRefreshRealm(realm) {
		panic(fmt.Sprintf("metrics: invalid refresh realm %q", realm))
	}
	if !validRefreshOutcome(outcome) {
		panic(fmt.Sprintf("metrics: invalid refresh outcome %q", outcome))
	}
}

func validRefreshRealm(realm RefreshRealm) bool {
	switch realm {
	case RefreshRealmOperator, RefreshRealmCustomer:
		return true
	default:
		return false
	}
}

func validRefreshOutcome(outcome RefreshOutcome) bool {
	switch outcome {
	case RefreshOutcomeSuccess,
		RefreshOutcomeRotated,
		RefreshOutcomeReuse,
		RefreshOutcomeInvalid,
		RefreshOutcomeStoreError:
		return true
	default:
		return false
	}
}
