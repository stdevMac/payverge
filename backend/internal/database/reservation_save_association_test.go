package database

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// reservationWritesAgainstTable returns the captured statements that INSERT or
// UPDATE the given table.
func reservationWritesAgainstTable(statements []string, table string) []string {
	var hits []string
	for _, statement := range statements {
		normalized := strings.ToLower(strings.TrimSpace(statement))
		for _, prefix := range []string{
			"insert into `" + table + "`",
			"insert into \"" + table + "\"",
			"insert into " + table,
			"update `" + table + "`",
			"update \"" + table + "\"",
			"update " + table,
		} {
			if strings.HasPrefix(normalized, prefix) {
				hits = append(hits, statement)
				break
			}
		}
	}
	return hits
}

// Reservation aggregates are read with a partial-column Business preload
// (reservationBusinessColumns — no owner_address, no user_id). GORM's Save
// persists loaded associations by default, so saving such an aggregate also
// emits an INSERT against businesses carrying an empty owner_address and NULL
// user_id. The businesses_has_canonical_owner CHECK rejects
// that row (SQLSTATE 23514) before ON CONFLICT arbitration, which breaks every
// reservation transition, edit, and guest cancel in production. The reservation
// write funnels must touch table_reservations only — never the preloaded
// Business/Table/StatusHistory rows.
func TestReservationSavesMustNotWriteAssociatedTables(t *testing.T) {
	funnels := []struct {
		name string
		save func(t *testing.T, loaded *TableReservation)
	}{
		{
			// Status-transition path: confirm/assign/seat/cancel/no-show all
			// funnel through UpdateReservationStatusGuardedTx.
			name: "status transition",
			save: func(t *testing.T, loaded *TableReservation) {
				prior := loaded.Status
				now := time.Now()
				loaded.Status = "confirmed"
				loaded.ConfirmedAt = &now
				require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
					return UpdateReservationStatusGuardedTx(tx, loaded, prior, 0, ReservationWriteGuards{})
				}))
			},
		},
		{
			// Operator edit path: PUT /reservations/:id funnels through
			// UpdateReservationTx under a transaction.
			name: "guarded edit",
			save: func(t *testing.T, loaded *TableReservation) {
				loaded.SpecialRequests = "window seat"
				require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
					return UpdateReservationTx(tx, loaded, 0)
				}))
			},
		},
		{
			name: "plain update",
			save: func(t *testing.T, loaded *TableReservation) {
				loaded.Notes = "updated by staff"
				require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
					return UpdateReservationTx(tx, loaded, 0)
				}))
			},
		},
	}

	for _, funnel := range funnels {
		t.Run(funnel.name, func(t *testing.T) {
			recorder := &reservationSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
			setupReservationPerfTestDB(t, recorder)
			require.NoError(t, db.AutoMigrate(&ReservationStatusHistory{}))

			biz := helperReservationPerfBusiness(t)
			table := &Table{
				BusinessID: biz.ID,
				TableCode:  fmt.Sprintf("assoc-%s-%d", funnel.name, time.Now().UnixNano()),
				Name:       "T1",
				Capacity:   4,
			}
			require.NoError(t, db.Create(table).Error)

			res := &TableReservation{
				BusinessID:       biz.ID,
				TableID:          &table.ID,
				CustomerName:     "Assoc Guest",
				CustomerEmail:    "assoc@example.com",
				PartySize:        2,
				ReservationTime:  time.Now().Add(3 * time.Hour),
				Duration:         60,
				Status:           "pending",
				Source:           "customer",
				ConfirmationCode: fmt.Sprintf("ASSOC-%s-%d", funnel.name, time.Now().UnixNano()),
			}
			require.NoError(t, db.Create(res).Error)
			require.NoError(t, db.Create(&ReservationStatusHistory{
				ReservationID: res.ID,
				Status:        "pending",
				ChangedBy:     "guest",
			}).Error)

			// Load through the real handler read shape so the partial Business
			// (and Table/StatusHistory) preloads are attached, exactly as in
			// the transition/edit handlers.
			loaded, err := GetReservationByBusinessAndID(biz.ID, res.ID)
			require.NoError(t, err)
			require.NotZero(t, loaded.Business.ID, "read shape must preload Business")
			require.Empty(t, loaded.Business.OwnerAddress,
				"precondition: the preload is a partial projection without owner_address")

			recorder.statements = nil
			funnel.save(t, loaded)

			for _, associated := range []string{"businesses", "tables", "reservation_status_histories"} {
				require.Emptyf(t, reservationWritesAgainstTable(recorder.statements, associated),
					"saving a reservation must not write %s — a partial-projection association "+
						"write violates businesses_has_canonical_owner in production", associated)
			}

			var reloadedBiz Business
			require.NoError(t, db.First(&reloadedBiz, biz.ID).Error)
			require.Equal(t, biz.OwnerAddress, reloadedBiz.OwnerAddress,
				"business owner must survive a reservation save untouched")
		})
	}
}
