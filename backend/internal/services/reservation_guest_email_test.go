package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGuestReservationEmail(t *testing.T) {
	t.Parallel()

	t.Run("rejects undeliverable ParseAddress passers", func(t *testing.T) {
		t.Parallel()
		for _, email := range []string{
			"",
			"   ",
			"not-an-email",
			"john@gmail",
			"john@localhost",
			"a@b",
			"John <john@gmail>",
			"user@[127.0.0.1]",
			"missing-domain@",
			"@missing-local",
		} {
			_, err := ParseGuestReservationEmail(email)
			require.Error(t, err, "email %q must be rejected", email)
			require.Contains(t, strings.ToLower(err.Error()), "email",
				"error for %q must name the email problem", email)
		}
	})

	t.Run("accepts a reachable mailbox and canonicalizes display-name form", func(t *testing.T) {
		t.Parallel()
		got, err := ParseGuestReservationEmail("guest@example.com")
		require.NoError(t, err)
		require.Equal(t, "guest@example.com", got)

		got, err = ParseGuestReservationEmail("  Guest Booker <guest@example.com>  ")
		require.NoError(t, err)
		require.Equal(t, "guest@example.com", got)
	})
}
