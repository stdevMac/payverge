package server

import (
	"errors"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// Generic client-safe copy when a low-level database / driver error would
// otherwise cross the API boundary (CO-1 / B-15). Handlers must log the full
// original error server-side before returning this.
const clientSafeDBErrorMessage = "Unable to complete this request. Please try again."

// isLowLevelDBError reports whether err is (or wraps) a PostgreSQL / GORM
// driver failure whose Error() text must never reach API clients.
//
// Detection is two-layer:
//  1. Typed: errors.As *pgconn.PgError and errors.Is well-known GORM constraint
//     sentinels (the postgres dialector Translate path).
//  2. Text heuristics for shapes that still leak when TranslateError is off or
//     the error was string-wrapped before typed identity was lost: SQLSTATE,
//     "duplicate key", "violates", "constraint", `relation "`.
//
// Domain validation strings from services (e.g. "unsupported reservation
// transition", "no available table for this reservation", "cancellation window
// has closed") match none of these markers and pass through unchanged.
func isLowLevelDBError(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return true
	}
	switch {
	case errors.Is(err, gorm.ErrDuplicatedKey),
		errors.Is(err, gorm.ErrForeignKeyViolated),
		errors.Is(err, gorm.ErrCheckConstraintViolated),
		errors.Is(err, gorm.ErrInvalidField),
		errors.Is(err, gorm.ErrInvalidTransaction),
		errors.Is(err, gorm.ErrInvalidDB):
		return true
	}

	msg := err.Error()
	lower := strings.ToLower(msg)
	// Prefer precise markers over broad tokens so legit product copy never
	// collapses: "constraint" alone is not enough (and is only matched as part
	// of the PG phrases below or via the typed path).
	if strings.Contains(msg, "SQLSTATE") ||
		strings.Contains(lower, "sqlstate") ||
		strings.Contains(lower, "duplicate key") ||
		strings.Contains(lower, "duplicated key") ||
		strings.Contains(lower, "violates") ||
		strings.Contains(lower, "check constraint") ||
		strings.Contains(lower, "unique constraint") ||
		strings.Contains(lower, "foreign key constraint") ||
		strings.Contains(msg, `relation "`) ||
		strings.Contains(msg, `relation \"`) ||
		// GORM AddError join of two driver failures: "ERROR: … (SQLSTATE …); ERROR: …"
		(strings.Contains(msg, "ERROR:") && strings.Contains(msg, "(SQLSTATE")) ||
		// Common lib/pq / pgx driver prefixes that never appear in domain copy.
		strings.HasPrefix(msg, "ERROR:") ||
		strings.HasPrefix(msg, "pq: ") ||
		strings.Contains(lower, "pq: ") {
		return true
	}
	return false
}

// ClientSafeErrorMessage returns a message safe to put in a JSON error body.
// Low-level DB/driver errors collapse to clientSafeDBErrorMessage (and the
// caller should log the full err). Domain validation and other product copy
// pass through unchanged.
func ClientSafeErrorMessage(err error) string {
	if err == nil {
		return clientSafeDBErrorMessage
	}
	if isLowLevelDBError(err) {
		return clientSafeDBErrorMessage
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return clientSafeDBErrorMessage
	}
	return msg
}

// LogAndClientSafeErrorMessage is the usual handler helper: log the full
// original error server-side, return only a client-safe message.
func LogAndClientSafeErrorMessage(context string, err error) string {
	if err == nil {
		return clientSafeDBErrorMessage
	}
	if isLowLevelDBError(err) {
		log.Printf("[client-errors] %s: %v", context, err)
		return clientSafeDBErrorMessage
	}
	return ClientSafeErrorMessage(err)
}
