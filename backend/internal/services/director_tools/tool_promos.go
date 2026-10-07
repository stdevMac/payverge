package director_tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// PromosTool is a READ of live offers and bundles. It never enables, disables,
// or edits promotions — agent 06 owns persistence / 86 cascade.
type PromosTool struct{}

func (t *PromosTool) Name() string { return "get_promos" }

func (t *PromosTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Revisando ofertas y combos"
	case "fr":
		return "Lecture des offres et menus"
	case "ar":
		return "قراءة العروض والحزم"
	default:
		return "Reading offers and bundles"
	}
}

func (t *PromosTool) Description() string {
	return "Returns live offers and bundles (name, active flag, discount or price, target). Call when the owner asks about a promo, combo, Date Night bundle, dollar-off offer, or whether a promotion still exists. This is read-only — it does not kill or edit offers."
}

func (t *PromosTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{Type: llm.TypeObject, Properties: map[string]*llm.JSONSchema{}}
}

func (t *PromosTool) Run(_ context.Context, _ map[string]any, env ToolEnv) (ToolResult, error) {
	if env.DB == nil {
		return ToolResult{}, fmt.Errorf("get_promos: nil DB in tool env")
	}

	offers, err := database.GetOffersByBusinessID(env.BusinessID)
	if err != nil {
		return ToolResult{}, fmt.Errorf("get_promos: offers: %w", err)
	}
	bundles, err := database.GetBundlesByBusinessID(env.BusinessID)
	if err != nil {
		return ToolResult{}, fmt.Errorf("get_promos: bundles: %w", err)
	}

	offerRows := make([]map[string]any, 0, len(offers))
	for _, o := range offers {
		target := ""
		if o.TargetID != nil {
			target = *o.TargetID
		}
		offerRows = append(offerRows, map[string]any{
			"name":           o.Name,
			"active":         o.IsActive,
			"discount_type":  o.DiscountType,
			"discount_value": o.DiscountValue,
			"applies_to":     o.ApplicableTo,
			"target":         target,
		})
	}
	bundleRows := make([]map[string]any, 0, len(bundles))
	for _, b := range bundles {
		bundleRows = append(bundleRows, map[string]any{
			"name":   b.Name,
			"active": b.IsActive,
			"price":  b.Price,
		})
	}

	spanish := strings.HasPrefix(strings.ToLower(strings.ReplaceAll(env.Locale, "_", "-")), "es")
	summary := promosSummary(spanish, offerRows, bundleRows)
	return ToolResult{
		Summary: summary,
		Data: map[string]any{
			"offers":        offerRows,
			"bundles":       bundleRows,
			"offers_count":  len(offerRows),
			"bundles_count": len(bundleRows),
		},
	}, nil
}

func promosSummary(spanish bool, offers, bundles []map[string]any) string {
	if len(offers) == 0 && len(bundles) == 0 {
		if spanish {
			return "No hay ofertas ni combos cargados."
		}
		return "No live offers or bundles are on the menu."
	}
	names := make([]string, 0, len(offers)+len(bundles))
	for _, o := range offers {
		if n, _ := o["name"].(string); n != "" {
			state := "on"
			if spanish {
				state = "activa"
			}
			if active, _ := o["active"].(bool); !active {
				state = "off"
				if spanish {
					state = "inactiva"
				}
			}
			names = append(names, fmt.Sprintf("%s (%s)", n, state))
		}
	}
	for _, b := range bundles {
		if n, _ := b["name"].(string); n != "" {
			state := "on"
			if spanish {
				state = "activo"
			}
			if active, _ := b["active"].(bool); !active {
				state = "off"
				if spanish {
					state = "inactivo"
				}
			}
			names = append(names, fmt.Sprintf("%s (%s)", n, state))
		}
	}
	if spanish {
		return "Ofertas y combos: " + strings.Join(names, "; ") + "."
	}
	return "Offers and bundles: " + strings.Join(names, "; ") + "."
}
