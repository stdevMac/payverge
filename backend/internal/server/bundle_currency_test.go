package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestResolveBundleCurrency(t *testing.T) {
	t.Parallel()

	got, err := resolveBundleCurrency("ARS", "")
	require.NoError(t, err)
	assert.Equal(t, "ARS", got)

	got, err = resolveBundleCurrency("ars", "ARS")
	require.NoError(t, err)
	assert.Equal(t, "ARS", got)

	got, err = resolveBundleCurrency("", "")
	require.NoError(t, err)
	assert.Equal(t, "USD", got)

	_, err = resolveBundleCurrency("ARS", "EUR")
	require.ErrorIs(t, err, errBundleCurrencyMismatch)
}

func seedBundleMenu(t *testing.T, businessID uint) {
	t.Helper()
	raw, err := json.Marshal([]database.MenuCategory{{
		ID:   "cat-main",
		Name: "Mains",
		Items: []database.MenuItem{
			{ID: "item-burger", Name: "Burger", Price: 20, IsAvailable: true},
		},
	}})
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: businessID,
		Categories: string(raw),
		IsActive:   true,
	}).Error)
}

func postBundle(t *testing.T, businessID uint, currency string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{
		"name":     "Combo",
		"price":    10,
		"currency": currency,
		"items":    []map[string]interface{}{{"menu_item_id": "item-burger", "quantity": 1}},
	})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", businessID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	CreateBundle(c)
	return w
}

func TestCreateBundle_StampsBusinessDefaultCurrencyWhenOmitted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Menu{}, &database.Bundle{}))
	business := createBusinessHandlerTestBusiness(t, "0xBundleOwner", "biz-bundle-default-currency")
	business.DefaultCurrency = "ARS"
	require.NoError(t, database.GetDB().Save(business).Error)
	seedBundleMenu(t, business.ID)

	w := postBundle(t, business.ID, "")
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var bundle database.Bundle
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &bundle))
	assert.Equal(t, "ARS", bundle.Currency)
}

func TestCreateBundle_RejectsNonDefaultCurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Menu{}, &database.Bundle{}))
	business := createBusinessHandlerTestBusiness(t, "0xBundleOwner2", "biz-bundle-reject-currency")
	business.DefaultCurrency = "ARS"
	require.NoError(t, database.GetDB().Save(business).Error)
	seedBundleMenu(t, business.ID)

	w := postBundle(t, business.ID, "EUR")
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "bundle_currency_mismatch")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Bundle{}).Where("business_id = ?", business.ID).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

func TestUpdateBundle_RejectsNonDefaultCurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Menu{}, &database.Bundle{}))
	business := createBusinessHandlerTestBusiness(t, "0xBundleOwner3", "biz-bundle-update-currency")
	business.DefaultCurrency = "ARS"
	require.NoError(t, database.GetDB().Save(business).Error)
	seedBundleMenu(t, business.ID)

	created := postBundle(t, business.ID, "ARS")
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var bundle database.Bundle
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &bundle))

	body, err := json.Marshal(map[string]interface{}{
		"name":     "Combo",
		"price":    10,
		"currency": "EUR",
		"items":    []map[string]interface{}{{"menu_item_id": "item-burger", "quantity": 1}},
	})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "bundleId", Value: fmt.Sprintf("%d", bundle.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	UpdateBundle(c)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "bundle_currency_mismatch")
}
