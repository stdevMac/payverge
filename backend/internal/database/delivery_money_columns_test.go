package database

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// Delivery money columns hold int64 cents (the GORM models use int64), so the
// physical type must be bigint, never numeric. A numeric column would accept a
// fractional value the Go side silently truncates.
func TestGenesisDeliveryMoneyColumnsAreBigint(t *testing.T) {
	for _, c := range [][2]string{
		{"delivery_orders", "delivery_fee"},
		{"delivery_orders", "driver_tip"},
		{"delivery_orders", "platform_fee"},
		{"delivery_zones", "delivery_fee"},
		{"delivery_zones", "minimum_order_amount"},
		{"delivery_settings", "default_delivery_fee"},
		{"delivery_settings", "free_delivery_threshold"},
		{"delivery_settings", "minimum_order_amount"},
	} {
		body := genesisTableBody(t, c[0])
		require.Regexpf(t, `(?m)^\s+`+regexp.QuoteMeta(c[1])+` bigint\b`, body,
			"%s.%s must be bigint cents", c[0], c[1])
	}
}
