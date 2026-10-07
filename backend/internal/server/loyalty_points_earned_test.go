package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPointsEarnedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file:points_earned_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.Migrator().DropTable(
		&database.Table{}, &database.Bill{}, &database.CustomerBusiness{}, &database.CustomerVisit{},
	))
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{}, &database.Bill{}, &database.CustomerBusiness{}, &database.CustomerVisit{},
	))
	database.SetTestDB(gormDB)
	return gormDB
}

func pointsEarnedRouter(customerID uint) *gin.Engine {
	router := gin.New()
	router.GET("/guest/table/:code/points-earned", func(c *gin.Context) {
		c.Set("customer_id", customerID)
		GetLoyaltyPointsEarned(c)
	})
	return router
}

func TestGetLoyaltyPointsEarnedReturnsVisitPoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPointsEarnedTestDB(t)

	require.NoError(t, db.Create(&database.Table{BusinessID: 9, TableCode: "TBL-1", Name: "Table 1", IsActive: true}).Error)
	bill := database.Bill{BusinessID: 9, BillNumber: "B-100", Status: database.BillStatusPaid, SettlementAddr: "0x1", TippingAddr: "0x2"}
	require.NoError(t, db.Create(&bill).Error)
	connection := database.CustomerBusiness{CustomerID: 77, BusinessID: 9, IsActive: true, LoyaltyPoints: 240}
	require.NoError(t, db.Create(&connection).Error)
	require.NoError(t, db.Create(&database.CustomerVisit{
		CustomerBusinessID: connection.ID,
		BillID:             &bill.ID,
		PointsEarned:       40,
		VisitDate:          time.Now(),
	}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/guest/table/TBL-1/points-earned?bill_number=B-100", nil)
	pointsEarnedRouter(77).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, float64(40), payload["points_earned"])
	assert.Equal(t, float64(240), payload["total_points"])
}

func TestGetLoyaltyPointsEarnedBeforeVisitRecorded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPointsEarnedTestDB(t)

	require.NoError(t, db.Create(&database.Table{BusinessID: 9, TableCode: "TBL-1", Name: "Table 1", IsActive: true}).Error)
	require.NoError(t, db.Create(&database.Bill{BusinessID: 9, BillNumber: "B-100", Status: database.BillStatusPaid, SettlementAddr: "0x1", TippingAddr: "0x2"}).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{CustomerID: 77, BusinessID: 9, IsActive: true}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/guest/table/TBL-1/points-earned?bill_number=B-100", nil)
	pointsEarnedRouter(77).ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, "visit_not_recorded", payload["code"])
}
