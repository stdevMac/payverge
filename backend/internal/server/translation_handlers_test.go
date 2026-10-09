package server

import (
	"bytes"
	"encoding/json"
	"io"
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
	"github.com/stdevmac/payverge/backend/internal/services"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func setupTranslationHandlerTestDB(t *testing.T) {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Menu{},
		&database.BusinessLanguage{},
		&database.Translation{},
	))
}

func createTranslationHandlerTestBusiness(t *testing.T) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:     "biz-translate-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Name:           "Translation Handler Business",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		TippingAddr:    "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
	}

	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

func installGoogleTranslateStub(t *testing.T) {
	t.Helper()

	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var payload services.GoogleTranslateRequest
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			return nil, err
		}

		response, err := json.Marshal(map[string]any{
			"data": map[string]any{
				"translations": []map[string]string{
					{"translatedText": "tr:" + payload.Q},
				},
			},
		})
		if err != nil {
			return nil, err
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(response)),
		}, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})
}

func configureBatchTranslationService(t *testing.T, apiKey string) {
	t.Helper()

	SetBatchTranslationService(services.NewTranslationService(database.GetDBWrapper(), apiKey))
	t.Cleanup(func() {
		SetBatchTranslationService(nil)
	})
}

func TestGenerateJobID_UniquePerInvocation(t *testing.T) {
	first := generateJobID(42)
	second := generateJobID(42)

	assert.NotEqual(t, first, second)
}

func TestGetTranslationStatus_ReturnsNotFoundForUnknownJob(t *testing.T) {
	resetTranslationJobs()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "jobId", Value: "missing-job"}}
	c.Request = httptest.NewRequest("GET", "/translation-jobs/missing-job/status", nil)

	GetTranslationStatus(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTranslateEntireMenu_AllowsStaffWithMatchingBusiness(t *testing.T) {
	setupTranslationHandlerTestDB(t)
	resetTranslationJobs()

	business := createTranslationHandlerTestBusiness(t)
	body, err := json.Marshal(TranslateMenuRequest{LanguageCodes: []string{"fr"}})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(business.ID))}}
	c.Request = httptest.NewRequest("POST", "/businesses/"+strconv.Itoa(int(business.ID))+"/translate", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_type", "staff")
	c.Set("staff_business_id", business.ID)

	TranslateEntireMenu(c)

	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestTranslateEntireMenu_AcceptsBusinessSlug(t *testing.T) {
	setupTranslationHandlerTestDB(t)
	resetTranslationJobs()

	business := createTranslationHandlerTestBusiness(t)
	body, err := json.Marshal(TranslateMenuRequest{LanguageCodes: []string{"fr"}})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: business.BusinessId}}
	c.Request = httptest.NewRequest("POST", "/businesses/"+business.BusinessId+"/translate", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_type", "staff")
	c.Set("staff_business_id", business.ID)

	TranslateEntireMenu(c)

	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestPerformBatchTranslation_UsesMenuIndexEntityIDs(t *testing.T) {
	setupTranslationHandlerTestDB(t)
	resetTranslationJobs()
	installGoogleTranslateStub(t)
	configureBatchTranslationService(t, "test-api-key")

	business := createTranslationHandlerTestBusiness(t)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID:   business.ID,
		LanguageCode: "en",
		IsDefault:    true,
		DisplayOrder: 0,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID:   business.ID,
		LanguageCode: "es-AR",
		IsDefault:    false,
		DisplayOrder: 2,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID:   business.ID,
		LanguageCode: "es",
		IsDefault:    false,
		DisplayOrder: 1,
	}).Error)

	menuCategories := []database.MenuCategory{
		{
			ID:          "cat-0",
			Name:        "Desserts",
			Description: "Sweet plates",
			Items: []database.MenuItem{
				{
					ID:          "item-0",
					Name:        "Cake",
					Description: "Chocolate cake",
					Price:       7,
					IsAvailable: true,
				},
			},
		},
	}
	menuPayload, err := json.Marshal(menuCategories)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID,
		Categories: string(menuPayload),
		IsActive:   true,
		Version:    1,
	}).Error)

	jobID := registerTranslationJob(business.ID)
	require.NoError(t, performBatchTranslation(business.ID, []string{"es-AR"}, jobID))

	var categoryTranslation database.Translation
	require.NoError(t, database.GetDB().Where(
		"business_id = ? AND entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?",
		business.ID, "category", 0, "name", "es-AR",
	).First(&categoryTranslation).Error)
	assert.Equal(t, "tr:Desserts", categoryTranslation.TranslatedText)

	var itemTranslation database.Translation
	require.NoError(t, database.GetDB().Where(
		"business_id = ? AND entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?",
		business.ID, "menu_item", 0, "name", "es-AR",
	).First(&itemTranslation).Error)
	assert.Equal(t, "tr:Cake", itemTranslation.TranslatedText)

	var spanishCount int64
	require.NoError(t, database.GetDB().
		Model(&database.Translation{}).
		Where("business_id = ? AND language_code = ?", business.ID, "es").
		Count(&spanishCount).Error)
	assert.Zero(t, spanishCount)
}

func TestPerformBatchTranslation_ReturnsErrorWhenServiceDisabled(t *testing.T) {
	setupTranslationHandlerTestDB(t)
	resetTranslationJobs()
	configureBatchTranslationService(t, "")

	business := createTranslationHandlerTestBusiness(t)
	menuPayload, err := json.Marshal([]database.MenuCategory{})
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID,
		Categories: string(menuPayload),
		IsActive:   true,
		Version:    1,
	}).Error)

	jobID := registerTranslationJob(business.ID)
	err = performBatchTranslation(business.ID, []string{"es-AR"}, jobID)

	require.EqualError(t, err, "translation service is not enabled")
}
