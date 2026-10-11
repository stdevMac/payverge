package services

import (
	"context"
	"errors"
	"fmt"
	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/schedclaim"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
)

// GuestFeedbackScheduler handles sending feedback emails to guests 24h after payment
type GuestFeedbackScheduler struct {
	db        *database.DB
	stopChan  chan struct{}
	wg        sync.WaitGroup
	isRunning bool
	mu        sync.Mutex
}

// NewGuestFeedbackScheduler creates a new guest feedback scheduler
func NewGuestFeedbackScheduler(db *database.DB) *GuestFeedbackScheduler {
	return &GuestFeedbackScheduler{
		db:       db,
		stopChan: make(chan struct{}),
	}
}

// Start begins the guest feedback scheduler service
func (gfs *GuestFeedbackScheduler) Start() error {
	gfs.mu.Lock()
	defer gfs.mu.Unlock()

	if gfs.isRunning {
		return fmt.Errorf("guest feedback scheduler is already running")
	}

	gfs.isRunning = true
	gfs.stopChan = make(chan struct{}) // re-create so a Stop→Start cycle is safe
	gfs.wg.Add(1)
	go gfs.schedulerLoop()

	log.Println("Guest feedback scheduler service started")
	return nil
}

// Stop gracefully stops the guest feedback scheduler service
func (gfs *GuestFeedbackScheduler) Stop() error {
	gfs.mu.Lock()
	defer gfs.mu.Unlock()

	if !gfs.isRunning {
		return fmt.Errorf("guest feedback scheduler is not running")
	}

	log.Println("Stopping guest feedback scheduler service...")
	close(gfs.stopChan)
	gfs.wg.Wait()
	gfs.isRunning = false
	log.Println("Guest feedback scheduler service stopped")
	return nil
}

// schedulerLoop runs the main scheduler loop
func (gfs *GuestFeedbackScheduler) schedulerLoop() {
	defer gfs.wg.Done()

	// Check every 6 hours for bills that need feedback emails
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	// Run initial check immediately
	logger.SafeTick("guest-feedback-scheduler", func() { gfs.checkFeedbackEmails() })

	for {
		select {
		case <-ticker.C:
			logger.SafeTick("guest-feedback-scheduler", func() { gfs.checkFeedbackEmails() })
		case <-gfs.stopChan:
			return
		}
	}
}

// checkFeedbackEmails finds bills closed 24–48 hours ago whose CRM
// customer has opted in and hasn't yet received a feedback email,
// then sends one per bill. Idempotency via Bill.FeedbackEmailSentAt.
//
// Window shape: the lower bound (24h) is the courtesy delay before asking for
// feedback; the upper bound (48h) is a full extra day of catch-up headroom so
// a scheduler gap longer than the 6h tick (deploy, crash, host downtime) does
// not silently drop that stretch of bills forever — the old 24–30h window did
// exactly that. 48h is defensible: a feedback ask two days after the visit is
// still relevant; older than that reads as spam. Widening cannot double-send —
// the feedback_email_sent_at claim column (claimFeedback) dedups.
func (gfs *GuestFeedbackScheduler) checkFeedbackEmails() (sent, skipped int) {
	log.Println("Checking for guest feedback emails to send...")

	twentyFourHoursAgo := time.Now().Add(-24 * time.Hour)
	fortyEightHoursAgo := time.Now().Add(-48 * time.Hour)

	var bills []database.Bill
	if err := database.GetDB().
		Preload("Business").
		Preload("CRMCustomer").
		Where("status = ? AND closed_at BETWEEN ? AND ? AND crm_customer_id IS NOT NULL AND feedback_email_sent_at IS NULL",
			database.BillStatusPaid, fortyEightHoursAgo, twentyFourHoursAgo).
		Find(&bills).Error; err != nil {
		log.Printf("Error fetching bills for guest feedback: %v", err)
		return 0, 0
	}

	for i := range bills {
		bill := &bills[i]
		if bill.CRMCustomer == nil || bill.CRMCustomer.Email == "" {
			// No CRM email to send to — definitive skip.
			gfs.markFeedbackDecided(bill)
			continue
		}

		// Gate on the per-business opt-in flag.
		var link database.CustomerBusiness
		if err := database.GetDB().
			Where("customer_id = ? AND business_id = ?", bill.CRMCustomer.ID, bill.BusinessID).
			First(&link).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// No customer-business link exists — definitive skip.
				gfs.markFeedbackDecided(bill)
			}
			// Transient errors leave the row NULL so the next tick retries.
			continue
		}
		if !link.OptInEmail {
			// Customer opted out — definitive skip.
			gfs.markFeedbackDecided(bill)
			continue
		}
		if emails.IsReservedRecipient(bill.CRMCustomer.Email) {
			// Special-use address (demo seed data such as
			// demo+...@payverge.local): the mailer would drop it, so do not
			// count or log it as sent. Definitive skip.
			gfs.markFeedbackDecided(bill)
			skipped++
			continue
		}

		// Claim FIRST so a concurrent tick/replica or an overlapping slow send
		// cannot double-email the guest. Loser skips silently.
		if !gfs.claimFeedback(context.Background(), bill.ID) {
			continue
		}
		if err := gfs.sendGuestFeedbackEmail(bill, bill.CRMCustomer.Email); err != nil {
			log.Printf("guest feedback email failed for bill %d: %v", bill.ID, err)
			// Transient send error — release the claim so the next tick retries.
			gfs.releaseFeedbackClaim(bill.ID)
			continue
		}
		sent++
	}

	if sent > 0 {
		log.Printf("✅ Sent %d guest feedback emails", sent)
	}
	if skipped > 0 {
		log.Printf("Skipped %d guest feedback emails to special-use addresses (demo or test data)", skipped)
	}
	return sent, skipped
}

// markFeedbackDecided stamps FeedbackEmailSentAt so the bill exits the
// candidate set for future scheduler ticks. Used for both successful
// sends and definitive skips (opted-out, no CRM link). NOT used for
// transient errors — those leave the timestamp NULL to allow retry.
func (gfs *GuestFeedbackScheduler) markFeedbackDecided(bill *database.Bill) {
	now := time.Now()
	if err := database.GetDB().Model(bill).Update("feedback_email_sent_at", &now).Error; err != nil {
		log.Printf("failed to mark feedback_email_sent_at for bill %d: %v", bill.ID, err)
	}
}

// claimFeedback atomically reserves the feedback send for a bill by stamping
// feedback_email_sent_at where it is still NULL. RowsAffected==1 means this
// caller won and may send; a loser must not send.
func (gfs *GuestFeedbackScheduler) claimFeedback(ctx context.Context, billID uint) bool {
	c := schedclaim.New(gfs.db.GetGorm())
	claimed, err := c.Claim(ctx, "bills", "feedback_email_sent_at", "id", billID)
	if err != nil {
		log.Printf("guest feedback claim failed for bill %d: %v", billID, err)
		return false
	}
	return claimed
}

// releaseFeedbackClaim returns a claimed bill to the candidate set after a
// transient send failure so a later tick can retry it.
func (gfs *GuestFeedbackScheduler) releaseFeedbackClaim(billID uint) {
	if err := gfs.db.GetGorm().
		Model(&database.Bill{}).
		Where("id = ?", billID).
		Update("feedback_email_sent_at", nil).Error; err != nil {
		log.Printf("guest feedback release failed for bill %d: %v", billID, err)
	}
}

// sendGuestFeedbackEmail sends feedback request email to guest
func (gfs *GuestFeedbackScheduler) sendGuestFeedbackEmail(bill *database.Bill, customerEmail string) error {
	if emails.EmailServerInstance == nil {
		return nil
	}

	language := "en"
	if bill.Business.DefaultLanguage != "" {
		language = bill.Business.DefaultLanguage
	}

	baseURL := config.FrontendBaseURL()

	// Generate feedback URL (could be a review page or feedback form)
	feedbackURL := fmt.Sprintf("%s/feedback/%s", baseURL, bill.Business.BusinessId)

	// Send feedback email
	if err := emails.EmailServerInstance.ForBill(bill.BusinessID, bill.ID).SendGuestFeedbackEmail(
		[]string{customerEmail},
		bill.Business.Name,
		feedbackURL,
		language,
	); err != nil {
		return err
	}

	logger.Logger.Infof("Guest feedback email sent to %s (bill #%d)", logger.RedactEmail(customerEmail), bill.ID)
	return nil
}
