package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/emails"
)

// Staff invite / resend are tenant-triggered mail to an address the tenant
// typed, so they go through the per-tenant outbound budget. The invitation row
// is already created when the send is refused, so the handler must stay 2xx,
// hand back the copyable link, and say why no email went out.
func TestStaffInviteAndResend_TenantBudgetExceededIsDisclosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	business := createOwnedBusiness(t, "0xBudgetOwner", "invite-email-budget")

	var seenBusinessIDs []uint
	budgetErr := fmt.Errorf("%w: business_daily_trial", emails.ErrTenantMailBudgetExceeded)
	orig := sendStaffInvitationEmailIfConfigured
	sendStaffInvitationEmailIfConfigured = func(businessID uint, _ []string, _, _, _, _, _ string, _ int) (bool, error) {
		seenBusinessIDs = append(seenBusinessIDs, businessID)
		return false, budgetErr
	}
	t.Cleanup(func() { sendStaffInvitationEmailIfConfigured = orig })

	body, _ := json.Marshal(map[string]string{
		"email": "budget.hire@example.com",
		"name":  "Budget Hire",
		"role":  "server",
	})
	c, w := makeTestContext("POST", "/businesses/"+business.BusinessId+"/staff/invite",
		gin.Params{{Key: "id", Value: business.BusinessId}})
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xBudgetOwner")

	InviteStaff(c)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, false, resp["email_sent"])
	require.Equal(t, "email_budget_exceeded", resp["email_error_code"])
	require.Contains(t, resp["invitation_url"], "/staff/accept-invitation?token=")

	invitationID := strconv.FormatFloat(resp["invitation_id"].(float64), 'f', 0, 64)
	c, w = makeTestContext("POST", "/businesses/"+business.BusinessId+"/staff/invitations/"+invitationID+"/resend",
		gin.Params{{Key: "id", Value: business.BusinessId}, {Key: "invitationId", Value: invitationID}})
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set("address", "0xBudgetOwner")

	ResendInvitation(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	resp = map[string]interface{}{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, false, resp["email_sent"])
	require.Equal(t, "email_budget_exceeded", resp["email_error_code"])

	require.Equal(t, []uint{business.ID, business.ID}, seenBusinessIDs,
		"both sends must be attributed to the inviting business")
}

func TestWithStaffInviteEmailError_OnlyFlagsBudget(t *testing.T) {
	plain := withStaffInviteEmailError(gin.H{"email_sent": false}, fmt.Errorf("provider outage"))
	require.NotContains(t, plain, "email_error_code", "other failures keep the existing response shape")

	ok := withStaffInviteEmailError(gin.H{"email_sent": true}, nil)
	require.NotContains(t, ok, "email_error_code")
}
