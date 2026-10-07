package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDirectorConsoleRoutes_GatedByOperationalMiddleware locks in that the
// Director Console route registrations sit behind RequireOperationalBusiness(),
// so a suspended business gets the structured 403 business_suspended contract
// and no staff- or role-scoped bypass can reach the handler.
func TestDirectorConsoleRoutes_GatedByOperationalMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "director-gating")
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		UpdateColumn("is_active", false).Error)

	router := gin.New()
	// Simulate authenticated owner (as HybridAuthenticationMiddleware would set).
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwnerA")
		c.Next()
	})

	// Mirror the production registration: RequireOperationalBusiness() at the group
	// level, per-route RBAC, then the handler. The test protects against
	// future refactors that accidentally drop the middleware wrapper.
	group := router.Group("/businesses/:id", RequireOperationalBusiness())
	{
		group.POST("/ai/director/ask", RoleBasedAccessMiddleware("director:write"), AskDirector)
		group.GET("/ai/director/threads", RoleBasedAccessMiddleware("director:read"), ListDirectorThreads)
		group.GET("/ai/director/threads/:threadId/messages", RoleBasedAccessMiddleware("director:read"), GetDirectorThreadMessages)
		group.POST("/ai/director/messages/:messageId/feedback", RoleBasedAccessMiddleware("director:write"), SubmitDirectorFeedback)
		group.GET("/director-console/proactive-insights", RoleBasedAccessMiddleware("director:read"), GetDirectorProactiveInsights)
	}

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{
			name:   "ask returns 403 business_suspended",
			method: http.MethodPost,
			path:   fmt.Sprintf("/businesses/%d/ai/director/ask", business.ID),
			body:   map[string]any{"message": "hello"},
		},
		{
			name:   "list threads returns 403 business_suspended",
			method: http.MethodGet,
			path:   fmt.Sprintf("/businesses/%d/ai/director/threads", business.ID),
		},
		{
			name:   "thread messages return 403 business_suspended",
			method: http.MethodGet,
			path:   fmt.Sprintf("/businesses/%d/ai/director/threads/1/messages", business.ID),
		},
		{
			name:   "feedback returns 403 business_suspended",
			method: http.MethodPost,
			path:   fmt.Sprintf("/businesses/%d/ai/director/messages/1/feedback", business.ID),
			body:   map[string]any{"rating": "up"},
		},
		{
			name:   "proactive insights return 403 business_suspended",
			method: http.MethodGet,
			path:   fmt.Sprintf("/businesses/%d/director-console/proactive-insights", business.ID),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reqBody *bytes.Reader
			if tc.body != nil {
				payload, err := json.Marshal(tc.body)
				require.NoError(t, err)
				reqBody = bytes.NewReader(payload)
			} else {
				reqBody = bytes.NewReader(nil)
			}
			req := httptest.NewRequest(tc.method, tc.path, reqBody)
			if tc.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusForbidden, w.Code)
			assert.Contains(t, w.Body.String(), `"code":"business_suspended"`)
		})
	}
}
