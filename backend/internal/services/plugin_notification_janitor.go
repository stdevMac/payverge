package services

import (
	"context"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// DefaultPluginNotificationTTL is how long a pending/retry delivery may sit
// before the janitor drops it. A healthy worker clears the queue in seconds, so
// 24h only ever reaps rows that no worker is consuming (disabled/crashed worker).
const DefaultPluginNotificationTTL = 24 * time.Hour

// PluginOutboxMaintenanceResult reports what one janitor pass did, so callers
// (and tests) can observe behavior without scraping logs.
type PluginOutboxMaintenanceResult struct {
	Expired int64            // rows dropped for exceeding the TTL
	Backlog map[string]int64 // plugin -> pending count, only for plugins whose delivery is disabled
}

// RunPluginOutboxMaintenance performs one maintenance pass: it expires stale
// pending/retry deliveries past the TTL, and surfaces any backlog that is piling
// up for a plugin whose delivery path is not running. It is pure with respect to
// the package database global, so it is directly testable.
func RunPluginOutboxMaintenance(pluginNames []string, ttl time.Duration, now time.Time) (PluginOutboxMaintenanceResult, error) {
	if ttl <= 0 {
		ttl = DefaultPluginNotificationTTL
	}
	res := PluginOutboxMaintenanceResult{Backlog: map[string]int64{}}

	expired, err := database.ExpireStalePluginNotificationDeliveries(now.Add(-ttl), now)
	if err != nil {
		return res, err
	}
	res.Expired = expired

	for _, name := range pluginNames {
		if PluginDeliveryEnabled(name) {
			continue
		}
		pending, err := database.CountPluginNotificationPending(name)
		if err != nil {
			return res, err
		}
		if pending > 0 {
			res.Backlog[name] = pending
		}
	}
	return res, nil
}

// StartPluginNotificationJanitor runs RunPluginOutboxMaintenance on a ticker for
// the lifetime of ctx, regardless of whether any delivery worker is running. It
// logs expirations and warns loudly about a growing backlog with no consumer, so
// a misconfigured (nil-bot / disabled) worker is never silent again.
func StartPluginNotificationJanitor(ctx context.Context, pluginNames []string, ttl, interval time.Duration) {
	if ttl <= 0 {
		ttl = DefaultPluginNotificationTTL
	}
	if interval <= 0 {
		interval = 30 * time.Minute
	}

	logger.SafeGo(func() {
		// Run once promptly so existing stale backlog (e.g. the rows stuck since
		// a worker was never started) is cleaned up shortly after boot.
		runPluginOutboxMaintenanceTick(pluginNames, ttl)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runPluginOutboxMaintenanceTick(pluginNames, ttl)
			}
		}
	})
}

func runPluginOutboxMaintenanceTick(pluginNames []string, ttl time.Duration) {
	logger.SafeTick("plugin-notification-janitor", func() {
		res, err := RunPluginOutboxMaintenance(pluginNames, ttl, time.Now().UTC())
		if err != nil {
			logger.Logger.Warnf("Plugin notification janitor pass failed: %v", err)
			return
		}
		if res.Expired > 0 {
			logger.Logger.Warnf("Plugin notification janitor expired %d stale deliveries (older than %s)", res.Expired, ttl)
		}
		for name, pending := range res.Backlog {
			logger.Logger.Warnf("Plugin notification backlog: %d undelivered %q notifications with no running worker — configure the worker or they will be expired after %s", pending, name, ttl)
		}
	})
}
