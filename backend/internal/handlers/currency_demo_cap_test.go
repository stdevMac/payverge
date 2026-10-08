package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/demomode"
)

// A manual translation renders on the storefront in place of the source
// text, so in DEMO_MODE it is capped like the source fields; otherwise it
// would bypass the 500-character storefront cap.
func TestCurrencyHandler_UpdateTranslationCapsTextInDemoMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupCurrencyHandlerTestDB(t)
	const owner = "0x7777777777777777777777777777777777777777"
	business := createCurrencyTestBusiness(t, owner)
	config.SetDemoModeForTesting(t, true)

	put := func(translated, original string) *httptest.ResponseRecorder {
		body, err := json.Marshal(map[string]any{
			"business_id":     business.ID,
			"entity_type":     "business",
			"entity_id":       business.ID,
			"field_name":      "description",
			"language_code":   "fr",
			"translated_text": translated,
			"original_text":   original,
		})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPut, "/translations", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("address", owner)
		handler.UpdateTranslation(c)
		return w
	}

	long := strings.Repeat("é", demomode.MaxStorefrontTextRunes+1)
	for name, w := range map[string]*httptest.ResponseRecorder{
		"long translation": put(long, "Grill"),
		"long original":    put("Grill", long),
	} {
		require.Equal(t, http.StatusForbidden, w.Code, "%s: %s", name, w.Body.String())
		assert.Contains(t, w.Body.String(), demomode.ErrorCode, name)
	}

	w := put("Grill de quartier", "Neighbourhood grill")
	assert.NotEqual(t, http.StatusForbidden, w.Code, w.Body.String())
}
