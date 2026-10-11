package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// validateFakeProvider is a fiscal.Provider whose ValidateSettings returns a
// configured error (or nil); only ValidateSettings matters here.
type validateFakeProvider struct{ err error }

func (p *validateFakeProvider) Country() string { return "AR" }
func (p *validateFakeProvider) Name() string    { return "arca" }
func (p *validateFakeProvider) ValidateSettings(ctx context.Context, s fiscal.Settings) error {
	return p.err
}
func (p *validateFakeProvider) IssueReceipt(ctx context.Context, in fiscal.IssueInput) (*fiscal.ReceiptResult, error) {
	return nil, nil
}
func (p *validateFakeProvider) IssueCreditNote(ctx context.Context, in fiscal.CreditNoteInput) (*fiscal.ReceiptResult, error) {
	return nil, nil
}
func (p *validateFakeProvider) GetStatus(ctx context.Context, id string) (*fiscal.ReceiptStatus, error) {
	return nil, nil
}

type validateFakeFactory struct{ provider fiscal.Provider }

func (f *validateFakeFactory) Build(ctx context.Context, s *database.BusinessFiscalSettings) (fiscal.Provider, error) {
	return f.provider, nil
}

func TestFiscalHandlersPutSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))

	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.PUT("/businesses/:id/fiscal/settings", h.UpdateSettings)

	req := httptest.NewRequest(http.MethodPut, "/businesses/1/fiscal/settings", strings.NewReader(`{
		"country":"AR",
		"provider":"arca",
		"mode":"automatic_non_blocking",
		"environment":"sandbox",
		"tax_id":"20123456789",
		"tax_condition":"monotributo",
		"point_of_sale":1
	}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "automatic_non_blocking")
}

func TestFiscalHandlersPutSettingsRejectsRemovedBlockingMode(t *testing.T) {
	// automatic_blocking never blocked anything and is not a fiscal mode;
	// the handler rejects it like any other unknown value.
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))

	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.PUT("/businesses/:id/fiscal/settings", h.UpdateSettings)

	req := httptest.NewRequest(http.MethodPut, "/businesses/1/fiscal/settings", strings.NewReader(`{
		"country":"AR",
		"provider":"arca",
		"mode":"automatic_blocking",
		"environment":"sandbox",
		"tax_id":"20123456789",
		"tax_condition":"monotributo",
		"point_of_sale":1
	}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "invalid fiscal mode")
}

func TestFiscalHandlersPutSettingsRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "country",
			body: `{"country":"US","provider":"arca","mode":"manual","environment":"sandbox"}`,
		},
		{
			name: "provider",
			body: `{"country":"AR","provider":"unknown","mode":"manual","environment":"sandbox"}`,
		},
		{
			name: "arca for ae",
			body: `{"country":"AE","provider":"arca","mode":"manual","environment":"sandbox"}`,
		},
		{
			name: "edicom for ar",
			body: `{"country":"AR","provider":"edicom","mode":"manual","environment":"sandbox"}`,
		},
		{
			name: "environment",
			body: `{"country":"AR","provider":"arca","mode":"manual","environment":"staging"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, migrateFiscalHandlerDB(db))

			h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
			r := gin.New()
			r.PUT("/businesses/:id/fiscal/settings", h.UpdateSettings)

			req := httptest.NewRequest(http.MethodPut, "/businesses/1/fiscal/settings", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestFiscalHandlersPutSettingsDefaultsEnvironment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))

	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.PUT("/businesses/:id/fiscal/settings", h.UpdateSettings)

	req := httptest.NewRequest(http.MethodPut, "/businesses/1/fiscal/settings", strings.NewReader(`{
		"country":" ae ",
		"provider":" EDICOM ",
		"mode":"manual"
	}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"country":"AE"`)
	require.Contains(t, w.Body.String(), `"provider":"edicom"`)
	require.Contains(t, w.Body.String(), `"environment":"sandbox"`)
}

func TestFiscalHandlersIssueReceiptRejectsDifferentBusinessBill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))
	require.NoError(t, db.Create(&database.BusinessFiscalSettings{
		BusinessID:  1,
		Country:     "AR",
		Provider:    "arca",
		Mode:        database.FiscalModeManual,
		Environment: "sandbox",
		SetupStatus: "valid",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID:          22,
		BusinessID:  2,
		BillNumber:  "PV-test",
		Status:      database.BillStatusPaid,
		Items:       "[]",
		TotalAmount: 2400,
		PaidAmount:  2400,
	}).Error)

	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.POST("/businesses/:id/fiscal/receipts/issue", h.IssueReceipt)

	req := httptest.NewRequest(http.MethodPost, "/businesses/1/fiscal/receipts/issue", strings.NewReader(`{"bill_id":22}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	var count int64
	require.NoError(t, db.Model(&database.FiscalJob{}).Count(&count).Error)
	require.Zero(t, count)
}

// An issue request that names no bill is a client-side validation failure, and
// it must be answered as one: a 400 carrying a machine code, decided by the
// handler's own validation before any fiscal work is attempted. Live #906 saw
// a bare English string here, which gave the operator UI nothing to branch on
// or localize — it could only echo the backend's sentence verbatim.
func TestFiscalHandlersIssueReceiptEmptyRequestIsCodedValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))

	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.POST("/businesses/:id/fiscal/receipts/issue", h.IssueReceipt)

	// Both shapes of "no bill named": an empty object, and an explicit zero.
	for _, body := range []string{`{}`, `{"bill_id":0}`} {
		req := httptest.NewRequest(http.MethodPost, "/businesses/1/fiscal/receipts/issue", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		// 400 — not the 404 the service's bill lookup would answer for id 0,
		// which is what a request reaching the service would have produced.
		require.Equal(t, http.StatusBadRequest, w.Code, "body %s", body)

		var envelope struct {
			Error  string                 `json:"error"`
			Code   string                 `json:"code"`
			Params map[string]interface{} `json:"params"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope), "body %s", body)
		require.Equal(t, server.ErrCodeFieldInvalid, envelope.Code, "body %s", body)
		require.Equal(t, "bill_id", envelope.Params["field"], "body %s", body)
		require.NotEmpty(t, envelope.Error, "body %s", body)
	}

	// Validation short-circuited ahead of the service: nothing was enqueued and
	// nothing was recorded, so no provider call could ever have been reached.
	var jobs int64
	require.NoError(t, db.Model(&database.FiscalJob{}).Count(&jobs).Error)
	require.Zero(t, jobs)
	var receipts int64
	require.NoError(t, db.Model(&database.FiscalReceipt{}).Count(&receipts).Error)
	require.Zero(t, receipts)
}

// #907: re-issuing a bill that already carries a live factura used to answer
// 202 {"ok":true} — the per-bill idempotency key swallowed the duplicate and the
// operator saw "invoice issue queued" for work that never happened. It must be a
// coded 409 so the console can say the invoice already exists.
func TestFiscalHandlersIssueReceiptConflictsWhenBillAlreadyInvoiced(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))
	require.NoError(t, db.Create(&database.BusinessFiscalSettings{
		BusinessID:  1,
		Country:     "AR",
		Provider:    "arca",
		Mode:        database.FiscalModeManual,
		Environment: "sandbox",
		SetupStatus: "valid",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID:          88,
		BusinessID:  1,
		BillNumber:  "PV-88",
		Status:      database.BillStatusPaid,
		Items:       "[]",
		TotalAmount: 2400,
		PaidAmount:  2400,
	}).Error)
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 91, BusinessID: 1, SettingsID: 1, BillID: 88,
		Country: "AR", Provider: "arca", Action: fiscal.ActionIssueReceipt,
		ReceiptType: "factura_b", TotalAmountCents: 2400, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}).Error)

	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.POST("/businesses/:id/fiscal/receipts/issue", h.IssueReceipt)

	req := httptest.NewRequest(http.MethodPost, "/businesses/1/fiscal/receipts/issue", strings.NewReader(`{"bill_id":88}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var envelope struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.Equal(t, server.ErrCodeFiscalReceiptAlreadyIssued, envelope.Code)
	require.NotEmpty(t, envelope.Error)

	// And nothing was queued behind that refusal.
	var jobs int64
	require.NoError(t, db.Model(&database.FiscalJob{}).Count(&jobs).Error)
	require.Zero(t, jobs)
}

func TestFiscalHandlersRetryReceiptRejectsDifferentBusinessReceipt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID:               31,
		BusinessID:       2,
		SettingsID:       7,
		BillID:           44,
		Country:          "AR",
		Provider:         "arca",
		Action:           fiscal.ActionIssueReceipt,
		ReceiptType:      "B",
		TotalAmountCents: 2400,
		Currency:         "ARS",
		Status:           database.FiscalStatusFailedRetryable,
	}).Error)

	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.POST("/businesses/:id/fiscal/receipts/:receiptId/retry", h.RetryReceipt)

	req := httptest.NewRequest(http.MethodPost, "/businesses/1/fiscal/receipts/31/retry", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestFiscalHandlersRetryReceiptCreatesJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID:               32,
		BusinessID:       1,
		SettingsID:       7,
		BillID:           44,
		Country:          "AR",
		Provider:         "arca",
		Action:           fiscal.ActionIssueReceipt,
		ReceiptType:      "B",
		TotalAmountCents: 2400,
		Currency:         "ARS",
		Status:           database.FiscalStatusFailedRetryable,
	}).Error)

	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.POST("/businesses/:id/fiscal/receipts/:receiptId/retry", h.RetryReceipt)

	req := httptest.NewRequest(http.MethodPost, "/businesses/1/fiscal/receipts/32/retry", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)
	var job database.FiscalJob
	require.NoError(t, db.First(&job).Error)
	require.Equal(t, uint(1), job.BusinessID)
	require.Equal(t, uint(32), *job.ReceiptID)
	require.Equal(t, database.FiscalStatusPending, job.Status)
}

func TestFiscalHandlersCreditNoteCreatesJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID:               51,
		BusinessID:       1,
		SettingsID:       7,
		BillID:           44,
		Country:          "AR",
		Provider:         "arca",
		Action:           fiscal.ActionIssueReceipt,
		ReceiptType:      "B",
		TotalAmountCents: 2400,
		Currency:         "ARS",
		Status:           database.FiscalStatusAuthorized,
	}).Error)

	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.POST("/businesses/:id/fiscal/receipts/:receiptId/credit", h.CreditNote)

	req := httptest.NewRequest(http.MethodPost, "/businesses/1/fiscal/receipts/51/credit", strings.NewReader(`{"reason":"customer refund"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)
	var job database.FiscalJob
	require.NoError(t, db.First(&job).Error)
	require.Equal(t, uint(1), job.BusinessID)
	require.Equal(t, uint(51), *job.ReceiptID)
	require.Equal(t, fiscal.ActionCreditNote, job.Action)
	require.Equal(t, database.FiscalStatusPending, job.Status)
}

// API-CREDIT-01: a malformed body, a present amount_cents <= 0, or an amount
// above the receipt total is a 400 and enqueues no credit-note job.
func TestFiscalHandlersCreditNoteRejectsInvalidAmount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID:               51,
		BusinessID:       1,
		SettingsID:       7,
		BillID:           44,
		Country:          "AR",
		Provider:         "arca",
		Action:           fiscal.ActionIssueReceipt,
		ReceiptType:      "B",
		TotalAmountCents: 2400,
		Currency:         "ARS",
		Status:           database.FiscalStatusAuthorized,
	}).Error)

	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.POST("/businesses/:id/fiscal/receipts/:receiptId/credit", h.CreditNote)

	for _, body := range []string{
		`{"reason":"refund","amount_cents":-5}`,
		`{"reason":"refund","amount_cents":0}`,
		`{"reason":"refund","amount_cents":2401}`,
		`{"reason":"refund","amount_cents":"12"}`,
		`{"reason":`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/businesses/1/fiscal/receipts/51/credit", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code, body)
	}
	var n int64
	require.NoError(t, db.Model(&database.FiscalJob{}).Count(&n).Error)
	require.Zero(t, n, "no credit note may be enqueued for an invalid amount")

	// A partial amount within the total is still accepted.
	req := httptest.NewRequest(http.MethodPost, "/businesses/1/fiscal/receipts/51/credit", strings.NewReader(`{"reason":"refund","amount_cents":2400}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)
}

func TestEnqueueFiscalCreditNoteForRefundEnqueuesPerRefundIdempotently(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))
	prev := database.GetDB()
	database.SetTestDB(db)
	t.Cleanup(func() { database.SetTestDB(prev) })

	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID:               91,
		BusinessID:       1,
		SettingsID:       7,
		BillID:           44,
		Country:          "AR",
		Provider:         "arca",
		Action:           fiscal.ActionIssueReceipt,
		ReceiptType:      "C",
		TotalAmountCents: 12100,
		Currency:         "ARS",
		Status:           database.FiscalStatusAuthorized,
	}).Error)

	countCreditJobs := func() int64 {
		var n int64
		require.NoError(t, db.Model(&database.FiscalJob{}).
			Where("action = ?", fiscal.ActionCreditNote).Count(&n).Error)
		return n
	}

	// T7: a PARTIAL refund now enqueues a credit note for the refunded amount
	// (the worker + AR mapper honor the partial amount).
	enqueueFiscalCreditNoteForRefund(&database.Bill{ID: 44, BusinessID: 1, PaidAmount: 5000}, 5000, "payment:7", "operator")
	require.Equal(t, int64(1), countCreditJobs(), "partial refund must enqueue a credit note")

	// A distinct refund (different discriminator) enqueues a second credit note.
	enqueueFiscalCreditNoteForRefund(&database.Bill{ID: 44, BusinessID: 1, PaidAmount: 0}, 5000, "payment:8", "operator")
	require.Equal(t, int64(2), countCreditJobs(), "a distinct refund enqueues its own credit note")

	// Idempotent: a retried refund of the SAME payment does not double-credit.
	enqueueFiscalCreditNoteForRefund(&database.Bill{ID: 44, BusinessID: 1, PaidAmount: 0}, 5000, "payment:7", "operator")
	require.Equal(t, int64(2), countCreditJobs(), "credit-note enqueue must be idempotent per refund")
}

func TestFiscalHandlersResendReceipt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))

	// Delivery context the resend loads (LoadJobContext): business, bill, settings.
	require.NoError(t, db.Create(&database.Business{ID: 1, BusinessId: "biz-resend", Name: "Biz", OwnerAddress: "o", SettlementAddr: "s", TippingAddr: "t"}).Error)
	require.NoError(t, db.Create(&database.Bill{ID: 44, BusinessID: 1, BillNumber: "PV-resend", Status: database.BillStatusPaid, Items: "[]", TotalAmount: 12100, PaidAmount: 12100}).Error)
	require.NoError(t, db.Create(&database.BusinessFiscalSettings{ID: 7, BusinessID: 1, Country: "AR", Provider: "arca", TaxID: "20123456789", TaxCondition: "monotributo"}).Error)

	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 70, BusinessID: 1, SettingsID: 7, BillID: 44,
		Country: "AR", Provider: "arca", Action: fiscal.ActionIssueReceipt,
		ReceiptType: "C", TotalAmountCents: 12100, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}).Error)
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 71, BusinessID: 1, SettingsID: 7, BillID: 44,
		Country: "AR", Provider: "arca", Action: fiscal.ActionIssueReceipt,
		ReceiptType: "C", TotalAmountCents: 12100, Currency: "ARS",
		Status: database.FiscalStatusFailedRetryable,
	}).Error)

	// No dispatcher wired → delivery no-ops, but the action (and its audit) succeed.
	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.POST("/businesses/:id/fiscal/receipts/:receiptId/resend", h.ResendReceipt)

	do := func(path string) int {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
		return w.Code
	}

	require.Equal(t, http.StatusAccepted, do("/businesses/1/fiscal/receipts/70/resend"), "authorized receipt → 202")
	require.Equal(t, http.StatusConflict, do("/businesses/1/fiscal/receipts/71/resend"), "non-authorized receipt → 409")
	require.Equal(t, http.StatusNotFound, do("/businesses/1/fiscal/receipts/999/resend"), "missing receipt → 404")
}

func TestFiscalHandlersCreditNoteRejectsNonCreditableReceipt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID:               52,
		BusinessID:       1,
		SettingsID:       7,
		BillID:           44,
		Country:          "AR",
		Provider:         "arca",
		Action:           fiscal.ActionIssueReceipt,
		ReceiptType:      "B",
		TotalAmountCents: 2400,
		Currency:         "ARS",
		Status:           database.FiscalStatusFailedRetryable,
	}).Error)

	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.POST("/businesses/:id/fiscal/receipts/:receiptId/credit", h.CreditNote)

	req := httptest.NewRequest(http.MethodPost, "/businesses/1/fiscal/receipts/52/credit", strings.NewReader(`{"reason":"refund"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code)
	require.Zero(t, countFiscalJobsForTest(t, db))
}

func TestFiscalHandlersCreditNoteRejectsDifferentBusinessReceipt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID:               53,
		BusinessID:       2,
		SettingsID:       7,
		BillID:           44,
		Country:          "AR",
		Provider:         "arca",
		Action:           fiscal.ActionIssueReceipt,
		ReceiptType:      "B",
		TotalAmountCents: 2400,
		Currency:         "ARS",
		Status:           database.FiscalStatusAuthorized,
	}).Error)

	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.POST("/businesses/:id/fiscal/receipts/:receiptId/credit", h.CreditNote)

	req := httptest.NewRequest(http.MethodPost, "/businesses/1/fiscal/receipts/53/credit", strings.NewReader(`{"reason":"refund"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func countFiscalJobsForTest(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&database.FiscalJob{}).Count(&count).Error)
	return count
}

func TestFiscalHandlersValidateSettings_Ready(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))
	require.NoError(t, db.Create(&database.BusinessFiscalSettings{
		BusinessID:  1,
		Country:     "AR",
		Provider:    "arca",
		Mode:        database.FiscalModeManual,
		Environment: "sandbox",
		SetupStatus: "credentials_set",
	}).Error)

	svc := fiscal.NewService(db, fiscal.NewProviderRegistry()).
		WithProviderFactory(&validateFakeFactory{provider: &validateFakeProvider{}})
	h := NewFiscalHandlers(svc)
	r := gin.New()
	r.POST("/businesses/:id/fiscal/validate", h.ValidateSettings)

	req := httptest.NewRequest(http.MethodPost, "/businesses/1/fiscal/validate", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"setup_status":"ready"`)
}

func TestFiscalHandlersValidateSettings_FailureReturns200WithError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))
	require.NoError(t, db.Create(&database.BusinessFiscalSettings{
		BusinessID:  1,
		Country:     "AR",
		Provider:    "arca",
		Mode:        database.FiscalModeManual,
		Environment: "sandbox",
		SetupStatus: "credentials_set",
	}).Error)

	svc := fiscal.NewService(db, fiscal.NewProviderRegistry()).
		WithProviderFactory(&validateFakeFactory{provider: &validateFakeProvider{err: errors.New("wsfe FEDummy: AppServer is not OK")}})
	h := NewFiscalHandlers(svc)
	r := gin.New()
	r.POST("/businesses/:id/fiscal/validate", h.ValidateSettings)

	req := httptest.NewRequest(http.MethodPost, "/businesses/1/fiscal/validate", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "AppServer is not OK")
	require.NotContains(t, w.Body.String(), `"setup_status":"ready"`)
}

func TestFiscalHandlersValidateSettings_NoSettingsReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))

	svc := fiscal.NewService(db, fiscal.NewProviderRegistry()).
		WithProviderFactory(&validateFakeFactory{provider: &validateFakeProvider{}})
	h := NewFiscalHandlers(svc)
	r := gin.New()
	r.POST("/businesses/:id/fiscal/validate", h.ValidateSettings)

	req := httptest.NewRequest(http.MethodPost, "/businesses/99/fiscal/validate", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestFiscalHandlersListReceiptsMasksCustomerDocNumber locks F-PII: the receipts
// list must not expose a customer's full CUIT/DNI; it returns a masked value
// retaining only the last few digits.
func TestFiscalHandlersListReceiptsMasksCustomerDocNumber(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))

	docType := "CUIT"
	docNumber := "20123456789"
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 1, BusinessID: 1, SettingsID: 1, BillID: 1,
		Country: "AR", Provider: "arca", Action: fiscal.ActionIssueReceipt,
		ReceiptType: "factura_a", TotalAmountCents: 1000, Currency: "ARS",
		Status:          database.FiscalStatusAuthorized,
		CustomerDocType: &docType, CustomerDocNumber: &docNumber,
	}).Error)

	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.GET("/businesses/:id/fiscal/receipts", h.ListReceipts)

	req := httptest.NewRequest(http.MethodGet, "/businesses/1/fiscal/receipts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	require.NotContains(t, body, "20123456789", "full customer doc number must not leak in the list")
	require.Contains(t, body, "789", "masked doc should retain the last 3 digits")
	require.Contains(t, body, `"items"`, "legacy path must keep {items: ...} envelope")
}

// TestFiscalHandlersListReceiptsPageMasksAndEmbeds locks F-PII on the paged path
// and asserts the ReceiptsPage top-level shape (not legacy items-only).
func TestFiscalHandlersListReceiptsPageMasksAndEmbeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))

	docType := "CUIT"
	docNumber := "20123456789"
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 1, BusinessID: 1, SettingsID: 1, BillID: 1,
		Country: "AR", Provider: "arca", Action: fiscal.ActionIssueReceipt,
		ReceiptType: "factura_a", TotalAmountCents: 1000, Currency: "ARS",
		Status:          database.FiscalStatusAuthorized,
		CustomerDocType: &docType, CustomerDocNumber: &docNumber,
	}).Error)
	now := time.Now().UTC()
	require.NoError(t, db.Create(&database.FiscalDeliveryTask{
		BusinessID: 1, ReceiptID: 1, Channel: database.FiscalDeliveryChannelEmail,
		Status: database.FiscalDeliveryStatusSucceeded, MaxAttempts: 8,
		IdempotencyKey: "r1:email", NextAttemptAt: &now,
	}).Error)

	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.GET("/businesses/:id/fiscal/receipts", h.ListReceipts)

	req := httptest.NewRequest(http.MethodGet, "/businesses/1/fiscal/receipts?page=1&page_size=20", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	require.NotContains(t, body, "20123456789", "full customer doc number must not leak on paged path")
	require.Contains(t, body, "789")
	require.Contains(t, body, `"receipts"`)
	require.Contains(t, body, `"total"`)
	require.Contains(t, body, `"delivery"`)
	require.Contains(t, body, `"needs_attention"`)
	require.NotContains(t, body, `"items"`, "paged path must not use legacy items envelope")
}

func migrateFiscalHandlerDB(db *gorm.DB) error {
	return db.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.Bill{},
		&database.BusinessFiscalSettings{},
		&database.FiscalReceipt{},
		&database.FiscalJob{},
		&database.FiscalAuditEvent{},
		&database.FiscalDeliveryTask{},
	)
}

// Cross-tenant: business B cannot list delivery tasks for business A's receipt.
func TestFiscalHandlersListDelivery_CrossTenantDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))

	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 10, BusinessID: 1, SettingsID: 1, BillID: 1,
		Country: "AR", Provider: "arca", Action: fiscal.ActionIssueReceipt,
		ReceiptType: "factura_c", TotalAmountCents: 1000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}).Error)
	now := time.Now().UTC()
	require.NoError(t, db.Create(&database.FiscalDeliveryTask{
		BusinessID: 1, ReceiptID: 10, Channel: database.FiscalDeliveryChannelEmail,
		Status: database.FiscalDeliveryStatusDead, MaxAttempts: 8,
		IdempotencyKey: "t1", NextAttemptAt: &now,
	}).Error)

	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.GET("/businesses/:id/fiscal/receipts/:receiptId/delivery", h.ListReceiptDelivery)

	// Business 2 querying business 1's receipt → 404
	req := httptest.NewRequest(http.MethodGet, "/businesses/2/fiscal/receipts/10/delivery", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)

	// Business 1 → 200 with task
	req2 := httptest.NewRequest(http.MethodGet, "/businesses/1/fiscal/receipts/10/delivery", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)
	require.Contains(t, w2.Body.String(), "email")
	require.NotContains(t, w2.Body.String(), "smtp", "raw error must not leak")
}

func TestFiscalHandlersRetryDelivery_DoubleClickIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))

	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 11, BusinessID: 1, SettingsID: 1, BillID: 1,
		Country: "AR", Provider: "arca", Action: fiscal.ActionIssueReceipt,
		ReceiptType: "factura_c", TotalAmountCents: 1000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}).Error)
	now := time.Now().UTC()
	require.NoError(t, db.Create(&database.FiscalDeliveryTask{
		ID: 50, BusinessID: 1, ReceiptID: 11, Channel: database.FiscalDeliveryChannelPrint,
		Status: database.FiscalDeliveryStatusDead, MaxAttempts: 8, Attempts: 8,
		IdempotencyKey: "print-1", LastError: "print offline", DeadAt: &now,
	}).Error)

	h := NewFiscalHandlers(fiscal.NewService(db, fiscal.NewProviderRegistry()))
	r := gin.New()
	r.POST("/businesses/:id/fiscal/delivery-tasks/:taskId/retry", h.RetryDeliveryTask)

	do := func() int {
		req := httptest.NewRequest(http.MethodPost, "/businesses/1/fiscal/delivery-tasks/50/retry", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	require.Equal(t, http.StatusAccepted, do())
	require.Equal(t, http.StatusAccepted, do(), "second click must be idempotent 202")

	// Cross-tenant retry denied
	req := httptest.NewRequest(http.MethodPost, "/businesses/99/fiscal/delivery-tasks/50/retry", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)

	var task database.FiscalDeliveryTask
	require.NoError(t, db.First(&task, 50).Error)
	require.Equal(t, database.FiscalDeliveryStatusPending, task.Status)
}

// credentialsErrStub is a fiscalService whose SetCredentials returns a fixed error.
type credentialsErrStub struct{ err error }

func (s *credentialsErrStub) UpdateSettings(ctx context.Context, settings *database.BusinessFiscalSettings) error {
	return nil
}
func (s *credentialsErrStub) GetSettings(ctx context.Context, businessID uint) (*database.BusinessFiscalSettings, error) {
	return nil, nil
}
func (s *credentialsErrStub) ListReceipts(ctx context.Context, businessID uint, status string) ([]database.FiscalReceipt, error) {
	return nil, nil
}
func (s *credentialsErrStub) ListReceiptsPage(ctx context.Context, params fiscal.ListReceiptsParams) (*fiscal.ReceiptsPage, error) {
	return nil, nil
}
func (s *credentialsErrStub) ListIssuableBills(ctx context.Context, businessID uint, q string) ([]fiscal.IssuableBill, error) {
	return nil, nil
}
func (s *credentialsErrStub) IssueReceipt(ctx context.Context, businessID, billID uint, actor string) error {
	return nil
}
func (s *credentialsErrStub) IssueReceiptWithReceiver(ctx context.Context, businessID, billID uint, actor string, override *fiscal.ReceiverOverride) error {
	return nil
}
func (s *credentialsErrStub) RetryReceipt(ctx context.Context, businessID, receiptID uint, actor string) error {
	return nil
}
func (s *credentialsErrStub) ResendReceipt(ctx context.Context, businessID, receiptID uint, actor string) error {
	return nil
}
func (s *credentialsErrStub) IssueCreditNote(ctx context.Context, businessID, receiptID uint, amountCents int64, discriminator, reason, actor string) error {
	return nil
}
func (s *credentialsErrStub) SetCredentials(ctx context.Context, businessID uint, certPEM, keyPEM string) error {
	return s.err
}
func (s *credentialsErrStub) ValidateSettings(ctx context.Context, businessID uint) (*database.BusinessFiscalSettings, error) {
	return nil, nil
}
func (s *credentialsErrStub) ListReceiptDeliveryTasks(ctx context.Context, businessID, receiptID uint) ([]fiscal.DeliveryTaskView, error) {
	return nil, nil
}
func (s *credentialsErrStub) RetryDeliveryTask(ctx context.Context, businessID, taskID uint, actor string) error {
	return nil
}

func postCredentialUpload(t *testing.T, svc fiscalService) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := &FiscalHandlers{svc: svc}
	r := gin.New()
	r.POST("/businesses/:id/fiscal/credentials", h.UploadCredentials)
	req := httptest.NewRequest(http.MethodPost, "/businesses/1/fiscal/credentials", strings.NewReader(`{"cert_pem":"CERT","key_pem":"KEY"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// API-FISCAL-CRED-01: SetCredentials failures must not echo raw error text.
// Encrypt/DB failures are 500 with a fixed message; invalid pairs and missing
// settings are 400 with fixed copy.
func TestFiscalHandlersUploadCredentials_HidesInternalErrors(t *testing.T) {
	t.Run("encrypt failure is 500 without detail", func(t *testing.T) {
		w := postCredentialUpload(t, &credentialsErrStub{err: errors.New("encrypt credentials: secret-detail-xyz")})
		require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), "secret-detail-xyz")
		require.Contains(t, w.Body.String(), "failed to store credentials")
	})

	t.Run("invalid credentials is 400 with fixed text", func(t *testing.T) {
		err := fmt.Errorf("parse bundle: %w", fiscal.ErrInvalidCredentials)
		w := postCredentialUpload(t, &credentialsErrStub{err: err})
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "certificate and private key are not a valid matching pair")
		require.NotContains(t, w.Body.String(), "parse bundle")
	})

	t.Run("missing settings is 400", func(t *testing.T) {
		w := postCredentialUpload(t, &credentialsErrStub{err: fiscal.ErrNoFiscalSettings})
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), fiscal.ErrNoFiscalSettings.Error())
	})
}
