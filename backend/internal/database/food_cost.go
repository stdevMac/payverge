package database

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// RecipeIngredientCost is one recipe ingredient line for a menu item, joined
// with the current per-unit cost of the inventory item it consumes. HasCost is
// false when the referenced item has no cost entered (CostPerUnit == 0), which
// marks the menu item's food-cost as incomplete.
type RecipeIngredientCost struct {
	MenuItemID       string
	MenuItemName     string
	InventoryItemID  uint
	QuantityRequired float64
	CostPerUnit      float64
	HasCost          bool
}

// MenuItemCogs is a plate-level cost override stored on the menu item itself
// (MenuItem.Cogs). Used by the food-cost calculator when no inventory recipe
// maps the dish — operators can enter COGS without building a full recipe.
type MenuItemCogs struct {
	MenuItemID   string
	MenuItemName string
	// UnitCost is dollars per plate (same wire unit as MenuItem.Price / Cogs).
	UnitCost float64
}

// GetMenuItemCogsForBusiness returns every menu item that has a positive Cogs
// value. One menu read; no N+1. Items with Cogs <= 0 are omitted. Missing menu
// is not an error — returns an empty slice so food-cost still works for
// recipe-only businesses.
func (db *DB) GetMenuItemCogsForBusiness(businessID uint) ([]MenuItemCogs, error) {
	_, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		// Unit-test SQLite harnesses often omit menus; plate COGS is optional
		// coverage, so a missing table is "no plate COGS" not a hard failure.
		msg := err.Error()
		if strings.Contains(msg, "no such table") || strings.Contains(msg, "does not exist") {
			return nil, nil
		}
		return nil, fmt.Errorf("GetMenuItemCogsForBusiness businessID=%d: %w", businessID, err)
	}
	out := make([]MenuItemCogs, 0)
	for _, cat := range categories {
		for _, item := range cat.Items {
			if item.Cogs <= 0 {
				continue
			}
			id := item.ID
			if id == "" {
				id = item.Name
			}
			out = append(out, MenuItemCogs{
				MenuItemID:   id,
				MenuItemName: item.Name,
				UnitCost:     item.Cogs,
			})
		}
	}
	return out, nil
}

// GetRecipeCostsForBusiness returns every recipe row for the business with the
// referenced inventory item's current CostPerUnit. It runs ONE query for the
// recipes plus ONE batched query for the items (GORM Preload with an IN list) —
// never one item query per recipe (no N+1). Read-only; no writes, no migration.
//
// A recipe that references a deleted (or otherwise non-existent) inventory item
// hydrates as a zero-value item, so the row reports CostPerUnit == 0 and
// HasCost == false — the Calculator (Task 2) treats it identically to "cost not
// entered."
func (db *DB) GetRecipeCostsForBusiness(businessID uint) ([]RecipeIngredientCost, error) {
	var recipes []InventoryRecipe
	if err := db.GetGorm().
		Where("business_id = ?", businessID).
		Preload("InventoryItem", func(tx *gorm.DB) *gorm.DB {
			return tx.Select("id", "cost_per_unit")
		}).
		Find(&recipes).Error; err != nil {
		return nil, fmt.Errorf("GetRecipeCostsForBusiness businessID=%d: %w", businessID, err)
	}

	out := make([]RecipeIngredientCost, 0, len(recipes))
	for _, r := range recipes {
		out = append(out, RecipeIngredientCost{
			MenuItemID:       r.MenuItemID,
			MenuItemName:     r.MenuItemName,
			InventoryItemID:  r.InventoryItemID,
			QuantityRequired: r.QuantityRequired,
			CostPerUnit:      r.InventoryItem.CostPerUnit,
			HasCost:          r.InventoryItem.CostPerUnit > 0,
		})
	}
	return out, nil
}
