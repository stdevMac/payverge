package server

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestClientSafeErrorMessage_DBShapes(t *testing.T) {
	pgErr := &pgconn.PgError{
		Severity:       "ERROR",
		Code:           "23514",
		Message:        `new row for relation "businesses" violates check constraint "businesses_has_canonical_owner"`,
		ConstraintName: "businesses_has_canonical_owner",
		TableName:      "businesses",
	}
	// Exact GORM AddError join shape that produced the production double body.
	doubled := fmt.Errorf("%v; %w", pgErr, pgErr)

	cases := []error{
		pgErr,
		doubled,
		fmt.Errorf("save: %w", pgErr),
		fmt.Errorf("ERROR: column \"x\" does not exist (SQLSTATE 42703)"),
		gorm.ErrCheckConstraintViolated,
		gorm.ErrDuplicatedKey,
		gorm.ErrForeignKeyViolated,
		errors.New(`pq: duplicate key value violates unique constraint "users_email_key"`),
	}
	for _, err := range cases {
		msg := ClientSafeErrorMessage(err)
		require.Equal(t, clientSafeDBErrorMessage, msg, "err=%v", err)
		assert.NotContains(t, msg, "SQLSTATE")
		assert.NotContains(t, msg, "constraint")
		assert.NotContains(t, msg, "relation")
		// No "; " doubling of the same sentence in the client message.
		if idx := strings.Index(msg, "; "); idx >= 0 {
			left := strings.TrimSpace(msg[:idx])
			right := strings.TrimSpace(msg[idx+2:])
			assert.NotEqual(t, left, right)
		}
	}
}

func TestClientSafeErrorMessage_DomainPassThrough(t *testing.T) {
	// Product copy that must never be flattened — proves heuristics avoid
	// false-positive collapse of legit validation messages.
	safe := []string{
		"unsupported reservation transition",
		"no available table for this reservation",
		"cancellation window has closed",
		"this reservation cannot be cancelled online",
		"cancellations are not allowed for this business",
		"party size must be between 1 and 20",
		"reservation time is in the past",
		"invalid reservation status: hacked",
		"min_party_size must be at least 1",
		"This reservation was just updated elsewhere. Refresh and try again.",
		"a valid email address is required to book online",
	}
	for _, s := range safe {
		assert.Equal(t, s, ClientSafeErrorMessage(errors.New(s)), "must pass through: %q", s)
	}
}

func TestIsLowLevelDBError_GORMSentinels(t *testing.T) {
	assert.True(t, isLowLevelDBError(fmt.Errorf("wrap: %w", gorm.ErrCheckConstraintViolated)))
	assert.False(t, isLowLevelDBError(errors.New("unsupported reservation transition")))
}
