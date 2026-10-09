package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeGenerateImageRequest_RejectsInvalid(t *testing.T) {
	valid := GenerateImageRequest{
		Name:        "House Burger",
		Description: "Angus beef",
		EntityType:  "menu_item",
		Currency:    "USD",
	}

	tests := []struct {
		name string
		mut  func(GenerateImageRequest) GenerateImageRequest
		want string
	}{
		{"blank name", func(r GenerateImageRequest) GenerateImageRequest { r.Name = "  "; return r }, "name is required"},
		{"bad entity", func(r GenerateImageRequest) GenerateImageRequest { r.EntityType = "hack"; return r }, "entity_type"},
		{"bad scope", func(r GenerateImageRequest) GenerateImageRequest { r.OfferScope = "world"; return r }, "offer_scope"},
		{"bad currency", func(r GenerateImageRequest) GenerateImageRequest { r.Currency = "XXX"; return r }, "currency"},
		{"negative price", func(r GenerateImageRequest) GenerateImageRequest { r.BundlePrice = -1; return r }, "bundle_price"},
		{"huge price", func(r GenerateImageRequest) GenerateImageRequest { r.BundlePrice = 2_000_000; return r }, "bundle_price"},
		{"too many related", func(r GenerateImageRequest) GenerateImageRequest {
			r.RelatedItemNames = make([]string, menuImageRelatedMax+1)
			for i := range r.RelatedItemNames {
				r.RelatedItemNames[i] = "x"
			}
			return r
		}, "related_item_names"},
		{"too many bundle rows", func(r GenerateImageRequest) GenerateImageRequest {
			r.BundleItems = make([]struct {
				Name     string `json:"name"`
				Quantity int    `json:"quantity"`
			}, menuImageBundleRowsMax+1)
			for i := range r.BundleItems {
				r.BundleItems[i].Name = "x"
				r.BundleItems[i].Quantity = 1
			}
			return r
		}, "bundle_items"},
		{"bad qty", func(r GenerateImageRequest) GenerateImageRequest {
			r.BundleItems = []struct {
				Name     string `json:"name"`
				Quantity int    `json:"quantity"`
			}{{Name: "Fries", Quantity: 0}}
			return r
		}, "quantity"},
		{"name too long", func(r GenerateImageRequest) GenerateImageRequest {
			r.Name = strings.Repeat("a", menuImageNameMaxRunes+1)
			return r
		}, "name exceeds"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := normalizeGenerateImageRequest(tc.mut(valid))
			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tc.want))
		})
	}
}

func TestNormalizeGenerateImageRequest_AcceptsValid(t *testing.T) {
	p, err := normalizeGenerateImageRequest(GenerateImageRequest{
		Name:             "  Family Combo ",
		Description:      "Great deal",
		EntityType:       "bundle",
		OfferScope:       "menu",
		Currency:         "ars",
		BundlePrice:      19.99,
		RelatedItemNames: []string{" Burger ", "", "Fries"},
		BundleItems: []struct {
			Name     string `json:"name"`
			Quantity int    `json:"quantity"`
		}{{Name: "Burger", Quantity: 2}, {Name: "Fries", Quantity: 1}},
	})
	require.NoError(t, err)
	assert.Equal(t, "Family Combo", p.Name)
	assert.Equal(t, "bundle", p.EntityType)
	assert.Equal(t, "menu", p.OfferScope)
	assert.Equal(t, "ARS", p.Currency)
	assert.Equal(t, []string{"Burger", "Fries"}, p.RelatedItemNames)
	require.Len(t, p.BundleItems, 2)
	assert.Equal(t, 2, p.BundleItems[0].Quantity)
}

func TestMenuImageGuardrailBrief_IncludesFields(t *testing.T) {
	p, err := normalizeGenerateImageRequest(GenerateImageRequest{
		Name: "Pizza", Description: "cheese", Ingredients: "dough",
		EntityType: "menu_item", OfferStampText: "HOT",
	})
	require.NoError(t, err)
	brief := menuImageGuardrailBrief(p)
	assert.Contains(t, brief, "Pizza")
	assert.Contains(t, brief, "cheese")
	assert.Contains(t, brief, "dough")
	assert.Contains(t, brief, "HOT")
}
