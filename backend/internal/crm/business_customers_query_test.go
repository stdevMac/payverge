package crm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// connectCustomerNamed creates a customer with a specific display name plus an
// active connection to the business, so query tests can assert on name/tier/
// spend/visit ordering. Returns the connection id.
func connectCustomerNamed(
	t *testing.T,
	db *gorm.DB,
	businessID uint,
	name, email, tier string,
	spend float64,
	visits int,
	lastVisit *time.Time,
) uint {
	t.Helper()
	customer := &database.Customer{Email: email, PasswordHash: "hash", Name: name, IsActive: true}
	require.NoError(t, db.Create(customer).Error)
	conn := &database.CustomerBusiness{
		CustomerID:   customer.ID,
		BusinessID:   businessID,
		LoyaltyTier:  tier,
		TotalSpent:   spend,
		VisitCount:   visits,
		LastVisitAt:  lastVisit,
		FirstVisitAt: time.Now().Add(-10 * 24 * time.Hour),
		IsActive:     true,
	}
	require.NoError(t, db.Create(conn).Error)
	return conn.ID
}

// TestGetBusinessCustomers_SearchIsCaseInsensitive is the RED-first regression
// for fix 5: the search was a case-sensitive LIKE so "acme" would miss "ACME
// Corp". It must match regardless of case (ILIKE semantics), keeping the leading
// wildcard so substrings match.
func TestGetBusinessCustomers_SearchIsCaseInsensitive(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	service := NewService(db)
	business := createCRMTestBusiness(t, db, "biz-search", "0xSearchOwner", "Search Biz")

	now := time.Now()
	connectCustomerNamed(t, db, business.ID, "ACME Corp", "acme@example.com", "Gold", 500, 5, &now)
	connectCustomerNamed(t, db, business.ID, "Beta LLC", "beta@example.com", "Bronze", 50, 1, &now)

	// Lowercase query must still match the mixed-case name.
	rows, total, err := service.GetBusinessCustomers(business.ID, 1, 20, "acme", "")
	require.NoError(t, err)
	assert.EqualValues(t, 1, total, "lowercase search must match the mixed-case name")
	require.Len(t, rows, 1)
	assert.Equal(t, "ACME Corp", rows[0].Customer.Name)

	// Uppercase query on an email substring must also match.
	rows, total, err = service.GetBusinessCustomers(business.ID, 1, 20, "BETA", "")
	require.NoError(t, err)
	assert.EqualValues(t, 1, total, "uppercase search must match the lowercase email")
	require.Len(t, rows, 1)
	assert.Equal(t, "Beta LLC", rows[0].Customer.Name)
}

// TestGetBusinessCustomers_ServerSort is the RED-first regression for fix 6: the
// header sort only reordered the visible 20-row page. The list must sort
// server-side over the whole base by whitelisted columns, so page 1 truly holds
// the top rows.
func TestGetBusinessCustomers_ServerSort(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	service := NewService(db)
	business := createCRMTestBusiness(t, db, "biz-sort", "0xSortOwner", "Sort Biz")

	now := time.Now()
	// Insert in a deliberately non-sorted order.
	connectCustomerNamed(t, db, business.ID, "Charlie", "c@example.com", "Bronze", 300, 3, &now)
	connectCustomerNamed(t, db, business.ID, "Alice", "a@example.com", "Gold", 100, 9, &now)
	connectCustomerNamed(t, db, business.ID, "Bob", "b@example.com", "Silver", 900, 1, &now)

	// total_spent DESC → Bob (900), Charlie (300), Alice (100).
	rows, _, err := service.GetBusinessCustomersSorted(business.ID, 1, 20, "", "", "total_spent", "desc")
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, "Bob", rows[0].Customer.Name)
	assert.Equal(t, "Charlie", rows[1].Customer.Name)
	assert.Equal(t, "Alice", rows[2].Customer.Name)

	// visit_count ASC → Bob (1), Charlie (3), Alice (9).
	rows, _, err = service.GetBusinessCustomersSorted(business.ID, 1, 20, "", "", "visit_count", "asc")
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, "Bob", rows[0].Customer.Name)
	assert.Equal(t, "Charlie", rows[1].Customer.Name)
	assert.Equal(t, "Alice", rows[2].Customer.Name)

	// name ASC (joins customers) → Alice, Bob, Charlie.
	rows, _, err = service.GetBusinessCustomersSorted(business.ID, 1, 20, "", "", "name", "asc")
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, "Alice", rows[0].Customer.Name)
	assert.Equal(t, "Bob", rows[1].Customer.Name)
	assert.Equal(t, "Charlie", rows[2].Customer.Name)
}

// TestGetBusinessCustomers_UnknownSortFallsBackSafely asserts an unrecognized
// sort column does NOT open a SQL-injection surface and falls back to the
// default ordering.
func TestGetBusinessCustomers_UnknownSortFallsBackSafely(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	service := NewService(db)
	business := createCRMTestBusiness(t, db, "biz-badsort", "0xBadSortOwner", "Bad Sort Biz")

	now := time.Now()
	connectCustomerNamed(t, db, business.ID, "Zed", "z@example.com", "Bronze", 10, 1, &now)

	// A hostile column must not error or inject — it falls back to the default.
	rows, total, err := service.GetBusinessCustomersSorted(
		business.ID, 1, 20, "", "",
		"total_spent; DROP TABLE customer_businesses;--", "desc",
	)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, rows, 1)

	// Table must still exist (the injection must not have run).
	var count int64
	require.NoError(t, db.Model(&database.CustomerBusiness{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

// TestGetBusinessCustomers_SegmentFilter is the RED-first regression for fix 7:
// the list must accept an optional segment param implementing the SAME
// predicates as GetSegments (lapsed/vip/new/at-risk).
func TestGetBusinessCustomers_SegmentFilter(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	service := NewService(db)
	business := createCRMTestBusiness(t, db, "biz-seg", "0xSegOwner", "Segment Biz")

	now := time.Now()
	lapsed := now.Add(-120 * 24 * time.Hour)
	atRisk := now.Add(-60 * 24 * time.Hour)
	recent := now.Add(-2 * 24 * time.Hour)

	// lapsed: last visit 120d ago.
	lapsedID := connectCustomerNamed(t, db, business.ID, "Lapsed One", "lapsed@example.com", "Bronze", 40, 2, &lapsed)
	// at-risk: last visit 60d ago.
	connectCustomerNamed(t, db, business.ID, "AtRisk One", "atrisk@example.com", "Bronze", 40, 2, &atRisk)
	// active recent, not lapsed/at-risk.
	connectCustomerNamed(t, db, business.ID, "Active One", "active@example.com", "Gold", 40, 3, &recent)

	rows, total, err := service.GetBusinessCustomersSegment(business.ID, 1, 20, "lapsed")
	require.NoError(t, err)
	assert.EqualValues(t, 1, total, "only the lapsed customer must match the lapsed segment")
	require.Len(t, rows, 1)
	assert.Equal(t, lapsedID, rows[0].ID)

	rows, total, err = service.GetBusinessCustomersSegment(business.ID, 1, 20, "at-risk")
	require.NoError(t, err)
	assert.EqualValues(t, 1, total, "only the at-risk customer must match the at-risk segment")
	require.Len(t, rows, 1)
	assert.Equal(t, "AtRisk One", rows[0].Customer.Name)
}

// TestSegmentPredicateMatchesGetSegmentsCounts proves the single-source-of-truth
// invariant (fix 7): the per-segment list filter (customerSegmentPredicate) must
// produce the same per-band counts as the GetSegments aggregate, over a mixed
// population that exercises every band. If the two ever diverge, a segment card
// would show a count that the drilled-in list can't reproduce.
func TestSegmentPredicateMatchesGetSegmentsCounts(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	service := NewService(db)
	business := createCRMTestBusiness(t, db, "biz-segparity", "0xSegParity", "Segment Parity Biz")
	handler := NewHandler(service)

	now := time.Now()
	lapsed := now.Add(-120 * 24 * time.Hour)
	atRisk := now.Add(-60 * 24 * time.Hour)
	recent := now.Add(-2 * 24 * time.Hour)

	// A spread that lands customers in each band. VIP needs visit_count>=5 and
	// above-average spend, so give a couple of high spenders many visits.
	mk := func(name string, spend float64, visits int, last, first time.Time) {
		customer := &database.Customer{Email: name + "@example.com", PasswordHash: "h", Name: name, IsActive: true}
		require.NoError(t, db.Create(customer).Error)
		require.NoError(t, db.Create(&database.CustomerBusiness{
			CustomerID:   customer.ID,
			BusinessID:   business.ID,
			TotalSpent:   spend,
			VisitCount:   visits,
			LastVisitAt:  &last,
			FirstVisitAt: first,
			IsActive:     true,
		}).Error)
	}
	old := now.Add(-200 * 24 * time.Hour)
	brandNew := now.Add(-5 * 24 * time.Hour)
	mk("Lapsed A", 40, 2, lapsed, old)
	mk("Lapsed B", 40, 2, lapsed, old)
	mk("AtRisk A", 40, 2, atRisk, old)
	mk("VIP A", 5000, 12, recent, old)
	mk("VIP B", 6000, 20, recent, old)
	mk("New A", 10, 1, brandNew, brandNew)
	mk("Regular A", 20, 2, recent, old)

	// Aggregate counts from GetSegments.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: business.BusinessId}}
	c.Request = httptest.NewRequest(http.MethodGet, "/businesses/"+business.BusinessId+"/crm/segments", nil)
	handler.GetSegments(c)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	var aggregate map[string]int
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &aggregate))

	for _, seg := range []string{"lapsed", "at-risk", "vip", "new"} {
		_, total, err := service.GetBusinessCustomersSegment(business.ID, 1, 100, seg)
		require.NoError(t, err)
		// GetSegments emits at-risk under the key "atRisk".
		key := seg
		if seg == "at-risk" {
			key = "atRisk"
		}
		assert.EqualValues(t, aggregate[key], total,
			"per-segment list count for %q must equal the GetSegments aggregate", seg)
	}
}

// BenchmarkGetBusinessCustomersSorted measures the sorted-page read (whole-base
// server sort) over 1,000 customers — the query shape header sort now uses.
func BenchmarkGetBusinessCustomersSorted(b *testing.B) {
	service, businessID := setupSummaryPerfDB(b, 1000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := service.GetBusinessCustomersSorted(businessID, 1, 20, "", "", "total_spent", "desc"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetBusinessCustomersSegment measures a segment-filtered page read
// over 1,000 customers — the query shape a Segments drilldown uses.
func BenchmarkGetBusinessCustomersSegment(b *testing.B) {
	service, businessID := setupSummaryPerfDB(b, 1000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := service.GetBusinessCustomersSegment(businessID, 1, 20, "lapsed"); err != nil {
			b.Fatal(err)
		}
	}
}
