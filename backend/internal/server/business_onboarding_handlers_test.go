package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCompleteOnboarding_IsIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupOnboardingHandlerTestDB(t)

	business := &database.Business{
		BusinessId:   "onboarding-complete-track",
		Name:         "Onboarding Complete Track",
		OwnerAddress: "0xowner",
		IsActive:     true,
	}
	require.NoError(t, database.GetDB().Create(business).Error)

	makeRequest := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: business.BusinessId}}
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		c.Set("address", "0xowner")
		CompleteOnboarding(c)
		return w
	}

	w1 := makeRequest()
	require.Equal(t, http.StatusOK, w1.Code)

	w2 := makeRequest()
	require.Equal(t, http.StatusOK, w2.Code)

	var refreshed database.Business
	require.NoError(t, database.GetDB().First(&refreshed, business.ID).Error)
	require.NotNil(t, refreshed.OnboardingCompletedAt)
}

func TestComputeSetupStatusAllEmpty(t *testing.T) {
	result := computeSetupStatus(setupStatusInput{
		businessName:      "",
		addressCity:       "",
		defaultCurrency:   "",
		tableCount:        0,
		menuCategories:    0,
		menuItems:         0,
		staffCount:        0,
		activePluginCount: 0,
	})

	require.False(t, result.Steps.BusinessProfile.Done)
	require.False(t, result.Steps.Tables.Done)
	require.False(t, result.Steps.Menu.Done)
	require.False(t, result.Steps.Staff.Done)
	require.False(t, result.Steps.Payment.Done)
	require.Equal(t, 0, result.CompletedCount)
	require.Equal(t, 5, result.TotalCount)
	require.False(t, result.RequiredDone)
	require.False(t, result.AllDone)
}

func TestComputeSetupStatusRequiredDone(t *testing.T) {
	result := computeSetupStatus(setupStatusInput{
		businessName:      "My Restaurant",
		addressCity:       "Dubai",
		defaultCurrency:   "AED",
		tableCount:        3,
		menuCategories:    2,
		menuItems:         5,
		staffCount:        0,
		activePluginCount: 0,
	})

	require.True(t, result.Steps.BusinessProfile.Done)
	require.True(t, result.Steps.Tables.Done)
	require.True(t, result.Steps.Menu.Done)
	require.False(t, result.Steps.Staff.Done)
	require.False(t, result.Steps.Payment.Done)
	require.Equal(t, 3, result.CompletedCount)
	require.Equal(t, 5, result.TotalCount)
	require.True(t, result.RequiredDone)
	require.False(t, result.AllDone)
}

// Layout is advisory only — published spaces never change go-live totals.
func TestComputeSetupStatusLayoutIsOptional(t *testing.T) {
	withoutLayout := computeSetupStatus(setupStatusInput{
		businessName:        "My Restaurant",
		defaultCurrency:     "AED",
		tableCount:          3,
		menuCategories:      2,
		menuItems:           5,
		publishedSpaceCount: 0,
	})
	withLayout := computeSetupStatus(setupStatusInput{
		businessName:        "My Restaurant",
		defaultCurrency:     "AED",
		tableCount:          3,
		menuCategories:      2,
		menuItems:           5,
		publishedSpaceCount: 2,
	})

	require.False(t, withoutLayout.Steps.Layout.Done)
	require.True(t, withLayout.Steps.Layout.Done)
	require.Equal(t, int64(2), withLayout.Steps.Layout.Count)
	// Totals / required / all_done identical regardless of layout.
	require.Equal(t, withoutLayout.CompletedCount, withLayout.CompletedCount)
	require.Equal(t, withoutLayout.TotalCount, withLayout.TotalCount)
	require.Equal(t, withoutLayout.RequiredDone, withLayout.RequiredDone)
	require.Equal(t, withoutLayout.AllDone, withLayout.AllDone)
	require.Equal(t, 5, withLayout.TotalCount)
}

func TestComputeSetupStatusAllDone(t *testing.T) {
	result := computeSetupStatus(setupStatusInput{
		businessName:      "My Restaurant",
		addressCity:       "Dubai",
		defaultCurrency:   "AED",
		tableCount:        6,
		menuCategories:    3,
		menuItems:         12,
		staffCount:        2,
		activePluginCount: 1,
	})

	require.True(t, result.Steps.BusinessProfile.Done)
	require.True(t, result.Steps.Tables.Done)
	require.True(t, result.Steps.Menu.Done)
	require.True(t, result.Steps.Staff.Done)
	require.True(t, result.Steps.Payment.Done)
	require.Equal(t, 5, result.CompletedCount)
	require.True(t, result.RequiredDone)
	require.True(t, result.AllDone)
}

// The payment step means "guests can actually pay digitally": crypto plugins
// only count once a settlement (payout) address exists; fiat plugins (Stripe
// etc.) count on their own.
func TestComputeSetupStatusPaymentRequiresPayoutRoute(t *testing.T) {
	tests := []struct {
		name                 string
		activePluginCount    int64
		cryptoPluginCount    int64
		hasSettlementAddress bool
		wantDone             bool
	}{
		{"no plugins at all", 0, 0, false, false},
		{"crypto plugins but no payout wallet", 2, 2, false, false},
		{"crypto plugins with payout wallet", 2, 2, true, true},
		{"fiat plugin only, no wallet needed", 1, 0, false, true},
		{"fiat + crypto, no wallet — fiat carries it", 3, 2, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeSetupStatus(setupStatusInput{
				activePluginCount:    tt.activePluginCount,
				cryptoPluginCount:    tt.cryptoPluginCount,
				hasSettlementAddress: tt.hasSettlementAddress,
			})
			require.Equal(t, tt.wantDone, result.Steps.Payment.Done)
			require.Equal(t, tt.hasSettlementAddress, result.Steps.Payment.HasSettlementAddress)
		})
	}
}

// P3: the Tables-tab "no orders yet" banner needs a cheap activation signal on
// the setup-status payload. Pure mapping test — the handler fills the input
// via database.BusinessHasPaidBill.
func TestComputeSetupStatusCarriesHasFirstPaidBill(t *testing.T) {
	withBill := computeSetupStatus(setupStatusInput{
		businessName: "B", defaultCurrency: "USD",
		tableCount: 1, menuCategories: 1, menuItems: 1,
		hasFirstPaidBill: true,
	})
	assert.True(t, withBill.HasFirstPaidBill)

	withoutBill := computeSetupStatus(setupStatusInput{
		businessName: "B", defaultCurrency: "USD",
		hasFirstPaidBill: false,
	})
	assert.False(t, withoutBill.HasFirstPaidBill)
}

// P3 2026-07-16: city is informational only — profile step needs name + currency.
func TestComputeSetupStatusBusinessProfileRequiresNameAndCurrency(t *testing.T) {
	tests := []struct {
		name            string
		businessName    string
		addressCity     string
		defaultCurrency string
		wantDone        bool
	}{
		{"all empty", "", "", "", false},
		{"only name", "Test", "", "", false},
		{"only address", "", "Dubai", "", false},
		{"only currency", "", "", "AED", false},
		{"name+address", "Test", "Dubai", "", false},
		{"name+currency", "Test", "", "AED", true},
		{"address+currency", "", "Dubai", "AED", false},
		{"all three", "Test", "Dubai", "AED", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeSetupStatus(setupStatusInput{
				businessName:    tt.businessName,
				addressCity:     tt.addressCity,
				defaultCurrency: tt.defaultCurrency,
			})
			require.Equal(t, tt.wantDone, result.Steps.BusinessProfile.Done)
		})
	}
}

func TestComputeSetupStatusMenuRequiresBothCategoriesAndItems(t *testing.T) {
	tests := []struct {
		name       string
		categories int
		items      int
		wantDone   bool
	}{
		{"zero/zero", 0, 0, false},
		{"categories but no items", 2, 0, false},
		{"items but no categories", 0, 3, false},
		{"both present", 1, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeSetupStatus(setupStatusInput{
				menuCategories: tt.categories,
				menuItems:      tt.items,
			})
			require.Equal(t, tt.wantDone, result.Steps.Menu.Done)
		})
	}
}

func setupOnboardingHandlerTestDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Staff{}))
	InitializeRBAC(database.GetDBWrapper())
}
