package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func quoteNotFoundCatalog() []database.MenuCategory {
	return []database.MenuCategory{{
		ID:   "mains",
		Name: "Mains",
		Items: []database.MenuItem{{
			ID: "demo-steak", Name: "Steak Plate", Price: 42, IsAvailable: true,
			Options: []database.MenuItemOption{{ID: "opt-salt", Name: "Extra Salt", PriceChange: 1}},
		}},
	}}
}

func decodeQuoteErrorBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return body
}

func TestSerializeOrderQuoteUsesDecimalMajorUnitsAndOrderability(t *testing.T) {
	projection := services.OrderQuoteProjection{
		Quote: services.Quote{
			SubtotalCents: 999, DiscountCents: 100, TotalCents: 899,
			NetSubtotalCents: 899, TaxCents: 90, ServiceFeeCents: 45,
			TipCents: 25, FinalTotalCents: 1059,
			Lines: []services.QuotedLine{{Key: "burger", LineType: services.OrderItemTypeMenuItem, UnitPriceCents: 333, Quantity: 3, SubtotalCents: 999}},
		},
		Orderability: map[string]services.Orderability{
			"burger": {Orderable: true, State: services.OrderabilityAvailable},
		},
	}

	got := serializeOrderQuote(projection)

	assert.Equal(t, 9.99, got.Subtotal)
	assert.Equal(t, 1.0, got.Discount)
	assert.Equal(t, 8.99, got.NetSubtotal)
	assert.Equal(t, 10.59, got.Total)
	raw, err := json.Marshal(got)
	if assert.NoError(t, err) {
		var contract map[string]interface{}
		if assert.NoError(t, json.Unmarshal(raw, &contract)) {
			assert.Equal(t, 8.99, contract["net_subtotal"])
			assert.Equal(t, 0.9, contract["tax"])
			assert.Equal(t, 0.45, contract["service_fee"])
			assert.Equal(t, 0.25, contract["tip"])
			assert.Equal(t, 10.59, contract["total"])
			assert.NotContains(t, contract, "final_total")
		}
	}
	if assert.Len(t, got.Lines, 1) {
		assert.Equal(t, 3.33, got.Lines[0].UnitPrice)
		assert.Equal(t, 9.99, got.Lines[0].Subtotal)
		assert.Equal(t, services.OrderItemTypeMenuItem, got.Lines[0].LineType)
		assert.Equal(t, services.OrderabilityAvailable, got.Lines[0].Orderability.State)
	}
}

func TestRespondOrderQuoteError_TypedNotFoundProducersStay400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	missingBundleID := uint(999)
	cases := []struct {
		name     string
		input    []services.PromotionInputLine
		wantCode string
	}{
		{
			name: "unknown option",
			input: []services.PromotionInputLine{{
				Name: "Steak Plate", MenuItemID: "demo-steak", Quantity: 1, ItemType: services.OrderItemTypeMenuItem,
				Options: []database.MenuItemOption{{ID: "opt-x", Name: "Extra Truffle", PriceChange: -40}},
			}},
			wantCode: services.OrderErrCodeOptionNotFound,
		},
		{
			name: "unknown menu item",
			input: []services.PromotionInputLine{{
				Name: "Definitely Not On The Menu", MenuItemID: "demo-missing", Quantity: 1, ItemType: services.OrderItemTypeMenuItem,
			}},
			wantCode: services.OrderErrCodeItemNotFound,
		},
		{
			name: "unknown bundle",
			input: []services.PromotionInputLine{{
				Name: "Ghost Combo", Quantity: 1, ItemType: services.OrderItemTypeBundle, BundleID: &missingBundleID,
			}},
			wantCode: services.OrderErrCodeBundleNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := services.ApplyPromotionsToOrder(quoteNotFoundCatalog(), nil, nil, tc.input)
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), "not found",
				"producer message wording is what used to flip the quote mapper to 404")

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			respondOrderQuoteError(c, err)

			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			body := decodeQuoteErrorBody(t, w)
			assert.Equal(t, tc.wantCode, body["code"])
			assert.NotEqual(t, ErrCodeBusinessNotFound, body["code"])
			assert.NotEqual(t, "Business or table not found", body["error"])
		})
	}
}

func TestRespondOrderQuoteError_GenuineLookupNotFoundIs404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name string
		err  error
	}{
		{name: "gorm record not found", err: gorm.ErrRecordNotFound},
		{name: "wrapped table not found", err: fmt.Errorf("table not found: %w", gorm.ErrRecordNotFound)},
		{name: "wrapped business not found", err: fmt.Errorf("business not found: %w", gorm.ErrRecordNotFound)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			respondOrderQuoteError(c, tc.err)

			require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
			body := decodeQuoteErrorBody(t, w)
			assert.Equal(t, ErrCodeBusinessNotFound, body["code"])
			assert.Equal(t, "Business or table not found", body["error"])
		})
	}
}

func TestQuoteGuestOrder_UnknownCatalogLinesStay400(t *testing.T) {
	table := seedGuestSteakMenuAndTable(t, "quote-not-found-catalog")
	missingBundleID := uint(999)
	cases := []struct {
		name     string
		payload  map[string]any
		wantCode string
	}{
		{
			name: "unknown option",
			payload: map[string]any{
				"items": []map[string]any{{
					"menu_item_id":   "demo-steak",
					"menu_item_name": "Steak Plate",
					"quantity":       1,
					"options": []map[string]any{{
						"id": "opt-x", "name": "Extra Truffle", "price_change": -40, "is_required": false,
					}},
				}},
			},
			wantCode: services.OrderErrCodeOptionNotFound,
		},
		{
			name: "unknown menu item",
			payload: map[string]any{
				"items": []map[string]any{{
					"menu_item_id":   "demo-missing",
					"menu_item_name": "Definitely Not On The Menu",
					"quantity":       1,
				}},
			},
			wantCode: services.OrderErrCodeItemNotFound,
		},
		{
			name: "unknown bundle",
			payload: map[string]any{
				"items": []map[string]any{{
					"menu_item_name": "Ghost Combo",
					"quantity":       1,
					"item_type":      "bundle",
					"bundle_id":      missingBundleID,
				}},
			},
			wantCode: services.OrderErrCodeBundleNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.payload)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
			c.Request = httptest.NewRequest(
				http.MethodPost,
				fmt.Sprintf("/guest/table/%s/order/quote", table.TableCode),
				bytes.NewReader(body),
			)
			c.Request.Header.Set("Content-Type", "application/json")

			QuoteGuestOrder(c)

			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			got := decodeQuoteErrorBody(t, w)
			assert.Equal(t, tc.wantCode, got["code"])
			assert.NotEqual(t, ErrCodeBusinessNotFound, got["code"])
		})
	}
}

func TestQuoteGuestOrder_StaleMenuItemIDRemapsOrderability(t *testing.T) {
	table := seedGuestSteakMenuAndTable(t, "quote-stale-id-remap")
	quote := func(itemID string) orderQuoteResponse {
		t.Helper()
		body, err := json.Marshal(map[string]any{
			"items": []map[string]any{{
				"menu_item_id":   itemID,
				"menu_item_name": "Steak Plate",
				"quantity":       1,
			}},
		})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
		c.Request = httptest.NewRequest(
			http.MethodPost,
			fmt.Sprintf("/guest/table/%s/order/quote", table.TableCode),
			bytes.NewReader(body),
		)
		c.Request.Header.Set("Content-Type", "application/json")
		QuoteGuestOrder(c)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var got orderQuoteResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got), w.Body.String())
		return got
	}

	control := quote("demo-steak")
	stale := quote("not-a-real-id-999")
	require.Len(t, stale.Lines, 1)
	require.NotNil(t, stale.Lines[0].Orderability, "stale id must still attach orderability")
	assert.Equal(t, "demo-steak", stale.Lines[0].Key)
	assert.Equal(t, control.Lines[0].Orderability, stale.Lines[0].Orderability)
	assert.Equal(t, control.Subtotal, stale.Subtotal)
}

func TestQuoteGuestOrder_QuantityErrorNamesMenuItemID(t *testing.T) {
	table := seedGuestSteakMenuAndTable(t, "quote-qty-id")
	body, err := json.Marshal(map[string]any{
		"items": []map[string]any{{
			"menu_item_id": "demo-steak",
			"quantity":     99,
		}},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/guest/table/%s/order/quote", table.TableCode),
		bytes.NewReader(body),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	QuoteGuestOrder(c)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	got := decodeQuoteErrorBody(t, w)
	assert.Equal(t, services.OrderErrCodeQuantityExceeded, got["code"])
	assert.Contains(t, fmt.Sprint(got["error"]), "demo-steak")
	assert.NotContains(t, fmt.Sprint(got["error"]), "''")
}

func TestQuoteGuestOrder_UnknownTableStays404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: "NOSUCHTABLE"}}
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/guest/table/NOSUCHTABLE/order/quote",
		bytes.NewReader([]byte(`{"items":[{"menu_item_id":"demo-steak","menu_item_name":"Steak Plate","quantity":1}]}`)),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	QuoteGuestOrder(c)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Table not found")
	assert.NotContains(t, w.Body.String(), ErrCodeBusinessNotFound)
}
