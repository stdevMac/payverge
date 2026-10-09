package llm

import (
	"sort"
	"sync"
	"time"
)

type CallInfo struct {
	Feature      string
	Model        string
	ServedModel  string
	InputTokens  int
	OutputTokens int
	Latency      time.Duration
	Err          error
	// BusinessID is set by retry.go from GenerateRequest.BusinessID (0 = unknown/aggregate);
	// enables per-business roll-up (Lane J).
	BusinessID uint
	// EstimatedCostUSD is filled by Lane J's AnnotateCost (0 for unknown models).
	EstimatedCostUSD float64
	// CostPriced reports whether EstimatedCostUSD reflects a real price-table
	// entry for the served model. When false, EstimatedCostUSD is 0 because the
	// model is UNPRICED (e.g. an OpenRouter failover model with no static rate),
	// NOT because it is genuinely free — so failover spend stays visible.
	CostPriced bool
}

type Observer func(CallInfo)

// AnnotateCost fills ci.EstimatedCostUSD from the static price table. cachedInput
// is the subset of ci.InputTokens served from cache (Usage.CachedTokens).
func AnnotateCost(ci *CallInfo, cachedInput int) {
	model := ci.ServedModel
	if model == "" {
		model = ci.Model
	}
	ci.EstimatedCostUSD = EstimateCostUSD(model, ci.InputTokens, cachedInput, ci.OutputTokens)
	ci.CostPriced = ModelPriced(model)
}

// CostLine is one business's aggregated daily cost.
type CostLine struct {
	BusinessID uint
	Calls      int
	TotalUSD   float64
	ByFeature  map[string]float64
}

// DailyCostRollup accumulates per-business cost across calls for process-local
// metrics only. It MUST NOT authorize spend — that is BudgetStore's job. Flush
// remains available for tests; production emits UTC-date metrics from the
// durable store (see BudgetStore.MetricsForUTCDate).
type DailyCostRollup struct {
	mu    sync.Mutex
	byBiz map[uint]*CostLine
}

func NewDailyCostRollup() *DailyCostRollup {
	return &DailyCostRollup{byBiz: map[uint]*CostLine{}}
}

func (r *DailyCostRollup) Add(businessID uint, feature string, costUSD float64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	line := r.byBiz[businessID]
	if line == nil {
		line = &CostLine{BusinessID: businessID, ByFeature: map[string]float64{}}
		r.byBiz[businessID] = line
	}
	line.Calls++
	line.TotalUSD += costUSD
	line.ByFeature[feature] += costUSD
}

// RunningTotalUSD returns the process-local accumulated cost so far (metrics /
// unit-test helper). Production authorization uses BudgetStore.CommittedSpend.
func (r *DailyCostRollup) RunningTotalUSD(businessID uint) float64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if line := r.byBiz[businessID]; line != nil {
		return line.TotalUSD
	}
	return 0
}

// RunningFeatureUSD returns process-local feature spend (metrics / unit-test
// helper). Production authorization uses BudgetStore.CommittedSpend.
func (r *DailyCostRollup) RunningFeatureUSD(businessID uint, feature string) float64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if line := r.byBiz[businessID]; line != nil {
		return line.ByFeature[feature]
	}
	return 0
}

// Flush returns the accumulated lines sorted by BusinessID and resets the
// process-local rollup. Prefer BudgetStore.MetricsForUTCDate for production
// daily metrics (UTC calendar date, durable across replicas/restarts).
func (r *DailyCostRollup) Flush() []CostLine {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]CostLine, 0, len(r.byBiz))
	for _, l := range r.byBiz {
		out = append(out, *l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BusinessID < out[j].BusinessID })
	r.byBiz = map[uint]*CostLine{}
	return out
}
