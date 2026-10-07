// Package schedclaim provides an atomic "claim before side effect" helper:
// exactly one caller can flip a done-marker column from NULL to now() for a
// given row, so duplicate ticks / restarts never double-fire a side effect.
package schedclaim

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Claimer issues atomic claim UPDATEs against a *gorm.DB.
type Claimer struct{ db *gorm.DB }

// New returns a Claimer bound to db (pass database.GetDB() / d.GetGorm()).
func New(db *gorm.DB) *Claimer { return &Claimer{db: db} }

// Claim runs UPDATE <table> SET <doneCol>=now() WHERE <keyCol>=? AND <doneCol> IS NULL
// and reports whether this caller won the claim (RowsAffected == 1). table,
// doneCol and keyCol are identifiers supplied by trusted call sites only — they
// are quoted but never user-derived. keyVal is bound as a parameter.
func (c *Claimer) Claim(ctx context.Context, table, doneCol, keyCol string, keyVal interface{}) (bool, error) {
	sql := fmt.Sprintf(
		"UPDATE %s SET %s = CURRENT_TIMESTAMP WHERE %s = ? AND %s IS NULL",
		quoteIdent(table), quoteIdent(doneCol), quoteIdent(keyCol), quoteIdent(doneCol),
	)
	res := c.db.WithContext(ctx).Exec(sql, keyVal)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// quoteIdent double-quotes a SQL identifier (Postgres + SQLite compatible) and
// escapes embedded quotes. Call sites pass static identifiers, so this is a
// defense-in-depth guard, not a substitute for validated input.
func quoteIdent(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '"')
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			out = append(out, '"')
		}
		out = append(out, s[i])
	}
	return string(append(out, '"'))
}
