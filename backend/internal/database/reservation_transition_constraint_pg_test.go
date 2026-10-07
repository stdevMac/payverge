//go:build integration

package database

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// PostgreSQL integration: reservation lifecycle against the REAL
// businesses_has_canonical_owner CHECK in the genesis schema.
// Requires TEST_DATABASE_URL (e.g. postgres://test:test@localhost:5432/test?sslmode=disable).
//
// Reservation aggregates are read with a partial-column Business preload (no
// owner_address, no user_id). GORM's Save persists loaded associations by
// default, and PostgreSQL evaluates CHECK constraints on the proposed row
// BEFORE ON CONFLICT arbitration — so the owner-less association INSERT aborts
// the whole transition transaction with SQLSTATE 23514 unless the reservation
// write funnels omit associations. This reproduces the production incident that
// broke every reservation transition, edit, and guest cancel.
func TestReservationLifecycleSurvivesCanonicalOwnerConstraintPG(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)

	for _, stmt := range []string{
		`DROP TABLE IF EXISTS reservation_status_histories CASCADE`,
		`DROP TABLE IF EXISTS table_reservations CASCADE`,
		`DROP TABLE IF EXISTS tables CASCADE`,
		`DROP TABLE IF EXISTS businesses CASCADE`,
	} {
		require.NoError(t, gdb.Exec(stmt).Error)
	}
	require.NoError(t, gdb.AutoMigrate(&Business{}, &Table{}, &TableReservation{}, &ReservationStatusHistory{}))

	// The exact constraint DDL shipped by the genesis schema.
	require.NoError(t, gdb.Exec(`
		ALTER TABLE businesses
		    ADD CONSTRAINT businesses_has_canonical_owner
		    CHECK (
		        owner_address ~ '[^[:space:]]'
		        OR user_id IS NOT NULL
		    ) NOT VALID`).Error)

	prevDB := db
	db = gdb
	t.Cleanup(func() {
		db = prevDB
		_ = sqlDB.Close()
	})

	biz := &Business{
		Name:         "Constraint Resto",
		BusinessId:   fmt.Sprintf("pg9-%d", time.Now().UnixNano()),
		OwnerAddress: "0xpg9canonicalowner",
	}
	require.NoError(t, gdb.Create(biz).Error)
	table := &Table{
		BusinessID: biz.ID,
		TableCode:  fmt.Sprintf("pg9-table-%d", time.Now().UnixNano()),
		Name:       "T1",
		Capacity:   4,
	}
	require.NoError(t, gdb.Create(table).Error)

	makeReservation := func(code string) *TableReservation {
		res := &TableReservation{
			BusinessID:       biz.ID,
			TableID:          &table.ID,
			CustomerName:     "PG Guest",
			CustomerEmail:    "pg9@example.com",
			PartySize:        2,
			ReservationTime:  time.Now().Add(3 * time.Hour),
			Duration:         60,
			Status:           "pending",
			Source:           "customer",
			ConfirmationCode: code,
		}
		require.NoError(t, gdb.Create(res).Error)
		return res
	}

	// Full happy path pending → confirmed → seated → completed, re-loading
	// through the real handler read shape (partial Business preload) each hop —
	// exactly what the transition handlers do.
	res := makeReservation(fmt.Sprintf("PG9-LIFE-%d", time.Now().UnixNano()))
	now := time.Now()
	for _, hop := range []struct {
		from, to string
		stamp    func(r *TableReservation)
	}{
		{"pending", "confirmed", func(r *TableReservation) { r.ConfirmedAt = &now }},
		{"confirmed", "seated", func(r *TableReservation) { r.SeatedAt = &now }},
		{"seated", "completed", func(r *TableReservation) { r.CompletedAt = &now }},
	} {
		loaded, err := GetReservationByBusinessAndID(biz.ID, res.ID)
		require.NoError(t, err)
		require.NotZero(t, loaded.Business.ID, "read shape must preload Business")
		loaded.Status = hop.to
		hop.stamp(loaded)
		require.NoErrorf(t, gdb.Transaction(func(tx *gorm.DB) error {
			return UpdateReservationStatusGuardedTx(tx, loaded, hop.from, 0, ReservationWriteGuards{})
		}), "transition %s→%s must survive businesses_has_canonical_owner", hop.from, hop.to)
	}

	// Cancel path (operator cancel and guest cancel-by-code both end here).
	res2 := makeReservation(fmt.Sprintf("PG9-CANCEL-%d", time.Now().UnixNano()))
	loaded2, err := GetReservationByBusinessAndID(biz.ID, res2.ID)
	require.NoError(t, err)
	loaded2.Status = "cancelled"
	loaded2.CancelledAt = &now
	loaded2.CancelledBy = "customer"
	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		return UpdateReservationStatusGuardedTx(tx, loaded2, "pending", 0, ReservationWriteGuards{})
	}), "cancel must survive businesses_has_canonical_owner")

	// Edit path (PUT /reservations/:id).
	res3 := makeReservation(fmt.Sprintf("PG9-EDIT-%d", time.Now().UnixNano()))
	loaded3, err := GetReservationByBusinessAndID(biz.ID, res3.ID)
	require.NoError(t, err)
	loaded3.SpecialRequests = "window seat"
	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		return UpdateReservationTx(tx, loaded3, 0)
	}), "edit must survive businesses_has_canonical_owner")

	// The business row itself must be untouched by all of the above.
	var reloaded Business
	require.NoError(t, gdb.First(&reloaded, biz.ID).Error)
	require.Equal(t, biz.OwnerAddress, reloaded.OwnerAddress)
}
