package metrics

import "github.com/prometheus/client_golang/prometheus"

// FileMutationOutcomes tracks bounded security and storage outcomes for
// tenant-scoped file mutations. Labels are deliberately low-cardinality and
// never contain business IDs, object keys, emails, or provider error text.
var FileMutationOutcomes = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "payverge_file_mutation_outcomes_total",
		Help: "Tenant-scoped file mutation outcomes by operation and bounded outcome.",
	},
	[]string{"operation", "outcome"},
)
