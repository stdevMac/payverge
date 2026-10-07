package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestPrintJobHandlers_MarkPrintedTransitions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	job := database.PrintJob{
		BusinessID: business.ID, Kind: database.PrintJobKindBill,
		SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusPrinting,
	}
	require.NoError(t, db.Create(&job).Error)
	h := NewPrintJobHandlers(db)

	router := gin.New()
	router.POST("/businesses/:id/print/jobs/:jobId/mark-printed", h.MarkPrinted)

	bizID := strconv.FormatUint(uint64(business.ID), 10)
	jobID := strconv.FormatUint(uint64(job.ID), 10)
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+bizID+"/print/jobs/"+jobID+"/mark-printed", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var reloaded database.PrintJob
	require.NoError(t, db.First(&reloaded, job.ID).Error)
	require.Equal(t, database.PrintJobStatusPrinted, reloaded.Status)
}

// TestPrintJobHandlers_ListJobsFiltersWithoutAmbiguity guards against a
// regression where ListJobs joined the printers table (which also has a
// business_id column and a created_at column) without qualifying the
// print_jobs columns, producing an ambiguous-column SQL error on every poll.
// Under sqlite the same unqualified column raises "ambiguous column name", so
// this test fails before the fix and passes after.
func TestPrintJobHandlers_ListJobsFiltersWithoutAmbiguity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)

	printer := database.Printer{
		BusinessID: business.ID, Name: "front", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	require.NoError(t, db.Create(&printer).Error)

	routed := database.PrintJob{
		BusinessID: business.ID, PrinterID: &printer.ID, Kind: database.PrintJobKindBill,
		SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusRouted,
	}
	require.NoError(t, db.Create(&routed).Error)

	// A job in another status must be filtered out by ?status=routed.
	printed := database.PrintJob{
		BusinessID: business.ID, PrinterID: &printer.ID, Kind: database.PrintJobKindBill,
		SourceType: "bill", SourceID: 2, Status: database.PrintJobStatusPrinted,
	}
	require.NoError(t, db.Create(&printed).Error)

	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.GET("/businesses/:id/print/jobs", h.ListJobs)

	bizID := strconv.FormatUint(uint64(business.ID), 10)
	req := httptest.NewRequest(http.MethodGet,
		"/businesses/"+bizID+"/print/jobs?status=routed&transport=browser", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var out struct {
		Items []database.PrintJob `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	require.Len(t, out.Items, 1)
	require.Equal(t, routed.ID, out.Items[0].ID)
	require.Equal(t, database.PrintJobStatusRouted, out.Items[0].Status)
}

func TestPrintJobHandlers_ListJobsOmitsPayloadHTML(t *testing.T) {
	// FIND-041: list is history UI only; payload HTML stays on claim/reprint.
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)

	html := "<html><body>SECRET TICKET CONTENTS</body></html>"
	job := database.PrintJob{
		BusinessID: business.ID, Kind: database.PrintJobKindKitchen,
		SourceType: "order", SourceID: 9, Status: database.PrintJobStatusPrinted,
		PayloadHTML: &html, MaxAttempts: 6,
	}
	require.NoError(t, db.Create(&job).Error)

	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.GET("/businesses/:id/print/jobs", h.ListJobs)

	bizID := strconv.FormatUint(uint64(business.ID), 10)
	req := httptest.NewRequest(http.MethodGet, "/businesses/"+bizID+"/print/jobs?limit=10", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
	require.NotContains(t, rr.Body.String(), "SECRET TICKET CONTENTS")

	var out struct {
		Items []database.PrintJob `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	require.Len(t, out.Items, 1)
	require.Equal(t, job.ID, out.Items[0].ID)
	require.Nil(t, out.Items[0].PayloadHTML)
}

func TestPrintJobHandlers_ListJobsHonorsLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	for i := 0; i < 5; i++ {
		j := database.PrintJob{
			BusinessID: business.ID, Kind: database.PrintJobKindReceipt,
			SourceType: "bill", SourceID: uint(i + 1), Status: database.PrintJobStatusPrinted,
		}
		require.NoError(t, db.Create(&j).Error)
	}
	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.GET("/businesses/:id/print/jobs", h.ListJobs)
	bizID := strconv.FormatUint(uint64(business.ID), 10)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/businesses/"+bizID+"/print/jobs?limit=2", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	var out struct {
		Items []database.PrintJob `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	require.Len(t, out.Items, 2)
}

func TestPrintJobHandlers_ReprintBlockedForKitchen(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	job := database.PrintJob{
		BusinessID: business.ID, Kind: database.PrintJobKindKitchen,
		SourceType: "order", SourceID: 1, Status: database.PrintJobStatusPrinted,
	}
	require.NoError(t, db.Create(&job).Error)
	h := NewPrintJobHandlers(db)

	router := gin.New()
	router.POST("/businesses/:id/print/jobs/:jobId/reprint", h.Reprint)

	bizID := strconv.FormatUint(uint64(business.ID), 10)
	jobID := strconv.FormatUint(uint64(job.ID), 10)
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+bizID+"/print/jobs/"+jobID+"/reprint", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusForbidden, rr.Code, "body: %s", rr.Body.String())
}

func TestPrintJobHandlers_ReprintRejectsNonterminalReceipt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	job := database.PrintJob{
		BusinessID: business.ID, Kind: database.PrintJobKindReceipt,
		SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusRouted,
	}
	require.NoError(t, db.Create(&job).Error)
	var beforeCount int64
	require.NoError(t, db.Model(&database.PrintJob{}).Count(&beforeCount).Error)
	h := NewPrintJobHandlers(db)

	router := gin.New()
	router.POST("/businesses/:id/print/jobs/:jobId/reprint", h.Reprint)
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+strconv.FormatUint(uint64(business.ID), 10)+
			"/print/jobs/"+strconv.FormatUint(uint64(job.ID), 10)+"/reprint", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusConflict, rr.Code, "body: %s", rr.Body.String())

	var count int64
	require.NoError(t, db.Model(&database.PrintJob{}).Count(&count).Error)
	require.Equal(t, beforeCount, count)
}

func TestPrintJobHandlers_CreateBillJobReturnsRoutedWithPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)

	// Need a bill + table for the formatter; setupPrinterHandlerTestDB only
	// creates Business + Printer + PrintJob. Extend on the fly here.
	require.NoError(t, db.AutoMigrate(&database.Table{}, &database.Bill{}, &database.Payment{}))
	table := database.Table{BusinessID: business.ID, TableCode: "tbl-create", Name: "T1"}
	require.NoError(t, db.Create(&table).Error)
	bill := database.Bill{
		BusinessID: business.ID, TableID: table.ID,
		BillNumber: "BN-create-job", SettlementAddr: "0xs", TippingAddr: "0xt",
		Status: database.BillStatusOpen, Subtotal: 1000, TotalAmount: 1000,
	}
	require.NoError(t, db.Create(&bill).Error)

	printer := database.Printer{
		BusinessID: business.ID, Name: "front", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	require.NoError(t, db.Create(&printer).Error)

	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.POST("/businesses/:id/print/jobs", h.Create)

	bizID := strconv.FormatUint(uint64(business.ID), 10)
	body := []byte(`{"kind":"bill","source_type":"bill","source_id":` +
		strconv.FormatUint(uint64(bill.ID), 10) + `,"language":"en"}`)
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+bizID+"/print/jobs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusCreated, rr.Code, "body: %s", rr.Body.String())

	var job database.PrintJob
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &job))
	require.Equal(t, database.PrintJobStatusRouted, job.Status)
	require.NotNil(t, job.PayloadHTML)
	require.NotEmpty(t, *job.PayloadHTML)
}

func TestPrintJobHandlers_CreateReceiptRequiresPaidBill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.Table{}, &database.Bill{}, &database.Payment{}))
	bill := database.Bill{
		BusinessID: business.ID, BillNumber: "BN-unpaid-receipt",
		SettlementAddr: "0xs", TippingAddr: "0xt",
		Status: database.BillStatusOpen, TotalAmount: 1000,
	}
	require.NoError(t, db.Create(&bill).Error)
	var beforeCount int64
	require.NoError(t, db.Model(&database.PrintJob{}).Count(&beforeCount).Error)

	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.POST("/businesses/:id/print/jobs", h.Create)
	body := []byte(`{"kind":"receipt","source_type":"bill","source_id":` +
		strconv.FormatUint(uint64(bill.ID), 10) + `}`)
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+strconv.FormatUint(uint64(business.ID), 10)+"/print/jobs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusConflict, rr.Code, "body: %s", rr.Body.String())

	var count int64
	require.NoError(t, db.Model(&database.PrintJob{}).Count(&count).Error)
	require.Equal(t, beforeCount, count)
}

func TestPrintJobHandlers_CreateRejectsKindSourceMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.Order{}))
	order := database.Order{BusinessID: business.ID}
	require.NoError(t, db.Create(&order).Error)
	var beforeCount int64
	require.NoError(t, db.Model(&database.PrintJob{}).Count(&beforeCount).Error)

	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.POST("/businesses/:id/print/jobs", h.Create)
	body := []byte(`{"kind":"receipt","source_type":"order","source_id":` +
		strconv.FormatUint(uint64(order.ID), 10) + `}`)
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+strconv.FormatUint(uint64(business.ID), 10)+"/print/jobs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusBadRequest, rr.Code, "body: %s", rr.Body.String())

	var count int64
	require.NoError(t, db.Model(&database.PrintJob{}).Count(&count).Error)
	require.Equal(t, beforeCount, count)
}

func TestPrintJobHandlers_CreateRejectsUnauthorizedReceiptKind(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(authorizedAnyPermissionsContextKey, []string{"print:bill"})
	})
	router.POST("/businesses/:id/print/jobs", h.Create)
	body := []byte(`{"kind":"receipt","source_type":"bill","source_id":1}`)
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+strconv.FormatUint(uint64(business.ID), 10)+"/print/jobs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusForbidden, rr.Code)
}

// TestPrintJobHandlers_CreateRejectsCrossTenantBill verifies that a print
// job Create call with a source_id belonging to a different business is
// rejected and does not leak the other business's data. (PRINT-XTENANT-SOURCEID-1)
func TestPrintJobHandlers_CreateRejectsCrossTenantBill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, bizA := setupPrinterHandlerTestDB(t)

	// Create a second business (B) that owns a bill with sensitive data.
	require.NoError(t, db.AutoMigrate(&database.Table{}, &database.Bill{}, &database.Payment{}))
	bizB := database.Business{
		BusinessId:     "bizB-" + t.Name(),
		OwnerAddress:   "0xownerB",
		Name:           "Victim Diner",
		SettlementAddr: "0xSETTLE_B",
		TippingAddr:    "0xTIP_B",
	}
	require.NoError(t, db.Create(&bizB).Error)

	tableB := database.Table{BusinessID: bizB.ID, TableCode: "tbl-victim", Name: "T1"}
	require.NoError(t, db.Create(&tableB).Error)

	billB := database.Bill{
		BusinessID: bizB.ID, TableID: tableB.ID,
		BillNumber: "BN-VICTIM-LEAK", SettlementAddr: "0xsB", TippingAddr: "0xtB",
		Status: database.BillStatusPaid, Subtotal: 5000, TotalAmount: 5000,
	}
	require.NoError(t, db.Create(&billB).Error)

	// Add a confirmed crypto payment with a TxHash for B's bill.
	payment := database.Payment{
		BillID:        billB.ID,
		PayerAddr:     "0xpayer",
		Amount:        5000,
		TxHash:        "0xdeadbeefTXLEAK",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
	}
	require.NoError(t, db.Create(&payment).Error)

	// Printer for business A.
	printerA := database.Printer{
		BusinessID: bizA.ID, Name: "frontA", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	require.NoError(t, db.Create(&printerA).Error)

	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.POST("/businesses/:id/print/jobs", h.Create)

	bizAID := strconv.FormatUint(uint64(bizA.ID), 10)
	body := []byte(`{"kind":"bill","source_type":"bill","source_id":` +
		strconv.FormatUint(uint64(billB.ID), 10) + `,"language":"en"}`)
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+bizAID+"/print/jobs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equalf(t, http.StatusNotFound, rr.Code,
		"cross-tenant print must return 404, body: %s", rr.Body.String())

	var job database.PrintJob
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &job))

	// The response must NOT leak B's data. Payload must be unconditionally
	// empty — a 404 from the handler-level gate shouldn't produce any
	// payload_html at all.
	require.Nil(t, job.PayloadHTML, "cross-tenant job must have nil payload_html")
}

// TestPrintJobHandlers_CreateRejectsCrossTenantKitchen verifies the
// same tenant check for kitchen/bar ticket kinds.
func TestPrintJobHandlers_CreateRejectsCrossTenantKitchen(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, bizA := setupPrinterHandlerTestDB(t)

	require.NoError(t, db.AutoMigrate(&database.Table{}, &database.Bill{}, &database.Payment{}, &database.Order{}))

	bizB := database.Business{
		BusinessId:     "bizB-kitchen-" + t.Name(),
		OwnerAddress:   "0xownerB",
		Name:           "Victim Kitchen",
		SettlementAddr: "0xSETTLE_BK",
		TippingAddr:    "0xTIP_BK",
	}
	require.NoError(t, db.Create(&bizB).Error)

	tableB := database.Table{BusinessID: bizB.ID, TableCode: "tbl-kv", Name: "K1"}
	require.NoError(t, db.Create(&tableB).Error)

	billB := database.Bill{
		BusinessID: bizB.ID, TableID: tableB.ID,
		BillNumber: "BN-KITCHEN-VICTIM", SettlementAddr: "0xsBK", TippingAddr: "0xtBK",
		Status: database.BillStatusOpen, Subtotal: 3000, TotalAmount: 3000,
	}
	require.NoError(t, db.Create(&billB).Error)

	// Order for bizB.
	orderB := database.Order{
		BillID: billB.ID, BusinessID: bizB.ID,
		OrderNumber: "O-VICTIM", Status: database.OrderStatusPending,
		CreatedBy: "guest", Items: "[]",
	}
	require.NoError(t, db.Create(&orderB).Error)

	printerA := database.Printer{
		BusinessID: bizA.ID, Name: "kitchenA", Role: "kitchen",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	require.NoError(t, db.Create(&printerA).Error)

	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.POST("/businesses/:id/print/jobs", h.Create)

	bizAID := strconv.FormatUint(uint64(bizA.ID), 10)
	body := []byte(`{"kind":"kitchen","source_type":"order","source_id":` +
		strconv.FormatUint(uint64(orderB.ID), 10) + `,"language":"en"}`)
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+bizAID+"/print/jobs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equalf(t, http.StatusNotFound, rr.Code,
		"cross-tenant kitchen print must return 404, body: %s", rr.Body.String())

	var job database.PrintJob
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &job))
	require.Nil(t, job.PayloadHTML, "cross-tenant kitchen job must have nil payload_html")
}
