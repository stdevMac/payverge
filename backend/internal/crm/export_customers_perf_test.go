package crm

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// BenchmarkExportCustomersCSV measures the full operator CSV export handler
// over 2 000 linked customers (plus 50 noise rows on another business).
func BenchmarkExportCustomersCSV(b *testing.B) {
	gin.SetMode(gin.TestMode)
	service, businessID := setupSummaryPerfDB(b, 2000)
	handler := NewHandler(service)
	id := strconv.FormatUint(uint64(businessID), 10)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Params = gin.Params{{Key: "id", Value: id}}
		handler.ExportCustomers(c)
		if w.Code != http.StatusOK {
			b.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		if lines := strings.Count(w.Body.String(), "\n"); lines != 2001 {
			b.Fatalf("expected 2001 CSV lines, got %d", lines)
		}
	}
}
