package services

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupOwnerLangTestDB opens an in-memory SQLite DB with the Business + User
// tables migrated and wired as the global DB, so DetermineBusinessOwnerLanguage
// can resolve the owner's User.LanguageSelected.
func setupOwnerLangTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&database.Business{}, &database.User{}))
	database.SetTestDB(db)
	return db
}

// resolveOwnerEmailLanguage maps any business language onto the email template
// family that actually ships (eng/es/es_ar); see EMAIL-1.
func TestResolveOwnerEmailLanguage_FamilyResolution(t *testing.T) {
	cases := map[string]string{
		"es-AR": "es_ar",
		"es_ar": "es_ar",
		"ES-AR": "es_ar",
		"es":    "es",
		"ES":    "es",
		"en":    "eng",
		"fr":    "eng",
		"":      "eng",
		"zz":    "eng",
	}
	for in, want := range cases {
		assert.Equalf(t, want, resolveOwnerEmailLanguage(in), "resolveOwnerEmailLanguage(%q)", in)
	}
}

// EMAIL-2: owner-facing emails must key on the owner's UI locale
// (User.LanguageSelected via DetermineBusinessOwnerLanguage), NOT the customer
// display field (business.DefaultLanguage). An owner whose account language is
// es-AR with a business.DefaultLanguage of "en" must receive the Rioplatense
// email family.
func TestBusinessOwnerEmailLanguage_PrefersOwnerUILocaleOverCustomerDefault(t *testing.T) {
	setupOwnerLangTestDB(t)

	userID := uint(0)
	owner := &database.User{
		Email:            "owner@resto.ar",
		Address:          "0xowner",
		LanguageSelected: "es-AR",
	}
	require.NoError(t, database.GetDB().Create(owner).Error)
	userID = owner.ID

	business := &database.Business{
		BusinessId:      "owner-lang-es-ar",
		Name:            "Resto AR",
		OwnerName:       "Owner",
		Email:           "owner@resto.ar",
		UserID:          &userID,
		DefaultLanguage: "en", // customer display language — must NOT drive owner emails
	}
	require.NoError(t, database.GetDB().Create(business).Error)

	// DetermineBusinessOwnerLanguage resolves the owner's UI locale...
	assert.Equal(t, "es-AR", DetermineBusinessOwnerLanguage(business))
	// ...and the combined owner-email helper resolves it to the es_ar family,
	// not English (which business.DefaultLanguage="en" would have produced).
	assert.Equal(t, "es_ar", BusinessOwnerEmailLanguage(business))
}

// When there is no owner User row, the helper falls back to business.DefaultLanguage
// then the email family resolver.
func TestBusinessOwnerEmailLanguage_FallsBackToDefaultLanguage(t *testing.T) {
	setupOwnerLangTestDB(t)

	business := &database.Business{
		BusinessId:      "owner-lang-fallback",
		Name:            "Resto",
		Email:           "owner@resto.com",
		DefaultLanguage: "es-AR",
	}
	require.NoError(t, database.GetDB().Create(business).Error)

	assert.Equal(t, "es_ar", BusinessOwnerEmailLanguage(business))
}
