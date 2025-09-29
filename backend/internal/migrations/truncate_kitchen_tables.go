package migrations

import (
	"gorm.io/gorm"
)

// TruncateKitchenTables removes all data from old kitchen tables and drops them
func TruncateKitchenTables(db *gorm.DB) error {
	// Drop old kitchen tables completely
	if err := db.Exec("DROP TABLE IF EXISTS kitchen_order_items").Error; err != nil {
		return err
	}
	
	if err := db.Exec("DROP TABLE IF EXISTS kitchen_orders").Error; err != nil {
		return err
	}
	
	return nil
}
