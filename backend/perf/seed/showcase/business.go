package main

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// seedBusiness upserts the hero "Trattoria Bella Vista" business keyed by
// `business_id = showcase-bellavista`. If the row already exists the existing
// settings (including any manual edits) are left untouched.
func seedBusiness(ctx context.Context, db *gorm.DB) error {
	lat := 37.7976
	lon := -122.4090

	biz := database.Business{
		BusinessId:   BusinessSeedID,
		CustomURL:    BusinessSeedID,
		OwnerAddress: PlaceholderAddress,
		OwnerName:    "Marco Rossi",
		Name:         "Trattoria Bella Vista",
		Logo:         "",
		Description:  "Family-run Italian kitchen serving handmade pasta, wood-fired pizza, and Tuscan classics in the heart of North Beach. Open since 2014.",
		Phone:        "+1 (415) 555-0142",
		Email:        "ciao@trattoriabellavista.example",
		Website:      "https://trattoriabellavista.example",
		Address: database.BusinessAddress{
			Street:     "247 Marina Way",
			City:       "San Francisco",
			State:      "CA",
			PostalCode: "94133",
			Country:    "United States",
		},
		SettlementAddr:       PlaceholderAddress,
		TippingAddr:          PlaceholderAddress,
		TaxRate:              0.0875,
		ServiceFeeRate:       0,
		TaxInclusive:         false,
		ServiceInclusive:     false,
		IsActive:             true,
		DefaultCurrency:      "USD",
		DisplayCurrency:      "USD",
		DefaultLanguage:      "en",
		SourceLanguage:       "en",
		BusinessPageEnabled:  true,
		ShowReviews:          true,
		GoogleReviewsEnabled: false,
		Timezone:             "America/Los_Angeles",
		CounterEnabled:       false,
		KitchenEnabled:       true,
		OrdersEnabled:        true,
		CRMEnabled:           true,
		Latitude:             &lat,
		Longitude:            &lon,
		WelcomeMessage:       "Benvenuti! Welcome to our table.",
		AboutStory:           "Three generations of the Rossi family bring Tuscany to San Francisco. Every pasta is rolled by hand each morning, every loaf baked in our wood-fired oven, every sauce simmered the way Nonna taught us. We source produce from local farms and import what we can't grow ourselves — olive oil from Lucca, prosciutto from Parma, parmigiano from Reggio Emilia.",
		ShowWelcomeMessage:   true,
		ShowAboutStory:       true,
		ShowGallery:          true,
		ShowOperatingHours:   true,
		ShowSpecialFeatures:  true,
		DesignSettings: database.BusinessDesignSettings{
			PrimaryColor:      "#1a6b6a",
			SecondaryColor:    "#c0392b",
			FontFamily:        "DM Sans",
			Theme:             "light",
			MenuLayout:        "grid",
			ShowImages:        true,
			ShowDescriptions:  true,
			HeaderStyle:       "banner",
			CornerRadius:      "medium",
			ShadowIntensity:   "subtle",
			BackgroundPattern: "none",
			PatternOpacity:    0.1,
		},
		AiSettings: database.BusinessAiSettings{
			AiEnabled:             true,
			AiName:                "Lucia",
			AiPriority:            "balanced",
			SpecialInstructions:   "Be warm and Italian. Recommend the daily handmade pasta when guests are deciding. If they ask for wine, suggest a glass of house Chianti with red sauces and Pinot Grigio with seafood.",
			BusinessPageAiEnabled: true,
		},
		DefaultQRForegroundColor:  "#1a6b6a",
		DefaultQRBackgroundColor:  "#faf9f6",
		DefaultQRLogoSize:         22,
		DefaultQRShowBusinessName: true,
		DefaultQRShowTableName:    true,
		DefaultQRTextFont:         "Verdana",
		OnboardingState:           database.JSONRawMessage([]byte(`{}`)),
	}

	if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "business_id"}},
			DoNothing: true,
		}).
		Create(&biz).Error; err != nil {
		return fmt.Errorf("create business: %w", err)
	}
	return nil
}

// attachOwner links the showcase business to an existing user by email.
// We update `user_id` (the OAuth/email account linkage) and `owner_address`
// when the user has a wallet on file. If no user matches the email we log
// a warning and leave the placeholder owner in place — the seed itself is
// still useful, the operator just has to log in via another path.
func attachOwner(ctx context.Context, db *gorm.DB, bizID uint, email string) error {
	var user database.User
	err := db.WithContext(ctx).
		Where("email = ?", email).
		First(&user).Error
	if err == gorm.ErrRecordNotFound {
		fmt.Printf("warning: no user with email %q — leaving placeholder owner. Register %q first, then re-run.\n", email, email)
		return nil
	}
	if err != nil {
		return fmt.Errorf("lookup user %q: %w", email, err)
	}

	// Note: keys are DB column names (snake_case of the Go field name), NOT
	// the JSON tag. SettlementAddr → settlement_addr, TippingAddr →
	// tipping_addr. GORM doesn't translate JSON tags for map-based Updates.
	updates := map[string]any{
		"user_id":    user.ID,
		"owner_name": firstNonEmpty(user.Name, "Marco Rossi"),
	}
	if user.Address != "" {
		updates["owner_address"] = user.Address
		updates["settlement_addr"] = user.Address
		updates["tipping_addr"] = user.Address
	}
	if err := db.WithContext(ctx).
		Model(&database.Business{}).
		Where("id = ?", bizID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("update business owner: %w", err)
	}
	fmt.Printf("attached showcase business to user_id=%d (%s)\n", user.ID, email)
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// loadBusiness returns the showcase business row so callers can use its
// auto-assigned `id` column as the FK target for downstream rows.
func loadBusiness(ctx context.Context, db *gorm.DB) (database.Business, error) {
	var biz database.Business
	if err := db.WithContext(ctx).
		Where("business_id = ?", BusinessSeedID).
		First(&biz).Error; err != nil {
		return database.Business{}, fmt.Errorf("load showcase business: %w", err)
	}
	return biz, nil
}
