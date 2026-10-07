package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupQRBrandingDB(t testing.TB, tableCount int) uint {
	t.Helper()
	dsn := fmt.Sprintf("file:qr-branding-%d?mode=memory&cache=shared", time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	SetTestDB(gormDB)
	require.NoError(t, db.AutoMigrate(&Business{}, &Table{}))

	business := &Business{
		BusinessId:     fmt.Sprintf("qr-brand-%d", time.Now().UnixNano()),
		Name:           "QR Branding",
		OwnerAddress:   "0xQRBrandingOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, db.Create(business).Error)

	for i := 0; i < tableCount; i++ {
		require.NoError(t, db.Create(&Table{
			BusinessID:        business.ID,
			TableCode:         fmt.Sprintf("QR-%03d", i),
			Name:              fmt.Sprintf("Table %03d", i),
			IsActive:          true,
			QRForegroundColor: "#000000",
			QRBackgroundColor: "#FFFFFF",
		}).Error)
	}
	return business.ID
}

// TestApplyQRBrandingToAllTables verifies the bulk endpoint updates every table
// AND the business defaults in one transaction (replaces the 201-request fan-out).
func TestApplyQRBrandingToAllTables(t *testing.T) {
	businessID := setupQRBrandingDB(t, 50)

	branding := QRBranding{
		LogoURL:          "https://cdn.example/logo.png",
		ForegroundColor:  "#1a6b6a",
		BackgroundColor:  "#faf9f6",
		LogoSize:         25,
		ShowBusinessName: true,
		ShowTableName:    true,
		TextFont:         "Georgia",
	}

	affected, err := ApplyQRBrandingToAllTables(businessID, branding)
	require.NoError(t, err)
	assert.EqualValues(t, 50, affected)

	// Every table carries the new branding.
	var tables []Table
	require.NoError(t, db.Where("business_id = ?", businessID).Find(&tables).Error)
	require.Len(t, tables, 50)
	for _, tbl := range tables {
		assert.Equal(t, "#1a6b6a", tbl.QRForegroundColor)
		assert.Equal(t, "#faf9f6", tbl.QRBackgroundColor)
		assert.Equal(t, "Georgia", tbl.QRTextFont)
		assert.True(t, tbl.QRShowBusinessName)
	}

	// Business defaults updated too.
	var business Business
	require.NoError(t, db.First(&business, businessID).Error)
	assert.Equal(t, "#1a6b6a", business.DefaultQRForegroundColor)
	assert.Equal(t, "Georgia", business.DefaultQRTextFont)
	assert.True(t, business.DefaultQRShowTableName)
}

func BenchmarkApplyQRBrandingToAllTablesSQLite(b *testing.B) {
	businessID := setupQRBrandingDB(b, 200)
	branding := QRBranding{ForegroundColor: "#1a6b6a", BackgroundColor: "#faf9f6", TextFont: "Georgia"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ApplyQRBrandingToAllTables(businessID, branding); err != nil {
			b.Fatal(err)
		}
	}
}
