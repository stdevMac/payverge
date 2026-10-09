package services

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RecurringEntriesScheduler claims due recurring_entry_templates hourly and
// inserts ManualLedgerEntry rows with reference recurring:<id>:<YYYY-MM-DD>.
type RecurringEntriesScheduler struct {
	db        *database.DB
	stopChan  chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	isRunning bool
	// nowFn injectable for tests.
	nowFn func() time.Time
	// periodLocked optional Wave 5 hook; when true, skip + flag needs_attention.
	// An error means the lock state is unknown: the template is skipped
	// without generating or advancing, and retried next tick.
	periodLocked func(db *gorm.DB, businessID uint, day time.Time) (bool, error)
}

func NewRecurringEntriesScheduler(db *database.DB) *RecurringEntriesScheduler {
	return &RecurringEntriesScheduler{
		db:       db,
		stopChan: make(chan struct{}),
		nowFn:    time.Now,
	}
}

// WithPeriodLockChecker wires Wave 5 close-the-books skip semantics.
func (s *RecurringEntriesScheduler) WithPeriodLockChecker(fn func(db *gorm.DB, businessID uint, day time.Time) (bool, error)) *RecurringEntriesScheduler {
	s.periodLocked = fn
	return s
}

func (s *RecurringEntriesScheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isRunning {
		return fmt.Errorf("recurring entries scheduler already running")
	}
	s.isRunning = true
	s.stopChan = make(chan struct{})
	s.wg.Add(1)
	go s.loop()
	log.Println("Recurring entries scheduler started")
	return nil
}

func (s *RecurringEntriesScheduler) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.isRunning {
		return fmt.Errorf("recurring entries scheduler not running")
	}
	close(s.stopChan)
	s.wg.Wait()
	s.isRunning = false
	log.Println("Recurring entries scheduler stopped")
	return nil
}

func (s *RecurringEntriesScheduler) loop() {
	defer s.wg.Done()
	// Run once at start, then hourly.
	s.tick()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *RecurringEntriesScheduler) tick() {
	if err := s.ProcessDue(s.nowFn().UTC()); err != nil {
		logger.Logger.Warnf("recurring entries tick: %v", err)
	}
}

// ProcessDue claims and generates all templates whose next_run_on has arrived
// in the OWNING BUSINESS's local calendar (not UTC). Exported for tests.
func (s *RecurringEntriesScheduler) ProcessDue(asOf time.Time) error {
	if s.db == nil {
		return nil
	}
	gdb := s.db.GetGorm()
	asOfDay := time.Date(asOf.Year(), asOf.Month(), asOf.Day(), 0, 0, 0, 0, time.UTC)

	// Candidate horizon is UTC date +1 so businesses ahead of UTC (up to +14)
	// are not generated a day late; the per-business local-date check below is
	// the actual due gate. SKIP LOCKED on Postgres; plain select on SQLite tests.
	horizon := asOfDay.AddDate(0, 0, 1)
	var due []database.RecurringEntryTemplate
	q := gdb.Where("active = ? AND next_run_on <= ?", true, horizon.Format("2006-01-02")).
		Order("id ASC").Limit(100)
	if gdb.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
	}
	if err := q.Find(&due).Error; err != nil {
		return err
	}
	localTodays := s.localTodayByBusiness(gdb, asOf, due)
	for i := range due {
		localToday, ok := localTodays[due[i].BusinessID]
		if !ok {
			localToday = asOfDay
		}
		runDay := due[i].NextRunOn
		if time.Date(runDay.Year(), runDay.Month(), runDay.Day(), 0, 0, 0, 0, time.UTC).After(localToday) {
			continue // not due yet in the business's local calendar
		}
		if err := s.generateOne(gdb, &due[i], asOfDay); err != nil {
			logger.Logger.Warnf("recurring template %d: %v", due[i].ID, err)
		}
	}
	return nil
}

// localTodayByBusiness resolves each candidate business's IANA timezone and
// returns its current local calendar date (as a naive UTC-midnight time for
// comparison). Unknown/invalid timezones fall back to UTC.
func (s *RecurringEntriesScheduler) localTodayByBusiness(gdb *gorm.DB, asOf time.Time, due []database.RecurringEntryTemplate) map[uint]time.Time {
	out := make(map[uint]time.Time, len(due))
	if len(due) == 0 {
		return out
	}
	idSet := make(map[uint]struct{}, len(due))
	ids := make([]uint, 0, len(due))
	for i := range due {
		if _, seen := idSet[due[i].BusinessID]; !seen {
			idSet[due[i].BusinessID] = struct{}{}
			ids = append(ids, due[i].BusinessID)
		}
	}
	var rows []struct {
		ID       uint
		Timezone string
	}
	if err := gdb.Table("businesses").Select("id, timezone").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		logger.Logger.Warnf("recurring entries: loading business timezones: %v", err)
		return out
	}
	for _, r := range rows {
		loc := time.UTC
		if tz := strings.TrimSpace(r.Timezone); tz != "" {
			if parsed, err := time.LoadLocation(tz); err == nil {
				loc = parsed
			}
		}
		local := asOf.In(loc)
		out[r.ID] = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	}
	return out
}

func (s *RecurringEntriesScheduler) generateOne(db *gorm.DB, tmpl *database.RecurringEntryTemplate, asOfDay time.Time) error {
	runOn := tmpl.NextRunOn
	// Normalize to date-only UTC.
	runDay := time.Date(runOn.Year(), runOn.Month(), runOn.Day(), 0, 0, 0, 0, time.UTC)
	if s.periodLocked != nil {
		locked, err := s.periodLocked(db, tmpl.BusinessID, runDay)
		if err != nil {
			// Fail closed: never post into a period whose lock could not be read.
			return fmt.Errorf("period lock check for %s: %w", runDay.Format("2006-01-02"), err)
		}
		if locked {
			return db.Model(tmpl).Updates(map[string]interface{}{
				"needs_attention": true,
				"updated_at":      time.Now().UTC(),
			}).Error
		}
	}

	ref := fmt.Sprintf("recurring:%d:%s", tmpl.ID, runDay.Format("2006-01-02"))
	next := advanceNextRun(runDay, tmpl.Cadence, tmpl.AnchorDay)
	now := time.Now().UTC()

	return db.Transaction(func(tx *gorm.DB) error {
		// CAS on next_run_on — only one generator wins.
		res := tx.Model(&database.RecurringEntryTemplate{}).
			Where("id = ? AND next_run_on = ?", tmpl.ID, tmpl.NextRunOn).
			Updates(map[string]interface{}{
				"next_run_on":       next,
				"last_generated_at": now,
				"needs_attention":   false,
				"updated_at":        now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil // lost race
		}
		entry := database.ManualLedgerEntry{
			BusinessID:       tmpl.BusinessID,
			EntryType:        tmpl.EntryType,
			Category:         tmpl.Category,
			Amount:           tmpl.AmountCents,
			Currency:         tmpl.Currency,
			OccurredAt:       runDay,
			Description:      tmpl.Description,
			Notes:            tmpl.Notes,
			Reference:        ref,
			CreatedByUserID:  tmpl.CreatedByUserID,
			CreatedByStaffID: tmpl.CreatedByStaffID,
		}
		if err := tx.Create(&entry).Error; err != nil {
			// Unique ref collision = already generated; treat as success.
			if strings.Contains(strings.ToLower(err.Error()), "unique") ||
				strings.Contains(strings.ToLower(err.Error()), "duplicate") {
				return nil
			}
			return err
		}
		return nil
	})
}

func advanceNextRun(from time.Time, cadence string, anchorDay int) time.Time {
	switch strings.ToLower(strings.TrimSpace(cadence)) {
	case "weekly":
		return from.AddDate(0, 0, 7)
	default: // monthly
		// Anchor day 1–28; clamp if needed.
		y, m, _ := from.Date()
		nextM := m + 1
		nextY := y
		if nextM > 12 {
			nextM = 1
			nextY++
		}
		day := anchorDay
		if day < 1 {
			day = 1
		}
		if day > 28 {
			day = 28
		}
		return time.Date(nextY, nextM, day, 0, 0, 0, 0, time.UTC)
	}
}
