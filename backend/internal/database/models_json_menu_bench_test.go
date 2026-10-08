package database

import (
	"encoding/json"
	"testing"
	"time"
)

// BenchmarkMenuResponseSerialization measures the cost of marshaling the
// payload shape that GET /api/v1/business/:customUrl/menu returns to
// unauthenticated guests. The previous SEC-2 pass had a per-bill/per-table
// bench; this slice (SEC-2 cont.) cares about the menu payload because
// it embeds []Offer and []Bundle, each of which used to embed the full
// 94-field Business struct in its JSON output. The fix flips those
// embeds to json:"-", so the per-call cost should drop noticeably and
// payload bytes/op should shrink. Run with -benchmem -count=3 to make
// regressions visible.
func BenchmarkMenuResponseSerialization(b *testing.B) {
	now := time.Now()
	mkBusiness := func(id uint) Business {
		return Business{
			ID:              id,
			BusinessId:      "biz_bench",
			Name:            "Bench Business",
			OwnerAddress:    "0xowner",
			OwnerName:       "Jane Owner",
			Email:           "owner@x.com",
			Phone:           "+15551234567",
			SettlementAddr:  "0xsettle",
			TippingAddr:     "0xtip",
			Timezone:        "Asia/Dubai",
			DefaultCurrency: "AED",
			DisplayCurrency: "AED",
		}
	}

	offers := make([]Offer, 0, 8)
	for i := 0; i < 8; i++ {
		offers = append(offers, Offer{
			ID: uint(i + 1), BusinessID: 1, Name: "Offer", Description: "x",
			DiscountType: "percentage", DiscountValue: 10, IsActive: true,
			CreatedAt: now, UpdatedAt: now, Business: mkBusiness(1),
		})
	}
	bundles := make([]Bundle, 0, 8)
	for i := 0; i < 8; i++ {
		bundles = append(bundles, Bundle{
			ID: uint(i + 1), BusinessID: 1, Name: "Bundle", Description: "x",
			Price: 99.0, Items: "[]", IsActive: true,
			CreatedAt: now, UpdatedAt: now, Business: mkBusiness(1),
		})
	}

	payload := struct {
		Offers  []Offer  `json:"offers"`
		Bundles []Bundle `json:"bundles"`
	}{Offers: offers, Bundles: bundles}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := json.Marshal(payload); err != nil {
			b.Fatal(err)
		}
	}
}
