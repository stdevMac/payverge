package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FIND-060: floor handlers must not return raw err.Error() text to clients.
func TestTableFloorHandlers_NoRawErrErrorOnFailures(t *testing.T) {
	src, err := os.ReadFile("table_floor_handlers.go")
	require.NoError(t, err)
	require.NotContains(t, string(src), `gin.H{"error": err.Error()}`,
		"table floor handlers must not return raw err.Error() to clients (FIND-060)")
}

// #704 four Liberar arms: kitchen, pending-approval, and settle must stay
// distinct 409 codes so the host toast is not one English settle string.
func TestRespondFloorError_FourClearArms(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name string
		err  error
		code string
		body string
	}{
		{
			name: "kitchen_tickets_live",
			err:  database.ErrFloorLiveKitchenTickets,
			code: "kitchen_tickets_live",
			body: "kitchen is still working this table",
		},
		{
			name: "orders_pending_approval",
			err:  database.ErrFloorPendingOrders,
			code: "orders_pending_approval",
			body: "waiting for approval",
		},
		{
			name: "settle_required",
			err:  database.ErrFloorSettleRequired,
			code: "settle_required",
			body: "Settle or void the open check",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			respondFloorError(c, tc.err)
			require.Equal(t, http.StatusConflict, w.Code)

			var payload map[string]string
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
			assert.Equal(t, tc.code, payload["code"])
			assert.Contains(t, payload["error"], tc.body)
			if tc.code == "orders_pending_approval" {
				assert.NotContains(t, payload["error"], "kitchen")
			}
			if tc.code == "kitchen_tickets_live" {
				assert.NotEqual(t, "settle_required", payload["code"])
			}
		})
	}
}
