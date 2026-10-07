package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupKioskTestDB installs an isolated in-memory DB migrated with the kiosk
// models and returns a wired KioskHandler + gin engine. Throttle is left nil
// (lockout disabled) — the throttle behavior itself is covered in the server
// package's kiosk_pin_test.go.
func setupKioskTestDB(t *testing.T) (*KioskHandler, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, gdb.AutoMigrate(&database.Business{}, &database.Staff{}, &database.TimeEntry{}))
	prev := database.GetDB()
	database.SetTestDB(gdb)
	t.Cleanup(func() { database.SetTestDB(prev) })

	require.NoError(t, gdb.Create(&database.Business{ID: 1, BusinessId: "biz-1", Timezone: "UTC"}).Error)
	require.NoError(t, gdb.Create(&database.Business{ID: 2, BusinessId: "biz-2", Timezone: "UTC"}).Error)

	h := NewKioskHandler(database.GetDBWrapper())
	r := gin.New()
	r.GET("/b/:id/kiosk/roster", h.Roster)
	r.POST("/b/:id/kiosk/punch", h.Punch)
	return h, r
}

func seedStaff(t *testing.T, id, businessID uint, name, pin string) {
	t.Helper()
	require.NoError(t, database.GetDB().Create(&database.Staff{
		ID: id, BusinessID: businessID, Email: fmt.Sprintf("s%d@biz.test", id),
		Name: name, Role: database.StaffRoleServer, InvitedBy: "owner@biz.test", IsActive: true,
	}).Error)
	if pin != "" {
		require.NoError(t, database.SetStaffPin(id, pin))
	}
}

func kioskPunch(t *testing.T, r *gin.Engine, biz uint, staffID uint, pin string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(kioskPunchDTO{StaffID: staffID, Pin: pin})
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/b/%d/kiosk/punch", biz), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestKioskPunch_TogglesInThenOut proves a correct PIN clocks the staffer in on
// the first punch and out on the second, and that the response is money-free.
func TestKioskPunch_TogglesInThenOut(t *testing.T) {
	_, r := setupKioskTestDB(t)
	seedStaff(t, 10, 1, "Dana", "123456")

	w := kioskPunch(t, r, 1, 10, "123456")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), "$", "kiosk punch must be money-free")
	require.False(t, strings.Contains(strings.ToLower(w.Body.String()), "rate"), "no pay-rate field")

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Action  string `json:"action"`
			StaffID uint   `json:"staff_id"`
			Name    string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, kioskActionClockedIn, resp.Data.Action)
	require.Equal(t, "Dana", resp.Data.Name)

	// Second punch closes it out.
	w2 := kioskPunch(t, r, 1, 10, "123456")
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	require.Equal(t, kioskActionClockedOut, resp.Data.Action)
}

// TestKioskPunch_ForeignStaffIs404 proves the tenant safety net: a staffer that
// belongs to another business is rejected as not-found BEFORE any PIN check.
func TestKioskPunch_ForeignStaffIs404(t *testing.T) {
	_, r := setupKioskTestDB(t)
	seedStaff(t, 20, 2, "Zed", "123456") // business 2

	w := kioskPunch(t, r, 1, 20, "123456") // punched against business 1
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

// TestKioskPunch_NoPinIs409 proves a staffer without a PIN cannot be punched in.
func TestKioskPunch_NoPinIs409(t *testing.T) {
	_, r := setupKioskTestDB(t)
	seedStaff(t, 12, 1, "Fio", "") // no PIN

	w := kioskPunch(t, r, 1, 12, "0000")
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

// TestKioskPunch_WrongPinIs403 proves a wrong PIN is rejected with pin_invalid
// and leaves the staffer un-punched.
func TestKioskPunch_WrongPinIs403(t *testing.T) {
	h, r := setupKioskTestDB(t)
	seedStaff(t, 10, 1, "Dana", "123456")

	w := kioskPunch(t, r, 1, 10, "999999")
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "pin_invalid")

	// No punch was recorded.
	_, err := h.db.GetOpenEntry(1, 10)
	require.ErrorIs(t, err, database.ErrNotClockedIn)
}

// TestKioskPunch_BadBodyIs400 proves missing staff_id/pin is a 400.
func TestKioskPunch_BadBodyIs400(t *testing.T) {
	_, r := setupKioskTestDB(t)
	req, _ := http.NewRequest(http.MethodPost, "/b/1/kiosk/punch", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// TestKioskRosterEndpoint proves the roster envelope carries has_pin + on_clock
// and stays money-free.
func TestKioskRosterEndpoint(t *testing.T) {
	_, r := setupKioskTestDB(t)
	seedStaff(t, 10, 1, "Dana", "123456")
	seedStaff(t, 11, 1, "Eli", "") // no pin
	// Clock Dana in first via the punch path.
	require.Equal(t, http.StatusOK, kioskPunch(t, r, 1, 10, "123456").Code)

	req, _ := http.NewRequest(http.MethodGet, "/b/1/kiosk/roster", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), "$", "roster must be money-free")

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Roster []database.KioskStaffMember `json:"roster"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Len(t, resp.Data.Roster, 2)

	byID := map[uint]database.KioskStaffMember{}
	for _, m := range resp.Data.Roster {
		byID[m.StaffID] = m
	}
	require.True(t, byID[10].HasPin)
	require.True(t, byID[10].OnClock)
	require.False(t, byID[11].HasPin)
	require.False(t, byID[11].OnClock)
}
