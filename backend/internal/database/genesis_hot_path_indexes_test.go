package database

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/schema/genesis"
)

// Hot-path indexes for bill, payment, order, reservation, delivery, inventory
// and staff-login reads. The genesis baseline owns them; pinning the exact
// definitions keeps a regenerated baseline from silently dropping one or
// changing its column order.
var genesisHotPathIndexes = []string{
	`CREATE INDEX idx_bills_business_created_at ON public.bills USING btree (business_id, created_at DESC);`,
	`CREATE INDEX idx_bills_business_status_created_at ON public.bills USING btree (business_id, status, created_at DESC);`,
	`CREATE INDEX idx_bills_business_closed_staff_closed_at ON public.bills USING btree (business_id, closed_by_staff_id, closed_at DESC);`,
	`CREATE INDEX idx_bills_business_closed_at ON public.bills USING btree (business_id, closed_at DESC);`,
	`CREATE INDEX idx_bills_business_updated_at ON public.bills USING btree (business_id, updated_at DESC);`,
	`CREATE INDEX idx_bills_business_table_created_at ON public.bills USING btree (business_id, table_id, created_at DESC);`,
	`CREATE INDEX idx_bills_business_counter_created_at ON public.bills USING btree (business_id, counter_id, created_at DESC);`,
	`CREATE INDEX idx_bills_bill_number_trgm ON public.bills USING gin (lower(COALESCE(bill_number, ''::text)) public.gin_trgm_ops);`,
	`CREATE INDEX idx_bills_created_at ON public.bills USING btree (created_at DESC);`,
	`CREATE INDEX idx_bills_closed_at ON public.bills USING btree (closed_at DESC);`,
	`CREATE INDEX idx_bills_updated_at ON public.bills USING btree (updated_at DESC);`,
	`CREATE INDEX idx_payments_status_confirmed_at ON public.payments USING btree (status, confirmed_at DESC);`,
	`CREATE INDEX idx_payments_status_created_at ON public.payments USING btree (status, created_at DESC);`,
	`CREATE INDEX idx_payments_status_updated_at ON public.payments USING btree (status, updated_at DESC);`,
	`CREATE INDEX idx_payments_status_reversed_at ON public.payments USING btree (status, reversed_at DESC);`,
	`CREATE INDEX idx_payments_bill_id ON public.payments USING btree (bill_id);`,
	`CREATE INDEX idx_payments_payer_addr ON public.payments USING btree (payer_addr);`,
	`CREATE INDEX idx_fiscal_receipts_business_status_created_at ON public.fiscal_receipts USING btree (business_id, status, created_at DESC);`,
	`CREATE INDEX idx_alt_payments_status_method_confirmed_at ON public.alternative_payments USING btree (status, payment_method, confirmed_at DESC);`,
	`CREATE INDEX idx_alt_payments_status_method_created_at ON public.alternative_payments USING btree (status, payment_method, created_at DESC);`,
	`CREATE INDEX idx_alt_payments_bill_id ON public.alternative_payments USING btree (bill_id);`,
	`CREATE INDEX idx_business_milestone_status_created_at ON public.business_milestone_events USING btree (status, created_at);`,
	`CREATE INDEX idx_business_milestone_status_updated_at ON public.business_milestone_events USING btree (status, updated_at);`,
	`CREATE INDEX idx_orders_business_status_created_at ON public.orders USING btree (business_id, status, created_at DESC);`,
	`CREATE INDEX idx_orders_business_created_at ON public.orders USING btree (business_id, created_at DESC);`,
	`CREATE INDEX idx_orders_bill_created_at ON public.orders USING btree (bill_id, created_at DESC);`,
	`CREATE INDEX idx_table_reservations_table_status_time ON public.table_reservations USING btree (table_id, status, reservation_time);`,
	`CREATE INDEX idx_table_reservations_business_time ON public.table_reservations USING btree (business_id, reservation_time);`,
	`CREATE INDEX idx_table_reservations_reminder_scan ON public.table_reservations USING btree (business_id, reminder_sent, status, reservation_time);`,
	`CREATE INDEX idx_delivery_orders_business_status_created_at ON public.delivery_orders USING btree (business_id, status, created_at DESC);`,
	`CREATE INDEX idx_delivery_orders_business_updated_at ON public.delivery_orders USING btree (business_id, updated_at DESC);`,
	`CREATE INDEX idx_delivery_orders_driver_business_assigned_created_at ON public.delivery_orders USING btree (driver_id, business_id, assigned_at DESC, created_at DESC);`,
	`CREATE INDEX idx_inventory_items_business_active_name ON public.inventory_items USING btree (business_id, is_active, name);`,
	`CREATE INDEX idx_inventory_recipes_business_menu_item ON public.inventory_recipes USING btree (business_id, menu_item_id);`,
	`CREATE INDEX idx_inventory_recipes_business_inventory_item ON public.inventory_recipes USING btree (business_id, inventory_item_id);`,
	`CREATE INDEX idx_inventory_movements_business_created_at ON public.inventory_movements USING btree (business_id, created_at DESC);`,
	`CREATE INDEX idx_inventory_movements_business_order_type ON public.inventory_movements USING btree (business_id, reference_order_id, movement_type);`,
	`CREATE INDEX idx_staff_login_codes_code ON public.staff_login_codes USING btree (code);`,
	`CREATE INDEX idx_staff_login_codes_staff_code ON public.staff_login_codes USING btree (staff_id, code);`,
	`CREATE INDEX idx_staff_login_codes_expires_at ON public.staff_login_codes USING btree (expires_at);`,
}

func TestGenesisOwnsHotPathIndexes(t *testing.T) {
	for _, stmt := range genesisHotPathIndexes {
		require.Containsf(t, genesis.SchemaSQL, stmt+"\n", "genesis baseline lost or changed %q", stmt)
	}
}

func TestGenesisHasNoPaymentMethodOnlyIndex(t *testing.T) {
	require.NotContains(t, genesis.SchemaSQL, "idx_payments_payment_method ")
}

// Login codes are scoped per staff member: two staff members may hold the same
// six-digit code at once, so no unique index on code alone may exist.
func TestGenesisStaffLoginCodeIsNotGloballyUnique(t *testing.T) {
	re := regexp.MustCompile(`(?m)^CREATE UNIQUE INDEX \S+ ON public\.staff_login_codes USING btree \(code\);$`)
	require.False(t, re.MatchString(genesis.SchemaSQL))
	require.NotRegexp(t, `(?s)CREATE TABLE public\.staff_login_codes \(.*?code [^\n]*UNIQUE`, genesis.SchemaSQL)
	require.NotContains(t, genesis.SchemaSQL, "staff_login_codes_code_key")
}

func TestBillStatusPartialConstant(t *testing.T) {
	require.Equal(t, BillStatus("partial"), BillStatusPartial)
}
