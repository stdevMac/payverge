package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatInsightText_FoodCostHigh_EN(t *testing.T) {
	out := formatInsightTextEN("food_cost_high", 3, ": Steak, Pizza", inventoryStockSplit{})
	assert.True(t, strings.Contains(out, "3"))
	assert.True(t, strings.Contains(out, "40%"))
	assert.True(t, strings.Contains(out, "Steak"))
}

func TestFormatInsightText_FoodCostHigh_ES(t *testing.T) {
	out := formatInsightTextES("food_cost_high", 2, ": Bife", inventoryStockSplit{})
	assert.True(t, strings.Contains(out, "2"))
	assert.True(t, strings.Contains(out, "40%"))
}

func TestFormatInsightText_InventoryOutDoesNotInferSaleOrMenuState(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "EN", text: formatInsightTextEN("inventory_out_of_stock", 1, ": Premium Beef", inventoryStockSplit{})},
		{name: "ES", text: formatInsightTextES("inventory_out_of_stock", 1, ": Premium Beef", inventoryStockSplit{})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lower := strings.ToLower(tt.text)
			assert.Contains(t, tt.text, "Premium Beef")
			assert.NotContains(t, lower, "menu")
			assert.NotContains(t, lower, "menú")
			assert.NotContains(t, lower, "hide")
			assert.NotContains(t, lower, "ocult")
			assert.NotContains(t, lower, "ocúlt")
			assert.NotContains(t, lower, "sold")
			assert.NotContains(t, lower, "vender")
		})
	}
}

// L1-22: negative stock uses oversold wording, not "at zero" / "en cero".
func TestFormatInsightText_InventoryOversold_NotAtZero(t *testing.T) {
	en := formatInsightTextEN("inventory_out_of_stock", 1, ": Aceite", inventoryStockSplit{OversoldCount: 1, OversoldNames: "Aceite"})
	es := formatInsightTextES("inventory_out_of_stock", 1, ": Aceite", inventoryStockSplit{OversoldCount: 1, OversoldNames: "Aceite"})
	assert.Contains(t, strings.ToLower(en), "oversold")
	assert.NotContains(t, strings.ToLower(en), "at zero")
	assert.Contains(t, strings.ToLower(es), "sobrevendido")
	assert.NotContains(t, strings.ToLower(es), "en cero")
}

// R2-8: the digest has ONE Spanish body (isSpanishLanguage covers es and
// es-AR), so its register must be the neutral tuteo every Spanish operator
// reads. The L1-22 oversold line shipped Argentine voseo ("contá y corregí")
// into that shared body.
func TestFormatInsightTextES_UsesNeutralTuteo_R2_8(t *testing.T) {
	voseo := []string{"contá", "corregí", "revisá", "reponé", "ingresá", "elegí", "fijate", "ajustá", "verificá", "hacé", "poné"}

	lines := []string{
		formatInsightTextES("inventory_out_of_stock", 2, ": Aceite, Harina", inventoryStockSplit{OversoldCount: 2, OversoldNames: "Aceite, Harina"}),
		formatInsightTextES("inventory_out_of_stock", 1, ": Aceite", inventoryStockSplit{OversoldCount: 1, OversoldNames: "Aceite"}),
		formatInsightTextES("inventory_out_of_stock", 2, ": Aceite, Harina", inventoryStockSplit{}),
		formatInsightTextES("inventory_low_stock", 3, ": Aceite", inventoryStockSplit{}),
		formatInsightTextES("stale_open_bills", 2, "", inventoryStockSplit{}),
		formatInsightTextES("ai_conversations_pending", 2, "", inventoryStockSplit{}),
		formatInsightTextES("food_cost_high", 2, ": Bife", inventoryStockSplit{}),
		formatInsightText(DigestInsight{
			Type: "inventory_out_of_stock",
			Params: map[string]interface{}{
				"count": 2, "oversold_count": 1,
				"oversold_names": []string{"Aceite"},
				"zero_names":     []string{"Harina"},
			},
		}, "es"),
	}

	for _, line := range lines {
		lower := strings.ToLower(line)
		for _, token := range voseo {
			assert.NotContains(t, lower, token, "voseo leaked into the shared Spanish digest body: %s", line)
		}
	}
}

// R2-7: a mixed set (one negative + one exactly zero) must not claim the whole
// set is oversold. has_oversold is a boolean OR across the set, so the old copy
// read "2 inventory items are oversold: Oversold Oil, Zero Flour" — a lie about
// Zero Flour, and it hid the real oversold magnitude carried in oversold_count.
func TestFormatInsightText_InventoryMixedOversoldAndZero_R2_7(t *testing.T) {
	ins := DigestInsight{
		Type: "inventory_out_of_stock",
		Params: map[string]interface{}{
			"count":          2,
			"item_names":     []string{"Oversold Oil", "Zero Flour"},
			"has_oversold":   true,
			"oversold_count": 1,
			"oversold_names": []string{"Oversold Oil"},
			"zero_names":     []string{"Zero Flour"},
		},
	}

	t.Run("EN names each group with its own count", func(t *testing.T) {
		en := formatInsightText(ins, "en")
		assert.NotContains(t, en, "2 inventory items are oversold")
		assert.Contains(t, strings.ToLower(en), "oversold (1): oversold oil")
		assert.Contains(t, strings.ToLower(en), "at zero (1): zero flour")
	})

	t.Run("ES names each group with its own count", func(t *testing.T) {
		es := formatInsightText(ins, "es")
		assert.NotContains(t, es, "2 insumos de inventario están sobrevendidos")
		assert.Contains(t, strings.ToLower(es), "sobrevendidos (1): oversold oil")
		assert.Contains(t, strings.ToLower(es), "en cero (1): zero flour")
	})

	t.Run("escapes user-controlled names in the mixed branch", func(t *testing.T) {
		hostile := DigestInsight{
			Type: "inventory_out_of_stock",
			Params: map[string]interface{}{
				"count":          2,
				"has_oversold":   true,
				"oversold_count": 1,
				"oversold_names": []string{"<script>x</script>"},
				"zero_names":     []string{"Zero Flour"},
			},
		}
		en := formatInsightText(hostile, "en")
		assert.NotContains(t, en, "<script>")
		assert.Contains(t, en, "&lt;script&gt;")
	})

	t.Run("a set that is entirely oversold keeps the single-group copy", func(t *testing.T) {
		allOversold := DigestInsight{
			Type: "inventory_out_of_stock",
			Params: map[string]interface{}{
				"count":          2,
				"item_names":     []string{"Oil", "Butter"},
				"has_oversold":   true,
				"oversold_count": 2,
				"oversold_names": []string{"Oil", "Butter"},
				"zero_names":     []string{},
			},
		}
		en := formatInsightText(allOversold, "en")
		assert.Contains(t, en, "2 inventory items are oversold")
		assert.NotContains(t, strings.ToLower(en), "at zero")
	})

	t.Run("a legacy payload without split name lists stays on the old copy", func(t *testing.T) {
		legacy := DigestInsight{
			Type: "inventory_out_of_stock",
			Params: map[string]interface{}{
				"count":        2,
				"item_names":   []string{"Oil", "Flour"},
				"has_oversold": true,
			},
		}
		en := formatInsightText(legacy, "en")
		assert.Contains(t, en, "2 inventory items are oversold")
	})
}
