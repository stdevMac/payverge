package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

type DocumentHandler struct{ db *database.DB }

func NewDocumentHandler(db *database.DB) *DocumentHandler { return &DocumentHandler{db: db} }

type documentDTO struct {
	Title          string `json:"title"`
	Content        string `json:"content"`
	URL            string `json:"url"`
	RequireAck     bool   `json:"require_ack"`
	AudienceFilter string `json:"audience_filter"`
}

func (h *DocumentHandler) List(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, _ := staffIDFromContext(c)
	role := c.GetString("staff_role")
	isManager := callerIsManager(c)
	list, err := h.db.ListDocuments(businessID, staffID, role, isManager, 200)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list documents")
		return
	}
	// Per-caller ack state (one query, no N+1) so the UI can hide the Acknowledge
	// button on documents this staff member has already acked at the CURRENT
	// version. Acked ids are present->true; a missing id means not-yet-acked.
	acked, err := h.db.DocumentsAckedBy(businessID, staffID, list)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to resolve ack state")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list, "acked": acked})
}

func (h *DocumentHandler) Create(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	var in documentDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len(in.Title) > 255 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "title is required")
		return
	}
	in.Content = strings.TrimSpace(in.Content)
	in.URL = strings.TrimSpace(in.URL)
	if in.Content == "" && in.URL == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "content or url is required")
		return
	}
	if len(in.URL) > 1024 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "url too long")
		return
	}
	staffID, _ := staffIDFromContext(c)
	doc := &database.Document{
		BusinessID: businessID, CreatedByStaffID: staffID, Title: in.Title,
		Content: in.Content, URL: in.URL, Version: 1, RequireAck: in.RequireAck,
		AudienceFilter: in.AudienceFilter, IsActive: true,
	}
	if err := h.db.CreateDocument(doc); err != nil {
		if errors.Is(err, database.ErrInvalidAudienceFilter) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "invalid audience")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create document")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": doc})
}

// Update bumps a document to a new version (re-opening acknowledgement). Gated by
// doc:manage at the route layer.
func (h *DocumentHandler) Update(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	docID, ok := parseParamUint(c, "docId")
	if !ok {
		return
	}
	var in documentDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len(in.Title) > 255 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "title is required")
		return
	}
	in.Content = strings.TrimSpace(in.Content)
	in.URL = strings.TrimSpace(in.URL)
	if in.Content == "" && in.URL == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "content or url is required")
		return
	}
	if len(in.URL) > 1024 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "url too long")
		return
	}
	doc, err := h.db.UpdateDocument(businessID, docID, in.Title, in.URL, in.Content, in.RequireAck, in.AudienceFilter)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrDocumentNotFound):
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Document not found")
		case errors.Is(err, database.ErrInvalidAudienceFilter):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "invalid audience")
		default:
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to update document")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": doc})
}

func (h *DocumentHandler) Ack(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	docID, ok := parseParamUint(c, "docId")
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff context required")
		return
	}
	if err := h.db.AckDocument(businessID, docID, staffID); err != nil {
		if errors.Is(err, database.ErrDocumentNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Document not found")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to acknowledge document")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
