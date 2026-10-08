package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	qrcode "github.com/skip2/go-qrcode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

const testQRPayload = "00020101021243650016com.mercadolibre0201305015204TESTQR"

func mergeMercadoPagoConfigFields(t *testing.T, businessID uint, fields ...database.MergeBusinessPluginConfigField) {
	t.Helper()
	svc := services.NewPluginService(database.GetDBWrapper())
	require.NoError(t, svc.MergePluginConfigFields(businessID, "mercadopago", fields...))
}

func TestChargeMercadoPagoQR_ProvisionsCreatesOrderAndTracker(t *testing.T) {
	// The payer-facing order description names this instance, not upstream.
	t.Setenv("PRODUCT_NAME", "Acme POS")
	var (
		createBody map[string]interface{}
		paths      []string
	)
	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/stores"):
			_, _ = w.Write([]byte(`{"id":"12354567"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/pos":
			_, _ = w.Write([]byte(`{"id":"23545678"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/orders":
			require.NotEmpty(t, r.Header.Get("X-Idempotency-Key"))
			raw, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(raw, &createBody))
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"id":"ORD01QRCHARGE",
				"status":"created",
				"external_reference":"ignored",
				"type_response":{"qr_data":%q},
				"transactions":{"payments":[{"amount":"1500.00"}]}
			}`, testQRPayload)))
		default:
			t.Fatalf("unexpected MP call: %s %s", r.Method, r.URL.Path)
		}
	}))

	// First use: mp_user_id present, no store/POS yet → provision then charge.
	mergeMercadoPagoConfigFields(t, business.ID,
		database.MergeBusinessPluginConfigField{Key: "mp_user_id", Value: "123456789"},
	)

	bill := createPointTestBill(t, business.ID, 150000, 0) // ARS 1500.00 outstanding

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "bill_id", Value: fmt.Sprintf("%d", bill.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/charge", bytes.NewBufferString(`{"amount_cents":0}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).ChargeMercadoPagoQR(c)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ORD01QRCHARGE", resp["order_id"])
	assert.Equal(t, testQRPayload, resp["qr_data"])
	assert.Equal(t, "pending", resp["status"])

	// Decodable base64 PNG
	pngB64, _ := resp["qr_png_base64"].(string)
	require.NotEmpty(t, pngB64)
	pngBytes, err := base64.StdEncoding.DecodeString(pngB64)
	require.NoError(t, err)
	img, err := png.Decode(bytes.NewReader(pngBytes))
	require.NoError(t, err)
	require.NotNil(t, img)
	assert.Greater(t, img.Bounds().Dx(), 0)

	// expires_at ~ now+10m
	expiresRaw, _ := resp["expires_at"].(string)
	require.NotEmpty(t, expiresRaw)
	expiresAt, err := time.Parse(time.RFC3339, expiresRaw)
	require.NoError(t, err)
	delta := time.Until(expiresAt)
	assert.InDelta(t, 10*time.Minute.Seconds(), delta.Seconds(), 30)

	// Order body contract
	require.NotNil(t, createBody)
	assert.Equal(t, "qr", createBody["type"])
	assert.Equal(t, fmt.Sprintf("bill_%d_business_%d", bill.ID, business.ID), createBody["external_reference"])
	assert.Equal(t, "PT10M", createBody["expiration_time"])
	assert.Equal(t, fmt.Sprintf("Acme POS bill #%d", bill.ID), createBody["description"])
	tx := createBody["transactions"].(map[string]interface{})
	payments := tx["payments"].([]interface{})
	assert.Equal(t, "1500.00", payments[0].(map[string]interface{})["amount"])
	cfgBody := createBody["config"].(map[string]interface{})
	qrCfg := cfgBody["qr"].(map[string]interface{})
	assert.Equal(t, fmt.Sprintf("PAYVERGEPOS%d", business.ID), qrCfg["external_pos_id"])
	assert.Equal(t, "dynamic", qrCfg["mode"])

	// Pending tracker with ParticipantAddr = order id
	var tracker database.AlternativePayment
	require.NoError(t, database.GetDB().
		Where("bill_id = ? AND participant_addr = ?", bill.ID, "ORD01QRCHARGE").
		First(&tracker).Error)
	assert.Equal(t, database.AlternativePaymentMethod("mercadopago"), tracker.PaymentMethod)
	assert.Equal(t, database.AltPaymentStatusPending, tracker.Status)
	assert.Equal(t, int64(150000), tracker.Amount)
	assert.Equal(t, "mercadopago", tracker.ParticipantName)
	assert.NotEmpty(t, tracker.IdempotencyKey)

	// First use provisioned store + POS before create order.
	joined := strings.Join(paths, ",")
	assert.Contains(t, joined, "POST /v1/orders")
	assert.Contains(t, joined, "/stores")
	assert.Contains(t, joined, "POST /pos")
}

// R4: QR encode failure must cancel the MP order and mark the tracker cancelled
// so the bill is not stuck pending (409 forever).
func TestChargeMercadoPagoQR_EncodeFailureCancelsOrderAndTracker(t *testing.T) {
	var cancelHits int
	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/orders":
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"id":"ORDENCODEFAIL",
				"status":"created",
				"type_response":{"qr_data":%q},
				"transactions":{"payments":[{"amount":"10.00"}]}
			}`, testQRPayload)))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cancel"):
			cancelHits++
			_, _ = w.Write([]byte(`{"id":"ORDENCODEFAIL","status":"canceled"}`))
		default:
			t.Fatalf("unexpected MP call: %s %s", r.Method, r.URL.Path)
		}
	}))
	mergeMercadoPagoConfigFields(t, business.ID,
		database.MergeBusinessPluginConfigField{Key: "mp_user_id", Value: "123"},
		database.MergeBusinessPluginConfigField{Key: "mp_store_id", Value: "store-1"},
		database.MergeBusinessPluginConfigField{Key: "mp_external_pos_id", Value: "payverge-pos-1"},
	)

	prev := mpQRCodeEncode
	mpQRCodeEncode = func(content string, level qrcode.RecoveryLevel, size int) ([]byte, error) {
		return nil, fmt.Errorf("forced encode failure")
	}
	t.Cleanup(func() { mpQRCodeEncode = prev })

	bill := createPointTestBill(t, business.ID, 1000, 0)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "bill_id", Value: fmt.Sprintf("%d", bill.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/charge", bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).ChargeMercadoPagoQR(c)

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.Equal(t, 1, cancelHits)

	var tracker database.AlternativePayment
	require.NoError(t, database.GetDB().Where("participant_addr = ?", "ORDENCODEFAIL").First(&tracker).Error)
	assert.Equal(t, database.AltPaymentStatusCancelled, tracker.Status)
}

func TestChargeMercadoPagoQR_UsesExistingPOSWithoutProvisioning(t *testing.T) {
	var paths []string
	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPost && r.URL.Path == "/v1/orders" {
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"id":"ORD02QR",
				"status":"created",
				"type_response":{"qr_data":%q},
				"transactions":{"payments":[{"amount":"10.00"}]}
			}`, testQRPayload)))
			return
		}
		t.Fatalf("unexpected provisioning call when POS already configured: %s %s", r.Method, r.URL.Path)
	}))

	mergeMercadoPagoConfigFields(t, business.ID,
		database.MergeBusinessPluginConfigField{Key: "mp_user_id", Value: "123"},
		database.MergeBusinessPluginConfigField{Key: "mp_store_id", Value: "store-1"},
		database.MergeBusinessPluginConfigField{Key: "mp_external_pos_id", Value: "payverge-pos-cached"},
	)

	bill := createPointTestBill(t, business.ID, 1000, 0)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "bill_id", Value: fmt.Sprintf("%d", bill.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/charge", bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).ChargeMercadoPagoQR(c)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Equal(t, []string{"POST /v1/orders"}, paths)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ORD02QR", resp["order_id"])
}

func TestChargeMercadoPagoQR_PendingChargeReturns409(t *testing.T) {
	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("MP API must not be called when a pending charge exists: %s %s", r.Method, r.URL.Path)
	}))

	bill := createPointTestBill(t, business.ID, 5000, 0)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "ORD_EXISTING_QR",
		ParticipantName: "mercadopago",
		Amount:          5000,
		PaymentMethod:   database.AlternativePaymentMethod("mercadopago"),
		Status:          database.AltPaymentStatusPending,
		IdempotencyKey:  "plugin:mercadopago:ORD_EXISTING_QR",
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "bill_id", Value: fmt.Sprintf("%d", bill.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/charge", strings.NewReader(`{"amount_cents":0}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).ChargeMercadoPagoQR(c)

	require.Equal(t, http.StatusConflict, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ORD_EXISTING_QR", resp["order_id"])
}

func TestChargeMercadoPagoQR_WrongBusinessBill404(t *testing.T) {
	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected MP call")
	}))
	other := createTestBusiness(t)
	bill := createPointTestBill(t, other.ID, 1000, 0)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "bill_id", Value: fmt.Sprintf("%d", bill.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/charge", strings.NewReader(`{"amount_cents":0}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).ChargeMercadoPagoQR(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
