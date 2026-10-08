package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

// Regression cover for #852 / #856: an ARS venue's bill must never serialize a
// USD currency, and a guest bill whose legacy `items` snapshot is empty must
// still ship the live bill_items rows the operator view shows.
//
// Both failures share one root cause: the projected bill reads used by the
// guest routes and the operator bill list never load the business currency, so
// every consumer falls through to the "USD" default in resolveBusinessCurrency
// / publicBillCurrency.

func createARSGuestBusiness(t testing.TB, tableCode string) *database.Business {
	t.Helper()

	userID := uint(142)
	business := &database.Business{
		BusinessId:      fmt.Sprintf("ars-guest-%s", tableCode),
		Name:            "Parrilla Quebracho Azul",
		OwnerAddress:    "0xARSOWNER",
		UserID:          &userID,
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "ARS",
		DisplayCurrency: "ARS",
		DefaultLanguage: "es",
		Timezone:        "America/Argentina/Buenos_Aires",
		IsActive:        true,
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

func createGuestARSBill(t testing.TB, businessID, tableID uint, snapshot string) *database.Bill {
	t.Helper()

	bill := &database.Bill{
		BusinessID:     businessID,
		TableID:        tableID,
		BillNumber:     fmt.Sprintf("B-ars-%d", time.Now().UnixNano()),
		Items:          snapshot,
		Subtotal:       3710000,
		TotalAmount:    3710000,
		PaidAmount:     1855000,
		Status:         database.BillStatusPartial,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	return bill
}

func createGuestBillItemRow(t testing.TB, billID uint, id, name string, price float64, quantity int) {
	t.Helper()

	require.NoError(t, database.GetDB().Exec(
		`INSERT INTO bill_items (id, bill_id, menu_item_id, name, price, quantity, options, item_type, subtotal, created_at)
		 VALUES (?, ?, '', ?, ?, ?, '[]', 'menu_item', ?, ?)`,
		id, billID, name, price, quantity, price*float64(quantity), time.Now(),
	).Error)
}

func guestBillPayload(t *testing.T, w *httptest.ResponseRecorder) (map[string]any, []any) {
	t.Helper()

	var resp map[string]any
	require.NoError(t, decodeJSONResponse(w.Body.Bytes(), &resp))
	billResp, ok := resp["bill"].(map[string]any)
	require.True(t, ok, "expected a bill object, got %v", resp["bill"])
	items, _ := resp["items"].([]any)
	return billResp, items
}

// #856: GET /guest/bill/:bill_token on an ARS venue.
func TestGetBillByNumberPublicServesBusinessCurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicGuestTableHandlerTestDBWithLogger(t, recorder)
	services.ResetPricingCache()

	business := createARSGuestBusiness(t, "ARSTOK01")
	table := createGuestPublicTable(t, business.ID, "ARSTOK01")
	bill := createGuestARSBill(t, business.ID, table.ID, `[{"name":"Asado","quantity":1,"price":18550,"subtotal":18550}]`)

	recorder.statements = nil
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/bill/%s", bill.PublicToken), nil)

	GetBillByNumberPublic(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	billResp, _ := guestBillPayload(t, w)
	assert.Equal(t, "ARS", billResp["currency"], "ARS venue must not serialize a USD guest bill")

	// Access shape: the currency must ride the existing bills+businesses join,
	// not a second business read on this guest-hot route.
	assert.Zero(t, recorder.selectCount("businesses"), "guest bill lookup must not add a business round-trip")
	assert.Zero(t, recorder.selectStarCount("bills"), "guest bill lookup must stay projected")
}

// #856: GET /guest/table/:code/bill — the payload the guest bill screen renders.
func TestGetOpenBillByTableCodeServesBusinessCurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicGuestTableHandlerTestDBWithLogger(t, recorder)
	services.ResetPricingCache()

	business := createARSGuestBusiness(t, "ARSTBL01")
	table := createGuestPublicTable(t, business.ID, "ARSTBL01")
	createGuestARSBill(t, business.ID, table.ID, `[{"name":"Asado","quantity":1,"price":18550,"subtotal":18550}]`)

	recorder.statements = nil
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s/bill", table.TableCode), nil)

	GetOpenBillByTableCode(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	billResp, _ := guestBillPayload(t, w)
	assert.Equal(t, "ARS", billResp["currency"], "ARS venue must not serialize a USD guest bill")
	assert.Zero(t, recorder.selectStarCount("bills"), "guest open-bill lookup must stay projected")
}

// #856: an empty legacy snapshot must fall back to the live bill_items rows on
// the guest path, in the same shape the operator bill detail returns.
func TestGuestBillServesBillItemsWhenSnapshotEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicGuestTableHandlerTestDBWithLogger(t, recorder)
	services.ResetPricingCache()

	business := createARSGuestBusiness(t, "ARSITM01")
	table := createGuestPublicTable(t, business.ID, "ARSITM01")
	bill := createGuestARSBill(t, business.ID, table.ID, "")
	createGuestBillItemRow(t, bill.ID, "11111111-1111-1111-1111-111111111111", "Asado", 18550, 1)
	createGuestBillItemRow(t, bill.ID, "22222222-2222-2222-2222-222222222222", "Provoleta", 18550, 1)

	// Operator reference shape.
	_, operatorItems, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	require.Len(t, operatorItems, 2)
	operatorByName := map[string]database.BillItem{}
	for _, item := range operatorItems {
		operatorByName[item.Name] = item
	}

	for _, tc := range []struct {
		name string
		call func(*gin.Context)
		set  func(*gin.Context)
	}{
		{
			name: "bill token route",
			call: GetBillByNumberPublic,
			set: func(c *gin.Context) {
				c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
				c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/bill/%s", bill.PublicToken), nil)
			},
		},
		{
			name: "table code route",
			call: GetOpenBillByTableCode,
			set: func(c *gin.Context) {
				c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
				c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s/bill", table.TableCode), nil)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder.statements = nil
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			tc.set(c)
			tc.call(c)

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			_, items := guestBillPayload(t, w)
			require.Len(t, items, 2, "empty snapshot must fall back to bill_items rows")

			names := map[string]bool{}
			for _, raw := range items {
				line, ok := raw.(map[string]any)
				require.True(t, ok)
				name := fmt.Sprint(line["name"])
				names[name] = true
				// Same wire shape as the operator bill detail.
				operatorLine, found := operatorByName[name]
				require.True(t, found, "guest line %q is not in the operator payload", name)
				assert.Equal(t, operatorLine.Price, line["price"])
				assert.Equal(t, float64(operatorLine.Quantity), line["quantity"])
				assert.Equal(t, operatorLine.Subtotal, line["subtotal"])
			}
			assert.True(t, names["Asado"] && names["Provoleta"])
			assert.LessOrEqual(t, recorder.selectCount("bill_items"), 1, "item fallback must be one bounded read, not an N+1")
		})
	}
}

// #856 regression guard: bills whose items still live in the legacy JSON
// snapshot (pre-bill_items rows) must keep reading the snapshot, and must not
// gain an extra read beyond the single bill_items probe.
func TestGuestBillKeepsLegacySnapshotWhenNoBillItemRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicGuestTableHandlerTestDBWithLogger(t, recorder)
	services.ResetPricingCache()

	business := createARSGuestBusiness(t, "ARSSNP01")
	table := createGuestPublicTable(t, business.ID, "ARSSNP01")
	bill := createGuestARSBill(t, business.ID, table.ID,
		`[{"name":"Legacy Milanesa","quantity":2,"price":9275,"subtotal":18550}]`)

	recorder.statements = nil
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/bill/%s", bill.PublicToken), nil)

	GetBillByNumberPublic(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	billResp, items := guestBillPayload(t, w)
	assert.Equal(t, "ARS", billResp["currency"])
	require.Len(t, items, 1, "legacy snapshot bills must keep serving the snapshot")
	line, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Legacy Milanesa", line["name"])
	assert.Equal(t, float64(2), line["quantity"])

	assert.LessOrEqual(t, recorder.selectCount("bill_items"), 1, "snapshot path must probe bill_items at most once")
	assert.Zero(t, recorder.selectCount("businesses"), "snapshot path must not add a business round-trip")
}

// #852: operator bill list must label peso money as ARS.
func TestGetBusinessBillsServesBusinessCurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)
	services.ResetPricingCache()

	business := createARSGuestBusiness(t, "ARSLST01")
	table := createGuestPublicTable(t, business.ID, "ARSLST01")
	createGuestARSBill(t, business.ID, table.ID, `[{"name":"Asado","quantity":1,"price":18550,"subtotal":18550}]`)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/businesses/%d/bills", business.ID), nil)
	c.Set("address", business.OwnerAddress)

	GetBusinessBills(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, decodeJSONResponse(w.Body.Bytes(), &resp))
	bills, ok := resp["bills"].([]any)
	require.True(t, ok)
	require.Len(t, bills, 1)
	row, ok := bills[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ARS", row["currency"], "operator bill list must not label peso amounts USD")
}

// #852: POST /guest/table/:code/bill — the first bill payload a guest ever sees
// after scanning the QR of an empty table. The handler builds the Bill in
// memory, and Bill.Currency is gorm:"-", so nothing puts the venue currency on
// it unless the handler stamps it from the business it already loaded.
func TestCreateBillByTableCodeServesBusinessCurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)

	business := createARSGuestBusiness(t, "ARSNEW01")
	enableGuestOrderingForBusiness(t, business.ID)
	table := createGuestPublicTable(t, business.ID, "ARSNEW01")
	services.ResetPricingCache()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/bill", table.TableCode), nil)

	CreateBillByTableCode(c)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	billResp, _ := guestBillPayload(t, w)
	assert.Equal(t, "ARS", billResp["currency"], "the bill a guest opens on an ARS venue must not claim USD")
}

// #852: POST /guest/table/:code/order — the guest order confirmation carries the
// bill the checkout service loaded with a plain tx.First (no business, no
// currency), on both the fresh and the idempotent replay path.
func TestCreateGuestOrderServesBusinessCurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDBWithLogger(t, logger.Default.LogMode(logger.Silent))

	business := createARSGuestBusiness(t, "ARSORD01")
	enableGuestOrderingForBusiness(t, business.ID)
	table := createGuestPublicTable(t, business.ID, "ARSORD01")

	menuPayload, err := json.Marshal([]database.MenuCategory{
		{
			ID:   "parrilla",
			Name: "Parrilla",
			Items: []database.MenuItem{
				{ID: "asado", Name: "Asado", Price: 18550, IsAvailable: true},
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID,
		Categories: string(menuPayload),
		IsActive:   true,
		Version:    1,
	}).Error)
	services.ResetPricingCache()

	body, err := json.Marshal(map[string]any{
		"items": []map[string]any{
			{"menu_item_name": "Asado", "menu_item_id": "asado", "quantity": 1, "price": 18550.0},
		},
	})
	require.NoError(t, err)

	buildRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/order", table.TableCode), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-Id", "ars-guest-order-1")
		return req
	}

	firstW := httptest.NewRecorder()
	firstC, _ := gin.CreateTestContext(firstW)
	firstC.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	firstC.Request = buildRequest()

	CreateGuestOrder(firstC)

	require.Equal(t, http.StatusCreated, firstW.Code, firstW.Body.String())
	firstBill, _ := guestBillPayload(t, firstW)
	assert.Equal(t, "ARS", firstBill["currency"], "a guest order confirmation on an ARS venue must not claim USD")

	// Replay: same X-Request-Id returns the stored order and a bill loaded by
	// the replay reader, which is a separate read with the same blind spot.
	replayW := httptest.NewRecorder()
	replayC, _ := gin.CreateTestContext(replayW)
	replayC.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	replayC.Request = buildRequest()

	CreateGuestOrder(replayC)

	require.Equal(t, http.StatusOK, replayW.Code, replayW.Body.String())
	replayBill, _ := guestBillPayload(t, replayW)
	assert.Equal(t, "ARS", replayBill["currency"], "the replayed guest order must carry the venue currency too")
}
