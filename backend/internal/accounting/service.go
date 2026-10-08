package accounting

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrDuplicatePayrollPeriod is returned when a non-void payroll run already
// covers any day of the requested business + period. It is a sentinel so the race-proof
// in-transaction re-check can be mapped back to the same client-facing message
// as the fast-path pre-check.
var ErrDuplicatePayrollPeriod = errors.New("a payroll run already exists for an overlapping period")

// ErrInvalidDateRange is a validation sentinel so handlers can map bad
// start/end input to 400 while masking genuine DB failures as 500.
var ErrInvalidDateRange = errors.New("invalid accounting date range")

// ErrRangeTooLarge is returned when a timeseries window would produce more
// than maxTimeseriesDays day buckets (handler maps to 400).
var ErrRangeTooLarge = errors.New("accounting timeseries range exceeds 400 days")

// maxTimeseriesDays caps Overview chart series length so a hand-edited URL
// cannot request unbounded day payloads.
const maxTimeseriesDays = 400

var manualIncomeCategories = map[string]struct{}{
	"off_platform_sale": {},
	"catering":          {},
	"event":             {},
	"service":           {},
	"adjustment":        {},
	"other":             {},
}

var expenseCategories = map[string]struct{}{
	"rent":        {},
	"utilities":   {},
	"supplies":    {},
	"inventory":   {},
	"marketing":   {},
	"software":    {},
	"maintenance": {},
	"logistics":   {},
	"tax":         {},
	"other":       {},
}

var currencyCodePattern = regexp.MustCompile(`^[A-Z]{3}$`)
var nonStandardCurrencyCodes = map[string]struct{}{
	"USDC":  {},
	"USDT":  {},
	"BTC":   {},
	"ETH":   {},
	"MATIC": {},
	"BNB":   {},
}

type Service struct {
	db *database.DB
}

type CategoryTotal struct {
	Category string  `json:"category"`
	Total    float64 `json:"total"`
}

type PayrollSummary struct {
	PaidRuns       int     `json:"paid_runs"`
	TotalGross     float64 `json:"total_gross"`
	TotalBonus     float64 `json:"total_bonus"`
	TotalDeduction float64 `json:"total_deduction"`
	TotalNet       float64 `json:"total_net"`
}

type Summary struct {
	StartDate         time.Time `json:"start_date"`
	EndDate           time.Time `json:"end_date"`
	Currency          string    `json:"currency"`
	AutoIncomeTotal   float64   `json:"auto_income_total"`
	ManualIncomeTotal float64   `json:"manual_income_total"`
	ExpenseTotal      float64   `json:"expense_total"`
	PayrollTotal      float64   `json:"payroll_total"`
	// Net P/L is intentionally NOT on Summary — a second derived net without
	// COGS drifted from ComposeProfitLoss / GetProfitLoss. Component totals
	// stay here; Net lives only on the P&L statement (includes estimated COGS).
	BilledTotal          float64         `json:"billed_total"`
	CollectedTotal       float64         `json:"collected_total"`
	CollectionGap        float64         `json:"collection_gap"`
	IncomeBreakdown      []CategoryTotal `json:"income_breakdown"`
	ExpenseBreakdown     []CategoryTotal `json:"expense_breakdown"`
	PayrollSummary       PayrollSummary  `json:"payroll_summary"`
	SkippedManualEntries int             `json:"skipped_manual_entries"`
	Warnings             []FXWarning     `json:"warnings,omitempty"`
}

// FXWarning is a structured conversion warning for operator localization.
// The FE maps Code via businessDashboard.accountingDashboard.warnings.fxWarnings.*
// and interpolates Params (count, currency, details, scope, entry_type).
type FXWarning struct {
	Code   string            `json:"code"`
	Params map[string]string `json:"params,omitempty"`
}

// TimeseriesPoint is one day-bucket of manual income, expense, and paid payroll
// totals in reporting-currency dollars (YYYY-MM-DD in the business timezone).
type TimeseriesPoint struct {
	Date    string  `json:"date"`
	Income  float64 `json:"income"`
	Expense float64 `json:"expense"`
	Payroll float64 `json:"payroll"`
}

// Timeseries is a contiguous (zero-filled) day series for Overview charts.
// Start/End are inclusive business-local day labels for the first/last bucket.
type Timeseries struct {
	Start    string            `json:"start"`
	End      string            `json:"end"`
	Bucket   string            `json:"bucket"` // "day"
	Currency string            `json:"currency"`
	Series   []TimeseriesPoint `json:"series"`
}

// dayCurrencyTotal is a SQL (day, currency) aggregate in int64 cents.
type dayCurrencyTotal struct {
	Day      string `gorm:"column:day"`
	Currency string `gorm:"column:currency"`
	Total    int64  `gorm:"column:total"`
}

// dayCurrencyCategoryTotal is a SQL (day, currency, category) aggregate in
// int64 cents for GetSummary pre-aggregation (O(days×currencies×categories)).
type dayCurrencyCategoryTotal struct {
	Day      string `gorm:"column:day"`
	Currency string `gorm:"column:currency"`
	Category string `gorm:"column:category"`
	Total    int64  `gorm:"column:total"`
}

// dayCurrencyPayrollAgg is a SQL (day, currency) aggregate of paid payroll
// runs: summed totals in int64 cents plus run count for skip accounting.
type dayCurrencyPayrollAgg struct {
	Day            string `gorm:"column:day"`
	Currency       string `gorm:"column:currency"`
	GrossTotal     int64  `gorm:"column:gross_total"`
	BonusTotal     int64  `gorm:"column:bonus_total"`
	DeductionTotal int64  `gorm:"column:deduction_total"`
	NetTotal       int64  `gorm:"column:net_total"`
	RunCount       int    `gorm:"column:run_count"`
}

// dayCurrencyKey identifies a failed FX conversion bucket for the slow-path
// skip sample (entry IDs for warning details).
type dayCurrencyKey struct {
	Day      string
	Currency string
}

type manualEntriesSummary struct {
	Total        float64
	Breakdown    []CategoryTotal
	SkippedCount int
	Warnings     []FXWarning
}

type historicalRateResolver struct {
	timelines map[string][]database.ExchangeRate
}

type ListEntriesInput struct {
	BusinessID uint
	EntryType  *database.AccountingEntryType
	StartDate  *time.Time
	EndDate    *time.Time
	Page       int
	PageSize   int
	// Status: "", "all" → no filter; "active" → voided_at IS NULL; "voided" → voided_at IS NOT NULL
	Status string
	// Category: exact match when non-empty (normalized to lowercase).
	Category string
	// Query: case-insensitive substring match on description OR reference.
	Query string
	// Sort: "" → occurred_at DESC, created_at DESC; "occurred_at_asc" | "amount_desc" | "amount_asc"
	Sort string
}

type EntriesPage struct {
	Entries    []database.ManualLedgerEntry `json:"entries"`
	Total      int64                        `json:"total"`
	Page       int                          `json:"page"`
	PageSize   int                          `json:"page_size"`
	TotalPages int                          `json:"total_pages"`
}

// ListPayrollRunsParams is the filter/pagination input for ListPayrollRunsPage.
type ListPayrollRunsParams struct {
	BusinessID uint
	Status     string // optional; mapped to PayrollRunStatus when non-empty
	Start      *time.Time
	End        *time.Time
	Page       int // 1-based
	PageSize   int // default 20, max 100
}

// PayrollRunRow is a payroll run list row with aggregate payee_count (no LineItems preload).
type PayrollRunRow struct {
	database.PayrollRun
	PayeeCount int `json:"payee_count"`
}

// MarshalJSON must exist on the row itself: the embedded PayrollRun's custom
// money marshaler is otherwise promoted to the row and drops PayeeCount from
// the wire entirely. It also strips the never-loaded business relation, which
// json's omitempty cannot elide for a non-pointer struct.
func (r PayrollRunRow) MarshalJSON() ([]byte, error) {
	base, err := json.Marshal(r.PayrollRun)
	if err != nil {
		return nil, err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(base, &payload); err != nil {
		return nil, err
	}
	payload["payee_count"] = r.PayeeCount
	delete(payload, "business")
	return json.Marshal(payload)
}

// PayrollRunsPage is the paginated envelope for payroll runs.
type PayrollRunsPage struct {
	Runs       []PayrollRunRow `json:"runs"`
	Total      int64           `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalPages int             `json:"total_pages"`
}

type CreateManualLedgerEntryInput struct {
	BusinessID       uint
	EntryType        database.AccountingEntryType
	Category         string
	Amount           float64
	Currency         string
	OccurredAt       time.Time
	Description      string
	Notes            string
	Reference        string
	CreatedByUserID  *uint
	CreatedByStaffID *uint
}

type CreatePayrollLineItemInput struct {
	PayeeType       database.PayrollPayeeType `json:"payee_type"`
	StaffID         *uint                     `json:"staff_id,omitempty"`
	PayeeName       string                    `json:"payee_name,omitempty"`
	GrossAmount     float64                   `json:"gross_amount"`
	BonusAmount     float64                   `json:"bonus_amount"`
	DeductionAmount float64                   `json:"deduction_amount"`
	Notes           string                    `json:"notes"`
}

type CreatePayrollRunInput struct {
	BusinessID       uint                         `json:"business_id"`
	PeriodStart      time.Time                    `json:"period_start"`
	PeriodEnd        time.Time                    `json:"period_end"`
	Notes            string                       `json:"notes"`
	LineItems        []CreatePayrollLineItemInput `json:"line_items"`
	CreatedByUserID  *uint                        `json:"created_by_user_id,omitempty"`
	CreatedByStaffID *uint                        `json:"created_by_staff_id,omitempty"`
}

func NewService(db *database.DB) *Service {
	return &Service{db: db}
}

func normalizeRange(startDate, endDate time.Time) (time.Time, time.Time, error) {
	if startDate.IsZero() || endDate.IsZero() {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: start and end dates are required", ErrInvalidDateRange)
	}

	start := startDate.UTC()
	end := endDate.UTC()
	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: end date must be on or after start date", ErrInvalidDateRange)
	}

	return start, end, nil
}

func normalizeEntryType(entryType database.AccountingEntryType) database.AccountingEntryType {
	return database.AccountingEntryType(strings.ToLower(strings.TrimSpace(string(entryType))))
}

func normalizeCategory(category string) string {
	return strings.ToLower(strings.TrimSpace(category))
}

func normalizeCurrencyCode(code string) string {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	if currencyCodePattern.MatchString(normalized) {
		return normalized
	}
	if _, ok := nonStandardCurrencyCodes[normalized]; ok {
		return normalized
	}
	return ""
}

func (s *Service) validateManualEntry(input CreateManualLedgerEntryInput) error {
	if input.BusinessID == 0 {
		return fmt.Errorf("business_id is required")
	}
	if input.Amount <= 0 {
		return fmt.Errorf("amount must be greater than zero")
	}
	if input.OccurredAt.IsZero() {
		return fmt.Errorf("occurred_at is required")
	}
	if strings.TrimSpace(input.Description) == "" {
		return fmt.Errorf("description is required")
	}

	entryType := normalizeEntryType(input.EntryType)
	category := normalizeCategory(input.Category)

	switch entryType {
	case database.AccountingEntryTypeIncome:
		if _, ok := manualIncomeCategories[category]; !ok {
			return fmt.Errorf("invalid income category")
		}
	case database.AccountingEntryTypeExpense:
		if _, ok := expenseCategories[category]; !ok {
			return fmt.Errorf("invalid expense category")
		}
	default:
		return fmt.Errorf("invalid entry_type")
	}

	return nil
}

func (s *Service) resolveCurrency(businessID uint, requested string) string {
	if normalized := normalizeCurrencyCode(requested); normalized != "" {
		return normalized
	}

	business, err := s.db.GetBusinessByID(businessID)
	if err == nil {
		if normalized := normalizeCurrencyCode(business.DefaultCurrency); normalized != "" {
			return normalized
		}
	}

	return "USD"
}

func (s *Service) getExchangeRateAtOrBefore(fromCurrency, toCurrency string, asOf time.Time) (*database.ExchangeRate, error) {
	if asOf.IsZero() {
		return s.db.CurrencyService.GetExchangeRate(fromCurrency, toCurrency)
	}
	return s.db.CurrencyService.GetExchangeRateAtOrBefore(fromCurrency, toCurrency, asOf.UTC())
}

func (s *Service) convertAmountToCurrencyAt(amount float64, fromCurrency, toCurrency string, asOf time.Time) (float64, error) {
	from := normalizeCurrencyCode(fromCurrency)
	to := normalizeCurrencyCode(toCurrency)
	if from == "" {
		return 0, fmt.Errorf("invalid source currency %q", fromCurrency)
	}
	if to == "" {
		return 0, fmt.Errorf("invalid target currency %q", toCurrency)
	}
	if from == to {
		return amount, nil
	}

	if directRate, err := s.getExchangeRateAtOrBefore(from, to, asOf); err == nil {
		if directRate.Rate <= 0 {
			return 0, fmt.Errorf("invalid direct exchange rate for %s to %s", from, to)
		}
		return amount * directRate.Rate, nil
	}

	if reverseRate, err := s.getExchangeRateAtOrBefore(to, from, asOf); err == nil {
		if reverseRate.Rate <= 0 {
			return 0, fmt.Errorf("invalid reverse exchange rate for %s to %s", from, to)
		}
		return amount / reverseRate.Rate, nil
	}

	if from == "USDC" {
		rate, err := s.getExchangeRateAtOrBefore("USDC", to, asOf)
		if err != nil {
			return 0, fmt.Errorf("missing exchange rate for %s to %s", from, to)
		}
		if rate.Rate <= 0 {
			return 0, fmt.Errorf("invalid exchange rate for %s to %s", from, to)
		}
		return amount * rate.Rate, nil
	}

	if to == "USDC" {
		rate, err := s.getExchangeRateAtOrBefore("USDC", from, asOf)
		if err != nil {
			return 0, fmt.Errorf("missing exchange rate for %s to %s", from, to)
		}
		if rate.Rate <= 0 {
			return 0, fmt.Errorf("invalid exchange rate for %s to %s", from, to)
		}
		return amount / rate.Rate, nil
	}

	fromRate, err := s.getExchangeRateAtOrBefore("USDC", from, asOf)
	if err != nil {
		return 0, fmt.Errorf("missing exchange rate for USDC to %s", from)
	}
	toRate, err := s.getExchangeRateAtOrBefore("USDC", to, asOf)
	if err != nil {
		return 0, fmt.Errorf("missing exchange rate for USDC to %s", to)
	}
	if fromRate.Rate <= 0 || toRate.Rate <= 0 {
		return 0, fmt.Errorf("invalid cross exchange rate for %s to %s", from, to)
	}

	return amount * (toRate.Rate / fromRate.Rate), nil
}

func rateTimelineKey(fromCurrency, toCurrency string) string {
	return fromCurrency + "->" + toCurrency
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// newHistoricalRateResolverForCurrencies builds a rate resolver covering every
// supplied source currency (normalized; empties and the reporting currency are
// dropped). Used by both manual-ledger and payroll summaries.
//
// Only rates that can answer a lookup in [windowStart, cutoff] are loaded: the
// rows fetched inside the window plus, per pair, the latest row at or before
// the window start (the rate in force when the window opens). The pair's older
// history is never read.
func (s *Service) newHistoricalRateResolverForCurrencies(rawCurrencies []string, reportingCurrency string, windowStart, cutoff time.Time) (*historicalRateResolver, error) {
	currencies := make([]string, 0, len(rawCurrencies))
	usdcTargets := []string{reportingCurrency}

	for _, raw := range rawCurrencies {
		normalizedCurrency := normalizeCurrencyCode(raw)
		if normalizedCurrency == "" || normalizedCurrency == reportingCurrency {
			continue
		}
		currencies = append(currencies, normalizedCurrency)
		usdcTargets = append(usdcTargets, normalizedCurrency)
	}

	currencies = uniqueSortedStrings(currencies)
	usdcTargets = uniqueSortedStrings(usdcTargets)

	resolver := &historicalRateResolver{
		timelines: map[string][]database.ExchangeRate{},
	}
	if len(currencies) == 0 {
		return resolver, nil
	}

	// Day buckets resolve at local day start; a day of slack keeps any
	// timezone-shifted first-day lookup inside the loaded range.
	lower := windowStart.UTC().Add(-24 * time.Hour)
	upper := cutoff.UTC()
	if lower.After(upper) {
		lower = upper
	}
	pairFilter := "((from_currency IN ? AND to_currency = ?) OR (from_currency = ? AND to_currency IN ?) OR (from_currency = ? AND to_currency IN ?))"
	pairArgs := []any{currencies, reportingCurrency, reportingCurrency, currencies, "USDC", usdcTargets}
	gdb := s.db.GetGorm()

	// One read: window rows, plus each pair's anchor via a grouped MAX
	// (served by idx_exchange_rates_pair_fetched).
	// Raw expression, not a *gorm.DB subquery: GORM renders those through a
	// dry-run of the query callbacks, which would count as a second read.
	anchorKeys := gorm.Expr(
		"SELECT from_currency, to_currency, MAX(fetched_at) FROM exchange_rates WHERE fetched_at <= ? AND "+pairFilter+" GROUP BY from_currency, to_currency",
		append([]any{lower}, pairArgs...)...,
	)
	var rates []database.ExchangeRate
	if err := gdb.Model(&database.ExchangeRate{}).
		Where(pairFilter, pairArgs...).
		Where("fetched_at <= ?", upper).
		Where("fetched_at > ? OR (from_currency, to_currency, fetched_at) IN (?)", lower, anchorKeys).
		Order("from_currency ASC, to_currency ASC, fetched_at ASC").
		Find(&rates).Error; err != nil {
		return nil, fmt.Errorf("failed to load historical exchange rates: %w", err)
	}

	for _, rate := range rates {
		key := rateTimelineKey(rate.FromCurrency, rate.ToCurrency)
		resolver.timelines[key] = append(resolver.timelines[key], rate)
	}

	return resolver, nil
}

func (r *historicalRateResolver) rateAtOrBefore(fromCurrency, toCurrency string, asOf time.Time) (database.ExchangeRate, bool) {
	timeline, ok := r.timelines[rateTimelineKey(fromCurrency, toCurrency)]
	if !ok || len(timeline) == 0 {
		return database.ExchangeRate{}, false
	}

	asOfUTC := asOf.UTC()
	index := sort.Search(len(timeline), func(i int) bool {
		return timeline[i].FetchedAt.After(asOfUTC)
	})
	if index == 0 {
		return database.ExchangeRate{}, false
	}

	return timeline[index-1], true
}

func (r *historicalRateResolver) convertAmountAt(amount float64, fromCurrency, toCurrency string, asOf time.Time) (float64, error) {
	from := normalizeCurrencyCode(fromCurrency)
	to := normalizeCurrencyCode(toCurrency)
	if from == "" {
		return 0, fmt.Errorf("invalid source currency %q", fromCurrency)
	}
	if to == "" {
		return 0, fmt.Errorf("invalid target currency %q", toCurrency)
	}
	if from == to {
		return amount, nil
	}

	if directRate, ok := r.rateAtOrBefore(from, to, asOf); ok {
		if directRate.Rate <= 0 {
			return 0, fmt.Errorf("invalid direct exchange rate for %s to %s", from, to)
		}
		return amount * directRate.Rate, nil
	}

	if reverseRate, ok := r.rateAtOrBefore(to, from, asOf); ok {
		if reverseRate.Rate <= 0 {
			return 0, fmt.Errorf("invalid reverse exchange rate for %s to %s", from, to)
		}
		return amount / reverseRate.Rate, nil
	}

	if from == "USDC" {
		rate, ok := r.rateAtOrBefore("USDC", to, asOf)
		if !ok {
			return 0, fmt.Errorf("missing exchange rate for %s to %s", from, to)
		}
		if rate.Rate <= 0 {
			return 0, fmt.Errorf("invalid exchange rate for %s to %s", from, to)
		}
		return amount * rate.Rate, nil
	}

	if to == "USDC" {
		rate, ok := r.rateAtOrBefore("USDC", from, asOf)
		if !ok {
			return 0, fmt.Errorf("missing exchange rate for %s to %s", from, to)
		}
		if rate.Rate <= 0 {
			return 0, fmt.Errorf("invalid exchange rate for %s to %s", from, to)
		}
		return amount / rate.Rate, nil
	}

	fromRate, ok := r.rateAtOrBefore("USDC", from, asOf)
	if !ok {
		return 0, fmt.Errorf("missing exchange rate for USDC to %s", from)
	}
	toRate, ok := r.rateAtOrBefore("USDC", to, asOf)
	if !ok {
		return 0, fmt.Errorf("missing exchange rate for USDC to %s", to)
	}
	if fromRate.Rate <= 0 || toRate.Rate <= 0 {
		return 0, fmt.Errorf("invalid cross exchange rate for %s to %s", from, to)
	}

	return amount * (toRate.Rate / fromRate.Rate), nil
}

func categoryTotalsFromMap(totals map[string]float64) []CategoryTotal {
	categories := make([]string, 0, len(totals))
	for category := range totals {
		categories = append(categories, category)
	}
	sort.Strings(categories)

	result := make([]CategoryTotal, 0, len(categories))
	for _, category := range categories {
		result = append(result, CategoryTotal{
			Category: category,
			Total:    totals[category],
		})
	}

	return result
}

func (s *Service) CreateManualEntry(input CreateManualLedgerEntryInput) (*database.ManualLedgerEntry, error) {
	if err := s.validateManualEntry(input); err != nil {
		return nil, err
	}

	occurredAt := input.OccurredAt.UTC()
	entryCurrency := s.resolveCurrency(input.BusinessID, input.Currency)
	reportingCurrency := s.resolveCurrency(input.BusinessID, "")
	if _, err := s.convertAmountToCurrencyAt(input.Amount, entryCurrency, reportingCurrency, occurredAt); err != nil {
		return nil, fmt.Errorf(
			"manual entry currency %s cannot be converted to reporting currency %s at occurred_at: %w",
			entryCurrency,
			reportingCurrency,
			err,
		)
	}

	entry := &database.ManualLedgerEntry{
		BusinessID:       input.BusinessID,
		EntryType:        normalizeEntryType(input.EntryType),
		Category:         normalizeCategory(input.Category),
		Amount:           int64(math.Round(input.Amount * 100)),
		Currency:         entryCurrency,
		OccurredAt:       occurredAt,
		Description:      strings.TrimSpace(input.Description),
		Notes:            strings.TrimSpace(input.Notes),
		Reference:        strings.TrimSpace(input.Reference),
		CreatedByUserID:  input.CreatedByUserID,
		CreatedByStaffID: input.CreatedByStaffID,
	}

	if err := s.db.GetGorm().Create(entry).Error; err != nil {
		return nil, fmt.Errorf("failed to create manual entry: %w", err)
	}

	return entry, nil
}

func (s *Service) VoidManualEntry(businessID, entryID uint, voidedByUserID, voidedByStaffID *uint) error {
	var entry database.ManualLedgerEntry
	if err := s.db.GetGorm().
		Where("id = ? AND business_id = ?", entryID, businessID).
		First(&entry).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("manual entry not found")
		}
		return fmt.Errorf("failed to load manual entry: %w", err)
	}

	if entry.VoidedAt != nil {
		return fmt.Errorf("manual entry already voided")
	}

	now := time.Now().UTC()
	updates := map[string]any{
		"voided_at":          &now,
		"voided_by_user_id":  voidedByUserID,
		"voided_by_staff_id": voidedByStaffID,
		"updated_at":         now,
	}

	// CAS on voided_at IS NULL: two concurrent voids both pass the unlocked
	// pre-read above, so without this guard the second UPDATE would silently
	// overwrite the first's void attribution (wrong actor recorded). RowsAffected
	// == 0 means another writer already voided it — treat as already-voided.
	res := s.db.GetGorm().
		Model(&database.ManualLedgerEntry{}).
		Where("id = ? AND business_id = ? AND voided_at IS NULL", entryID, businessID).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("manual entry already voided")
	}
	return nil
}

func listEntriesOrderClause(sort string) string {
	switch strings.TrimSpace(sort) {
	case "occurred_at_asc":
		return "occurred_at ASC, created_at ASC"
	case "amount_desc":
		return "amount DESC, occurred_at DESC, created_at DESC"
	case "amount_asc":
		return "amount ASC, occurred_at DESC, created_at DESC"
	default:
		// Empty / unknown: preserve legacy order (handler validates sort before call).
		return "occurred_at DESC, created_at DESC"
	}
}

// attachListEntryActors batch-loads CreatedByUser / CreatedByStaff / VoidedByStaff
// for a page of entries with at most one users query and one staff query.
// selectCols must be the slim projection "id, name, email" so MarshalJSON stays honest.
func attachListEntryActors(db *gorm.DB, entries []database.ManualLedgerEntry, selectCols string) error {
	if len(entries) == 0 {
		return nil
	}
	userIDSet := make(map[uint]struct{})
	staffIDSet := make(map[uint]struct{})
	for _, e := range entries {
		if e.CreatedByUserID != nil {
			userIDSet[*e.CreatedByUserID] = struct{}{}
		}
		if e.CreatedByStaffID != nil {
			staffIDSet[*e.CreatedByStaffID] = struct{}{}
		}
		if e.VoidedByStaffID != nil {
			staffIDSet[*e.VoidedByStaffID] = struct{}{}
		}
	}

	usersByID := make(map[uint]*database.User, len(userIDSet))
	if len(userIDSet) > 0 {
		ids := make([]uint, 0, len(userIDSet))
		for id := range userIDSet {
			ids = append(ids, id)
		}
		var users []database.User
		if err := db.Select(selectCols).Where("id IN ?", ids).Find(&users).Error; err != nil {
			return fmt.Errorf("failed to load entry created_by users: %w", err)
		}
		for i := range users {
			u := users[i]
			usersByID[u.ID] = &u
		}
	}

	staffByID := make(map[uint]*database.Staff, len(staffIDSet))
	if len(staffIDSet) > 0 {
		ids := make([]uint, 0, len(staffIDSet))
		for id := range staffIDSet {
			ids = append(ids, id)
		}
		var staff []database.Staff
		if err := db.Select(selectCols).Where("id IN ?", ids).Find(&staff).Error; err != nil {
			return fmt.Errorf("failed to load entry actor staff: %w", err)
		}
		for i := range staff {
			s := staff[i]
			staffByID[s.ID] = &s
		}
	}

	for i := range entries {
		if entries[i].CreatedByUserID != nil {
			if u := usersByID[*entries[i].CreatedByUserID]; u != nil {
				// Copy so each entry owns a stable pointer (map values are reused).
				cp := *u
				entries[i].CreatedByUser = &cp
			}
		}
		if entries[i].CreatedByStaffID != nil {
			if s := staffByID[*entries[i].CreatedByStaffID]; s != nil {
				cp := *s
				entries[i].CreatedByStaff = &cp
			}
		}
		if entries[i].VoidedByStaffID != nil {
			if s := staffByID[*entries[i].VoidedByStaffID]; s != nil {
				cp := *s
				entries[i].VoidedByStaff = &cp
			}
		}
	}
	return nil
}

func (s *Service) ListEntries(input ListEntriesInput) (*EntriesPage, error) {
	page := input.Page
	if page < 1 {
		page = 1
	}
	pageSize := input.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	query := s.db.GetGorm().
		Model(&database.ManualLedgerEntry{}).
		Where("business_id = ?", input.BusinessID)

	if input.EntryType != nil {
		query = query.Where("entry_type = ?", normalizeEntryType(*input.EntryType))
	}
	if input.StartDate != nil {
		query = query.Where("occurred_at >= ?", input.StartDate.UTC())
	}
	if input.EndDate != nil {
		query = query.Where("occurred_at < ?", input.EndDate.UTC())
	}

	switch strings.ToLower(strings.TrimSpace(input.Status)) {
	case "active":
		query = query.Where("voided_at IS NULL")
	case "voided":
		query = query.Where("voided_at IS NOT NULL")
		// "", "all", or anything else: no status filter (handler validates allowed values).
	}

	if category := normalizeCategory(input.Category); category != "" {
		query = query.Where("category = ?", category)
	}

	if q := strings.TrimSpace(input.Query); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		query = query.Where(
			"(LOWER(description) LIKE ? OR LOWER(COALESCE(reference, '')) LIKE ?)",
			like, like,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count entries: %w", err)
	}

	// L6-15 + decision-14: page query without GORM association Preload, then
	// batch-hydrate actors with at most one users SELECT and one staff SELECT
	// (CreatedByStaff + VoidedByStaff share the staff IN list). Columns are
	// exactly id,name,email so Staff/User MarshalJSON emit the slim shape.
	const entryActorSelect = "id, name, email"

	var entries []database.ManualLedgerEntry
	if err := query.
		Order(listEntriesOrderClause(input.Sort)).
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("failed to list entries: %w", err)
	}

	if len(entries) > 0 {
		if err := attachListEntryActors(s.db.GetGorm(), entries, entryActorSelect); err != nil {
			return nil, err
		}
		// Attachment counts in one grouped query (no N+1, no SELECT *).
		ids := make([]uint, len(entries))
		for i, e := range entries {
			ids[i] = e.ID
		}
		type countRow struct {
			EntryID uint
			Cnt     int
		}
		var counts []countRow
		if err := s.db.GetGorm().
			Model(&database.LedgerEntryAttachment{}).
			Select("entry_id, COUNT(*) AS cnt").
			Where("entry_id IN ?", ids).
			Group("entry_id").
			Scan(&counts).Error; err != nil {
			return nil, fmt.Errorf("failed to count entry attachments: %w", err)
		}
		byID := make(map[uint]int, len(counts))
		for _, c := range counts {
			byID[c.EntryID] = c.Cnt
		}
		for i := range entries {
			entries[i].AttachmentCount = byID[entries[i].ID]
		}
	}

	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return &EntriesPage{
		Entries:    entries,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// overlappingPayrollRuns scopes to the business's non-void payroll runs whose
// [period_start, period_end] range overlaps [start, end]. Payroll periods are
// inclusive calendar days (the form sends YYYY-MM-DD for both ends), so a run
// ending on the 15th and one starting on the 15th share a day and overlap;
// a run starting on the 16th is adjacent and allowed.
func overlappingPayrollRuns(db *gorm.DB, businessID uint, start, end time.Time) *gorm.DB {
	return db.Model(&database.PayrollRun{}).
		Where("business_id = ? AND status != ? AND period_start <= ? AND period_end >= ?",
			businessID, database.PayrollRunStatusVoid, end, start)
}

func (s *Service) CreatePayrollRun(input CreatePayrollRunInput) (*database.PayrollRun, error) {
	start, end, err := normalizeRange(input.PeriodStart, input.PeriodEnd)
	if err != nil {
		return nil, err
	}
	if input.BusinessID == 0 {
		return nil, fmt.Errorf("business_id is required")
	}
	if len(input.LineItems) == 0 {
		return nil, fmt.Errorf("at least one payroll line item is required")
	}

	// Reject a period that overlaps any non-void run (a retried/double-submitted
	// request, an exact duplicate, or a partially overlapping range) — two paid
	// runs covering the same days would double-count payroll in P&L, and runs are
	// otherwise hard to reverse. Adjacent periods (the next starts the day after
	// one ends) do not overlap. A voided prior run does not block a corrected
	// re-run for the same period. This pre-check is a
	// cheap fast-path; the authoritative, race-proof check runs inside the create
	// transaction under a business-row lock (below).
	var duplicates int64
	if err := overlappingPayrollRuns(s.db.GetGorm(), input.BusinessID, start, end).
		Count(&duplicates).Error; err != nil {
		return nil, fmt.Errorf("failed to check for duplicate payroll run: %w", err)
	}
	if duplicates > 0 {
		return nil, ErrDuplicatePayrollPeriod
	}

	run := &database.PayrollRun{
		BusinessID:       input.BusinessID,
		PeriodStart:      start,
		PeriodEnd:        end,
		Status:           database.PayrollRunStatusDraft,
		Currency:         s.resolveCurrency(input.BusinessID, ""),
		Notes:            strings.TrimSpace(input.Notes),
		CreatedByUserID:  input.CreatedByUserID,
		CreatedByStaffID: input.CreatedByStaffID,
	}

	lineItems := make([]database.PayrollLineItem, 0, len(input.LineItems))

	// Pre-pass: collect unique staff IDs that will need validation. No errors
	// are raised here — all errors (including nil/zero staff_id) fire in the
	// main loop at the correct index, preserving first-failing-item order.
	staffByID := make(map[uint]string) // id → name
	var staffIDsToLoad []uint
	seen := make(map[uint]struct{})
	for _, item := range input.LineItems {
		if database.PayrollPayeeType(strings.ToLower(strings.TrimSpace(string(item.PayeeType)))) == database.PayrollPayeeTypeStaff &&
			item.StaffID != nil && *item.StaffID != 0 {
			if _, dup := seen[*item.StaffID]; !dup {
				staffIDsToLoad = append(staffIDsToLoad, *item.StaffID)
				seen[*item.StaffID] = struct{}{}
			}
		}
	}
	if len(staffIDsToLoad) > 0 {
		var staffRows []database.Staff
		if err := s.db.GetGorm().
			Select("id", "name").
			Where("id IN ? AND business_id = ?", staffIDsToLoad, input.BusinessID).
			Find(&staffRows).Error; err == nil {
			for _, sr := range staffRows {
				staffByID[sr.ID] = sr.Name
			}
		}
	}

	for idx, item := range input.LineItems {
		payeeType := database.PayrollPayeeType(strings.ToLower(strings.TrimSpace(string(item.PayeeType))))
		if payeeType != database.PayrollPayeeTypeStaff && payeeType != database.PayrollPayeeTypeContractor {
			return nil, fmt.Errorf("line_items[%d].payee_type is invalid", idx)
		}
		if item.GrossAmount <= 0 {
			return nil, fmt.Errorf("line_items[%d].gross_amount must be greater than zero", idx)
		}
		if item.BonusAmount < 0 || item.DeductionAmount < 0 {
			return nil, fmt.Errorf("line_items[%d] amounts cannot be negative", idx)
		}

		lineItem := database.PayrollLineItem{
			BusinessID:      input.BusinessID,
			PayeeType:       payeeType,
			GrossAmount:     int64(math.Round(item.GrossAmount * 100)),
			BonusAmount:     int64(math.Round(item.BonusAmount * 100)),
			DeductionAmount: int64(math.Round(item.DeductionAmount * 100)),
			Notes:           strings.TrimSpace(item.Notes),
		}

		switch payeeType {
		case database.PayrollPayeeTypeStaff:
			if item.StaffID == nil || *item.StaffID == 0 {
				return nil, fmt.Errorf("line_items[%d].staff_id is required for staff payees", idx)
			}
			name, ok := staffByID[*item.StaffID]
			if !ok {
				return nil, fmt.Errorf("line_items[%d].staff_id is invalid", idx)
			}
			lineItem.StaffID = item.StaffID
			lineItem.PayeeName = name

		case database.PayrollPayeeTypeContractor:
			if item.StaffID != nil {
				return nil, fmt.Errorf("line_items[%d].staff_id is not allowed for contractor payees", idx)
			}
			if strings.TrimSpace(item.PayeeName) == "" {
				return nil, fmt.Errorf("line_items[%d].payee_name is required for contractor payees", idx)
			}
			lineItem.PayeeName = strings.TrimSpace(item.PayeeName)
		}

		lineItem.NetAmount = lineItem.GrossAmount + lineItem.BonusAmount - lineItem.DeductionAmount
		if lineItem.NetAmount < 0 {
			return nil, fmt.Errorf("line_items[%d].net_amount cannot be negative", idx)
		}

		run.GrossTotal += lineItem.GrossAmount
		run.BonusTotal += lineItem.BonusAmount
		run.DeductionTotal += lineItem.DeductionAmount
		run.NetTotal += lineItem.NetAmount

		lineItems = append(lineItems, lineItem)
	}

	err = s.db.GetGorm().Transaction(func(tx *gorm.DB) error {
		// Serialize concurrent payroll creates for this business by locking the
		// business row, then re-run the duplicate-period check INSIDE the lock.
		// The pre-check above is a read-then-write across separate statements, so
		// two truly-concurrent identical submissions could both pass it and each
		// create a run — double payroll. Locking the parent business row makes the
		// check-then-insert atomic without needing a partial unique index (which
		// would risk failing to build on any pre-existing duplicate). On SQLite the
		// lock clause is a no-op but its single-writer model serializes anyway.
		var lockBiz database.Business
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").First(&lockBiz, input.BusinessID).Error; err != nil {
			return fmt.Errorf("failed to lock business for payroll create: %w", err)
		}

		var dupInTx int64
		if err := overlappingPayrollRuns(tx, input.BusinessID, start, end).
			Count(&dupInTx).Error; err != nil {
			return fmt.Errorf("failed to re-check duplicate payroll run: %w", err)
		}
		if dupInTx > 0 {
			return ErrDuplicatePayrollPeriod
		}

		if err := tx.Create(run).Error; err != nil {
			return fmt.Errorf("failed to create payroll run: %w", err)
		}

		for idx := range lineItems {
			lineItems[idx].PayrollRunID = run.ID
		}
		if err := tx.Create(&lineItems).Error; err != nil {
			return fmt.Errorf("failed to create payroll line items: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	run.LineItems = lineItems
	return run, nil
}

// applyPayrollRunsFilters applies status + date windowing shared by legacy
// ListPayrollRuns and ListPayrollRunsPage. Paid runs window on paid_at; draft
// and void window on period_* so voided runs remain visible for audit.
func applyPayrollRunsFilters(query *gorm.DB, startDate, endDate *time.Time, status *database.PayrollRunStatus) *gorm.DB {
	if status != nil {
		query = query.Where("status = ?", *status)
		switch *status {
		case database.PayrollRunStatusPaid:
			if startDate != nil {
				query = query.Where("paid_at IS NOT NULL AND paid_at >= ?", startDate.UTC())
			}
			if endDate != nil {
				query = query.Where("paid_at IS NOT NULL AND paid_at < ?", endDate.UTC())
			}
		default:
			if startDate != nil {
				query = query.Where("period_end >= ?", startDate.UTC())
			}
			if endDate != nil {
				query = query.Where("period_start < ?", endDate.UTC())
			}
		}
		return query
	}

	// Paid runs are windowed by paid_at; draft AND void runs by their period
	// (void runs stay visible for the audit trail in the period they covered).
	periodStatuses := []database.PayrollRunStatus{
		database.PayrollRunStatusDraft,
		database.PayrollRunStatusVoid,
	}
	switch {
	case startDate != nil && endDate != nil:
		query = query.Where(
			"(status = ? AND paid_at IS NOT NULL AND paid_at >= ? AND paid_at < ?) OR (status IN ? AND period_end >= ? AND period_start < ?)",
			database.PayrollRunStatusPaid,
			startDate.UTC(),
			endDate.UTC(),
			periodStatuses,
			startDate.UTC(),
			endDate.UTC(),
		)
	case startDate != nil:
		query = query.Where(
			"(status = ? AND paid_at IS NOT NULL AND paid_at >= ?) OR (status IN ? AND period_end >= ?)",
			database.PayrollRunStatusPaid,
			startDate.UTC(),
			periodStatuses,
			startDate.UTC(),
		)
	case endDate != nil:
		query = query.Where(
			"(status = ? AND paid_at IS NOT NULL AND paid_at < ?) OR (status IN ? AND period_start < ?)",
			database.PayrollRunStatusPaid,
			endDate.UTC(),
			periodStatuses,
			endDate.UTC(),
		)
	}
	return query
}

// ListPayrollRunsPage returns a paginated payroll runs list with payee_count
// filled from a single GROUP BY aggregate (no LineItems Preload).
func (s *Service) ListPayrollRunsPage(params ListPayrollRunsParams) (*PayrollRunsPage, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var status *database.PayrollRunStatus
	if raw := strings.TrimSpace(params.Status); raw != "" {
		value := database.PayrollRunStatus(raw)
		status = &value
	}

	query := s.db.GetGorm().
		Model(&database.PayrollRun{}).
		Where("business_id = ?", params.BusinessID)

	query = applyPayrollRunsFilters(query, params.Start, params.End, status)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count payroll runs: %w", err)
	}

	var runs []database.PayrollRun
	// Intentionally no Preload("LineItems") — payee_count comes from one aggregate below.
	if err := query.
		Order("period_start DESC, id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&runs).Error; err != nil {
		return nil, fmt.Errorf("failed to list payroll runs: %w", err)
	}

	counts := make(map[uint]int, len(runs))
	if len(runs) > 0 {
		ids := make([]uint, len(runs))
		for i, run := range runs {
			ids[i] = run.ID
		}
		type payeeCountRow struct {
			PayrollRunID uint
			Cnt          int
		}
		var rows []payeeCountRow
		if err := s.db.GetGorm().
			Model(&database.PayrollLineItem{}).
			Select("payroll_run_id, COUNT(*) AS cnt").
			Where("payroll_run_id IN ?", ids).
			Group("payroll_run_id").
			Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("failed to count payroll payees: %w", err)
		}
		for _, row := range rows {
			counts[row.PayrollRunID] = row.Cnt
		}
	}

	out := make([]PayrollRunRow, len(runs))
	for i, run := range runs {
		out[i] = PayrollRunRow{
			PayrollRun: run,
			PayeeCount: counts[run.ID],
		}
	}

	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return &PayrollRunsPage{
		Runs:       out,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// payrollActorStaffSelect is the narrow projection for detail-drawer actor
// staff (created / paid / voided by). FE LedgerActorStaff only needs id + name;
// leaving created_at unloaded triggers Staff.MarshalJSON's slim {id,name,email}
// shape (FIND-052) and avoids serializing role / authz / permission columns.
const payrollActorStaffSelect = "id, name, email"

func (s *Service) GetPayrollRun(businessID, runID uint) (*database.PayrollRun, error) {
	var run database.PayrollRun
	if err := s.db.GetGorm().
		Preload("LineItems").
		// Audit actors for the detail drawer — id+name only (L6-16).
		Preload("CreatedByStaff", func(db *gorm.DB) *gorm.DB {
			return db.Select(payrollActorStaffSelect)
		}).
		Preload("PaidByStaff", func(db *gorm.DB) *gorm.DB {
			return db.Select(payrollActorStaffSelect)
		}).
		Preload("VoidedByStaff", func(db *gorm.DB) *gorm.DB {
			return db.Select(payrollActorStaffSelect)
		}).
		Where("id = ? AND business_id = ?", runID, businessID).
		First(&run).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("payroll run not found")
		}
		return nil, fmt.Errorf("failed to load payroll run: %w", err)
	}
	return &run, nil
}

func (s *Service) MarkPayrollRunPaid(businessID, runID uint, paidAt time.Time, paidByUserID, paidByStaffID *uint) (*database.PayrollRun, error) {
	if paidAt.IsZero() {
		paidAt = time.Now().UTC()
	}

	// Compare-and-set: only a DRAFT run transitions to paid, in a single
	// conditional UPDATE. This closes the TOCTOU window where two concurrent
	// marks both read 'draft' and both wrote (last-writer-wins on the audit
	// actor), and it makes the operation safely idempotent. The period lock
	// is checked inside the same transaction under LockBooksTx, so a close
	// that lands concurrently cannot be bypassed.
	var rowsAffected int64
	err := s.db.GetGorm().Transaction(func(tx *gorm.DB) error {
		if err := LockBooksTx(tx, businessID); err != nil {
			return fmt.Errorf("failed to lock books: %w", err)
		}
		var run database.PayrollRun
		if err := tx.Where("id = ? AND business_id = ?", runID, businessID).First(&run).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("payroll run not found")
			}
			return fmt.Errorf("failed to load payroll run: %w", err)
		}
		if run.Status == database.PayrollRunStatusDraft {
			if err := checkPayrollRunPeriodTx(tx, businessID, &run, paidAt); err != nil {
				return err
			}
		}
		res := tx.
			Model(&database.PayrollRun{}).
			Where("id = ? AND business_id = ? AND status = ?", runID, businessID, database.PayrollRunStatusDraft).
			Updates(map[string]any{
				"status":           database.PayrollRunStatusPaid,
				"paid_at":          paidAt,
				"paid_by_user_id":  paidByUserID,
				"paid_by_staff_id": paidByStaffID,
				"updated_at":       paidAt,
			})
		if res.Error != nil {
			return fmt.Errorf("failed to mark payroll run paid: %w", res.Error)
		}
		rowsAffected = res.RowsAffected
		return nil
	})
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		// No draft row matched: the run is already paid (idempotent), or in a
		// non-payable terminal state (void).
		run, err := s.GetPayrollRun(businessID, runID)
		if err != nil {
			return nil, err
		}
		switch run.Status {
		case database.PayrollRunStatusPaid:
			return run, nil
		case database.PayrollRunStatusVoid:
			return nil, fmt.Errorf("cannot mark a voided payroll run paid")
		default:
			return nil, fmt.Errorf("payroll run is not in a payable state")
		}
	}

	return s.GetPayrollRun(businessID, runID)
}

// DeletePayrollRun permanently removes a DRAFT payroll run and its line items.
// Only drafts can be deleted — a paid run carries financial meaning and must be
// VoidPayrollRun'd instead (which keeps the audit trail). The run is locked and
// the parent delete remains a status CAS, so a concurrent mark-paid and delete
// cannot both succeed.
func (s *Service) DeletePayrollRun(businessID, runID uint) error {
	return s.db.GetGorm().Transaction(func(tx *gorm.DB) error {
		if err := LockBooksTx(tx, businessID); err != nil {
			return fmt.Errorf("failed to lock books: %w", err)
		}
		var run database.PayrollRun
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND business_id = ?", runID, businessID).
			First(&run).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("payroll run not found")
			}
			return fmt.Errorf("failed to lock payroll run: %w", err)
		}
		if run.Status != database.PayrollRunStatusDraft {
			return fmt.Errorf("only draft payroll runs can be deleted; void a paid run instead")
		}
		if err := checkPayrollRunPeriodTx(tx, businessID, &run); err != nil {
			return err
		}

		if err := tx.
			Where("payroll_run_id = ? AND business_id = ?", runID, businessID).
			Delete(&database.PayrollLineItem{}).Error; err != nil {
			return fmt.Errorf("failed to delete payroll line items: %w", err)
		}

		res := tx.
			Where("id = ? AND business_id = ? AND status = ?", runID, businessID, database.PayrollRunStatusDraft).
			Delete(&database.PayrollRun{})
		if res.Error != nil {
			return fmt.Errorf("failed to delete payroll run: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("only draft payroll runs can be deleted; void a paid run instead")
		}
		return nil
	})
}

// VoidPayrollRun reverses a PAID payroll run: it flips the status to 'void' and
// records the actor, so the run's net no longer counts toward P&L while staying
// visible for the audit trail. CAS on status='paid'; idempotent if already void.
func (s *Service) VoidPayrollRun(businessID, runID uint, voidedAt time.Time, voidedByUserID, voidedByStaffID *uint) (*database.PayrollRun, error) {
	if voidedAt.IsZero() {
		voidedAt = time.Now().UTC()
	}

	// The period check runs in the write transaction under LockBooksTx (see
	// MarkPayrollRunPaid).
	var rowsAffected int64
	err := s.db.GetGorm().Transaction(func(tx *gorm.DB) error {
		if err := LockBooksTx(tx, businessID); err != nil {
			return fmt.Errorf("failed to lock books: %w", err)
		}
		var run database.PayrollRun
		if err := tx.Where("id = ? AND business_id = ?", runID, businessID).First(&run).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("payroll run not found")
			}
			return fmt.Errorf("failed to load payroll run: %w", err)
		}
		if run.Status == database.PayrollRunStatusPaid {
			if err := checkPayrollRunPeriodTx(tx, businessID, &run); err != nil {
				return err
			}
		}
		res := tx.
			Model(&database.PayrollRun{}).
			Where("id = ? AND business_id = ? AND status = ?", runID, businessID, database.PayrollRunStatusPaid).
			Updates(map[string]any{
				"status":             database.PayrollRunStatusVoid,
				"voided_at":          voidedAt,
				"voided_by_user_id":  voidedByUserID,
				"voided_by_staff_id": voidedByStaffID,
				"updated_at":         voidedAt,
			})
		if res.Error != nil {
			return fmt.Errorf("failed to void payroll run: %w", res.Error)
		}
		rowsAffected = res.RowsAffected
		return nil
	})
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		run, err := s.GetPayrollRun(businessID, runID)
		if err != nil {
			return nil, err
		}
		switch run.Status {
		case database.PayrollRunStatusVoid:
			return run, nil // idempotent
		case database.PayrollRunStatusDraft:
			return nil, fmt.Errorf("only paid payroll runs can be voided; delete the draft instead")
		default:
			return nil, fmt.Errorf("payroll run cannot be voided")
		}
	}

	return s.GetPayrollRun(businessID, runID)
}

// GetSummary builds the profit & loss summary for [startDate, endDate).
//
// Recognition basis (PAY-6) — the window is applied to a different timestamp per
// source, by design:
//   - Auto income (confirmed payments + bill-managed alt-payments): the shared
//     recognized-payment ledger (database/payment_ledger.go) — recognized at
//     confirmed_at, with refunds/reversals netted by a dated negative event at
//     refund time (matches the analytics dashboard; earning periods stay
//     immutable).
//   - Manual ledger entries (income/expense): occurred_at (operator-stated date).
//   - Payroll: paid_at, and ONLY status='paid' runs count (cash basis — a run is
//     an expense when it is paid, not when its period falls). Draft and void runs
//     never hit P&L.
//
// So a run whose period is in June but is marked paid in July lands in July's P&L.
// This is intentional cash-basis treatment for payroll; do not "fix" it to the
// period window without revisiting the income/expense bases too.
func (s *Service) GetSummary(businessID uint, startDate, endDate time.Time) (*Summary, error) {
	start, end, err := normalizeRange(startDate, endDate)
	if err != nil {
		return nil, err
	}

	summary := &Summary{
		StartDate:            start,
		EndDate:              end,
		Currency:             s.resolveCurrency(businessID, ""),
		IncomeBreakdown:      []CategoryTotal{},
		ExpenseBreakdown:     []CategoryTotal{},
		Warnings:             []FXWarning{},
		SkippedManualEntries: 0,
	}

	db := s.db.GetGorm()

	// Auto income uses the recognized-payment ledger — the single source of truth
	// shared with the analytics dashboard (see database/payment_ledger.go). A
	// payment stays recognized in the period it was earned (confirmed_at), and a
	// later refund/reversal is netted by a DATED negative event at refund time
	// rather than being retroactively erased from the earning period. This keeps
	// closed-period P&L immutable and consistent with analytics revenue for the
	// same window. The ledger also applies the plugin double-count guard (only
	// bill-managed alt-payment methods count) and the legacy-bill fallback, so
	// this one call replaces the previous bespoke confirmed-only queries.
	recognized, err := database.GetRecognizedPaymentSummary(businessID, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to summarize recognized payments: %w", err)
	}
	summary.AutoIncomeTotal = float64(recognized.TotalRevenueCents) / 100.0

	// Voided bills carry no collection expectation (VoidBill preserves
	// total_amount for audit but the money was never owed), so they must not
	// inflate billed_total. Live open/partial checks still count toward billed
	// activity. collection_gap is leftover remainings on the same non-void set.
	if err := db.Table("bills").
		Select("COALESCE(SUM(total_amount), 0) / 100.0").
		Where("business_id = ? AND status != ? AND created_at >= ? AND created_at < ?",
			businessID, database.BillStatusVoided, start, end).
		Scan(&summary.BilledTotal).Error; err != nil {
		return nil, fmt.Errorf("failed to summarize billed totals: %w", err)
	}

	// Pre-aggregate manual entries and paid payroll in SQL (GROUP BY day,
	// currency, category) so FX conversion is O(days×currencies×categories)
	// instead of O(records). A single historical-rate resolver still covers
	// income + expense + payroll; single-currency businesses issue zero rate
	// queries (resolver early-returns when no source currency differs).
	dialect := db.Name()
	loc := time.UTC

	incomeBuckets, err := s.loadManualDayCurrencyCategoryTotals(businessID, database.AccountingEntryTypeIncome, start, end, dialect, loc)
	if err != nil {
		return nil, err
	}
	expenseBuckets, err := s.loadManualDayCurrencyCategoryTotals(businessID, database.AccountingEntryTypeExpense, start, end, dialect, loc)
	if err != nil {
		return nil, err
	}
	payrollBuckets, err := s.loadPayrollDayCurrencyAggs(businessID, start, end, dialect, loc)
	if err != nil {
		return nil, err
	}

	rawCurrencies := make([]string, 0, len(incomeBuckets)+len(expenseBuckets)+len(payrollBuckets))
	for _, b := range incomeBuckets {
		rawCurrencies = append(rawCurrencies, b.Currency)
	}
	for _, b := range expenseBuckets {
		rawCurrencies = append(rawCurrencies, b.Currency)
	}
	for _, b := range payrollBuckets {
		rawCurrencies = append(rawCurrencies, b.Currency)
	}
	resolver, err := s.newHistoricalRateResolverForCurrencies(rawCurrencies, summary.Currency, start, end)
	if err != nil {
		return nil, err
	}

	manualIncomeSummary := s.summarizeManualBuckets(
		incomeBuckets, resolver, summary.Currency, database.AccountingEntryTypeIncome,
		businessID, start, end, loc,
	)
	summary.ManualIncomeTotal = manualIncomeSummary.Total
	summary.IncomeBreakdown = manualIncomeSummary.Breakdown
	summary.SkippedManualEntries += manualIncomeSummary.SkippedCount
	summary.Warnings = append(summary.Warnings, manualIncomeSummary.Warnings...)

	expenseSummary := s.summarizeManualBuckets(
		expenseBuckets, resolver, summary.Currency, database.AccountingEntryTypeExpense,
		businessID, start, end, loc,
	)
	summary.ExpenseTotal = expenseSummary.Total
	summary.ExpenseBreakdown = expenseSummary.Breakdown
	summary.SkippedManualEntries += expenseSummary.SkippedCount
	summary.Warnings = append(summary.Warnings, expenseSummary.Warnings...)

	payrollSummary, payrollTotal, payrollSkipped, payrollWarnings := s.summarizePaidPayrollBuckets(
		payrollBuckets, resolver, summary.Currency, loc,
	)
	summary.PayrollSummary = payrollSummary
	summary.PayrollTotal = payrollTotal
	summary.SkippedManualEntries += payrollSkipped
	summary.Warnings = append(summary.Warnings, payrollWarnings...)

	summary.CollectedTotal = summary.AutoIncomeTotal
	// Collection gap is leftover remaining on every non-void bill created in
	// the window — closed/abandoned walk-outs AND live open/partial checks.
	// Counting only unpaid *closed* checks made BRECHA DE COBRO ignore the
	// floor (billed − collected) (#651). unpaid-bills uses the same remaining
	// predicate as-of end so the KPI always has a drill-down (#770).
	var remainingDue float64
	if err := RemainingDueInRange(db.Table("bills"), businessID, start, end, "").
		Select("COALESCE(SUM(total_amount - paid_amount), 0) / 100.0").
		Scan(&remainingDue).Error; err != nil {
		return nil, fmt.Errorf("failed to summarize collection gap: %w", err)
	}
	summary.CollectionGap = remainingDue
	// Net is not derived here — use ComposeProfitLoss / GetProfitLoss (includes COGS).

	return summary, nil
}

// loadManualDayCurrencyCategoryTotals aggregates non-voided manual ledger
// entries of a given type into (business-local day, currency, category) →
// SUM(amount cents) buckets for GetSummary.
func (s *Service) loadManualDayCurrencyCategoryTotals(
	businessID uint,
	entryType database.AccountingEntryType,
	start, end time.Time,
	dialect string,
	loc *time.Location,
) ([]dayCurrencyCategoryTotal, error) {
	dayExpr := dayBucketExpression(dialect, "occurred_at", loc)
	var rows []dayCurrencyCategoryTotal
	query := fmt.Sprintf(`
		SELECT %s AS day, currency AS currency, category AS category, COALESCE(SUM(amount), 0) AS total
		FROM manual_ledger_entries
		WHERE business_id = ? AND entry_type = ? AND voided_at IS NULL
		  AND occurred_at >= ? AND occurred_at < ?
		GROUP BY day, currency, category
		ORDER BY category ASC, day ASC, currency ASC
	`, dayExpr)
	if err := s.db.GetGorm().Raw(query, businessID, entryType, start, end).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to load manual %s day/category buckets: %w", entryType, err)
	}
	return rows, nil
}

// loadPayrollDayCurrencyAggs aggregates paid payroll runs into (day of paid_at,
// currency) buckets with summed totals (cents) and run count.
func (s *Service) loadPayrollDayCurrencyAggs(
	businessID uint,
	start, end time.Time,
	dialect string,
	loc *time.Location,
) ([]dayCurrencyPayrollAgg, error) {
	dayExpr := dayBucketExpression(dialect, "paid_at", loc)
	var rows []dayCurrencyPayrollAgg
	query := fmt.Sprintf(`
		SELECT %s AS day, currency AS currency,
			COALESCE(SUM(gross_total), 0) AS gross_total,
			COALESCE(SUM(bonus_total), 0) AS bonus_total,
			COALESCE(SUM(deduction_total), 0) AS deduction_total,
			COALESCE(SUM(net_total), 0) AS net_total,
			COUNT(*) AS run_count
		FROM payroll_runs
		WHERE business_id = ? AND status = ? AND paid_at IS NOT NULL
		  AND paid_at >= ? AND paid_at < ?
		GROUP BY day, currency
		ORDER BY day ASC, currency ASC
	`, dayExpr)
	if err := s.db.GetGorm().Raw(query, businessID, database.PayrollRunStatusPaid, start, end).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to load payroll day/currency buckets: %w", err)
	}
	return rows, nil
}

// summarizeManualBuckets converts pre-aggregated (day, currency, category)
// buckets into reporting-currency dollars using the rate at local day start.
// Buckets whose currency cannot convert trigger a slow-path re-query for entry
// IDs so skipped_manual_entries / warning details match the per-record path.
func (s *Service) summarizeManualBuckets(
	buckets []dayCurrencyCategoryTotal,
	resolver *historicalRateResolver,
	reportingCurrency string,
	entryType database.AccountingEntryType,
	businessID uint,
	start, end time.Time,
	loc *time.Location,
) *manualEntriesSummary {
	result := &manualEntriesSummary{
		Breakdown: []CategoryTotal{},
	}
	breakdown := map[string]float64{}
	failed := make(map[dayCurrencyKey]struct{})

	for _, b := range buckets {
		dollars, ok := convertDayBucketAmount(resolver, b.Total, b.Currency, reportingCurrency, b.Day, loc)
		if !ok {
			failed[dayCurrencyKey{Day: b.Day, Currency: b.Currency}] = struct{}{}
			continue
		}
		result.Total += dollars
		breakdown[b.Category] += dollars
	}

	result.Breakdown = categoryTotalsFromMap(breakdown)
	if len(failed) == 0 {
		return result
	}

	skippedCount, skippedDetails := s.sampleSkippedManualEntries(businessID, entryType, start, end, failed, loc)
	result.SkippedCount = skippedCount
	params := map[string]string{
		"count":      strconv.Itoa(skippedCount),
		"entry_type": string(entryType),
		"currency":   reportingCurrency,
		"scope":      "manual_" + string(entryType),
	}
	if len(skippedDetails) > 0 {
		params["details"] = strings.Join(skippedDetails, ", ")
	}
	result.Warnings = append(result.Warnings, FXWarning{
		Code:   "fx_rate_missing",
		Params: params,
	})
	return result
}

// sampleSkippedManualEntries re-queries entry IDs for failed (day, currency)
// buckets so warning details keep the "#id currency" form (max 3 samples).
// Count equals the number of non-voided entries in those buckets.
func (s *Service) sampleSkippedManualEntries(
	businessID uint,
	entryType database.AccountingEntryType,
	start, end time.Time,
	failed map[dayCurrencyKey]struct{},
	loc *time.Location,
) (int, []string) {
	if len(failed) == 0 {
		return 0, nil
	}
	if loc == nil {
		loc = time.UTC
	}

	currencies := make([]string, 0, len(failed))
	seenCur := map[string]struct{}{}
	for k := range failed {
		if _, ok := seenCur[k.Currency]; ok {
			continue
		}
		seenCur[k.Currency] = struct{}{}
		currencies = append(currencies, k.Currency)
	}
	sort.Strings(currencies)

	type skipRow struct {
		ID         uint
		Currency   string
		OccurredAt time.Time
	}
	var rows []skipRow
	if err := s.db.GetGorm().
		Table("manual_ledger_entries").
		Select("id, currency, occurred_at").
		Where("business_id = ? AND entry_type = ? AND voided_at IS NULL AND occurred_at >= ? AND occurred_at < ?",
			businessID, entryType, start, end).
		Where("currency IN ?", currencies).
		Order("category ASC, occurred_at ASC, id ASC").
		Scan(&rows).Error; err != nil {
		// Slow-path failure: still report at least one skip so operators know
		// something was omitted; details stay empty rather than invent IDs.
		return len(failed), nil
	}

	count := 0
	details := make([]string, 0, 3)
	for _, row := range rows {
		day := row.OccurredAt.In(loc).Format("2006-01-02")
		if _, ok := failed[dayCurrencyKey{Day: day, Currency: row.Currency}]; !ok {
			continue
		}
		count++
		if len(details) < 3 {
			details = append(details, fmt.Sprintf("#%d %s", row.ID, row.Currency))
		}
	}
	return count, details
}

// summarizePaidPayrollBuckets converts pre-aggregated paid payroll day/currency
// buckets into reporting currency (rate at local day start of paid_at). Failed
// buckets contribute RunCount to the skip total; warning details list the
// currency once per skipped run (capped at 3).
func (s *Service) summarizePaidPayrollBuckets(
	buckets []dayCurrencyPayrollAgg,
	resolver *historicalRateResolver,
	reportingCurrency string,
	loc *time.Location,
) (PayrollSummary, float64, int, []FXWarning) {
	summary := PayrollSummary{}
	skipped := 0
	skippedDetails := make([]string, 0, 3)
	var warnings []FXWarning

	for _, row := range buckets {
		convert := func(cents int64) (float64, bool) {
			return convertDayBucketAmount(resolver, cents, row.Currency, reportingCurrency, row.Day, loc)
		}

		gross, okGross := convert(row.GrossTotal)
		bonus, okBonus := convert(row.BonusTotal)
		deduction, okDeduction := convert(row.DeductionTotal)
		net, okNet := convert(row.NetTotal)
		if !okGross || !okBonus || !okDeduction || !okNet {
			runCount := row.RunCount
			if runCount <= 0 {
				runCount = 1
			}
			skipped += runCount
			for i := 0; i < runCount && len(skippedDetails) < 3; i++ {
				skippedDetails = append(skippedDetails, row.Currency)
			}
			continue
		}

		summary.PaidRuns += row.RunCount
		summary.TotalGross += gross
		summary.TotalBonus += bonus
		summary.TotalDeduction += deduction
		summary.TotalNet += net
	}

	if skipped > 0 {
		params := map[string]string{
			"count":    strconv.Itoa(skipped),
			"currency": reportingCurrency,
			"scope":    "payroll",
		}
		if len(skippedDetails) > 0 {
			params["details"] = strings.Join(skippedDetails, ", ")
		}
		warnings = append(warnings, FXWarning{
			Code:   "fx_rate_missing",
			Params: params,
		})
	}

	return summary, summary.TotalNet, skipped, warnings
}

// GetTimeseries builds a contiguous day-bucket series of manual income, manual
// expense, and paid payroll for [startDate, endDate) in the business timezone.
//
// Amounts are summed in SQL as int64 cents per (day, currency), FX-converted
// with the shared historical-rate resolver (rate at local day start), then
// emitted as float64 dollars. Voided ledger entries and non-paid payroll runs
// are excluded. The range is capped at maxTimeseriesDays day buckets.
//
// loc is the business timezone used for day labels and SQL day extraction; nil
// falls back to UTC (matches resolveBusinessLocation fallback).
func (s *Service) GetTimeseries(businessID uint, startDate, endDate time.Time, loc *time.Location) (*Timeseries, error) {
	start, end, err := normalizeRange(startDate, endDate)
	if err != nil {
		return nil, err
	}
	if loc == nil {
		loc = time.UTC
	}

	days := enumerateBusinessDays(start, end, loc)
	if len(days) == 0 {
		return nil, fmt.Errorf("%w: empty day range", ErrInvalidDateRange)
	}
	if len(days) > maxTimeseriesDays {
		return nil, fmt.Errorf("%w: %d days requested (max %d)", ErrRangeTooLarge, len(days), maxTimeseriesDays)
	}

	reportingCurrency := s.resolveCurrency(businessID, "")
	dialect := s.db.GetGorm().Name()

	incomeBuckets, err := s.loadManualDayCurrencyTotals(businessID, database.AccountingEntryTypeIncome, start, end, dialect, loc)
	if err != nil {
		return nil, err
	}
	expenseBuckets, err := s.loadManualDayCurrencyTotals(businessID, database.AccountingEntryTypeExpense, start, end, dialect, loc)
	if err != nil {
		return nil, err
	}
	payrollBuckets, err := s.loadPayrollDayCurrencyTotals(businessID, start, end, dialect, loc)
	if err != nil {
		return nil, err
	}

	rawCurrencies := make([]string, 0, len(incomeBuckets)+len(expenseBuckets)+len(payrollBuckets))
	for _, b := range incomeBuckets {
		rawCurrencies = append(rawCurrencies, b.Currency)
	}
	for _, b := range expenseBuckets {
		rawCurrencies = append(rawCurrencies, b.Currency)
	}
	for _, b := range payrollBuckets {
		rawCurrencies = append(rawCurrencies, b.Currency)
	}
	resolver, err := s.newHistoricalRateResolverForCurrencies(rawCurrencies, reportingCurrency, start, end)
	if err != nil {
		return nil, err
	}

	byDay := make(map[string]*TimeseriesPoint, len(days))
	for _, day := range days {
		byDay[day] = &TimeseriesPoint{Date: day}
	}

	addBuckets := func(buckets []dayCurrencyTotal, field string) {
		for _, b := range buckets {
			point, ok := byDay[b.Day]
			if !ok {
				// Outside the enumerated window (shouldn't happen for aligned
				// half-open ranges); ignore rather than inflate the series.
				continue
			}
			dollars, convOK := convertDayBucketAmount(resolver, b.Total, b.Currency, reportingCurrency, b.Day, loc)
			if !convOK {
				continue
			}
			switch field {
			case "income":
				point.Income += dollars
			case "expense":
				point.Expense += dollars
			case "payroll":
				point.Payroll += dollars
			}
		}
	}
	addBuckets(incomeBuckets, "income")
	addBuckets(expenseBuckets, "expense")
	addBuckets(payrollBuckets, "payroll")

	series := make([]TimeseriesPoint, 0, len(days))
	for _, day := range days {
		series = append(series, *byDay[day])
	}

	return &Timeseries{
		Start:    days[0],
		End:      days[len(days)-1],
		Bucket:   "day",
		Currency: reportingCurrency,
		Series:   series,
	}, nil
}

// loadManualDayCurrencyTotals aggregates non-voided manual ledger entries of a
// given type into (business-local day, currency) → SUM(amount cents) buckets.
func (s *Service) loadManualDayCurrencyTotals(
	businessID uint,
	entryType database.AccountingEntryType,
	start, end time.Time,
	dialect string,
	loc *time.Location,
) ([]dayCurrencyTotal, error) {
	dayExpr := dayBucketExpression(dialect, "occurred_at", loc)
	var rows []dayCurrencyTotal
	query := fmt.Sprintf(`
		SELECT %s AS day, currency AS currency, COALESCE(SUM(amount), 0) AS total
		FROM manual_ledger_entries
		WHERE business_id = ? AND entry_type = ? AND voided_at IS NULL
		  AND occurred_at >= ? AND occurred_at < ?
		GROUP BY day, currency
		ORDER BY day ASC, currency ASC
	`, dayExpr)
	if err := s.db.GetGorm().Raw(query, businessID, entryType, start, end).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to load manual %s day buckets: %w", entryType, err)
	}
	return rows, nil
}

// loadPayrollDayCurrencyTotals aggregates paid payroll runs into
// (business-local day of paid_at, currency) → SUM(net_total cents) buckets.
func (s *Service) loadPayrollDayCurrencyTotals(
	businessID uint,
	start, end time.Time,
	dialect string,
	loc *time.Location,
) ([]dayCurrencyTotal, error) {
	dayExpr := dayBucketExpression(dialect, "paid_at", loc)
	var rows []dayCurrencyTotal
	query := fmt.Sprintf(`
		SELECT %s AS day, currency AS currency, COALESCE(SUM(net_total), 0) AS total
		FROM payroll_runs
		WHERE business_id = ? AND status = ? AND paid_at IS NOT NULL
		  AND paid_at >= ? AND paid_at < ?
		GROUP BY day, currency
		ORDER BY day ASC, currency ASC
	`, dayExpr)
	if err := s.db.GetGorm().Raw(query, businessID, database.PayrollRunStatusPaid, start, end).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to load payroll day buckets: %w", err)
	}
	return rows, nil
}

// dayBucketExpression returns a SQL expression that yields YYYY-MM-DD for a
// timestamp column in loc. Mirrors analytics dayExpressionForDialect so
// SQLite tests (UTC or fixed-offset zones) and Postgres (IANA AT TIME ZONE)
// agree on business-local calendar days.
func dayBucketExpression(dialectName, column string, loc *time.Location) string {
	col := localizedDayColumn(dialectName, column, loc)
	if dialectName == "sqlite" {
		return fmt.Sprintf("strftime('%%Y-%%m-%%d', %s)", col)
	}
	return fmt.Sprintf("to_char(%s, 'YYYY-MM-DD')", col)
}

// localizedDayColumn shifts a stored-UTC timestamp into loc for day extraction.
// UTC/nil leaves the column unchanged so SQLite unit tests stay simple.
func localizedDayColumn(dialectName, column string, loc *time.Location) string {
	if loc == nil || loc == time.UTC {
		return column
	}
	if dialectName == "sqlite" {
		_, offset := time.Now().In(loc).Zone()
		return fmt.Sprintf("datetime(%s, '%+d seconds')", column, offset)
	}
	// Postgres: session is UTC; cast then AT TIME ZONE yields local wall time.
	return fmt.Sprintf("(%s)::timestamptz AT TIME ZONE '%s'", column, sqlSafeZoneName(loc))
}

// sqlSafeZoneName returns an IANA zone name safe for embedding in SQL. Only
// allows alphanumerics, slash, underscore, plus, and hyphen (mirrors analytics).
func sqlSafeZoneName(loc *time.Location) string {
	if loc == nil {
		return "UTC"
	}
	name := loc.String()
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '/', r == '_', r == '+', r == '-':
		default:
			return "UTC"
		}
	}
	return name
}

// enumerateBusinessDays returns every YYYY-MM-DD key from start (inclusive) to
// end (exclusive) stepping in loc so half-open UTC windows from parseDateRange
// produce the correct business-local day list (DST-safe via AddDate).
func enumerateBusinessDays(start, end time.Time, loc *time.Location) []string {
	if loc == nil {
		loc = time.UTC
	}
	startLocal := start.In(loc)
	endLocal := end.In(loc)
	cursor := time.Date(startLocal.Year(), startLocal.Month(), startLocal.Day(), 0, 0, 0, 0, loc)
	limit := time.Date(endLocal.Year(), endLocal.Month(), endLocal.Day(), 0, 0, 0, 0, loc)

	var days []string
	for d := cursor; d.Before(limit); d = d.AddDate(0, 0, 1) {
		days = append(days, d.Format("2006-01-02"))
	}
	return days
}

// convertDayBucketAmount converts a cents total from sourceCurrency to
// reportingCurrency using the rate at local day start. Empty source currency
// (legacy/unstamped payroll) is taken at face value.
func convertDayBucketAmount(
	resolver *historicalRateResolver,
	cents int64,
	sourceCurrency, reportingCurrency, day string,
	loc *time.Location,
) (float64, bool) {
	dollars := float64(cents) / 100.0
	from := normalizeCurrencyCode(sourceCurrency)
	if from == "" || from == reportingCurrency {
		return dollars, true
	}
	asOf := dayStartInLocation(day, loc)
	converted, err := resolver.convertAmountAt(dollars, sourceCurrency, reportingCurrency, asOf)
	if err != nil {
		return 0, false
	}
	return converted, true
}

func dayStartInLocation(day string, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	t, err := time.ParseInLocation("2006-01-02", day, loc)
	if err != nil {
		// Malformed day keys from SQL should not panic conversion; fall back to UTC parse.
		if parsed, perr := time.Parse("2006-01-02", day); perr == nil {
			return parsed
		}
		return time.Time{}
	}
	return t
}
