package database

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// scripts/sanitize-clone.sql is the PII/secret scrub applied to a restored
// production-backup clone BEFORE it is used in a restore drill (Priority-4 gate
// (e): "sanitized production-backup clone"). These tests execute the real scrub
// statements against a seeded schema and prove that:
//   - every PII/secret column is overwritten (no seeded real value survives),
//   - UNIQUE PII columns (emails) are scrubbed to DISTINCT per-row values so the
//     scrub cannot violate a unique constraint,
//   - non-PII columns (a business's public name, an active flag, the row id) are
//     left untouched — i.e. the scrub is targeted, not a blanket wipe.
//
// This gives Docker-free execution evidence of the scrub LOGIC. The end-to-end
// "raw production dump -> scrubbed clone" proof still runs under Postgres in the
// restore drill (run-restore-drill.sh), which is gated on Docker.

func sanitizeCloneSQLPath(t *testing.T) string {
	t.Helper()
	// internal/database -> ../../scripts/sanitize-clone.sql (backend/scripts).
	p, err := filepath.Abs(filepath.Join("..", "..", "scripts", "sanitize-clone.sql"))
	require.NoError(t, err)
	return p
}

var updateTableRe = regexp.MustCompile(`(?is)^\s*UPDATE\s+([a-z_][a-z0-9_]*)`)

// applyScrubForTables runs each UPDATE statement in the scrub file whose target
// table is in `tables`. It deliberately does NOT swallow execution errors on
// matched statements — a scrub statement that fails to run is a real defect.
// Statements targeting tables not in the seeded set are skipped (their tables
// don't exist in this focused SQLite schema).
func applyScrubForTables(t *testing.T, db *gorm.DB, sqlText string, tables map[string]bool) int {
	t.Helper()
	ran := 0
	for _, raw := range strings.Split(sqlText, ";") {
		// Drop full-line SQL comments so the leading UPDATE keyword is visible.
		var lines []string
		for _, ln := range strings.Split(raw, "\n") {
			if strings.HasPrefix(strings.TrimSpace(ln), "--") {
				continue
			}
			lines = append(lines, ln)
		}
		stmt := strings.TrimSpace(strings.Join(lines, "\n"))
		if stmt == "" {
			continue
		}
		m := updateTableRe.FindStringSubmatch(stmt)
		if m == nil {
			continue // BEGIN/COMMIT/other — not a table UPDATE.
		}
		if !tables[strings.ToLower(m[1])] {
			continue
		}
		require.NoError(t, db.Exec(stmt).Error, "scrub statement failed:\n%s", stmt)
		ran++
	}
	return ran
}

func newSanitizeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file:sanitize_clone_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&User{}, &Staff{}, &Business{}, &Customer{}))
	// Isolate from any shared in-memory state left by a prior test.
	for _, tbl := range []string{"users", "staff", "businesses", "customers"} {
		require.NoError(t, gormDB.Exec("DELETE FROM "+tbl).Error)
	}
	db = gormDB
	return gormDB
}

func TestSanitizeClone_ScrubsPII_PreservesNonPII(t *testing.T) {
	db := newSanitizeTestDB(t)

	sqlBytes, err := os.ReadFile(sanitizeCloneSQLPath(t))
	require.NoError(t, err, "scripts/sanitize-clone.sql must exist")
	scrub := string(sqlBytes)

	// --- Seed realistic PII. Two customers + two staff share the "same shape"
	// so the per-row-unique email scrub is exercised for collision safety.
	require.NoError(t, db.Create(&User{
		Email: "alice@real.example", Name: "Alice Real", Address: "0xAAAAaaaaAAAAaaaaAAAAaaaaAAAAaaaaAAAAaaaa",
	}).Error)

	require.NoError(t, db.Create(&Customer{
		Email: "bob@real.example", PasswordHash: "$2a$bobhash", Name: "Bob Real",
		Phone: "+15551234567", WalletAddress: "0xBBBBbbbb", VerificationToken: "vtok-bob",
		PasswordResetToken: "rtok-bob", IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&Customer{
		Email: "carol@real.example", PasswordHash: "$2a$carolhash", Name: "Carol Real",
		Phone: "+15559876543", WalletAddress: "0xCCCCcccc", IsActive: true,
	}).Error)

	require.NoError(t, db.Create(&Staff{
		BusinessID: 1, Email: "dan@real.example", Name: "Dan Real", PinHash: "$2a$danpin",
		InvitedBy: "owner@real.example", Role: StaffRoleServer, IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&Staff{
		BusinessID: 1, Email: "eve@real.example", Name: "Eve Real", PinHash: "$2a$evepin",
		InvitedBy: "owner@real.example", Role: StaffRoleServer, IsActive: true,
	}).Error)

	require.NoError(t, db.Create(&Business{
		OwnerName: "Frank Owner", Name: "Frank's Diner", Phone: "+15550001111",
		Email: "frank@real.example", OwnerAddress: "0xFFFFffff",
		SettlementAddr: "0x1111settle", TippingAddr: "0x2222tip",
	}).Error)

	// --- Apply the real scrub statements for the seeded tables.
	ran := applyScrubForTables(t, db, scrub, map[string]bool{
		"users": true, "staff": true, "businesses": true, "customers": true,
	})
	require.Greater(t, ran, 0, "expected the scrub file to contain UPDATE statements for seeded tables")

	// --- (1) No seeded real PII literal survives anywhere in the scrubbed tables.
	realPII := []string{
		"alice@real.example", "Alice Real",
		"bob@real.example", "$2a$bobhash", "Bob Real", "+15551234567", "0xBBBBbbbb", "vtok-bob", "rtok-bob",
		"carol@real.example", "$2a$carolhash", "Carol Real", "+15559876543", "0xCCCCcccc",
		"dan@real.example", "Dan Real", "$2a$danpin", "owner@real.example",
		"eve@real.example", "Eve Real", "$2a$evepin",
		"Frank Owner", "+15550001111", "frank@real.example", "0xFFFFffff", "0x1111settle", "0x2222tip",
	}
	for _, tbl := range []string{"users", "staff", "businesses", "customers"} {
		var rows []map[string]interface{}
		require.NoError(t, db.Table(tbl).Find(&rows).Error)
		for _, row := range rows {
			for col, val := range row {
				s, ok := val.(string)
				if !ok {
					continue
				}
				for _, pii := range realPII {
					assert.NotContainsf(t, s, pii,
						"table %s column %s still contains seeded PII %q after scrub", tbl, col, pii)
				}
			}
		}
	}

	// --- (2) UNIQUE email columns scrubbed to DISTINCT, non-real, pattern values.
	var custEmails []string
	require.NoError(t, db.Table("customers").Order("id").Pluck("email", &custEmails).Error)
	require.Len(t, custEmails, 2)
	assert.NotEqual(t, custEmails[0], custEmails[1], "unique customer emails must stay distinct after scrub")
	for _, e := range custEmails {
		assert.Contains(t, e, "@example.invalid", "scrubbed email must use the reserved .invalid TLD")
	}

	var staffEmails []string
	require.NoError(t, db.Table("staff").Order("id").Pluck("email", &staffEmails).Error)
	require.Len(t, staffEmails, 2)
	assert.NotEqual(t, staffEmails[0], staffEmails[1], "unique staff emails must stay distinct after scrub")

	// --- (3) Targeted, not a blanket wipe: public business name + active flags +
	// ids are preserved.
	var bizName string
	require.NoError(t, db.Table("businesses").Select("name").Row().Scan(&bizName))
	assert.Equal(t, "Frank's Diner", bizName, "public business name is not PII and must be preserved")

	var activeCount int64
	require.NoError(t, db.Table("customers").Where("is_active = ?", true).Count(&activeCount).Error)
	assert.Equal(t, int64(2), activeCount, "non-PII is_active flag must be preserved")
}

// TestSanitizeClone_ScrubsPlatformSecrets proves every is_secret
// platform_settings row is emptied by a production-backup clone scrub while
// non-secret operator tuning survives.
func TestSanitizeClone_ScrubsPlatformSecrets(t *testing.T) {
	gormDB, err := gorm.Open(sqlite.Open("file:sanitize_clone_platform_settings?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&PlatformSettings{}))
	require.NoError(t, gormDB.Exec("DELETE FROM platform_settings").Error)

	seed := []PlatformSettings{
		{Key: "provider_api_key", Value: "sk_live_51AbCdEfGhIjKlMnOpQrStUvWxYz0123456789secret", Category: CategoryGeneral, IsSecret: true},
		{Key: SettingImageDailyLimit, Value: "42", Category: CategoryGeneral, IsSecret: false},
	}
	for i := range seed {
		require.NoError(t, gormDB.Create(&seed[i]).Error)
	}

	sqlBytes, err := os.ReadFile(sanitizeCloneSQLPath(t))
	require.NoError(t, err)
	ran := applyScrubForTables(t, gormDB, string(sqlBytes), map[string]bool{"platform_settings": true})
	require.GreaterOrEqual(t, ran, 1, "expected the is_secret scrub UPDATE for platform_settings")

	var rows []PlatformSettings
	require.NoError(t, gormDB.Find(&rows).Error)
	require.Len(t, rows, len(seed))
	byKey := make(map[string]PlatformSettings, len(rows))
	for _, r := range rows {
		byKey[r.Key] = r
	}
	assert.Equal(t, "", byKey["provider_api_key"].Value, "secret rows must be emptied")
	assert.Equal(t, "42", byKey[SettingImageDailyLimit].Value, "non-secret tuning must survive")
}
