package fiscal

import (
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestListIssuableBillsAccessShape is the dinner-service picker access-shape
// gate: one projected Scan (no SELECT *, no per-row receipt lookup), Limit
// clamped, status literals inlined, and the broader paid/invoiced predicate.
func TestListIssuableBillsAccessShape(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	const businessID uint = 77
	closed := time.Now().UTC().Add(-15 * time.Minute)

	require.NoError(t, db.Create(&database.Business{
		ID: businessID, BusinessId: "biz-issuable-shape", Name: "Shape Cafe",
		OwnerAddress: "0xshape", SettlementAddr: "settle", TippingAddr: "tip",
		DefaultCurrency: "ARS",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 701, BusinessID: businessID, BillNumber: "SHAPE-PAID",
		Status: database.BillStatusPaid, Items: "[]",
		TotalAmount: 1500, PaidAmount: 1500, ClosedAt: &closed,
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 702, BusinessID: businessID, BillNumber: "SHAPE-OPEN-PAID",
		Status: database.BillStatusOpen, Items: "[]",
		TotalAmount: 2200, PaidAmount: 2200, ClosedAt: &closed,
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 703, BusinessID: businessID, BillNumber: "SHAPE-INVOICED",
		Status: database.BillStatusClosed, Items: "[]",
		TotalAmount: 1800, PaidAmount: 1800, ClosedAt: &closed,
	}).Error)
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 80, BusinessID: businessID, SettingsID: 1, BillID: 703,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "B", TotalAmountCents: 1800, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 704, BusinessID: businessID, BillNumber: "SHAPE-NOISE",
		Status: database.BillStatusOpen, Items: "[]",
		TotalAmount: 900, PaidAmount: 0,
	}).Error)

	var sqls []string
	capture := func(tx *gorm.DB) {
		if sql := strings.TrimSpace(tx.Statement.SQL.String()); sql != "" {
			sqls = append(sqls, sql)
		}
	}
	// Scan into a struct slice can land on Query or Row depending on dialect.
	const queryCB = "payverge:test_list_issuable_sql_query"
	const rowCB = "payverge:test_list_issuable_sql_row"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(queryCB, capture))
	require.NoError(t, db.Callback().Row().After("gorm:row").Register(rowCB, capture))
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(queryCB)
		_ = db.Callback().Row().Remove(rowCB)
	})

	got, err := repo.ListIssuableBills(businessID, "", 20)
	require.NoError(t, err)
	require.Len(t, got, 3)

	require.GreaterOrEqual(t, len(sqls), 1)
	require.LessOrEqual(t, len(sqls), 2, "picker is two bounded scans, not a per-row receipt lookup")
	joined := strings.ToLower(strings.Join(sqls, "\n"))
	require.NotContains(t, joined, "select *", "must project issuable columns")
	require.Contains(t, joined, "limit", "picker must stay bounded")
	require.Contains(t, joined, "paid_amount", "just-paid open bills stay in the predicate")
	require.Contains(t, joined, "fiscal_receipts", "already-invoiced bills stay discoverable")
	// Drift gate (#907): the picker inlines these literals while the
	// manual-issue guard reads blockingIssueReceiptStatuses. Assert against the
	// slice itself so adding a blocking status in Go without touching the
	// subquery fails here instead of silently letting a flagged bill be
	// re-issued.
	for _, status := range blockingIssueReceiptStatuses {
		require.Contains(t, joined, string(status),
			"blocking statuses must be inlined literals, in sync with blockingIssueReceiptStatuses")
	}
	require.NotRegexp(t, `(?i)status in \(\?`, joined, "GORM IN ? inside the receipt subquery 500'd the picker")
	for _, sql := range sqls {
		require.Contains(t, strings.ToLower(sql), "limit")
		require.NotContains(t, strings.ToLower(sql), "select *")
	}
}
