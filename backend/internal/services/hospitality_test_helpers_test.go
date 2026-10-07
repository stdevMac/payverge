package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupHospitalityServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	database.InitTestDB(db)

	err = db.AutoMigrate(
		&database.Business{},
		&database.BusinessOperatingHours{},
		&database.BusinessOperatingException{},
		&database.Menu{},
		&database.Offer{},
		&database.Bundle{},
		&database.Table{},
		&database.ReservationSettings{},
		&database.TableReservation{},
		&database.ReservationStatusHistory{},
		&database.Bill{},
		&database.Payment{},
		&database.Order{},
		&database.DeliverySettings{},
		&database.DeliveryZone{},
		&database.DeliveryOrder{},
		&database.DeliveryStatusHistory{},
		&database.DeliveryDriver{},
		&database.Customer{},
		&database.CustomerBusiness{},
		&database.Staff{},
		&database.PluginNotificationDelivery{},
		&database.PluginNotificationDeliveryAttempt{},
		&database.BillHistoryEvent{},
		&database.InventorySettings{},
		&database.InventoryItem{},
		&database.InventoryRecipe{},
		&database.InventoryMovement{},
	)
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS bill_items (
			id TEXT PRIMARY KEY,
			bill_id INTEGER NOT NULL,
			menu_item_id TEXT DEFAULT '',
			name TEXT NOT NULL,
			price REAL NOT NULL,
			quantity INTEGER NOT NULL,
			options TEXT,
			item_type TEXT DEFAULT 'menu_item',
			bundle_id INTEGER,
			parent_bundle_id INTEGER,
			source_offer_id INTEGER,
			order_id INTEGER,
			subtotal REAL NOT NULL,
			created_at DATETIME
		)
	`).Error)

	return db
}

func createTestHospitalityBusiness(t *testing.T, db *gorm.DB, slug string) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:          fmt.Sprintf("biz_%s", slug),
		OwnerAddress:        fmt.Sprintf("owner_%s", slug),
		Name:                fmt.Sprintf("Business %s", slug),
		SettlementAddr:      "0xsettlement",
		TippingAddr:         "0xtipping",
		Timezone:            "UTC",
		IsActive:            true,
		TaxRate:             0.08,
		ServiceFeeRate:      0.1,
		BusinessPageEnabled: true,
	}
	require.NoError(t, db.Create(business).Error)

	// Use equal open/close (00:00–00:00). validateDeliveryWindow treats that as
	// an overnight 24h window, so hospitality tests stay inside active hours at
	// any wall-clock second — including the final minute of the UTC day where
	// CloseTime "23:59" (parsed as 23:59:00) used to reject 23:59:01–23:59:59.
	for day := 0; day < 7; day++ {
		hours := &database.BusinessOperatingHours{
			BusinessID: business.ID,
			DayOfWeek:  day,
			OpenTime:   "00:00",
			CloseTime:  "00:00",
			IsClosed:   false,
		}
		require.NoError(t, db.Create(hours).Error)
	}

	return business
}

func createTestTable(t *testing.T, db *gorm.DB, businessID uint, code string, capacity int) *database.Table {
	t.Helper()

	table := &database.Table{
		BusinessID:   businessID,
		TableCode:    code,
		Name:         code,
		Capacity:     capacity,
		IsActive:     true,
		IsReservable: true,
	}
	require.NoError(t, db.Create(table).Error)
	return table
}

func createTestHospitalityMenu(t *testing.T, db *gorm.DB, businessID uint, items ...database.MenuItem) *database.Menu {
	t.Helper()

	if len(items) == 0 {
		items = []database.MenuItem{
			{ID: "burger", Name: "Burger", Price: 12, IsAvailable: true},
			{ID: "fries", Name: "Fries", Price: 11, IsAvailable: true},
			{ID: "pizza", Name: "Pizza", Price: 16, IsAvailable: true},
			{ID: "cola", Name: "Cola", Price: 4, IsAvailable: true},
		}
	}

	categories := []database.MenuCategory{
		{
			ID:    "mains",
			Name:  "Mains",
			Items: items,
		},
	}
	payload, err := json.Marshal(categories)
	require.NoError(t, err)

	menu := &database.Menu{
		BusinessID: businessID,
		Categories: string(payload),
		IsActive:   true,
		Version:    1,
	}
	require.NoError(t, db.Create(menu).Error)
	return menu
}

func configureReservationSettings(t *testing.T, businessID uint, mutate func(*database.ReservationSettings)) *database.ReservationSettings {
	t.Helper()

	settings, err := database.GetReservationSettings(businessID)
	require.NoError(t, err)

	settings.Enabled = true
	settings.MinAdvanceMinutes = 0
	settings.MaxAdvanceDays = 30
	settings.MinPartySize = 1
	settings.MaxPartySize = 12
	settings.DefaultDuration = 90
	settings.SlotIntervalMinutes = 30
	settings.ServiceBufferMinutes = 0
	settings.MaxCoversPerSlot = 0
	settings.AutoAssignTables = true
	settings.ApprovalMode = database.ReservationApprovalAuto
	settings.AllowWaitlist = true
	settings.ExternalPartnerLinks = database.JSONRawMessage("[]")
	if mutate != nil {
		mutate(settings)
	}

	require.NoError(t, database.UpdateReservationSettings(settings))
	return settings
}

func createDeliverySettings(t *testing.T, db *gorm.DB, businessID uint, mutate func(*database.DeliverySettings)) *database.DeliverySettings {
	t.Helper()

	settings := &database.DeliverySettings{
		BusinessID:                  businessID,
		DeliveryEnabled:             true,
		InHouseDeliveryEnabled:      true,
		ThirdPartyEnabled:           true,
		FlatDeliveryFee:             5,
		FreeDeliveryMinimum:         0,
		MinimumOrderAmount:          10,
		EstimatedPrepTime:           20,
		MaxConcurrentDeliveries:     5,
		DeliveryHoursSameAsBusiness: false,
		// Equal start/end is the overnight 24h form — see createTestHospitalityBusiness.
		// "23:59" ends at 23:59:00 and flakes CI in the last wall-clock minute of the day.
		DeliveryStartTime:    "00:00",
		DeliveryEndTime:      "00:00",
		ExternalPartnerLinks: database.JSONRawMessage(`[{"name":"Partner","url":"https://partner.example"}]`),
	}
	if mutate != nil {
		mutate(settings)
	}
	// Capture before Create: GORM's default:true omits a false zero-value from
	// INSERT and then reflects the schema default back into the struct, so reading
	// settings.DeliveryHoursSameAsBusiness after Create would re-write true.
	wantSameAsBusiness := settings.DeliveryHoursSameAsBusiness
	require.NoError(t, db.Create(settings).Error)
	require.NoError(t, db.Model(settings).UpdateColumn(
		"delivery_hours_same_as_business", wantSameAsBusiness,
	).Error)
	settings.DeliveryHoursSameAsBusiness = wantSameAsBusiness
	return settings
}

func createDeliveryZone(t *testing.T, db *gorm.DB, businessID uint, name string, mutate func(*database.DeliveryZone)) *database.DeliveryZone {
	t.Helper()

	zone := &database.DeliveryZone{
		BusinessID:          businessID,
		Name:                name,
		Description:         name,
		Boundaries:          `{"cities":["testville"],"postal_codes":["12345"]}`,
		DeliveryFee:         700,  // $7.00
		MinimumOrderAmount:  2000, // $20.00
		EstimatedTime:       18,
		Priority:            1,
		CutoffBufferMinutes: 0,
		IsActive:            true,
	}
	if mutate != nil {
		mutate(zone)
	}
	require.NoError(t, db.Create(zone).Error)
	return zone
}

// pullReservationIntoArrivalWindow moves a reservation's time to just-started so
// the L1-13 temporal gate on seat/check-in and no-show transitions is satisfied.
// Creation still uses a known-good future slot, so availability validation is
// exercised unchanged — only the transition under test becomes reachable.
func pullReservationIntoArrivalWindow(t *testing.T, db *gorm.DB, reservationID uint) {
	t.Helper()
	require.NoError(t, db.Model(&database.TableReservation{}).
		Where("id = ?", reservationID).
		Update("reservation_time", time.Now().UTC().Add(-5*time.Minute)).Error)
}

func nextDayAt(hour, minute int) time.Time {
	base := time.Now().UTC().AddDate(0, 0, 1)
	return time.Date(base.Year(), base.Month(), base.Day(), hour, minute, 0, 0, time.UTC)
}
