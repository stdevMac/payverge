package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestGetBusinessByCustomURLProductionLikePayloadSize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicStorefrontAccessDB(t, nil)

	business := createPublicBusinessRouteTestBusiness(t, "payverge-core-demo-kitchen", true, true)
	require.NoError(t, database.GetDB().Model(business).Updates(map[string]interface{}{
		"is_demo":     true,
		"kind":        database.BusinessKindDemo,
		"description": "A Payverge showcase restaurant with realistic operational demo data.",
	}).Error)

	for i := 0; i < 3; i++ {
		require.NoError(t, database.GetDB().Create(&database.BusinessGalleryImage{
			BusinessID:   business.ID,
			ImageURL:     fmt.Sprintf("https://cdn.example.com/demo-%d.jpg", i),
			Caption:      fmt.Sprintf("Demo gallery %d", i+1),
			DisplayOrder: i,
			IsActive:     true,
		}).Error)
	}
	for day := 0; day < 7; day++ {
		require.NoError(t, database.GetDB().Create(&database.BusinessOperatingHours{
			BusinessID: business.ID,
			DayOfWeek:  day,
			OpenTime:   "11:00",
			CloseTime:  "23:00",
			IsClosed:   false,
		}).Error)
	}
	for i, title := range []string{"Patio", "Live music", "Pet friendly"} {
		require.NoError(t, database.GetDB().Create(&database.BusinessSpecialFeature{
			BusinessID:   business.ID,
			Title:        title,
			Description:  "Guest-facing amenity",
			Icon:         "sun",
			DisplayOrder: i,
			IsActive:     true,
		}).Error)
	}
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID: business.ID, LanguageCode: "en", IsDefault: true, DisplayOrder: 0,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID: business.ID, LanguageCode: "es", IsDefault: false, DisplayOrder: 1,
	}).Error)

	langs := []struct{ code, name, native string }{
		{"ar", "Arabic", "العربية"}, {"da", "Danish", "Dansk"}, {"de", "German", "Deutsch"},
		{"en", "English", "English"}, {"es", "Spanish", "Español"},
		{"es-AR", "Argentine Spanish", "Español (Argentina)"},
		{"fr", "French", "Français"}, {"hi", "Hindi", "हिन्दी"}, {"it", "Italian", "Italiano"},
		{"ja", "Japanese", "日本語"}, {"ko", "Korean", "한국어"}, {"nl", "Dutch", "Nederlands"},
		{"no", "Norwegian", "Norsk"}, {"pl", "Polish", "Polski"}, {"pt", "Portuguese", "Português"},
		{"ru", "Russian", "Русский"}, {"sv", "Swedish", "Svenska"}, {"th", "Thai", "ไทย"},
		{"tr", "Turkish", "Türkçe"}, {"vi", "Vietnamese", "Tiếng Việt"}, {"zh", "Chinese", "中文"},
	}
	for _, lang := range langs {
		require.NoError(t, database.GetDB().Create(&database.SupportedLanguage{
			Code: lang.code, Name: lang.name, NativeName: lang.native, IsActive: true,
			CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		}).Error)
	}

	w := performGetPublicStorefront(t, business.CustomURL)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	compact, err := json.Marshal(json.RawMessage(w.Body.Bytes()))
	require.NoError(t, err)
	t.Logf("storefront compact JSON bytes=%d wire=%d", len(compact), w.Body.Len())

	// Production probe was 14,211 compact bytes, two-thirds padding.
	assert.Less(t, len(compact), 9000, "demo storefront payload must drop materially below the 14,211-byte probe")

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["is_demo"])
	langsOut, _ := body["business_languages"].([]any)
	require.Len(t, langsOut, 2)
	_, hasBusiness := langsOut[0].(map[string]any)["business"]
	assert.False(t, hasBusiness)
}
