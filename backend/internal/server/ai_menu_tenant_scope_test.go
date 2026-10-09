package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestGetMenuExtractionJob_ScopedToRouteBusiness locks in the menu-digitizer
// tenant-scoping fix: a job belonging to business B must not be readable via
// business A's route (it would leak B's extracted-menu JSON). The mismatch
// returns 404 so cross-tenant job existence is not revealed.
func TestGetMenuExtractionJob_ScopedToRouteBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.MenuExtractionJob{},
		&database.MenuExtractionImage{},
	))
	database.SetTestDB(gormDB)

	bizA := &database.Business{BusinessId: "biz-a", Name: "A", OwnerAddress: "0xA"}
	bizB := &database.Business{BusinessId: "biz-b", Name: "B", OwnerAddress: "0xB"}
	require.NoError(t, gormDB.Create(bizA).Error)
	require.NoError(t, gormDB.Create(bizB).Error)

	job := &database.MenuExtractionJob{
		BusinessID:    bizB.ID,
		Status:        database.ExtractionStatusCompleted,
		ExtractedMenu: `{"secret":"B menu"}`,
	}
	require.NoError(t, database.CreateExtractionJob(job))

	call := func(routeBizID, jobID uint) int {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{
			{Key: "id", Value: fmt.Sprintf("%d", routeBizID)},
			{Key: "jobId", Value: fmt.Sprintf("%d", jobID)},
		}
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		GetMenuExtractionJob(c)
		return w.Code
	}

	// Cross-tenant: A asks for B's job → 404, no leak.
	require.Equal(t, http.StatusNotFound, call(bizA.ID, job.ID))
	// Owner: B asks for B's job → 200.
	require.Equal(t, http.StatusOK, call(bizB.ID, job.ID))
}
