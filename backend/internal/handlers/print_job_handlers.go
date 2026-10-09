package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services/print"
)

// emptyClaimThrottle rate-limits empty browser claim polls per (business, client).
var emptyClaimThrottle = struct {
	mu   sync.Mutex
	last map[string]time.Time
}{last: make(map[string]time.Time)}

const emptyClaimMinInterval = time.Second
const authorizedAnyPermissionsContextKey = "authorized_any_permissions"

var allBrowserPrintKinds = []database.PrintJobKind{
	database.PrintJobKindBill,
	database.PrintJobKindReceipt,
	database.PrintJobKindKitchen,
	database.PrintJobKindBar,
	database.PrintJobKindVoid,
	database.PrintJobKindModify,
}

// browserClientBodies caches the last-bound body for a request context key.
// Gin cannot re-read the JSON body, so withLeaseMutation stores the parse.

// PrintJobHandlers exposes endpoints for inspecting and advancing print jobs
// scoped to a business.
type PrintJobHandlers struct {
	db  *gorm.DB
	svc *print.Service
}

// NewPrintJobHandlers creates a PrintJobHandlers backed by db.
func NewPrintJobHandlers(db *gorm.DB) *PrintJobHandlers {
	return &PrintJobHandlers{db: db, svc: print.NewService(db)}
}

func authorizedPrintKinds(c *gin.Context) []database.PrintJobKind {
	value, exists := c.Get(authorizedAnyPermissionsContextKey)
	if !exists {
		// Handler unit tests historically mount methods without the production
		// RBAC chain. Production fails closed if route wiring ever omits it.
		if gin.Mode() == gin.TestMode {
			return append([]database.PrintJobKind(nil), allBrowserPrintKinds...)
		}
		return nil
	}
	permissions, ok := value.([]string)
	if !ok {
		return nil
	}
	seen := make(map[database.PrintJobKind]struct{})
	add := func(kinds ...database.PrintJobKind) {
		for _, kind := range kinds {
			seen[kind] = struct{}{}
		}
	}
	for _, permission := range permissions {
		switch permission {
		case "print:bill":
			add(database.PrintJobKindBill)
		case "print:receipt":
			add(database.PrintJobKindReceipt)
		case "orders:kitchen":
			add(database.PrintJobKindKitchen, database.PrintJobKindBar, database.PrintJobKindVoid, database.PrintJobKindModify)
		}
	}
	out := make([]database.PrintJobKind, 0, len(seen))
	for _, kind := range allBrowserPrintKinds {
		if _, ok := seen[kind]; ok {
			out = append(out, kind)
		}
	}
	return out
}

func printKindAuthorized(kind database.PrintJobKind, allowed []database.PrintJobKind) bool {
	for _, candidate := range allowed {
		if candidate == kind {
			return true
		}
	}
	return false
}

func requireAuthorizedPrintKinds(c *gin.Context) ([]database.PrintJobKind, bool) {
	kinds := authorizedPrintKinds(c)
	if len(kinds) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "no authorized print job kinds"})
		return nil, false
	}
	return kinds, true
}

// Service returns the underlying print service so callers (main.go, order
// handler) can share the same instance instead of constructing new ones.
func (h *PrintJobHandlers) Service() *print.Service { return h.svc }

// printJobListItem is the Recent Jobs wire shape: PrintJob metadata plus the
// bill/table/order labels operators need mid-shift (dinner QA #158).
type printJobListItem struct {
	database.PrintJob
	BillNumber string `json:"bill_number,omitempty"`
	TableName  string `json:"table_name,omitempty"`
}

// ListJobs returns the operator's queue for a business, filtered by status and/or
// transport if provided as query parameters.
func (h *PrintJobHandlers) ListJobs(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	// History UI only needs status/kind/source metadata. HTML payloads are
	// large (kitchen tickets) and only required by claim/reprint paths — omit
	// them here so Recent Print Jobs polls stay small. (FIND-041)
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit", "code": "VALIDATION_INVALID_INPUT"})
			return
		}
		if n > 100 {
			n = 100
		}
		limit = n
	}
	offset := 0
	if raw := c.Query("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid offset", "code": "VALIDATION_INVALID_INPUT"})
			return
		}
		offset = n
	}
	// Qualify every print_jobs column with its table prefix. The optional
	// LEFT JOIN onto printers (below) introduces a table that also has
	// business_id, status, and created_at columns, so unqualified references
	// are ambiguous and error on every poll (SQLSTATE 42702 / sqlite
	// "ambiguous column name").
	base := h.db.Model(&database.PrintJob{}).Where("print_jobs.business_id = ?", businessID)
	if status := c.Query("status"); status != "" {
		if status == "unassigned" {
			base = base.Where("print_jobs.printer_id IS NULL").
				Where("print_jobs.status IN ?", []database.PrintJobStatus{
					database.PrintJobStatusPending,
					database.PrintJobStatusFailedRetryable,
				})
		} else {
			base = base.Where("print_jobs.status = ?", status)
		}
	}
	if kind := c.Query("kind"); kind != "" {
		base = base.Where("print_jobs.kind = ?", kind)
	}
	if transport := c.Query("transport"); transport != "" {
		base = base.Joins("LEFT JOIN printers ON printers.id = print_jobs.printer_id").
			Where("printers.transport = ?", transport)
	}
	if printerParam := c.Query("printer_id"); printerParam != "" {
		printerID, err := strconv.ParseUint(printerParam, 10, 64)
		if err != nil || printerID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid printer_id"})
			return
		}
		base = base.Where("print_jobs.printer_id = ?", uint(printerID))
	}
	var total int64
	// printers join is 1:1 on printer_id, so a plain Count does not inflate.
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		log.Printf("ListJobs: failed to count print jobs for business %d: %v", businessID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list print jobs"})
		return
	}

	var jobs []database.PrintJob
	if err := base.Session(&gorm.Session{}).
		// L3-35: deterministic order — id breaks created_at ties (index-friendly).
		Order("print_jobs.created_at DESC, print_jobs.id DESC").
		Limit(limit).
		Offset(offset).
		Omit("payload_html", "payload_escpos").
		Find(&jobs).Error; err != nil {
		log.Printf("ListJobs: failed to list print jobs for business %d: %v", businessID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list print jobs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items": enrichPrintJobsWithSourceLabels(h.db, businessID, jobs),
		"total": total,
	})
}

// enrichPrintJobsWithSourceLabels attaches bill_number / table_name for bill-
// sourced jobs and table_name for order-sourced kitchen tickets so Recent Jobs
// never reads as a bare "receipt" label.
func enrichPrintJobsWithSourceLabels(db *gorm.DB, businessID uint, jobs []database.PrintJob) []printJobListItem {
	out := make([]printJobListItem, len(jobs))
	billIDs := make([]uint, 0, len(jobs))
	orderIDs := make([]uint, 0, len(jobs))
	for i, job := range jobs {
		out[i] = printJobListItem{PrintJob: job}
		switch job.SourceType {
		case "bill", "reprint":
			if job.SourceID != 0 {
				billIDs = append(billIDs, job.SourceID)
			}
		case "order":
			if job.SourceID != 0 {
				orderIDs = append(orderIDs, job.SourceID)
			}
		}
		if job.OrderID != nil && *job.OrderID != 0 {
			orderIDs = append(orderIDs, *job.OrderID)
		}
	}
	type billRow struct {
		ID         uint
		BillNumber string
		TableName  string
	}
	type orderRow struct {
		ID         uint
		TableName  string
		BillID     uint
		BillNumber string
	}
	orderByID := map[uint]orderRow{}
	if len(orderIDs) > 0 {
		var rows []orderRow
		_ = db.Table("orders").
			Select("orders.id, orders.bill_id, COALESCE(bills.bill_number, '') AS bill_number, COALESCE(tables.name, '') AS table_name").
			Joins("LEFT JOIN bills ON bills.id = orders.bill_id").
			Joins("LEFT JOIN tables ON tables.id = bills.table_id").
			Where("orders.business_id = ? AND orders.id IN ?", businessID, orderIDs).
			Scan(&rows).Error
		for _, row := range rows {
			orderByID[row.ID] = row
			if row.BillID != 0 {
				billIDs = append(billIDs, row.BillID)
			}
		}
	}
	billByID := map[uint]billRow{}
	if len(billIDs) > 0 {
		var rows []billRow
		_ = db.Table("bills").
			Select("bills.id, bills.bill_number, COALESCE(tables.name, '') AS table_name").
			Joins("LEFT JOIN tables ON tables.id = bills.table_id").
			Where("bills.business_id = ? AND bills.id IN ?", businessID, billIDs).
			Scan(&rows).Error
		for _, row := range rows {
			billByID[row.ID] = row
		}
	}
	for i := range out {
		job := out[i].PrintJob
		switch job.SourceType {
		case "bill", "reprint":
			if row, ok := billByID[job.SourceID]; ok {
				out[i].BillNumber = row.BillNumber
				out[i].TableName = row.TableName
			}
		case "order":
			if row, ok := orderByID[job.SourceID]; ok {
				out[i].TableName = row.TableName
				out[i].BillNumber = row.BillNumber
			}
		}
		if out[i].TableName == "" && job.OrderID != nil {
			if row, ok := orderByID[*job.OrderID]; ok {
				out[i].TableName = row.TableName
				if out[i].BillNumber == "" {
					out[i].BillNumber = row.BillNumber
				}
			}
		}
	}
	return out
}

// ListBrowserStations exposes only the non-sensitive fields needed to bind a
// workstation. Print-only floor roles do not need access to printer settings or
// historical payloads just to select their physical station.
func (h *PrintJobHandlers) ListBrowserStations(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	type station struct {
		ID           uint   `json:"id"`
		Name         string `json:"name"`
		Role         string `json:"role"`
		PaperWidthMM int    `json:"paper_width_mm"`
	}
	var stations []station
	if err := h.db.Model(&database.Printer{}).
		Select("id", "name", "role", "paper_width_mm").
		Where("business_id = ? AND transport = ? AND enabled = ?", businessID, "browser", true).
		Order("name ASC, id ASC").
		Scan(&stations).Error; err != nil {
		// FIND-060: never surface GORM/driver text to operators.
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not list browser stations")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": stations})
}

// PendingBrowserJobCount returns queue depth only, scoped to the selected
// station and the caller's authorized job kinds. It never exposes payload HTML.
func (h *PrintJobHandlers) PendingBrowserJobCount(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	kinds, ok := requireAuthorizedPrintKinds(c)
	if !ok {
		return
	}
	printerID, err := strconv.ParseUint(c.Query("printer_id"), 10, 64)
	if err != nil || printerID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid printer_id"})
		return
	}
	var printerCount int64
	if err := h.db.Model(&database.Printer{}).
		Where("id = ? AND business_id = ? AND transport = ? AND enabled = ?", uint(printerID), businessID, "browser", true).
		Count(&printerCount).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not verify printer")
		return
	}
	if printerCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "browser printer not found"})
		return
	}
	count, err := h.pendingBrowserJobCount(businessID, uint(printerID), kinds)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not count print jobs")
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (h *PrintJobHandlers) pendingBrowserJobCount(businessID, printerID uint, kinds []database.PrintJobKind) (int64, error) {
	var count int64
	err := h.db.Model(&database.PrintJob{}).
		Where("business_id = ? AND printer_id = ? AND status = ? AND kind IN ?", businessID, printerID, database.PrintJobStatusRouted, kinds).
		Count(&count).Error
	return count, err
}

// MarkPrinted is the browser-transport completion callback; transitions the job
// from printing → printed.
func (h *PrintJobHandlers) MarkPrinted(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	id, err := parseJobID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	kinds, allowed := requireAuthorizedPrintKinds(c)
	if !allowed {
		return
	}
	if !authorizedJobBelongsToBusiness(h.db, id, businessID, kinds) {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	if err := h.svc.MarkPrinted(c.Request.Context(), id, actorFromCtx(c)); err != nil {
		if errors.Is(err, print.ErrIllegalTransition) {
			c.JSON(http.StatusConflict, gin.H{"error": "job is not in a printable state"})
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not mark job printed")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Cancel transitions a job to the cancelled status.
func (h *PrintJobHandlers) Cancel(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	id, err := parseJobID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !jobBelongsToBusiness(h.db, id, businessID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	if err := h.svc.Cancel(c.Request.Context(), id, actorFromCtx(c)); err != nil {
		if errors.Is(err, print.ErrIllegalTransition) {
			c.JSON(http.StatusConflict, gin.H{"error": "job is already in a terminal state"})
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not cancel print job")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Reroute re-assigns a printer to a pending/unassigned job after an operator
// adds or re-enables a station (dinner QA #141).
func (h *PrintJobHandlers) Reroute(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	id, err := parseJobID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	kinds, allowed := requireAuthorizedPrintKinds(c)
	if !allowed {
		return
	}
	if !authorizedJobBelongsToBusiness(h.db, id, businessID, kinds) {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	job, err := h.svc.Reroute(c.Request.Context(), id, actorFromCtx(c))
	if err != nil {
		if errors.Is(err, print.ErrJobNotReroutable) {
			c.JSON(http.StatusConflict, gin.H{"error": "job is not waiting for a printer"})
			return
		}
		if errors.Is(err, print.ErrNoPrinterForRole) {
			c.JSON(http.StatusConflict, gin.H{"error": "no printer registered for role"})
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not assign printer")
		return
	}
	c.JSON(http.StatusOK, job)
}

// Reprint forbids kitchen / bar / void / modify kinds and creates a new job
// for bill and receipt kinds.
func (h *PrintJobHandlers) Reprint(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	id, err := parseJobID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	kinds, allowed := requireAuthorizedPrintKinds(c)
	if !allowed {
		return
	}
	if !authorizedJobBelongsToBusiness(h.db, id, businessID, kinds) {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	newJob, err := h.svc.Reprint(c.Request.Context(), id, actorFromCtx(c))
	if err != nil {
		if errors.Is(err, print.ErrKindNotReprintable) {
			c.JSON(http.StatusForbidden, gin.H{"error": "kind not reprintable"})
			return
		}
		if errors.Is(err, print.ErrJobNotReprintableState) || errors.Is(err, print.ErrReceiptRequiresPaidBill) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not reprint job")
		return
	}
	c.JSON(http.StatusCreated, newJob)
}

type createPrintJobRequest struct {
	Kind       database.PrintJobKind `json:"kind"`
	SourceType string                `json:"source_type"`
	SourceID   uint                  `json:"source_id"`
	LocationID *uint                 `json:"location_id"`
	Language   string                `json:"language"`
}

// Create enqueues a new print job for the given business. Used by the
// frontend "Print bill" button so the bill print flows through the same
// queue + audit + reprint path as the auto-receipt.
func (h *PrintJobHandlers) Create(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	var req createPrintJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	kinds, allowed := requireAuthorizedPrintKinds(c)
	if !allowed {
		return
	}
	if !printKindAuthorized(req.Kind, kinds) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized for this print job kind"})
		return
	}
	if !operatorPrintSourceMatchesKind(req.Kind, req.SourceType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_type does not match print job kind"})
		return
	}
	// PRINT-XTENANT-SOURCEID-1: verify the source record belongs to this
	// business before rendering/returning its data.
	if !sourceBelongsToBusiness(h.db, req.SourceType, req.SourceID, businessID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}
	job, err := h.svc.Enqueue(c.Request.Context(), print.EnqueueParams{
		BusinessID: businessID,
		LocationID: req.LocationID,
		Kind:       req.Kind,
		SourceType: req.SourceType,
		SourceID:   req.SourceID,
		Language:   req.Language,
		CreatedBy:  actorFromCtx(c),
	})
	if err != nil {
		if errors.Is(err, print.ErrReceiptRequiresPaidBill) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, job)
}

func operatorPrintSourceMatchesKind(kind database.PrintJobKind, sourceType string) bool {
	switch kind {
	case database.PrintJobKindBill, database.PrintJobKindReceipt:
		return sourceType == "bill"
	case database.PrintJobKindKitchen, database.PrintJobKindBar:
		return sourceType == "order"
	default:
		return false
	}
}

// ---- Wave 4 browser-agent lease endpoints ----

type browserClientBody struct {
	ClientID  string `json:"client_id"`
	PrinterID uint   `json:"printer_id,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

func parseBrowserClientBody(c *gin.Context) browserClientBody {
	var body browserClientBody
	_ = c.ShouldBindJSON(&body)
	if body.ClientID == "" {
		body.ClientID = c.GetHeader("X-Print-Client-Id")
	}
	return body
}

func validClientID(id string) bool {
	if id == "" {
		return false
	}
	_, err := uuid.Parse(id)
	return err == nil
}

func emptyClaimKey(businessID, printerID uint, clientID string) string {
	return strconv.FormatUint(uint64(businessID), 10) + ":" + strconv.FormatUint(uint64(printerID), 10) + ":" + clientID
}

// shouldThrottleEmpty returns true when this client polled empty too recently.
func shouldThrottleEmpty(businessID, printerID uint, clientID string) bool {
	key := emptyClaimKey(businessID, printerID, clientID)
	emptyClaimThrottle.mu.Lock()
	defer emptyClaimThrottle.mu.Unlock()
	last, ok := emptyClaimThrottle.last[key]
	return ok && time.Since(last) < emptyClaimMinInterval
}

func recordEmptyClaim(businessID, printerID uint, clientID string) {
	key := emptyClaimKey(businessID, printerID, clientID)
	emptyClaimThrottle.mu.Lock()
	defer emptyClaimThrottle.mu.Unlock()
	now := time.Now()
	emptyClaimThrottle.last[key] = now
	if len(emptyClaimThrottle.last) > 10_000 {
		for k, t := range emptyClaimThrottle.last {
			if now.Sub(t) > time.Minute {
				delete(emptyClaimThrottle.last, k)
			}
		}
	}
}

func clearEmptyClaim(businessID, printerID uint, clientID string) {
	emptyClaimThrottle.mu.Lock()
	delete(emptyClaimThrottle.last, emptyClaimKey(businessID, printerID, clientID))
	emptyClaimThrottle.mu.Unlock()
}

// ClaimBrowserJob leases the next browser-transport job for this business to
// the calling agent client. Empty queue returns {"job":null}; empty polls are
// rate-limited per client to ~1/s.
// POST /businesses/:id/print/jobs/claim
func (h *PrintJobHandlers) ClaimBrowserJob(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	body := parseBrowserClientBody(c)
	if !validClientID(body.ClientID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client_id must be a UUID"})
		return
	}
	if body.PrinterID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "printer_id is required"})
		return
	}
	clientID := body.ClientID
	kinds, allowed := requireAuthorizedPrintKinds(c)
	if !allowed {
		return
	}

	now := time.Now()
	job, err := h.svc.ClaimAuthorizedBrowserJobForPrinter(c.Request.Context(), businessID, body.PrinterID, kinds, clientID, now, print.DefaultBrowserLease)
	if err != nil {
		if errors.Is(err, print.ErrInvalidBrowserPrinter) {
			c.JSON(http.StatusNotFound, gin.H{"error": "browser printer not found"})
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not claim print job")
		return
	}
	if job == nil {
		if shouldThrottleEmpty(businessID, body.PrinterID, clientID) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "claim poll rate limited", "job": nil})
			return
		}
		recordEmptyClaim(businessID, body.PrinterID, clientID)
	} else {
		clearEmptyClaim(businessID, body.PrinterID, clientID)
		metrics.PrintBrowserClaims.Inc()
		metrics.MarkPrintBrowserClientActive(businessID)
	}
	pendingCount, err := h.pendingBrowserJobCount(businessID, body.PrinterID, kinds)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not count print jobs")
		return
	}
	c.JSON(http.StatusOK, gin.H{"job": job, "pending_count": pendingCount})
}

// RenewBrowserLease extends the lease on a claimed job.
// POST /businesses/:id/print/jobs/:jobId/renew
func (h *PrintJobHandlers) RenewBrowserLease(c *gin.Context) {
	h.withLeaseMutation(c, func(jobID uint, clientID string, now time.Time, actor string, body browserClientBody) error {
		return h.svc.RenewBrowserLease(c.Request.Context(), jobID, clientID, now, print.DefaultBrowserLease)
	})
}

// MarkBrowserPresented records that window.print() / the dialog was shown.
// Does NOT mark printed.
// POST /businesses/:id/print/jobs/:jobId/presented
func (h *PrintJobHandlers) MarkBrowserPresented(c *gin.Context) {
	h.withLeaseMutation(c, func(jobID uint, clientID string, now time.Time, actor string, body browserClientBody) error {
		if err := h.svc.MarkPresented(c.Request.Context(), jobID, clientID, now, actor); err != nil {
			return err
		}
		metrics.PrintBrowserPresented.Inc()
		return nil
	})
}

// ConfirmBrowserPrinted is the explicit operator "Printed" confirmation.
// POST /businesses/:id/print/jobs/:jobId/confirm
func (h *PrintJobHandlers) ConfirmBrowserPrinted(c *gin.Context) {
	h.withLeaseMutation(c, func(jobID uint, clientID string, now time.Time, actor string, body browserClientBody) error {
		if err := h.svc.ConfirmPrinted(c.Request.Context(), jobID, clientID, now, actor); err != nil {
			return err
		}
		metrics.PrintBrowserConfirmed.Inc()
		return nil
	})
}

// RetryBrowserJob returns a leased job to the routed queue for re-presentation.
// POST /businesses/:id/print/jobs/:jobId/retry
func (h *PrintJobHandlers) RetryBrowserJob(c *gin.Context) {
	h.withLeaseMutation(c, func(jobID uint, clientID string, now time.Time, actor string, body browserClientBody) error {
		if err := h.svc.RetryBrowserJob(c.Request.Context(), jobID, clientID, now, actor); err != nil {
			return err
		}
		metrics.PrintBrowserRetry.Inc()
		return nil
	})
}

// CancelBrowserJob permanently fails a leased browser job from the confirmation panel.
// POST /businesses/:id/print/jobs/:jobId/agent-cancel
func (h *PrintJobHandlers) CancelBrowserJob(c *gin.Context) {
	h.withLeaseMutation(c, func(jobID uint, clientID string, now time.Time, actor string, body browserClientBody) error {
		reason := body.Reason
		if reason == "" {
			reason = "browser agent cancel"
		}
		return h.svc.FailBrowserJob(c.Request.Context(), jobID, clientID, now, actor, reason)
	})
}

type leaseMutator func(jobID uint, clientID string, now time.Time, actor string, body browserClientBody) error

func (h *PrintJobHandlers) withLeaseMutation(c *gin.Context, fn leaseMutator) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	jobID, err := parseJobID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	kinds, allowed := requireAuthorizedPrintKinds(c)
	if !allowed {
		return
	}
	if !authorizedJobBelongsToBusiness(h.db, jobID, businessID, kinds) {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	body := parseBrowserClientBody(c)
	if !validClientID(body.ClientID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client_id must be a UUID"})
		return
	}
	if err := fn(jobID, body.ClientID, time.Now(), actorFromCtx(c), body); err != nil {
		if errors.Is(err, print.ErrNotLeaseOwner) || errors.Is(err, print.ErrIllegalTransition) {
			// Domain sentinels are product-safe messages.
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not update print job")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// parseJobID reads the :jobId path param and converts it to a uint.
func parseJobID(c *gin.Context) (uint, error) {
	n, err := strconv.ParseUint(c.Param("jobId"), 10, 64)
	if err != nil || n == 0 {
		return 0, errors.New("invalid job id")
	}
	return uint(n), nil
}

// jobBelongsToBusiness returns true when a PrintJob with the given id exists
// and is owned by businessID.
func jobBelongsToBusiness(db *gorm.DB, jobID, businessID uint) bool {
	var count int64
	db.Model(&database.PrintJob{}).
		Where("id = ? AND business_id = ?", jobID, businessID).
		Count(&count)
	return count > 0
}

func authorizedJobBelongsToBusiness(db *gorm.DB, jobID, businessID uint, kinds []database.PrintJobKind) bool {
	if len(kinds) == 0 {
		return false
	}
	var count int64
	db.Model(&database.PrintJob{}).
		Where("id = ? AND business_id = ? AND kind IN ?", jobID, businessID, kinds).
		Count(&count)
	return count > 0
}

// sourceBelongsToBusiness checks that the source record referenced by a print
// job belongs to the requesting business. Mirrors jobBelongsToBusiness. (PRINT-XTENANT-SOURCEID-1)
func sourceBelongsToBusiness(db *gorm.DB, sourceType string, sourceID, businessID uint) bool {
	if sourceID == 0 || businessID == 0 {
		return false
	}
	var count int64
	switch sourceType {
	case "bill":
		db.Model(&database.Bill{}).
			Where("id = ? AND business_id = ?", sourceID, businessID).
			Count(&count)
	case "order":
		db.Model(&database.Order{}).
			Where("id = ? AND business_id = ?", sourceID, businessID).
			Count(&count)
	default:
		return false
	}
	return count > 0
}
