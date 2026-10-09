package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// listIssuableMock implements fiscalService for ListIssuableBills envelope tests.
type listIssuableMock struct {
	bills []fiscal.IssuableBill
	err   error
}

func (m *listIssuableMock) UpdateSettings(ctx context.Context, settings *database.BusinessFiscalSettings) error {
	panic("unexpected UpdateSettings")
}
func (m *listIssuableMock) GetSettings(ctx context.Context, businessID uint) (*database.BusinessFiscalSettings, error) {
	panic("unexpected GetSettings")
}
func (m *listIssuableMock) ListReceipts(ctx context.Context, businessID uint, status string) ([]database.FiscalReceipt, error) {
	panic("unexpected ListReceipts")
}
func (m *listIssuableMock) ListReceiptsPage(ctx context.Context, params fiscal.ListReceiptsParams) (*fiscal.ReceiptsPage, error) {
	panic("unexpected ListReceiptsPage")
}
func (m *listIssuableMock) ListIssuableBills(ctx context.Context, businessID uint, q string) ([]fiscal.IssuableBill, error) {
	return m.bills, m.err
}
func (m *listIssuableMock) IssueReceipt(ctx context.Context, businessID, billID uint, actor string) error {
	panic("unexpected IssueReceipt")
}
func (m *listIssuableMock) IssueReceiptWithReceiver(ctx context.Context, businessID, billID uint, actor string, override *fiscal.ReceiverOverride) error {
	panic("unexpected IssueReceiptWithReceiver")
}
func (m *listIssuableMock) RetryReceipt(ctx context.Context, businessID, receiptID uint, actor string) error {
	panic("unexpected RetryReceipt")
}
func (m *listIssuableMock) ResendReceipt(ctx context.Context, businessID, receiptID uint, actor string) error {
	panic("unexpected ResendReceipt")
}
func (m *listIssuableMock) IssueCreditNote(ctx context.Context, businessID, receiptID uint, amountCents int64, discriminator, reason, actor string) error {
	panic("unexpected IssueCreditNote")
}
func (m *listIssuableMock) SetCredentials(ctx context.Context, businessID uint, certPEM, keyPEM string) error {
	panic("unexpected SetCredentials")
}
func (m *listIssuableMock) ValidateSettings(ctx context.Context, businessID uint) (*database.BusinessFiscalSettings, error) {
	panic("unexpected ValidateSettings")
}
func (m *listIssuableMock) ListReceiptDeliveryTasks(ctx context.Context, businessID, receiptID uint) ([]fiscal.DeliveryTaskView, error) {
	panic("unexpected ListReceiptDeliveryTasks")
}
func (m *listIssuableMock) RetryDeliveryTask(ctx context.Context, businessID, taskID uint, actor string) error {
	panic("unexpected RetryDeliveryTask")
}

// L6-21: ListIssuableBills must JSON-emit the canonical `.bills` key (siblings
// use .bills / .data). A dual `.items` alias is allowed; dropping `.bills` is not.
// Rule 3: if production only emits `.items`, this response decode fails.
func TestListIssuableBills_JSONEmitsBillsKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	closed := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	mock := &listIssuableMock{
		bills: []fiscal.IssuableBill{{
			BillID:      42,
			BillNumber:  "B-42",
			TableLabel:  "T1",
			ClosedAt:    &closed,
			TotalAmount: 12.5,
			Currency:    "USD",
		}},
	}
	h := &FiscalHandlers{svc: mock}
	r := gin.New()
	r.GET("/businesses/:id/fiscal/issuable-bills", h.ListIssuableBills)

	req := httptest.NewRequest(http.MethodGet, "/businesses/7/fiscal/issuable-bills", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Contains(t, body, "bills",
		"L6-21: response must include bills key (not items-only)")

	var bills []fiscal.IssuableBill
	require.NoError(t, json.Unmarshal(body["bills"], &bills))
	require.Len(t, bills, 1)
	require.Equal(t, uint(42), bills[0].BillID)
	require.Equal(t, "B-42", bills[0].BillNumber)
}

func TestListIssuableBills_EmptyListStillHasBillsKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &FiscalHandlers{svc: &listIssuableMock{bills: nil}}
	r := gin.New()
	r.GET("/businesses/:id/fiscal/issuable-bills", h.ListIssuableBills)

	req := httptest.NewRequest(http.MethodGet, "/businesses/7/fiscal/issuable-bills", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Contains(t, body, "bills")
	var bills []fiscal.IssuableBill
	require.NoError(t, json.Unmarshal(body["bills"], &bills))
	require.Empty(t, bills)
}
