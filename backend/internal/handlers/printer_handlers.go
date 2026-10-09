package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	printsvc "github.com/stdevmac/payverge/backend/internal/services/print"
	"github.com/stdevmac/payverge/backend/internal/utils"
)

// PrinterHandlers exposes CRUD endpoints for printers scoped to a business.
type PrinterHandlers struct {
	db *gorm.DB
}

func NewPrinterHandlers(db *gorm.DB) *PrinterHandlers {
	return &PrinterHandlers{db: db}
}

type createPrinterRequest struct {
	Name              string `json:"name"`
	Role              string `json:"role"`
	Transport         string `json:"transport"`
	LocationID        *uint  `json:"location_id"`
	PaperWidthMM      int    `json:"paper_width_mm"`
	CodePage          string `json:"code_page"`
	FallbackPrinterID *uint  `json:"fallback_printer_id"`
	// Enabled is optional on PATCH so operators can re-enable a soft-disabled
	// printer from the Edit modal without delete+recreate. Omitted on create.
	Enabled *bool `json:"enabled"`
}

func (h *PrinterHandlers) CreatePrinter(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	var req createPrinterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if req.Name == "" || req.Role == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and role are required"})
		return
	}
	if req.Transport == "" {
		req.Transport = "browser"
	}
	if req.Transport != "browser" {
		// cloudprnt is half-built (no poll endpoint, no ESC/POS render), so a
		// cloudprnt printer would black-hole every job at status=routed. Reject
		// it until Sprint 2 lands the transport rather than silently dropping
		// kitchen tickets.
		c.JSON(http.StatusBadRequest, gin.H{"error": "transport \"cloudprnt\" is not yet available; use browser printing"})
		return
	}
	if req.PaperWidthMM == 0 {
		req.PaperWidthMM = 80
	}
	if req.PaperWidthMM != 80 && req.PaperWidthMM != 58 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paper_width_mm must be 80 or 58"})
		return
	}
	if req.CodePage == "" {
		req.CodePage = "CP858"
	}
	p := database.Printer{
		BusinessID:        businessID,
		LocationID:        req.LocationID,
		Name:              req.Name,
		Role:              req.Role,
		Transport:         req.Transport,
		PaperWidthMM:      req.PaperWidthMM,
		CodePage:          req.CodePage,
		Enabled:           true,
		FallbackPrinterID: req.FallbackPrinterID,
	}
	if err := h.db.Create(&p).Error; err != nil {
		// FIND-060: never surface GORM/driver text to operators.
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not create printer")
		return
	}
	c.JSON(http.StatusCreated, p)
}

func (h *PrinterHandlers) ListPrinters(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	// Include disabled printers so the Edit modal can re-enable them.
	// Soft-delete (DELETE) still sets enabled=false; they stay inspectable here.
	var items []database.Printer
	if err := h.db.
		Where("business_id = ?", businessID).
		Order("enabled DESC, id ASC").
		Find(&items).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not list printers")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *PrinterHandlers) UpdatePrinter(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	id, err := strconv.ParseUint(c.Param("printerId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid printer id"})
		return
	}
	var p database.Printer
	if err := h.db.Where("id = ? AND business_id = ?", id, businessID).First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "printer not found"})
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not load printer")
		return
	}
	var req createPrinterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if req.Name != "" {
		p.Name = req.Name
	}
	if req.Role != "" {
		p.Role = req.Role
	}
	if req.PaperWidthMM == 80 || req.PaperWidthMM == 58 {
		p.PaperWidthMM = req.PaperWidthMM
	}
	if req.CodePage != "" {
		p.CodePage = req.CodePage
	}
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	if err := h.db.Save(&p).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not update printer")
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *PrinterHandlers) DeletePrinter(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	id, err := strconv.ParseUint(c.Param("printerId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid printer id"})
		return
	}
	res := h.db.Model(&database.Printer{}).
		Where("id = ? AND business_id = ?", id, businessID).
		Update("enabled", false)
	if res.Error != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not disable printer")
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "printer not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func businessIDFromCtx(c *gin.Context) (uint, bool) {
	if rawBusiness, exists := c.Get("_business"); exists {
		switch business := rawBusiness.(type) {
		case *database.Business:
			return business.ID, business.ID != 0
		case database.Business:
			return business.ID, business.ID != 0
		}
	}

	raw := c.Param("id")
	n, err := strconv.ParseUint(raw, 10, 64)
	if err == nil && n > 0 {
		return uint(n), true
	}

	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil || business == nil || business.ID == 0 {
		return 0, false
	}
	return business.ID, true
}

// testPrintCopy is the localized copy on the printer test page. The locale
// tier matches the print label bundles (en / es / es-AR); anything else
// collapses to English upstream in BusinessPrintLanguage.
type testPrintCopy struct {
	Body string
	Hint string
}

var testPrintCopyByLang = map[string]testPrintCopy{
	"en": {
		Body: "Printer test successful.",
		Hint: "If you can read this, the print path works.",
	},
	"es": {
		Body: "Prueba de impresora exitosa.",
		Hint: "Si puedes leer esto, la ruta de impresión funciona.",
	},
	"es-AR": {
		Body: "Prueba de impresora exitosa.",
		Hint: "Si podés leer esto, la ruta de impresión funciona.",
	},
}

const testPrintHTMLTemplate = `<!doctype html>
<html lang="%s"><head><meta charset="utf-8">
<style>
  @page { size: %dmm 200mm; margin: 0; }
  html, body { width: %dmm; margin: 0; }
  body { box-sizing: border-box; font-family: ui-monospace, monospace; font-size: 14px; text-align: center; padding: 6mm 4mm; }
</style></head><body>
<h1>Payverge</h1>
<p>%s</p>
<p style="font-size:10px;color:#444">%s</p>
</body></html>`

// renderTestPrintHTML renders the localized printer test page. lang must be a
// canonical label-bundle tag; unknown tags fall back to English.
func renderTestPrintHTML(lang string, paperWidthMM int) string {
	c, ok := testPrintCopyByLang[lang]
	if !ok {
		lang, c = "en", testPrintCopyByLang["en"]
	}
	if paperWidthMM != 58 {
		paperWidthMM = 80
	}
	return fmt.Sprintf(testPrintHTMLTemplate, lang, paperWidthMM, paperWidthMM, c.Body, c.Hint)
}

// TestPrint enqueues a simple "Hello, printer!" job to verify the integration.
// Browser transport: payload_html is dropped in directly. CloudPRNT: Sprint 2
// will replace this with an ESC/POS render.
func (h *PrinterHandlers) TestPrint(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	id, err := strconv.ParseUint(c.Param("printerId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid printer id"})
		return
	}
	var printer database.Printer
	if err := h.db.Where("id = ? AND business_id = ? AND enabled = ?", id, businessID, true).First(&printer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "printer not found"})
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not load printer")
		return
	}
	lang := printsvc.BusinessPrintLanguage(h.db, businessID)
	html := renderTestPrintHTML(lang, printer.PaperWidthMM)
	job := database.PrintJob{
		BusinessID: businessID, LocationID: printer.LocationID,
		PrinterID: &printer.ID, Kind: database.PrintJobKindBill,
		SourceType: "test", SourceID: printer.ID,
		Status: database.PrintJobStatusRouted, PayloadHTML: &html,
		Language: lang, CreatedBy: actorFromCtx(c),
	}
	if printer.Transport == "browser" {
		// The settings page presents this payload immediately. Persisting it as a
		// routed row lets the background agent claim the same test while the print
		// dialog is open, producing duplicate paper.
		job.Status = database.PrintJobStatusPrinted
		c.JSON(http.StatusCreated, job)
		return
	}
	if err := h.db.Create(&job).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not create print job")
		return
	}
	c.JSON(http.StatusCreated, job)
}

func actorFromCtx(c *gin.Context) string {
	if addr, ok := c.Get("address"); ok {
		if s, ok := addr.(string); ok && s != "" {
			return s
		}
	}
	if uid, ok := c.Get("user_id"); ok {
		return "user:" + strconv.FormatUint(uintFromCtx(uid), 10)
	}
	return "system"
}

func uintFromCtx(v interface{}) uint64 {
	switch x := v.(type) {
	case float64:
		return uint64(x)
	case uint:
		return uint64(x)
	case int:
		return uint64(x)
	}
	return 0
}
