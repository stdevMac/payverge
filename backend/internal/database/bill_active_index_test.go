package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCreateBill_ActiveBillUniquePerTable locks in the duplicate-bill race fix.
// The partial unique index guarantees a table can hold at
// most one open/partial bill, so a concurrent second create is rejected with
// ErrActiveBillExists instead of forking the table across two bills.
//
// GORM AutoMigrate does not create the partial index (it is SQL-only), so the
// test mirrors the migration DDL on the in-memory SQLite DB. SQLite supports
// partial indexes with the same WHERE syntax as Postgres.
func TestCreateBill_ActiveBillUniquePerTable(t *testing.T) {
	setupTestDB(t)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX idx_bills_active_per_table ON bills (table_id) `+
			`WHERE table_id IS NOT NULL AND table_id <> 0 AND status IN ('open','partial')`,
	).Error)

	first := &Bill{BusinessID: 1, TableID: 1, BillNumber: "B1-aaaaaaaaaaaa", Status: BillStatusOpen}
	require.NoError(t, CreateBill(first, nil))

	// Second active bill for the SAME table loses the race.
	second := &Bill{BusinessID: 1, TableID: 1, BillNumber: "B1-bbbbbbbbbbbb", Status: BillStatusOpen}
	require.ErrorIs(t, CreateBill(second, nil), ErrActiveBillExists)

	// A DIFFERENT table is unaffected.
	require.NoError(t, CreateBill(
		&Bill{BusinessID: 1, TableID: 2, BillNumber: "B1-cccccccccccc", Status: BillStatusOpen}, nil,
	))

	// Bills with no table (delivery/reservation, table_id = 0) are never constrained.
	require.NoError(t, CreateBill(
		&Bill{BusinessID: 1, TableID: 0, BillNumber: "B1-dddddddddddd", Status: BillStatusOpen}, nil,
	))
	require.NoError(t, CreateBill(
		&Bill{BusinessID: 1, TableID: 0, BillNumber: "B1-eeeeeeeeeeee", Status: BillStatusOpen}, nil,
	))
}
