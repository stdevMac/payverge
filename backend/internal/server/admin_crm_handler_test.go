package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAdminCRMTestDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file:admin_crm_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.Migrator().DropTable(&database.Escalation{}))
	require.NoError(t, gormDB.AutoMigrate(&database.Escalation{}))
	database.SetTestDB(gormDB)
}

func TestGetAdminEscalationsFiltersByStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAdminCRMTestDB(t)

	require.NoError(t, database.CreateEscalation(&database.Escalation{
		Source: database.EscalationSourceOps,
		Issue:  "needs attention",
		Status: "open",
	}))
	require.NoError(t, database.CreateEscalation(&database.Escalation{
		Source: database.EscalationSourceOps,
		Issue:  "already handled",
		Status: "resolved",
	}))

	router := gin.New()
	router.GET("/admin/escalations", GetAdminEscalations)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/escalations?status=open", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Escalations []database.Escalation `json:"escalations"`
		Total       int64                 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, int64(1), resp.Total)
	require.Len(t, resp.Escalations, 1)
	require.Equal(t, "open", resp.Escalations[0].Status)
	require.Equal(t, "needs attention", resp.Escalations[0].Issue)
}

func TestPatchAdminEscalationUpdatesStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAdminCRMTestDB(t)

	require.NoError(t, database.CreateEscalation(&database.Escalation{
		Source: database.EscalationSourceOps,
		Issue:  "need help",
		Status: "open",
	}))

	router := gin.New()
	router.PATCH("/admin/escalations/:id", PatchAdminEscalation)

	body, _ := json.Marshal(map[string]string{"status": "resolved", "admin_notes": "handled"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/admin/escalations/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Escalation database.Escalation `json:"escalation"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "resolved", resp.Escalation.Status)
	require.Equal(t, "handled", resp.Escalation.AdminNotes)
}
