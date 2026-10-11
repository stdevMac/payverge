package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

type PollHandler struct{ db *database.DB }

func NewPollHandler(db *database.DB) *PollHandler { return &PollHandler{db: db} }

// pollWithOptionsDTO is the staff-facing list shape: the poll, its options, and
// the caller's OWN vote (option id) so the UI can pre-select. It never carries
// other voters' identities.
type pollWithOptionsDTO struct {
	database.Poll
	Options []database.PollOption `json:"options"`
	MyVote  *uint                 `json:"my_vote"`
}

func (h *PollHandler) List(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff context required")
		return
	}
	role := c.GetString("staff_role")
	isManager := callerIsManager(c)
	polls, err := h.db.ListPolls(businessID, staffID, role, isManager, 100)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list polls")
		return
	}
	ids := make([]uint, 0, len(polls))
	for _, p := range polls {
		ids = append(ids, p.ID)
	}
	options, err := h.db.ListPollOptionsForPolls(businessID, ids)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load options")
		return
	}
	myVotes, err := h.db.ListStaffVotesForPolls(businessID, staffID, ids)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load votes")
		return
	}
	optsByPoll := map[uint][]database.PollOption{}
	for _, o := range options {
		optsByPoll[o.PollID] = append(optsByPoll[o.PollID], o)
	}
	voteByPoll := map[uint]uint{}
	for _, v := range myVotes {
		voteByPoll[v.PollID] = v.OptionID
	}
	out := make([]pollWithOptionsDTO, 0, len(polls))
	for _, p := range polls {
		dto := pollWithOptionsDTO{Poll: p, Options: optsByPoll[p.ID]}
		if opt, ok := voteByPoll[p.ID]; ok {
			o := opt
			dto.MyVote = &o
		}
		out = append(out, dto)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

type pollOptionDTO struct {
	Label     string `json:"label"`
	SortOrder int    `json:"sort_order"`
}
type pollDTO struct {
	Question       string          `json:"question"`
	IsAnonymous    bool            `json:"is_anonymous"`
	AudienceFilter string          `json:"audience_filter"`
	ClosesAt       string          `json:"closes_at"` // RFC3339, optional
	Options        []pollOptionDTO `json:"options"`
}

func (h *PollHandler) Create(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	var in pollDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	in.Question = strings.TrimSpace(in.Question)
	if in.Question == "" || len(in.Question) > 500 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "question is required")
		return
	}
	options := make([]database.PollOption, 0, len(in.Options))
	for _, o := range in.Options {
		label := strings.TrimSpace(o.Label)
		if label == "" || len(label) > 255 {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "option label is required")
			return
		}
		options = append(options, database.PollOption{Label: label, SortOrder: o.SortOrder})
	}
	if len(options) < 2 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "at least two options are required")
		return
	}
	var closesAt *time.Time
	if strings.TrimSpace(in.ClosesAt) != "" {
		if parsed, err := time.Parse(time.RFC3339, in.ClosesAt); err == nil {
			closesAt = &parsed
		}
	}
	staffID, _ := staffIDFromContext(c)
	p := &database.Poll{
		BusinessID: businessID, AuthorStaffID: staffID, Question: in.Question,
		IsAnonymous: in.IsAnonymous, AudienceFilter: in.AudienceFilter,
		Status: database.PollStatusOpen, ClosesAt: closesAt,
	}
	if err := h.db.CreatePoll(p, options); err != nil {
		if errors.Is(err, database.ErrInvalidAudienceFilter) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "invalid audience")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create poll")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": p})
}

type voteDTO struct {
	OptionID uint `json:"option_id"`
}

func (h *PollHandler) Vote(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	pollID, ok := parseParamUint(c, "pollId")
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff context required")
		return
	}
	var in voteDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if in.OptionID == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "option_id is required")
		return
	}
	if err := h.db.CastVote(businessID, pollID, in.OptionID, staffID); err != nil {
		switch {
		case errors.Is(err, database.ErrAlreadyVoted):
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Already voted")
		case errors.Is(err, database.ErrPollClosed):
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Poll is closed")
		case errors.Is(err, database.ErrPollOptionInvalid):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Option does not belong to poll")
		case errors.Is(err, database.ErrPollNotFound):
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Poll not found")
		default:
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to vote")
		}
		return
	}
	// Return the (voter-identity-aware) results so the UI can render the tally.
	res, err := h.db.PollResults(businessID, pollID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load results")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": res})
}

func (h *PollHandler) Results(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	pollID, ok := parseParamUint(c, "pollId")
	if !ok {
		return
	}
	res, err := h.db.PollResults(businessID, pollID)
	if err != nil {
		if errors.Is(err, database.ErrPollNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Poll not found")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load results")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": res})
}

func (h *PollHandler) Close(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	pollID, ok := parseParamUint(c, "pollId")
	if !ok {
		return
	}
	if err := h.db.ClosePoll(businessID, pollID); err != nil {
		if errors.Is(err, database.ErrPollNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Poll not found")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to close poll")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
