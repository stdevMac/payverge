package director_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services/director_actions"
)

// ProposePriceChangeTool stages (dry-run only) a bulk/scoped price adjustment.
// It NEVER writes menu data — it persists a pending proposal row the operator
// applies later via POST /ai/director/actions/apply.
type ProposePriceChangeTool struct{}

func (t *ProposePriceChangeTool) Name() string { return "propose_price_change" }

func (t *ProposePriceChangeTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Preparando cambio de precios"
	case "fr":
		return "Préparation du changement de prix"
	case "ar":
		return "إعداد تغيير الأسعار"
	default:
		return "Preparing price change"
	}
}

func (t *ProposePriceChangeTool) Description() string {
	return "Stages a PROPOSED price change for the operator to preview and apply — does not change anything itself. Call when the owner asks to raise or lower prices (all items, a category, or one item) by a percent or flat amount. Returns a preview diff (before→after examples, affected count) and a proposal the operator applies with one click. Never claim the price was changed."
}

func (t *ProposePriceChangeTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"item_name":     {Type: llm.TypeString, Description: "Name of ONE item as the owner said it (e.g. 'Caesar Salad'). The server resolves it to the right item — ALWAYS use this when the owner names an item; never guess IDs and never widen to 'all' instead."},
			"category_name": {Type: llm.TypeString, Description: "Name of ONE category as the owner said it (e.g. 'Drinks'). The server resolves it — use when the owner names a category."},
			"scope":         {Type: llm.TypeString, Description: "Only when not using item_name/category_name: 'all' (ONLY if the owner explicitly asked to change every item on the menu), 'category:<categoryId>', or 'item:<itemId>' with a known ID."},
			"mode":          {Type: llm.TypeString, Description: "'percent' or 'flat'. Default: percent.", Enum: []string{"percent", "flat"}},
			"value":         {Type: llm.TypeNumber, Description: "Positive magnitude. percent: 20 = 20%. flat: dollars."},
			"direction":     {Type: llm.TypeString, Description: "'up' or 'down'. Default: up.", Enum: []string{"up", "down"}},
		},
		Required: []string{"value"},
	}
}

func (t *ProposePriceChangeTool) Run(_ context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	if env.DB == nil {
		return ToolResult{}, fmt.Errorf("propose_price_change: nil DB in tool env")
	}
	params, err := director_actions.ParsePriceChangeParams(args)
	if err != nil {
		return ToolResult{}, err
	}
	menu, categories, err := database.GetMenuByBusinessID(env.BusinessID)
	if err != nil {
		return ToolResult{}, fmt.Errorf("propose_price_change: load menu: %w", err)
	}
	// Resolve owner-facing names to IDs server-side — the model cannot know
	// menu IDs, and a named item must narrow the scope, never broaden it (L4-16).
	itemName, _ := args["item_name"].(string)
	categoryName, _ := args["category_name"].(string)
	params.Scope, err = resolveScopeFromNames(env.DB, env.BusinessID, categories, params.Scope, itemName, categoryName)
	if err != nil {
		return ToolResult{}, err
	}
	// Resolve the business currency so the proposal copy reads in the business's
	// own currency (AED, €, …) instead of a hardcoded "$". Best-effort → USD. (audit C6)
	currency := "USD"
	if biz, berr := database.GetBusinessByID(env.BusinessID); berr == nil && biz != nil {
		if biz.DisplayCurrency != "" {
			currency = biz.DisplayCurrency
		} else if biz.DefaultCurrency != "" {
			currency = biz.DefaultCurrency
		}
	}
	_, preview, warnings, reconfirm, err := director_actions.ComputePriceChange(categories, params, currency)
	if err != nil {
		return ToolResult{}, err // surfaced to the model as a tool error (e.g. would zero a price)
	}
	title := director_actions.PriceChangeTitleWithPreview(params, preview, currency)
	desc := director_actions.PriceChangeDescription(preview)
	margin := priceChangeMarginData(env, params.Scope, preview)
	if m, ok := margin["proposed_margin"].(float64); ok {
		title = title + fmt.Sprintf(". New margin $%s", humanizeMoney(m))
	}
	data := map[string]any{
		"kind":               string(director_actions.KindAdjustPrices),
		"preview":            preview,
		"warnings":           warnings,
		"requires_reconfirm": reconfirm,
		"note":               "This is a preview only. The operator must apply it.",
	}
	if margin != nil {
		data["margin"] = margin
	}
	if env.FreezeWrites {
		data["note"] = "Preview only. The operator asked not to apply this."
		return ToolResult{Summary: title, Data: data}, nil
	}
	row, err := persistProposal(env, database.DirectorProposedAction{
		BusinessID:  env.BusinessID,
		ThreadID:    env.ThreadID,
		Kind:        string(director_actions.KindAdjustPrices),
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

func priceChangeMarginData(env ToolEnv, scope string, preview director_actions.ActionPreview) map[string]any {
	if len(preview.Examples) == 0 {
		return nil
	}
	ex := preview.Examples[0]
	before, okB := previewFloat(ex.Before)
	after, okA := previewFloat(ex.After)
	if !okB || !okA {
		return nil
	}
	itemID := ""
	if strings.HasPrefix(scope, "item:") {
		itemID = strings.TrimPrefix(scope, "item:")
	}
	cost, ok := plateCostForItem(env, itemID, ex.Name)
	out := map[string]any{
		"item":           ex.Name,
		"current_price":  before,
		"proposed_price": after,
	}
	if !ok {
		out["unit_cost_known"] = false
		return out
	}
	out["unit_cost"] = cost
	out["current_margin"] = before - cost
	out["proposed_margin"] = after - cost
	out["unit_cost_known"] = true
	if before > 0 {
		out["current_food_cost_pct"] = cost / before
	}
	if after > 0 {
		out["proposed_food_cost_pct"] = cost / after
	}
	return out
}

// persistProposal marshals params + the full preview blob and writes a pending row.
// The preview_json blob is the ActionPreview fields (affected_count, examples, summary)
// PLUS title, description, warnings, requires_reconfirm at the same top level.
// This shape is what assembleProposedActions (director_console_service.go) decodes.
func persistProposal(env ToolEnv, base database.DirectorProposedAction, params any, preview director_actions.ActionPreview, title, desc string, warnings []string, reconfirm bool) (*database.DirectorProposedAction, error) {
	// A proposal affecting zero items is unactionable — its Apply can only ever
	// fail (or, pre-L4-17, fake-succeed). Reject here so every propose tool gets
	// the gate: the model sees the error and can re-scope instead of staging a
	// dead card in the operator's console.
	if preview.AffectedCount == 0 {
		return nil, fmt.Errorf("no menu items match the requested scope — nothing to propose; check the item/category and try again")
	}
	paramsJSON, _ := json.Marshal(params)
	previewBlob, _ := json.Marshal(struct {
		director_actions.ActionPreview
		Title             string   `json:"title"`
		Description       string   `json:"description"`
		Warnings          []string `json:"warnings"`
		RequiresReconfirm bool     `json:"requires_reconfirm"`
	}{preview, title, desc, warnings, reconfirm})
	base.ParamsJSON = string(paramsJSON)
	base.PreviewJSON = string(previewBlob)
	if base.ExpiresAt.IsZero() {
		base.ExpiresAt = time.Now().Add(director_actions.ProposalTTL)
	}
	if err := database.CreateDirectorProposedAction(&base); err != nil {
		return nil, fmt.Errorf("persist proposal: %w", err)
	}
	return &base, nil
}
