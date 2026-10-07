package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/require"
)

// On the public demo a guest must be able to pay at the counter before any
// operator has opened Caja: the boot-time seed opens every showroom drawer,
// and only showroom (kind=demo) venues get one.
func TestOpenPublicDemoDrawers(t *testing.T) {
	db := newCLITestDB(t, &database.Business{}, &database.CashRegisterSession{})
	ctx := context.Background()
	showroom := database.Business{Name: "Showroom", BusinessId: "showroom", Kind: database.BusinessKindDemo}
	real := database.Business{Name: "Real", BusinessId: "real", Kind: database.BusinessKind("real")}
	require.NoError(t, db.Create(&showroom).Error)
	require.NoError(t, db.Create(&real).Error)

	require.Equal(t, 1, openPublicDemoDrawers(ctx, db))
	open, err := database.FindOpenCashRegisterSessionForBusinessTx(db, showroom.ID)
	require.NoError(t, err)
	require.NotNil(t, open, "the showroom drawer is open for guests")
	none, err := database.FindOpenCashRegisterSessionForBusinessTx(db, real.ID)
	require.NoError(t, err)
	require.Nil(t, none, "a real venue's drawer is never opened for it")

	// Idempotent: a second boot reuses the open drawer.
	require.Equal(t, 1, openPublicDemoDrawers(ctx, db))
	var count int64
	require.NoError(t, db.Model(&database.CashRegisterSession{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

// Every boot-time demo seed opens the showroom drawers (a fresh `--demo`
// install must take counter payments before anyone opens Caja), on a private
// install as well as the public demo. A drawer an operator closed stays shut.
func TestRunDemoStartupEnsureOpensShowroomDrawers(t *testing.T) {
	for _, demoMode := range []bool{false, true} {
		t.Run(fmt.Sprintf("demo_mode=%v", demoMode), func(t *testing.T) {
			db := newCLITestDB(t, &database.Business{}, &database.CashRegisterSession{})
			ctx := context.Background()
			showroom := database.Business{Name: "Showroom", BusinessId: "showroom", Kind: database.BusinessKindDemo}
			require.NoError(t, db.Create(&showroom).Error)

			config.SetDemoModeForTesting(t, demoMode)
			runDemoStartupEnsure(ctx, db, &fakeStartupDemo{}, demoStartupPlan{SeedOwner: true}, 7, nil)
			open, err := database.FindOpenCashRegisterSessionForBusinessTx(db, showroom.ID)
			require.NoError(t, err)
			require.NotNil(t, open, "the seeded showroom drawer is open for guests")
		})
	}
}

func TestRunDemoStartupEnsureKeepsOperatorClosedDrawerShut(t *testing.T) {
	db := newCLITestDB(t, &database.Business{}, &database.CashRegisterSession{})
	ctx := context.Background()
	showroom := database.Business{Name: "Showroom", BusinessId: "showroom", Kind: database.BusinessKindDemo}
	require.NoError(t, db.Create(&showroom).Error)
	operator := uint(42)
	closedAt := time.Now()
	require.NoError(t, db.Create(&database.CashRegisterSession{
		BusinessID:     showroom.ID,
		Status:         database.CashRegisterSessionStatusClosed,
		OpenedAt:       closedAt.Add(-time.Hour),
		ClosedAt:       &closedAt,
		ClosedByUserID: &operator,
	}).Error)

	runDemoStartupEnsure(ctx, db, &fakeStartupDemo{}, demoStartupPlan{SeedOwner: true}, 7, nil)
	open, err := database.FindOpenCashRegisterSessionForBusinessTx(db, showroom.ID)
	require.NoError(t, err)
	require.Nil(t, open, "a drawer an operator closed is not reopened by the seed")
}
