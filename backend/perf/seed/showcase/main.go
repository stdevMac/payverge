// Package main is the showcase seed CLI.
//
// Where perf-seed (../) produces deterministic, gibberish-named fixtures for
// benchmarks and load tests, showcase-seed produces a single hand-curated
// restaurant ("Trattoria Bella Vista") with realistic data across every
// surface that screenshots the marketing site and FirstRunWizard need:
//
//   - Rich business profile (address, hours, AI settings, design)
//   - Italian menu — 6 categories, ~34 named items with descriptions,
//     allergens, dietary tags, varied prices
//   - 19 named tables (Patio / Window / Bar / Booth / Private Dining)
//   - 9 named staff across all roles (PINs bcrypted to "showcase")
//   - Loyalty program with 4 tiers
//   - Reservation settings + 12 reservations (past + upcoming, varied parties)
//   - 15 CRM customers with visits, points, tags
//   - 6 active bills/orders (kitchen-display ready) + 80 historical bills
//   - 3 promotional offers
//
// Idempotency: every entity uses a natural key prefixed with `showcase-` so
// re-runs are no-ops on existing rows. To regenerate from scratch, clear the
// business via:
//
//	DELETE FROM businesses WHERE business_id = 'showcase-bellavista';
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
	quiet            = flag.Bool("quiet", false, "Suppress GORM info logging")
	ownerEmailFlag   = flag.String("owner-email", "", "Attach the showcase business to the user with this email (must already exist; falls back to placeholder owner if not found)")
	refreshBillsFlag = flag.Bool("refresh-bills", false, "Delete the existing showcase bills/payments/orders before seeding so they are re-dated to the current 90-day window (bills are keyed on bill_number, so a plain re-run is otherwise a no-op and dates go stale)")
	resetAllFlag     = flag.Bool("reset-all", false, "Delete the entire showcase business (cascade) before seeding, for a fully clean slate")
)

// PlaceholderAddress is reused for OwnerAddress / Settlement / Tipping /
// Payer across the showcase fixture. A single recognisable value keeps the
// row easy to spot in the DB and avoids accidental dependency on per-row
// wallet addresses.
const PlaceholderAddress = "0xSHOWCASE000000000000000000000000000000000"

// BusinessSeedID is the natural key for the showcase business — `business_id`
// column on the `businesses` table.
const BusinessSeedID = "showcase-bellavista"

func main() {
	flag.Parse()

	dsn := *dsnFlag
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		log.Fatal("DSN required: pass --dsn or set DATABASE_URL")
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

	start := time.Now()
	if err := seedAll(ctx, db); err != nil {
		log.Fatalf("seed: %v", err)
	}
	fmt.Printf("showcase seed complete in %s\n", time.Since(start).Round(time.Millisecond))
}

func seedAll(ctx context.Context, db *gorm.DB) error {
	// --reset-all wipes the whole business first so the run rebuilds every
	// surface from scratch (cascades cover most child rows). Must happen
	// before seedBusiness re-creates the row.
	if *resetAllFlag {
		if err := resetShowcaseBusiness(ctx, db); err != nil {
			return fmt.Errorf("reset-all: %w", err)
		}
		log.Printf("reset-all: deleted existing '%s' business (cascade)", BusinessSeedID)
	}
	if err := seedBusiness(ctx, db); err != nil {
		return fmt.Errorf("business: %w", err)
	}
	biz, err := loadBusiness(ctx, db)
	if err != nil {
		return fmt.Errorf("load business: %w", err)
	}
	if *ownerEmailFlag != "" {
		if err := attachOwner(ctx, db, biz.ID, *ownerEmailFlag); err != nil {
			return fmt.Errorf("attach owner: %w", err)
		}
	}
	if err := seedOperatingHours(ctx, db, biz.ID); err != nil {
		return fmt.Errorf("hours: %w", err)
	}
	if err := seedGallery(ctx, db, biz.ID); err != nil {
		return fmt.Errorf("gallery: %w", err)
	}
	if err := seedSpecialFeatures(ctx, db, biz.ID); err != nil {
		return fmt.Errorf("special features: %w", err)
	}
	if err := seedTables(ctx, db, biz.ID); err != nil {
		return fmt.Errorf("tables: %w", err)
	}
	if err := seedMenu(ctx, db, biz.ID); err != nil {
		return fmt.Errorf("menu: %w", err)
	}
	if err := seedStaff(ctx, db, biz.ID); err != nil {
		return fmt.Errorf("staff: %w", err)
	}
	if err := seedLoyalty(ctx, db, biz.ID); err != nil {
		return fmt.Errorf("loyalty: %w", err)
	}
	if err := seedReservations(ctx, db, biz.ID); err != nil {
		return fmt.Errorf("reservations: %w", err)
	}
	if err := seedCustomers(ctx, db, biz.ID); err != nil {
		return fmt.Errorf("customers: %w", err)
	}
	if err := seedOffers(ctx, db, biz.ID); err != nil {
		return fmt.Errorf("offers: %w", err)
	}
	// --refresh-bills clears the existing BV-* bills (and their payments +
	// orders) so seedHistoricalBills/seedActiveBills below recreate them dated
	// to *this* run's 90-day window. Without it, the bill_number OnConflict
	// DoNothing makes a re-run a no-op and the dates go stale. Runs after the
	// curated surfaces above so the profile/menu/staff/customers stay intact.
	if *refreshBillsFlag {
		if err := refreshShowcaseBills(ctx, db, biz.ID); err != nil {
			return fmt.Errorf("refresh-bills: %w", err)
		}
	}
	if err := seedHistoricalBills(ctx, db, biz.ID); err != nil {
		return fmt.Errorf("historical bills: %w", err)
	}
	if err := seedActiveBills(ctx, db, biz.ID); err != nil {
		return fmt.Errorf("active bills: %w", err)
	}
	printSeedSummary(ctx, db, biz.ID)
	return nil
}
