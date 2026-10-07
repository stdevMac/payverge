package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

type RecognitionHandler struct{ db *database.DB }

func NewRecognitionHandler(db *database.DB) *RecognitionHandler { return &RecognitionHandler{db: db} }

type shoutoutDTO struct {
	ToStaffID  uint   `json:"to_staff_id"`
	Message    string `json:"message"`
	Emoji      string `json:"emoji"`
	Visibility string `json:"visibility"`
}

var validShoutoutVisibility = map[string]bool{
	database.ShoutoutVisibilityTeam: true, database.ShoutoutVisibilityPrivate: true,
}

func (h *RecognitionHandler) List(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff context required")
		return
	}
	list, err := h.db.ListShoutouts(businessID, staffID, 100)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list shoutouts")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

func (h *RecognitionHandler) Create(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff context required")
		return
	}
	var in shoutoutDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if in.ToStaffID == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "to_staff_id is required")
		return
	}
	in.Message = strings.TrimSpace(in.Message)
	if in.Message == "" || len(in.Message) > 500 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "message is required")
		return
	}
	in.Emoji = strings.TrimSpace(in.Emoji)
	if len(in.Emoji) > 16 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "emoji too long")
		return
	}
	visibility := strings.TrimSpace(in.Visibility)
	if visibility == "" {
		visibility = database.ShoutoutVisibilityTeam
	}
	if !validShoutoutVisibility[visibility] {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "invalid visibility")
		return
	}
	// from_staff_id is taken from the authenticated context, never the body.
	s := &database.Shoutout{
		BusinessID: businessID, FromStaffID: staffID, ToStaffID: in.ToStaffID,
		Message: in.Message, Emoji: in.Emoji, Visibility: visibility,
	}
	if err := h.db.CreateShoutout(s); err != nil {
		switch {
		case errors.Is(err, database.ErrShoutoutSelf):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "cannot shout out yourself")
		case errors.Is(err, database.ErrShoutoutRecipient):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "recipient not in business")
		default:
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create shoutout")
		}
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": s})
}
