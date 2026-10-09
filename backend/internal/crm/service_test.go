package crm

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestRegisterCustomer_NormalizesEmailAndName(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	service := NewService(db)

	customer, err := service.RegisterCustomer("  MixedCase@Example.com ", "password123", "  Test User  ")
	require.NoError(t, err)

	assert.Equal(t, "mixedcase@example.com", customer.Email)
	assert.Equal(t, "Test User", customer.Name)

	authed, err := service.AuthenticateCustomer("MIXEDCASE@EXAMPLE.COM", "password123")
	require.NoError(t, err)
	assert.Equal(t, customer.ID, authed.ID)
	assert.Equal(t, "mixedcase@example.com", authed.Email)
}

func TestRegisterCustomer_DefaultsShareDataToFalse(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	service := NewService(db)

	customer, err := service.RegisterCustomer("optin-default@example.com", "password123", "Opt In Default")
	require.NoError(t, err)
	require.NotNil(t, customer.Preferences, "response must include reloaded preferences")
	assert.False(t, customer.Preferences.ShareDataWithBusinesses, "new customers default to sharing OFF")

	var prefs database.CustomerPreferences
	require.NoError(t, db.Where("customer_id = ?", customer.ID).First(&prefs).Error)
	assert.False(t, prefs.ShareDataWithBusinesses)

	// Exactly one preferences row for the new customer.
	var prefCount int64
	require.NoError(t, db.Model(&database.CustomerPreferences{}).Where("customer_id = ?", customer.ID).Count(&prefCount).Error)
	assert.EqualValues(t, 1, prefCount)
}

func TestUpdateCustomerPreferences_UpsertsMissingRow(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	service := NewService(db)
	customer := createCRMTestCustomer(t, db, "no-prefs-yet@example.com")

	// No preferences row yet — Updates used to silently touch zero rows.
	var before int64
	require.NoError(t, db.Model(&database.CustomerPreferences{}).Where("customer_id = ?", customer.ID).Count(&before).Error)
	require.EqualValues(t, 0, before)

	err := service.UpdateCustomerPreferences(customer.ID, map[string]interface{}{
		"share_data_with_businesses": true,
		"preferred_language":         "es",
	})
	require.NoError(t, err)

	var prefs database.CustomerPreferences
	require.NoError(t, db.Where("customer_id = ?", customer.ID).First(&prefs).Error)
	assert.True(t, prefs.ShareDataWithBusinesses, "upsert must apply the update")
	assert.Equal(t, "es", prefs.PreferredLanguage)

	// Second update on the existing row must not create duplicates.
	err = service.UpdateCustomerPreferences(customer.ID, map[string]interface{}{
		"share_data_with_businesses": false,
	})
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.Model(&database.CustomerPreferences{}).Where("customer_id = ?", customer.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, db.Where("customer_id = ?", customer.ID).First(&prefs).Error)
	assert.False(t, prefs.ShareDataWithBusinesses)
}

func TestConcurrent_RegisterCustomer_SameEmailOneCustomerOnePrefs(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	// Allow concurrent transactions on the shared in-memory SQLite DB.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)

	service := NewService(db)
	const email = "race@example.com"
	const n = 2

	var wg sync.WaitGroup
	errs := make(chan error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			var regErr error
			// In-memory SQLite + bcrypt can return "database table is locked"
			// for both writers under -race. Retry only that substrate error so
			// the uniqueness contract still has a winner.
			for attempt := 0; attempt < 12; attempt++ {
				_, regErr = service.RegisterCustomer(email, "password123", "Race Customer")
				if regErr == nil || !isSQLiteLockError(regErr) {
					break
				}
				time.Sleep(time.Duration(20*(attempt+1)) * time.Millisecond)
			}
			errs <- regErr
		}()
	}
	wg.Wait()
	close(errs)

	successes := 0
	for e := range errs {
		if e == nil {
			successes++
		}
	}
	require.Equal(t, 1, successes, "exactly one concurrent registration must win")

	var customers int64
	require.NoError(t, db.Model(&database.Customer{}).Where("email = ?", email).Count(&customers).Error)
	assert.EqualValues(t, 1, customers, "must not create duplicate customers")

	var prefs int64
	require.NoError(t, db.Model(&database.CustomerPreferences{}).
		Joins("JOIN customers ON customers.id = customer_preferences.customer_id").
		Where("customers.email = ?", email).
		Count(&prefs).Error)
	assert.EqualValues(t, 1, prefs, "must not create duplicate preference rows")

	var pref database.CustomerPreferences
	require.NoError(t, db.Joins("JOIN customers ON customers.id = customer_preferences.customer_id").
		Where("customers.email = ?", email).
		First(&pref).Error)
	assert.False(t, pref.ShareDataWithBusinesses, "winner's prefs must be opt-in default (false)")
}

func TestConnectCustomerToBusinessReactivatesInactiveConnection(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	service := NewService(db)
	business := createCRMTestBusiness(t, db, "reactivate-biz", "0xReactivateOwner", "Reactivate Biz")
	customer := createCRMTestCustomer(t, db, "reactivate@example.com")
	firstVisitAt := time.Now().Add(-30 * 24 * time.Hour)
	connection := &database.CustomerBusiness{
		CustomerID:         customer.ID,
		BusinessID:         business.ID,
		LoyaltyPoints:      120,
		TotalSpent:         45.50,
		VisitCount:         4,
		FirstVisitAt:       firstVisitAt,
		OptInMarketing:     false,
		OptInEmail:         false,
		IsActive:           false,
		DietaryPreferences: `["vegetarian"]`,
	}
	require.NoError(t, db.Create(connection).Error)
	require.NoError(t, db.Model(&database.CustomerBusiness{}).Where("id = ?", connection.ID).Updates(map[string]interface{}{
		"is_active":        false,
		"opt_in_marketing": false,
		"opt_in_email":     false,
	}).Error)

	reactivated, err := service.ConnectCustomerToBusiness(customer.ID, business.ID, true)
	require.NoError(t, err)
	require.NotNil(t, reactivated)
	require.Equal(t, connection.ID, reactivated.ID)
	require.True(t, reactivated.IsActive)
	require.True(t, reactivated.OptInMarketing)
	require.True(t, reactivated.OptInEmail)
	require.Equal(t, 120, reactivated.LoyaltyPoints)
	require.InDelta(t, 45.50, reactivated.TotalSpent, 0.001)
	require.Equal(t, 4, reactivated.VisitCount)
	require.Equal(t, `["vegetarian"]`, reactivated.DietaryPreferences)

	var visibleCount int64
	require.NoError(t, db.Model(&database.CustomerBusiness{}).
		Where("customer_id = ? AND business_id = ? AND is_active = ?", customer.ID, business.ID, true).
		Count(&visibleCount).Error)
	require.EqualValues(t, 1, visibleCount)
}
