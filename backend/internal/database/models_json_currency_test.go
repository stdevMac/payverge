package database

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillMarshalJSONIncludesCurrencyAndPropagatesToPayments(t *testing.T) {
	bill := Bill{
		BusinessID:  12,
		TotalAmount: 1234,
		PaidAmount:  1234,
		Business: Business{
			DefaultCurrency: "JPY",
		},
		Payments: []Payment{
			{
				BillID:    1,
				Amount:    1234,
				TipAmount: 0,
				Status:    PaymentStatusConfirmed,
			},
		},
	}

	payload, err := json.Marshal(bill)
	require.NoError(t, err)

	var decoded struct {
		Currency string `json:"currency"`
		Payments []struct {
			Currency string  `json:"currency"`
			Amount   float64 `json:"amount"`
		} `json:"payments"`
	}
	require.NoError(t, json.Unmarshal(payload, &decoded))

	assert.Equal(t, "JPY", decoded.Currency)
	require.Len(t, decoded.Payments, 1)
	assert.Equal(t, "JPY", decoded.Payments[0].Currency)
	assert.Equal(t, 12.34, decoded.Payments[0].Amount)
}

func TestOrderResolvedCurrencyFallsBackDisplayDefaultUSD(t *testing.T) {
	assert.Equal(t, "AED", Order{Business: Business{DisplayCurrency: "AED", DefaultCurrency: "USD"}}.ResolvedCurrency())
	assert.Equal(t, "MXN", Order{Business: Business{DefaultCurrency: "MXN"}}.ResolvedCurrency())
	assert.Equal(t, "USD", Order{}.ResolvedCurrency())
	assert.Equal(t, "EUR", Order{Currency: "EUR", Business: Business{DisplayCurrency: "AED"}}.ResolvedCurrency())
}

func TestOrderMarshalJSONIncludesBusinessCurrency(t *testing.T) {
	order := Order{
		BusinessID: 7,
		Business: Business{
			DisplayCurrency: "AED",
			DefaultCurrency: "USD",
		},
	}

	payload, err := json.Marshal(order)
	require.NoError(t, err)

	var decoded struct {
		Currency string `json:"currency"`
	}
	require.NoError(t, json.Unmarshal(payload, &decoded))

	assert.Equal(t, "AED", decoded.Currency)
}
