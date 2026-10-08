package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestTestPrint_LocalizedByBusinessDefaultLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", business.ID).
		Update("default_language", "es").Error)
	printer := database.Printer{
		BusinessID: business.ID, Name: "front", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	require.NoError(t, db.Create(&printer).Error)

	h := NewPrinterHandlers(db)
	router := gin.New()
	router.POST("/businesses/:id/printers/:printerId/test", h.TestPrint)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/businesses/%d/printers/%d/test", business.ID, printer.ID), nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusCreated, rr.Code, "body: %s", rr.Body.String())

	var job database.PrintJob
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &job))
	require.Equal(t, "es", job.Language)
	require.NotNil(t, job.PayloadHTML)
	require.Contains(t, *job.PayloadHTML, "Prueba de impresora exitosa.")
	require.NotContains(t, *job.PayloadHTML, "Printer test successful.")
}

func TestTestPrint_DefaultsToEnglish(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, business := setupPrinterHandlerTestDB(t)
	printer := database.Printer{
		BusinessID: business.ID, Name: "front", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	require.NoError(t, db.Create(&printer).Error)

	h := NewPrinterHandlers(db)
	router := gin.New()
	router.POST("/businesses/:id/printers/:printerId/test", h.TestPrint)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/businesses/%d/printers/%d/test", business.ID, printer.ID), nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)

	var job database.PrintJob
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &job))
	require.Equal(t, "en", job.Language)
	require.NotNil(t, job.PayloadHTML)
	require.Contains(t, *job.PayloadHTML, "Printer test successful.")
}
