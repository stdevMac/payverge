package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newEnhanceContext(t *testing.T, slug, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: slug}}
	req := httptest.NewRequest(http.MethodPost, "/businesses/"+slug+"/ai/enhance-image", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Set("address", "0xowner")
	return c, w
}

func TestEnhanceMenuItemImage_RejectsMissingItemName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIMenuHandlerTestDB(t)
	business := createAIMenuBusiness(t, "enh-missing-name")

	c, w := newEnhanceContext(t, business.BusinessId, `{"image_url":"https://x/a.png"}`)
	EnhanceMenuItemImage(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestEnhanceMenuItemImage_RejectsForeignImageURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIMenuHandlerTestDB(t)
	business := createAIMenuBusiness(t, "enh-foreign-url")

	c, w := newEnhanceContext(t, business.BusinessId,
		`{"image_url":"https://evil.example.com/a.png","item_name":"Burger"}`)
	EnhanceMenuItemImage(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid image url")
}

func TestEnhanceMenuItemImage_ForeignURLErrorIsOpaque(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIMenuHandlerTestDB(t)
	business := createAIMenuBusiness(t, "enh-opaque")

	c, w := newEnhanceContext(t, business.BusinessId,
		`{"image_url":"https://evil.example.com/a.png","item_name":"Burger"}`)
	EnhanceMenuItemImage(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "invalid image url")
	// The opaque error must not echo the attacker host or any allowlist detail.
	assert.NotContains(t, body, "evil.example.com")
	assert.NotContains(t, body, "allowed")
}
