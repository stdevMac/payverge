package director_tools

import (
	"context"
	"fmt"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services/director_actions"
)

// ProposeContentEditTool stages (dry-run only) a content edit (description
// and/or dietary tags) for a single menu item. It NEVER writes menu data — it
// persists a pending proposal row the operator applies later.
//
// ALLERGEN STANCE: This tool may ADD a dietary tag when the owner confirms an
// ingredient. It never removes an allergen or claims a dish is safe for any
// dietary restriction.
type ProposeContentEditTool struct{}

func (t *ProposeContentEditTool) Name() string { return "propose_content_edit" }

func (t *ProposeContentEditTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Preparando edición de contenido"
	case "fr":
		return "Préparation de l'édition de contenu"
	case "ar":
		return "إعداد تحرير المحتوى"
	default:
		return "Preparing content edit"
	}
}

func (t *ProposeContentEditTool) Description() string {
	return "Stages a PROPOSED content edit (description and/or dietary tags) for a single menu item — does not change anything itself. Call when the owner wants to update an item description or add a dietary tag. May ADD a dietary tag (e.g. 'vegan', 'gluten-free') but NEVER removes an allergen or claims a dish is safe for any restriction. Returns a preview diff and a proposal the operator applies with one click. Never claim the item was changed."
}

func (t *ProposeContentEditTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"item_id":      {Type: llm.TypeString, Description: "ID of the menu item to edit."},
			"description":  {Type: llm.TypeString, Description: "New description text (max 500 runes). Omit to leave unchanged."},
			"dietary_tags": {Type: llm.TypeString, Description: "Full replacement list of dietary tags (allowed: vegan, vegetarian, gluten-free, dairy-free, nut-free, mild, low-sodium). Omit to leave unchanged."},
		},
		Required: []string{"item_id"},
	}
}

func (t *ProposeContentEditTool) Run(_ context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	if env.DB == nil {
		return ToolResult{}, fmt.Errorf("propose_content_edit: nil DB in tool env")
	}
	params, err := director_actions.ParseContentEditParams(args)
	if err != nil {
		return ToolResult{}, err
	}
	menu, categories, err := database.GetMenuByBusinessID(env.BusinessID)
	if err != nil {
		return ToolResult{}, fmt.Errorf("propose_content_edit: load menu: %w", err)
	}
	_, preview, warnings, reconfirm, err := director_actions.ComputeContentEdit(categories, params)
	if err != nil {
		return ToolResult{}, err
	}
	title := director_actions.ContentEditTitle(params)
	desc := director_actions.ContentEditDescription(preview)
	row, err := persistProposal(env, database.DirectorProposedAction{
		BusinessID:  env.BusinessID,
		ThreadID:    env.ThreadID,
		Kind:        string(director_actions.KindEditContent),
		MenuVersion: menu.Version,
	}, params, preview, title, desc, warnings, reconfirm)
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{
		Summary: title,
		Data: map[string]any{
			"proposal_id":        row.PublicID,
			"kind":               string(director_actions.KindEditContent),
			"preview":            preview,
			"warnings":           warnings,
			"requires_reconfirm": reconfirm,
			"note":               "This is a preview only. The operator must apply it.",
		},
		ProposalID: row.ID,
	}, nil
}
