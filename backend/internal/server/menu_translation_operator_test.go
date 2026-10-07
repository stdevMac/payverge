package server

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestMenuTranslationSourceLanguageFallsBackToEnglish(t *testing.T) {
	assert.Equal(t, "en", menuTranslationSourceLanguage(""))
	assert.Equal(t, "en", menuTranslationSourceLanguage("   "))
	assert.Equal(t, "en", menuTranslationSourceLanguage("en"))
	assert.Equal(t, "es-AR", menuTranslationSourceLanguage("es-AR"))
	assert.NotEqual(t, "es", menuTranslationSourceLanguage(""))
}

func TestGetMenu_SchedulesBackfillWhenTranslationsMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTranslationHandlerTestDB(t)

	business := seedBackfillMenu(t)
	business.DefaultLanguage = "en"
	require.NoError(t, database.GetDB().Save(business).Error)

	var (
		mu      sync.Mutex
		gotBiz  uint
		gotLang string
		called  bool
	)
	previous := enqueueMenuTranslationBackfill
	enqueueMenuTranslationBackfill = func(businessID uint, languageCode string) {
		mu.Lock()
		defer mu.Unlock()
		called = true
		gotBiz = businessID
		gotLang = languageCode
	}
	t.Cleanup(func() {
		enqueueMenuTranslationBackfill = previous
	})

	_, w := callGetMenu(t, business.ID, "es")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var payload struct {
		ParsedCategories []database.MenuCategory `json:"parsed_categories"`
		Language         string                  `json:"language"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Equal(t, "es", payload.Language)
	require.NotEmpty(t, payload.ParsedCategories)
	assert.Equal(t, "Starters", payload.ParsedCategories[0].Name)

	mu.Lock()
	defer mu.Unlock()
	assert.True(t, called, "operator menu read must schedule a translation backfill when rows are missing")
	assert.Equal(t, business.ID, gotBiz)
	assert.Equal(t, "es", gotLang)
}
