package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// TestParseDateRange_RejectsInvertedRange guards the start<=end check added to
// the shared accounting date-range parser: an inverted range must 400 with a
// clear message rather than silently returning an empty result set.
func TestParseDateRange_RejectsInvertedRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	biz := &database.Business{Timezone: "UTC"}

	cases := []struct {
		name     string
		start    string
		end      string
		wantOK   bool
		wantCode int
	}{
		{"valid range", "2026-01-01", "2026-01-31", true, http.StatusOK},
		{"same day ok", "2026-01-10", "2026-01-10", true, http.StatusOK},
		{"inverted rejected", "2026-02-01", "2026-01-01", false, http.StatusBadRequest},
		{"missing end rejected", "2026-01-01", "", false, http.StatusBadRequest},
		{"bad format rejected", "01/01/2026", "2026-01-31", false, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("GET", "/?start="+tc.start+"&end="+tc.end, nil)

			_, _, ok := parseDateRange(c, biz)
			if ok != tc.wantOK {
				t.Fatalf("parseDateRange(start=%q,end=%q) ok=%v, want %v", tc.start, tc.end, ok, tc.wantOK)
			}
			if !tc.wantOK && rec.Code != tc.wantCode {
				t.Fatalf("expected status %d on rejection, got %d", tc.wantCode, rec.Code)
			}
		})
	}
}

// TestParseDateRange_RejectsUnboundedWindow caps an explicit start/end span at
// 400 calendar days. 2025-01-01..2026-02-05 is exactly 400 days apart and is
// allowed; 2025-01-01..2026-02-06 is 401 days apart and is rejected. Period
// presets do not pass through this check.
func TestParseDateRange_RejectsUnboundedWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	biz := &database.Business{Timezone: "UTC"}

	cases := []struct {
		name   string
		start  string
		end    string
		wantOK bool
	}{
		{"400 day span allowed", "2025-01-01", "2026-02-05", true},
		{"401 day span rejected", "2025-01-01", "2026-02-06", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("GET", "/?start="+tc.start+"&end="+tc.end, nil)

			_, _, ok := parseDateRange(c, biz)
			if ok != tc.wantOK {
				t.Fatalf("parseDateRange(start=%q,end=%q) ok=%v, want %v (body %s)", tc.start, tc.end, ok, tc.wantOK, rec.Body.String())
			}
			if tc.wantOK {
				return
			}
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d on rejection, got %d", http.StatusBadRequest, rec.Code)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error body: %v (%s)", err, rec.Body.String())
			}
			if body["code"] != server.ErrCodeInvalidDateRange {
				t.Fatalf("code=%v, want %s (body %s)", body["code"], server.ErrCodeInvalidDateRange, rec.Body.String())
			}
		})
	}
}
