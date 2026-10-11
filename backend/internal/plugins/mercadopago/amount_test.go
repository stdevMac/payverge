package mercadopago

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// R2: CLP remaining must floor so charged amount never exceeds remaining.
// 150050 cents → CLP 1500.50 → floor whole CLP 1500 → 150000 tracker cents
// (half-up to 150100 would exceed remaining and settle permanently rejected).
func TestCanonicalAmountCents_CLPUnroundedMatchesOrder(t *testing.T) {
	const in = int64(150050)
	canonical := CanonicalAmountCents(in, "CLP")
	require.Equal(t, int64(150000), canonical)
	require.LessOrEqual(t, canonical, in)

	// Order decimal string from the same canonical cents.
	require.Equal(t, "1500", DecimalAmount(canonical, "CLP"))
	// And from the original cents (helper floors itself).
	require.Equal(t, "1500", DecimalAmount(in, "CLP"))
	// Preference/refund major amount agrees.
	require.Equal(t, 1500.0, mercadoPagoMajorAmount(in, "CLP"))
	require.Equal(t, 1500.0, mercadoPagoMajorAmount(canonical, "CLP"))
}

// R2: sub-half-unit remainder floors to 0 (handlers must 4xx, not create 0-order).
func TestCanonicalAmountCents_CLPSubHalfUnitFloorsToZero(t *testing.T) {
	require.Equal(t, int64(0), CanonicalAmountCents(49, "CLP"))
	require.Equal(t, int64(0), CanonicalAmountCents(99, "COP"))
	require.Equal(t, "0", DecimalAmount(49, "CLP"))
}

// F4: partial COP refund must be whole units (not 2-decimal sub-units).
func TestMercadoPagoMajorAmount_COPPartialIsWholeUnit(t *testing.T) {
	// 12345 cents → COP 123.45 → whole COP 123 for MP (floor).
	require.Equal(t, 123.0, mercadoPagoMajorAmount(12345, "COP"))
	require.Equal(t, "123", DecimalAmount(12345, "COP"))
	require.Equal(t, int64(12300), CanonicalAmountCents(12345, "COP"))
	require.LessOrEqual(t, CanonicalAmountCents(12345, "COP"), int64(12345))
}

func TestCanonicalAmountCents_TwoDecimalUnchanged(t *testing.T) {
	require.Equal(t, int64(12345), CanonicalAmountCents(12345, "ARS"))
	require.Equal(t, int64(12345), CanonicalAmountCents(12345, "USD"))
	require.Equal(t, "123.45", DecimalAmount(12345, "ARS"))
	require.Equal(t, 123.45, mercadoPagoMajorAmount(12345, "ARS"))
}

func TestCanonicalAmountCents_EvenCLPUnchanged(t *testing.T) {
	require.Equal(t, int64(150000), CanonicalAmountCents(150000, "CLP"))
	require.Equal(t, "1500", DecimalAmount(150000, "CLP"))
}

func TestShouldAbsorbSettlementDust(t *testing.T) {
	// CLP/COP: remainder strictly below one major unit (100 cents) — absorb.
	require.True(t, ShouldAbsorbSettlementDust(50, "CLP"))
	require.True(t, ShouldAbsorbSettlementDust(99, "COP"))
	// Boundary: 0 and ≥100 must not absorb.
	require.False(t, ShouldAbsorbSettlementDust(0, "CLP"))
	require.False(t, ShouldAbsorbSettlementDust(100, "CLP"))
	require.False(t, ShouldAbsorbSettlementDust(150, "COP"))
	// Two-decimal currencies never absorb (even tiny remainders).
	require.False(t, ShouldAbsorbSettlementDust(50, "ARS"))
	require.False(t, ShouldAbsorbSettlementDust(1, "USD"))
}
