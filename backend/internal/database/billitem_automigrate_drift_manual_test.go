package database

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestBillItemAutoMigrate_NoDestructiveDrift is a MANUAL diagnostic (guarded by
// PV_MIGTEST_HOST) that runs GORM AutoMigrate(&BillItem{}) against a table with
// the EXACT production shape (genesis bill_items, no order_id) and reports
// every column-type change: whether GORM AutoMigrate (used by SQLite test
// fixtures, never by startup) would rewrite the genesis decimal(10,2) money
// columns if pointed at a real bill_items table.
func TestBillItemAutoMigrate_NoDestructiveDrift(t *testing.T) {
	host := os.Getenv("PV_MIGTEST_HOST")
	if host == "" {
		t.Skip("set PV_MIGTEST_HOST/PORT/USER/PASS to run the manual drift diagnostic")
	}
	port := os.Getenv("PV_MIGTEST_PORT")
	user := os.Getenv("PV_MIGTEST_USER")
	pass := os.Getenv("PV_MIGTEST_PASS")

	adminDSN := fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=disable", user, pass, host, port)
	admin, err := sql.Open("postgres", adminDSN)
	require.NoError(t, err)
	defer func() { _ = admin.Close() }()
	_, _ = admin.Exec(`DROP DATABASE IF EXISTS pv_drift`)
	_, err = admin.Exec(`CREATE DATABASE pv_drift`)
	require.NoError(t, err)

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/pv_drift?sslmode=disable", user, pass, host, port)
	raw, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer func() { _ = raw.Close() }()

	// Exact prod shape: 000028 base + 000030 promo columns, WITHOUT order_id.
	_, err = raw.Exec(`
		CREATE TABLE bill_items (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			bill_id INTEGER NOT NULL,
			menu_item_id VARCHAR(255) NOT NULL DEFAULT '',
			name VARCHAR(255) NOT NULL,
			price DECIMAL(10,2) NOT NULL,
			quantity INTEGER NOT NULL,
			options JSONB,
			item_type VARCHAR(50) DEFAULT 'menu_item',
			bundle_id INTEGER,
			parent_bundle_id INTEGER,
			source_offer_id INTEGER,
			subtotal DECIMAL(10,2) NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`)
	require.NoError(t, err)

	colTypes := func() map[string]string {
		rows, err := raw.Query(`SELECT column_name, data_type, COALESCE(numeric_precision::text,''), COALESCE(numeric_scale::text,'') FROM information_schema.columns WHERE table_name='bill_items' ORDER BY column_name`)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()
		out := map[string]string{}
		for rows.Next() {
			var name, dt, prec, scale string
			require.NoError(t, rows.Scan(&name, &dt, &prec, &scale))
			out[name] = fmt.Sprintf("%s(%s,%s)", dt, prec, scale)
		}
		return out
	}

	before := colTypes()

	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gdb.AutoMigrate(&BillItem{}))

	after := colTypes()

	t.Logf("order_id present after AutoMigrate: %v", after["order_id"] != "")
	for col, b := range before {
		if a := after[col]; a != b {
			t.Logf("DRIFT: column %q changed %s -> %s", col, b, a)
		}
	}
	for col, a := range after {
		if _, existed := before[col]; !existed {
			t.Logf("ADDED: column %q = %s", col, a)
		}
	}
	require.NotEmpty(t, after["order_id"], "order_id must be added by AutoMigrate")
}
