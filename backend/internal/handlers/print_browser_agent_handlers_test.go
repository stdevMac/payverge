package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestPrintJobHandlers_ClaimBrowserJob_CrossTenantDenial(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, bizA := setupPrinterHandlerTestDB(t)
	bizB := database.Business{
		BusinessId: "bizB-claim-" + t.Name(), OwnerAddress: "0xb",
		Name: "B", SettlementAddr: "0xs", TippingAddr: "0xt",
	}
	require.NoError(t, db.Create(&bizB).Error)

	printerA := database.Printer{
		BusinessID: bizA.ID, Name: "front", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	require.NoError(t, db.Create(&printerA).Error)
	html := "<html>secret-a</html>"
	jobA := database.PrintJob{
		BusinessID: bizA.ID, PrinterID: &printerA.ID, Kind: database.PrintJobKindBill,
		SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusRouted,
		PayloadHTML: &html, MaxAttempts: 6,
	}
	require.NoError(t, db.Create(&jobA).Error)

	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.POST("/businesses/:id/print/jobs/claim", h.ClaimBrowserJob)

	clientID := uuid.NewString()
	body, _ := json.Marshal(map[string]interface{}{"client_id": clientID, "printer_id": printerA.ID})
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+strconv.FormatUint(uint64(bizB.ID), 10)+"/print/jobs/claim",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestPrintJobHandlers_ClaimBrowserJob_ReturnsPostClaimPendingCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, biz := setupPrinterHandlerTestDB(t)
	printer := database.Printer{
		BusinessID: biz.ID, Name: "front", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	require.NoError(t, db.Create(&printer).Error)
	html := "<html>x</html>"
	for sourceID := uint(1); sourceID <= 2; sourceID++ {
		require.NoError(t, db.Create(&database.PrintJob{
			BusinessID: biz.ID, PrinterID: &printer.ID, Kind: database.PrintJobKindBill,
			SourceType: "bill", SourceID: sourceID, Status: database.PrintJobStatusRouted,
			PayloadHTML: &html, MaxAttempts: 6,
		}).Error)
	}

	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.POST("/businesses/:id/print/jobs/claim", h.ClaimBrowserJob)
	clientID := uuid.NewString()
	path := "/businesses/" + strconv.FormatUint(uint64(biz.ID), 10) + "/print/jobs/claim"

	claim := func() struct {
		Job          *database.PrintJob `json:"job"`
		PendingCount int64              `json:"pending_count"`
	} {
		body, _ := json.Marshal(map[string]interface{}{"client_id": clientID, "printer_id": printer.ID})
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var response struct {
			Job          *database.PrintJob `json:"job"`
			PendingCount int64              `json:"pending_count"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
		return response
	}

	first := claim()
	require.NotNil(t, first.Job)
	require.EqualValues(t, 1, first.PendingCount)
	second := claim()
	require.NotNil(t, second.Job)
	require.Zero(t, second.PendingCount)
	empty := claim()
	require.Nil(t, empty.Job)
	require.Zero(t, empty.PendingCount)
}

func TestPrintJobHandlers_ConfirmIllegalTransition409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, biz := setupPrinterHandlerTestDB(t)
	printer := database.Printer{
		BusinessID: biz.ID, Name: "front", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	require.NoError(t, db.Create(&printer).Error)
	html := "<html>x</html>"
	job := database.PrintJob{
		BusinessID: biz.ID, PrinterID: &printer.ID, Kind: database.PrintJobKindBill,
		SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusRouted,
		PayloadHTML: &html, MaxAttempts: 6,
	}
	require.NoError(t, db.Create(&job).Error)

	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.POST("/businesses/:id/print/jobs/claim", h.ClaimBrowserJob)
	router.POST("/businesses/:id/print/jobs/:jobId/confirm", h.ConfirmBrowserPrinted)
	router.POST("/businesses/:id/print/jobs/:jobId/presented", h.MarkBrowserPresented)

	clientID := uuid.NewString()
	claimBody, _ := json.Marshal(map[string]interface{}{"client_id": clientID, "printer_id": printer.ID})
	bizID := strconv.FormatUint(uint64(biz.ID), 10)

	// Claim
	req := httptest.NewRequest(http.MethodPost, "/businesses/"+bizID+"/print/jobs/claim", bytes.NewReader(claimBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Present then confirm, then confirm again → 409
	jobID := strconv.FormatUint(uint64(job.ID), 10)
	for _, path := range []string{"presented", "confirm"} {
		b, _ := json.Marshal(map[string]string{"client_id": clientID})
		r := httptest.NewRequest(http.MethodPost, "/businesses/"+bizID+"/print/jobs/"+jobID+"/"+path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		require.Equalf(t, http.StatusOK, w.Code, "path=%s body=%s", path, w.Body.String())
	}
	// Duplicate confirm
	b, _ := json.Marshal(map[string]string{"client_id": clientID})
	r := httptest.NewRequest(http.MethodPost, "/businesses/"+bizID+"/print/jobs/"+jobID+"/confirm", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	require.Equal(t, http.StatusConflict, w.Code)

	// Wrong client ID on renew of already-printed job → 409
	other := uuid.NewString()
	// Re-seed a leased job to test ownership
	job2 := database.PrintJob{
		BusinessID: biz.ID, PrinterID: &printer.ID, Kind: database.PrintJobKindBill,
		SourceType: "bill", SourceID: 2, Status: database.PrintJobStatusPrinting,
		PayloadHTML: &html, MaxAttempts: 6,
	}
	owner := uuid.NewString()
	job2.ClaimedBy = &owner
	exp := time.Now().Add(time.Minute)
	job2.LeaseExpiresAt = &exp
	require.NoError(t, db.Create(&job2).Error)

	router.POST("/businesses/:id/print/jobs/:jobId/renew", h.RenewBrowserLease)
	b, _ = json.Marshal(map[string]string{"client_id": other})
	r = httptest.NewRequest(http.MethodPost,
		"/businesses/"+bizID+"/print/jobs/"+strconv.FormatUint(uint64(job2.ID), 10)+"/renew",
		bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, r)
	require.Equal(t, http.StatusConflict, w.Code)
}

func TestPrintJobHandlers_ClaimRequiresUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, biz := setupPrinterHandlerTestDB(t)
	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.POST("/businesses/:id/print/jobs/claim", h.ClaimBrowserJob)

	body := []byte(`{"client_id":"not-a-uuid"}`)
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+strconv.FormatUint(uint64(biz.ID), 10)+"/print/jobs/claim",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPrintJobHandlers_ClaimRequiresPrinterID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, biz := setupPrinterHandlerTestDB(t)
	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.POST("/businesses/:id/print/jobs/claim", h.ClaimBrowserJob)

	body, _ := json.Marshal(map[string]string{"client_id": uuid.NewString()})
	req := httptest.NewRequest(http.MethodPost,
		"/businesses/"+strconv.FormatUint(uint64(biz.ID), 10)+"/print/jobs/claim",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPrintJobHandlers_ListJobsFiltersSelectedPrinter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, biz := setupPrinterHandlerTestDB(t)
	front := database.Printer{BusinessID: biz.ID, Name: "front", Role: "bill", Transport: "browser", PaperWidthMM: 80, Enabled: true}
	kitchen := database.Printer{BusinessID: biz.ID, Name: "kitchen", Role: "kitchen", Transport: "browser", PaperWidthMM: 80, Enabled: true}
	require.NoError(t, db.Create(&front).Error)
	require.NoError(t, db.Create(&kitchen).Error)
	html := "<html>x</html>"
	frontJob := database.PrintJob{BusinessID: biz.ID, PrinterID: &front.ID, Kind: database.PrintJobKindBill, SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusRouted, PayloadHTML: &html, MaxAttempts: 6}
	kitchenJob := database.PrintJob{BusinessID: biz.ID, PrinterID: &kitchen.ID, Kind: database.PrintJobKindKitchen, SourceType: "order", SourceID: 2, Status: database.PrintJobStatusRouted, PayloadHTML: &html, MaxAttempts: 6}
	require.NoError(t, db.Create(&frontJob).Error)
	require.NoError(t, db.Create(&kitchenJob).Error)

	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.GET("/businesses/:id/print/jobs", h.ListJobs)
	path := "/businesses/" + strconv.FormatUint(uint64(biz.ID), 10) + "/print/jobs?status=routed&transport=browser&printer_id=" + strconv.FormatUint(uint64(kitchen.ID), 10)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, rr.Code)
	var response struct {
		Items []database.PrintJob `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	require.Len(t, response.Items, 1)
	require.Equal(t, kitchenJob.ID, response.Items[0].ID)
}

func TestPrintJobHandlers_BillOnlyAgentCannotClaimOrCountReceipt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, biz := setupPrinterHandlerTestDB(t)
	printer := database.Printer{BusinessID: biz.ID, Name: "front", Role: "bill", Transport: "browser", PaperWidthMM: 80, Enabled: true}
	require.NoError(t, db.Create(&printer).Error)
	receiptHTML := "<html>payment secret</html>"
	billHTML := "<html>guest bill</html>"
	receipt := database.PrintJob{BusinessID: biz.ID, PrinterID: &printer.ID, Kind: database.PrintJobKindReceipt, SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusRouted, PayloadHTML: &receiptHTML, MaxAttempts: 6}
	bill := database.PrintJob{BusinessID: biz.ID, PrinterID: &printer.ID, Kind: database.PrintJobKindBill, SourceType: "bill", SourceID: 2, Status: database.PrintJobStatusRouted, PayloadHTML: &billHTML, MaxAttempts: 6}
	require.NoError(t, db.Create(&receipt).Error)
	require.NoError(t, db.Create(&bill).Error)

	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(authorizedAnyPermissionsContextKey, []string{"print:bill"})
	})
	router.POST("/businesses/:id/print/jobs/claim", h.ClaimBrowserJob)
	router.GET("/businesses/:id/print/jobs/pending-count", h.PendingBrowserJobCount)
	bizID := strconv.FormatUint(uint64(biz.ID), 10)
	printerID := strconv.FormatUint(uint64(printer.ID), 10)

	countRR := httptest.NewRecorder()
	router.ServeHTTP(countRR, httptest.NewRequest(http.MethodGet, "/businesses/"+bizID+"/print/jobs/pending-count?printer_id="+printerID, nil))
	require.Equal(t, http.StatusOK, countRR.Code)
	require.JSONEq(t, `{"count":1}`, countRR.Body.String())

	body, _ := json.Marshal(map[string]interface{}{"client_id": uuid.NewString(), "printer_id": printer.ID})
	claimReq := httptest.NewRequest(http.MethodPost, "/businesses/"+bizID+"/print/jobs/claim", bytes.NewReader(body))
	claimReq.Header.Set("Content-Type", "application/json")
	claimRR := httptest.NewRecorder()
	router.ServeHTTP(claimRR, claimReq)
	require.Equal(t, http.StatusOK, claimRR.Code)
	var claimed struct {
		Job *database.PrintJob `json:"job"`
	}
	require.NoError(t, json.Unmarshal(claimRR.Body.Bytes(), &claimed))
	require.NotNil(t, claimed.Job)
	require.Equal(t, bill.ID, claimed.Job.ID)
	require.NotContains(t, claimRR.Body.String(), "payment secret")

	var untouched database.PrintJob
	require.NoError(t, db.First(&untouched, receipt.ID).Error)
	require.Equal(t, database.PrintJobStatusRouted, untouched.Status)
}

func TestPrintJobHandlers_ListBrowserStationsReturnsOnlyBindingFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, biz := setupPrinterHandlerTestDB(t)
	printer := database.Printer{BusinessID: biz.ID, Name: "Front desk", Role: "bill", Transport: "browser", PaperWidthMM: 58, Enabled: true, CodePage: "secret-config"}
	require.NoError(t, db.Create(&printer).Error)
	h := NewPrintJobHandlers(db)
	router := gin.New()
	router.GET("/businesses/:id/print/stations", h.ListBrowserStations)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/businesses/"+strconv.FormatUint(uint64(biz.ID), 10)+"/print/stations", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "Front desk")
	require.Contains(t, rr.Body.String(), `"paper_width_mm":58`)
	require.NotContains(t, rr.Body.String(), "secret-config")
	require.NotContains(t, rr.Body.String(), "business_id")
}
