package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
)

// IssueDemoStaffSession signs staff in exactly like a staff login code would
// (same session class, cookies and response), for the public demo's one-click
// "Enter demo as staff" buttons. The session carries the demo_staff provider
// so it dies when DEMO_MODE is switched off and a refresh-token replay on it
// revokes only that one session. Callers must have checked DEMO_MODE.
func IssueDemoStaffSession(c *gin.Context, staff *database.Staff) error {
	if staff == nil || staff.ID == 0 || !staff.IsActive {
		return errors.New("demo staff unavailable")
	}
	prepared, err := prepareStaffSession(c, staff, demomode.StaffSessionProvider)
	if err != nil {
		return err
	}
	finishStaffLoginResponse(c, staff, prepared)
	return nil
}

// demoTableLimit bounds GET /api/v1/demo/tables per venue.
const demoTableLimit = 6

type demoTable struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

type demoVenue struct {
	Name      string      `json:"name"`
	CustomURL string      `json:"custom_url"`
	Tables    []demoTable `json:"tables"`
}

// GetDemoTables lists a few table codes of each public-demo venue so the demo
// banner can offer "try it as a guest" links and QR codes. 404 unless
// DEMO_MODE: a normal install never advertises its table codes.
func GetDemoTables(c *gin.Context) {
	if !config.DemoModeEnabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
		return
	}
	db := database.GetDB()
	ctx := c.Request.Context()
	owner, err := demomode.FindShowroomOwner(ctx, db)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"venues": []demoVenue{}})
		return
	}
	venues, err := demomode.ShowroomVenues(ctx, db, owner.ID)
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not load the demo tables")
		return
	}
	out := make([]demoVenue, 0, len(venues))
	for _, v := range venues {
		var tables []database.Table
		if err := db.WithContext(ctx).
			Select("name", "table_code").
			Where("business_id = ? AND is_active = ?", v.ID, true).
			Order("id").Limit(demoTableLimit).Find(&tables).Error; err != nil {
			RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not load the demo tables")
			return
		}
		dv := demoVenue{Name: v.Name, CustomURL: strings.TrimSpace(v.CustomURL), Tables: make([]demoTable, 0, len(tables))}
		for _, t := range tables {
			dv.Tables = append(dv.Tables, demoTable{Name: t.Name, Code: t.TableCode})
		}
		out = append(out, dv)
	}
	c.Header("Cache-Control", "public, max-age=60")
	c.JSON(http.StatusOK, gin.H{"venues": out})
}
