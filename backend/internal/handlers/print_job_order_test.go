package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// L3-35: the queue must come back newest-first with a deterministic tiebreak.
//
// Print jobs are enqueued in bursts (one order fans out to kitchen + bar + bill
// tickets in the same millisecond), so created_at alone leaves the order up to
// whatever the storage engine feels like returning: the history list reshuffles
// between polls and the operator cannot tell which ticket actually printed
// last. id DESC breaks the tie, and it is the same leading column as the
// created_at index so the sort stays index-friendly.
//
// This exercises the real handler against a real DB — the previous version of
// this test grepped print_job_handlers.go for the ORDER BY literal, which
// passes for any string that happens to contain those bytes and proves nothing
// about the rows that come back.
func TestListPrintJobs_OrderByCreatedAtAndID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	h := NewPrintJobHandlers(db)

	base := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	// Three jobs share the newest timestamp (one order fanning out) and one is
	// older. Insert deliberately out of order so a missing ORDER BY cannot
	// accidentally look correct.
	seed := []struct {
		name string
		at   time.Time
	}{
		{"tie-b", base},
		{"older", base.Add(-2 * time.Minute)},
		{"tie-c", base},
		{"tie-a", base},
	}
	ids := map[string]uint{}
	for i, s := range seed {
		job := database.PrintJob{
			BusinessID: business.ID,
			Kind:       database.PrintJobKindBill,
			Status:     database.PrintJobStatusPending,
			SourceType: "bill",
			SourceID:   uint(i + 1),
			CreatedAt:  s.at,
		}
		require.NoError(t, db.Create(&job).Error)
		// GORM autoCreateTime overwrites CreatedAt on insert; force the
		// timestamp we actually want to sort on.
		require.NoError(t, db.Model(&database.PrintJob{}).
			Where("id = ?", job.ID).
			UpdateColumn("created_at", s.at).Error)
		ids[s.name] = job.ID
	}

	router := gin.New()
	router.GET("/businesses/:id/print-jobs", h.ListJobs)

	req := httptest.NewRequest(http.MethodGet,
		"/businesses/"+strconv.FormatUint(uint64(business.ID), 10)+"/print-jobs", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var out struct {
		Items []database.PrintJob `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	require.Len(t, out.Items, 4)

	got := make([]uint, 0, len(out.Items))
	for _, item := range out.Items {
		got = append(got, item.ID)
	}
	// Newest first; within the tie, highest id first; oldest last.
	want := []uint{ids["tie-a"], ids["tie-c"], ids["tie-b"], ids["older"]}
	require.Equal(t, want, got)
}

// BenchmarkListPrintJobs measures the Recent Print Jobs poll (dashboard route,
// Backend Performance Gate). The L3-35 tie-break adds `id DESC` after the
// existing `created_at DESC`; `id` is the primary key, so the sort stays
// index-friendly and this exists to prove the added column is not a cost.
func BenchmarkListPrintJobs(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(b)
	base := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 500; i++ {
		job := database.PrintJob{
			BusinessID: business.ID,
			Kind:       database.PrintJobKindBill,
			Status:     database.PrintJobStatusPending,
			SourceType: "bill",
			SourceID:   uint(i + 1),
			// Bursts of 5 share a timestamp so the tie-break actually engages.
			CreatedAt: base.Add(time.Duration(i/5) * time.Second),
		}
		require.NoError(b, db.Create(&job).Error)
	}

	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.GET("/businesses/:id/print-jobs", h.ListJobs)
	url := "/businesses/" + strconv.FormatUint(uint64(business.ID), 10) + "/print-jobs"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, url, nil))
		if rr.Code != http.StatusOK {
			b.Fatalf("status %d: %s", rr.Code, rr.Body.String())
		}
	}
}
