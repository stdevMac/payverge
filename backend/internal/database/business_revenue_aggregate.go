package database

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ensureBusinessRevenueAggregateRowTx(tx *gorm.DB, businessID uint) error {
	if !tx.Migrator().HasTable(&BusinessRevenueAggregate{}) {
		return nil
	}

	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "business_id"}},
		DoNothing: true,
	}).Create(&BusinessRevenueAggregate{BusinessID: businessID}).Error
}
