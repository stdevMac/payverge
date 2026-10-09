package services

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// MilestoneTracker manages business milestone tracking and celebration emails
type MilestoneTracker struct {
	db *database.DB
}

// NewMilestoneTracker creates a new milestone tracker instance
func NewMilestoneTracker(db *database.DB) *MilestoneTracker {
	return &MilestoneTracker{
		db: db,
	}
}

func (mt *MilestoneTracker) ProcessPendingMilestones(businessID uint) error {
	events, err := database.ClaimPendingBusinessMilestoneEvents(businessID, 25)
	if err != nil {
		return fmt.Errorf("failed to list pending milestones: %w", err)
	}
	if len(events) == 0 {
		return nil
	}

	business, err := database.GetBusinessByID(businessID)
	if err != nil {
		return fmt.Errorf("failed to get business: %w", err)
	}

	// P1-5 single-owner parity: the retired lifecycle milestone checks skipped
	// demo businesses and opted-out owners; the event system is now the only
	// milestone sender, so it enforces the same policy. Skipped events are
	// marked sent (not failed) so they never retry.
	if business.IsDemo || marketingEmailOptedOut(business) {
		for _, event := range events {
			if err := database.MarkBusinessMilestoneEventSent(event.ID, event.ProcessingToken); err != nil {
				log.Printf("Failed to mark skipped milestone event %d sent: %v", event.ID, err)
			}
		}
		return nil
	}

	for _, event := range events {
		if err := mt.sendMilestoneEvent(business, event); err != nil {
			log.Printf("Failed to send milestone event %d: %v", event.ID, err)
			if markErr := database.MarkBusinessMilestoneEventFailed(event.ID, event.ProcessingToken, err); markErr != nil {
				log.Printf("Failed to mark milestone event %d failed: %v", event.ID, markErr)
			}
			continue
		}
		if err := database.MarkBusinessMilestoneEventSent(event.ID, event.ProcessingToken); err != nil {
			log.Printf("Failed to mark milestone event %d sent: %v", event.ID, err)
			continue
		}
		log.Printf("🎉 Milestone reached for business %s: %s", business.Name, event.MilestoneType)
	}
	return nil
}

func (mt *MilestoneTracker) ProcessPendingMilestoneBatch(limit int) error {
	businessIDs, err := database.ListBusinessesWithPendingMilestoneEvents(limit)
	if err != nil {
		return fmt.Errorf("failed to list businesses with pending milestones: %w", err)
	}

	for _, businessID := range businessIDs {
		if err := mt.ProcessPendingMilestones(businessID); err != nil {
			log.Printf("Failed to process pending milestones for business %d: %v", businessID, err)
		}
	}
	return nil
}

func (mt *MilestoneTracker) sendMilestoneEvent(business *database.Business, event database.BusinessMilestoneEvent) error {
	switch event.MilestoneType {
	case database.BusinessMilestoneTypeFirstOrder:
		return mt.sendFirstOrderEmail(business)
	case database.BusinessMilestoneTypeRevenue:
		return mt.sendRevenueMilestoneEmail(business, milestoneRevenueDisplayText(event))
	default:
		return nil
	}
}

func milestoneRevenueDisplayText(event database.BusinessMilestoneEvent) string {
	if event.Payload != nil {
		if display, ok := event.Payload["display_text"].(string); ok && display != "" {
			return display
		}
	}
	return database.RevenueMilestoneDisplayText(event.ThresholdCents)
}

// sendFirstOrderEmail sends celebration email for first order
func (mt *MilestoneTracker) sendFirstOrderEmail(business *database.Business) error {
	if business == nil || business.Email == "" || emails.EmailServerInstance == nil {
		return nil // No email to send to
	}

	// EMAIL-2: owner-facing milestone email — source the language from the
	// owner's UI locale, not the customer-display DefaultLanguage.
	language := BusinessOwnerEmailLanguage(business)

	// Send first order celebration email
	if err := emails.EmailServerInstance.SendMilestoneFirstOrderEmail(
		[]string{business.Email},
		business.OwnerName,
		BuildBusinessDashboardURL(business),
		language,
	); err != nil {
		return fmt.Errorf("failed to send first order email: %w", err)
	}

	return nil
}

// sendRevenueMilestoneEmail sends celebration email for revenue milestone
func (mt *MilestoneTracker) sendRevenueMilestoneEmail(business *database.Business, revenueAmount string) error {
	if business == nil || business.Email == "" || emails.EmailServerInstance == nil {
		return nil // No email to send to
	}

	// EMAIL-2: owner-facing milestone email — source the language from the
	// owner's UI locale, not the customer-display DefaultLanguage.
	language := BusinessOwnerEmailLanguage(business)

	// Send revenue milestone celebration email
	if err := emails.EmailServerInstance.SendMilestonesRevenueThresholdsEmail(
		[]string{business.Email},
		business.OwnerName,
		revenueAmount,
		BuildBusinessDashboardURL(business),
		language,
	); err != nil {
		return fmt.Errorf("failed to send revenue milestone email: %w", err)
	}

	return nil
}

// MilestoneStatus represents the current milestone status for a business
type MilestoneStatus struct {
	TotalRevenue        float64  `json:"total_revenue"`
	TotalOrders         int64    `json:"total_orders"`
	AchievedMilestones  []string `json:"achieved_milestones"`
	NextMilestone       string   `json:"next_milestone,omitempty"`
	NextMilestoneAmount float64  `json:"next_milestone_amount,omitempty"`
	ProgressToNext      float64  `json:"progress_to_next,omitempty"`
}

type MilestoneScheduler struct {
	tracker   *MilestoneTracker
	interval  time.Duration
	stopChan  chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	isRunning bool
}

func NewMilestoneScheduler(db *database.DB) *MilestoneScheduler {
	return &MilestoneScheduler{
		tracker:  NewMilestoneTracker(db),
		interval: time.Minute,
		stopChan: make(chan struct{}),
	}
}

func (ms *MilestoneScheduler) Start() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.isRunning {
		return fmt.Errorf("milestone scheduler is already running")
	}

	ms.isRunning = true
	ms.stopChan = make(chan struct{}) // re-create so a Stop→Start cycle is safe
	log.Println("Starting milestone scheduler service...")
	ms.wg.Add(1)
	go ms.schedulerLoop()
	log.Println("Milestone scheduler service started successfully")
	return nil
}

func (ms *MilestoneScheduler) Stop() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if !ms.isRunning {
		return fmt.Errorf("milestone scheduler is not running")
	}

	log.Println("Stopping milestone scheduler service...")
	close(ms.stopChan)
	ms.wg.Wait()
	ms.isRunning = false
	log.Println("Milestone scheduler service stopped")
	return nil
}

func (ms *MilestoneScheduler) schedulerLoop() {
	defer ms.wg.Done()

	ticker := time.NewTicker(ms.interval)
	defer ticker.Stop()

	logger.SafeTick("milestone-tracker", ms.processPending)
	for {
		select {
		case <-ticker.C:
			logger.SafeTick("milestone-tracker", ms.processPending)
		case <-ms.stopChan:
			return
		}
	}
}

func (ms *MilestoneScheduler) processPending() {
	if err := ms.tracker.ProcessPendingMilestoneBatch(100); err != nil {
		log.Printf("Failed to process pending milestone batch: %v", err)
	}
}
