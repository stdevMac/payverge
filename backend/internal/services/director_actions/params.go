package director_actions

import (
	"fmt"
	"strings"
)

// canonicalDietaryTags mirrors internal/services/menu_enum_validation.go:canonicalDietaryTagIDs.
// Do NOT cross-import — that would create an import cycle. The guard test
// TestDietaryTagsMatchCanonical will fail if these diverge.
var canonicalDietaryTags = map[string]struct{}{
	"vegan": {}, "vegetarian": {}, "gluten-free": {}, "dairy-free": {},
	"nut-free": {}, "mild": {}, "low-sodium": {},
}

// PriceChangeParams is the validated payload for menu.adjust_prices.
type PriceChangeParams struct {
	Scope     string  `json:"scope"`     // "all" | "category:<id>" | "item:<id>"
	Mode      string  `json:"mode"`      // "percent" | "flat"
	Value     float64 `json:"value"`     // > 0
	Direction string  `json:"direction"` // "up" | "down"
}

// AvailabilityParams is the validated payload for menu.set_availability.
type AvailabilityParams struct {
	Target    string `json:"target"` // "all" | "category:<id>" | "item:<id>"
	Available bool   `json:"available"`
}

// ContentEditParams is the validated payload for menu.edit_content.
type ContentEditParams struct {
	ItemID      string    `json:"item_id"`
	Description *string   `json:"description,omitempty"`  // nil = unchanged
	DietaryTags *[]string `json:"dietary_tags,omitempty"` // nil = unchanged
}

// ParsePriceChangeParams parses and validates price change tool args.
func ParsePriceChangeParams(args map[string]any) (PriceChangeParams, error) {
	p := PriceChangeParams{
		Scope:     strOr(args, "scope", "all"),
		Mode:      strOr(args, "mode", "percent"),
		Direction: strOr(args, "direction", "up"),
	}
	v, ok := args["value"].(float64)
	if !ok {
		return p, fmt.Errorf("propose_price_change: value must be a number")
	}
	p.Value = v
	if err := validateScope(p.Scope); err != nil {
		return p, err
	}
	if p.Mode != "percent" && p.Mode != "flat" {
		return p, fmt.Errorf("propose_price_change: mode must be percent|flat")
	}
	if p.Direction != "up" && p.Direction != "down" {
		return p, fmt.Errorf("propose_price_change: direction must be up|down")
	}
	if p.Value <= 0 {
		return p, fmt.Errorf("propose_price_change: value must be > 0")
	}
	return p, nil
}

// ParseAvailabilityParams parses and validates availability change tool args.
func ParseAvailabilityParams(args map[string]any) (AvailabilityParams, error) {
	p := AvailabilityParams{
		Target: strOr(args, "target", "all"),
	}
	if av, ok := args["available"].(bool); ok {
		p.Available = av
	}
	if err := validateScope(p.Target); err != nil {
		return p, fmt.Errorf("propose_availability_change: %w", err)
	}
	return p, nil
}

// ParseContentEditParams parses and validates content edit tool args.
func ParseContentEditParams(args map[string]any) (ContentEditParams, error) {
	p := ContentEditParams{}
	itemID, ok := args["item_id"].(string)
	if !ok || itemID == "" {
		return p, fmt.Errorf("propose_content_edit: item_id is required")
	}
	p.ItemID = itemID

	if desc, ok := args["description"].(string); ok {
		p.Description = &desc
	}
	if rawTags, ok := args["dietary_tags"].([]any); ok {
		tags := make([]string, 0, len(rawTags))
		for _, rt := range rawTags {
			tag, ok := rt.(string)
			if !ok {
				return p, fmt.Errorf("propose_content_edit: dietary_tags must be strings")
			}
			tags = append(tags, tag)
		}
		p.DietaryTags = &tags
	}

	// Validate dietary tags against canonical set.
	if p.DietaryTags != nil {
		for _, tag := range *p.DietaryTags {
			if _, ok := canonicalDietaryTags[tag]; !ok {
				return p, fmt.Errorf("propose_content_edit: unknown dietary tag %q (allowed: vegan, vegetarian, gluten-free, dairy-free, nut-free, mild, low-sodium)", tag)
			}
		}
	}

	if p.Description == nil && p.DietaryTags == nil {
		return p, fmt.Errorf("propose_content_edit: at least one of description or dietary_tags must be provided")
	}
	return p, nil
}

// validateScope accepts "all", "category:<nonempty>", "item:<nonempty>".
func validateScope(scope string) error {
	if scope == "all" {
		return nil
	}
	if strings.HasPrefix(scope, "category:") && len(scope) > len("category:") {
		return nil
	}
	if strings.HasPrefix(scope, "item:") && len(scope) > len("item:") {
		return nil
	}
	return fmt.Errorf("invalid scope %q (allowed: all, category:<id>, item:<id>)", scope)
}

// strOr reads a string arg with a default.
func strOr(args map[string]any, key, def string) string {
	if v, ok := args[key].(string); ok && v != "" {
		return v
	}
	return def
}
