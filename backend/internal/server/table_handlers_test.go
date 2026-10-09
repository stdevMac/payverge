package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetBusinessTables_AcceptsBusinessSlug(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTableHandlerTestDB(t)

	business := createTableHandlerBusiness(t, "slug")
	createTableHandlerRecord(t, business.ID, "Window")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: business.BusinessId}}
	c.Request = httptest.NewRequest(http.MethodGet, "/inside/businesses/"+business.BusinessId+"/tables", nil)

	GetBusinessTables(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"name":"Window"`)
}

// FIND-060: table CRUD/list 500 paths must not return raw err.Error() text.
func TestTableHandlers_NoRawErrErrorOnInternalFailures(t *testing.T) {
	src, err := os.ReadFile("table_handlers.go")
	require.NoError(t, err)
	text := string(src)
	require.NotContains(t, text, `gin.H{"error": err.Error()}`,
		"table handlers must not return raw err.Error() to clients (FIND-060)")
}
