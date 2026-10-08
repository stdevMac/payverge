package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

type LogbookHandler struct{ db *database.DB }

func NewLogbookHandler(db *database.DB) *LogbookHandler { return &LogbookHandler{db: db} }

const (
	shiftNoteListLimit  = 200  // bounded feed: hard cap a single-day read
	shiftNoteMaxContent = 4000 // free-text ceiling
	shiftNoteDateLayout = "2006-01-02"
)

// shiftNoteDTO is the POST body. AuthorStaffID is intentionally absent — the
// author is taken from the staff session, never the client (anti-spoof).
type shiftNoteDTO struct {
	Date     string `json:"date"` // YYYY-MM-DD; empty → today (UTC)
	Category string `json:"category"`
	Content  string `json:"content"`
	ShiftID  *uint  `json:"shift_id"`
}

// staffActorID reads the logging staff id from the Gin context (set by the
// staff-auth middleware). Mirrors the type-switch in accounting.go's actorIDs.
func staffActorID(c *gin.Context) (uint, bool) {
	raw, exists := c.Get("staff_id")
	if !exists {
		return 0, false
	}
	switch v := raw.(type) {
	case uint:
		return v, v != 0
	case int:
		return uint(v), v > 0
	case int64:
		return uint(v), v > 0
	case float64:
		return uint(v), v > 0
	default:
		return 0, false
	}
}

// utcDay parses YYYY-MM-DD to midnight UTC; empty string → today (UTC).
func utcDay(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Now().UTC().Truncate(24 * time.Hour), true
	}
	d, err := time.Parse(shiftNoteDateLayout, s)
	if err != nil {
		return time.Time{}, false
	}
	return d.UTC(), true
}

// Create logs one shift-handover note. Any staff may log (schedule:read). The
// whole DTO is validated before any DB write (reject-before-persist).
func (h *LogbookHandler) Create(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	authorStaffID, ok := staffActorID(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeStaffNoAccess, "Staff session required")
		return
	}
	var in shiftNoteDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if !database.IsValidShiftNoteCategory(in.Category) {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "category must be one of sales|guests|staffing|maintenance|other")
		return
	}
	content := strings.TrimSpace(in.Content)
	if content == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "content is required")
		return
	}
	if len(content) > shiftNoteMaxContent {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "content too long")
		return
	}
	forDate, ok := utcDay(in.Date)
	if !ok {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "date must be YYYY-MM-DD")
		return
	}
	note := &database.ShiftNote{
		BusinessID:    businessID,
		ShiftID:       in.ShiftID,
		ForDate:       forDate,
		AuthorStaffID: authorStaffID, // from session, not the body
		Category:      in.Category,
		Content:       content,
	}
	if err := h.db.CreateShiftNote(note); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to log note")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": note})
}

// shiftNoteResponse is a shift note enriched with its author's display name so
// the operator logbook can show WHO logged each handover (accountability). The
// embedded ShiftNote's json fields serialize flat; author_name is additive (the
// staff feed simply ignores it). Names resolve in ONE batched query, never N+1,
// and cover since-deactivated authors (StaffNamesByIDs is id-scoped, not active).
type shiftNoteResponse struct {
	database.ShiftNote
	AuthorName string `json:"author_name"`
}

// List returns the date-scoped feed (?date=YYYY-MM-DD; default today), newest
// first, each note enriched with its author_name. Bounded: one list read + one
// batched name lookup over the distinct authors — no N+1.
func (h *LogbookHandler) List(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	day, ok := utcDay(c.Query("date"))
	if !ok {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "date must be YYYY-MM-DD")
		return
	}
	notes, err := h.db.ListShiftNotes(businessID, day, day.Add(24*time.Hour), shiftNoteListLimit)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list shift notes")
		return
	}

	// Resolve the distinct authors in ONE batched query, then attach names.
	ids := make([]uint, 0, len(notes))
	seen := make(map[uint]bool, len(notes))
	for _, n := range notes {
		if n.AuthorStaffID != 0 && !seen[n.AuthorStaffID] {
			seen[n.AuthorStaffID] = true
			ids = append(ids, n.AuthorStaffID)
		}
	}
	names, err := h.db.StaffNamesByIDs(businessID, ids)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list shift notes")
		return
	}
	out := make([]shiftNoteResponse, len(notes))
	for i, n := range notes {
		out[i] = shiftNoteResponse{ShiftNote: n, AuthorName: names[n.AuthorStaffID]}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
