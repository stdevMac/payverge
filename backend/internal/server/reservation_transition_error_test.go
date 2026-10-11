package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// B-15 (CO-1): a handler that fails on a database constraint returns a generic
// message; the response body contains no SQLSTATE, no relation name, and no
// constraint name, and the same message is not concatenated twice.
//
// Production DELETE reservation 400 body was verbatim:
//
//	ERROR: new row for relation "businesses" violates check constraint
//	"businesses_has_canonical_owner" (SQLSTATE 23514); ERROR: new row for
//	relation "businesses" violates check constraint
//	"businesses_has_canonical_owner" (SQLSTATE 23514)
//
// The doubling is GORM AddError joining two identical PgErrors with "; ".
func TestRespondReservationTransitionError_B15_NoDBLeak(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pgErr := &pgconn.PgError{
		Severity:       "ERROR",
		Code:           "23514",
		Message:        `new row for relation "businesses" violates check constraint "businesses_has_canonical_owner"`,
		ConstraintName: "businesses_has_canonical_owner",
		TableName:      "businesses",
	}
	// Reproduce GORM's AddError join shape: fmt.Errorf("%v; %w", existing, next)
	// when the same constraint fires twice during association Save.
	doubled := fmt.Errorf("%v; %w", pgErr, pgErr)
	// Service layer typically wraps the driver error once more.
	wrapped := fmt.Errorf("failed to lock reservation: %w", doubled)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/reservations/1", nil)

	respondReservationTransitionError(c, wrapped)

	require.Equal(t, http.StatusBadRequest, w.Code, "transition fallthrough stays 400")
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	msg, _ := body["error"].(string)
	require.NotEmpty(t, msg, "must return a client message")

	// (a) no schema / driver internals
	for _, leak := range []string{
		"SQLSTATE",
		"constraint",
		"businesses_has_canonical_owner",
		"relation",
		"businesses",
		"23514",
	} {
		assert.NotContains(t, msg, leak, "client body must not contain %q", leak)
	}
	// Full raw driver text must not appear either.
	assert.NotContains(t, msg, pgErr.Error())
	assert.NotContains(t, msg, doubled.Error())

	// Generic, non-empty product copy — not the empty string.
	assert.NotEqual(t, "", strings.TrimSpace(msg))
	// Must not look like a raw PG severity line.
	assert.False(t, strings.HasPrefix(msg, "ERROR:"), "must not echo PG severity prefix")

	// (b) no "; " doubling of the same sentence
	if idx := strings.Index(msg, "; "); idx >= 0 {
		left := strings.TrimSpace(msg[:idx])
		right := strings.TrimSpace(msg[idx+2:])
		assert.NotEqual(t, left, right, "same message must not be concatenated twice with '; '")
	}
}

func TestRespondReservationTransitionError_B15_DomainErrorsPassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("validation message unchanged", func(t *testing.T) {
		// Existing transition validation copy from ReservationService.
		domain := errors.New("unsupported reservation transition")

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/reservations/1/cancel", nil)

		respondReservationTransitionError(c, domain)

		require.Equal(t, http.StatusBadRequest, w.Code)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "unsupported reservation transition", body["error"],
			"domain validation copy must pass through unchanged")
	})

	t.Run("status conflict stays 409 with safe copy", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/reservations/1/cancel", nil)

		respondReservationTransitionError(c, database.ErrReservationStatusConflict)

		require.Equal(t, http.StatusConflict, w.Code)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "This reservation was just updated elsewhere. Refresh and try again.", body["error"])
		assert.NotContains(t, body["error"], "SQLSTATE")
		assert.NotContains(t, body["error"], "constraint")
	})

	t.Run("capacity validation message unchanged", func(t *testing.T) {
		domain := errors.New("no available table for this reservation")

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/reservations/1/check-in", nil)

		respondReservationTransitionError(c, domain)

		require.Equal(t, http.StatusBadRequest, w.Code)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "no available table for this reservation", body["error"])
	})
}

func TestRespondReservationWriteError_B15_InheritsSanitizer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pgErr := &pgconn.PgError{
		Severity:       "ERROR",
		Code:           "23514",
		Message:        `new row for relation "businesses" violates check constraint "businesses_has_canonical_owner"`,
		ConstraintName: "businesses_has_canonical_owner",
		TableName:      "businesses",
	}
	doubled := fmt.Errorf("%v; %w", pgErr, pgErr)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/reservations/1", nil)

	respondReservationWriteError(c, doubled)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	msg, _ := body["error"].(string)
	assert.NotContains(t, msg, "SQLSTATE")
	assert.NotContains(t, msg, "businesses_has_canonical_owner")
	assert.NotContains(t, msg, "relation")
}
