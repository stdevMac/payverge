package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func setupPublicStorefrontAccessDB(t testing.TB, gormLogger logger.Interface) *gorm.DB {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.BusinessGalleryImage{},
		&database.BusinessOperatingHours{},
		&database.BusinessOperatingException{},
		&database.BusinessSpecialFeature{},
		&database.BusinessLanguage{},
		&database.SupportedLanguage{},
	))
	services.ResetPricingCache()
	t.Cleanup(services.ResetPricingCache)
	return gormDB
}

func seedPublicStorefrontFixture(t testing.TB, customURL string) *database.Business {
	t.Helper()

	business := createPublicBusinessRouteTestBusiness(t, customURL, true, true)
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	for i := 0; i < 30; i++ {
		require.NoError(t, database.GetDB().Create(&database.BusinessGalleryImage{
			BusinessID:   business.ID,
			ImageURL:     fmt.Sprintf("https://cdn.example.com/g-%d.jpg", i),
			Caption:      fmt.Sprintf("Gallery %d", i),
			DisplayOrder: i,
			IsActive:     true,
		}).Error)
	}
	require.NoError(t, database.GetDB().Create(&database.BusinessGalleryImage{
		BusinessID:   business.ID,
		ImageURL:     "https://cdn.example.com/inactive.jpg",
		Caption:      "Hidden",
		DisplayOrder: 99,
		IsActive:     false,
	}).Error)

	for day := 0; day < 7; day++ {
		require.NoError(t, database.GetDB().Create(&database.BusinessOperatingHours{
			BusinessID: business.ID,
			DayOfWeek:  day,
			OpenTime:   "11:00",
			CloseTime:  "23:00",
			IsClosed:   day == 1,
		}).Error)
	}

	require.NoError(t, database.GetDB().Create(&database.BusinessOperatingException{
		BusinessID:    business.ID,
		ExceptionDate: today.AddDate(-2, 0, 0),
		IsClosed:      true,
		Label:         "Past holiday",
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessOperatingException{
		BusinessID:    business.ID,
		ExceptionDate: today,
		IsClosed:      true,
		Label:         "Today closed",
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessOperatingException{
		BusinessID:    business.ID,
		ExceptionDate: today.AddDate(0, 0, 21),
		IsClosed:      true,
		Label:         "Upcoming private event",
	}).Error)

	for i := 0; i < 30; i++ {
		require.NoError(t, database.GetDB().Create(&database.BusinessSpecialFeature{
			BusinessID:   business.ID,
			Title:        fmt.Sprintf("Feature %d", i),
			Description:  "Patio seating",
			Icon:         "sun",
			DisplayOrder: i,
			IsActive:     true,
		}).Error)
	}

	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID:   business.ID,
		LanguageCode: "en",
		IsDefault:    true,
		DisplayOrder: 0,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID:   business.ID,
		LanguageCode: "es",
		IsDefault:    false,
		DisplayOrder: 1,
	}).Error)

	for i, lang := range []struct {
		code, name, native string
	}{
		{"en", "English", "English"},
		{"es", "Spanish", "Español"},
		{"fr", "French", "Français"},
	} {
		require.NoError(t, database.GetDB().Create(&database.SupportedLanguage{
			Code:       lang.code,
			Name:       lang.name,
			NativeName: lang.native,
			IsActive:   true,
			CreatedAt:  time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Hour),
		}).Error)
	}

	return business
}

func performGetPublicStorefront(t testing.TB, customURL string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: customURL}}
	c.Request = httptest.NewRequest(http.MethodGet, "/business/"+customURL, nil)
	GetBusinessByCustomURL(c)
	return w
}

func storefrontSelectsFrom(recorder *publicGuestSQLRecorder, table string) []string {
	var out []string
	for _, stmt := range recorder.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(stmt), " "))
		if !strings.HasPrefix(normalized, "select") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table+" ") ||
			strings.HasSuffix(normalized, "from "+table) {
			out = append(out, normalized)
		}
	}
	return out
}

func assertProjectedBoundedSelects(t *testing.T, stmts []string, table string) {
	t.Helper()
	require.NotEmpty(t, stmts, "expected at least one SELECT from %s", table)
	for _, stmt := range stmts {
		assert.NotContains(t, stmt, "select *", "%s must project columns instead of SELECT *", table)
		assert.Contains(t, stmt, "limit ", "%s must be bounded with LIMIT", table)
	}
}

// TestGetBusinessByCustomURLAccessShape is the #566 query-shape gate: every
// public storefront list read must project columns and carry an explicit bound.
// Operating exceptions must also date-filter so venue history cannot grow the
// anonymous landing payload forever.
func TestGetBusinessByCustomURLAccessShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicStorefrontAccessDB(t, recorder)
	seedPublicStorefrontFixture(t, "storefront-shape")

	recorder.statements = nil
	w := performGetPublicStorefront(t, "storefront-shape")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	for _, table := range []string{
		"business_gallery_images",
		"business_operating_hours",
		"business_operating_exceptions",
		"business_special_features",
		"business_languages",
		"supported_languages",
	} {
		assertProjectedBoundedSelects(t, storefrontSelectsFrom(recorder, table), table)
	}

	exceptionSQL := strings.Join(storefrontSelectsFrom(recorder, "business_operating_exceptions"), "\n")
	require.Contains(t, exceptionSQL, "exception_date", "exceptions must use the date index predicate")
	require.True(t,
		strings.Contains(exceptionSQL, "exception_date >=") || strings.Contains(exceptionSQL, "exception_date >"),
		"exceptions must filter exception_date >= current window, got %s", exceptionSQL)
}

func TestGetBusinessByCustomURLOmitsPastOperatingExceptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicStorefrontAccessDB(t, nil)
	seedPublicStorefrontFixture(t, "storefront-exceptions")

	w := performGetPublicStorefront(t, "storefront-exceptions")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	raw, ok := body["operating_exceptions"].([]any)
	require.True(t, ok, "response missing operating_exceptions")

	labels := make([]string, 0, len(raw))
	for _, row := range raw {
		item, ok := row.(map[string]any)
		require.True(t, ok)
		if label, _ := item["label"].(string); label != "" {
			labels = append(labels, label)
		}
	}
	assert.NotContains(t, labels, "Past holiday")
	assert.Contains(t, labels, "Today closed")
	assert.Contains(t, labels, "Upcoming private event")
}

func TestGetBusinessByCustomURLOmitsBusinessLanguageEmbed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicStorefrontAccessDB(t, nil)
	seedPublicStorefrontFixture(t, "storefront-langs")

	w := performGetPublicStorefront(t, "storefront-langs")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	langs, ok := body["business_languages"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, langs)

	for i, row := range langs {
		item, ok := row.(map[string]any)
		require.True(t, ok, "business_languages[%d] must be an object", i)
		_, hasBusiness := item["business"]
		assert.False(t, hasBusiness, "business_languages[%d] must not embed Business (#523/#566)", i)
		_, hasCode := item["language_code"]
		assert.True(t, hasCode, "business_languages[%d] must keep language_code", i)
	}

	supported, ok := body["supported_languages"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, supported)
	for i, row := range supported {
		item, ok := row.(map[string]any)
		require.True(t, ok, "supported_languages[%d] must be an object", i)
		assert.NotEmpty(t, item["code"])
		assert.NotEmpty(t, item["name"])
		assert.NotEmpty(t, item["native_name"])
		_, hasID := item["id"]
		_, hasCreated := item["created_at"]
		_, hasUpdated := item["updated_at"]
		assert.False(t, hasID, "supported_languages must drop unused id padding")
		assert.False(t, hasCreated, "supported_languages must drop unused created_at padding")
		assert.False(t, hasUpdated, "supported_languages must drop unused updated_at padding")
	}
}

func TestGetBusinessByCustomURLCachesSupportedLanguages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicStorefrontAccessDB(t, recorder)
	seedPublicStorefrontFixture(t, "storefront-lang-cache")

	recorder.statements = nil
	for i := 0; i < 2; i++ {
		w := performGetPublicStorefront(t, "storefront-lang-cache")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	require.LessOrEqual(t, len(storefrontSelectsFrom(recorder, "supported_languages")), 1,
		"supported_languages is global static data and must not be re-queried per storefront hit")
}

func TestGetBusinessByCustomURLBoundsHospitalityLists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicStorefrontAccessDB(t, nil)
	seedPublicStorefrontFixture(t, "storefront-bounds")

	w := performGetPublicStorefront(t, "storefront-bounds")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	gallery, _ := body["gallery_images"].([]any)
	features, _ := body["special_features"].([]any)
	hours, _ := body["operating_hours"].([]any)
	assert.LessOrEqual(t, len(gallery), database.PublicStorefrontGalleryLimit)
	assert.LessOrEqual(t, len(features), database.PublicStorefrontFeatureLimit)
	assert.Equal(t, 7, len(hours), "weekly hours grid must still be complete")
	assert.Greater(t, len(gallery), 0)
	assert.Greater(t, len(features), 0)
}

func TestGetBusinessByCustomURLKeepsGuestVisibleHospitalityFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicStorefrontAccessDB(t, nil)
	seedPublicStorefrontFixture(t, "storefront-contract")

	w := performGetPublicStorefront(t, "storefront-contract")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "Public Route Business", body["name"])
	assert.Equal(t, "storefront-contract", body["custom_url"])

	gallery := body["gallery_images"].([]any)[0].(map[string]any)
	assert.NotEmpty(t, gallery["image_url"])
	assert.NotEmpty(t, gallery["caption"])

	hours := body["operating_hours"].([]any)[0].(map[string]any)
	assert.Contains(t, hours, "day_of_week")
	assert.Contains(t, hours, "open_time")
	assert.Contains(t, hours, "is_closed")

	feature := body["special_features"].([]any)[0].(map[string]any)
	assert.Equal(t, true, feature["is_active"])
	assert.NotEmpty(t, feature["title"])
}

func BenchmarkGetBusinessByCustomURLStorefrontSQLite(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	setupPublicStorefrontAccessDB(b, nil)
	seedPublicStorefrontFixture(b, "storefront-bench")
	services.ResetPricingCache()

	r := gin.New()
	r.GET("/business/:customUrl", GetBusinessByCustomURL)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/business/storefront-bench", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("got %d, body=%s", w.Code, w.Body.String())
		}
	}
}
