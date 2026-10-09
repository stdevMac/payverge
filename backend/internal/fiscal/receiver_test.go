package fiscal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateReceiverOverride(t *testing.T) {
	require.NoError(t, ValidateReceiverOverride(nil))
	require.NoError(t, ValidateReceiverOverride(&ReceiverOverride{}))
	require.NoError(t, ValidateReceiverOverride(&ReceiverOverride{
		CustomerDocType: "DNI", CustomerDocNumber: "12345678",
		CustomerTaxCondition: "consumidor_final", CustomerName: "Juan",
	}))
	require.NoError(t, ValidateReceiverOverride(&ReceiverOverride{
		CustomerDocType: "CUIT", CustomerDocNumber: "20-11111111-2",
		CustomerTaxCondition: "responsable_inscripto",
	}))
	require.Error(t, ValidateReceiverOverride(&ReceiverOverride{
		CustomerDocType: "DNI", CustomerDocNumber: "12",
	}))
	require.Error(t, ValidateReceiverOverride(&ReceiverOverride{
		CustomerDocType: "CUIT", CustomerDocNumber: "20123456789",
	}))
	require.Error(t, ValidateReceiverOverride(&ReceiverOverride{
		CustomerTaxCondition: "not_a_real_condition",
	}))
}

func TestValidateCUITMod11(t *testing.T) {
	require.True(t, validateCUITMod11("20111111112"))
	require.False(t, validateCUITMod11("20123456789"))
}
