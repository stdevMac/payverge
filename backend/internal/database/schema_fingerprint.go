package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// schemaFingerprintDimensions is the ordered set of catalog dimensions always
// present in a SchemaFingerprint map (even when empty today). Empty dimensions
// are intentional drift tripwires: a future enum/view/trigger/etc. must appear
// here rather than silently passing an incomplete fingerprint.
var schemaFingerprintDimensions = []string{
	"tables",
	"columns",
	"sequences",
	"indexes",
	"constraints",
	"enums",
	"views",
	"matviews",
	"functions",
	"triggers",
	"extensions",
}

// tableExclusionSQL is the common public-schema filter applied to every
// table-scoped dimension query. schema_migrations is excluded because its
// ROWS (version ledger) differ by nature; whatsmeow_% tables are library-owned
// runtime schema never present in the genesis dump.
const tableExclusionSQL = `
	table_schema = 'public'
	AND table_name <> 'schema_migrations'
	AND table_name NOT LIKE 'whatsmeow_%'
`

// SchemaFingerprint returns a normalized, order-stable fingerprint of the live
// public schema as one deterministic string per dimension. Each dimension's
// rows are sorted by a stable key and joined with '\n'. Empty dimensions are
// included as empty strings so a future addition is a detectable delta.
func SchemaFingerprint(gormDB *gorm.DB) (map[string]string, error) {
	if gormDB == nil {
		return nil, fmt.Errorf("SchemaFingerprint: gormDB is nil")
	}

	// One transaction so SET LOCAL search_path applies to every dimension
	// query. A pool-level set_config is transaction-local and would land on a
	// different connection than the reads that follow.
	out := make(map[string]string, len(schemaFingerprintDimensions))
	err := gormDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL search_path TO public`).Error; err != nil {
			return fmt.Errorf("SchemaFingerprint: set search_path: %w", err)
		}

		for _, dim := range schemaFingerprintDimensions {
			out[dim] = ""
		}

		var err error
		if out["tables"], err = fpTables(tx); err != nil {
			return err
		}
		if out["columns"], err = fpColumns(tx); err != nil {
			return err
		}
		if out["sequences"], err = fpSequences(tx); err != nil {
			return err
		}
		if out["indexes"], err = fpIndexes(tx); err != nil {
			return err
		}
		if out["constraints"], err = fpConstraints(tx); err != nil {
			return err
		}
		if out["enums"], err = fpEnums(tx); err != nil {
			return err
		}
		if out["views"], err = fpViews(tx, "v"); err != nil {
			return err
		}
		if out["matviews"], err = fpViews(tx, "m"); err != nil {
			return err
		}
		if out["functions"], err = fpFunctions(tx); err != nil {
			return err
		}
		if out["triggers"], err = fpTriggers(tx); err != nil {
			return err
		}
		if out["extensions"], err = fpExtensions(tx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SchemaFingerprintSHA returns the SHA-256 hex digest of the fingerprint map.
// Dimensions are sorted by name; each contribution is name + '\x00' + value
// concatenated in order (no inter-pair separator).
func SchemaFingerprintSHA(gormDB *gorm.DB) (string, error) {
	fp, err := SchemaFingerprint(gormDB)
	if err != nil {
		return "", err
	}
	return hashFingerprint(fp), nil
}

func hashFingerprint(fp map[string]string) string {
	keys := make([]string, 0, len(fp))
	for k := range fp {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(fp[k]))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func scanSortedLines(gormDB *gorm.DB, query string, args ...any) (string, error) {
	var rows []string
	if err := gormDB.Raw(query, args...).Scan(&rows).Error; err != nil {
		return "", err
	}
	// Defensive sort: queries already ORDER BY, but Scan order is not
	// contractually guaranteed across drivers.
	sort.Strings(rows)
	return strings.Join(rows, "\n"), nil
}

func fpTables(db *gorm.DB) (string, error) {
	s, err := scanSortedLines(db, `
		SELECT table_name
		FROM information_schema.tables
		WHERE `+tableExclusionSQL+`
		  AND table_type = 'BASE TABLE'
		ORDER BY table_name`)
	if err != nil {
		return "", fmt.Errorf("fingerprint tables: %w", err)
	}
	return s, nil
}

func fpColumns(db *gorm.DB) (string, error) {
	// information_schema.columns covers UDT name, precision/scale, nullability,
	// defaults, generated columns, and identity. identity_generation is null
	// when the column is not an identity column.
	s, err := scanSortedLines(db, `
		SELECT
			c.table_name || '.' || c.column_name
			|| '|' || COALESCE(c.data_type, '')
			|| '|' || COALESCE(c.udt_name, '')
			|| '|' || COALESCE(c.numeric_precision::text, '')
			|| '|' || COALESCE(c.numeric_scale::text, '')
			|| '|' || COALESCE(c.character_maximum_length::text, '')
			|| '|' || COALESCE(c.is_nullable, '')
			|| '|' || COALESCE(c.column_default, '')
			|| '|' || COALESCE(c.is_generated, 'NEVER')
			|| '|' || COALESCE(c.generation_expression, '')
			|| '|' || COALESCE(c.identity_generation, '')
		FROM information_schema.columns c
		WHERE c.table_schema = 'public'
		  AND c.table_name <> 'schema_migrations'
		  AND c.table_name NOT LIKE 'whatsmeow_%'
		ORDER BY c.table_name, c.column_name`)
	if err != nil {
		return "", fmt.Errorf("fingerprint columns: %w", err)
	}
	return s, nil
}

func fpSequences(db *gorm.DB) (string, error) {
	// NEVER include last_value/currval — only name, type, and OWNED BY.
	// deptype 'a' is the auto-dependency used by OWNED BY.
	s, err := scanSortedLines(db, `
		SELECT
			seq.relname
			|| '|' || COALESCE(format_type(s.seqtypid, NULL), '')
			|| '|' || COALESCE(tbl.relname || '.' || att.attname, '')
		FROM pg_class seq
		JOIN pg_namespace n ON n.oid = seq.relnamespace
		JOIN pg_sequence s ON s.seqrelid = seq.oid
		LEFT JOIN pg_depend d
			ON d.objid = seq.oid
			AND d.classid = 'pg_class'::regclass
			AND d.deptype = 'a'
		LEFT JOIN pg_class tbl ON tbl.oid = d.refobjid
		LEFT JOIN pg_attribute att
			ON att.attrelid = d.refobjid
			AND att.attnum = d.refobjsubid
			AND NOT att.attisdropped
		WHERE n.nspname = 'public'
		  AND seq.relkind = 'S'
		  AND seq.relname NOT LIKE 'whatsmeow_%'
		  AND (tbl.relname IS NULL OR (
				tbl.relname <> 'schema_migrations'
				AND tbl.relname NOT LIKE 'whatsmeow_%'
		  ))
		ORDER BY seq.relname`)
	if err != nil {
		return "", fmt.Errorf("fingerprint sequences: %w", err)
	}
	return s, nil
}

func fpIndexes(db *gorm.DB) (string, error) {
	// Full pg_get_indexdef captures expression indexes and partial predicates.
	// Each row is passed
	// through canonicalizeConstraintDef because a partial index's WHERE predicate
	// carries the SAME benign varchar->text deparse artifact as a CHECK
	// constraint: a dump-restored baseline serializes
	// `... = ANY (ARRAY['a'::character varying::text, ...])` while the freshly
	// reconciled reference serializes `... = ANY ((ARRAY['a'::character varying,
	// ...])::text[])`. Both accept identical rows; canonicalizing collapses the
	// cast spelling so genesis and reference agree. The regexes only strip cast
	// tokens and their grouping parens, so a real index change (name, columns,
	// operator, or allowed-value literal) still diffs — proven by
	// TestSchemaFingerprint_MutationDetection's indexes subtest.
	var rows []string
	if err := db.Raw(`
		SELECT pg_get_indexdef(i.indexrelid)
		FROM pg_index i
		JOIN pg_class idx ON idx.oid = i.indexrelid
		JOIN pg_class tbl ON tbl.oid = i.indrelid
		JOIN pg_namespace n ON n.oid = tbl.relnamespace
		WHERE n.nspname = 'public'
		  AND tbl.relname <> 'schema_migrations'
		  AND tbl.relname NOT LIKE 'whatsmeow_%'
		  AND idx.relname NOT LIKE 'whatsmeow_%'
		ORDER BY idx.relname, pg_get_indexdef(i.indexrelid)`).Scan(&rows).Error; err != nil {
		return "", fmt.Errorf("fingerprint indexes: %w", err)
	}
	for i := range rows {
		rows[i] = canonicalizeConstraintDef(rows[i])
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n"), nil
}

func fpConstraints(db *gorm.DB) (string, error) {
	// Full pg_get_constraintdef includes ON DELETE/UPDATE and DEFERRABLE. Each
	// row is passed through canonicalizeConstraintDef to erase a benign Postgres
	// deparse artifact (see that function) before sorting/joining.
	var rows []string
	if err := db.Raw(`
		SELECT
			tbl.relname || '.' || c.conname
			|| '|' || pg_get_constraintdef(c.oid, true)
		FROM pg_constraint c
		JOIN pg_class tbl ON tbl.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = tbl.relnamespace
		WHERE n.nspname = 'public'
		  AND c.conrelid <> 0
		  AND tbl.relname <> 'schema_migrations'
		  AND tbl.relname NOT LIKE 'whatsmeow_%'
		ORDER BY tbl.relname, c.conname`).Scan(&rows).Error; err != nil {
		return "", fmt.Errorf("fingerprint constraints: %w", err)
	}
	for i := range rows {
		rows[i] = canonicalizeConstraintDef(rows[i])
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n"), nil
}

// Regexes for canonicalizeConstraintDef. Compiled once; used read-only.
var (
	// (ARRAY[...])::text[]  ->  ARRAY[...]   (paren-wrapped array cast; pg_dump text form)
	reArrayCastParen = regexp.MustCompile(`\((ARRAY\[[^\]]*\])\)::text\[\]`)
	// ARRAY[...]::text[]    ->  ARRAY[...]   (bare array cast; pg_get_constraintdef pretty=true form)
	reArrayCastBare = regexp.MustCompile(`(ARRAY\[[^\]]*\])::text\[\]`)
	// ::character varying redundant scalar/element cast.
	reCharVaryingCast = regexp.MustCompile(`::character varying`)
	// ::text scalar cast, but NOT ::text[] (array cast). RE2 has no lookahead,
	// so match the following char and preserve it; the [ case never matches.
	reTextCast = regexp.MustCompile(`::text([^\[]|$)`)
	// Redundant parens around a lone string literal left behind by the
	// per-element cast form `('x'::character varying)::text` once its casts are
	// stripped: ('x') -> 'x'. This makes the per-element IN(...) spelling
	// converge with the array-level spelling. Requires surrounding quotes, so it
	// can only ever match a quoted literal element — never `(col)` or a larger
	// parenthesised expression.
	reLiteralParen = regexp.MustCompile(`\('([^']*)'\)`)
)

// canonicalizeConstraintDef erases a benign Postgres deparse artifact so that
// two SEMANTICALLY IDENTICAL CHECK constraints fingerprint the same. Postgres
// stores `col IN ('a','b')` on a varchar column and re-serializes it two
// equivalent ways depending on how it arrived:
//
//	authored via GORM/ensure IN(...):  ... = ANY ((ARRAY['a'::character varying, 'b'::character varying])::text[])
//	restored from the pg_dump baseline: ... = ANY (ARRAY['a'::character varying::text, 'b'::character varying::text])
//
// Both accept/reject identical rows — the difference is purely where Postgres
// places the varchar->text coercion after a dump/restore round-trip. The
// genesis baseline (a dump) and the reconciled reference (freshly created via
// GORM/ensure) therefore disagree ONLY in this cast spelling. Stripping the
// redundant varchar/text casts (and the array-cast grouping parens) collapses
// both to one canonical string.
//
// This is applied symmetrically to both fingerprints, so it can only ever make
// two constraints that differ SOLELY in this cast spelling compare equal — it
// cannot mask a real predicate/column/operator/allowed-value change (proven by
// TestSchemaFingerprint_MutationDetection's fk_on_delete and check_predicate
// subtests, which mutate constraint substance and still diff).
func canonicalizeConstraintDef(def string) string {
	def = reArrayCastParen.ReplaceAllString(def, "$1")
	def = reArrayCastBare.ReplaceAllString(def, "$1")
	def = reCharVaryingCast.ReplaceAllString(def, "")
	def = reTextCast.ReplaceAllString(def, "$1")
	def = reLiteralParen.ReplaceAllString(def, "'$1'")
	return def
}

func fpEnums(db *gorm.DB) (string, error) {
	s, err := scanSortedLines(db, `
		SELECT
			t.typname || '|' || string_agg(e.enumlabel, ',' ORDER BY e.enumsortorder)
		FROM pg_type t
		JOIN pg_namespace n ON n.oid = t.typnamespace
		JOIN pg_enum e ON e.enumtypid = t.oid
		WHERE n.nspname = 'public'
		  AND t.typtype = 'e'
		GROUP BY t.typname
		ORDER BY t.typname`)
	if err != nil {
		return "", fmt.Errorf("fingerprint enums: %w", err)
	}
	return s, nil
}

// fpViews fingerprints ordinary views (relkind 'v') or materialized views ('m').
func fpViews(db *gorm.DB, relkind string) (string, error) {
	s, err := scanSortedLines(db, `
		SELECT c.relname || '|' || pg_get_viewdef(c.oid, true)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
		  AND c.relkind = ?
		  AND c.relname NOT LIKE 'whatsmeow_%'
		ORDER BY c.relname`, relkind)
	if err != nil {
		return "", fmt.Errorf("fingerprint views(relkind=%s): %w", relkind, err)
	}
	return s, nil
}

func fpFunctions(db *gorm.DB) (string, error) {
	// Application functions only: exclude anything owned by an extension
	// (pg_depend deptype='e'). This keeps pg_trgm helpers out of "functions"
	// and listed solely under "extensions".
	s, err := scanSortedLines(db, `
		SELECT pg_get_functiondef(p.oid)
		FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'public'
		  AND p.prokind IN ('f', 'p', 'w')
		  AND NOT EXISTS (
			SELECT 1
			FROM pg_depend d
			WHERE d.objid = p.oid
			  AND d.classid = 'pg_proc'::regclass
			  AND d.deptype = 'e'
		  )
		ORDER BY p.proname, pg_get_function_identity_arguments(p.oid)`)
	if err != nil {
		return "", fmt.Errorf("fingerprint functions: %w", err)
	}
	return s, nil
}

func fpTriggers(db *gorm.DB) (string, error) {
	// Exclude internal/FK-RI triggers (tgisinternal = true).
	s, err := scanSortedLines(db, `
		SELECT
			c.relname || '.' || t.tgname
			|| '|' || pg_get_triggerdef(t.oid, true)
		FROM pg_trigger t
		JOIN pg_class c ON c.oid = t.tgrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
		  AND NOT t.tgisinternal
		  AND c.relname <> 'schema_migrations'
		  AND c.relname NOT LIKE 'whatsmeow_%'
		ORDER BY c.relname, t.tgname`)
	if err != nil {
		return "", fmt.Errorf("fingerprint triggers: %w", err)
	}
	return s, nil
}

func fpExtensions(db *gorm.DB) (string, error) {
	// NEVER include extversion — only name + namespace schema.
	s, err := scanSortedLines(db, `
		SELECT e.extname || '|' || n.nspname
		FROM pg_extension e
		JOIN pg_namespace n ON n.oid = e.extnamespace
		ORDER BY e.extname, n.nspname`)
	if err != nil {
		return "", fmt.Errorf("fingerprint extensions: %w", err)
	}
	return s, nil
}
