package jobs

import (
	"log"

	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/robfig/cron/v3"
)

// ReservationScheduler manages scheduled jobs for reservation reminders
type ReservationScheduler struct {
	cron            *cron.Cron
	reminderService *services.ReservationReminderService
}

// NewReservationScheduler creates a new reservation scheduler
func NewReservationScheduler() *ReservationScheduler {
	return &ReservationScheduler{
		// WithChain(Recover(...)) so a panic in the reminder or approval-sweeper
		// job is recovered and logged instead of unwinding the cron goroutine
		// and crashing the entire process (all tenants).
		cron:            cron.New(cron.WithChain(cron.Recover(cron.DefaultLogger))),
		reminderService: services.NewReservationReminderService(),
	}
}

// Start begins the scheduled jobs
func (s *ReservationScheduler) Start() error {
	log.Println("Starting reservation scheduler...")

	// Run reminder check every hour
	// Cron format: minute hour day month weekday
	// "0 * * * *" means "at minute 0 of every hour"
	_, err := s.cron.AddFunc("0 * * * *", func() {
		log.Println("Running scheduled reservation reminder check...")
		if err := s.reminderService.ProcessUpcomingReservations(); err != nil {
			log.Printf("Error processing reservation reminders: %v", err)
		}
	})
	if err != nil {
		return err
	}

	// Auto-decline pending approval requests past their deadline every 10
	// minutes. Overlapping passes are safe: the sweeper claims each row via a
	// status-guarded UPDATE before any side effects.
	approvalSweeper := services.NewReservationApprovalSweeper()
	noShowSweeper := services.NewReservationNoShowSweeper()
	_, err = s.cron.AddFunc("*/10 * * * *", func() {
		if _, err := approvalSweeper.ProcessExpiredApprovalRequests(); err != nil {
			log.Printf("Error sweeping expired reservation approvals: %v", err)
		}
		// After declines, so a row never gets a nudge and an auto-decline in
		// the same tick: the reminder pass skips anything already past its
		// deadline, and expired rows are gone by the time it runs.
		if _, err := approvalSweeper.ProcessApprovalReminders(); err != nil {
			log.Printf("Error sending reservation approval reminders: %v", err)
		}
		if _, err := noShowSweeper.ProcessExpiredConfirmedArrivals(); err != nil {
			log.Printf("Error sweeping expired confirmed reservations: %v", err)
		}
	})
	if err != nil {
		return err
	}

	// Start the cron scheduler
	s.cron.Start()
	log.Println("Reservation scheduler started successfully")

	return nil
}

// Stop stops the scheduler
func (s *ReservationScheduler) Stop() {
	log.Println("Stopping reservation scheduler...")
	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Println("Reservation scheduler stopped")
}
