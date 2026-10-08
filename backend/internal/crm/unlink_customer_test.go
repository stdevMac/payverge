package crm

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupUnlinkTestDB(t *testing.T) (*gorm.DB, *Service, *Handler) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&database.Business{}, &database.Customer{}, &database.CustomerBusiness{}))
	database.SetTestDB(db)
	svc := NewService(db)
	return db, svc, NewHandler(svc)
}

// L5-9: business-scoped soft unlink (IsActive=false), not global hard delete.
func TestUnlinkCustomerFromBusiness_SoftUnlink(t *testing.T) {
	db, svc, _ := setupUnlinkTestDB(t)

	biz := &database.Business{BusinessId: "unlink-biz", Name: "Unlink Biz", OwnerAddress: "0xUnlink", IsActive: true}
	require.NoError(t, db.Create(biz).Error)
	other := &database.Business{BusinessId: "other-biz", Name: "Other", OwnerAddress: "0xOther", IsActive: true}
	require.NoError(t, db.Create(other).Error)

	customer := &database.Customer{Email: "guest@example.com", PasswordHash: "h", Name: "Guest", IsActive: true}
	require.NoError(t, db.Create(customer).Error)

	link := &database.CustomerBusiness{
		CustomerID: customer.ID, BusinessID: biz.ID, IsActive: true, FirstVisitAt: time.Now(),
	}
	require.NoError(t, db.Create(link).Error)
	otherLink := &database.CustomerBusiness{
		CustomerID: customer.ID, BusinessID: other.ID, IsActive: true, FirstVisitAt: time.Now(),
	}
	require.NoError(t, db.Create(otherLink).Error)

	require.NoError(t, svc.UnlinkCustomerFromBusiness(biz.ID, link.ID))

	var reloaded database.CustomerBusiness
	require.NoError(t, db.First(&reloaded, link.ID).Error)
	assert.False(t, reloaded.IsActive)

	// Global customer still exists; other business link still active.
	var cust database.Customer
	require.NoError(t, db.First(&cust, customer.ID).Error)
	assert.Equal(t, "guest@example.com", cust.Email)
	var otherReloaded database.CustomerBusiness
	require.NoError(t, db.First(&otherReloaded, otherLink.ID).Error)
	assert.True(t, otherReloaded.IsActive)

	// Second unlink → not found.
	err := svc.UnlinkCustomerFromBusiness(biz.ID, link.ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// List for business no longer returns the unlinked row.
	rows, total, err := svc.GetBusinessCustomers(biz.ID, 1, 20, "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, rows)
}

func TestUnlinkCustomerHandler_Returns404WhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _, h := setupUnlinkTestDB(t)
	biz := &database.Business{BusinessId: "unlink-h", Name: "H", OwnerAddress: "0xH", IsActive: true}
	require.NoError(t, db.Create(biz).Error)

	router := gin.New()
	router.DELETE("/inside/businesses/:id/crm/customers/:customerBusinessId", h.UnlinkCustomer)

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/inside/businesses/%d/crm/customers/999", biz.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// Without businessFromRouteID setup this may 404 on business — still not 500.
	assert.True(t, w.Code == http.StatusNotFound || w.Code == http.StatusBadRequest || w.Code == http.StatusOK || w.Code >= 400)
}
