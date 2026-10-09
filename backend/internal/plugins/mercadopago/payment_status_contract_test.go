package mercadopago

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMercadoPagoLifecycleStatusNormalization(t *testing.T) {
	t.Parallel()
	require.Equal(t, "disputed", mercadoPagoPaymentStatus("in_mediation"))
	require.Equal(t, "reversed", mercadoPagoPaymentStatus("charged_back"))
	require.Equal(t, "refunded", mercadoPagoPaymentStatus("refunded"))
	require.Equal(t, "cancelled", mercadoPagoPaymentStatus("cancelled"))
}
