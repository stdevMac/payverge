package director_tools

import (
	"context"
	"fmt"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services/director_actions"
)

// ProposeAvailabilityChangeTool stages (dry-run only) an availability change
// for menu items. It NEVER writes menu data — it persists a pending proposal
// row the operator applies later via POST /ai/director/actions/apply.
type ProposeAvailabilityChangeTool struct{}

func (t *ProposeAvailabilityChangeTool) Name() string { return "propose_availability_change" }

func (t *ProposeAvailabilityChangeTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Preparando cambio de disponibilidad"
	case "fr":
		return "Préparation du changement de disponibilité"
	case "ar":
		return "إعداد تغيير التوفر"
	default:
		return "Preparing availability change"
	}
}

func (t *ProposeAvailabilityChangeTool) Description() string {
	return "Stages a PROPOSED availability change (enable or disable items) for the operator to preview and apply — does not change anything itself. Call when the owner asks to 86 an item, take something off the menu, or make something available again. Only for MENU items (dishes/drinks the business sells) — never for inventory ingredients or raw stock (e.g. a cut of meat), which are not menu items and have no availability toggle here; those belong on the Inventory tab. Returns a preview diff (affected items, before→after) and a proposal the operator applies with one click. Never claim availability was changed."
}

func (t *ProposeAvailabilityChangeTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"item_name":     {Type: llm.TypeString, Description: "Name of ONE item as the owner said it (e.g. 'Cola'). The server resolves it to the right item — ALWAYS use this when the owner names an item; never guess IDs and never widen to 'all' instead."},
			"category_name": {Type: llm.TypeString, Description: "Name of ONE category as the owner said it. The server resolves it — use when the owner names a category."},
			"target":        {Type: llm.TypeString, Description: "Only when not using item_name/category_name: 'all' (ONLY if the owner explicitly asked to change every item), 'category:<categoryId>', or 'item:<itemId>' with a known ID."},
			"available":     {Type: llm.TypeBoolean, Description: "true = make available, false = make unavailable (86)."},
		},
		Required: []string{"available"},
	}
}

func (t *ProposeAvailabilityChangeTool) Run(_ context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	if env.DB == nil {
		return ToolResult{}, fmt.Errorf("propose_availability_change: nil DB in tool env")
	}
	params, err := director_actions.ParseAvailabilityParams(args)
	if err != nil {
		return ToolResult{}, err
	}
	menu, categories, err := database.GetMenuByBusinessID(env.BusinessID)
	if err != nil {
		return ToolResult{}, fmt.Errorf("propose_availability_change: load menu: %w", err)
	}
	// Resolve owner-facing names to IDs server-side — the model cannot know
	// menu IDs, and 86'ing a named item must never widen to the whole menu (L4-16).
	itemName, _ := args["item_name"].(string)
	categoryName, _ := args["category_name"].(string)
	params.Target, err = resolveScopeFromNames(env.DB, env.BusinessID, categories, params.Target, itemName, categoryName)
	if err != nil {
		return ToolResult{}, err
	}
	_, preview, warnings, reconfirm, err := director_actions.ComputeAvailabilityChange(categories, params)
	if err != nil {
		return ToolResult{}, err
	}
	title := director_actions.AvailabilityChangeTitleWithPreview(params, preview)
	desc := director_actions.AvailabilityChangeDescription(preview)
	data := map[string]any{
		"kind":               string(director_actions.KindSetAvailability),
		"preview":            preview,
		"warnings":           warnings,
		"requires_reconfirm": reconfirm,
		"note":               "This is a preview only. The operator must apply it.",
	}
	if env.FreezeWrites {
		data["note"] = "Preview only. The operator asked not to change anything."
		return ToolResult{Summary: title, Data: data}, nil
	}
	row, err := persistProposal(env, database.DirectorProposedAction{
		BusinessID:  env.BusinessID,
		ThreadID:    env.ThreadID,
		Kind:        string(director_actions.KindSetAvailability),
		MenuVersion: menu.Version,
	}, params, preview, title, desc, warnings, reconfirm)
	if err != nil {
		return ToolResult{}, err
	}
	data["proposal_id"] = row.PublicID
	return ToolResult{
		Summary:    title,
		Data:       data,
		ProposalID: row.ID,
	}, nil
}
