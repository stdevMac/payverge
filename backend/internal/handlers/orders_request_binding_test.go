package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCreateOrderRequest_AllowsZeroPriceItems(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payload := `{"bill_id":1,"items":[{"menu_item_name":"Comp Dessert","quantity":1,"price":0}]}`

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")

	var req CreateOrderRequest
	require.NoError(t, c.ShouldBindJSON(&req))
	require.Len(t, req.Items, 1)
}
