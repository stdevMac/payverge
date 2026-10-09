package crm

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupDeleteAccountDB(t *testing.T) (*gorm.DB, *Service) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.Customer{},
		&database.CustomerPreferences{},
		&database.CustomerAddress{},
		&database.CustomerBusiness{},
		&database.CustomerVisit{},
		&database.CustomerCommunication{},
		&database.Bill{},
		&database.DeliveryOrder{},
		&database.DeliveryStatusHistory{},
	))
	return db, NewService(db)
}

func TestDeleteCustomerAccount_MissingCustomer(t *testing.T) {
	_, svc := setupDeleteAccountDB(t)
	err := svc.DeleteCustomerAccount(424242)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestDeleteCustomerAccount_AnonymizesPII(t *testing.T) {
	db, svc := setupDeleteAccountDB(t)

	biz := &database.Business{
		BusinessId: "del-acct-biz", Name: "Delete Account Biz",
		OwnerAddress: "0xowner", IsActive: true,
	}
	require.NoError(t, db.Create(biz).Error)

	birthday := time.Date(1991, 4, 2, 0, 0, 0, 0, time.UTC)
	resetExpiry := time.Now().Add(time.Hour).UTC()
	customer := &database.Customer{
		Email:               "ada@example.com",
		PasswordHash:        "hashed-password",
		Name:                "Ada Lovelace",
		Phone:               "+15551212",
		WalletAddress:       "0xada",
		Birthday:            &birthday,
		ProfileImageURL:     "https://cdn.example/ada.png",
		IsActive:            true,
		EmailVerified:       true,
		VerificationToken:   "verify-token",
		PasswordResetToken:  "reset-token",
		PasswordResetExpiry: &resetExpiry,
	}
	require.NoError(t, db.Create(customer).Error)

	prefs := &database.CustomerPreferences{
		CustomerID:        customer.ID,
		PreferredLanguage: "en",
		PreferredCurrency: "USD",
	}
	require.NoError(t, db.Create(prefs).Error)

	addr := &database.CustomerAddress{
		CustomerID: customer.ID,
		Label:      "Home",
		Street:     "1 Analytical Engine",
		City:       "London",
		Country:    "UK",
	}
	require.NoError(t, db.Create(addr).Error)

	link := &database.CustomerBusiness{
		CustomerID:         customer.ID,
		BusinessID:         biz.ID,
		IsActive:           true,
		FirstVisitAt:       time.Now(),
		OptInMarketing:     true,
		OptInSMS:           true,
		OptInEmail:         true,
		Notes:              "allergic to shellfish, seat by the window",
		Allergies:          `["shellfish"]`,
		DietaryPreferences: `["vegetarian"]`,
		FavoriteItems:      `[12, 15]`,
		Tags:               `["vip"]`,
	}
	require.NoError(t, db.Create(link).Error)

	visit := &database.CustomerVisit{
		CustomerBusinessID: link.ID,
		VisitDate:          time.Now(),
		Feedback:           "call me at +15551212",
	}
	require.NoError(t, db.Create(visit).Error)

	comm := &database.CustomerCommunication{
		BusinessID:         biz.ID,
		CustomerBusinessID: link.ID,
		Type:               database.CommunicationTypeEmail,
		Status:             database.CommunicationStatusFailed,
		ErrorMessage:       "bounce ada@example.com",
	}
	require.NoError(t, db.Create(comm).Error)

	bill := &database.Bill{
		BusinessID:     biz.ID,
		BillNumber:     "B-del-acct",
		Status:         "open",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.Create(bill).Error)

	customerID := customer.ID
	delivered := newDeleteAccountDelivery(biz.ID, bill.ID, &customerID, "DEL-DONE", database.DeliveryStatusDelivered)
	inTransit := newDeleteAccountDelivery(biz.ID, bill.ID, &customerID, "DEL-LIVE", database.DeliveryStatusInTransit)
	require.NoError(t, db.Create(delivered).Error)
	require.NoError(t, db.Create(inTransit).Error)

	fixAt := time.Now().UTC()
	doneHistory := &database.DeliveryStatusHistory{
		DeliveryOrderID: delivered.ID,
		Status:          database.DeliveryStatusDelivered,
		Location:        &database.Location{Latitude: 51.5237, Longitude: -0.1585, Timestamp: &fixAt},
		Notes:           "left with doorman at apt 2",
		ChangedBy:       "driver",
	}
	liveHistory := &database.DeliveryStatusHistory{
		DeliveryOrderID: inTransit.ID,
		Status:          database.DeliveryStatusInTransit,
		Location:        &database.Location{Latitude: 51.5, Longitude: -0.12, Timestamp: &fixAt},
		Notes:           "en route",
		ChangedBy:       "driver",
	}
	require.NoError(t, db.Create(doneHistory).Error)
	require.NoError(t, db.Create(liveHistory).Error)

	require.NoError(t, svc.DeleteCustomerAccount(customer.ID))

	var persisted database.Customer
	require.NoError(t, db.First(&persisted, customer.ID).Error)
	assert.Equal(t, fmt.Sprintf("deleted-%d@deleted.invalid", customer.ID), persisted.Email)
	assert.Empty(t, persisted.Name)
	assert.Empty(t, persisted.Phone)
	assert.Empty(t, persisted.WalletAddress)
	assert.Nil(t, persisted.Birthday)
	assert.Empty(t, persisted.ProfileImageURL)
	assert.Empty(t, persisted.PasswordHash)
	assert.Empty(t, persisted.VerificationToken)
	assert.Empty(t, persisted.PasswordResetToken)
	assert.Nil(t, persisted.PasswordResetExpiry)
	assert.False(t, persisted.EmailVerified)
	assert.False(t, persisted.IsActive)

	var prefCount, addrCount int64
	require.NoError(t, db.Model(&database.CustomerPreferences{}).Where("customer_id = ?", customer.ID).Count(&prefCount).Error)
	require.NoError(t, db.Model(&database.CustomerAddress{}).Where("customer_id = ?", customer.ID).Count(&addrCount).Error)
	assert.Zero(t, prefCount)
	assert.Zero(t, addrCount)

	var persistedLink database.CustomerBusiness
	require.NoError(t, db.First(&persistedLink, link.ID).Error)
	assert.False(t, persistedLink.IsActive)
	assert.False(t, persistedLink.OptInMarketing)
	assert.False(t, persistedLink.OptInSMS)
	assert.False(t, persistedLink.OptInEmail)
	assert.Empty(t, persistedLink.Notes)
	assert.Empty(t, persistedLink.Allergies)
	assert.Empty(t, persistedLink.DietaryPreferences)
	assert.Empty(t, persistedLink.FavoriteItems)
	assert.Empty(t, persistedLink.Tags)

	var persistedVisit database.CustomerVisit
	require.NoError(t, db.First(&persistedVisit, visit.ID).Error)
	assert.Empty(t, persistedVisit.Feedback)

	var persistedComm database.CustomerCommunication
	require.NoError(t, db.First(&persistedComm, comm.ID).Error)
	assert.Empty(t, persistedComm.ErrorMessage)

	assertDeliveryAnonymized(t, db, delivered.ID)
	assertDeliveryUntouched(t, db, inTransit.ID, customer.ID)
	assertStatusHistoryScrubbed(t, db, doneHistory.ID)

	var keptHistory database.DeliveryStatusHistory
	require.NoError(t, db.First(&keptHistory, liveHistory.ID).Error)
	require.NotNil(t, keptHistory.Location)
	assert.InDelta(t, 51.5, keptHistory.Location.Latitude, 1e-9)
	assert.Equal(t, "en route", keptHistory.Notes)
}

func assertStatusHistoryScrubbed(t *testing.T, db *gorm.DB, historyID uint) {
	t.Helper()
	var row struct {
		Latitude  sql.NullFloat64
		Longitude sql.NullFloat64
		Timestamp sql.NullString
		Notes     string
	}
	require.NoError(t, db.Raw(`SELECT latitude, longitude, timestamp, notes FROM delivery_status_history WHERE id = ?`, historyID).Scan(&row).Error)
	assert.False(t, row.Latitude.Valid)
	assert.False(t, row.Longitude.Valid)
	assert.False(t, row.Timestamp.Valid)
	assert.Empty(t, row.Notes)
}

func newDeleteAccountDelivery(businessID, billID uint, customerID *uint, number string, status database.DeliveryStatus) *database.DeliveryOrder {
	return &database.DeliveryOrder{
		BusinessID:     businessID,
		BillID:         billID,
		CustomerID:     customerID,
		DeliveryNumber: number,
		DeliveryType:   database.DeliveryTypeInHouse,
		Status:         status,
		CustomerName:   "Ada Lovelace",
		CustomerPhone:  "+15551212",
		CustomerEmail:  "ada@example.com",
		DeliveryAddress: database.DeliveryAddress{
			Street:           "1 Analytical Engine",
			Apartment:        "2",
			City:             "London",
			State:            "LN",
			PostalCode:       "EC1",
			Country:          "UK",
			FormattedAddress: "1 Analytical Engine, London",
		},
		DeliveryInstructions: "ring the bell",
		DropoffLocation:      database.Location{Latitude: 51.5237, Longitude: -0.1585},
		CurrentLocation:      &database.Location{Latitude: 51.52, Longitude: -0.15},
	}
}

func assertDeliveryAnonymized(t *testing.T, db *gorm.DB, orderID uint) {
	t.Helper()
	var row struct {
		CustomerName             string
		CustomerPhone            string
		CustomerEmail            sql.NullString
		DeliveryStreet           sql.NullString
		DeliveryApartment        sql.NullString
		DeliveryCity             sql.NullString
		DeliveryState            sql.NullString
		DeliveryPostalCode       sql.NullString
		DeliveryCountry          sql.NullString
		DeliveryFormattedAddress sql.NullString
		DeliveryInstructions     sql.NullString
		CustomerID               sql.NullInt64
		DropoffLatitude          sql.NullFloat64
		DropoffLongitude         sql.NullFloat64
		CurrentLatitude          sql.NullFloat64
		CurrentLongitude         sql.NullFloat64
	}
	require.NoError(t, db.Raw(`
		SELECT customer_name, customer_phone, customer_email,
		       delivery_street, delivery_apartment, delivery_city, delivery_state,
		       delivery_postal_code, delivery_country, delivery_formatted_address,
		       delivery_instructions, customer_id,
		       dropoff_latitude, dropoff_longitude, current_latitude, current_longitude
		FROM delivery_orders WHERE id = ?`, orderID).Scan(&row).Error)
	assert.Equal(t, "Deleted customer", row.CustomerName)
	assert.Empty(t, row.CustomerPhone)
	assert.False(t, row.CustomerEmail.Valid)
	assert.False(t, row.DeliveryStreet.Valid)
	assert.False(t, row.DeliveryApartment.Valid)
	assert.False(t, row.DeliveryCity.Valid)
	assert.False(t, row.DeliveryState.Valid)
	assert.False(t, row.DeliveryPostalCode.Valid)
	assert.False(t, row.DeliveryCountry.Valid)
	assert.False(t, row.DeliveryFormattedAddress.Valid)
	assert.False(t, row.DeliveryInstructions.Valid)
	assert.False(t, row.CustomerID.Valid)
	assert.False(t, row.DropoffLatitude.Valid)
	assert.False(t, row.DropoffLongitude.Valid)
	assert.False(t, row.CurrentLatitude.Valid)
	assert.False(t, row.CurrentLongitude.Valid)
}

func assertDeliveryUntouched(t *testing.T, db *gorm.DB, orderID, customerID uint) {
	t.Helper()
	var row database.DeliveryOrder
	require.NoError(t, db.First(&row, orderID).Error)
	assert.Equal(t, "Ada Lovelace", row.CustomerName)
	assert.Equal(t, "+15551212", row.CustomerPhone)
	assert.Equal(t, "ada@example.com", row.CustomerEmail)
	assert.Equal(t, "ring the bell", row.DeliveryInstructions)
	assert.Equal(t, "1 Analytical Engine", row.DeliveryAddress.Street)
	require.NotNil(t, row.CustomerID)
	assert.Equal(t, customerID, *row.CustomerID)
}
