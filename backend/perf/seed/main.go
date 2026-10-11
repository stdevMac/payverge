// Package main is the perf-fixture seeding CLI.
//
// It idempotently inserts a fixed-size cohort of businesses (each with a menu,
// staff roster, tables, and historical bills/payments/orders) into Postgres so
// that benchmarks (Tasks 7-8) and k6 load tests (Tasks 10-11) have realistic
// data to operate against.
//
// Idempotency is achieved via natural keys:
//   - Business: business_id = "perf-seed-NNN"
//   - Staff:    email       = "perf-seed-staff-NNN-M@example.test"
//   - Table:    table_code  = "perf-seed-NNN-tMM"
//   - Bill:     bill_number = "PERF-NNN-PPPPP"
//   - Menu:     FirstOrCreate by business_id (one menu row per perf business)
//   - Order:    composite (bill_id, created_by, client_request_id)
//
// Re-running the CLI with the same flags is a no-op except for newly added
// rows (e.g. bumping --orders-history fills in the gaps).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var (
	dsnFlag          = flag.String("dsn", "", "Postgres DSN (overrides DATABASE_URL)")
	businesses       = flag.Int("businesses", 20, "Number of businesses to seed")
	menuItemsPerBiz  = flag.Int("products-per-business", 50, "Menu items per business (live in Menu.Categories JSON)")
	staffPerBiz      = flag.Int("staff-per-business", 5, "Staff users per business")
	historicalOrders = flag.Int("orders-history", 1000, "Historical orders per business")
	quiet            = flag.Bool("quiet", false, "Suppress GORM info logging")
)

// PlaceholderAddress is reused for OwnerAddress / Settlement / Tipping / Payer
// across every seeded record. Using a single value keeps fixtures recognisable
// in the DB and avoids accidental dependence on per-row addresses.
const PlaceholderAddress = "0xPERFSEED0000000000000000000000000000000"

func main() {
	flag.Parse()
	dsn := *dsnFlag
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		log.Fatal("DSN required: --dsn or DATABASE_URL")
	}

	cfg := &gorm.Config{}
	if *quiet {
		cfg.Logger = gormlogger.Default.LogMode(gormlogger.Silent)
	} else {
		cfg.Logger = gormlogger.Default.LogMode(gormlogger.Warn)
	}

	db, err := gorm.Open(postgres.Open(dsn), cfg)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	ctx := context.Background()

	opts := seedOpts{
		Businesses:       *businesses,
		MenuItemsPerBiz:  *menuItemsPerBiz,
		StaffPerBiz:      *staffPerBiz,
		HistoricalOrders: *historicalOrders,
	}

	start := time.Now()
	if err := seedAll(ctx, db, opts); err != nil {
		log.Fatalf("seed: %v", err)
	}
	fmt.Printf("perf seed complete in %s: %d businesses, %d menus, %d staff, %d historical orders (target)\n",
		time.Since(start).Round(time.Millisecond),
		opts.Businesses,
		opts.Businesses,
		opts.Businesses*opts.StaffPerBiz,
		opts.Businesses*opts.HistoricalOrders)
}

type seedOpts struct {
	Businesses       int
	MenuItemsPerBiz  int
	StaffPerBiz      int
	HistoricalOrders int
}

func seedAll(ctx context.Context, db *gorm.DB, opts seedOpts) error {
	if err := seedBusinesses(ctx, db, opts.Businesses); err != nil {
		return fmt.Errorf("businesses: %w", err)
	}
	if err := seedTables(ctx, db, opts.Businesses); err != nil {
		return fmt.Errorf("tables: %w", err)
	}
	if err := seedMenus(ctx, db, opts.Businesses, opts.MenuItemsPerBiz); err != nil {
		return fmt.Errorf("menus: %w", err)
	}
	if err := seedStaff(ctx, db, opts.Businesses, opts.StaffPerBiz); err != nil {
		return fmt.Errorf("staff: %w", err)
	}
	if err := seedHistoricalOrders(ctx, db, opts.Businesses, opts.HistoricalOrders, opts.MenuItemsPerBiz); err != nil {
		return fmt.Errorf("orders: %w", err)
	}
	if err := seedActiveFixtures(ctx, db); err != nil {
		return fmt.Errorf("active fixtures: %w", err)
	}
	return nil
}
