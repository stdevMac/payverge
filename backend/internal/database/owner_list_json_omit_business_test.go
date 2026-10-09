package database

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FIND-037: hot owner list models must not embed zero-value Business blobs.
// encoding/json never omits non-pointer structs even with omitempty, so a bare
// Business association dumped ~3KB of empty fields per row on staff/CRM/reservations.
func TestOwnerListModels_OmitBusinessFromJSON(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		v    any
	}{
		// Full staff rows (created_at set) keep business_id for list/session clients.
		{"staff", Staff{ID: 1, BusinessID: 50, Email: "a@b.c", Name: "Maya", Role: StaffRoleManager, InvitedBy: "owner", CreatedAt: now, UpdatedAt: now}},
		{"staff_invitation", StaffInvitation{ID: 1, BusinessID: 50, Email: "a@b.c", Name: "New", Role: StaffRoleServer, Token: "secret-must-not-serialize", InvitedBy: "owner"}},
		{"staff_permission_deny", StaffPermissionDeny{ID: 1, BusinessID: 50, StaffID: 1, Permission: "bills:void"}},
		{"customer_business", CustomerBusiness{ID: 1, CustomerID: 1, BusinessID: 50, TotalSpent: 140}},
		{"table_reservation", TableReservation{ID: 1, BusinessID: 50, CustomerName: "Ada", PartySize: 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.v)
			require.NoError(t, err)
			var m map[string]any
			require.NoError(t, json.Unmarshal(raw, &m))
			_, has := m["business"]
			assert.False(t, has, "%s JSON must not embed business (got keys %v)", tc.name, keysOf(m))
			assert.Contains(t, m, "business_id")
			if tc.name == "staff_invitation" {
				_, hasToken := m["token"]
				assert.False(t, hasToken, "StaffInvitation.Token must never serialize (got keys %v)", keysOf(m))
			}
		})
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestDeliveryAlertsAccounting_OmitBusinessFromJSON(t *testing.T) {
	driver := DeliveryDriver{ID: 1, BusinessID: 50, Name: "Dee", Phone: "1"}
	raw, err := json.Marshal(driver)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	_, has := m["business"]
	assert.False(t, has, "driver must not embed business")

	order := DeliveryOrder{ID: 1, BusinessID: 50, CustomerName: "Ada"}
	raw, err = json.Marshal(order)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &m))
	// Custom MarshalJSON may omit empty business pointer
	assert.NotContains(t, m, "business")

	alert := OperationalAlert{ID: 1, BusinessID: 50, Title: "New order", AlertType: OperationalAlertTypeOrderNew, ResourceType: OperationalAlertResourceTypeOrder, ResourceID: 1}
	raw, err = json.Marshal(alert)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &m))
	assert.NotContains(t, m, "business")

	entry := ManualLedgerEntry{ID: 1, BusinessID: 50, Amount: 185000, Currency: "USD", Description: "rent", Category: "rent", EntryType: AccountingEntryTypeExpense}
	raw, err = json.Marshal(entry)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &m))
	assert.NotContains(t, m, "business")
	assert.Equal(t, 1850.0, m["amount"], "amount must still marshal as dollars")
}
