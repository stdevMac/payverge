package reporting

// Status classifies a cost-health report for the operator surface.
// A ratio above 1.0 is never clamped into a "healthy" band — it is a data
// problem, and the status must say so.
type Status string

const (
	StatusOK           Status = "ok"
	StatusInsufficient Status = "insufficient_data" // no sales (ratios undefined)
	StatusImplausible  Status = "implausible"       // a ratio a real restaurant cannot produce
)

// Reason codes for StatusImplausible / StatusInsufficient.
const (
	ReasonNoSales             = "no_sales"
	ReasonLaborExceedsRevenue = "labor_exceeds_revenue"
	ReasonFoodExceedsRevenue  = "food_exceeds_revenue"
	ReasonPrimeExceedsRevenue = "prime_exceeds_revenue"
)

// Distinct labels for the two labor-dollar definitions that must never be
// silently equated when they disagree (finding 8).
const (
	// LabelPayrollPaidCashBasis is GetSummary().PayrollTotal — PAID payroll runs
	// fully recognized as cash when the run is marked paid (not window-prorated).
	LabelPayrollPaidCashBasis = "payroll_paid_cash_basis"
	// LabelLaborAccruedProrated is labor.Report.LaborCost — paid-payroll dollars
	// prorated by the overlap of each run's work period with the reporting window.
	LabelLaborAccruedProrated = "labor_accrued_prorated"
)

// Report is the shared-denominator cost-health view. Every percentage divides
// by RevenueBasis.RecognizedRevenue. RecipeCoveragePct is reported separately
// so sparse recipe mapping is visible rather than baked into food_cost_pct.
//
// PrimeCostPct is a pointer so StatusImplausible / StatusInsufficient can omit
// it from JSON (a ratio above 1.0 must not sit next to a composed prime%).
type Report struct {
	Status            Status   `json:"status"`
	FoodCostPct       float64  `json:"food_cost_pct"`
	LaborCostPct      float64  `json:"labor_cost_pct"`
	PrimeCostPct      *float64 `json:"prime_cost_pct,omitempty"`
	RecipeCoveragePct float64  `json:"recipe_coverage_pct"`
	Reason            string   `json:"reason,omitempty"`
	// LaborBasis is always LabelLaborAccruedProrated for Compute output — the
	// laborCost argument is the prorated/accrued definition.
	LaborBasis string `json:"labor_basis"`
}

// LabeledLaborAmounts holds both labor-dollar definitions under their contract
// names so a consumer cannot silently pick one number for both surfaces.
type LabeledLaborAmounts struct {
	PayrollPaidCashBasis float64 `json:"payroll_paid_cash_basis"`
	LaborAccruedProrated float64 `json:"labor_accrued_prorated"`
}

// Compute builds a cost-health Report from a shared revenue basis plus the
// labor and COGS dollar numerators for the same window.
//
// All percentages divide by basis.RecognizedRevenue. RecipeMappedSales only
// drives RecipeCoveragePct. When any ratio exceeds 1.0 the status is
// StatusImplausible and PrimeCostPct is omitted — never clamped.
func Compute(basis RevenueBasis, laborCost, cogs float64) Report {
	rep := Report{
		LaborBasis: LabelLaborAccruedProrated,
	}

	if basis.RecognizedRevenue <= 0 {
		rep.Status = StatusInsufficient
		rep.Reason = ReasonNoSales
		// Coverage is undefined without recognized sales.
		return rep
	}

	rep.RecipeCoveragePct = basis.RecipeMappedSales / basis.RecognizedRevenue
	rep.FoodCostPct = cogs / basis.RecognizedRevenue
	rep.LaborCostPct = laborCost / basis.RecognizedRevenue
	prime := rep.FoodCostPct + rep.LaborCostPct

	// Never clamp. Surface impossible ratios with an explicit reason; omit prime
	// so a 329% labor figure cannot sit next to a composed "healthy" prime.
	switch {
	case rep.LaborCostPct > 1.0:
		rep.Status = StatusImplausible
		rep.Reason = ReasonLaborExceedsRevenue
		return rep
	case rep.FoodCostPct > 1.0:
		rep.Status = StatusImplausible
		rep.Reason = ReasonFoodExceedsRevenue
		return rep
	case prime > 1.0:
		rep.Status = StatusImplausible
		rep.Reason = ReasonPrimeExceedsRevenue
		return rep
	}

	rep.Status = StatusOK
	rep.PrimeCostPct = &prime
	return rep
}
