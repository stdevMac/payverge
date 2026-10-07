package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/guardrails"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/logger"
	requestmiddleware "github.com/stdevmac/payverge/backend/internal/middleware"
	"github.com/stdevmac/payverge/backend/internal/s3"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"
)

// --- Menu AI Service Injection ---

var menuAIService *services.MenuAIService

const maxMenuExtractionUploadBytes = 10 << 20
const wizardClientMessagesDefaultLimit = 100
const wizardClientMessagesMaxLimit = 200

var menuExtractionUploadMIMEs = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/webp": {},
}

// Injectable protected-object operations so unit tests can capture upload MIME
// without a live S3 client. Production defaults to the s3 package helpers.
var (
	uploadExtractionObject   = s3.UploadBytesProtected
	downloadExtractionObject = s3.DownloadFileProtected
	deleteExtractionObject   = s3.DeleteFileProtected
)

// SetMenuAIService sets the MenuAIService for the handler package
func SetMenuAIService(service *services.MenuAIService) {
	menuAIService = service
}

// GetMenuAIService returns the MenuAIService
func GetMenuAIService() *services.MenuAIService {
	return menuAIService
}

// imagePromptClassifier screens user-supplied image prompt text before a paid
// credit is reserved. Defaults to fail-open AllowAll; main.go wires the real
// GeminiClassifier. Lane E owns the implementations (C2).
var imagePromptClassifier guardrails.InputClassifier = guardrails.AllowAll{}

// imageJobGroup collapses identical concurrent image jobs so a double-click
// reserves ONE credit and both callers share the result (P2-11). Keys must
// hash the complete image brief (see imageJobBriefKey).
var imageJobGroup singleflight.Group

// SetImagePromptClassifier injects the classifier (called from main.go).
func SetImagePromptClassifier(c guardrails.InputClassifier) {
	if c != nil {
		imagePromptClassifier = c
	}
}

// evaluateImagePromptGuardrail classifies the user-supplied custom prompt. It
// returns (allowed, category). Empty text is allowed without a call. Classifier
// errors fail open (C2) so a moderation outage never blocks paying owners.
func evaluateImagePromptGuardrail(c guardrails.InputClassifier, ctx context.Context, businessID uint, locale, text string) (bool, string) {
	if strings.TrimSpace(text) == "" {
		return true, "ok"
	}
	verdict, err := c.Classify(ctx, guardrails.ClassifyRequest{
		Surface:    guardrails.SurfaceImagePrompt,
		BusinessID: businessID,
		Locale:     locale,
		Text:       text,
	})
	if err != nil {
		log.Printf("evaluateImagePromptGuardrail: classifier error (fail-open) business=%d: %v", businessID, err)
		return true, "ok"
	}
	if !verdict.Allowed {
		return false, verdict.Category
	}
	return true, "ok"
}

func wizardClientMessagesLimit(raw string) int {
	requestedLimit, err := strconv.Atoi(raw)
	if err != nil || requestedLimit <= 0 {
		return wizardClientMessagesDefaultLimit
	}
	if requestedLimit > wizardClientMessagesMaxLimit {
		return wizardClientMessagesMaxLimit
	}
	return requestedLimit
}

func getAIMenuRouteBusiness(c *gin.Context) (*database.Business, bool) {
	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return nil, false
	}
	return business, true
}

// --- Request/Response Types ---

// ExtractMenuRequest represents a request to extract menu from images
type ExtractMenuRequest struct {
	Images []struct {
		Base64   string `json:"base64" binding:"required"`
		MimeType string `json:"mime_type" binding:"required"`
	} `json:"images" binding:"required"`
}

// ImportMenuRequest represents a request to import extracted menu
type ImportMenuRequest struct {
	Categories          []database.MenuCategory `json:"categories" binding:"required"`
	Currency            string                  `json:"currency"`
	ConfirmSanitization bool                    `json:"confirm_sanitization"`
}

type ImportWizardMenuRequest struct {
	ConfirmSanitization bool `json:"confirm_sanitization"`
}

// WizardMessageRequest represents a user message in wizard conversation
type WizardMessageRequest struct {
	Message  string `json:"message"`
	Language string `json:"language"`
	Retry    bool   `json:"retry"`
}

// WizardStartRequest is the optional body for starting a wizard session.
// Frontend sends the active UI locale so the AI replies in the same language
// the user is interacting in (DB-stored profile language can drift).
type WizardStartRequest struct {
	Language string `json:"language"`
}

// RegenerateImageRequest represents a request to regenerate a menu item image
type RegenerateImageRequest struct {
	ItemName        string `json:"item_name" binding:"required"`
	ItemDescription string `json:"item_description"`
	CustomPrompt    string `json:"custom_prompt"`
	// DietaryTags are the item's dietary tag ids (e.g. "vegetarian", "vegan")
	// from Menu Builder. Used only to select a pre-written hard-negative
	// sentence in the prompt builder (#601) — see GenerateImageRequest.DietaryTags.
	DietaryTags []string `json:"dietary_tags"`
}

// EnhanceImageRequest represents a request to enhance an existing uploaded photo.
type EnhanceImageRequest struct {
	ImageURL        string `json:"image_url" binding:"required"`
	ItemName        string `json:"item_name" binding:"required"`
	ItemDescription string `json:"item_description"`
	// DietaryTags are the item's dietary tag ids (e.g. "vegetarian", "vegan")
	// from Menu Builder. Used only to select a pre-written hard-negative
	// sentence in the prompt builder (#601) — see GenerateImageRequest.DietaryTags.
	DietaryTags []string `json:"dietary_tags"`
}

// --- Handlers ---

// --- Handlers ---

// StartMenuExtraction inititates a new menu extraction job
// POST /api/business/:id/ai/extract-menu/start
func StartMenuExtraction(c *gin.Context) {
	if !aiProviderConfigured() {
		respondAINotConfigured(c)
		return
	}
	// AI-Pro entitlement and menu RBAC are enforced by middleware on the
	// aiMenuRoutes group in main.go. Handlers resolve the route business and
	// keep tenant scope on jobs/sessions; they do not re-check ownership.
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}
	// Create job
	job := &database.MenuExtractionJob{
		BusinessID: business.ID,
		Status:     database.ExtractionStatusPending,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := database.CreateExtractionJob(job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create extraction job"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"job_id": job.ID})
}

// UploadMenuPage handles uploading a single page/image for a job
// POST /api/business/:id/ai/extract-menu/upload/:jobId
func UploadMenuPage(c *gin.Context) {
	if !aiProviderConfigured() {
		respondAINotConfigured(c)
		return
	}
	jobID, err := strconv.ParseUint(c.Param("jobId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	// Get job and verify ownership (indirectly via business_id check or job lookup)
	job, err := database.GetExtractionJobByID(uint(jobID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	// Verify request business matches job business
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}
	if job.BusinessID != business.ID {
		// 404 (not 403) so a cross-tenant caller cannot distinguish "job exists
		// elsewhere" from "no such job" — matches ProcessMenuExtraction / GetMenuExtractionJob.
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	if !parseMultipartWithUploadCap(c, maxMenuExtractionUploadBytes) {
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}

	safeName, contentType, ok := validateUploadedFile(c, file, uploadPolicy{
		MaxBytes:    maxMenuExtractionUploadBytes,
		AllowedMIME: menuExtractionUploadMIMEs,
	})
	if !ok {
		return
	}

	// Get page order
	pageOrder, _ := strconv.Atoi(c.PostForm("page_order"))

	// Read verified bytes once; new rows store them in protected object storage
	// so restarts do not depend on local container disk.
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read uploaded file"})
		return
	}
	pageBytes, readErr := io.ReadAll(io.LimitReader(src, maxMenuExtractionUploadBytes+1))
	_ = src.Close()
	if readErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read uploaded file"})
		return
	}
	if int64(len(pageBytes)) > maxMenuExtractionUploadBytes {
		respondPayloadTooLarge(c, maxMenuExtractionUploadBytes)
		return
	}

	objectName := menuExtractionPageFilename(pageOrder, safeName, contentType)
	folder := fmt.Sprintf("ai/menu-extraction/%d/%d", business.ID, job.ID)
	location, err := uploadExtractionObject(pageBytes, objectName, folder, contentType)
	if err != nil {
		log.Printf("ERROR: protected upload for extraction job %d failed: %v", job.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store image"})
		return
	}
	// Prefer the relative storage key (folder/name) for durable downloads; fall
	// back to the returned location when the uploader only yields a full URL.
	storageKey := folder + "/" + objectName
	if location != "" && !strings.HasPrefix(location, "http") {
		storageKey = location
	}

	// Create image record with verified MIME and protected storage identity.
	// FilePath is retained as empty for new rows (legacy local path only).
	img := &database.MenuExtractionImage{
		JobID:      job.ID,
		FilePath:   "",
		PageOrder:  pageOrder,
		MIMEType:   contentType,
		StorageKey: storageKey,
		CreatedAt:  time.Now(),
	}
	if err := database.AddExtractionImage(img); err != nil {
		// Best-effort cleanup of the orphaned protected object.
		_ = deleteExtractionObject(storageKey)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record image"})
		return
	}

	// Update job status
	job.Status = database.ExtractionStatusUploading
	job.ImageCount++
	if err := database.UpdateExtractionJob(job); err != nil {
		log.Printf("ERROR: failed to update extraction job %d: %v", job.ID, err)
	}

	c.JSON(http.StatusOK, gin.H{"image_id": img.ID})
}

func menuExtractionPageFilename(pageOrder int, safeName string, contentType string) string {
	ext := extensionForUploadMIME(safeName, contentType)
	return fmt.Sprintf("page_%d_%s", pageOrder, generateUniqueFilename("menu-page"+ext))
}

// LoadMenuExtractionInputsForWorker is the production downloader for the
// menu extraction worker (package-level hook used from cmd/app).
func LoadMenuExtractionInputsForWorker(images []database.MenuExtractionImage) ([]services.MenuExtractionInput, error) {
	return loadMenuExtractionInputs(images)
}

// CleanupMenuExtractionAssetsForWorker is the production cleaner for terminal jobs.
func CleanupMenuExtractionAssetsForWorker(images []database.MenuExtractionImage) {
	cleanupMenuExtractionAssets(images)
}

// loadMenuExtractionInputs materializes verified page bytes for the extractor.
// Prefer protected StorageKey; fall back to legacy local FilePath. MIME is the
// stored verified type or content-sniffed for pre-000153 rows.
func loadMenuExtractionInputs(images []database.MenuExtractionImage) ([]services.MenuExtractionInput, error) {
	out := make([]services.MenuExtractionInput, 0, len(images))
	for _, img := range images {
		var data []byte
		var err error
		name := fmt.Sprintf("page_%d", img.PageOrder)
		if strings.TrimSpace(img.StorageKey) != "" {
			data, err = downloadExtractionObject(img.StorageKey)
			if err != nil {
				return nil, fmt.Errorf("download page %d: %w", img.PageOrder, err)
			}
		} else if strings.TrimSpace(img.FilePath) != "" {
			data, err = os.ReadFile(img.FilePath)
			if err != nil {
				return nil, fmt.Errorf("read legacy page %d: %w", img.PageOrder, err)
			}
		} else {
			return nil, fmt.Errorf("page %d has no storage identity", img.PageOrder)
		}
		mime := strings.TrimSpace(img.MIMEType)
		if mime == "" {
			mime = http.DetectContentType(data)
		}
		if normalized, ok := services.NormalizeExtractionMIME(mime); ok {
			mime = normalized
		} else {
			return nil, fmt.Errorf("unsupported MIME for page %d: %s", img.PageOrder, mime)
		}
		out = append(out, services.MenuExtractionInput{
			Name:     name,
			MIMEType: mime,
			Data:     data,
		})
	}
	return out, nil
}

func cleanupMenuExtractionAssets(images []database.MenuExtractionImage) {
	var legacyDir string
	for _, img := range images {
		if key := strings.TrimSpace(img.StorageKey); key != "" {
			if err := deleteExtractionObject(key); err != nil {
				log.Printf("WARNING: failed to delete protected extraction object %s: %v", key, err)
			}
		}
		if path := strings.TrimSpace(img.FilePath); path != "" {
			_ = os.Remove(path)
			if legacyDir == "" {
				legacyDir = filepath.Dir(path)
			}
		}
	}
	if legacyDir != "" {
		_ = os.RemoveAll(legacyDir)
	}
}

func extensionForUploadMIME(name string, contentType string) string {
	if ext := strings.ToLower(filepath.Ext(name)); ext != "" {
		return ext
	}
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}

// ProcessMenuExtraction triggers durable AI processing via the extraction worker.
// POST /api/business/:id/ai/extract-menu/process/:jobId
// Idempotent: repeated calls for a processing job return already_running; completed
// jobs return their terminal status without launching another provider call.
func ProcessMenuExtraction(c *gin.Context) {
	if !aiProviderConfigured() {
		respondAINotConfigured(c)
		return
	}
	jobID, err := strconv.ParseUint(c.Param("jobId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	job, err := database.GetExtractionJobByID(uint(jobID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	// Scope the job to the route business — a job ID from another tenant must
	// not be processable. 404 (not 403) so cross-tenant job existence isn't
	// revealed. Mirrors UploadMenuPage.
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}
	if job.BusinessID != business.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	switch job.Status {
	case database.ExtractionStatusCompleted:
		c.JSON(http.StatusOK, gin.H{
			"status":          "completed",
			"job_id":          job.ID,
			"already_running": false,
		})
		return
	case database.ExtractionStatusProcessing:
		// Wake the worker in case the prior claim is stale; claim logic is the
		// single-flight arbiter so this cannot double-bill a live lease.
		_ = services.EnqueueMenuExtraction(job.ID)
		c.JSON(http.StatusOK, gin.H{
			"status":          "processing",
			"job_id":          job.ID,
			"already_running": true,
		})
		return
	}

	if err := services.EnqueueMenuExtraction(job.ID); err != nil {
		log.Printf("ERROR: enqueue extraction job %d: %v", job.ID, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Menu extraction worker unavailable",
			"code":  "worker_unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":          "processing",
		"job_id":          job.ID,
		"already_running": false,
	})
}

// GetMenuExtractionJob retrieves job status
// GET /api/business/:id/ai/extract-menu/:jobId
func GetMenuExtractionJob(c *gin.Context) {
	jobID, err := strconv.ParseUint(c.Param("jobId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	job, err := database.GetExtractionJobByID(uint(jobID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	// Scope the job to the route business so one tenant cannot read another
	// tenant's extracted-menu JSON. 404 (not 403) to avoid leaking existence.
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}
	if job.BusinessID != business.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	c.JSON(http.StatusOK, job)
}

// ImportExtractedMenu imports an extracted menu to the database
// POST /api/business/:id/ai/import-extracted-menu
func ImportExtractedMenu(c *gin.Context) {
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}

	var req ImportMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	sanitized, report := services.SanitizeMenuCategories(req.Categories)
	if report.HasDrops() && !req.ConfirmSanitization {
		c.JSON(http.StatusOK, gin.H{
			"message":               "Review menu sanitization before import",
			"requires_confirmation": true,
			"sanitized_categories":  sanitized,
			"sanitization":          report,
			"dropped_allergens":     report.DroppedAllergens,
			"dropped_dietary_tags":  report.DroppedDietaryTags,
			"dropped_items":         report.DroppedItems,
		})
		return
	}
	newVersion, err := database.AppendMenuCategories(business.ID, sanitized)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to import menu"})
		return
	}

	services.InvalidatePricingCache(business.ID)

	importedBusinessID := business.ID
	logger.SafeGo(func() { seedImportedMenuTranslations(importedBusinessID) })

	// Surface sanitize counts so review-before-publish can see silent drops
	// (unknown allergen IDs, bad prices). Near-canonical IDs are rewritten
	// in SanitizeMenuCategories and do not count as dropped.
	c.JSON(http.StatusOK, gin.H{
		"message":               "Menu imported successfully",
		"categories":            len(sanitized),
		"version":               newVersion,
		"requires_confirmation": false,
		"sanitization":          report,
		"dropped_allergens":     report.DroppedAllergens,
		"dropped_dietary_tags":  report.DroppedDietaryTags,
		"dropped_items":         report.DroppedItems,
	})
}

// StartWizardSession starts a new AI menu wizard session
// POST /api/business/:id/ai/wizard/start
func StartWizardSession(c *gin.Context) {
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}

	// The admin lifecycle lock is enforced by RequireOperationalBusiness on the route group (main.go).
	service := GetMenuAIService()
	if service == nil {
		respondAINotConfigured(c)
		return
	}

	// Prefer the language the frontend sends (current UI locale).
	// Fall back to the stored profile language only if the client didn't send one.
	var startReq WizardStartRequest
	_ = c.ShouldBindJSON(&startReq)
	language := strings.TrimSpace(startReq.Language)
	if language == "" {
		language = determineBusinessOwnerLanguage(business)
	}
	requestID := c.GetString(requestmiddleware.RequestIDKey)
	requestCtx := llm.WithRequestID(c.Request.Context(), requestID)
	session, response, err := service.StartWizardSession(requestCtx, business.ID, language)
	if err != nil {
		code, _, _, _ := MapAIError(err)
		log.Printf("ai_wizard_request_failed request_id=%s business_id=%d code=%s", requestID, business.ID, code)
		RespondAIError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id": session.ID,
		"response":   response,
	})
}

// SendWizardMessage sends a message in the wizard conversation
// POST /api/business/:id/ai/wizard/:sessionId/message
func SendWizardMessage(c *gin.Context) {
	sessionID, err := strconv.ParseUint(c.Param("sessionId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session ID"})
		return
	}

	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}

	// Verify session belongs to business
	session, err := database.GetWizardSessionByID(uint(sessionID))
	if err != nil || session.BusinessID != business.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	var req WizardMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	service := GetMenuAIService()
	if service == nil {
		respondAINotConfigured(c)
		return
	}

	requestID := c.GetString(requestmiddleware.RequestIDKey)
	requestCtx := llm.WithRequestID(c.Request.Context(), requestID)
	message := strings.TrimSpace(req.Message)
	if !req.Retry && message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Message is required"})
		return
	}
	if len([]rune(message)) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Message is too long"})
		return
	}
	var response *services.WizardResponse
	if req.Retry {
		response, err = service.RetryWizardConversation(requestCtx, uint(sessionID))
	} else {
		response, err = service.ContinueWizardConversation(requestCtx, uint(sessionID), message)
	}
	if err != nil {
		code, _, _, _ := MapAIError(err)
		log.Printf("ai_wizard_request_failed request_id=%s session_id=%d code=%s", requestID, sessionID, code)
		RespondAIError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id": sessionID,
		"response":   response,
	})
}

// GetWizardSession retrieves the current state of a wizard session
// GET /api/business/:id/ai/wizard/:sessionId
func GetWizardSession(c *gin.Context) {
	sessionID, err := strconv.ParseUint(c.Param("sessionId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session ID"})
		return
	}

	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}

	session, err := database.GetWizardSessionByID(uint(sessionID))
	if err != nil || session.BusinessID != business.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	messages, err := database.GetWizardMessagesForClient(uint(sessionID), wizardClientMessagesLimit(c.Query("limit")))
	if err != nil {
		messages = []database.MenuWizardMessage{}
	}

	// Filter out system messages for client
	clientMessages := []gin.H{}
	for _, msg := range messages {
		if msg.Role != "system" {
			clientMessages = append(clientMessages, gin.H{
				"role":       msg.Role,
				"content":    msg.Content,
				"created_at": msg.CreatedAt,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id":     session.ID,
		"status":         session.Status,
		"config":         session.Config,
		"generated_menu": session.GeneratedMenu,
		"messages":       clientMessages,
		"created_at":     session.CreatedAt,
	})
}

// GenerateMenuFromWizard generates a full menu from wizard session
// POST /api/business/:id/ai/wizard/:sessionId/generate
func GenerateMenuFromWizard(c *gin.Context) {
	sessionID, err := strconv.ParseUint(c.Param("sessionId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session ID"})
		return
	}

	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}

	session, err := database.GetWizardSessionByID(uint(sessionID))
	if err != nil || session.BusinessID != business.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	service := GetMenuAIService()
	if service == nil {
		respondAINotConfigured(c)
		return
	}

	menu, err := service.GenerateMenuFromWizard(c.Request.Context(), uint(sessionID))
	if err != nil {
		log.Printf("AI menu wizard: GenerateMenuFromWizard failed (session=%d): %v", sessionID, err)
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not generate menu")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id": sessionID,
		"menu":       menu,
	})
}

// ImportWizardMenu imports a generated wizard menu to the database
// POST /api/business/:id/ai/wizard/:sessionId/import
func ImportWizardMenu(c *gin.Context) {
	sessionID, err := strconv.ParseUint(c.Param("sessionId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session ID"})
		return
	}

	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}

	session, err := database.GetWizardSessionByID(uint(sessionID))
	if err != nil || session.BusinessID != business.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	if session.GeneratedMenu == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No menu has been generated yet"})
		return
	}

	var req ImportWizardMenuRequest
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondBindError(c, err)
			return
		}
	}

	// Parse the generated menu
	var generatedMenu services.GeneratedMenu
	if err := json.Unmarshal([]byte(session.GeneratedMenu), &generatedMenu); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse generated menu"})
		return
	}

	sanitized, report := services.SanitizeMenuCategories(generatedMenu.Categories)
	if report.HasDrops() && !req.ConfirmSanitization {
		c.JSON(http.StatusOK, gin.H{
			"message":               "Review menu sanitization before import",
			"requires_confirmation": true,
			"sanitized_categories":  sanitized,
			"sanitization":          report,
			"dropped_allergens":     report.DroppedAllergens,
			"dropped_dietary_tags":  report.DroppedDietaryTags,
			"dropped_items":         report.DroppedItems,
		})
		return
	}
	newVersion, err := database.AppendMenuCategories(business.ID, sanitized)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to import menu"})
		return
	}

	services.InvalidatePricingCache(business.ID)

	importedBusinessID := business.ID
	logger.SafeGo(func() { seedImportedMenuTranslations(importedBusinessID) })

	c.JSON(http.StatusOK, gin.H{
		"message":               "Menu imported successfully",
		"categories":            len(sanitized),
		"version":               newVersion,
		"requires_confirmation": false,
		"sanitization":          report,
		"dropped_allergens":     report.DroppedAllergens,
		"dropped_dietary_tags":  report.DroppedDietaryTags,
		"dropped_items":         report.DroppedItems,
	})
}

// seedImportedMenuTranslations synchronously translates the stored menu into
// the business's configured guest languages. It is the synchronous core used
// (via SafeGo) after AI menu imports, mirroring the language-add batch path —
// without it, AI-onboarded menus stay single-language until a manual "Sync
// Translations" or a guest-miss backfill. No-op when batch translation is not
// configured. (D4c)
func seedImportedMenuTranslations(businessID uint) {
	if !IsBatchTranslationEnabled() {
		return
	}
	languageCodes := businessGuestLanguageCodes(businessID)
	if len(languageCodes) == 0 {
		return
	}
	if err := TranslateBusinessMenuForLanguages(businessID, languageCodes); err != nil {
		log.Printf("post-import menu translation failed for business %d: %v", businessID, err)
	}
}

// reserveGenerateRefundImage reserves one image generation, runs gen, and
// refunds it on any error OR panic (the panic is re-raised after the refund).
//
// The monthly alert fires only once the generation has stuck. Reserving claims
// the alert latch, but a claim that gets refunded is rolled back along with the
// latch, so alerting at reservation time would report a generation that never
// happened — and, because the rollback re-arms the latch, would let the next
// real crossing alert a second time for a single crossing of the threshold.
var (
	validateGeneratedImageDelivery = services.ValidateGeneratedImageDelivery
	refundReservedImageGeneration  = func(r database.ImageUsageReservation) error {
		return database.RefundImageGeneration(r)
	}
)

func reserveGenerateRefundImage(ctx context.Context, business *database.Business, gen func() (*services.GeneratedImage, error)) (result *services.GeneratedImage, err error) {
	res, rerr := database.ReserveImageGeneration(business, imageDailyLimit(), imageMonthlyAlert())
	if rerr != nil {
		return nil, rerr
	}
	defer func() {
		if rec := recover(); rec != nil {
			if refundErr := refundReservedImageGeneration(res); refundErr != nil {
				log.Printf("reserveGenerateRefundImage: refund failed for business %d after panic: %v", business.ID, refundErr)
			}
			panic(rec)
		}
		if err != nil {
			if refundErr := refundReservedImageGeneration(res); refundErr != nil {
				log.Printf("reserveGenerateRefundImage: refund failed for business %d: %v", business.ID, refundErr)
				err = fmt.Errorf("%w; image usage refund failed: %v", err, refundErr)
			}
			return
		}
		notifyImageUsageAlert(res)
	}()
	result, err = gen()
	if err == nil {
		err = validateGeneratedImageDelivery(ctx, result)
	}
	return result, err
}

// RegenerateMenuItemImage regenerates an image for a menu item
// POST /api/business/:id/ai/regenerate-image
func RegenerateMenuItemImage(c *gin.Context) {
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}
	if !aiProviderConfigured() {
		respondAINotConfigured(c)
		return
	}

	// The admin lifecycle lock is enforced by RequireOperationalBusiness on the route group (main.go).
	var req RegenerateImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	dietaryTags, tagErr := normalizeImageDietaryTags(req.DietaryTags)
	if tagErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": tagErr.Error(), "code": "invalid_image_request"})
		return
	}

	service := GetMenuAIService()
	if service == nil {
		respondAINotConfigured(c)
		return
	}

	if allowed, category := evaluateImagePromptGuardrail(imagePromptClassifier, c.Request.Context(), business.ID, determineBusinessOwnerLanguage(business), req.CustomPrompt); !allowed {
		log.Printf("RegenerateMenuItemImage: custom prompt blocked (%s) for business %s", category, business.BusinessId)
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "That request can't be used for an image. Try describing the dish itself.",
			"code":  "prompt_rejected",
		})
		return
	}

	key := imageJobBriefKey(imageJobBrief{
		BusinessID:  business.ID,
		Tool:        "regenerate",
		Name:        req.ItemName,
		Description: req.ItemDescription,
		Prompt:      req.CustomPrompt,
		DietaryTags: dietaryTags,
	})
	v, genErr := doSharedImageJob(c.Request.Context(), key, func(jobCtx context.Context) (interface{}, error) {
		result, gErr := reserveGenerateRefundImage(jobCtx, business, func() (*services.GeneratedImage, error) {
			return service.RegenerateItemImage(jobCtx, business.ID, req.ItemName, req.ItemDescription, req.CustomPrompt, dietaryTags)
		})
		if gErr != nil {
			return nil, gErr
		}
		if recErr := database.RecordAIGeneratedImage(business.ID, result.URL, "regenerate", result.Model); recErr != nil {
			log.Printf("RegenerateMenuItemImage: provenance record failed for business %s: %v", business.BusinessId, recErr)
		}
		return result, nil
	})
	if respondImageDailyLimit(c, genErr) {
		return
	}
	if genErr != nil {
		log.Printf("RegenerateMenuItemImage: generation failed for business %s: %v", business.BusinessId, genErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate image"})
		return
	}
	image := v.(*services.GeneratedImage)
	c.JSON(http.StatusOK, gin.H{
		"url":       image.URL,
		"credit":    "Generated by AI",
		"mime_type": image.MIMEType,
		"model":     image.Model,
	})
}

// EnhanceMenuItemImage enhances an existing uploaded photo (image-to-image).
// POST /api/business/:id/ai/enhance-image
func EnhanceMenuItemImage(c *gin.Context) {
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}
	// Answer before the S3 fetch: no provider means the fetch is wasted work.
	if !aiProviderConfigured() {
		respondAINotConfigured(c)
		return
	}

	var req EnhanceImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	dietaryTags, tagErr := normalizeImageDietaryTags(req.DietaryTags)
	if tagErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": tagErr.Error(), "code": "invalid_image_request"})
		return
	}

	// SSRF guard: only ever fetch images from our own public asset bucket.
	imageBytes, mimeType, err := s3.DownloadPublicAsset(c.Request.Context(), req.ImageURL)
	if err != nil {
		// Logged server-side for diagnosability (distinguishes a tenant passing a
		// foreign/garbage URL from our own bucket fetch failing); the client body
		// stays opaque so it never leaks the allowlist host or internal errors.
		log.Printf("EnhanceMenuItemImage: asset fetch rejected for business %s: %v", business.BusinessId, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image url"})
		return
	}

	service := GetMenuAIService()
	if service == nil {
		respondAINotConfigured(c)
		return
	}

	key := imageJobBriefKey(imageJobBrief{
		BusinessID:   business.ID,
		Tool:         "enhance",
		Name:         req.ItemName,
		Description:  req.ItemDescription,
		SourceSHA256: sourceImageSHA256(imageBytes),
		DietaryTags:  dietaryTags,
	})
	v, genErr := doSharedImageJob(c.Request.Context(), key, func(jobCtx context.Context) (interface{}, error) {
		result, gErr := reserveGenerateRefundImage(jobCtx, business, func() (*services.GeneratedImage, error) {
			return service.EnhanceItemImage(jobCtx, business.ID, imageBytes, mimeType, req.ItemName, req.ItemDescription, "1:1", dietaryTags)
		})
		if gErr != nil {
			return nil, gErr
		}
		if recErr := database.RecordAIGeneratedImage(business.ID, result.URL, "enhance", result.Model); recErr != nil {
			log.Printf("EnhanceMenuItemImage: provenance record failed for business %s: %v", business.BusinessId, recErr)
		}
		return result, nil
	})
	if respondImageDailyLimit(c, genErr) {
		return
	}
	if genErr != nil {
		log.Printf("EnhanceMenuItemImage: enhancement failed for business %s: %v", business.BusinessId, genErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enhance image"})
		return
	}
	image := v.(*services.GeneratedImage)
	c.JSON(http.StatusOK, gin.H{
		"url":       image.URL,
		"credit":    "Enhanced by AI",
		"mime_type": image.MIMEType,
		"model":     image.Model,
	})
}
