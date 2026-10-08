package services

import (
	"github.com/stdevmac/payverge/backend/internal/money"
)

// QuoteLine is a single authoritative, minor-unit priced line.
type QuoteLine struct {
	Key            string
	UnitPriceCents int64
	Quantity       int
}

// QuotedLine is the cent-exact result for one input line.
type QuotedLine struct {
	Key            string `json:"key"`
	LineType       string `json:"line_type"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	Quantity       int    `json:"quantity"`
	SubtotalCents  int64  `json:"subtotal_cents"`
}

// QuoteInput contains only integer minor units and basis points.
type QuoteInput struct {
	Lines              []QuoteLine
	DiscountBPS        int64
	FixedDiscountCents int64
}

// Quote is the internal cent-exact pricing result. HTTP handlers convert these
// values to decimal major units only while serializing response DTOs.
type Quote struct {
	SubtotalCents int64 `json:"subtotal_cents"`
	DiscountCents int64 `json:"discount_cents"`
	// TotalCents is the legacy promotion-engine net subtotal. Public response
	// serializers expose it as net_subtotal, never as the final payable total.
	TotalCents       int64        `json:"total_cents"`
	NetSubtotalCents int64        `json:"net_subtotal_cents"`
	TaxCents         int64        `json:"tax_cents"`
	ServiceFeeCents  int64        `json:"service_fee_cents"`
	TipCents         int64        `json:"tip_cents"`
	FinalTotalCents  int64        `json:"final_total_cents"`
	Lines            []QuotedLine `json:"lines"`
}

func percentageDiscount(cents, basisPoints int64) int64 {
	return (cents*basisPoints + 5000) / 10000
}

func applySettlementRates(quote *Quote, taxRate, serviceFeeRate float64, tipCents int64) {
	if quote == nil {
		return
	}
	if tipCents < 0 {
		tipCents = 0
	}
	quote.NetSubtotalCents = quote.TotalCents
	quote.TaxCents = money.PercentageCents(quote.NetSubtotalCents, taxRate)
	quote.ServiceFeeCents = money.PercentageCents(quote.NetSubtotalCents, serviceFeeRate)
	quote.TipCents = tipCents
	quote.FinalTotalCents = quote.NetSubtotalCents + quote.TaxCents + quote.ServiceFeeCents + quote.TipCents
}
