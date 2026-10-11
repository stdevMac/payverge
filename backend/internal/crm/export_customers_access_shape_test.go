package crm

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// createExportCustomer links one customer to businessID. share == nil leaves
// the customer without a preferences row (fail closed). A non-nil false is
// written with an explicit update because the column default is true.
func createExportCustomer(t *testing.T, db *gorm.DB, businessID uint, email, phone string, share *bool) {
	t.Helper()
	customer := &database.Customer{
		Email:        email,
		PasswordHash: "hash",
		Name:         "Export Customer",
		Phone:        phone,
		IsActive:     true,
	}
	require.NoError(t, db.Create(customer).Error)
	if share != nil {
		require.NoError(t, db.Create(&database.CustomerPreferences{
			CustomerID:              customer.ID,
			PreferredLanguage:       "en",
			PreferredCurrency:       "USD",
			ShareDataWithBusinesses: true,
		}).Error)
		if !*share {
			require.NoError(t, db.Model(&database.CustomerPreferences{}).
				Where("customer_id = ?", customer.ID).
				Update("share_data_with_businesses", false).Error)
		}
	}
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:    customer.ID,
		BusinessID:    businessID,
		IsActive:      true,
		FirstVisitAt:  time.Now().UTC(),
		LoyaltyPoints: 4,
		VisitCount:    2,
	}).Error)
}

func assertExportSQLShape(t *testing.T, statements []string) {
	t.Helper()
	require.NotEmpty(t, statements)
	for _, statement := range statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		assert.Contains(t, normalized, "limit", "export query must be paged: %s", statement)
		assert.NotContains(t, normalized, "password", "export must not select password material: %s", statement)
		assert.NotContains(t, normalized, "token", "export must not select tokens: %s", statement)
		assert.NotContains(t, normalized, "select *", "export must project columns: %s", statement)
	}
}

func TestExportBusinessCustomers_AccessShapeRedactsOptedOutPhone(t *testing.T) {
	rec := &lcrmSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db, _ := setupLCRMHandlerDB(t, rec)
	lcrmSeedBusiness(t, db, 100)

	optedIn := true
	optedOut := false
	createExportCustomer(t, db, 100, "in@example.com", "555-opted-in", &optedIn)
	createExportCustomer(t, db, 100, "out@example.com", "555-opted-out", &optedOut)
	createExportCustomer(t, db, 100, "none@example.com", "555-no-prefs", nil)

	rec.reset()
	var rows []CustomerExportRow
	err := NewService(db).ExportBusinessCustomers(100, func(row CustomerExportRow) error {
		rows = append(rows, row)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assertExportSQLShape(t, rec.statements)

	byEmail := map[string]CustomerExportRow{}
	for _, row := range rows {
		byEmail[row.Email] = row
	}
	assert.Equal(t, "555-opted-in", byEmail["in@example.com"].Phone)
	assert.Equal(t, "", byEmail["out@example.com"].Phone, "opted-out phone must be redacted")
	assert.Equal(t, "", byEmail["none@example.com"].Phone, "missing consent fails closed")
}

func TestExportBusinessCustomers_PagesPast500(t *testing.T) {
	rec := &lcrmSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db, _ := setupLCRMHandlerDB(t, rec)
	lcrmSeedBusiness(t, db, 7)

	const n = 501
	optedOut := false
	for i := 0; i < n; i++ {
		var share *bool
		phone := "555-keep"
		if i == 0 {
			share = &optedOut
			phone = "555-opted-out"
		}
		createExportCustomer(t, db, 7, fmt.Sprintf("page-%d@example.com", i), phone, share)
	}

	rec.reset()
	var rows []CustomerExportRow
	err := NewService(db).ExportBusinessCustomers(7, func(row CustomerExportRow) error {
		rows = append(rows, row)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, rows, n)
	require.Len(t, rec.statements, 2, "501 rows must take two pages of %d: %v", customerExportPageSize, rec.statements)
	assertExportSQLShape(t, rec.statements)

	var found bool
	for _, row := range rows {
		if row.Email == "page-0@example.com" {
			found = true
			assert.Equal(t, "", row.Phone, "opted-out phone must be redacted")
		}
	}
	require.True(t, found)
}
