package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// API-RECUR-01: the template amount is capped at $100,000,000 (an unbounded
// float overflowed the int64 cents conversion) and rounds to the nearest cent.
func TestCreateRecurringTemplate_AmountBoundsAndRounding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAccountingHandlerDB(t)
	require.NoError(t, db.AutoMigrate(&database.RecurringEntryTemplate{}))
	business := createAccountingHandlerBusiness(t, "0xRecurOwner")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", business.OwnerAddress)
		c.Set("business_owner_address", business.OwnerAddress)
		c.Next()
	})
	handler := NewAccountingHandler(database.GetDBWrapper())
	router.POST("/inside/businesses/:id/accounting/recurring-templates", server.RoleBasedAccessMiddleware("financial:write"), handler.CreateRecurringTemplate)
	path := fmt.Sprintf("/inside/businesses/%d/accounting/recurring-templates", business.ID)
	body := func(amount float64) map[string]any {
		return map[string]any{
			"entry_type":  "expense",
			"category":    "rent",
			"amount":      amount,
			"description": "Rent",
			"cadence":     "monthly",
			"anchor_day":  1,
			"next_run_on": "2026-11-01",
		}
	}

	w := performAccountingRequest(t, router, http.MethodPost, path, body(1e12))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var n int64
	require.NoError(t, db.Model(&database.RecurringEntryTemplate{}).Count(&n).Error)
	require.Zero(t, n)

	for _, tc := range []struct {
		amount float64
		cents  int64
	}{
		{100000000, 10_000_000_000},
		{19.99, 1999},
		{0.29, 29},
	} {
		w := performAccountingRequest(t, router, http.MethodPost, path, body(tc.amount))
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var resp struct {
			Data struct {
				AmountCents int64 `json:"amount_cents"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, tc.cents, resp.Data.AmountCents, "amount %v", tc.amount)
	}
}
