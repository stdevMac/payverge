package database

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/schema/genesis"
)

// The genesis baseline is the whole schema of a fresh install. These tests pin
// the SQL-only invariants (partial unique indexes, CHECKs, triggers) that the
// application relies on for money safety, idempotency and auth, so a
// regenerated baseline cannot silently drop one.

func genesisTableBody(t *testing.T, table string) string {
	t.Helper()
	re := regexp.MustCompile(`(?s)CREATE TABLE public\.` + regexp.QuoteMeta(table) + ` \((.*?)\n\);`)
	m := re.FindStringSubmatch(genesis.SchemaSQL)
	require.NotNilf(t, m, "genesis baseline has no table %q", table)
	return m[1]
}

func TestGenesisInvariants_NamedObjectsPresent(t *testing.T) {
	for _, name := range []string{
		// Money and idempotency guards.
		"idx_bills_active_per_table",
		"idx_bill_split_shares_payment_settled",
		"bill_split_shares_amount_cents_positive",
		"idx_alt_payments_bill_idem",
		"bills_public_token_not_blank",
		"orders_guest_request_identity_uq",
		// Cash register.
		"idx_cash_register_sessions_one_open_per_business",
		"idx_cash_register_movements_alt_payment_type_unique",
		"cash_register_movements_amount_check",
		"cash_register_sessions_status_check",
		// Fiscal worker.
		"idx_fiscal_receipts_unique_number",
		"idx_fiscal_jobs_claim",
		// Identity.
		"idx_staff_business_email_lower",
		"idx_users_email_lower_active",
		// Auth session refresh history.
		"user_session_refresh_history_token_hash_key",
		"idx_user_session_refresh_history_expires_at",
		"idx_user_sessions_cleanup_active_expiry",
		"trg_capture_user_session_refresh_history",
		"trg_default_user_session_refresh_history_expiry",
		// Email deliverability.
		"email_suppressions_email_canonical_check",
		"idx_email_delivery_events_type_received",
		// Workers.
		"report_deliveries_state_check",
		"idx_menu_extraction_jobs_claimable",
		"idx_whatsapp_business_devices_retry",
		"chk_whatsapp_business_device_status",
	} {
		require.Containsf(t, genesis.SchemaSQL, name, "genesis baseline lost invariant %q", name)
	}
}

func TestGenesisInvariants_NoAutoMigrateTableFK(t *testing.T) {
	// bills.table_id uses 0 as the "no table" sentinel for delivery/counter
	// bills, which a FK to tables(id) cannot express.
	require.NotContains(t, genesis.SchemaSQL, "fk_bills_table")
}

func TestGenesisInvariants_ColumnShapes(t *testing.T) {
	bills := genesisTableBody(t, "bills")
	require.Regexp(t, `(?m)^\s+public_token text [^\n]* NOT NULL,$`, bills)

	sessions := genesisTableBody(t, "user_sessions")
	require.Contains(t, sessions, "revoked_at timestamp with time zone,")
	require.Contains(t, sessions, "revocation_reason text")

	users := genesisTableBody(t, "users")
	require.Contains(t, users, "signup_source")
	require.Contains(t, users, "activated_at")

	businesses := genesisTableBody(t, "businesses")
	require.Contains(t, businesses, "qr_previewed_at timestamp with time zone")

	images := genesisTableBody(t, "menu_extraction_images")
	require.Contains(t, images, "mime_type character varying(32)")
	require.Contains(t, images, "storage_key")
	jobs := genesisTableBody(t, "menu_extraction_jobs")
	for _, col := range []string{"claimed_at", "claim_token", "attempt_count", "next_attempt_at"} {
		require.Contains(t, jobs, col)
	}

	// The plugins catalogue is free: no price column.
	require.NotRegexp(t, `(?m)^\s+price `, genesisTableBody(t, "plugins"))

	// Suppressions keep only metadata, never message bodies.
	suppressions := strings.ToLower(genesisTableBody(t, "email_suppressions"))
	for _, forbidden := range []string{"raw_payload", "html_body", "text_body", "subject"} {
		require.NotContains(t, suppressions, forbidden)
	}

	report := genesisTableBody(t, "report_deliveries")
	for _, col := range []string{"schedule_id", "window_start", "window_end", "lease_token", "lease_expires_at", "idempotency_key"} {
		require.Contains(t, report, col)
	}
}
