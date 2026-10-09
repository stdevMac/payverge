package services

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// inventoryAlertRow holds the result of the low-stock query.
type inventoryAlertRow struct {
	BusinessID       uint
	ItemID           uint
	ItemName         string
	Unit             string
	CurrentQuantity  float64
	ReorderThreshold float64
	AlertType        string
}

// InventoryAlertScheduler checks inventory items every 15 minutes and sends
// email + plugin notifications for items that have dropped to or below their
// reorder threshold and have not already been alerted in the last 24 hours.
type InventoryAlertScheduler struct {
	db          *gorm.DB
	emailServer *emails.EmailServer
	stopChan    chan struct{}
	wg          sync.WaitGroup
	mu          sync.Mutex
	isRunning   bool
}

// NewInventoryAlertScheduler creates a new InventoryAlertScheduler.
func NewInventoryAlertScheduler(db *gorm.DB, emailServer *emails.EmailServer) *InventoryAlertScheduler {
	return &InventoryAlertScheduler{
		db:          db,
		emailServer: emailServer,
		stopChan:    make(chan struct{}),
	}
}

// Start begins the scheduler loop in a background goroutine.
func (s *InventoryAlertScheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isRunning {
		return fmt.Errorf("inventory alert scheduler already running")
	}
	s.isRunning = true
	s.stopChan = make(chan struct{}) // re-create so a Stop→Start cycle is safe
	s.wg.Add(1)
	go s.loop()
	log.Println("Inventory alert scheduler started")
	return nil
}

// Stop gracefully shuts down the scheduler and waits for in-flight work.
func (s *InventoryAlertScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.isRunning {
		return
	}
	close(s.stopChan)
	s.wg.Wait()
	s.isRunning = false
	log.Println("Inventory alert scheduler stopped")
}

func (s *InventoryAlertScheduler) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	// Run immediately on startup so the first check is not delayed 15 minutes.
	logger.SafeTick("inventory-alert-scheduler", s.checkAlerts)

	for {
		select {
		case <-ticker.C:
			logger.SafeTick("inventory-alert-scheduler", s.checkAlerts)
		case <-s.stopChan:
			return
		}
	}
}

// checkAlerts queries for all items at or below their reorder threshold that
// have not been alerted in the last 24 hours, groups them by business, logs
// alert records, and dispatches email + plugin notifications.
func (s *InventoryAlertScheduler) checkAlerts() {
	rows, err := s.queryLowStockItems()
	if err != nil {
		log.Printf("[InventoryAlertScheduler] failed to query low-stock items: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	log.Printf("[InventoryAlertScheduler] found %d low-stock item(s) to alert", len(rows))

	// Group by business ID.
	byBusiness := make(map[uint][]inventoryAlertRow)
	for _, row := range rows {
		byBusiness[row.BusinessID] = append(byBusiness[row.BusinessID], row)
	}

	for businessID, items := range byBusiness {
		s.processBusinessAlerts(businessID, items)
	}
}

// lowStockQuery is the anti-duplication SQL that returns only items whose
// current_quantity <= reorder_threshold and that have not been alerted for the
// same alert_type in the last 24 hours.
//
// The two alert_type CASE expressions (SELECT projection and dedup NOT EXISTS
// subquery) MUST stay identical, otherwise deduplication breaks. A
// non-positive (zero OR negative) quantity classifies as out_of_stock; in
// "warn" availability mode an approved order can drive current_quantity below
// zero, so the test must be <= 0 rather than = 0.
//
// The INNER JOIN on inventory_settings gates every alert on:
//   - inventory_enabled = true  — business must have opted in to inventory tracking
//   - low_stock_warnings_enabled = true — business must have alerts switched on
//
// Businesses without a settings row are excluded by the INNER JOIN (equivalent
// to inventory disabled, which is the correct default — inventory_enabled
// defaults to false in InventorySettings).
const lowStockQuery = `
SELECT
    ii.business_id,
    ii.id                  AS item_id,
    ii.name                AS item_name,
    ii.unit,
    ii.current_quantity,
    ii.reorder_threshold,
    CASE WHEN ii.current_quantity <= 0 THEN 'out_of_stock' ELSE 'low_stock' END AS alert_type
FROM inventory_items ii
INNER JOIN inventory_settings iset
    ON iset.business_id             = ii.business_id
   AND iset.inventory_enabled       = true
   AND iset.low_stock_warnings_enabled = true
WHERE ii.is_active = true
  AND ii.current_quantity <= ii.reorder_threshold
  AND ii.reorder_threshold > 0
  AND NOT EXISTS (
      SELECT 1 FROM inventory_alert_logs ial
      WHERE ial.business_id        = ii.business_id
        AND ial.inventory_item_id  = ii.id
        AND ial.alert_type         = CASE WHEN ii.current_quantity <= 0 THEN 'out_of_stock' ELSE 'low_stock' END
        AND ial.alerted_at         > NOW() - INTERVAL '24 hours'
  )
`

// queryLowStockItems runs lowStockQuery and maps the rows.
func (s *InventoryAlertScheduler) queryLowStockItems() ([]inventoryAlertRow, error) {
	query := lowStockQuery
	var results []struct {
		BusinessID       uint    `gorm:"column:business_id"`
		ItemID           uint    `gorm:"column:item_id"`
		ItemName         string  `gorm:"column:item_name"`
		Unit             string  `gorm:"column:unit"`
		CurrentQuantity  float64 `gorm:"column:current_quantity"`
		ReorderThreshold float64 `gorm:"column:reorder_threshold"`
		AlertType        string  `gorm:"column:alert_type"`
	}

	if err := s.db.Raw(query).Scan(&results).Error; err != nil {
		return nil, err
	}

	rows := make([]inventoryAlertRow, 0, len(results))
	for _, r := range results {
		rows = append(rows, inventoryAlertRow{
			BusinessID:       r.BusinessID,
			ItemID:           r.ItemID,
			ItemName:         r.ItemName,
			Unit:             r.Unit,
			CurrentQuantity:  r.CurrentQuantity,
			ReorderThreshold: r.ReorderThreshold,
			AlertType:        r.AlertType,
		})
	}
	return rows, nil
}

// claimAlertLog inserts the alert-log row as the claim. The daily UNIQUE index
// (uq_inventory_alert_logs_daily on business_id, inventory_item_id, alert_type,
// alert_day) makes a duplicate insert a no-op (ON CONFLICT DO NOTHING);
// RowsAffected==1 means this caller created the row and should send. Replaces
// the read-then-insert race.
func (s *InventoryAlertScheduler) claimAlertLog(businessID, itemID uint, alertType string, now time.Time) bool {
	res := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&database.InventoryAlertLog{
		BusinessID:      businessID,
		InventoryItemID: itemID,
		AlertType:       alertType,
		AlertedAt:       now,
	})
	if res.Error != nil {
		log.Printf("[InventoryAlertScheduler] claim insert failed for item %d (business %d): %v", itemID, businessID, res.Error)
		return false
	}
	return res.RowsAffected == 1
}

// processBusinessAlerts logs alert records and dispatches notifications for a
// single business. The insert-claim pattern (claimAlertLog) is the authoritative
// dedup: only the goroutine that successfully inserts the log row sends the
// notification. A concurrent or repeat tick will get RowsAffected==0 and skip.
func (s *InventoryAlertScheduler) processBusinessAlerts(businessID uint, items []inventoryAlertRow) {
	now := time.Now().UTC()

	// Insert-claim each item. claimAlertLog (daily UNIQUE index) is the
	// authoritative dedup for the EMAIL, which has no other dedup store.
	var toSend []inventoryAlertRow
	for _, item := range items {
		if s.claimAlertLog(businessID, item.ItemID, item.AlertType, now) {
			toSend = append(toSend, item)
		}
	}
	if len(toSend) == 0 {
		return
	}

	// Enqueue the Telegram notifications through the SAME helper the on-demand
	// (order/adjustment) path uses, so both emit the canonical
	// inventoryLowStockEventID and the outbox unique key collapses cross-path
	// duplicates instead of double-alerting the operator (INV-L1).
	healthItems := make([]database.InventoryItemHealth, 0, len(toSend))
	for _, item := range toSend {
		healthItems = append(healthItems, database.InventoryItemHealth{
			ID:               item.ItemID,
			Name:             item.ItemName,
			Unit:             item.Unit,
			CurrentQuantity:  item.CurrentQuantity,
			ReorderThreshold: item.ReorderThreshold,
			Status:           item.AlertType,
		})
	}
	if _, err := EnqueueTelegramInventoryLowStockAlerts(businessID, healthItems, now); err != nil {
		log.Printf("[InventoryAlertScheduler] failed to enqueue plugin notifications for business %d: %v", businessID, err)
	}

	// Send email if we can resolve the business owner's address.
	s.sendAlertEmail(businessID, toSend, now)
}

// sendAlertEmail looks up the business, then sends a low-stock alert email to
// the owner. Errors are logged but do not block other processing.
func (s *InventoryAlertScheduler) sendAlertEmail(businessID uint, items []inventoryAlertRow, now time.Time) {
	var business database.Business
	if err := s.db.First(&business, businessID).Error; err != nil {
		log.Printf("[InventoryAlertScheduler] failed to load business %d for email: %v", businessID, err)
		return
	}
	if business.Email == "" {
		log.Printf("[InventoryAlertScheduler] business %d has no email address, skipping alert email", businessID)
		return
	}

	ownerName := business.OwnerName
	if ownerName == "" {
		ownerName = "Business Owner"
	}

	dashboardURL := BuildBusinessDashboardURL(&business)
	language := DetermineBusinessOwnerLanguage(&business)

	emailItems := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		emailItems = append(emailItems, map[string]interface{}{
			"name":              item.ItemName,
			"unit":              item.Unit,
			"current_quantity":  fmt.Sprintf("%.2f", item.CurrentQuantity),
			"reorder_threshold": fmt.Sprintf("%.2f", item.ReorderThreshold),
			"alert_type":        item.AlertType,
		})
	}

	if err := s.emailServer.SendLowStockAlertEmail(
		[]string{business.Email},
		ownerName,
		dashboardURL,
		language,
		emailItems,
	); err != nil {
		log.Printf("[InventoryAlertScheduler] failed to send low-stock alert email to business %d: %v", businessID, err)
		return
	}

	log.Printf("[InventoryAlertScheduler] sent low-stock alert email for business %d (%d item(s))", businessID, len(items))
}
