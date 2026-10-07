package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// TestUnassignPositionPayRateRBAC: a manager (schedule:write, no payroll:write)
// cannot unassign a rated link, can unassign an unrated one, and an owner can
// unassign the rated link.
func TestUnassignPositionPayRateRBAC(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newPositionHandlerTestDB(t)
	server.InitializeRBAC(db)
	require.NoError(t, db.GetGorm().Create(&database.Staff{
		ID: 7, BusinessID: 42, Email: "s@x.io", Name: "S", Role: database.StaffRoleServer, InvitedBy: "o",
	}).Error)

	rated := &database.Position{BusinessID: 42, Name: "Server"}
	free := &database.Position{BusinessID: 42, Name: "Host"}
	require.NoError(t, db.CreatePosition(rated))
	require.NoError(t, db.CreatePosition(free))
	require.NoError(t, db.AssignPosition(42, 7, rated.ID, true))
	require.NoError(t, db.AssignPosition(42, 7, free.ID, false))
	require.NoError(t, db.SetPayRateCents(42, 7, rated.ID, 1850))

	h := NewPositionHandler(db)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-Actor") == "owner" {
			c.Set("token_type", "user")
			c.Set("user_id", uint(1))
			c.Set("business_owner_user_id", uint(1))
			c.Next()
			return
		}
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Next()
	})
	r.DELETE("/b/:id/staff/:staffId/positions/:positionId", h.Unassign)

	do := func(actor string, positionID uint) *httptest.ResponseRecorder {
		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/b/42/staff/7/positions/%d", positionID), nil)
		require.NoError(t, err)
		req.Header.Set("X-Test-Actor", actor)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	w := do("manager", rated.ID)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), server.ErrCodeForbidden)
	cents, err := db.GetPayRateCents(42, 7, rated.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1850), cents)

	w = do("manager", free.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	_, err = db.GetPayRateCents(42, 7, free.ID)
	require.ErrorIs(t, err, database.ErrStaffPositionNotFound)

	w = do("owner", rated.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	_, err = db.GetPayRateCents(42, 7, rated.ID)
	require.ErrorIs(t, err, database.ErrStaffPositionNotFound)
}
