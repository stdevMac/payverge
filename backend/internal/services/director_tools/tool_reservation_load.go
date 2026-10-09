package director_tools

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// ReservationLoadTool returns reservation volume bucketed by day across
// a lookback/lookahead window. Each row carries date, reservation count,
// covers (party-size total), and a utilization percentage relative to
// the highest-load day in the window.
//
// Implementation note: database.GetReservationStats returns only aggregate
// status counts (no by-day buckets and no party-size sum), so we query
// TableReservation directly and group in Go. That keeps the SQL portable
// across Postgres and the SQLite test DB.
type ReservationLoadTool struct{}

// Name is the snake_case function identifier sent to the model.
func (t *ReservationLoadTool) Name() string { return "get_reservation_load" }

// HumanLabel is the localized pill label the UI shows while the tool runs.
func (t *ReservationLoadTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Revisando carga de reservas"
	case "fr":
		return "Vérification de la charge de réservation"
	case "ar":
		return "مراجعة حمل الحجوزات"
	default:
		return "Reading reservation load"
	}
}

// Description is the model-facing tool description sent to the provider.
func (t *ReservationLoadTool) Description() string {
	return "Returns reservation volume by day across a lookback/lookahead window, with covers and a per-day utilization percentage. Call for booking demand, busy/quiet reservation days, or capacity-planning questions."
}

// Schema declares the argument shape the model sees.
func (t *ReservationLoadTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"lookback_days": {
				Type:        llm.TypeInteger,
				Description: "Days of history to include (0-30). Default: 7.",
			},
			"lookahead_days": {
				Type:        llm.TypeInteger,
				Description: "Days of upcoming reservations to include (0-30). Default: 7.",
			},
		},
	}
}

// Run validates the bounds, pulls reservations in the window, and
// returns a per-day breakdown.
func (t *ReservationLoadTool) Run(_ context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	if env.DB == nil {
		return ToolResult{}, fmt.Errorf("get_reservation_load: nil DB in tool env")
	}

	lookback, err := normalizeReservationDays(args, "lookback_days", 7)
	if err != nil {
		return ToolResult{}, err
	}
	lookahead, err := normalizeReservationDays(args, "lookahead_days", 7)
	if err != nil {
		return ToolResult{}, err
	}

	now := time.Now()
	// Normalize the window to whole-day boundaries so callers seeding
	// reservations "today" or "+N days" land predictably and the
	// lookahead bound is inclusive of the last day requested.
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	since := today.AddDate(0, 0, -lookback)
	until := today.AddDate(0, 0, lookahead+1)

	var rows []database.TableReservation
	if err := env.DB.GetGorm().
		Where("business_id = ? AND reservation_time >= ? AND reservation_time < ?", env.BusinessID, since, until).
		Find(&rows).Error; err != nil {
		return ToolResult{}, fmt.Errorf("get_reservation_load: query failed: %w", err)
	}

	type day struct {
		reservations int
		covers       int
	}
	buckets := make(map[string]*day, lookback+lookahead+1)
	for _, r := range rows {
		key := r.ReservationTime.Format("2006-01-02")
		b, ok := buckets[key]
		if !ok {
			b = &day{}
			buckets[key] = b
		}
		b.reservations++
		b.covers += r.PartySize
	}

	// Pin the peak so we can compute a utilization ratio that's
	// meaningful even without seat-count context. utilization_pct =
	// 100 * covers / peak_covers.
	peakCovers := 0
	peakDay := ""
	for k, b := range buckets {
		if b.covers > peakCovers {
			peakCovers = b.covers
			peakDay = k
		}
	}

	byDay := make([]map[string]any, 0, len(buckets))
	for k, b := range buckets {
		utilization := 0.0
		if peakCovers > 0 {
			utilization = (float64(b.covers) / float64(peakCovers)) * 100.0
		}
		byDay = append(byDay, map[string]any{
			"date":            k,
			"reservations":    b.reservations,
			"covers":          b.covers,
			"utilization_pct": utilization,
		})
	}
	sort.SliceStable(byDay, func(i, j int) bool {
		return byDay[i]["date"].(string) < byDay[j]["date"].(string)
	})

	totalAhead := 0
	aheadCutoff := now.Format("2006-01-02")
	for _, row := range byDay {
		if row["date"].(string) >= aheadCutoff {
			totalAhead += row["reservations"].(int)
		}
	}

	summary := fmt.Sprintf(
		"Next %dd: %d reservations, peak %s %.0f%%",
		lookahead, totalAhead, peakDay, peakCoversToPct(peakCovers, peakCovers),
	)

	return ToolResult{
		Summary: summary,
		Data: map[string]any{
			"by_day":         byDay,
			"lookback_days":  lookback,
			"lookahead_days": lookahead,
			"peak_day":       peakDay,
			"peak_covers":    peakCovers,
		},
	}, nil
}

// peakCoversToPct returns 100% when peak is set, 0% otherwise — used to
// keep the summary line stable when the window has no reservations.
func peakCoversToPct(actual, peak int) float64 {
	if peak <= 0 {
		return 0
	}
	return float64(actual) / float64(peak) * 100.0
}

// normalizeReservationDays accepts an integer arg in [0, 30],
// defaulting to fallback when missing/empty.
func normalizeReservationDays(args map[string]any, name string, fallback int) (int, error) {
	raw, ok := args[name]
	if !ok {
		return fallback, nil
	}
	var n int
	switch v := raw.(type) {
	case int:
		n = v
	case int64:
		n = int(v)
	case float64:
		n = int(v)
	case float32:
		n = int(v)
	default:
		return 0, fmt.Errorf("get_reservation_load: %s must be a number, got %T", name, raw)
	}
	if n < 0 || n > 30 {
		return 0, fmt.Errorf("get_reservation_load: %s %d out of range (0-30)", name, n)
	}
	return n, nil
}
