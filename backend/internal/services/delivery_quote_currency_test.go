package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Tests for the DEL-GT-5 quote-message slice: the below-minimum quote message
// embedded a bare amount with no currency ("Minimum delivery order is 15.00"),
// wrong for every non-USD business. The message must carry the business
// display currency, and reason codes must stay stable machine codes since the
// frontend localizes by reason_code.

func TestQuoteDeliveryBelowMinimumMessageIncludesBusinessCurrency(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-quote-currency")
	business.DefaultCurrency = "ARS"
	business.DisplayCurrency = "EUR"
	require.NoError(t, db.Save(business).Error)
	createDeliverySettings(t, db, business.ID, nil)
	createDeliveryZone(t, db, business.ID, "Downtown", nil) // zone minimum $20.00

	quote, err := service.QuoteDelivery(business.ID, DeliveryQuoteRequest{
		OrderSubtotal: 5,
		DeliveryAddress: database.DeliveryAddress{
			Street: "123 Main Street", City: "Testville", State: "CA",
			PostalCode: "12345", Country: "US",
		},
	})
	require.NoError(t, err)
	assert.False(t, quote.Eligible)
	assert.Equal(t, "below_minimum", quote.ReasonCode, "reason codes must stay stable machine codes")
	assert.Contains(t, quote.Message, "EUR 20.00", "below-minimum message must carry the business display currency")
}

func TestQuoteDeliveryBelowMinimumMessageFallsBackToDefaultCurrencyThenUSD(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)

	address := database.DeliveryAddress{
		Street: "123 Main Street", City: "Testville", State: "CA",
		PostalCode: "12345", Country: "US",
	}

	// DisplayCurrency empty → DefaultCurrency wins.
	withDefault := createTestHospitalityBusiness(t, db, "delivery-quote-default-ccy")
	withDefault.DefaultCurrency = "ARS"
	require.NoError(t, db.Save(withDefault).Error)
	createDeliverySettings(t, db, withDefault.ID, nil)
	createDeliveryZone(t, db, withDefault.ID, "Downtown", nil)

	quote, err := service.QuoteDelivery(withDefault.ID, DeliveryQuoteRequest{OrderSubtotal: 5, DeliveryAddress: address})
	require.NoError(t, err)
	assert.Equal(t, "below_minimum", quote.ReasonCode)
	assert.Contains(t, quote.Message, "ARS 20.00")

	// Neither currency configured → USD fallback.
	bare := createTestHospitalityBusiness(t, db, "delivery-quote-usd-fallback")
	createDeliverySettings(t, db, bare.ID, nil)
	createDeliveryZone(t, db, bare.ID, "Downtown", nil)

	quote, err = service.QuoteDelivery(bare.ID, DeliveryQuoteRequest{OrderSubtotal: 5, DeliveryAddress: address})
	require.NoError(t, err)
	assert.Equal(t, "below_minimum", quote.ReasonCode)
	assert.Contains(t, quote.Message, "USD 20.00")
}

// TestQuoteDeliveryReasonCodesAreStableMachineCodes pins the reason_code
// contract the frontend localizes against.
func TestQuoteDeliveryReasonCodesAreStableMachineCodes(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	address := database.DeliveryAddress{
		Street: "1 Elsewhere Road", City: "Otherville",
		PostalCode: "00000", Country: "US",
	}

	// delivery_disabled
	disabled := createTestHospitalityBusiness(t, db, "delivery-rc-disabled")
	createDeliverySettings(t, db, disabled.ID, func(s *database.DeliverySettings) {
		s.InHouseDeliveryEnabled = false
	})
	quote, err := service.QuoteDelivery(disabled.ID, DeliveryQuoteRequest{OrderSubtotal: 50, DeliveryAddress: address})
	require.NoError(t, err)
	assert.Equal(t, "delivery_disabled", quote.ReasonCode)

	// zone_unavailable
	zoned := createTestHospitalityBusiness(t, db, "delivery-rc-zone")
	createDeliverySettings(t, db, zoned.ID, nil)
	createDeliveryZone(t, db, zoned.ID, "Downtown", nil) // matches testville/12345 only
	quote, err = service.QuoteDelivery(zoned.ID, DeliveryQuoteRequest{OrderSubtotal: 50, DeliveryAddress: address})
	require.NoError(t, err)
	assert.Equal(t, "zone_unavailable", quote.ReasonCode)
}
