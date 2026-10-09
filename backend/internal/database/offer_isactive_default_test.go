package database

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupOfferDefaultTestDB points the package-global db at a fresh in-memory
// sqlite with the Offer table migrated straight from the model, so the
// struct-tag-driven GORM behavior under test (zero-value + default handling)
// is exercised exactly as it is in production.
func setupOfferDefaultTestDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := gormDB.AutoMigrate(&Business{}, &Offer{}); err != nil {
		t.Fatalf("auto-migrate Offer schema: %v", err)
	}
	db = gormDB
}

// TestCreateOffer_PersistsInactive is a regression guard for the GORM
// zero-value + default gotcha. Offer.IsActive must persist exactly the value
// the caller set. When the field carried `gorm:"default:true"`, GORM omitted
// an explicit `IsActive: false` (a zero-value bool) from the INSERT and the
// schema default flipped it back to true — so an operator asking for a draft
// offer got a live one. The default now lives solely in the handler.
func TestCreateOffer_PersistsInactive(t *testing.T) {
	setupOfferDefaultTestDB(t)

	inactive := &Offer{
		BusinessID:    1,
		Name:          "Draft Promo",
		DiscountType:  "percentage",
		DiscountValue: 10,
		IsActive:      false,
	}
	if err := CreateOffer(inactive); err != nil {
		t.Fatalf("CreateOffer(inactive): %v", err)
	}

	// Read a fresh row — never trust the in-memory struct, GORM can reflect
	// a schema default back into it after Create.
	var got Offer
	require.NoError(t, db.First(&got, inactive.ID).Error)
	require.False(t, got.IsActive, "an offer created with IsActive:false must persist as inactive")
}

// TestCreateOffer_PersistsActive guards the opposite direction so the fix does
// not over-correct: an explicitly-active offer stays active.
func TestCreateOffer_PersistsActive(t *testing.T) {
	setupOfferDefaultTestDB(t)

	active := &Offer{
		BusinessID:    1,
		Name:          "Live Promo",
		DiscountType:  "fixed",
		DiscountValue: 5,
		IsActive:      true,
	}
	require.NoError(t, CreateOffer(active))

	var got Offer
	require.NoError(t, db.First(&got, active.ID).Error)
	require.True(t, got.IsActive, "an offer created with IsActive:true must persist as active")
}
