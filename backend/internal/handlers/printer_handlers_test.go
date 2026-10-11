package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

var printerTestBusinessSeq int64

// Takes testing.TB so BenchmarkListPrintJobs can share the harness.
func setupPrinterHandlerTestDB(t testing.TB) (*gorm.DB, *database.Business) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_busy_timeout=5000"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&database.Business{}, &database.Printer{}, &database.PrintJob{}))
	database.SetTestDB(db)
	// The in-memory DB is process-shared, and a benchmark calls this once per
	// -count run, so the slug needs more than t.Name() to stay unique.
	business := database.Business{
		BusinessId: "biz-" + t.Name() + "-" +
			strconv.FormatInt(atomic.AddInt64(&printerTestBusinessSeq, 1), 10),
		OwnerAddress:   "0xowner",
		Name:           "Test Bistro",
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
	}
	require.NoError(t, db.Create(&business).Error)
	return db, &business
}

func TestPrinterHandlers_CreateAndList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	h := NewPrinterHandlers(db)

	router := gin.New()
	router.POST("/businesses/:id/printers", h.CreatePrinter)
	router.GET("/businesses/:id/printers", h.ListPrinters)

	body, _ := json.Marshal(map[string]interface{}{
		"name":           "Front",
		"role":           "bill",
		"transport":      "browser",
		"paper_width_mm": 80,
	})
	bizID := strconv.FormatUint(uint64(business.ID), 10)
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+bizID+"/printers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusCreated, rr.Code, "body: %s", rr.Body.String())

	listReq := httptest.NewRequest(http.MethodGet,
		"/businesses/"+bizID+"/printers", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	require.Equal(t, http.StatusOK, listRR.Code)

	var out struct {
		Items []database.Printer `json:"items"`
	}
	require.NoError(t, json.Unmarshal(listRR.Body.Bytes(), &out))
	require.Len(t, out.Items, 1)
	require.Equal(t, "Front", out.Items[0].Name)
}

func TestPrinterHandlers_ListPrintersAcceptsBusinessSlug(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	printer := database.Printer{
		BusinessID: business.ID, Name: "Slug Front", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	require.NoError(t, db.Create(&printer).Error)
	h := NewPrinterHandlers(db)

	router := gin.New()
	router.GET("/businesses/:id/printers", h.ListPrinters)

	req := httptest.NewRequest(http.MethodGet,
		"/businesses/"+business.BusinessId+"/printers", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var out struct {
		Items []database.Printer `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	require.Len(t, out.Items, 1)
	require.Equal(t, "Slug Front", out.Items[0].Name)
}

// Stream 9: list includes disabled printers so the Edit modal can re-enable
// them (previously soft-delete hid them forever without a re-create).
func TestPrinterHandlers_ListPrintersIncludesDisabledPrinters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	enabledPrinter := database.Printer{
		BusinessID: business.ID, Name: "Active front", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	disabledPrinter := database.Printer{
		BusinessID: business.ID, Name: "Removed front", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	require.NoError(t, db.Create(&enabledPrinter).Error)
	require.NoError(t, db.Create(&disabledPrinter).Error)
	require.NoError(t, db.Model(&database.Printer{}).
		Where("id = ?", disabledPrinter.ID).
		Update("enabled", false).Error)
	h := NewPrinterHandlers(db)

	router := gin.New()
	router.GET("/businesses/:id/printers", h.ListPrinters)

	bizID := strconv.FormatUint(uint64(business.ID), 10)
	req := httptest.NewRequest(http.MethodGet,
		"/businesses/"+bizID+"/printers", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var out struct {
		Items []database.Printer `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	require.Len(t, out.Items, 2)
	// Enabled first (ORDER BY enabled DESC).
	require.Equal(t, "Active front", out.Items[0].Name)
	require.True(t, out.Items[0].Enabled)
	require.Equal(t, "Removed front", out.Items[1].Name)
	require.False(t, out.Items[1].Enabled)
}

func TestPrinterHandlers_UpdatePrinterNamePaperWidthEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	printer := database.Printer{
		BusinessID: business.ID, Name: "Front", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	require.NoError(t, db.Create(&printer).Error)
	h := NewPrinterHandlers(db)

	router := gin.New()
	router.PATCH("/businesses/:id/printers/:printerId", h.UpdatePrinter)

	enabled := false
	body, _ := json.Marshal(map[string]interface{}{
		"name":           "Front bar",
		"paper_width_mm": 58,
		"enabled":        enabled,
	})
	bizID := strconv.FormatUint(uint64(business.ID), 10)
	printerID := strconv.FormatUint(uint64(printer.ID), 10)
	req := httptest.NewRequest(http.MethodPatch,
		"/businesses/"+bizID+"/printers/"+printerID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var updated database.Printer
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &updated))
	require.Equal(t, "Front bar", updated.Name)
	require.Equal(t, 58, updated.PaperWidthMM)
	require.False(t, updated.Enabled)

	// Re-enable via explicit true.
	bodyOn, _ := json.Marshal(map[string]interface{}{"enabled": true})
	reqOn := httptest.NewRequest(http.MethodPatch,
		"/businesses/"+bizID+"/printers/"+printerID, bytes.NewReader(bodyOn))
	reqOn.Header.Set("Content-Type", "application/json")
	rrOn := httptest.NewRecorder()
	router.ServeHTTP(rrOn, reqOn)
	require.Equal(t, http.StatusOK, rrOn.Code)
	var reenabled database.Printer
	require.NoError(t, json.Unmarshal(rrOn.Body.Bytes(), &reenabled))
	require.True(t, reenabled.Enabled)
}

func TestPrinterHandlers_RejectsInvalidTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	h := NewPrinterHandlers(db)

	router := gin.New()
	router.POST("/businesses/:id/printers", h.CreatePrinter)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "Front",
		"role":      "bill",
		"transport": "smoke-signals",
	})
	bizID := strconv.FormatUint(uint64(business.ID), 10)
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+bizID+"/printers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusBadRequest, rr.Code, "body: %s", rr.Body.String())
}

func TestPrinterHandlers_RejectsCloudPRNTUntilSprint2(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	h := NewPrinterHandlers(db)

	router := gin.New()
	router.POST("/businesses/:id/printers", h.CreatePrinter)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "Kitchen",
		"role":      "kitchen",
		"transport": "cloudprnt",
	})
	bizID := strconv.FormatUint(uint64(business.ID), 10)
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+bizID+"/printers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusBadRequest, rr.Code, "cloudprnt must be rejected until the poll/ESCPOS path ships; body: %s", rr.Body.String())
	require.Contains(t, rr.Body.String(), "not yet available")

	// And it must NOT persist a printer row.
	var count int64
	db.Model(&database.Printer{}).Where("business_id = ?", business.ID).Count(&count)
	require.Zero(t, count, "no cloudprnt printer should be created")
}

func TestPrinterHandlers_TestPrintReturnsDirectBrowserPayloadWithoutQueueing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	printer := database.Printer{
		BusinessID: business.ID, Name: "p", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	require.NoError(t, db.Create(&printer).Error)
	h := NewPrinterHandlers(db)

	router := gin.New()
	router.POST("/businesses/:id/printers/:printerId/test", h.TestPrint)

	bizID := strconv.FormatUint(uint64(business.ID), 10)
	printerID := strconv.FormatUint(uint64(printer.ID), 10)
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+bizID+"/printers/"+printerID+"/test", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusCreated, rr.Code, "body: %s", rr.Body.String())

	var job database.PrintJob
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &job))
	require.Zero(t, job.ID, "direct browser test prints must not enter the claim queue")
	require.NotNil(t, job.PayloadHTML)
	require.Contains(t, *job.PayloadHTML, "width: 80mm")
	require.NotContains(t, *job.PayloadHTML, "80mm auto")

	var count int64
	require.NoError(t, db.Model(&database.PrintJob{}).Where("printer_id = ?", printer.ID).Count(&count).Error)
	require.Zero(t, count)
}

func TestPrinterHandlers_TestPrintUsesConfigured58mmRoll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	printer := database.Printer{
		BusinessID: business.ID, Name: "narrow", Role: "bill",
		Transport: "browser", PaperWidthMM: 58, Enabled: true,
	}
	require.NoError(t, db.Create(&printer).Error)
	h := NewPrinterHandlers(db)
	router := gin.New()
	router.POST("/businesses/:id/printers/:printerId/test", h.TestPrint)
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+strconv.FormatUint(uint64(business.ID), 10)+"/printers/"+strconv.FormatUint(uint64(printer.ID), 10)+"/test", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)
	require.Contains(t, rr.Body.String(), "width: 58mm")
}

func TestPrinterHandlers_TestPrintRejectsDisabledPrinter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	printer := database.Printer{
		BusinessID: business.ID, Name: "removed", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	require.NoError(t, db.Create(&printer).Error)
	require.NoError(t, db.Model(&database.Printer{}).
		Where("id = ?", printer.ID).
		Update("enabled", false).Error)
	h := NewPrinterHandlers(db)

	router := gin.New()
	router.POST("/businesses/:id/printers/:printerId/test", h.TestPrint)

	bizID := strconv.FormatUint(uint64(business.ID), 10)
	printerID := strconv.FormatUint(uint64(printer.ID), 10)
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+bizID+"/printers/"+printerID+"/test", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusNotFound, rr.Code, "body: %s", rr.Body.String())

	var count int64
	db.Model(&database.PrintJob{}).Where("printer_id = ?", printer.ID).Count(&count)
	require.Zero(t, count)
}

// TestPrinterHandlers_NoRawErrErrorInResponses is a FIND-060 source gate:
// 500 responses must use server.RespondWithError product copy, never
// gin.H{"error": err.Error()} which can leak GORM/driver internals.
func TestPrinterHandlers_NoRawErrErrorInResponses(t *testing.T) {
	src, err := os.ReadFile("printer_handlers.go")
	require.NoError(t, err)
	text := string(src)
	require.NotContains(t, text, `gin.H{"error": err.Error()}`,
		"printer handlers must not return raw err.Error() to clients (FIND-060)")
	require.NotContains(t, text, `gin.H{"error": res.Error.Error()}`,
		"printer handlers must not return raw res.Error.Error() to clients (FIND-060)")
}
