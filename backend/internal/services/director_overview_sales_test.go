package services

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectorPaymentsEnabled_FloorBeatsMissingPlugin(t *testing.T) {
	assert.False(t, directorPaymentsEnabled(false, 0, 0, 0))
	assert.True(t, directorPaymentsEnabled(true, 0, 0, 0))
	assert.True(t, directorPaymentsEnabled(false, 1, 0, 0), "open bill means payments work")
	assert.True(t, directorPaymentsEnabled(false, 0, 18.04, 0), "today sales means payments work")
	assert.True(t, directorPaymentsEnabled(false, 0, 0, 12.50))
	assert.False(t, directorPaymentsEnabled(false, 0, 0, 0), "kitchen tickets alone do not enable payments")
}

func TestDirectorPluginIsEnabled_LooseTypes(t *testing.T) {
	assert.True(t, directorPluginIsEnabled(true))
	assert.True(t, directorPluginIsEnabled(int64(1)))
	assert.True(t, directorPluginIsEnabled("true"))
	assert.False(t, directorPluginIsEnabled(false))
	assert.False(t, directorPluginIsEnabled(int64(0)))
	assert.False(t, directorPluginIsEnabled("no"))
}

func TestDirectorAsInt_JSONFloat64(t *testing.T) {
	live := map[string]interface{}{
		"open_checks":       float64(1),
		"open_checks_total": float64(18.04),
	}
	n, total := liveOpsOpenChecks(live)
	assert.Equal(t, 1, n)
	assert.InDelta(t, 18.04, total, 0.001)
}

func TestClassifyReadinessState_OpenCheckIsEstablished(t *testing.T) {
	assert.Equal(t, "setup", classifyReadinessState(0, false))
	assert.Equal(t, "established", classifyReadinessState(0, true), "live floor / today sales must not read as setup")
	assert.Equal(t, "established", classifyReadinessState(5, false))
}

func TestGroundOverviewSalesIfNeeded_RewritesTonightSales(t *testing.T) {
	s := &DirectorConsoleService{}
	payload := overviewSalesPayload()

	empty := DirectorStructuredResponse{
		Summary:   "Not enough data",
		Diagnosis: "There isn't enough activity yet for a data-driven analysis.",
		Evidence:  []string{"weekly_revenue is 0"},
	}
	got := s.groundOverviewSalesIfNeeded("how much did we sell tonight", "en", 86, empty, payload)
	assert.Contains(t, got.Summary, "18.04")
	assert.Contains(t, strings.ToLower(got.Summary), "remaining")
	assert.NotContains(t, strings.ToLower(got.Summary), "today's sales are $18.04")
	assert.NotContains(t, strings.ToLower(got.Summary), "not enough data")
	assert.Contains(t, strings.ToLower(got.Diagnosis), "overview")
}

func TestGroundOverviewSalesIfNeeded_DoesNotDumpSalesForBestSeller(t *testing.T) {
	s := &DirectorConsoleService{}
	payload := overviewSalesPayload()
	leaky := DirectorStructuredResponse{
		Summary:   "El sistema de pagos no está habilitado, así que no se pueden registrar ventas.",
		Diagnosis: "Payments are not enabled so Harvest Bowl cannot be ranked.",
		Evidence:  []string{"payments_enabled is false"},
		ActionPlan: []DirectorAction{{
			Title: "Connect payments", Description: "Enable a processor", DeepLink: "/business/1/dashboard?tab=plugins", Priority: "high",
		}},
	}

	got := s.groundOverviewSalesIfNeeded("what's our best seller", "es", 86, leaky, payload)
	blob := strings.ToLower(directorProseBlob(got))
	assert.NotContains(t, blob, "sistema de pagos no")
	assert.NotContains(t, blob, "payments are not enabled")
	assert.NotContains(t, got.Summary, "18.04", "best-seller must not become tonight's sales dump")
	assert.NotContains(t, strings.ToLower(got.Summary), "today's sales")
}

func TestGroundOverviewSalesIfNeeded_AnswersTableWaitFromLiveOps(t *testing.T) {
	s := &DirectorConsoleService{}
	payload := overviewSalesPayload()
	payload.LiveOps = map[string]interface{}{
		"open_checks":       float64(1),
		"open_checks_total": 18.04,
		"service_calls": []interface{}{
			map[string]any{"table": "4", "wait_minutes": float64(12), "title": "Water"},
		},
	}
	leaky := DirectorStructuredResponse{
		Summary:   "Payments are not enabled, check table 4 in person.",
		Diagnosis: "The payments system is not enabled so no sales can be recorded.",
	}

	got := s.groundOverviewSalesIfNeeded("why is table 4 waiting", "en", 86, leaky, payload)
	blob := strings.ToLower(directorProseBlob(got))
	assert.NotContains(t, blob, "payments are not enabled")
	assert.NotContains(t, got.Summary, "Today's sales are $18.04")
	assert.Contains(t, strings.ToLower(got.Summary), "table 4")
	assert.Contains(t, strings.ToLower(got.Summary), "waiting")
	assert.Contains(t, got.Summary, "12")
}

func TestGroundOverviewSalesIfNeeded_LeavesHonestSetupAlone(t *testing.T) {
	s := &DirectorConsoleService{}
	payload := &directorContext{}
	empty := DirectorStructuredResponse{
		Summary:   "Your business is just getting set up.",
		Diagnosis: "Connect payments to start taking money.",
	}
	got := s.groundOverviewSalesIfNeeded("how is setup going", "en", 1, empty, payload)
	assert.Equal(t, empty.Summary, got.Summary)
}

func TestDirectorWantsTonightSales_DoesNotMatchBestSellerOrTableWait(t *testing.T) {
	assert.True(t, directorWantsTonightSales("how much did we sell tonight"))
	assert.True(t, directorWantsTonightSales("cuánto vendimos esta noche"))
	assert.False(t, directorWantsTonightSales("what's our best seller"))
	assert.False(t, directorWantsTonightSales("qué vendimos más"))
	assert.False(t, directorWantsTonightSales("why is table 4 waiting"))
	assert.True(t, directorWantsTableWait("why is table 4 waiting"))
	assert.False(t, directorWantsTableWait("how much did we sell tonight"))
}

func TestUniqueDirectorThreadDTOs_OneRowPerId(t *testing.T) {
	older := time.Now().Add(-time.Hour)
	newer := time.Now()
	in := []DirectorThreadDTO{
		{ID: 1, Title: "what's our best seller", UpdatedAt: older},
		{ID: 2, Title: "cierra la caja", UpdatedAt: older},
		{ID: 1, Title: "what's our best seller", UpdatedAt: older},
		{ID: 3, Title: "what's our best seller", UpdatedAt: newer},
		{ID: 4, Title: "cierra la caja", UpdatedAt: newer},
		{ID: 5, Title: "mandá un mozo", UpdatedAt: newer},
	}
	got := uniqueDirectorThreadDTOs(in)
	require.Len(t, got, 5)
	ids := make([]uint, 0, len(got))
	for _, trow := range got {
		ids = append(ids, trow.ID)
	}
	assert.Equal(t, []uint{1, 2, 3, 4, 5}, ids)
}

func TestUniqueDirectorThreadDTOs_SecondMoneyNightKeepsBothIds(t *testing.T) {
	older := time.Now().Add(-24 * time.Hour)
	newer := time.Now()
	in := []DirectorThreadDTO{
		{ID: 10, Title: "how much did we sell tonight", UpdatedAt: older},
		{ID: 11, Title: "how much did we sell tonight", UpdatedAt: newer},
	}
	got := uniqueDirectorThreadDTOs(in)
	require.Len(t, got, 2, "title-collapse would hide the second money night")
	assert.Equal(t, []uint{10, 11}, []uint{got[0].ID, got[1].ID})

	// ListThreadsPaged.Total is the database count, not len after a title pass.
	result := ListThreadsResult{Threads: got, Total: 2}
	assert.Equal(t, int64(2), result.Total)
	assert.Len(t, result.Threads, 2)
}

func overviewSalesPayload() *directorContext {
	payload := &directorContext{}
	payload.Metrics.TodayRevenue = 18.04
	payload.Metrics.TodayCollected = 0
	payload.Metrics.TodayFloorRemaining = 18.04
	payload.Metrics.ActiveBills = 1
	payload.DataReadiness.PaymentsEnabled = true
	payload.DataReadiness.State = "established"
	return payload
}
