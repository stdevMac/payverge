package server

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

// waiterToolRepresentations builds guest wire parts and the persisted ToolCalls
// JSON exclusively from already-validated tool calls. Invalid/hallucinated calls
// must never reach either representation.
func waiterToolRepresentations(calls []llm.ToolCall) ([]gin.H, string) {
	parts := make([]gin.H, 0, len(calls))
	wire := make([]gin.H, 0, len(calls))
	for _, tc := range calls {
		fc := gin.H{"name": tc.Name, "args": tc.Args}
		parts = append(parts, gin.H{"functionCall": fc})
		wire = append(wire, fc)
	}
	if len(wire) == 0 {
		return parts, ""
	}
	encoded, _ := json.Marshal(wire)
	return parts, string(encoded)
}

func normalizeName(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

// validateCartToolCalls resolves stable IDs only against Task 2's orderable
// snapshot projections. The name-only branch is temporary V1 compatibility;
// it still resolves server-side to one unambiguous stable ID before emitting a
// canonical call.
// ValidateCartToolCallsForContract is the production cart ID gate used by the
// AI Waiter serializer and hermetic aicontract adapters.
func ValidateCartToolCallsForContract(calls []llm.ToolCall, cats []database.MenuCategory, bundles []database.Bundle) ([]llm.ToolCall, int) {
	return validateCartToolCalls(calls, cats, bundles)
}

type canonicalCartEntity struct {
	itemType    string
	id          string
	displayName string
	price       float64
	currency    string
}

func validateCartToolCalls(calls []llm.ToolCall, cats []database.MenuCategory, bundles []database.Bundle) ([]llm.ToolCall, int) {
	itemByID := make(map[string]canonicalCartEntity)
	duplicateItemIDs := make(map[string]struct{})
	entitiesByName := make(map[string][]canonicalCartEntity)
	for _, cat := range cats {
		if strings.TrimSpace(cat.ID) == "" {
			continue
		}
		for _, it := range cat.Items {
			id := strings.TrimSpace(it.ID)
			name := strings.TrimSpace(it.Name)
			if id == "" || name == "" || !it.IsAvailable {
				continue
			}
			if _, exists := itemByID[id]; exists {
				delete(itemByID, id)
				duplicateItemIDs[id] = struct{}{}
				continue
			}
			if _, duplicate := duplicateItemIDs[id]; duplicate {
				continue
			}
			entity := canonicalCartEntity{
				itemType: "menu_item", id: id, displayName: name,
				price: it.Price, currency: strings.TrimSpace(it.Currency),
			}
			itemByID[id] = entity
		}
	}
	bundleByID := make(map[string]canonicalCartEntity)
	duplicateBundleIDs := make(map[string]struct{})
	for _, b := range bundles {
		name := strings.TrimSpace(b.Name)
		if b.ID == 0 || b.BusinessID == 0 || name == "" || !b.IsActive {
			continue
		}
		id := strconv.FormatUint(uint64(b.ID), 10)
		if _, exists := bundleByID[id]; exists {
			delete(bundleByID, id)
			duplicateBundleIDs[id] = struct{}{}
			continue
		}
		if _, duplicate := duplicateBundleIDs[id]; duplicate {
			continue
		}
		entity := canonicalCartEntity{
			itemType: "bundle", id: id, displayName: name,
			price: b.Price, currency: strings.TrimSpace(b.Currency),
		}
		bundleByID[id] = entity
	}
	for _, entity := range itemByID {
		name := normalizeName(entity.displayName)
		entitiesByName[name] = append(entitiesByName[name], entity)
	}
	for _, entity := range bundleByID {
		name := normalizeName(entity.displayName)
		entitiesByName[name] = append(entitiesByName[name], entity)
	}

	var out []llm.ToolCall
	dropped := 0
	for _, tc := range calls {
		if tc.Name != "add_to_cart" {
			out = append(out, tc)
			continue
		}
		entity, ok := resolveCanonicalCartEntity(tc.Args, itemByID, bundleByID, entitiesByName)
		if !ok {
			dropped++
			continue
		}

		q, okQ := toInt(tc.Args["quantity"])
		if !okQ || q < 1 || q > 20 {
			dropped++
			continue
		}
		notes, notesOK := canonicalCartNotes(tc.Args)
		if !notesOK {
			dropped++
			continue
		}

		args := map[string]any{
			"item_type": entity.itemType, "item_name": entity.displayName,
			"price": entity.price, "currency": entity.currency, "quantity": float64(q),
		}
		if entity.itemType == "bundle" {
			args["bundle_id"] = entity.id
		} else {
			args["menu_item_id"] = entity.id
		}
		if notes != "" {
			args["notes"] = notes
		}
		tc.Args = args
		out = append(out, tc)
	}
	return out, dropped
}

func canonicalCartNotes(args map[string]any) (string, bool) {
	raw, exists := args["notes"]
	if !exists || raw == nil {
		return "", true
	}
	notes, ok := raw.(string)
	if !ok {
		return "", false
	}
	return services.SanitizePromptField(notes, 200), true
}

func resolveCanonicalCartEntity(
	args map[string]any,
	itemByID map[string]canonicalCartEntity,
	bundleByID map[string]canonicalCartEntity,
	entitiesByName map[string][]canonicalCartEntity,
) (canonicalCartEntity, bool) {
	rawItemID, hasItemKey := args["menu_item_id"]
	rawBundleID, hasBundleKey := args["bundle_id"]
	itemSet := hasItemKey && rawItemID != nil
	bundleSet := hasBundleKey && rawBundleID != nil
	if itemSet && bundleSet {
		return canonicalCartEntity{}, false
	}
	if itemSet {
		id, ok := stableStringID(rawItemID)
		if !ok {
			return canonicalCartEntity{}, false
		}
		entity, ok := itemByID[id]
		return entity, ok
	}
	if bundleSet {
		id, ok := stableBundleID(rawBundleID)
		if !ok {
			return canonicalCartEntity{}, false
		}
		entity, ok := bundleByID[id]
		return entity, ok
	}
	if hasItemKey || hasBundleKey {
		return canonicalCartEntity{}, false
	}

	name := normalizeName(toStr(args["item_name"]))
	matches := entitiesByName[name]
	if rawItemType, hasItemType := args["item_type"]; hasItemType {
		itemType, ok := rawItemType.(string)
		itemType = strings.ToLower(strings.TrimSpace(itemType))
		if !ok || (itemType != "menu_item" && itemType != "bundle") {
			return canonicalCartEntity{}, false
		}
		filtered := make([]canonicalCartEntity, 0, len(matches))
		for _, match := range matches {
			if match.itemType == itemType {
				filtered = append(filtered, match)
			}
		}
		matches = filtered
	}
	if name == "" || len(matches) != 1 {
		return canonicalCartEntity{}, false
	}
	return matches[0], true
}

func stableStringID(value any) (string, bool) {
	id, ok := value.(string)
	id = strings.TrimSpace(id)
	return id, ok && id != ""
}

func stableBundleID(value any) (string, bool) {
	const maxExactJSONInteger = 9_007_199_254_740_991
	if id, ok := stableStringID(value); ok {
		return id, true
	}
	switch number := value.(type) {
	case float64:
		if math.IsNaN(number) || math.IsInf(number, 0) || number <= 0 || number > maxExactJSONInteger || number != math.Trunc(number) {
			return "", false
		}
		return strconv.FormatFloat(number, 'f', -1, 64), true
	case int:
		if number <= 0 || number > maxExactJSONInteger {
			return "", false
		}
		return strconv.Itoa(number), true
	default:
		return "", false
	}
}

func toStr(v any) string {
	s, _ := v.(string)
	return s
}

// toInt accepts float64 (JSON numbers) and rejects non-integers.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) || n < -1_000_000 || n > 1_000_000 || n != math.Trunc(n) {
			return 0, false
		}
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}
