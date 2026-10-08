package director_tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/director_actions"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestBusinessWithMenu seeds a business plus an active menu (version 1)
// with one category containing a single item (Burger, $10, available).
// Returns the business ID.
func createTestBusinessWithMenu(t *testing.T, db *database.DB, name string) uint {
	t.Helper()
	bizID := createTestBusiness(t, db, name)

	cats := []database.MenuCategory{{
		ID:   "cat-1",
		Name: "Mains",
		Items: []database.MenuItem{
			{ID: "i1", Name: "Burger", Price: 10.0, IsAvailable: true},
		},
	}}
	catsJSON, err := json.Marshal(cats)
	require.NoError(t, err)

	menu := database.Menu{
		BusinessID: bizID,
		Categories: string(catsJSON),
		IsActive:   true,
		Version:    1,
	}
	require.NoError(t, db.GetGorm().Create(&menu).Error)
	return bizID
}

// TestProposePriceChangeTool_DryRunPersistsPendingNoMutation asserts:
// 1. Run returns a non-zero ProposalID (a row was persisted).
// 2. The proposal row has status=pending and menu_version=1.
// 3. The menu Version is STILL 1 — the tool is dry-run only.
func TestProposePriceChangeTool_DryRunPersistsPendingNoMutation(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusinessWithMenu(t, db, "Price Test Bistro")

	tool := &ProposePriceChangeTool{}
	res, err := tool.Run(context.Background(),
		map[string]any{"scope": "all", "mode": "percent", "value": float64(20), "direction": "up"},
		ToolEnv{BusinessID: bizID, ThreadID: 99, DB: db},
	)
	require.NoError(t, err, "price change tool should not error for valid params")
	require.NotZero(t, res.ProposalID, "expected a persisted proposal row ID")

	// DRY-RUN INVARIANT: menu version must still be 1.
	menu, _, err := database.GetMenuByBusinessID(bizID)
	require.NoError(t, err)
	require.Equal(t, uint(1), menu.Version, "proposal tool must NOT mutate menu version")

	// Row should be pending with the correct menu version.
	row, err := database.GetDirectorProposedActionByPublicID(bizID,
		res.Data["proposal_id"].(string))
	require.NoError(t, err)
	require.Equal(t, database.DirectorProposalPending, row.Status)
	require.Equal(t, uint(1), row.MenuVersion)
	require.Equal(t, string(director_actions.KindAdjustPrices), row.Kind)

	// Preview JSON must include affected_count >= 1.
	var preview director_actions.ActionPreview
	require.NoError(t, json.Unmarshal([]byte(row.PreviewJSON), &preview))
	require.GreaterOrEqual(t, preview.AffectedCount, 1)
}

// TestProposePriceChangeTool_RejectsInjectionZeroPrice asserts that a flat
// down with a huge value (would zero a price) causes Run to return an error
// and NO proposal row is persisted.
func TestProposePriceChangeTool_RejectsInjectionZeroPrice(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusinessWithMenu(t, db, "Injection Test Bistro")

	tool := &ProposePriceChangeTool{}
	_, err := tool.Run(context.Background(),
		map[string]any{"scope": "all", "mode": "flat", "value": float64(99999), "direction": "down"},
		ToolEnv{BusinessID: bizID, ThreadID: 1, DB: db},
	)
	require.Error(t, err, "tool should reject a price change that would result in price <= 0")
	require.Contains(t, err.Error(), "price")

	// No row should have been created.
	var count int64
	db.GetGorm().Model(&database.DirectorProposedAction{}).
		Where("business_id = ?", bizID).Count(&count)
	require.Zero(t, count, "no proposal row should be persisted on compute error")
}

// createTestBusinessWithRicherMenu seeds a two-category menu for the
// name→ID scope-resolution tests: Mains (Classic Burger, Bacon Burger,
// Caesar Salad) + Drinks (Cola).
func createTestBusinessWithRicherMenu(t *testing.T, db *database.DB, name string) uint {
	t.Helper()
	bizID := createTestBusiness(t, db, name)

	cats := []database.MenuCategory{
		{
			ID:   "cat-1",
			Name: "Mains",
			Items: []database.MenuItem{
				{ID: "i1", Name: "Classic Burger", Price: 10.0, IsAvailable: true},
				{ID: "i2", Name: "Bacon Burger", Price: 12.0, IsAvailable: true},
				{ID: "i3", Name: "Caesar Salad", Price: 9.0, IsAvailable: true},
			},
		},
		{
			ID:   "cat-2",
			Name: "Drinks",
			Items: []database.MenuItem{
				{ID: "i4", Name: "Cola", Price: 3.0, IsAvailable: true},
			},
		},
	}
	catsJSON, err := json.Marshal(cats)
	require.NoError(t, err)
	menu := database.Menu{BusinessID: bizID, Categories: string(catsJSON), IsActive: true, Version: 1}
	require.NoError(t, db.GetGorm().Create(&menu).Error)
	return bizID
}

// TestProposePriceChangeTool_ItemNameNarrowsScope asserts the L4-16 fix: the
// model cannot know item IDs, so when the owner names an item the tool must
// resolve the NAME server-side and narrow to that single item — even if the
// model also passed scope "all". Before the fix a named-single-item request
// degraded to a menu-wide price change.
func TestProposePriceChangeTool_ItemNameNarrowsScope(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusinessWithRicherMenu(t, db, "NameResolve Bistro")

	tool := &ProposePriceChangeTool{}
	res, err := tool.Run(context.Background(),
		map[string]any{
			"scope":     "all", // model's bad default — the name must win
			"item_name": "caesar salad",
			"mode":      "percent", "value": float64(10), "direction": "up",
		},
		ToolEnv{BusinessID: bizID, ThreadID: 1, DB: db},
	)
	require.NoError(t, err, "a resolvable item name must stage a proposal")

	row, err := database.GetDirectorProposedActionByPublicID(bizID,
		res.Data["proposal_id"].(string))
	require.NoError(t, err)
	var params struct {
		Scope string `json:"scope"`
	}
	require.NoError(t, json.Unmarshal([]byte(row.ParamsJSON), &params))
	require.Equal(t, "item:i3", params.Scope,
		"item_name must resolve to the single named item, never broaden to 'all'")

	var preview director_actions.ActionPreview
	require.NoError(t, json.Unmarshal([]byte(row.PreviewJSON), &preview))
	require.Equal(t, 1, preview.AffectedCount)
}

// TestProposePriceChangeTool_CategoryNameNarrowsScope mirrors the item case
// for category names.
func TestProposePriceChangeTool_CategoryNameNarrowsScope(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusinessWithRicherMenu(t, db, "CatResolve Bistro")

	tool := &ProposePriceChangeTool{}
	res, err := tool.Run(context.Background(),
		map[string]any{
			"category_name": "drinks",
			"mode":          "percent", "value": float64(5), "direction": "up",
		},
		ToolEnv{BusinessID: bizID, ThreadID: 1, DB: db},
	)
	require.NoError(t, err)

	row, err := database.GetDirectorProposedActionByPublicID(bizID,
		res.Data["proposal_id"].(string))
	require.NoError(t, err)
	var params struct {
		Scope string `json:"scope"`
	}
	require.NoError(t, json.Unmarshal([]byte(row.ParamsJSON), &params))
	require.Equal(t, "category:cat-2", params.Scope)
}

// TestProposePriceChangeTool_AmbiguousItemNameRejected asserts that a name
// matching several items errors (listing the candidates) instead of guessing
// or broadening.
func TestProposePriceChangeTool_AmbiguousItemNameRejected(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusinessWithRicherMenu(t, db, "Ambiguous Bistro")

	tool := &ProposePriceChangeTool{}
	_, err := tool.Run(context.Background(),
		map[string]any{"item_name": "burger", "mode": "percent", "value": float64(10), "direction": "up"},
		ToolEnv{BusinessID: bizID, ThreadID: 1, DB: db},
	)
	require.Error(t, err, "an ambiguous item name must be rejected")
	require.Contains(t, err.Error(), "Classic Burger")
	require.Contains(t, err.Error(), "Bacon Burger")

	var count int64
	db.GetGorm().Model(&database.DirectorProposedAction{}).
		Where("business_id = ?", bizID).Count(&count)
	require.Zero(t, count, "ambiguous names must not stage a proposal")
}

// TestProposePriceChangeTool_UnknownItemNameRejected asserts that a name
// matching nothing errors instead of silently falling back to scope 'all'.
func TestProposePriceChangeTool_UnknownItemNameRejected(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusinessWithRicherMenu(t, db, "Unknown Bistro")

	tool := &ProposePriceChangeTool{}
	_, err := tool.Run(context.Background(),
		map[string]any{"item_name": "Sushi", "mode": "percent", "value": float64(10), "direction": "up"},
		ToolEnv{BusinessID: bizID, ThreadID: 1, DB: db},
	)
	require.Error(t, err, "an unknown item name must be rejected, never broadened")

	var count int64
	db.GetGorm().Model(&database.DirectorProposedAction{}).
		Where("business_id = ?", bizID).Count(&count)
	require.Zero(t, count)
}

// createTestBusinessWithMenuAndInventory seeds the standard single-item menu
// (createTestBusinessWithMenu: category "Mains" / item "Burger", id "i1")
// plus one active inventory item that is NOT on the menu — the #579
// regression fixture: "Premium Beef" is a raw stock ingredient, never a menu
// item, so it must never be misclassified as one.
func createTestBusinessWithMenuAndInventory(t *testing.T, db *database.DB, bizName, inventoryName string) uint {
	t.Helper()
	bizID := createTestBusinessWithMenu(t, db, bizName)
	item := database.InventoryItem{
		BusinessID:      bizID,
		Name:            inventoryName,
		Unit:            "lb",
		CurrentQuantity: 0,
		IsActive:        true,
	}
	require.NoError(t, db.GetGorm().Create(&item).Error)
	return bizID
}

// TestProposeAvailabilityChangeTool_InventoryIngredientClassifiedNotMenu is
// the #579 regression: "Premium Beef" is an inventory ingredient, not a menu
// item. Before the fix, an unresolved item name always returned the generic
// "no menu item named" error regardless of what the name actually was — the
// model (and ultimately the owner) had no deterministic signal that this was
// an inventory entity rather than a mistyped/missing menu item. Now the tool
// must say so explicitly so the caller can route the owner to Inventory
// instead of silently producing a menu-scoped ("mark unavailable" → Menu)
// action plan.
func TestProposeAvailabilityChangeTool_InventoryIngredientClassifiedNotMenu(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusinessWithMenuAndInventory(t, db, "Inventory Classify Bistro", "Premium Beef")

	tool := &ProposeAvailabilityChangeTool{}
	_, err := tool.Run(context.Background(),
		map[string]any{"item_name": "Premium Beef", "available": false},
		ToolEnv{BusinessID: bizID, ThreadID: 1, DB: db},
	)
	require.Error(t, err, "an inventory-only ingredient name must be rejected, never treated as a menu item")
	require.Contains(t, err.Error(), "inventory ingredient",
		"error must identify the entity type so the caller can route to Inventory, not Menu")
	require.NotContains(t, err.Error(), "no menu item named",
		"must not fall back to the generic unknown-menu-item error once classified")

	var count int64
	db.GetGorm().Model(&database.DirectorProposedAction{}).
		Where("business_id = ?", bizID).Count(&count)
	require.Zero(t, count, "no proposal row may be persisted for an inventory ingredient")
}

// TestProposePriceChangeTool_InventoryIngredientClassifiedNotMenu mirrors the
// availability-tool regression for the price-change tool: both tools share
// resolveScopeFromNames, so the classification fix must cover both call sites.
func TestProposePriceChangeTool_InventoryIngredientClassifiedNotMenu(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusinessWithMenuAndInventory(t, db, "Inventory Price Bistro", "Premium Beef")

	tool := &ProposePriceChangeTool{}
	_, err := tool.Run(context.Background(),
		map[string]any{"item_name": "Premium Beef", "mode": "percent", "value": float64(10), "direction": "up"},
		ToolEnv{BusinessID: bizID, ThreadID: 1, DB: db},
	)
	require.Error(t, err, "an inventory-only ingredient name must be rejected, never treated as a menu item")
	require.Contains(t, err.Error(), "inventory ingredient")
}

// TestProposeAvailabilityChangeTool_MenuItemStillResolvesWithInventoryPresent
// asserts a real menu item still resolves correctly — menu routes keep
// working — even when the business also has inventory items seeded.
// Inventory classification only runs after menu-name resolution fails, so it
// must never shadow a genuine menu-name match.
func TestProposeAvailabilityChangeTool_MenuItemStillResolvesWithInventoryPresent(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusinessWithMenuAndInventory(t, db, "Menu Still Works Bistro", "Premium Beef")

	tool := &ProposeAvailabilityChangeTool{}
	res, err := tool.Run(context.Background(),
		map[string]any{"item_name": "Burger", "available": false},
		ToolEnv{BusinessID: bizID, ThreadID: 1, DB: db},
	)
	require.NoError(t, err, "a real menu item must still resolve even when inventory items exist")

	row, err := database.GetDirectorProposedActionByPublicID(bizID,
		res.Data["proposal_id"].(string))
	require.NoError(t, err)
	var params struct {
		Target string `json:"target"`
	}
	require.NoError(t, json.Unmarshal([]byte(row.ParamsJSON), &params))
	require.Equal(t, "item:i1", params.Target, "a genuine menu item must resolve normally, unaffected by inventory classification")
}

// TestProposeAvailabilityChangeTool_ItemNameNarrowsTarget gives the
// availability tool the same name→ID resolution (86 "Cola" must not 86 the
// whole menu).
func TestProposeAvailabilityChangeTool_ItemNameNarrowsTarget(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusinessWithRicherMenu(t, db, "AvailResolve Bistro")

	tool := &ProposeAvailabilityChangeTool{}
	res, err := tool.Run(context.Background(),
		map[string]any{"item_name": "cola", "available": false},
		ToolEnv{BusinessID: bizID, ThreadID: 1, DB: db},
	)
	require.NoError(t, err)

	row, err := database.GetDirectorProposedActionByPublicID(bizID,
		res.Data["proposal_id"].(string))
	require.NoError(t, err)
	var params struct {
		Target string `json:"target"`
	}
	require.NoError(t, json.Unmarshal([]byte(row.ParamsJSON), &params))
	require.Equal(t, "item:i4", params.Target,
		"item_name must narrow the availability target to the named item")
	// Single-item 86 must not demand the all-off reconfirm.
	require.False(t, res.Data["requires_reconfirm"].(bool))
}

// TestProposeScopedTools_SchemasExposeNameResolution asserts both scoped
// propose tools advertise item_name/category_name so the model has a correct
// alternative to guessing IDs or defaulting to 'all'.
func TestProposeScopedTools_SchemasExposeNameResolution(t *testing.T) {
	price := (&ProposePriceChangeTool{}).Schema()
	require.Contains(t, price.Properties, "item_name")
	require.Contains(t, price.Properties, "category_name")
	require.NotContains(t, price.Properties["scope"].Description, "Default: all.",
		"scope copy must not nudge the model toward menu-wide changes")

	avail := (&ProposeAvailabilityChangeTool{}).Schema()
	require.Contains(t, avail.Properties, "item_name")
	require.Contains(t, avail.Properties, "category_name")
	require.NotContains(t, avail.Properties["target"].Description, "Default: all.")
}

// TestProposePriceChangeTool_RejectsZeroMatchScope asserts that a scope
// matching no items (e.g. a hallucinated item ID) is rejected at PROPOSE time
// and persists nothing. Before the L4-17 fix this produced a pending proposal
// whose preview said "0 items" but whose Apply button fake-succeeded.
func TestProposePriceChangeTool_RejectsZeroMatchScope(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusinessWithMenu(t, db, "ZeroMatch Price Bistro")

	tool := &ProposePriceChangeTool{}
	_, err := tool.Run(context.Background(),
		map[string]any{"scope": "item:does-not-exist", "mode": "percent", "value": float64(10), "direction": "up"},
		ToolEnv{BusinessID: bizID, ThreadID: 1, DB: db},
	)
	require.Error(t, err, "a zero-match scope must be rejected, not staged")
	require.Contains(t, err.Error(), "no menu items match")

	var count int64
	db.GetGorm().Model(&database.DirectorProposedAction{}).
		Where("business_id = ?", bizID).Count(&count)
	require.Zero(t, count, "no proposal row may be persisted for a zero-match scope")
}

// TestProposeAvailabilityChangeTool_RejectsZeroMatchTarget mirrors the
// zero-match gate for the availability tool (the gate lives in the shared
// persist path, so all propose tools get it).
func TestProposeAvailabilityChangeTool_RejectsZeroMatchTarget(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusinessWithMenu(t, db, "ZeroMatch Avail Bistro")

	tool := &ProposeAvailabilityChangeTool{}
	_, err := tool.Run(context.Background(),
		map[string]any{"target": "item:does-not-exist", "available": false},
		ToolEnv{BusinessID: bizID, ThreadID: 1, DB: db},
	)
	require.Error(t, err, "a zero-match target must be rejected, not staged")
	require.Contains(t, err.Error(), "no menu items match")

	var count int64
	db.GetGorm().Model(&database.DirectorProposedAction{}).
		Where("business_id = ?", bizID).Count(&count)
	require.Zero(t, count, "no proposal row may be persisted for a zero-match target")
}

// TestProposeContentEditTool_AddOnlyAllergensRefused asserts that a
// non-canonical dietary tag is rejected and no row is persisted.
func TestProposeContentEditTool_AddOnlyAllergensRefused(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusinessWithMenu(t, db, "Tag Test Bistro")

	tool := &ProposeContentEditTool{}
	_, err := tool.Run(context.Background(),
		map[string]any{
			"item_id":      "i1",
			"dietary_tags": []any{"definitely-safe"}, // not in canonical set
		},
		ToolEnv{BusinessID: bizID, ThreadID: 1, DB: db},
	)
	require.Error(t, err, "non-canonical dietary tag should be rejected")

	var count int64
	db.GetGorm().Model(&database.DirectorProposedAction{}).
		Where("business_id = ?", bizID).Count(&count)
	require.Zero(t, count, "no proposal row should be persisted on validation error")
}

// TestProposeAvailabilityChangeTool_DryRunPersistsPendingNoMutation checks the
// availability tool also persists a pending row without mutating the menu.
func TestProposeAvailabilityChangeTool_DryRunPersistsPendingNoMutation(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusinessWithMenu(t, db, "Avail Test Bistro")

	tool := &ProposeAvailabilityChangeTool{}
	res, err := tool.Run(context.Background(),
		map[string]any{"target": "all", "available": false},
		ToolEnv{BusinessID: bizID, ThreadID: 5, DB: db},
	)
	require.NoError(t, err)
	require.NotZero(t, res.ProposalID)

	// DRY-RUN INVARIANT.
	menu, _, err := database.GetMenuByBusinessID(bizID)
	require.NoError(t, err)
	require.Equal(t, uint(1), menu.Version)

	row, err := database.GetDirectorProposedActionByPublicID(bizID,
		res.Data["proposal_id"].(string))
	require.NoError(t, err)
	require.Equal(t, database.DirectorProposalPending, row.Status)
	require.Equal(t, string(director_actions.KindSetAvailability), row.Kind)
	// All-off → should require reconfirm.
	require.True(t, res.Data["requires_reconfirm"].(bool))
}

// TestProposeContentEditTool_DryRunPersistsPendingNoMutation checks the
// content tool also persists a pending row without mutating the menu.
func TestProposeContentEditTool_DryRunPersistsPendingNoMutation(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusinessWithMenu(t, db, "Content Test Bistro")

	desc := "A juicy burger with fresh ingredients"
	tool := &ProposeContentEditTool{}
	res, err := tool.Run(context.Background(),
		map[string]any{"item_id": "i1", "description": desc},
		ToolEnv{BusinessID: bizID, ThreadID: 7, DB: db},
	)
	require.NoError(t, err)
	require.NotZero(t, res.ProposalID)

	// DRY-RUN INVARIANT.
	menu, _, err := database.GetMenuByBusinessID(bizID)
	require.NoError(t, err)
	require.Equal(t, uint(1), menu.Version)

	row, err := database.GetDirectorProposedActionByPublicID(bizID,
		res.Data["proposal_id"].(string))
	require.NoError(t, err)
	require.Equal(t, database.DirectorProposalPending, row.Status)
	require.Equal(t, string(director_actions.KindEditContent), row.Kind)
}

// TestProposeTools_DescriptionsAreModelFacing asserts:
// 1. Description() is non-empty.
// 2. Description() != HumanLabel("en") — model-facing copy differs from UI pill.
func TestProposeTools_DescriptionsAreModelFacing(t *testing.T) {
	tools := []Tool{
		&ProposePriceChangeTool{},
		&ProposeAvailabilityChangeTool{},
		&ProposeContentEditTool{},
	}
	for _, tl := range tools {
		desc := tl.Description()
		label := tl.HumanLabel("en")
		require.NotEmpty(t, desc, "%s: Description must not be empty", tl.Name())
		require.NotEqual(t, desc, label, "%s: Description must differ from HumanLabel", tl.Name())
	}
}

// TestProposeContentEditTool_DescriptionMentionsAllergenGuard asserts that
// the content tool's model-facing Description explicitly states it may ADD a
// dietary tag but never removes an allergen or asserts safety.
func TestProposeContentEditTool_DescriptionMentionsAllergenGuard(t *testing.T) {
	tool := &ProposeContentEditTool{}
	desc := tool.Description()
	require.Contains(t, desc, "allergen", "content tool description must mention allergen guard")
}

func createHarvestBowlMenu(t *testing.T, db *database.DB, name string) uint {
	t.Helper()
	bizID := createTestBusiness(t, db, name)
	cats := []database.MenuCategory{{
		ID:   "cat-mains",
		Name: "Mains",
		Items: []database.MenuItem{
			{ID: "demo-bowl", Name: "Harvest Bowl", Price: 18.50, Cogs: 8.50, IsAvailable: true},
			{ID: "demo-steak", Name: "Steak Plate", Price: 24.00, Cogs: 11.00, IsAvailable: true},
		},
	}}
	catsJSON, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, db.GetGorm().Create(&database.Menu{
		BusinessID: bizID, Categories: string(catsJSON), IsActive: true, Version: 1,
	}).Error)
	return bizID
}

func TestProposePriceChangeTool_PreviewIncludesMarginAndDisplayName(t *testing.T) {
	db := newTestDB(t)
	bizID := createHarvestBowlMenu(t, db, "Margin Preview Bistro")

	tool := &ProposePriceChangeTool{}
	res, err := tool.Run(context.Background(),
		map[string]any{
			"item_name": "Harvest Bowl",
			"mode":      "flat",
			"value":     float64(2),
			"direction": "up",
		},
		ToolEnv{BusinessID: bizID, ThreadID: 1, DB: db},
	)
	require.NoError(t, err)
	require.Contains(t, res.Summary, "Harvest Bowl")
	require.NotContains(t, res.Summary, "demo-bowl")
	margin, ok := res.Data["margin"].(map[string]any)
	require.True(t, ok, "price preview must include margin math")
	assert.InDelta(t, 8.50, margin["unit_cost"].(float64), 0.01)
	assert.InDelta(t, 18.50, margin["current_price"].(float64), 0.01)
	assert.InDelta(t, 20.50, margin["proposed_price"].(float64), 0.01)
	assert.InDelta(t, 10.00, margin["current_margin"].(float64), 0.01)
	assert.InDelta(t, 12.00, margin["proposed_margin"].(float64), 0.01)
	assert.Contains(t, res.Summary, "12")
}

func TestProposePriceChangeTool_FreezeWritesDoesNotPersistProposal(t *testing.T) {
	db := newTestDB(t)
	bizID := createHarvestBowlMenu(t, db, "Freeze Price Bistro")

	tool := &ProposePriceChangeTool{}
	res, err := tool.Run(context.Background(),
		map[string]any{
			"item_name": "Harvest Bowl",
			"mode":      "flat",
			"value":     float64(2),
			"direction": "up",
		},
		ToolEnv{BusinessID: bizID, ThreadID: 1, DB: db, FreezeWrites: true},
	)
	require.NoError(t, err)
	require.Zero(t, res.ProposalID)
	assert.Nil(t, res.Data["proposal_id"])
	margin, ok := res.Data["margin"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, 12.00, margin["proposed_margin"].(float64), 0.01)

	var count int64
	db.GetGorm().Model(&database.DirectorProposedAction{}).
		Where("business_id = ?", bizID).Count(&count)
	require.Zero(t, count)
}

func TestProposeAvailabilityChangeTool_FreezeWritesDoesNotPersistProposal(t *testing.T) {
	db := newTestDB(t)
	bizID := createHarvestBowlMenu(t, db, "Freeze Avail Bistro")

	tool := &ProposeAvailabilityChangeTool{}
	res, err := tool.Run(context.Background(),
		map[string]any{"item_name": "Steak Plate", "available": false},
		ToolEnv{BusinessID: bizID, ThreadID: 1, DB: db, FreezeWrites: true},
	)
	require.NoError(t, err)
	require.Zero(t, res.ProposalID)
	require.Contains(t, res.Summary, "Steak Plate")
	require.NotContains(t, res.Summary, "demo-steak")

	var count int64
	db.GetGorm().Model(&database.DirectorProposedAction{}).
		Where("business_id = ?", bizID).Count(&count)
	require.Zero(t, count)
}

func TestProposeAvailabilityChangeTool_TitleUsesDisplayName(t *testing.T) {
	db := newTestDB(t)
	bizID := createHarvestBowlMenu(t, db, "Avail Title Bistro")

	tool := &ProposeAvailabilityChangeTool{}
	res, err := tool.Run(context.Background(),
		map[string]any{"item_name": "Steak Plate", "available": false},
		ToolEnv{BusinessID: bizID, ThreadID: 1, DB: db},
	)
	require.NoError(t, err)
	require.Contains(t, res.Summary, "Steak Plate")
	require.NotContains(t, res.Summary, "demo-steak")
}
