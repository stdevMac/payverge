package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
)

const storedReservationPartnerLinks = `[{"name":"OpenTable","provider_key":"opentable","url":"https://www.opentable.com/r/venue"}]`

const changedReservationPartnerLinks = `{"external_partner_links":[{"name":"Uber Eats","provider_key":"ubereats","url":"https://www.ubereats.com/store/x"}]}`

func putReservationSettings(t *testing.T, businessID uint, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", businessID)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	UpdateReservationSettings(c)
	return w
}

func seedReservationPartnerLinks(t *testing.T, businessID uint) {
	t.Helper()
	require.NoError(t, database.GetDB().AutoMigrate(&database.ReservationSettings{}))
	require.NoError(t, database.GetDB().Create(&database.ReservationSettings{
		BusinessID:            businessID,
		Enabled:               true,
		MaxAdvanceDays:        30,
		MinAdvanceMinutes:     30,
		MinPartySize:          1,
		MaxPartySize:          20,
		DefaultDuration:       120,
		SlotIntervalMinutes:   30,
		ServiceBufferMinutes:  15,
		AutoAssignTables:      true,
		ApprovalMode:          database.ReservationApprovalAuto,
		AllowWaitlist:         true,
		HoldDurationMinutes:   15,
		AllowCancellation:     true,
		CancellationDeadline:  24,
		NoShowGraceMinutes:    15,
		SendConfirmationEmail: true,
		SendReminderEmail:     true,
		ReminderHoursBefore:   24,
		ExternalPartnerLinks:  database.JSONRawMessage(storedReservationPartnerLinks),
	}).Error)
}

func reservationPartnerLinksJSON(t *testing.T, businessID uint) string {
	t.Helper()
	var row database.ReservationSettings
	require.NoError(t, database.GetDB().Where("business_id = ?", businessID).First(&row).Error)
	return string(row.ExternalPartnerLinks)
}

func TestUpdateReservationSettings_DemoModeFreezesPartnerLinks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	config.SetDemoModeForTesting(t, true)
	biz := createOwnedBusiness(t, "0xOwnerA", "reservation-partner-freeze")
	seedReservationPartnerLinks(t, biz.ID)

	changed := putReservationSettings(t, biz.ID, changedReservationPartnerLinks)
	require.Equal(t, http.StatusForbidden, changed.Code, changed.Body.String())
	assert.Contains(t, changed.Body.String(), demomode.ErrorCode)
	assert.Contains(t, changed.Body.String(), string(demomode.KindStorefront))
	stored := reservationPartnerLinksJSON(t, biz.ID)
	assert.Contains(t, stored, "https://www.opentable.com/r/venue")
	assert.NotContains(t, stored, "ubereats.com")

	unchanged := putReservationSettings(t, biz.ID, `{"external_partner_links":`+storedReservationPartnerLinks+`}`)
	require.Equal(t, http.StatusOK, unchanged.Code, unchanged.Body.String())
	assert.Contains(t, reservationPartnerLinksJSON(t, biz.ID), "https://www.opentable.com/r/venue")

	omitted := putReservationSettings(t, biz.ID, `{"max_advance_days":45}`)
	require.Equal(t, http.StatusOK, omitted.Code, omitted.Body.String())
	assert.Contains(t, reservationPartnerLinksJSON(t, biz.ID), "https://www.opentable.com/r/venue")
	assert.NotContains(t, reservationPartnerLinksJSON(t, biz.ID), "ubereats.com")
}

func TestUpdateReservationSettings_DemoModeOffSavesPartnerLinks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	config.SetDemoModeForTesting(t, false)
	biz := createOwnedBusiness(t, "0xOwnerA", "reservation-partner-save")
	seedReservationPartnerLinks(t, biz.ID)

	saved := putReservationSettings(t, biz.ID, changedReservationPartnerLinks)
	require.Equal(t, http.StatusOK, saved.Code, saved.Body.String())
	assert.Contains(t, reservationPartnerLinksJSON(t, biz.ID), "https://www.ubereats.com/store/x")
	assert.Contains(t, saved.Body.String(), "https://www.ubereats.com/store/x")
}
