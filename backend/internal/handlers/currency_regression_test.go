package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func setupCurrencyHandlerTestDB(t *testing.T) *CurrencyHandler {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)

	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.BusinessLanguage{},
		&database.Translation{},
	))

	dbw := database.GetDBWrapper()
	return NewCurrencyHandler(dbw, nil, services.NewTranslationService(dbw, ""))
}

func createCurrencyTestBusiness(t *testing.T, ownerAddress string) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:     "biz-currency-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		OwnerAddress:   ownerAddress,
		Name:           "Currency Test Business",
		SettlementAddr: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		TippingAddr:    "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
	}

	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

func TestCurrencyHandler_GetTranslatedContentRequiresBusinessID(t *testing.T) {
	handler := setupCurrencyHandlerTestDB(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/translations?entity_type=category&entity_id=1&field_name=name&language_code=fr", nil)
	c.Set("staff_business_id", uint(1))
	c.Set("token_type", "staff")

	handler.GetTranslatedContent(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCurrencyHandler_UpdateBusinessLanguagesRejectsUnsupportedExactCode(t *testing.T) {
	handler := setupCurrencyHandlerTestDB(t)
	business := createCurrencyTestBusiness(t, "0x1111111111111111111111111111111111111111")

	payload := map[string]any{
		"language_codes": []string{"en", "es-ar"},
		"default_code":   "es-ar",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(business.ID))}}
	c.Request = httptest.NewRequest("PUT", "/businesses/"+strconv.Itoa(int(business.ID))+"/languages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateBusinessLanguages(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), server.ErrCodeInvalidInput)
}

func TestCurrencyHandler_UpdateBusinessLanguagesPreservesCanonicalCode(t *testing.T) {
	handler := setupCurrencyHandlerTestDB(t)
	business := createCurrencyTestBusiness(t, "0x2222222222222222222222222222222222222222")

	payload := map[string]any{
		"language_codes": []string{"en", "es-AR"},
		"default_code":   "es-AR",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(business.ID))}}
	c.Request = httptest.NewRequest("PUT", "/businesses/"+strconv.Itoa(int(business.ID))+"/languages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateBusinessLanguages(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var canonicalCount int64
	require.NoError(t, database.GetDB().
		Model(&database.BusinessLanguage{}).
		Where("business_id = ? AND language_code = ?", business.ID, "es-AR").
		Count(&canonicalCount).Error)
	assert.Equal(t, int64(1), canonicalCount)

	var lowerCount int64
	require.NoError(t, database.GetDB().
		Model(&database.BusinessLanguage{}).
		Where("business_id = ? AND language_code = ?", business.ID, "es-ar").
		Count(&lowerCount).Error)
	assert.Zero(t, lowerCount)
}

func TestCurrencyHandler_GetTranslatedContentRejectsUnauthorizedBusinessAccess(t *testing.T) {
	handler := setupCurrencyHandlerTestDB(t)
	allowedBusiness := createCurrencyTestBusiness(t, "0x1111111111111111111111111111111111111111")
	blockedBusiness := createCurrencyTestBusiness(t, "0x2222222222222222222222222222222222222222")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		"GET",
		"/translations?business_id="+strconv.Itoa(int(blockedBusiness.ID))+"&entity_type=category&entity_id=1&field_name=name&language_code=fr",
		nil,
	)
	c.Set("staff_business_id", allowedBusiness.ID)
	c.Set("token_type", "staff")

	handler.GetTranslatedContent(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCurrencyHandler_UpdateTranslationRejectsUnauthorizedBusinessAccess(t *testing.T) {
	handler := setupCurrencyHandlerTestDB(t)
	allowedBusiness := createCurrencyTestBusiness(t, "0x3333333333333333333333333333333333333333")
	blockedBusiness := createCurrencyTestBusiness(t, "0x4444444444444444444444444444444444444444")

	payload := map[string]any{
		"business_id":     blockedBusiness.ID,
		"entity_type":     "menu_item",
		"entity_id":       10,
		"field_name":      "name",
		"language_code":   "fr",
		"translated_text": "Croissant",
		"original_text":   "Croissant",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("PUT", "/translations", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("staff_business_id", allowedBusiness.ID)
	c.Set("token_type", "staff")
	c.Set("staff_role", string(database.StaffRoleManager))

	handler.UpdateTranslation(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCurrencyHandler_UpdateTranslationRejectsStaffWithoutTranslatePermission(t *testing.T) {
	handler := setupCurrencyHandlerTestDB(t)
	business := createCurrencyTestBusiness(t, "0x9999999999999999999999999999999999999999")

	payload := map[string]any{
		"business_id":     business.ID,
		"entity_type":     "menu_item",
		"entity_id":       10,
		"field_name":      "name",
		"language_code":   "fr",
		"translated_text": "Croissant",
		"original_text":   "Croissant",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("PUT", "/translations", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("staff_business_id", business.ID)
	c.Set("token_type", "staff")
	c.Set("staff_role", string(database.StaffRoleServer))

	handler.UpdateTranslation(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCurrencyHandler_GetMenuTranslationsRejectsUnauthorizedBusinessAccess(t *testing.T) {
	handler := setupCurrencyHandlerTestDB(t)
	allowedBusiness := createCurrencyTestBusiness(t, "0x5555555555555555555555555555555555555555")
	blockedBusiness := createCurrencyTestBusiness(t, "0x6666666666666666666666666666666666666666")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		"GET",
		"/menu-translations?business_id="+strconv.Itoa(int(blockedBusiness.ID))+"&language_code=fr",
		nil,
	)
	c.Set("staff_business_id", allowedBusiness.ID)
	c.Set("token_type", "staff")

	handler.GetMenuTranslations(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCurrencyHandler_GetTranslatedContentReturnsBusinessScopedTranslation(t *testing.T) {
	handler := setupCurrencyHandlerTestDB(t)
	allowedBusiness := createCurrencyTestBusiness(t, "0x7777777777777777777777777777777777777777")
	otherBusiness := createCurrencyTestBusiness(t, "0x8888888888888888888888888888888888888888")

	require.NoError(t, database.GetDB().Create(&database.Translation{
		BusinessID:        allowedBusiness.ID,
		EntityType:        "category",
		EntityID:          1,
		FieldName:         "name",
		LanguageCode:      "fr",
		OriginalText:      "Starters",
		TranslatedText:    "Entrees",
		IsAutoTranslated:  false,
		TranslationSource: "manual",
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.Translation{
		BusinessID:        otherBusiness.ID,
		EntityType:        "category",
		EntityID:          1,
		FieldName:         "name",
		LanguageCode:      "fr",
		OriginalText:      "Starters",
		TranslatedText:    "Wrong Business",
		IsAutoTranslated:  false,
		TranslationSource: "manual",
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		"GET",
		"/translations?business_id="+strconv.Itoa(int(allowedBusiness.ID))+"&entity_type=category&entity_id=1&field_name=name&language_code=fr",
		nil,
	)
	c.Set("staff_business_id", allowedBusiness.ID)
	c.Set("token_type", "staff")
	c.Set("staff_role", string(database.StaffRoleServer))

	handler.GetTranslatedContent(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Entrees")
	assert.NotContains(t, w.Body.String(), "Wrong Business")
}
