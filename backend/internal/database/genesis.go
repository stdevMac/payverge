package database

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/schema/genesis"
)

// DBClass is how a connected database relates to the migration/bootstrap lifecycle.
type DBClass int

const (
	// DBEmpty has zero base tables in public — the only safe genesis bootstrap target.
	DBEmpty DBClass = iota
	// DBVersioned has a schema_migrations table that either holds a version row
	// or is empty alongside every required genesis table (the migration-0
	// baseline, which writes no ledger row).
	DBVersioned
	// DBLegacy has tables but no usable migration ledger: no schema_migrations
	// table, or an empty one without the full genesis table set.
	DBLegacy
)

func (c DBClass) String() string {
	switch c {
	case DBEmpty:
		return "empty"
	case DBVersioned:
		return "versioned"
	case DBLegacy:
		return "legacy"
	default:
		return fmt.Sprintf("DBClass(%d)", int(c))
	}
}

// genesisAdvisoryLockKey serializes concurrent empty-DB bootstraps. pg_advisory_xact_lock
// is held for the duration of the bootstrap transaction and auto-releases on commit or
// rollback, so a failed baseline cannot leave the lock held across connections.
//
// Contract:
//   - Only a truly empty public schema is a bootstrap target (ClassifyDatabase → DBEmpty).
//   - Inside the transaction we re-check emptiness after acquiring the lock. Under READ
//     COMMITTED (GORM default), that SELECT sees tables committed by a prior winner, so
//     concurrent callers become idempotent no-ops instead of racing DDL.
//   - Established (versioned) and legacy databases must never receive the baseline dump;
//     BootstrapGenesisSchema is a no-op when the DB is non-empty.
const genesisAdvisoryLockKey int64 = 0x50560601

// requiredGenesisTables must exist after a successful baseline apply (and for
// VerifySchemaAtVersion). These are core operational tables plus the migrate ledger.
var requiredGenesisTables = []string{
	"businesses",
	"bills",
	"payments",
	"tables",
	"staff",
	"orders",
	"idempotency_keys",
	"printers",
	"schema_migrations",
}

type genesisVersionMeta struct {
	MigrationHead int64  `json:"migration_head"`
	SchemaSHA256  string `json:"schema_sha256"`
	// FingerprintSHA256 is SchemaFingerprintSHA of a database bootstrapped
	// from this baseline. Startup compares the live schema against it.
	FingerprintSHA256 string `json:"fingerprint_sha256"`
	// PostgresMajor is the server major version the baseline was dumped from.
	PostgresMajor int `json:"postgres_major"`
}

var (
	genesisMetaOnce sync.Once
	genesisMeta     genesisVersionMeta
	genesisMetaErr  error
)

func loadGenesisMeta() (genesisVersionMeta, error) {
	genesisMetaOnce.Do(func() {
		genesisMeta, genesisMetaErr = parseGenesisVersionJSON(genesis.VersionJSON)
	})
	return genesisMeta, genesisMetaErr
}

func parseGenesisVersionJSON(versionJSON []byte) (genesisVersionMeta, error) {
	var meta genesisVersionMeta
	if err := json.Unmarshal(versionJSON, &meta); err != nil {
		return meta, fmt.Errorf("parse genesis version.json: %w", err)
	}
	if meta.MigrationHead < 0 {
		return meta, fmt.Errorf("parse genesis version.json: migration_head must not be negative, got %d", meta.MigrationHead)
	}
	if meta.SchemaSHA256 == "" {
		return meta, fmt.Errorf("parse genesis version.json: schema_sha256 is required")
	}
	return meta, nil
}

// GenesisMigrationHead returns the embedded baseline migration HEAD (schema_migrations.version).
func GenesisMigrationHead() int64 {
	meta, err := loadGenesisMeta()
	if err != nil {
		return 0
	}
	return meta.MigrationHead
}

// ClassifyDatabase inspects the public schema to decide empty / versioned / legacy.
//
//   - 0 public base tables → DBEmpty (sole bootstrap target)
//   - schema_migrations holds a row → DBVersioned (golang-migrate owns it)
//   - schema_migrations is empty and every required genesis table exists →
//     DBVersioned. Only a head-0 baseline leaves the ledger empty (a head-N
//     baseline writes its row atomically), so this is a database bootstrapped
//     by a head-0 release; golang-migrate applies 000001.. from there, and
//     VerifySchemaFingerprint catches a hand-made schema at the head.
//   - otherwise → DBLegacy (tables created outside the genesis/migration path,
//     including a hand-made schema whose tool created an empty ledger)
func ClassifyDatabase(gormDB *gorm.DB) (DBClass, error) {
	if gormDB == nil {
		return 0, fmt.Errorf("ClassifyDatabase: gormDB is nil")
	}
	count, err := countPublicBaseTables(gormDB)
	if err != nil {
		return 0, err
	}
	if count == 0 {
		return DBEmpty, nil
	}

	var hasMigrationsTable bool
	if err := gormDB.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public'
			  AND table_name = 'schema_migrations'
			  AND table_type = 'BASE TABLE'
		)`).Scan(&hasMigrationsTable).Error; err != nil {
		return 0, fmt.Errorf("ClassifyDatabase: check schema_migrations: %w", err)
	}
	if !hasMigrationsTable {
		return DBLegacy, nil
	}

	var ledgerRows int64
	if err := gormDB.Raw(`SELECT count(*) FROM public.schema_migrations`).Scan(&ledgerRows).Error; err != nil {
		return 0, fmt.Errorf("ClassifyDatabase: count schema_migrations: %w", err)
	}
	if ledgerRows > 0 {
		return DBVersioned, nil
	}
	missing, err := missingRequiredTable(gormDB)
	if err != nil {
		return 0, fmt.Errorf("ClassifyDatabase: %w", err)
	}
	return classifyEmptyLedger(missing == ""), nil
}

// classifyEmptyLedger decides a database whose schema_migrations table exists
// but holds no row. The embedded head is deliberately not consulted: a
// database bootstrapped by a head-0 release keeps an empty ledger, and a
// later binary whose regenerated baseline sits at head N > 0 must still
// classify it as versioned so RunMigrations can apply 000001..N. Refusing it
// as legacy would strand every head-0 install on its first upgrade.
func classifyEmptyLedger(allGenesisTables bool) DBClass {
	if !allGenesisTables {
		return DBLegacy
	}
	return DBVersioned
}

func countPublicBaseTables(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Raw(`
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`).Scan(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count public base tables: %w", err)
	}
	return count, nil
}

// genesisSessionResetSQL undoes the session-level SETs at the top of the
// embedded pg_dump baseline. Applied on the commit path only; see bootstrapGenesis.
const genesisSessionResetSQL = "RESET statement_timeout; RESET lock_timeout; RESET idle_in_transaction_session_timeout; RESET transaction_timeout; RESET row_security; RESET check_function_bodies; RESET client_min_messages; RESET xmloption"

// BootstrapGenesisSchema applies the embedded genesis baseline to an empty database.
// Non-empty databases are a no-op (idempotent; does not mutate established/legacy state).
func BootstrapGenesisSchema(gormDB *gorm.DB) error {
	return bootstrapGenesis(gormDB, genesis.SchemaSQL, genesis.VersionJSON)
}

// bootstrapGenesis is the testable implementation. Tests may pass crafted schemaSQL and
// versionJSON pairs. Production callers use BootstrapGenesisSchema.
//
// Only a truly empty public schema is bootstrapped. The advisory xact lock + in-txn
// emptiness re-check make concurrent bootstrap safe: exactly one winner applies DDL;
// others no-op after the winner commits. Any error rolls the transaction back so a
// failed/partial baseline is never left marked current.
func bootstrapGenesis(gormDB *gorm.DB, schemaSQL string, versionJSON []byte) error {
	if gormDB == nil {
		return fmt.Errorf("bootstrapGenesis: gormDB is nil")
	}

	meta, err := parseGenesisVersionJSON(versionJSON)
	if err != nil {
		return err
	}

	sum := sha256.Sum256([]byte(schemaSQL))
	gotSHA := hex.EncodeToString(sum[:])
	if gotSHA != meta.SchemaSHA256 {
		// Refuse before touching the DB — a mismatched artifact must never be applied.
		return fmt.Errorf("genesis baseline SHA mismatch: embedded artifact does not match version.json")
	}

	return gormDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", genesisAdvisoryLockKey).Error; err != nil {
			return fmt.Errorf("genesis advisory lock: %w", err)
		}

		// Re-check emptiness under the lock so concurrent bootstraps serialize:
		// after the winner commits, losers see tables and become no-ops.
		count, err := countPublicBaseTables(tx)
		if err != nil {
			return err
		}
		if count != 0 {
			return nil
		}

		// Full baseline as one multi-statement Exec. pgx extended protocol rejects
		// multi-statement SQL; QueryExecModeSimpleProtocol is required (still one
		// Exec, no statement-boundary splitting). Same connection/txn as the lock.
		// Bypass GORM's SQL builder here. A pg_dump may legitimately contain the
		// PostgreSQL JSONB `?` operator; GORM treats that byte as a bind marker and
		// corrupts the dump when it rewrites placeholders. The transaction's raw
		// ConnPool still reaches pgx on the same connection and accepts the query
		// execution mode as its leading special argument.
		if _, err := tx.Statement.ConnPool.ExecContext(
			tx.Statement.Context,
			schemaSQL,
			pgx.QueryExecModeSimpleProtocol,
		); err != nil {
			return fmt.Errorf("apply genesis schema: %w", err)
		}

		// pg_dump emits set_config('search_path', '', false) (session-level empty
		// path). That would leave this pooled connection unable to resolve
		// unqualified table names after commit — restore public immediately.
		if err := tx.Exec(`SELECT pg_catalog.set_config('search_path', 'public', false)`).Error; err != nil {
			return fmt.Errorf("restore search_path after genesis schema: %w", err)
		}

		// On the error path the transaction rolls back, which also reverts
		// these SETs, so only the commit path needs the reset.
		if _, err := tx.Statement.ConnPool.ExecContext(
			tx.Statement.Context,
			genesisSessionResetSQL,
			pgx.QueryExecModeSimpleProtocol,
		); err != nil {
			return fmt.Errorf("reset session settings after genesis schema: %w", err)
		}

		if err := verifyRequiredTables(tx); err != nil {
			return err
		}
		if err := seedGenesisRuntimeControls(tx); err != nil {
			return err
		}

		// Head 0 means the baseline predates every numbered migration: leave the
		// ledger empty so golang-migrate applies 000001+ from a nil version.
		if meta.MigrationHead == 0 {
			return nil
		}
		if err := tx.Exec(
			`INSERT INTO public.schema_migrations (version, dirty) VALUES (?, false)`,
			meta.MigrationHead,
		).Error; err != nil {
			return fmt.Errorf("insert schema_migrations baseline version %d: %w", meta.MigrationHead, err)
		}
		return nil
	})
}

// seedGenesisRuntimeControls materializes the durable launch defaults. The
// embedded genesis artifact is intentionally schema-only, so seed rows that a
// fresh install needs are written here.
func seedGenesisRuntimeControls(db *gorm.DB) error {
	if err := db.Exec(`
		INSERT INTO public.runtime_controls
			(key, enabled, owner, reason, expires_at, updated_by)
		VALUES
			('maintenance_mode', FALSE, 'platform-oncall', 'Safe launch default: normal service', '2100-01-01T00:00:00Z', 'genesis-bootstrap'),
			('read_only_mode', FALSE, 'platform-oncall', 'Safe launch default: mutations allowed', '2100-01-01T00:00:00Z', 'genesis-bootstrap'),
			('payments_enabled', TRUE, 'payments-oncall', 'Preserve existing payment availability', '2100-01-01T00:00:00Z', 'genesis-bootstrap'),
			('fiscal_enabled', FALSE, 'security_owner', 'Public-launch containment pending fiscal sandbox evidence', '2100-01-01T00:00:00Z', 'genesis-bootstrap'),
			('ai_enabled', TRUE, 'ai-oncall', 'Preserve existing AI availability', '2100-01-01T00:00:00Z', 'genesis-bootstrap'),
			('uploads_enabled', TRUE, 'platform-oncall', 'Preserve existing upload availability', '2100-01-01T00:00:00Z', 'genesis-bootstrap'),
			('guest_orders_enabled', TRUE, 'hospitality-oncall', 'Preserve existing guest ordering availability', '2100-01-01T00:00:00Z', 'genesis-bootstrap')
		ON CONFLICT (key) DO NOTHING`).Error; err != nil {
		return fmt.Errorf("seed genesis runtime controls: %w", err)
	}
	return nil
}

func verifyRequiredTables(db *gorm.DB) error {
	missing, err := missingRequiredTable(db)
	if err != nil {
		return err
	}
	if missing != "" {
		return fmt.Errorf("genesis baseline missing required table %q", missing)
	}
	return nil
}

// missingRequiredTable returns the first required genesis table that does not
// exist, or "" when all of them do.
func missingRequiredTable(db *gorm.DB) (string, error) {
	for _, name := range requiredGenesisTables {
		var exists bool
		if err := db.Raw(`
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public'
				  AND table_name = ?
				  AND table_type = 'BASE TABLE'
			)`, name).Scan(&exists).Error; err != nil {
			return "", fmt.Errorf("verify required table %q: %w", name, err)
		}
		if !exists {
			return name, nil
		}
	}
	return "", nil
}

// VerifySchemaAtVersion asserts schema_migrations has exactly one clean row at head
// (or no row at all when head is 0, the genesis baseline with no numbered
// migration applied yet) and that all required tables exist. Used post-RunMigrations so a failed or drifted
// baseline cannot be treated as current.
func VerifySchemaAtVersion(gormDB *gorm.DB, head int64) error {
	if gormDB == nil {
		return fmt.Errorf("VerifySchemaAtVersion: gormDB is nil")
	}
	if head < 0 {
		return fmt.Errorf("VerifySchemaAtVersion: invalid head %d", head)
	}

	var rowCount int64
	if err := gormDB.Raw(`SELECT count(*) FROM public.schema_migrations`).Scan(&rowCount).Error; err != nil {
		return fmt.Errorf("VerifySchemaAtVersion: count schema_migrations: %w", err)
	}
	if head == 0 {
		if rowCount != 0 {
			return fmt.Errorf("VerifySchemaAtVersion: expected no schema_migrations row at head 0, got %d", rowCount)
		}
		if err := verifyRequiredTables(gormDB); err != nil {
			return fmt.Errorf("VerifySchemaAtVersion: %w", err)
		}
		return nil
	}
	if rowCount != 1 {
		return fmt.Errorf("VerifySchemaAtVersion: expected exactly 1 schema_migrations row, got %d", rowCount)
	}

	var version int64
	var dirty bool
	row := gormDB.Raw(`SELECT version, dirty FROM public.schema_migrations LIMIT 1`).Row()
	if err := row.Scan(&version, &dirty); err != nil {
		return fmt.Errorf("VerifySchemaAtVersion: read schema_migrations: %w", err)
	}
	if version != head {
		return fmt.Errorf("VerifySchemaAtVersion: version %d != expected head %d", version, head)
	}
	if dirty {
		return fmt.Errorf("VerifySchemaAtVersion: schema_migrations.dirty is true at version %d", version)
	}

	if err := verifyRequiredTables(gormDB); err != nil {
		return fmt.Errorf("VerifySchemaAtVersion: %w", err)
	}
	return nil
}

// VerifySchemaFingerprint refuses a database whose server major version or
// live schema differs from the embedded genesis baseline. head is the latest
// migration this binary ships. The fingerprint is compared only when head is
// the baseline head: the baseline is regenerated with every new migration, so
// a head past it means a migration landed without a regenerated baseline and
// there is no recorded fingerprint to compare against. It returns the live
// fingerprint SHA (empty when it was not computed).
func VerifySchemaFingerprint(gormDB *gorm.DB, head int64) (string, error) {
	if gormDB == nil {
		return "", fmt.Errorf("VerifySchemaFingerprint: gormDB is nil")
	}
	meta, err := loadGenesisMeta()
	if err != nil {
		return "", err
	}
	var serverVersionNum int
	if err := gormDB.Raw(`SELECT current_setting('server_version_num')::int`).Scan(&serverVersionNum).Error; err != nil {
		return "", fmt.Errorf("VerifySchemaFingerprint: read server_version_num: %w", err)
	}
	if err := checkPostgresMajor(serverVersionNum, meta.PostgresMajor); err != nil {
		return "", err
	}
	if head != meta.MigrationHead {
		return "", nil
	}
	live, err := SchemaFingerprintSHA(gormDB)
	if err != nil {
		return "", fmt.Errorf("VerifySchemaFingerprint: %w", err)
	}
	if err := checkFingerprint(live, meta.FingerprintSHA256); err != nil {
		return live, err
	}
	return live, nil
}

func checkPostgresMajor(serverVersionNum, want int) error {
	if want <= 0 {
		return fmt.Errorf("genesis version.json: postgres_major is required")
	}
	if got := serverVersionNum / 10000; got != want {
		return fmt.Errorf("postgres major version %d does not match the genesis baseline's %d; move the database to PostgreSQL %d first (self-host: deploy/upgrade-postgres.sh, docs/self-hosting/upgrades.md \"PostgreSQL 18\")", got, want, want)
	}
	return nil
}

func checkFingerprint(live, want string) error {
	if want == "" {
		return fmt.Errorf("genesis version.json: fingerprint_sha256 is required; regenerate with backend/scripts/generate-genesis-schema.sh")
	}
	if live != want {
		return fmt.Errorf("schema drift: live schema fingerprint %s does not match the genesis baseline's %s", live, want)
	}
	return nil
}
