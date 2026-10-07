package handlers

import (
	"context"
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

type translationsSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *translationsSQLRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *translationsSQLRecorder) translationReads() []string {
	var out []string
	for _, s := range r.statements {
		n := strings.ToLower(s)
		if strings.HasPrefix(n, "select") && strings.Contains(n, "from `translations`") {
			out = append(out, n)
		}
	}
	return out
}

// seedGuestTranslationsVenue builds one venue with an active table and
// rowsPerType translations of each entity type the venue can hold. Only
// menu_item and bundle rows are read by the guest bill.
func seedGuestTranslationsVenue(tb testing.TB, rec logger.Interface, rowsPerType int) (*CurrencyHandler, string) {
	tb.Helper()
	dsn := fmt.Sprintf("file:guest-translations-%d?mode=memory&cache=shared", time.Now().UnixNano())
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	if rec != nil {
		cfg.Logger = rec
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(tb, err)
	sqlDB, err := gormDB.DB()
	require.NoError(tb, err)
	sqlDB.SetMaxOpenConns(1)
	tb.Cleanup(func() { _ = sqlDB.Close() })
	database.SetTestDB(gormDB)
	require.NoError(tb, gormDB.AutoMigrate(&database.Business{}, &database.Table{}, &database.Translation{}))

	biz := &database.Business{
		BusinessId: fmt.Sprintf("guest-tr-%d", time.Now().UnixNano()), Name: "Guest Translations",
		OwnerAddress:   "0x9999999999999999999999999999999999999999",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(tb, gormDB.Create(biz).Error)
	code := fmt.Sprintf("T%d", time.Now().UnixNano())
	require.NoError(tb, gormDB.Create(&database.Table{BusinessID: biz.ID, TableCode: code, Name: "T1", IsActive: true}).Error)

	var rows []database.Translation
	for _, et := range []string{"menu_item", "bundle", "category", "offer", "business", "marketing_hero", "menu_item_option", "special_feature"} {
		for i := 0; i < rowsPerType; i++ {
			rows = append(rows, database.Translation{
				BusinessID: biz.ID, EntityType: et, EntityID: uint(i), FieldName: "name",
				LanguageCode: "es", OriginalText: strings.Repeat("o", 200), TranslatedText: fmt.Sprintf("%s-%d", et, i),
			})
		}
	}
	require.NoError(tb, gormDB.CreateInBatches(rows, 200).Error)
	dbw := database.GetDBWrapper()
	return NewCurrencyHandler(dbw, nil, services.NewTranslationService(dbw, "")), code
}

func invokeGuestMenuTranslations(h *CurrencyHandler, code string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: code}}
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/table/"+code+"/menu-translations?language_code=es", nil)
	h.GetGuestMenuTranslations(c)
	return w
}

// TestGetGuestMenuTranslationsReadsOnlyGuestEntityTypes is the I2 guard: the
// public guest endpoint reads only the entity types the guest bill renders
// (menu_item, bundle), through named columns, so the
// idx_translations_guest_lookup_effective prefix serves it.
func TestGetGuestMenuTranslationsReadsOnlyGuestEntityTypes(t *testing.T) {
	rec := &translationsSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	h, code := seedGuestTranslationsVenue(t, rec, 5)
	rec.statements = nil

	w := invokeGuestMenuTranslations(h, code)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Translations map[string]map[string]map[string]string `json:"translations"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Len(t, body.Translations["menu_item"], 5)
	assert.Len(t, body.Translations["bundle"], 5)
	assert.Equal(t, "menu_item-3", body.Translations["menu_item"]["3"]["name"])
	for et := range body.Translations {
		assert.Contains(t, []string{"menu_item", "bundle"}, et, "guest endpoint must not ship %s translations", et)
	}

	reads := rec.translationReads()
	require.Len(t, reads, 1)
	assert.False(t, strings.HasPrefix(reads[0], "select *"), "must not SELECT * translations: %s", reads[0])
	assert.NotContains(t, reads[0], "original_text", "guest read must not load original_text")
	assert.Contains(t, reads[0], "entity_type in", "guest read must filter entity_type")
}

func BenchmarkGetGuestMenuTranslationsSQLite(b *testing.B) {
	h, code := seedGuestTranslationsVenue(b, nil, 200)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if w := invokeGuestMenuTranslations(h, code); w.Code != http.StatusOK {
			b.Fatalf("status %d", w.Code)
		}
	}
}
