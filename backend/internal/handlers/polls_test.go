package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newPollTestServer(t *testing.T, staffID uint, role database.StaffRole) (
	func(method, url string, body interface{}) *httptest.ResponseRecorder, *database.DB, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Staff{}, &database.Position{}, &database.StaffPosition{},
		&database.Poll{}, &database.PollOption{}, &database.PollVote{}))
	database.SetTestDB(gormDB)
	d := database.GetDBWrapper()
	h := NewPollHandler(d)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if staffID != 0 {
			c.Set("staff_id", staffID)
			c.Set("staff_role", string(role))
		}
		c.Next()
	})
	r.GET("/b/:id/polls", h.List)
	r.POST("/b/:id/polls", h.Create)
	r.GET("/b/:id/polls/:pollId/results", h.Results)
	r.POST("/b/:id/polls/:pollId/vote", h.Vote)
	r.POST("/b/:id/polls/:pollId/close", h.Close)
	do := func(method, url string, body interface{}) *httptest.ResponseRecorder {
		var rdr *bytes.Reader
		if body != nil {
			bb, _ := json.Marshal(body)
			rdr = bytes.NewReader(bb)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, url, rdr)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	return do, d, func() { _ = sqlDB.Close() }
}

func seedHandlerPoll(t *testing.T, d *database.DB, biz uint, anon bool) (uint, []uint) {
	t.Helper()
	p := &database.Poll{BusinessID: biz, AuthorStaffID: 1, Question: "Pizza?", IsAnonymous: anon, AudienceFilter: "all", Status: database.PollStatusOpen}
	require.NoError(t, d.GetGorm().Create(p).Error)
	o1 := &database.PollOption{BusinessID: biz, PollID: p.ID, Label: "Yes", SortOrder: 0}
	o2 := &database.PollOption{BusinessID: biz, PollID: p.ID, Label: "No", SortOrder: 1}
	require.NoError(t, d.GetGorm().Create(o1).Error)
	require.NoError(t, d.GetGorm().Create(o2).Error)
	return p.ID, []uint{o1.ID, o2.ID}
}

// TestPollVoteSecondVoteIs409: a second vote by the same staff returns 409.
func TestPollVoteSecondVoteIs409(t *testing.T) {
	do, d, cleanup := newPollTestServer(t, 7, database.StaffRoleServer)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	pollID, opts := seedHandlerPoll(t, d, 1, false)

	w := do(http.MethodPost, fmt.Sprintf("/b/1/polls/%d/vote", pollID), map[string]any{"option_id": opts[0]})
	require.Equal(t, http.StatusOK, w.Code)

	w = do(http.MethodPost, fmt.Sprintf("/b/1/polls/%d/vote", pollID), map[string]any{"option_id": opts[1]})
	require.Equal(t, http.StatusConflict, w.Code, "second vote by same staff must 409")
}

// TestPollAnonymousResultsJSONHasNoVoter: anonymous-poll results never carry a
// voter roster over the wire.
func TestPollAnonymousResultsJSONHasNoVoter(t *testing.T) {
	do, d, cleanup := newPollTestServer(t, 7, database.StaffRoleServer)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	pollID, opts := seedHandlerPoll(t, d, 1, true) // anonymous

	w := do(http.MethodPost, fmt.Sprintf("/b/1/polls/%d/vote", pollID), map[string]any{"option_id": opts[0]})
	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, w.Body.String(), "voter", "anonymous vote response must not carry voters")

	w = do(http.MethodGet, fmt.Sprintf("/b/1/polls/%d/results", pollID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, w.Body.String(), "voter", "anonymous results must not carry voters")
}

// TestPollCreateRequiresTwoOptions: a poll with <2 options 400s.
func TestPollCreateRequiresTwoOptions(t *testing.T) {
	do, d, cleanup := newPollTestServer(t, 1, database.StaffRoleManager)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)

	w := do(http.MethodPost, "/b/1/polls", map[string]any{
		"question": "One choice?",
		"options":  []map[string]any{{"label": "Only"}},
	})
	require.Equal(t, http.StatusBadRequest, w.Code)

	w = do(http.MethodPost, "/b/1/polls", map[string]any{
		"question": "Two choices?",
		"options":  []map[string]any{{"label": "Yes"}, {"label": "No"}},
	})
	require.Equal(t, http.StatusCreated, w.Code)
}

// TestPollCloseThenVoteIs409: voting on a closed poll returns 409.
func TestPollCloseThenVoteIs409(t *testing.T) {
	do, d, cleanup := newPollTestServer(t, 1, database.StaffRoleManager)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	pollID, opts := seedHandlerPoll(t, d, 1, false)

	w := do(http.MethodPost, fmt.Sprintf("/b/1/polls/%d/close", pollID), nil)
	require.Equal(t, http.StatusOK, w.Code)

	w = do(http.MethodPost, fmt.Sprintf("/b/1/polls/%d/vote", pollID), map[string]any{"option_id": opts[0]})
	require.Equal(t, http.StatusConflict, w.Code, "voting on a closed poll must 409")
}

// TestPollVotePastCloseTimeIs409: voting on an open poll whose closes_at is in
// the past returns 409.
func TestPollVotePastCloseTimeIs409(t *testing.T) {
	do, d, cleanup := newPollTestServer(t, 7, database.StaffRoleServer)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	past := time.Now().UTC().Add(-time.Hour)
	p := &database.Poll{BusinessID: 1, AuthorStaffID: 1, Question: "Q", AudienceFilter: "all", Status: database.PollStatusOpen, ClosesAt: &past}
	require.NoError(t, d.GetGorm().Create(p).Error)
	o1 := &database.PollOption{BusinessID: 1, PollID: p.ID, Label: "Yes", SortOrder: 0}
	require.NoError(t, d.GetGorm().Create(o1).Error)

	w := do(http.MethodPost, fmt.Sprintf("/b/1/polls/%d/vote", p.ID), map[string]any{"option_id": o1.ID})
	require.Equal(t, http.StatusConflict, w.Code, "voting on a past-its-close poll must 409")
}

// TestPollCreateRejectsInvalidAudience: a malformed audience filter 400s.
func TestPollCreateRejectsInvalidAudience(t *testing.T) {
	do, d, cleanup := newPollTestServer(t, 1, database.StaffRoleManager)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)

	w := do(http.MethodPost, "/b/1/polls", map[string]any{
		"question":        "Pizza?",
		"audience_filter": "dept:",
		"options":         []map[string]any{{"label": "Yes"}, {"label": "No"}},
	})
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestPollListAudiencePrivacy: a line-staff caller sees only polls targeted at
// "all"/their role/their dept; a manager caller sees every poll.
func TestPollListAudiencePrivacy(t *testing.T) {
	do, d, cleanup := newPollTestServer(t, 7, database.StaffRoleServer)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	for _, aud := range []string{"all", "role:server", "role:manager"} {
		p := &database.Poll{BusinessID: 1, AuthorStaffID: 1, Question: "Q", AudienceFilter: aud, Status: database.PollStatusOpen}
		require.NoError(t, d.GetGorm().Create(p).Error)
	}

	w := do(http.MethodGet, "/b/1/polls", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data []struct {
			AudienceFilter string `json:"audience_filter"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 2, "server sees all + role:server, not role:manager")

	asMgr := pollRouterAs(2, database.StaffRoleManager)
	w = asMgr(http.MethodGet, "/b/1/polls", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 3, "manager sees every poll")
}

// pollRouterAs drives the poll routes as a different identity against the CURRENT
// test DB.
func pollRouterAs(staffID uint, role database.StaffRole) func(method, url string, body interface{}) *httptest.ResponseRecorder {
	h := NewPollHandler(database.GetDBWrapper())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("staff_id", staffID)
		c.Set("staff_role", string(role))
		c.Next()
	})
	r.GET("/b/:id/polls", h.List)
	return func(method, url string, body interface{}) *httptest.ResponseRecorder {
		var rdr *bytes.Reader
		if body != nil {
			bb, _ := json.Marshal(body)
			rdr = bytes.NewReader(bb)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, url, rdr)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
}
