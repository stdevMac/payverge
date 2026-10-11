package demo

// #921/#904/#905: the US showroom venues were still selling "Harvest Bowl" at
// "18 Demo Market St", still carrying Spanish leftover tickets on an EN venue
// and a week-old ghost check on Table 1 — content from a seed version retired
// long ago. The reseed those venues need runs inside ONE transaction: wipe the
// owned demo rows, then regenerate. Postgres enforces every foreign key into
// businesses (and into the demo rows hanging off them), so a single table the
// wipe forgets aborts the delete, rolls the whole transaction back, and the
// instance stays frozen on its old seed through every deploy — forever, since
// each hourly tick retries and fails the same way.
//
// This guard reads the genesis schema (the source of truth for production DDL)
// and fails when a foreign key that would block deleting a demo business has no
// cleanup in deleteOwnedDemoData. It is a schema-driven test on purpose: the
// next table someone adds with a business_id must show up here, not in a live
// showroom that quietly stops reseeding.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
)

const genesisSchemaPath = "../../schema/genesis/current_schema.sql"

var genesisForeignKeyPattern = regexp.MustCompile(
	`ALTER TABLE ONLY public\.(\w+)\s*\n\s*ADD CONSTRAINT \w+ FOREIGN KEY \(([^)]+)\) REFERENCES public\.(\w+)\(([^)]+)\)([^;]*);`)

var genesisTablePattern = regexp.MustCompile(`CREATE TABLE public\.(\w+) \(`)

// tablesBlockingDeleteOf walks the genesis foreign keys and returns every table
// whose rows must be gone before a row in root can be deleted. ON DELETE
// CASCADE and ON DELETE SET NULL edges clean themselves, so they are skipped.
func tablesBlockingDeleteOf(t *testing.T, root string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(genesisSchemaPath)
	require.NoError(t, err, "genesis schema is the production DDL source of truth")

	children := map[string][][2]string{}
	for _, m := range genesisForeignKeyPattern.FindAllStringSubmatch(string(raw), -1) {
		child, column, parent, action := m[1], strings.TrimSpace(m[2]), m[3], m[5]
		if strings.Contains(action, "ON DELETE CASCADE") || strings.Contains(action, "ON DELETE SET NULL") {
			continue
		}
		children[parent] = append(children[parent], [2]string{child, column})
	}
	require.NotEmpty(t, children[root], "no foreign keys found into %s — the schema parse regressed", root)

	blocking := map[string]string{}
	queue := []string{root}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, edge := range children[parent] {
			if _, seen := blocking[edge[0]]; seen {
				continue
			}
			blocking[edge[0]] = parent + "." + edge[1]
			queue = append(queue, edge[0])
		}
	}
	return blocking
}

func tableNameOf(t *testing.T, model interface{}) string {
	t.Helper()
	parsed, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	require.NoErrorf(t, err, "cannot resolve a table name for %T", model)
	return parsed.Table
}

func TestDemoWipeClearsEveryTableThatBlocksABusinessDelete(t *testing.T) {
	blocking := tablesBlockingDeleteOf(t, "businesses")

	covered := map[string]bool{}
	for _, spec := range demoWipeChildSpecs {
		covered[tableNameOf(t, spec.model)] = true
	}
	for _, model := range demoWipeBusinessModels() {
		covered[tableNameOf(t, model)] = true
	}

	// Handled by name in deleteOwnedDemoData / EnsureForAdmin rather than by a
	// model list, because neither is owned by the business being deleted.
	handledElsewhere := map[string]string{
		"demo_instances": "the instance survives the reseed; its business pointers are nulled first",
		"demo_runs":      "deleted by admin_user_id right after the wipe",
	}

	missing := []string{}
	for table, via := range blocking {
		if covered[table] || handledElsewhere[table] != "" {
			continue
		}
		missing = append(missing, table+" (references "+via+")")
	}
	sort.Strings(missing)
	require.Emptyf(t, missing,
		"these tables block deleting a demo business and nothing in deleteOwnedDemoData clears them, "+
			"so the forced reseed rolls back and the showroom freezes on its old seed: %s",
		strings.Join(missing, ", "))
}

// A model whose table no longer exists means the wipe is silently deleting
// nothing — the same failure mode, one layer up.
func TestDemoWipeModelsAllExistInGenesisSchema(t *testing.T) {
	raw, err := os.ReadFile(genesisSchemaPath)
	require.NoError(t, err)
	known := map[string]bool{}
	for _, m := range genesisTablePattern.FindAllStringSubmatch(string(raw), -1) {
		known[m[1]] = true
	}
	require.NotEmpty(t, known)

	models := make([]interface{}, 0, len(demoWipeChildSpecs)+len(demoWipeBusinessModels()))
	for _, spec := range demoWipeChildSpecs {
		models = append(models, spec.model)
	}
	models = append(models, demoWipeBusinessModels()...)
	for _, model := range models {
		table := tableNameOf(t, model)
		require.Truef(t, known[table], "%T maps to table %q, which the genesis schema does not define", model, table)
	}
}

// The grandchild cleanups must run children-before-parents, and every spec must
// name an id set deleteOwnedDemoData actually collects.
func TestDemoWipeChildSpecsAreOrderedAndFed(t *testing.T) {
	fed := map[string]bool{
		"bills": true, "payments": true, "delivery_orders": true, "table_reservations": true,
		"customer_businesses": true, "customers": true, "loyalty_programs": true,
		"ai_waiter_conversations": true, "menu_wizard_sessions": true, "menu_extraction_jobs": true,
		"plugin_notification_deliveries": true, "staff": true,
	}
	seen := map[string]int{}
	for i, spec := range demoWipeChildSpecs {
		require.Truef(t, fed[spec.parent],
			"spec %d cleans %q keyed on %s, but deleteOwnedDemoData collects no ids for it", i, spec.parent, spec.column)
		seen[tableNameOf(t, spec.model)] = i
	}
	// Refund evidence must be cleared before the payments it hangs off.
	require.Lessf(t, seen["payment_refund_destinations"], seen["payments"],
		"payment_refund_destinations must be deleted before payments")
}
