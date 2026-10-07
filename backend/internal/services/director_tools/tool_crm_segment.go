package director_tools

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// CRMSegmentTool returns a named slice of the business's customer base.
// Filters:
//   - vip               — top decile by total_spent (at least 1 row when any exist)
//   - churning          — last_visit_at older than 30 days AND visit_count >= 2
//   - new               — created within the last 14 days
//   - opt_in_marketing  — opt_in_marketing flag set
type CRMSegmentTool struct{}

// crmSegmentFilters is the allow-list surfaced in the schema and used to
// validate the incoming filter arg.
var crmSegmentFilters = []string{"vip", "churning", "new", "opt_in_marketing"}

// Name is the snake_case function identifier sent to the model.
func (t *CRMSegmentTool) Name() string { return "get_crm_segment" }

// HumanLabel is the localized pill label the UI shows while the tool runs.
func (t *CRMSegmentTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Construyendo segmento CRM"
	case "fr":
		return "Construction du segment CRM"
	case "ar":
		return "بناء شريحة CRM"
	default:
		return "Reading CRM segment"
	}
}

// Description is the model-facing tool description sent to the provider.
func (t *CRMSegmentTool) Description() string {
	return "Returns the size and aggregate spend/visit/recency stats of ONE customer segment: vip (top spenders), churning (lapsed regulars), new (recent first-timers), or opt_in_marketing (reachable for promos). Call for retention, loyalty, win-back, or marketing-reach questions. The result is aggregate-only: it contains no customer names, contact details, table numbers, or per-person history, so naming an individual customer or the table they sat at is always an invention. When size is 0 the segment is empty and avg_spend, avg_visits and last_visit_median_days_ago are ABSENT — there is nothing to report, not a customer who spends zero. Read the guidance field."
}

// Schema declares the argument shape the model sees.
func (t *CRMSegmentTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"filter": {
				Type:        llm.TypeString,
				Description: "Segment filter. One of: vip, churning, new, opt_in_marketing.",
				Enum:        crmSegmentFilters,
			},
		},
		Required: []string{"filter"},
	}
}

// Run validates the filter, queries customer_businesses, and shapes
// the size + aggregate response.
func (t *CRMSegmentTool) Run(_ context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	if env.DB == nil {
		return ToolResult{}, fmt.Errorf("get_crm_segment: nil DB in tool env")
	}

	filter, err := normalizeCRMFilter(args)
	if err != nil {
		return ToolResult{}, err
	}

	gormDB := env.DB.GetGorm()
	var rows []database.CustomerBusiness

	switch filter {
	case "vip":
		var total int64
		if err := gormDB.Model(&database.CustomerBusiness{}).
			Where("business_id = ?", env.BusinessID).
			Count(&total).Error; err != nil {
			return ToolResult{}, fmt.Errorf("get_crm_segment: count failed: %w", err)
		}
		if total == 0 {
			rows = nil
			break
		}
		topN := int(math.Round(float64(total) * 0.10))
		if topN < 1 {
			topN = 1
		}
		if err := gormDB.
			Where("business_id = ?", env.BusinessID).
			Order("total_spent DESC").
			Limit(topN).
			Find(&rows).Error; err != nil {
			return ToolResult{}, fmt.Errorf("get_crm_segment: vip query failed: %w", err)
		}

	case "churning":
		cutoff := time.Now().AddDate(0, 0, -30)
		if err := gormDB.
			Where("business_id = ? AND last_visit_at IS NOT NULL AND last_visit_at < ? AND visit_count >= ?", env.BusinessID, cutoff, 2).
			Find(&rows).Error; err != nil {
			return ToolResult{}, fmt.Errorf("get_crm_segment: churning query failed: %w", err)
		}

	case "new":
		cutoff := time.Now().AddDate(0, 0, -14)
		if err := gormDB.
			Where("business_id = ? AND created_at >= ?", env.BusinessID, cutoff).
			Find(&rows).Error; err != nil {
			return ToolResult{}, fmt.Errorf("get_crm_segment: new query failed: %w", err)
		}

	case "opt_in_marketing":
		if err := gormDB.
			Where("business_id = ? AND opt_in_marketing = ?", env.BusinessID, true).
			Find(&rows).Error; err != nil {
			return ToolResult{}, fmt.Errorf("get_crm_segment: opt_in query failed: %w", err)
		}
	}

	size := len(rows)
	avgSpend, avgVisits, medianDaysAgo, recencyKnown := aggregateCRM(rows)

	// #876: an empty segment used to serialize as avg_spend 0, avg_visits 0 and
	// last_visit_median_days_ago 0 — three measurements that read as "customers
	// who spend nothing and were here today" rather than "there are no
	// customers". That, plus a result that never said it carries no identities,
	// is what let the Director describe a named regular at a named table from a
	// segment that held nobody at all.
	data := map[string]any{
		"filter":                       filter,
		"size":                         size,
		"segment_measurable":           size > 0,
		"contains_customer_identities": false,
		"guidance":                     crmSegmentGuidance(filter, size),
	}

	summary := fmt.Sprintf("%s segment: no customers match it yet", humanLabelForFilter(filter))
	if size > 0 {
		data["avg_spend"] = avgSpend
		data["avg_visits"] = avgVisits
		if recencyKnown {
			data["last_visit_median_days_ago"] = medianDaysAgo
		}
		summary = fmt.Sprintf(
			"%s segment: %d customers, avg spend $%s",
			humanLabelForFilter(filter), size, humanizeMoney(avgSpend),
		)
	} else {
		data["no_data_reason"] = fmt.Sprintf(
			"no customer of this business matches the %s segment, so it has no spend, visit or recency figures at all",
			filter,
		)
	}

	return ToolResult{Summary: summary, Data: data}, nil
}

// crmSegmentGuidance is the anti-fabrication instruction that ships with every
// CRM segment result (#876).
//
// The tool has always been aggregate-only, but it never said so, and an empty
// segment looked identical to a segment of zero-spend customers. Both facts are
// now stated in the result itself.
func crmSegmentGuidance(filter string, size int) string {
	const neverInvent = " This result is aggregate-only: it carries no customer name, contact detail, table number, or per-person visit history. Never name a customer or the table they sat at, and quote only the counts and averages present here."

	if size == 0 {
		return fmt.Sprintf("The %s segment is EMPTY — no customer matches it, so there is nothing to report about its spend, visits or recency. Say the segment has no customers yet.", humanLabelForFilter(filter)) + neverInvent
	}
	return fmt.Sprintf("The %s segment has %d customers; the averages below describe the group, not any individual.", humanLabelForFilter(filter), size) + neverInvent
}

// aggregateCRM computes the mean spend, mean visits, and median
// "days since last visit" across the rows. Rows without a
// last_visit_at are skipped for the median; recencyKnown reports whether any
// row carried a visit date at all, so a segment nobody has visited is not
// described as having been here today (#876).
func aggregateCRM(rows []database.CustomerBusiness) (avgSpend, avgVisits float64, lastVisitMedianDaysAgo int, recencyKnown bool) {
	if len(rows) == 0 {
		return 0, 0, 0, false
	}
	var totalSpend float64
	var totalVisits int
	days := make([]int, 0, len(rows))
	now := time.Now()
	for _, r := range rows {
		totalSpend += r.TotalSpent
		totalVisits += r.VisitCount
		if r.LastVisitAt != nil {
			delta := int(now.Sub(*r.LastVisitAt).Hours() / 24)
			if delta < 0 {
				delta = 0
			}
			days = append(days, delta)
		}
	}
	avgSpend = totalSpend / float64(len(rows))
	avgVisits = float64(totalVisits) / float64(len(rows))
	if len(days) > 0 {
		recencyKnown = true
		sort.Ints(days)
		mid := len(days) / 2
		if len(days)%2 == 1 {
			lastVisitMedianDaysAgo = days[mid]
		} else {
			lastVisitMedianDaysAgo = (days[mid-1] + days[mid]) / 2
		}
	}
	return
}

// normalizeCRMFilter validates the filter arg against the allow-list.
// Returns an error when the arg is missing — the schema marks it required.
func normalizeCRMFilter(args map[string]any) (string, error) {
	raw, ok := args["filter"]
	if !ok {
		return "", fmt.Errorf("get_crm_segment: filter is required")
	}
	str, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("get_crm_segment: filter must be a string, got %T", raw)
	}
	for _, allowed := range crmSegmentFilters {
		if str == allowed {
			return str, nil
		}
	}
	return "", fmt.Errorf("get_crm_segment: unsupported filter %q (allowed: %v)", str, crmSegmentFilters)
}

// humanLabelForFilter returns the cosmetic label used in the summary
// line. Kept English-only here — the localized pill is HumanLabel.
func humanLabelForFilter(filter string) string {
	switch filter {
	case "vip":
		return "VIP"
	case "churning":
		return "Churning"
	case "new":
		return "New"
	case "opt_in_marketing":
		return "Opt-in marketing"
	default:
		return filter
	}
}
