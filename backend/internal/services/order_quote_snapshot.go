package services

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/stdevmac/payverge/backend/internal/database"
)

const orderQuoteSnapshotVersion = 1

type persistedOrderQuoteSnapshot struct {
	Version int   `json:"version"`
	Quote   Quote `json:"quote"`
}

func marshalOrderQuoteSnapshot(quote Quote) (database.JSONRawMessage, error) {
	if !validOrderQuoteSettlement(quote) {
		return nil, fmt.Errorf("cannot persist invalid order quote snapshot")
	}
	raw, err := json.Marshal(persistedOrderQuoteSnapshot{
		Version: orderQuoteSnapshotVersion,
		Quote:   quote,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal order quote snapshot: %w", err)
	}
	return database.JSONRawMessage(raw), nil
}

func orderQuoteFromSnapshot(raw database.JSONRawMessage, reconstructed Quote) (Quote, bool) {
	if len(raw) == 0 {
		return Quote{}, false
	}
	var snapshot persistedOrderQuoteSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil || snapshot.Version != orderQuoteSnapshotVersion {
		return Quote{}, false
	}
	if !validOrderQuoteSettlement(snapshot.Quote) || !sameOrderQuotePricing(snapshot.Quote, reconstructed) {
		return Quote{}, false
	}
	return snapshot.Quote, true
}

func validOrderQuoteSettlement(quote Quote) bool {
	if quote.SubtotalCents < 0 || quote.DiscountCents < 0 || quote.DiscountCents > quote.SubtotalCents {
		return false
	}
	if quote.TotalCents != quote.SubtotalCents-quote.DiscountCents || quote.NetSubtotalCents != quote.TotalCents {
		return false
	}
	if quote.TaxCents < 0 || quote.ServiceFeeCents < 0 || quote.TipCents < 0 || len(quote.Lines) == 0 {
		return false
	}
	finalTotal := quote.NetSubtotalCents
	for _, component := range []int64{quote.TaxCents, quote.ServiceFeeCents, quote.TipCents} {
		if component > int64(^uint64(0)>>1)-finalTotal {
			return false
		}
		finalTotal += component
	}
	return quote.FinalTotalCents == finalTotal
}

func sameOrderQuotePricing(snapshot, reconstructed Quote) bool {
	return snapshot.SubtotalCents == reconstructed.SubtotalCents &&
		snapshot.DiscountCents == reconstructed.DiscountCents &&
		snapshot.TotalCents == reconstructed.TotalCents &&
		reflect.DeepEqual(snapshot.Lines, reconstructed.Lines)
}
