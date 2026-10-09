package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminBusinessDetailUsesRecognizedLedgerForRecentRevenue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupAccountingHandlerDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Order{}, &database.AdminAction{}))

	business := createAccountingHandlerBusiness(t, "0xAdminDetailOwner")
	now := time.Now()
	confirmedAt := now.Add(-2 * time.Hour)
	olderConfirmedAt := now.AddDate(0, 0, -40)

	partialBill := createAdminBusinessDetailBill(t, business.ID, "ADMIN-DETAIL-PARTIAL", 10000, 1000, database.BillStatusPartial, confirmedAt)
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        partialBill.ID,
		PayerAddr:     "0xadminpartial",
		Amount:        1000,
		TipAmount:     200,
		TxHash:        "admin_detail_partial_payment",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	olderBill := createAdminBusinessDetailBill(t, business.ID, "ADMIN-DETAIL-OLDER", 2000, 2000, database.BillStatusPaid, olderConfirmedAt)
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        olderBill.ID,
		PayerAddr:     "0xadminolder",
		Amount:        2000,
		TxHash:        "admin_detail_older_payment",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &olderConfirmedAt,
		CreatedAt:     olderConfirmedAt,
		UpdatedAt:     olderConfirmedAt,
	}).Error)
	router := gin.New()
	handler := NewAdminBusinessHandler(database.GetDB())
	router.GET("/admin/businesses/:id/detail", handler.GetBusinessDetail)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/admin/businesses/%d/detail", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Activity AdminBusinessDetailActivity `json:"activity"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, int64(1), response.Activity.RecentPaymentCount)
	assert.InDelta(t, 10.0, response.Activity.TotalRevenue, 0.01)
	assert.InDelta(t, 2.0, response.Activity.TotalTips, 0.01)
}

func TestAdminBusinessDetailProjectsWithoutSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupAccountingHandlerDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Order{}, &database.AdminAction{}))

	const (
		settlement = "0xsettle-secret-admin-detail"
		tipping    = "0xtip-secret-admin-detail"
		ownerEmail = "owner@venue.example"
		staffEmail = "jamie@floor.example"
	)

	business := createAccountingHandlerBusiness(t, "0xAdminDetailProjection")
	owner := &database.User{Email: ownerEmail, Name: "Hidden Owner"}
	require.NoError(t, database.GetDB().Create(owner).Error)
	business.UserID = &owner.ID
	business.SettlementAddr = settlement
	business.TippingAddr = tipping
	business.CustomURL = "venue-slug"
	business.Email = "contact@secret-venue.example"
	business.Phone = "+15551212"
	business.OwnerAddress = "0xowner-secret-wallet"
	require.NoError(t, database.GetDB().Save(business).Error)

	require.NoError(t, database.GetDB().Create(&database.Staff{
		BusinessID: business.ID,
		Email:      staffEmail,
		Name:       "Jamie",
		Role:       database.StaffRoleServer,
		IsActive:   true,
		InvitedBy:  "owner",
	}).Error)

	router := gin.New()
	handler := NewAdminBusinessHandler(database.GetDB())
	router.GET("/admin/businesses/:id/detail", handler.GetBusinessDetail)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/admin/businesses/%d/detail", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	for _, secret := range []string{
		settlement,
		tipping,
		`"settlement_address"`,
		`"tipping_address"`,
		ownerEmail,
		staffEmail,
		"Hidden Owner",
		"contact@secret-venue.example",
		"+15551212",
		"0xowner-secret-wallet",
	} {
		assert.NotContains(t, body, secret)
	}

	var response struct {
		Business AdminBusinessDetailBusiness `json:"business"`
		Owner    *AdminBusinessDetailOwner   `json:"owner"`
		Staff    []AdminBusinessDetailStaff  `json:"staff"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.True(t, response.Business.HasSettlementAddress)
	assert.True(t, response.Business.HasTippingAddress)
	assert.Equal(t, "venue-slug", response.Business.Slug)
	assert.Equal(t, business.ID, response.Business.ID)
	require.NotNil(t, response.Owner)
	assert.Equal(t, owner.ID, response.Owner.ID)
	assert.Equal(t, "venue.example", response.Owner.EmailDomain)
	require.Len(t, response.Staff, 1)
	assert.Equal(t, "floor.example", response.Staff[0].EmailDomain)
	assert.Equal(t, "Jamie", response.Staff[0].Name)
	assert.Equal(t, string(database.StaffRoleServer), response.Staff[0].Role)
}

func TestAdminBusinessDetailAcceptsBusinessSlug(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupAccountingHandlerDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Order{}, &database.AdminAction{}))

	business := createAccountingHandlerBusiness(t, "0xAdminSlugOwner")
	business.BusinessId = fmt.Sprintf("admin-slug-%d", time.Now().UnixNano())
	require.NoError(t, database.GetDB().Save(business).Error)

	router := gin.New()
	handler := NewAdminBusinessHandler(database.GetDB())
	router.GET("/admin/businesses/:id/detail", handler.GetBusinessDetail)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/admin/businesses/%s/detail", business.BusinessId), nil)
	require.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Business database.Business `json:"business"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, business.ID, response.Business.ID)
}

func createAdminBusinessDetailBill(t *testing.T, businessID uint, number string, total, paid int64, status database.BillStatus, at time.Time) database.Bill {
	t.Helper()

	bill := database.Bill{
		BusinessID:     businessID,
		BillNumber:     fmt.Sprintf("%s-%d", number, time.Now().UnixNano()),
		Subtotal:       total,
		TotalAmount:    total,
		PaidAmount:     paid,
		Status:         status,
		SettlementAddr: "settlement-" + number,
		TippingAddr:    "tipping-" + number,
		CreatedAt:      at,
		UpdatedAt:      at,
	}
	require.NoError(t, database.GetDB().Create(&bill).Error)
	return bill
}
