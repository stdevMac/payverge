package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// P6e: two sequential concurrent charge-lock acquisitions on the same bill must
// serialize through the process mutex (SQLite / non-Postgres path). If the lock
// is not held, both would observe overlapping execution.
func TestWithMercadoPagoBillChargeLock_SerializesConcurrentCallers(t *testing.T) {
	const billID uint = 424242
	// Fresh mutex entry for this bill id.
	mercadoPagoBillChargeLocalMu.Delete(billID)

	var concurrent int32
	var maxConcurrent int32
	var wg sync.WaitGroup
	start := make(chan struct{})

	run := func() {
		defer wg.Done()
		<-start
		err := withMercadoPagoBillChargeLock(billID, func() error {
			cur := atomic.AddInt32(&concurrent, 1)
			for {
				prev := atomic.LoadInt32(&maxConcurrent)
				if cur <= prev || atomic.CompareAndSwapInt32(&maxConcurrent, prev, cur) {
					break
				}
			}
			time.Sleep(40 * time.Millisecond)
			atomic.AddInt32(&concurrent, -1)
			return nil
		})
		require.NoError(t, err)
	}

	wg.Add(2)
	go run()
	go run()
	close(start)
	wg.Wait()

	require.Equal(t, int32(1), maxConcurrent,
		"charge lock must serialize concurrent callers; max concurrent was %d (lock path broken)", maxConcurrent)
}

// Q4a: concurrent ChargeMercadoPagoPoint through the real handler — first
// CreateOrder is slow so the second waits on the call-site lock, then sees the
// pending tracker and returns 409. Pins withMercadoPagoBillChargeLock usage at
// the handler call site (not only the mutex helper unit test).
func TestChargeMercadoPagoPoint_ConcurrentSecondGets409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var createCount atomic.Int32
	firstCreateEntered := make(chan struct{})
	releaseFirstCreate := make(chan struct{})

	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/orders" {
			n := createCount.Add(1)
			if n == 1 {
				close(firstCreateEntered)
				<-releaseFirstCreate
				_, _ = w.Write([]byte(`{
					"id":"ORD01CONCURRENTPOINT",
					"status":"created",
					"transactions":{"payments":[{"amount":"10.00"}]}
				}`))
				return
			}
			// Second create must not run — lock + pending check prevent it.
			t.Errorf("unexpected second CreateOrder while first charge holds the lock / pending tracker")
			http.Error(w, "duplicate create", http.StatusConflict)
			return
		}
		t.Fatalf("unexpected MP call: %s %s", r.Method, r.URL.Path)
	}))

	bill := createPointTestBill(t, business.ID, 1000, 0)
	ph := NewPluginHandlers(nil, nil)

	type result struct {
		code    int
		orderID string
		body    string
	}
	results := make([]result, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})

	runCharge := func(i int) {
		defer wg.Done()
		<-start
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{
			{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
			{Key: "bill_id", Value: fmt.Sprintf("%d", bill.ID)},
		}
		c.Request = httptest.NewRequest(http.MethodPost, "/charge",
			strings.NewReader(`{"terminal_id":"NEWLAND_N950__T1","amount_cents":1000}`))
		c.Request.Header.Set("Content-Type", "application/json")
		ph.ChargeMercadoPagoPoint(c)
		orderID := ""
		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err == nil {
			if id, ok := resp["order_id"].(string); ok {
				orderID = id
			}
		}
		results[i] = result{code: w.Code, orderID: orderID, body: w.Body.String()}
	}

	wg.Add(2)
	go runCharge(0)
	go runCharge(1)
	close(start)

	// Wait until first CreateOrder is inside the lock, then release it.
	select {
	case <-firstCreateEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first CreateOrder to enter")
	}
	// Give the second goroutine time to block on the charge lock.
	time.Sleep(50 * time.Millisecond)
	close(releaseFirstCreate)
	wg.Wait()

	codes := []int{results[0].code, results[1].code}
	var created, conflicted int
	for _, r := range results {
		switch r.code {
		case http.StatusCreated:
			created++
			assert.Equal(t, "ORD01CONCURRENTPOINT", r.orderID)
		case http.StatusConflict:
			conflicted++
		default:
			t.Errorf("unexpected status %d body=%s", r.code, r.body)
		}
	}
	assert.Equal(t, 1, created, "exactly one charge must create an order; codes=%v", codes)
	assert.Equal(t, 1, conflicted, "second concurrent charge must 409; codes=%v results=%+v", codes, results)
	assert.Equal(t, int32(1), createCount.Load(), "CreateOrder must run once under the call-site lock")
}

// Q4a: same call-site lock pin for ChargeMercadoPagoQR.
func TestChargeMercadoPagoQR_ConcurrentSecondGets409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var createCount atomic.Int32
	firstCreateEntered := make(chan struct{})
	releaseFirstCreate := make(chan struct{})

	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/orders" {
			n := createCount.Add(1)
			if n == 1 {
				close(firstCreateEntered)
				<-releaseFirstCreate
				_, _ = w.Write([]byte(fmt.Sprintf(`{
					"id":"ORD01CONCURRENTQR",
					"status":"created",
					"type_response":{"qr_data":%q},
					"transactions":{"payments":[{"amount":"10.00"}]}
				}`, testQRPayload)))
				return
			}
			t.Errorf("unexpected second CreateOrder for concurrent QR charge")
			http.Error(w, "duplicate create", http.StatusConflict)
			return
		}
		t.Fatalf("unexpected MP call: %s %s", r.Method, r.URL.Path)
	}))

	mergeMercadoPagoConfigFields(t, business.ID,
		database.MergeBusinessPluginConfigField{Key: "mp_user_id", Value: "123"},
		database.MergeBusinessPluginConfigField{Key: "mp_store_id", Value: "store-1"},
		database.MergeBusinessPluginConfigField{Key: "mp_external_pos_id", Value: "payverge-pos-cached"},
	)

	bill := createPointTestBill(t, business.ID, 1000, 0)
	ph := NewPluginHandlers(nil, nil)

	type result struct {
		code int
		body string
	}
	results := make([]result, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})

	runCharge := func(i int) {
		defer wg.Done()
		<-start
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{
			{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
			{Key: "bill_id", Value: fmt.Sprintf("%d", bill.ID)},
		}
		c.Request = httptest.NewRequest(http.MethodPost, "/charge", bytes.NewBufferString(`{}`))
		c.Request.Header.Set("Content-Type", "application/json")
		ph.ChargeMercadoPagoQR(c)
		results[i] = result{code: w.Code, body: w.Body.String()}
	}

	wg.Add(2)
	go runCharge(0)
	go runCharge(1)
	close(start)

	select {
	case <-firstCreateEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first QR CreateOrder")
	}
	time.Sleep(50 * time.Millisecond)
	close(releaseFirstCreate)
	wg.Wait()

	var created, conflicted int
	for _, r := range results {
		switch r.code {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflicted++
		default:
			t.Errorf("unexpected status %d body=%s", r.code, r.body)
		}
	}
	assert.Equal(t, 1, created, "exactly one QR charge must succeed; results=%+v", results)
	assert.Equal(t, 1, conflicted, "second concurrent QR charge must 409; results=%+v", results)
	assert.Equal(t, int32(1), createCount.Load())
}
