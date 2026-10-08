package emails

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/structs"
)

// The delivery mail to the address on a delivery order reaches the provider
// through EmailServerDispatcher, not through a For* helper. Before this was
// wired, ForDeliveryOrder existed but nothing used it, so every delivery mail
// (including the order-received confirmation an anonymous public checkout
// sends to whatever address it typed) bypassed the tenant budget.
func TestEmailServerDispatcher_ClaimsNotificationMailOrigin(t *testing.T) {
	db := tenantBudgetTestDB(t)
	seedBudgetBusiness(t, db, 7, false, "real")
	// Caps far above what the test sends, so each scope writes its counter;
	// only the lane's per-recipient cap is tight.
	cfg := unlimitedBudgetConfig()
	cfg.BusinessDailyCap = 100
	cfg.RecipientDailyCap = 100
	cfg.RecipientGlobalDailyCap = 100
	cfg.ReservationRequestDailyCap = 100
	cfg.ReservationRequestRecipientDailyCap = 1
	budget, _ := newTestBudget(t, db, cfg)
	provider := &budgetProviderStub{}
	dispatcher := NewEmailServerDispatcher(newBudgetTestServer(t, provider, budget))
	guest := structs.User{Email: "typed-at-checkout@example.test"}

	notify := func(title string, origin structs.NotificationMailOrigin) {
		n := structs.NewNotification(title, title+" body", 0)
		n.MailOrigin = origin
		dispatcher.DispatchNotification(n, guest, "")
	}

	// The public checkout's confirmation is lane mail.
	notify("Order received A", structs.NotificationMailOrigin{BusinessID: 7, DeliveryOrderID: 1, Purpose: MailPurposeGuestDeliveryRequest})
	require.Equal(t, 1, provider.calls)
	require.EqualValues(t, 1, ledgerCount(t, db, tenantMailKindBusinessDay))
	require.EqualValues(t, 1, ledgerCount(t, db, tenantMailKindReservationRequestDay), "counted in the guest lane")
	require.EqualValues(t, 1, ledgerCount(t, db, tenantMailKindLaneRecipientGlobalDay), "and on the lane cross-tenant recipient key")

	// A second anonymous checkout typing the same address is over the lane's
	// per-recipient cap: refused, never delivered, nothing left on the ledger.
	notify("Order received B", structs.NotificationMailOrigin{BusinessID: 7, DeliveryOrderID: 2, Purpose: MailPurposeGuestDeliveryRequest})
	require.Equal(t, 1, provider.calls, "a refused notification never reaches the provider")
	require.EqualValues(t, 1, ledgerCount(t, db, tenantMailKindBusinessDay), "the refused claim rolled back")

	// The venue's own follow-up (accepted, status, cancelled) is operator mail:
	// counted against the business, untouched by the lane caps.
	notify("Order accepted", structs.NotificationMailOrigin{BusinessID: 7, DeliveryOrderID: 1})
	require.Equal(t, 2, provider.calls, "operator mail is not starved by the guest lane")
	require.EqualValues(t, 2, ledgerCount(t, db, tenantMailKindBusinessDay))
	require.EqualValues(t, 1, ledgerCount(t, db, tenantMailKindReservationRequestDay))

	// An unstamped notification stays system mail.
	notify("Platform notice", structs.NotificationMailOrigin{})
	require.Equal(t, 3, provider.calls)
	require.EqualValues(t, 2, ledgerCount(t, db, tenantMailKindBusinessDay), "system mail is never budgeted")
}

func TestEmailServerDispatcher_OriginDoesNotLeakIntoTheSharedServer(t *testing.T) {
	db := tenantBudgetTestDB(t)
	seedBudgetBusiness(t, db, 7, false, "real")
	budget, _ := newTestBudget(t, db, unlimitedBudgetConfig())
	server := newBudgetTestServer(t, &budgetProviderStub{}, budget)
	dispatcher := NewEmailServerDispatcher(server)
	n := structs.NewNotification("Order received", "body", 0)
	n.MailOrigin = structs.NotificationMailOrigin{BusinessID: 7, DeliveryOrderID: 1, Purpose: MailPurposeGuestDeliveryRequest}
	dispatcher.DispatchNotification(n, structs.User{Email: "guest@example.test"}, "")
	require.Equal(t, EmailOrigin{}, server.origin, "the dispatcher stamps a clone, never the shared server")
}

// BenchmarkDispatchNotificationTenantBudget measures delivery mail through
// EmailServerDispatcher with the real budget attached (caps high enough never
// to trip; provider stubbed; SQLite; templates cached as in production).
//
//   - unstamped: the notification carries no MailOrigin. This is exactly what
//     every delivery notification did before it was stamped, so it is the
//     baseline.
//   - delivery-operator: a venue action's mail to the order address
//     (accepted, status, cancelled, paid).
//   - delivery-guest-lane: the order-received confirmation of an anonymous
//     public checkout.
func BenchmarkDispatchNotificationTenantBudget(b *testing.B) {
	for _, mode := range []string{"unstamped", "delivery-operator", "delivery-guest-lane"} {
		b.Run(mode, func(b *testing.B) {
			db := tenantBudgetTestDB(b)
			seedBudgetBusiness(b, db, 7, false, "real")
			cfg := DefaultTenantMailBudgetConfig()
			cfg.BusinessDailyCap = 1 << 30
			cfg.RecipientDailyCap = 1 << 30
			cfg.RecipientGlobalDailyCap = 1 << 30
			cfg.ReservationRequestDailyCap = 1 << 30
			cfg.ReservationRequestRecipientDailyCap = 1 << 30
			server := newBudgetTestServer(b, &budgetProviderStub{}, NewGormTenantMailBudget(db, cfg))
			// Production renders from templates parsed once at startup; outside
			// production mode every render re-reads the template tree from
			// disk, which would swamp the budget delta this measures.
			server.templates.debug = false
			dispatcher := NewEmailServerDispatcher(server)
			var origin structs.NotificationMailOrigin
			switch mode {
			case "delivery-operator":
				origin = structs.NotificationMailOrigin{BusinessID: 7, DeliveryOrderID: 1}
			case "delivery-guest-lane":
				origin = structs.NotificationMailOrigin{BusinessID: 7, DeliveryOrderID: 1, Purpose: MailPurposeGuestDeliveryRequest}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				n := structs.NewNotification("Order received", "We received your order.", 0)
				n.MailOrigin = origin
				dispatcher.DispatchNotification(n, structs.User{Email: fmt.Sprintf("guest%d@example.test", i)}, "")
			}
		})
	}
}
