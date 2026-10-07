package handlers

import (
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// notifyStaff is the HTTP-handler entry to the staff-awareness chokepoint. It
// forwards to services.NotifyStaff (the shared fan-out used by both handlers and
// background schedulers), supplying the web-push service from the server
// singleton. Called BESIDE an existing PublishJSON at each emit site, AFTER the
// underlying mutation has already succeeded, so it is FULLY best-effort: any
// failure is logged inside NotifyStaff and never propagated.
func notifyStaff(db *database.DB, businessID uint, targets []uint, kind string, key services.PushKey, args services.PushArgs, url string) {
	services.NotifyStaff(db, server.GetWebPushService(), businessID, targets, kind, key, args, url)
}
