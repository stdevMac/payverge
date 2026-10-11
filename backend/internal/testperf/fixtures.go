package testperf

import (
	_ "embed"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

//go:embed testdata/seed.sql
var seedSQL string

// LoadFixtures runs the embedded seed.sql against db. Idempotent (uses
// ON CONFLICT DO NOTHING / NOT EXISTS guards). The seed assumes the full
// schema is already present — call this AFTER migrations.
//
// We strip `--` line comments then split on `;` and execute one statement
// at a time. pgx's extended query protocol — used by GORM when PrepareStmt
// is enabled — rejects multi-statement SQL (SQLSTATE 42601), so we cannot
// just call Exec(entireFile). Production opens GORM with PrepareStmt: true
// for query-hot-path performance; the seed is a one-off at bench setup so
// the overhead of N individual Execs is irrelevant. The seed file
// deliberately avoids semicolons inside string literals so a naive split
// after comment-stripping is safe; if that changes we'd need a real SQL
// splitter.
func LoadFixtures(db *gorm.DB) error {
	cleaned := stripLineComments(seedSQL)
	for i, stmt := range strings.Split(cleaned, ";") {
		s := strings.TrimSpace(stmt)
		if s == "" {
			continue
		}
		if err := db.Exec(s).Error; err != nil {
			return fmt.Errorf("seed statement %d: %w", i, err)
		}
	}
	return nil
}

// stripLineComments removes `-- ...` line comments from sql. It does not
// look inside string literals because seed.sql never embeds `--` inside a
// quoted string — keep that invariant if you edit the seed.
func stripLineComments(sql string) string {
	var b strings.Builder
	b.Grow(len(sql))
	for _, line := range strings.Split(sql, "\n") {
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
