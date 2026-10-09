package main

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// tenantMailBudgetPurgeInterval is how often expired tenant-mail ledger rows
// are deleted from auth_attempts.
const tenantMailBudgetPurgeInterval = time.Hour

// wireTenantMailBudget attaches the per-tenant outbound email budget (the
// EMAIL_TENANT_* caps, see internal/emails/tenant_budget.go) to the email
// server and starts its ledger janitor. It runs after migration verification,
// like the suppression store, because the ledger lives in auth_attempts.
func wireTenantMailBudget(emailServer *emails.EmailServer, db *gorm.DB) {
	if emailServer == nil || db == nil {
		return
	}
	cfg := emails.TenantMailBudgetConfigFromEnv()
	budget := emails.NewGormTenantMailBudget(db, cfg)
	emailServer.SetTenantMailBudget(budget)
	emails.StartTenantMailBudgetJanitor(context.Background(), budget, tenantMailBudgetPurgeInterval)
	logger.Logger.Infof(
		"Tenant email budget enabled (business/day=%d demo/day=%d recipient/day=%d booking-lane/day=%d booking-lane-recipient/day=%d booking-lane-recipient-global/day=%d recipient-global-all/day=%d dedupe=%s)",
		cfg.BusinessDailyCap, cfg.DemoBusinessDailyCap,
		cfg.RecipientDailyCap,
		cfg.ReservationRequestDailyCap, cfg.ReservationRequestRecipientDailyCap, cfg.RecipientGlobalDailyCap,
		cfg.RecipientGlobalAllDailyCap,
		budget.Config().DedupeWindow,
	)
}
