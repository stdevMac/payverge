package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/schedclaim"
)

// LifecycleScheduler manages business lifecycle email campaigns
type LifecycleScheduler struct {
	db        *database.DB
	stopChan  chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	isRunning bool
}

// NewLifecycleScheduler creates a new lifecycle scheduler instance
func NewLifecycleScheduler(db *database.DB) *LifecycleScheduler {
	return &LifecycleScheduler{
		db:       db,
		stopChan: make(chan struct{}),
	}
}

// Start begins the lifecycle email scheduler service
func (ls *LifecycleScheduler) Start() error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if ls.isRunning {
		return fmt.Errorf("lifecycle scheduler is already running")
	}

	ls.isRunning = true
	ls.stopChan = make(chan struct{}) // re-create so a Stop→Start cycle is safe
	log.Println("Starting lifecycle scheduler service...")

	// Start the main scheduler loop
	ls.wg.Add(1)
	go ls.schedulerLoop()

	log.Println("Lifecycle scheduler service started successfully")
	return nil
}

// Stop gracefully stops the lifecycle scheduler service
func (ls *LifecycleScheduler) Stop() error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if !ls.isRunning {
		return fmt.Errorf("lifecycle scheduler is not running")
	}

	log.Println("Stopping lifecycle scheduler service...")
	close(ls.stopChan)
	ls.wg.Wait()
	ls.isRunning = false
	log.Println("Lifecycle scheduler service stopped")
	return nil
}

// schedulerLoop is the main loop that checks for lifecycle email triggers
func (ls *LifecycleScheduler) schedulerLoop() {
	defer ls.wg.Done()

	// 24h ticker, phase-locked to process start (NOT a wall-clock send window;
	// timezone-aware windows are future work — dates ARE rendered in the
	// business TZ, see FormatBillingDateInLocation).
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Run initial check immediately
	logger.SafeTick("lifecycle-scheduler", ls.checkLifecycleEmails)

	for {
		select {
		case <-ticker.C:
			logger.SafeTick("lifecycle-scheduler", ls.checkLifecycleEmails)
		case <-ls.stopChan:
			return
		}
	}
}

// checkLifecycleEmails checks all lifecycle email triggers
func (ls *LifecycleScheduler) checkLifecycleEmails() {
	log.Println("Checking lifecycle email triggers...")

	// Check getting started emails (1 day after registration)
	ls.checkGettingStartedEmails()

	// Check day-3 setup nudges (created 3–4 days ago, required setup incomplete)
	ls.checkSetupNudgeEmails()

	// First-order and revenue milestone celebrations are owned exclusively by
	// the transactional event system (milestone_tracker.go / MilestoneScheduler,
	// 1-min tick). P1-5: this daily tick used to duplicate every celebration.

	log.Println("Lifecycle email check complete")
}

// checkGettingStartedEmails sends getting started emails to businesses 1 day after registration
func (ls *LifecycleScheduler) checkGettingStartedEmails() {
	// P1-8: window is 3 ticks wide (claim column owns dedup) so a failed tick
	// or a <3-day scheduler outage cannot silently lose the day's cohort.
	windowEnd := time.Now().Add(-24 * time.Hour)
	windowStart := time.Now().Add(-96 * time.Hour)

	var businesses []database.Business
	if err := database.GetDB().
		Select("id", "business_id", "name", "owner_name", "email", "default_language", "user_id", "owner_address").
		Where("created_at BETWEEN ? AND ?", windowStart, windowEnd).
		Where("email != ?", "").
		Where("is_demo = ?", false).
		Where("getting_started_email_sent_at IS NULL").
		Find(&businesses).Error; err != nil {
		log.Printf("Error fetching businesses for getting started emails: %v", err)
		return
	}

	emailsSent := 0
	for _, business := range businesses {
		if !ls.claimCampaign(context.Background(), business.ID, "getting_started_email_sent_at") {
			continue
		}
		if err := ls.sendGettingStartedEmail(&business); err != nil {
			log.Printf("Failed to send getting started email to business %d: %v", business.ID, err)
			ls.releaseCampaign(business.ID, "getting_started_email_sent_at")
		} else {
			emailsSent++
		}
	}

	if emailsSent > 0 {
		log.Printf("Sent %d getting started emails", emailsSent)
	}
}

// checkSetupNudgeEmails nudges businesses created 3–6 days ago that have NOT
// finished their required go-live setup, segmented by the first missing step.
// P1-8: window is 3 ticks wide (claim column owns dedup). Businesses whose
// required setup is already complete are neither claimed nor emailed (so a later
// lapse can't be spuriously nudged). One-shot claim + release-on-fail
// (claimCampaign/releaseCampaign); projection stays narrow (perf gate).
func (ls *LifecycleScheduler) checkSetupNudgeEmails() {
	windowEnd := time.Now().Add(-3 * 24 * time.Hour)
	windowStart := time.Now().Add(-6 * 24 * time.Hour)

	var businesses []database.Business
	if err := database.GetDB().
		Select("id", "business_id", "name", "owner_name", "email", "default_language", "user_id").
		Where("created_at BETWEEN ? AND ?", windowStart, windowEnd).
		Where("email != ?", "").
		Where("is_demo = ?", false).
		Where("setup_nudge_email_sent_at IS NULL").
		Find(&businesses).Error; err != nil {
		log.Printf("Error fetching businesses for setup nudge emails: %v", err)
		return
	}

	emailsSent := 0
	for i := range businesses {
		status, err := ComputeSetupStatus(businesses[i].ID)
		if err != nil {
			log.Printf("setup nudge status failed for business %d: %v", businesses[i].ID, err)
			continue
		}
		if status.RequiredDone {
			continue // already live — nothing to nudge
		}
		if !claimOneShotCampaign(context.Background(), businesses[i].ID, "setup_nudge_email_sent_at") {
			continue
		}
		// Re-check AFTER the claim: the owner may have finished setup between
		// the batch status computation and now — a stale nudge to a live
		// business reads as "they don't know their own product". Keeping the
		// consumed claim is correct: a finished business never needs this nudge.
		fresh, err := ComputeSetupStatus(businesses[i].ID)
		if err == nil && fresh.RequiredDone {
			continue
		}
		if err := ls.sendSetupNudgeEmail(&businesses[i], status.FirstMissingRequiredStep()); err != nil {
			log.Printf("Failed to send setup nudge email to business %d: %v", businesses[i].ID, err)
			releaseOneShotCampaign(businesses[i].ID, "setup_nudge_email_sent_at")
		} else {
			emailsSent++
		}
	}

	if emailsSent > 0 {
		log.Printf("Sent %d setup nudge emails", emailsSent)
	}
}

// sendSetupNudgeEmail renders the day-3 nudge for the first missing required step.
func (ls *LifecycleScheduler) sendSetupNudgeEmail(business *database.Business, missingStep string) error {
	if business == nil || business.Email == "" || emails.EmailServerInstance == nil {
		return nil
	}

	language := BusinessOwnerEmailLanguage(business)

	if err := emails.EmailServerInstance.SendSetupNudgeEmail(
		[]string{business.Email},
		business.OwnerName,
		missingStep,
		BuildBusinessDashboardURL(business),
		ownerEmailUnverified(business),
		language,
	); err != nil {
		return fmt.Errorf("failed to send setup nudge email: %w", err)
	}

	log.Printf("🧭 Setup nudge email sent to business: %s (missing: %s)", business.Name, missingStep)
	return nil
}

// ownerEmailUnverified reports whether the business owner's email-auth record
// is still unverified (the login path rejects unverified email accounts, so
// the nudge adds a verify reminder). Queries user_auths by table name because
// services must not import internal/auth (import cycle via server). Fails
// closed to false on lookup errors: better no reminder than a wrong claim.
func ownerEmailUnverified(business *database.Business) bool {
	if business == nil || business.UserID == nil {
		return false
	}
	// EMAIL_VERIFICATION=off: the flag stays false but there is nothing the
	// owner can (or needs to) verify, so no reminder.
	if !config.EmailVerificationRequired() {
		return false
	}
	var cnt int64
	if err := database.GetDB().Table("user_auths").
		Where("user_id = ? AND provider = ? AND email_verified = ?", *business.UserID, "email", false).
		Count(&cnt).Error; err != nil {
		log.Printf("setup nudge: owner verification lookup failed for business %d: %v", business.ID, err)
		return false
	}
	return cnt > 0
}

// sendGettingStartedEmail sends getting started guide email
func (ls *LifecycleScheduler) sendGettingStartedEmail(business *database.Business) error {
	if business == nil || business.Email == "" || emails.EmailServerInstance == nil {
		return nil
	}

	// EMAIL-2: owner-facing lifecycle email — source the language from the
	// owner's UI locale, not the customer-display DefaultLanguage.
	language := BusinessOwnerEmailLanguage(business)

	if err := emails.EmailServerInstance.SendGettingStartedEmail(
		[]string{business.Email},
		business.OwnerName,
		BuildBusinessDashboardURL(business),
		language,
	); err != nil {
		return fmt.Errorf("failed to send getting started email: %w", err)
	}

	log.Printf("📚 Getting started email sent to business: %s", business.Name)
	return nil
}

// claimCampaign atomically stamps the given nullable campaign column for a
// business where it is still NULL, so a restart's immediate-on-boot sweep
// cannot re-send the same onboarding email. RowsAffected==1 means this caller
// won and may send.
func (ls *LifecycleScheduler) claimCampaign(ctx context.Context, businessID uint, col string) bool {
	c := schedclaim.New(ls.db.GetGorm())
	claimed, err := c.Claim(ctx, "businesses", col, "id", businessID)
	if err != nil {
		log.Printf("lifecycle claim %s failed for business %d: %v", col, businessID, err)
		return false
	}
	return claimed
}

// releaseCampaign nulls out a claim column so the send can be retried on the
// next tick when delivery of the campaign email itself failed.
func (ls *LifecycleScheduler) releaseCampaign(businessID uint, col string) {
	if err := ls.db.GetGorm().Model(&database.Business{}).Where("id = ?", businessID).Update(col, nil).Error; err != nil {
		log.Printf("lifecycle release %s failed for business %d: %v — campaign will stay claimed until manually cleared", col, businessID, err)
	}
}

// claimOneShotCampaign is the package-function analogue of claimCampaign for the
// newer one-shot lifecycle columns. It claims via the global
// database.GetDB() handle — same rationale as claimRecurringCampaign — so the
// checks that use it stay consistent with the recurring path and remain testable
// under database.SetTestDB without constructing a *database.DB.
func claimOneShotCampaign(ctx context.Context, businessID uint, col string) bool {
	claimed, err := schedclaim.New(database.GetDB()).Claim(ctx, "businesses", col, "id", businessID)
	if err != nil {
		log.Printf("lifecycle one-shot claim %s failed for business %d: %v", col, businessID, err)
		return false
	}
	return claimed
}

// releaseOneShotCampaign nulls a one-shot claim column so a failed send is retried
// on the next tick.
func releaseOneShotCampaign(businessID uint, col string) {
	if err := database.GetDB().Model(&database.Business{}).Where("id = ?", businessID).Update(col, nil).Error; err != nil {
		log.Printf("lifecycle release %s failed for business %d: %v — campaign will stay claimed until manually cleared", col, businessID, err)
	}
}

// marketingEmailOptedOut reports whether the business owner has switched off
// email notifications (the master EmailEnabled preference behind every
// lifecycle email's "manage notifications" footer link). Marketing-toned
// campaigns (the milestone celebrations) must honor it; transactional emails
// such as receipts deliberately do not consult this. Fail-open: an unresolvable owner or a
// lookup error sends rather than silently killing a campaign.
func marketingEmailOptedOut(business *database.Business) bool {
	if business == nil {
		return false
	}
	switch {
	case business.UserID != nil && *business.UserID != 0:
		var user database.User
		if err := database.GetDB().Select("email_enabled").
			Where("id = ?", *business.UserID).First(&user).Error; err != nil {
			return false
		}
		return !user.NotificationPreferences.EmailEnabled
	case business.Email != "":
		// Case-colliding accounts (pre-P2-2 rows): deterministic + conservative —
		// if ANY matching account opted out, suppress the campaign.
		var users []database.User
		if err := database.GetDB().Select("email_enabled").
			Where("LOWER(email) = LOWER(?)", business.Email).Find(&users).Error; err != nil {
			return false
		}
		for i := range users {
			if !users[i].NotificationPreferences.EmailEnabled {
				return true
			}
		}
		return false
	default:
		return false
	}
}
