package services

import (
	"errors"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func validationTestCategories() []database.MenuCategory {
	return []database.MenuCategory{{
		ID:   "cat-1",
		Name: "Mains",
		Items: []database.MenuItem{
			{
				ID: "item-burger", Name: "Burger", Price: 12, IsAvailable: true,
				Options: []database.MenuItemOption{{ID: "opt-cheese", Name: "Extra cheese", PriceChange: 1.5}},
			},
			{ID: "item-86d", Name: "Sold Out Special", Price: 9, IsAvailable: false},
		},
	}}
}

func assertOrderValidationCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %q, got nil", wantCode)
	}
	var ve *OrderValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *OrderValidationError, got %T: %v", err, err)
	}
	if ve.Code != wantCode {
		t.Fatalf("expected code %q, got %q (message: %s)", wantCode, ve.Code, ve.Message)
	}
}

func TestApplyPromotionsToOrder_RejectsUnavailableItem(t *testing.T) {
	_, err := ApplyPromotionsToOrder(validationTestCategories(), nil, nil, []PromotionInputLine{
		{Name: "Sold Out Special", MenuItemID: "item-86d", Quantity: 1, ItemType: OrderItemTypeMenuItem},
	})
	assertOrderValidationCode(t, err, OrderErrCodeItemUnavailable)
	if !strings.Contains(err.Error(), "Sold Out Special") {
		t.Fatalf("error must name the item, got: %v", err)
	}
}

func TestApplyPromotionsToOrder_RejectsUnavailableBundleItem(t *testing.T) {
	bundles := []database.Bundle{{
		ID: 7, Name: "Mystery Combo", Price: 20, IsActive: true,
		Items: `[{"menu_item_id":"item-86d","quantity":1}]`,
	}}
	_, err := ApplyPromotionsToOrder(validationTestCategories(), bundles, nil, []PromotionInputLine{
		{Name: "Mystery Combo", Quantity: 1, ItemType: OrderItemTypeBundle, BundleID: ptrUint(7)},
	})
	assertOrderValidationCode(t, err, OrderErrCodeItemUnavailable)
}

func TestApplyPromotionsToOrder_RejectsQuantityOverCap(t *testing.T) {
	_, err := ApplyPromotionsToOrder(validationTestCategories(), nil, nil, []PromotionInputLine{
		{Name: "Burger", MenuItemID: "item-burger", Quantity: MaxOrderItemQuantity + 1, ItemType: OrderItemTypeMenuItem},
	})
	assertOrderValidationCode(t, err, OrderErrCodeQuantityExceeded)
}

func TestApplyPromotionsToOrder_QuantityErrorsNameMenuItemIDWhenNameEmpty(t *testing.T) {
	_, err := ApplyPromotionsToOrder(validationTestCategories(), nil, nil, []PromotionInputLine{
		{Name: "", MenuItemID: "item-burger", Quantity: MaxOrderItemQuantity + 1, ItemType: OrderItemTypeMenuItem},
	})
	assertOrderValidationCode(t, err, OrderErrCodeQuantityExceeded)
	if !strings.Contains(err.Error(), "item-burger") {
		t.Fatalf("qty error must name the menu_item_id, got: %v", err)
	}
	if strings.Contains(err.Error(), "''") {
		t.Fatalf("qty error must not refer to an empty item name, got: %v", err)
	}

	_, err = ApplyPromotionsToOrder(validationTestCategories(), nil, nil, []PromotionInputLine{
		{Name: "", MenuItemID: "item-burger", Quantity: 0, ItemType: OrderItemTypeMenuItem},
	})
	if err == nil {
		t.Fatal("quantity 0 must be rejected")
	}
	if !strings.Contains(err.Error(), "item-burger") {
		t.Fatalf("zero-qty error must name the menu_item_id, got: %v", err)
	}
}

func TestApplyPromotionsToOrder_AllowsQuantityAtCap(t *testing.T) {
	result, err := ApplyPromotionsToOrder(validationTestCategories(), nil, nil, []PromotionInputLine{
		{Name: "Burger", MenuItemID: "item-burger", Quantity: MaxOrderItemQuantity, ItemType: OrderItemTypeMenuItem},
	})
	if err != nil {
		t.Fatalf("quantity == cap must be allowed: %v", err)
	}
	if len(result.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(result.Lines))
	}
}

func TestApplyPromotionsToOrder_RejectsTooManyLines(t *testing.T) {
	input := make([]PromotionInputLine, MaxOrderLines+1)
	for i := range input {
		input[i] = PromotionInputLine{Name: "Burger", MenuItemID: "item-burger", Quantity: 1, ItemType: OrderItemTypeMenuItem}
	}
	_, err := ApplyPromotionsToOrder(validationTestCategories(), nil, nil, input)
	assertOrderValidationCode(t, err, OrderErrCodeTooManyItems)
}

func TestApplyPromotionsToOrder_RejectsOverlongSpecialRequests(t *testing.T) {
	_, err := ApplyPromotionsToOrder(validationTestCategories(), nil, nil, []PromotionInputLine{
		{
			Name: "Burger", MenuItemID: "item-burger", Quantity: 1, ItemType: OrderItemTypeMenuItem,
			SpecialRequests: strings.Repeat("a", MaxOrderTextLen+1),
		},
	})
	assertOrderValidationCode(t, err, OrderErrCodeTextTooLong)
}

func TestApplyPromotionsToOrder_OptionNotFoundCode(t *testing.T) {
	_, err := ApplyPromotionsToOrder(validationTestCategories(), nil, nil, []PromotionInputLine{
		{
			Name: "Burger", MenuItemID: "item-burger", Quantity: 1, ItemType: OrderItemTypeMenuItem,
			Options: []database.MenuItemOption{{ID: "addon-bogus", Name: "Phantom Topping"}},
		},
	})
	assertOrderValidationCode(t, err, OrderErrCodeOptionNotFound)
}

func TestApplyPromotionsToOrder_ItemNotFoundCode(t *testing.T) {
	_, err := ApplyPromotionsToOrder(validationTestCategories(), nil, nil, []PromotionInputLine{
		{Name: "Ghost Burger", MenuItemID: "item-missing", Quantity: 1, ItemType: OrderItemTypeMenuItem},
	})
	assertOrderValidationCode(t, err, OrderErrCodeItemNotFound)
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Fatalf("producer message must keep a not-found wording for mapper regression, got: %v", err)
	}
}

func TestApplyPromotionsToOrder_BundleNotFoundCode(t *testing.T) {
	missingID := uint(999)
	_, err := ApplyPromotionsToOrder(validationTestCategories(), nil, nil, []PromotionInputLine{
		{Name: "Ghost Combo", Quantity: 1, ItemType: OrderItemTypeBundle, BundleID: &missingID},
	})
	assertOrderValidationCode(t, err, OrderErrCodeBundleNotFound)
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Fatalf("producer message must keep a not-found wording for mapper regression, got: %v", err)
	}
}
