package main

import (
	"context"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/services/cashregister"
	"gorm.io/gorm"
)

// prepareDemoMode validates DEMO_MODE and, when it is on, ensures the
// non-admin showroom owner the public demo signs visitors in as. It returns
// that owner's id (0 when DEMO_MODE is off). A misconfigured public demo is
// fatal: starting would either expose nothing or expose the admin.
func prepareDemoMode(db *gorm.DB) uint {
	if !config.DemoModeEnabled() {
		return 0
	}
	if err := config.ValidateDemoModeEnv(); err != nil {
		logger.Logger.Fatalf("Refusing to start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id, err := demomode.EnsureShowroomOwner(ctx, db)
	if err != nil {
		logger.Logger.Fatalf("Refusing to start the public demo: %v", err)
	}
	logger.Logger.Warnf("DEMO_MODE is on: this is a PUBLIC demo. One-click demo sign-in is enabled (showroom owner user id %d), outbound email is logged only, signup is closed and integrations, uploads and account changes are refused.", id)
	return id
}

// openPublicDemoDrawers opens the house cash drawer of every showroom venue.
// The guest payment picker only offers "pay at the counter" while a drawer is
// open, and the house drawer otherwise opens only when an operator lands on
// Caja, so the first guests to scan a table of a fresh demo were told the
// venue "cannot take payment". It runs after every demo seed (DEMO_DATA boot,
// `server demo seed`, `server demo reset`), so the public demo's pristine
// snapshot and every reset carry open drawers. A drawer an operator closes
// stays closed until the next reset. It returns how many venues have an open
// drawer.
func openPublicDemoDrawers(ctx context.Context, db *gorm.DB) int {
	var ids []uint
	if err := db.WithContext(ctx).Model(&database.Business{}).
		Where("kind = ?", string(database.BusinessKindDemo)).
		Order("id ASC").Pluck("id", &ids).Error; err != nil {
		logger.Logger.Warnf("Public demo: listing showroom venues for the cash drawer failed: %v", err)
		return 0
	}
	svc := cashregister.NewService(db)
	open := 0
	for _, id := range ids {
		session, err := svc.EnsureDemoHouseCashSession(ctx, id)
		if err != nil {
			logger.Logger.Warnf("Public demo: opening the cash drawer of business %d failed: %v", id, err)
			continue
		}
		if session != nil {
			open++
		}
	}
	return open
}
