//go:build integration_postgres

package database

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/testperf"
)

// Schema fingerprint dimensions that are empty in the genesis baseline (no
// enums, matviews, or partitions). They are still PRESENT in
// every SchemaFingerprint map as empty strings so a future addition is caught
// as a dimension delta rather than silently ignored. Mutation subtests below
// exercise the non-empty dimensions; emptiness of these is asserted in the
// genesis-equals-reference path via DiffFingerprints (both sides empty).
//
// Empty-today dimensions: enums, matviews. Refresh-token replay history
// has an application trigger for mixed-version writer compatibility.
// (Partitions are not a separate dimension; partitioned tables would appear
// under tables/indexes/constraints if introduced.)

// TestSchemaFingerprint_GenesisEqualsReference compares the genesis bootstrap
// with the reconciled reference (genesis plus every numbered migration in
// backend/migrations). The baseline is regenerated with every migration, so
// both sides apply the same SQL and the diff stays empty unless someone lands
// a migration without regenerating genesis. The rest of the value of this
// test is the tripwires below (pinned views, empty dimensions, required
// functions/triggers/extensions);
// TestSchemaFingerprint_MutationDetection proves the fingerprint itself
// detects changes.
func TestSchemaFingerprint_GenesisEqualsReference(t *testing.T) {
	ctx := context.Background()

	// Fixture A: genesis-bootstrap from the embedded dump.
	pgA, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container A: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pgA.Terminate(ctx) })

	require.NoError(t, BootstrapGenesisSchema(pgA.DB), "BootstrapGenesisSchema")
	fpA, err := SchemaFingerprint(pgA.DB)
	require.NoError(t, err, "SchemaFingerprint genesis")
	shaA, err := SchemaFingerprintSHA(pgA.DB)
	require.NoError(t, err, "SchemaFingerprintSHA genesis")

	// Fixture B: full production reconciled reference.
	pgB, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container B: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pgB.Terminate(ctx) })

	prev := db
	t.Cleanup(func() { SetTestDB(prev) })
	require.NoError(t,
		ReconcileReferenceSchema(pgB.DB, pgB.DSN, freshDBMigrationsDir(t)),
		"ReconcileReferenceSchema")

	fpB, err := SchemaFingerprint(pgB.DB)
	require.NoError(t, err, "SchemaFingerprint reference")
	shaB, err := SchemaFingerprintSHA(pgB.DB)
	require.NoError(t, err, "SchemaFingerprintSHA reference")

	// Core invariant: genesis dump was generated FROM the reconciled reference.
	// Do NOT compare against version.json's schema_sha256 — that hashes pg_dump
	// TEXT, a different artifact.
	diff := DiffFingerprints(fpA, fpB)
	require.Empty(t, diff, "genesis fingerprint must equal reconciled reference:\n%s", diff)
	require.Equal(t, shaA, shaB, "SchemaFingerprintSHA must match when dimensions match")

	// Empty-dimension tripwires: still present, both empty today.
	for _, dim := range []string{"enums", "matviews"} {
		_, okA := fpA[dim]
		_, okB := fpB[dim]
		require.Truef(t, okA && okB, "dimension %q must be present", dim)
		require.Emptyf(t, fpA[dim], "dimension %q expected empty on HEAD (tripwire)", dim)
		require.Emptyf(t, fpB[dim], "dimension %q expected empty on HEAD (tripwire)", dim)
	}

	// views tripwire: exactly one intentional view in the schema —
	// payment_events (part of the genesis baseline). A new view must be pinned
	// here explicitly. The
	// fingerprint value embeds pretty-printed multi-line viewdefs, so count
	// views in the catalog rather than parsing the joined string.
	for fixture, gdb := range map[string]*gorm.DB{"genesis": pgA.DB, "reference": pgB.DB} {
		var viewNames []string
		require.NoError(t, gdb.Raw(`
			SELECT c.relname
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public'
			  AND c.relkind = 'v'
			  AND c.relname NOT LIKE 'whatsmeow_%'
			ORDER BY c.relname`).Scan(&viewNames).Error)
		require.Equalf(t, []string{"payment_events"}, viewNames,
			"exactly one intentional view (payment_events) expected on HEAD (%s fixture tripwire)", fixture)
	}
	require.True(t, strings.HasPrefix(fpA["views"], "payment_events|"),
		`dimension "views" must fingerprint the payment_events definition`)

	// Non-empty sanity: tables/functions/extensions must exist.
	require.NotEmpty(t, fpA["tables"])
	require.Contains(t, fpA["functions"], "capture_user_session_refresh_history")
	require.Contains(t, fpA["triggers"], "trg_capture_user_session_refresh_history")
	require.Contains(t, fpA["extensions"], "pg_trgm")
}

func TestSchemaFingerprint_MutationDetection(t *testing.T) {
	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	require.NoError(t, BootstrapGenesisSchema(pg.DB))

	baseline, err := SchemaFingerprint(pg.DB)
	require.NoError(t, err)
	baselineSHA, err := SchemaFingerprintSHA(pg.DB)
	require.NoError(t, err)
	require.NotEmpty(t, baselineSHA)

	// Each subtest mutates ONE catalog fact on the shared genesis DB, then
	// restores it so subsequent subtests start from a clean baseline.
	// Targets are real objects present in the genesis current_schema.sql.

	t.Run("column_default", func(t *testing.T) {
		// businesses.kind DEFAULT 'real'::text
		require.NoError(t, pg.DB.Exec(
			`ALTER TABLE public.businesses ALTER COLUMN kind SET DEFAULT 'demo'::text`,
		).Error)
		t.Cleanup(func() {
			_ = pg.DB.Exec(`ALTER TABLE public.businesses ALTER COLUMN kind SET DEFAULT 'real'::text`).Error
		})

		after, err := SchemaFingerprint(pg.DB)
		require.NoError(t, err)
		afterSHA, err := SchemaFingerprintSHA(pg.DB)
		require.NoError(t, err)

		diff := DiffFingerprints(baseline, after)
		require.NotEmpty(t, diff, "expected fingerprint change after DEFAULT mutation")
		require.Contains(t, diff, `dimension "columns"`, "expected columns dimension to change:\n%s", diff)
		require.NotEqual(t, baselineSHA, afterSHA, "SHA must change when columns change")
		require.NotEqual(t, baseline["columns"], after["columns"])
	})

	t.Run("index_predicate", func(t *testing.T) {
		// Partial unique index with a WHERE clause — drop and recreate with a
		// different predicate so pg_get_indexdef changes.
		require.NoError(t, pg.DB.Exec(
			`DROP INDEX IF EXISTS public.idx_bills_active_per_table`,
		).Error)
		require.NoError(t, pg.DB.Exec(`
			CREATE UNIQUE INDEX idx_bills_active_per_table
			ON public.bills USING btree (table_id)
			WHERE ((table_id IS NOT NULL) AND (table_id <> 0) AND (status = 'open'::text))
		`).Error)
		t.Cleanup(func() {
			_ = pg.DB.Exec(`DROP INDEX IF EXISTS public.idx_bills_active_per_table`).Error
			_ = pg.DB.Exec(`
				CREATE UNIQUE INDEX idx_bills_active_per_table
				ON public.bills USING btree (table_id)
				WHERE ((table_id IS NOT NULL) AND (table_id <> 0) AND (status = ANY (ARRAY['open'::text, 'partial'::text])))
			`).Error
		})

		after, err := SchemaFingerprint(pg.DB)
		require.NoError(t, err)
		afterSHA, err := SchemaFingerprintSHA(pg.DB)
		require.NoError(t, err)

		diff := DiffFingerprints(baseline, after)
		require.NotEmpty(t, diff)
		require.Contains(t, diff, `dimension "indexes"`, "expected indexes dimension:\n%s", diff)
		require.NotEqual(t, baselineSHA, afterSHA)
		require.NotEqual(t, baseline["indexes"], after["indexes"])
	})

	t.Run("fk_on_delete", func(t *testing.T) {
		// fk_ai_waiter_conversations_business has no ON DELETE action by default;
		// re-add with CASCADE so pg_get_constraintdef changes.
		require.NoError(t, pg.DB.Exec(
			`ALTER TABLE public.ai_waiter_conversations DROP CONSTRAINT fk_ai_waiter_conversations_business`,
		).Error)
		require.NoError(t, pg.DB.Exec(`
			ALTER TABLE public.ai_waiter_conversations
			ADD CONSTRAINT fk_ai_waiter_conversations_business
			FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE
		`).Error)
		t.Cleanup(func() {
			_ = pg.DB.Exec(`ALTER TABLE public.ai_waiter_conversations DROP CONSTRAINT IF EXISTS fk_ai_waiter_conversations_business`).Error
			_ = pg.DB.Exec(`
				ALTER TABLE public.ai_waiter_conversations
				ADD CONSTRAINT fk_ai_waiter_conversations_business
				FOREIGN KEY (business_id) REFERENCES public.businesses(id)
			`).Error
		})

		after, err := SchemaFingerprint(pg.DB)
		require.NoError(t, err)
		afterSHA, err := SchemaFingerprintSHA(pg.DB)
		require.NoError(t, err)

		diff := DiffFingerprints(baseline, after)
		require.NotEmpty(t, diff)
		require.Contains(t, diff, `dimension "constraints"`, "expected constraints dimension:\n%s", diff)
		require.NotEqual(t, baselineSHA, afterSHA)
		require.NotEqual(t, baseline["constraints"], after["constraints"])
		require.True(t,
			strings.Contains(after["constraints"], "ON DELETE CASCADE") ||
				strings.Contains(diff, "CASCADE"),
			"expected CASCADE to appear in constraint fingerprint:\n%s", after["constraints"])
	})

	t.Run("drop_not_null", func(t *testing.T) {
		// businesses.name is NOT NULL in the baseline.
		require.NoError(t, pg.DB.Exec(
			`ALTER TABLE public.businesses ALTER COLUMN name DROP NOT NULL`,
		).Error)
		t.Cleanup(func() {
			_ = pg.DB.Exec(`ALTER TABLE public.businesses ALTER COLUMN name SET NOT NULL`).Error
		})

		after, err := SchemaFingerprint(pg.DB)
		require.NoError(t, err)
		afterSHA, err := SchemaFingerprintSHA(pg.DB)
		require.NoError(t, err)

		diff := DiffFingerprints(baseline, after)
		require.NotEmpty(t, diff)
		require.Contains(t, diff, `dimension "columns"`, "expected columns dimension:\n%s", diff)
		require.NotEqual(t, baselineSHA, afterSHA)
		require.NotEqual(t, baseline["columns"], after["columns"])
	})

	t.Run("numeric_precision", func(t *testing.T) {
		// bill_items.price is numeric(10,2) — widen precision.
		require.NoError(t, pg.DB.Exec(
			`ALTER TABLE public.bill_items ALTER COLUMN price TYPE numeric(12,2)`,
		).Error)
		t.Cleanup(func() {
			_ = pg.DB.Exec(`ALTER TABLE public.bill_items ALTER COLUMN price TYPE numeric(10,2)`).Error
		})

		after, err := SchemaFingerprint(pg.DB)
		require.NoError(t, err)
		afterSHA, err := SchemaFingerprintSHA(pg.DB)
		require.NoError(t, err)

		diff := DiffFingerprints(baseline, after)
		require.NotEmpty(t, diff)
		require.Contains(t, diff, `dimension "columns"`, "expected columns dimension:\n%s", diff)
		require.NotEqual(t, baselineSHA, afterSHA)
		require.NotEqual(t, baseline["columns"], after["columns"])
	})

	t.Run("check_predicate", func(t *testing.T) {
		// End-to-end guard that the fpConstraints cast-canonicalizer does not
		// blind the constraints dimension to a REAL CHECK-predicate change.
		// Change an allowed value in the cash_register movement_type CHECK — the
		// exact constraint whose cast spelling the canonicalizer erases. The new
		// literal ('cash_frozen') survives canonicalization, so the diff must fire.
		require.NoError(t, pg.DB.Exec(
			`ALTER TABLE public.cash_register_movements DROP CONSTRAINT cash_register_movements_type_check`,
		).Error)
		require.NoError(t, pg.DB.Exec(`
			ALTER TABLE public.cash_register_movements
			ADD CONSTRAINT cash_register_movements_type_check
			CHECK (movement_type IN ('cash_sale', 'cash_refund', 'cash_in', 'cash_frozen'))
		`).Error)
		t.Cleanup(func() {
			_ = pg.DB.Exec(`ALTER TABLE public.cash_register_movements DROP CONSTRAINT IF EXISTS cash_register_movements_type_check`).Error
			_ = pg.DB.Exec(`
				ALTER TABLE public.cash_register_movements
				ADD CONSTRAINT cash_register_movements_type_check
				CHECK (movement_type IN ('cash_sale', 'cash_refund', 'cash_in', 'cash_out'))
			`).Error
		})

		after, err := SchemaFingerprint(pg.DB)
		require.NoError(t, err)
		afterSHA, err := SchemaFingerprintSHA(pg.DB)
		require.NoError(t, err)

		diff := DiffFingerprints(baseline, after)
		require.NotEmpty(t, diff, "changed CHECK allowed-value must diff through the canonicalizer")
		require.Contains(t, diff, `dimension "constraints"`, "expected constraints dimension:\n%s", diff)
		require.NotEqual(t, baselineSHA, afterSHA)
		require.Contains(t, after["constraints"], "cash_frozen", "new allowed value must appear post-canonicalization")
	})
}
