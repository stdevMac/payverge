package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestGetBillByNumberPublic_AcceptTokenRejectBillNumber(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)

	business := createSensitiveGuestTableBusiness(t, "TOKRES1")
	table := createGuestPublicTable(t, business.ID, "TOKRES1")
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     fmt.Sprintf("B-token-res-%d", time.Now().UnixNano()),
		Items:          `[{"name":"Item","quantity":1,"price":10,"subtotal":10}]`,
		Subtotal:       1000,
		TotalAmount:    1000,
		Status:         database.BillStatusOpen,
		SettlementAddr: business.SettlementAddr,
		TippingAddr:    business.TippingAddr,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	require.NotEmpty(t, bill.PublicToken)

	// Token reaches success branch.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/bill/%s", bill.PublicToken), nil)
	GetBillByNumberPublic(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "private, no-store", w.Header().Get("Cache-Control"),
		"guest bill token responses must not be heuristically cacheable (#524)")

	// Guessable bill_number → canonical not-found envelope, no leak.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{{Key: "bill_token", Value: bill.BillNumber}}
	c2.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/bill/%s", bill.BillNumber), nil)
	GetBillByNumberPublic(c2)
	require.Equal(t, http.StatusNotFound, w2.Code, w2.Body.String())
	assert.Contains(t, w2.Body.String(), "Bill not found")

	// Empty token → same not-found envelope.
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Params = gin.Params{{Key: "bill_token", Value: "   "}}
	c3.Request = httptest.NewRequest(http.MethodGet, "/guest/bill/%20%20%20", nil)
	GetBillByNumberPublic(c3)
	require.Equal(t, http.StatusNotFound, w3.Code, w3.Body.String())
}

func TestPublicGuestBillProjectionReturnsOpaqueCapability(t *testing.T) {
	bill := &database.Bill{
		ID:          42,
		BillNumber:  "B-display-only",
		PublicToken: "0123456789abcdef0123456789abcdef",
		Status:      database.BillStatusOpen,
	}

	projected := buildPublicGuestBillResponse(bill)
	require.Equal(t, bill.PublicToken, projected["public_token"],
		"table-code bill creation/load must hand the guest the opaque capability used by public bill routes")
}

// TestGetTableByCode_AdvertisesResolvableBillCapability pins the QR landing
// payload against issue 908. The table-code response is the first — and for a
// guest who only scans and asks for the factura, the only — place a client
// learns there is an open bill. It advertised nothing but the human-readable
// bill_number, so "pedí la factura" sent B<biz>-<uuid12> to
// /guest/bill/:bill_token/fiscal-receipt, an identifier the guest capability
// predicate (bills.public_token = ?) can never resolve: a hard 404.
func TestGetOpenBillByTableCode_AdvertisesResolvableBillCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)

	business := createSensitiveGuestTableBusiness(t, "QRCAP01")
	table := createGuestPublicTable(t, business.ID, "QRCAP01")
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     fmt.Sprintf("B%d-0fbda0663b6%d", business.ID, time.Now().UnixNano()),
		Items:          `[{"name":"Item","quantity":1,"price":10,"subtotal":10}]`,
		Subtotal:       1000,
		TotalAmount:    1000,
		Status:         database.BillStatusOpen,
		SettlementAddr: business.SettlementAddr,
		TippingAddr:    business.TippingAddr,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	require.NotEmpty(t, bill.PublicToken)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/table/"+table.TableCode+"/bill", nil)
	GetOpenBillByTableCode(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body struct {
		Bill *struct {
			BillNumber  string `json:"bill_number"`
			PublicToken string `json:"public_token"`
		} `json:"bill"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.NotNil(t, body.Bill, "QR landing must report the open bill")
	assert.Equal(t, bill.BillNumber, body.Bill.BillNumber)
	require.NotEmpty(t, body.Bill.PublicToken,
		"QR landing payload must advertise the bill capability token, not just the display number (issue 908)")

	// The advertised identifier must actually resolve through the one predicate
	// every /guest/bill/:bill_token route uses.
	resolved, _, err := database.GetPublicBillByToken(body.Bill.PublicToken)
	require.NoError(t, err, "the advertised token must resolve for the factura/split/pay routes")
	require.Equal(t, bill.ID, resolved.ID)

	// The display number must stay unusable as a lookup key.
	_, _, err = database.GetPublicBillByToken(body.Bill.BillNumber)
	require.Error(t, err, "bill_number must never become a guest lookup key")
}

func TestPublicGuestBillProjectionCarriesCapability(t *testing.T) {
	summary := buildPublicGuestBillResponse(&database.Bill{
		ID:          7,
		BillNumber:  "B142-0fbda066-3b6",
		PublicToken: "0123456789abcdef0123456789abcdef",
		Status:      database.BillStatusOpen,
	})
	require.Equal(t, "0123456789abcdef0123456789abcdef", summary["public_token"],
		"bill projection must carry the capability guests need for factura/split/pay (issue 908)")
}
