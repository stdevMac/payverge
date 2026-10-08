package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRecordRateObservation_RejectsImplausibleJump locks F-RATEBAND part (b):
// a single observation that deviates by orders of magnitude from the prior
// accepted rate (the realistic shape of a corrupted upstream value) is rejected
// and the prior rate is kept, so a garbage rate never gets stored and locked
// into quotes.
func TestRecordRateObservation_RejectsImplausibleJump(t *testing.T) {
	s := setupExchangeRateTestService(t)
	base := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	require.NoError(t, s.RecordRateObservation("USDC", "ARS", 1000, "coinbase", base))

	// A 1000x garbage observation must be rejected (no new row, prior kept).
	require.NoError(t, s.RecordRateObservation("USDC", "ARS", 1_000_000, "coinbase", base.Add(5*time.Minute)))
	latest, err := s.GetExchangeRate("USDC", "ARS")
	require.NoError(t, err)
	require.Equal(t, float64(1000), latest.Rate, "an implausible jump must be rejected, keeping the prior rate")
	require.Equal(t, int64(1), countExchangeRateRows(t, s, "USDC", "ARS"))

	// A near-zero garbage observation is likewise rejected.
	require.NoError(t, s.RecordRateObservation("USDC", "ARS", 0.5, "coinbase", base.Add(6*time.Minute)))
	latest, err = s.GetExchangeRate("USDC", "ARS")
	require.NoError(t, err)
	require.Equal(t, float64(1000), latest.Rate)

	// A legitimate moderate move IS accepted and stored.
	require.NoError(t, s.RecordRateObservation("USDC", "ARS", 1100, "coinbase", base.Add(10*time.Minute)))
	latest, err = s.GetExchangeRate("USDC", "ARS")
	require.NoError(t, err)
	require.Equal(t, float64(1100), latest.Rate)
}

// The first observation for a pair has no prior to compare against and must
// always be accepted, regardless of magnitude.
func TestRecordRateObservation_FirstObservationAlwaysAccepted(t *testing.T) {
	s := setupExchangeRateTestService(t)
	base := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	require.NoError(t, s.RecordRateObservation("USDC", "VND", 25000, "coinbase", base))
	latest, err := s.GetExchangeRate("USDC", "VND")
	require.NoError(t, err)
	require.Equal(t, float64(25000), latest.Rate)
}
