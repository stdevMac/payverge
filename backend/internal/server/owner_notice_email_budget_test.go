package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
)

// ownerNoticeBudget records every tenant-budget claim and refuses all of them,
// the same answer a business whose daily cap is 0 (or already spent) gets.
type ownerNoticeBudget struct {
	mu     sync.Mutex
	claims []emails.TenantMailClaimRequest
	allow  bool
}

func (b *ownerNoticeBudget) Claim(_ context.Context, req emails.TenantMailClaimRequest) (emails.TenantMailClaim, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.claims = append(b.claims, req)
	if b.allow {
		return emails.TenantMailClaim{}, nil
	}
	return emails.TenantMailClaim{}, fmt.Errorf("%w: business_daily_standard", emails.ErrTenantMailBudgetExceeded)
}

func (b *ownerNoticeBudget) Release(context.Context, emails.TenantMailClaim) error { return nil }

func (b *ownerNoticeBudget) claimedFor(businessID uint, recipient string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, req := range b.claims {
		if req.BusinessID != businessID {
			continue
		}
		for _, to := range req.Recipients {
			if to == recipient {
				n++
			}
		}
	}
	return n
}

// ownerNoticeProvider records what actually reached the provider.
type ownerNoticeProvider struct {
	mu sync.Mutex
	to []string
}

func (p *ownerNoticeProvider) Send(_ context.Context, msg emails.EmailMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.to = append(p.to, msg.To...)
	return nil
}

func (p *ownerNoticeProvider) delivered() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.to...)
}

func installOwnerNoticeEmailServer(t *testing.T, budget emails.TenantMailBudget) *ownerNoticeProvider {
	t.Helper()
	prev := emails.EmailServerInstance
	t.Cleanup(func() { emails.EmailServerInstance = prev })
	provider := &ownerNoticeProvider{}
	server, err := emails.NewEmailServer(provider, "noreply@example.com", "updates@example.com", reservationEmailTemplatesRoot(t))
	require.NoError(t, err)
	server.SetTenantMailBudget(budget)
	return provider
}

const ownerNoticeContact = "venue-contact@example.test"

func setOwnerNoticeContact(t *testing.T, business *database.Business) {
	t.Helper()
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).Update("email", ownerNoticeContact).Error)
	business.Email = ownerNoticeContact
}

// Review M1: business.Email is tenant-typed and unverified, so the notices a
// tenant action sends to it (staff added, staff removed, wallet changed) are
// tenant mail. With the business's budget spent none of them may reach the
// provider; before the fix they were sent as uncounted system mail.
func TestOwnerNotices_CountAgainstTenantBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("staff added is skipped when the invite was refused", func(t *testing.T) {
		setupStaffHandlerTestDB(t)
		business := createOwnedBusiness(t, "0xNoticeOwner", "notice-invite")
		setOwnerNoticeContact(t, business)
		budget := &ownerNoticeBudget{}
		provider := installOwnerNoticeEmailServer(t, budget)

		body, _ := json.Marshal(map[string]string{"email": "hire@example.test", "name": "Hire", "role": "server"})
		c, w := makeTestContext("POST", "/", gin.Params{{Key: "id", Value: business.BusinessId}})
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("address", "0xNoticeOwner")

		InviteStaff(c)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "email_budget_exceeded")
		require.Empty(t, provider.delivered(), "no mail may leave once the tenant budget is spent")
		require.Equal(t, 1, budget.claimedFor(business.ID, "hire@example.test"), "the invite is claimed once")
		require.Zero(t, budget.claimedFor(business.ID, ownerNoticeContact),
			"an 'invitation sent' notice for an invite that was refused is not even attempted")
	})

	t.Run("staff added is tenant mail when the invite goes out", func(t *testing.T) {
		setupStaffHandlerTestDB(t)
		business := createOwnedBusiness(t, "0xNoticeOwner", "notice-invite-ok")
		setOwnerNoticeContact(t, business)
		budget := &ownerNoticeBudget{allow: true}
		provider := installOwnerNoticeEmailServer(t, budget)

		body, _ := json.Marshal(map[string]string{"email": "hire@example.test", "name": "Hire", "role": "server"})
		c, w := makeTestContext("POST", "/", gin.Params{{Key: "id", Value: business.BusinessId}})
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("address", "0xNoticeOwner")

		InviteStaff(c)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		require.ElementsMatch(t, []string{"hire@example.test", ownerNoticeContact}, provider.delivered())
		require.Equal(t, 1, budget.claimedFor(business.ID, ownerNoticeContact),
			"the owner notice is claimed against the inviting business")
	})

	t.Run("staff removed", func(t *testing.T) {
		setupStaffHandlerTestDB(t)
		business := createOwnedBusiness(t, "0xNoticeOwner", "notice-remove")
		setOwnerNoticeContact(t, business)
		staff := createStaffMember(t, business.ID, "leaver@example.test", "Leaver")
		budget := &ownerNoticeBudget{}
		provider := installOwnerNoticeEmailServer(t, budget)

		c, w := makeTestContext("DELETE", "/", gin.Params{
			{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
			{Key: "staffId", Value: fmt.Sprintf("%d", staff.ID)},
		})
		c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
		c.Set("address", "0xNoticeOwner")

		RemoveStaff(c)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Empty(t, provider.delivered(), "no mail may leave once the tenant budget is spent")
		require.Equal(t, 1, budget.claimedFor(business.ID, "leaver@example.test"))
		require.Equal(t, 1, budget.claimedFor(business.ID, ownerNoticeContact),
			"the staff-removed notice is claimed against the business")
	})

	t.Run("wallet changed", func(t *testing.T) {
		migrateWalletRotationTables(t, setupStaffHandlerTestDB(t))
		business := createBusinessHandlerTestBusiness(t, "0xNoticeOwner", "biz-notice-wallet")
		setOwnerNoticeContact(t, business)
		budget := &ownerNoticeBudget{}
		provider := installOwnerNoticeEmailServer(t, budget)

		body, _ := json.Marshal(map[string]string{"settlement_address": "0x3333333333333333333333333333333333333333"})
		c, w := makeTestContext("PUT", "/", gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}})
		c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("address", "0xNoticeOwner")

		UpdateBusiness(c)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Empty(t, provider.delivered(), "no mail may leave once the tenant budget is spent")
		require.Equal(t, 1, budget.claimedFor(business.ID, ownerNoticeContact),
			"the wallet-change notice is claimed against the business")
	})
}
