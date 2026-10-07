/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { MenuItemCard } from "./MenuItemCard";
import type { MenuItem } from "../../../../api/business";
import type { InventoryMenuItemStatus } from "../../../../api/inventory";

// Passthrough translator: returns the i18n key verbatim (repo modal-test convention).
const tString = (key: string) => key;

const item: MenuItem = {
  name: "Burger",
  description: "Beef, cheese, lettuce",
  price: 12,
  currency: "USD",
  is_available: true,
  allergens: [],
  dietary_tags: [],
} as unknown as MenuItem;

const baseProps = {
  item,
  originalCategoryIndex: 0,
  originalItemIndex: 0,
  tString,
  handleEditItem: jest.fn(),
  handleDeleteItem: jest.fn(),
  inventoryStatus: undefined,
};

// Edit + more-actions must be ≥44px touch targets. Delete is tucked into the
// overflow menu (#116) so it is not adjacent to Edit on the card face.
function assertTouchTargets() {
  const edit = screen.getByRole("button", { name: "buttons.edit Burger" });
  const more = screen.getByRole("button", {
    name: "buttons.moreActions Burger",
  });
  for (const btn of [edit, more]) {
    expect(btn.className).toContain("h-11");
    expect(btn.className).toContain("w-11");
    expect(btn.className).toContain("[@media(hover:hover)_and_(pointer:fine)]:h-8");
    expect(btn.className).not.toContain("[@media(hover:hover)]:h-7");
    expect(btn.className).not.toContain("[@media(hover:hover)]:h-8");
  }
  // Destructive delete must NOT sit as a face-level sibling of Edit.
  expect(
    screen.queryByRole("button", { name: "buttons.delete Burger" }),
  ).not.toBeInTheDocument();
}

function assertRevealGating(labelledBy: string) {
  const edit = screen.getByLabelText(labelledBy);
  const container = edit.parentElement?.className ?? "";
  expect(container).not.toContain("opacity-0");
  expect(container).not.toContain("[@media(hover:hover)_and_(pointer:fine)]:opacity-0");
  expect(container).not.toContain("[@media(hover:hover)]:opacity-0");
}

function assertSemanticActions() {
  expect(screen.queryByRole("button", { name: /^Burger$/ })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "buttons.edit Burger" })).toBeVisible();
  expect(
    screen.getByRole("button", { name: "buttons.moreActions Burger" }),
  ).toBeVisible();
}

it("grid view: edit/more are ≥44px touch targets; delete is in overflow", async () => {
  render(<MenuItemCard {...baseProps} viewMode="grid" />);
  assertTouchTargets();
  assertRevealGating("buttons.edit Burger");
  assertSemanticActions();
  const edit = screen.getByRole("button", { name: "buttons.edit Burger" });
  expect(edit.parentElement?.className).toContain("z-20");

  fireEvent.click(screen.getByTestId("menu-item-more-actions"));
  const deleteAction = await screen.findByTestId("menu-item-delete-action");
  expect(deleteAction).toBeInTheDocument();
  fireEvent.click(deleteAction);
  expect(baseProps.handleDeleteItem).toHaveBeenCalledWith(0, 0);
});

it("list view: edit/more are ≥44px touch targets; delete is in overflow", () => {
  render(<MenuItemCard {...baseProps} viewMode="list" />);
  assertTouchTargets();
  assertRevealGating("buttons.edit Burger");
  assertSemanticActions();
});

it("list view: each drag handle names the menu item it reorders", () => {
  const fries = { ...item, name: "Fries" };
  render(
    <>
      <MenuItemCard {...baseProps} viewMode="list" />
      <MenuItemCard {...baseProps} item={fries} originalItemIndex={1} viewMode="list" />
    </>,
  );

  expect(screen.getByRole("button", { name: "buttons.reorder Burger" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "buttons.reorder Fries" })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "buttons.reorder" })).not.toBeInTheDocument();
});

function inventoryStatus(
  overrides: Partial<InventoryMenuItemStatus>,
): InventoryMenuItemStatus {
  return {
    menu_item_id: "burger",
    menu_item_name: "Burger",
    category_name: "Mains",
    manual_available: true,
    has_recipe: true,
    status: "out_of_stock",
    max_possible_servings: 0,
    recommended_available: false,
    shows_warning: true,
    blocks_sale: false,
    affected_inventory: ["Beef"],
    warning_inventory: [],
    ...overrides,
  };
}

it("labels depleted warn-mode inventory as sales allowed", () => {
  render(
    <MenuItemCard
      {...baseProps}
      inventoryStatus={inventoryStatus({ blocks_sale: false })}
      viewMode="grid"
    />,
  );

  expect(screen.getByText("inventoryStatus.warningBadge")).toBeVisible();
  expect(screen.queryByText("inventoryStatus.outBadge")).not.toBeInTheDocument();
});

it("reserves the unavailable inventory badge for hard-block mode", () => {
  render(
    <MenuItemCard
      {...baseProps}
      inventoryStatus={inventoryStatus({ blocks_sale: true })}
      viewMode="grid"
    />,
  );

  expect(screen.getByText("inventoryStatus.outBadge")).toBeVisible();
  expect(screen.queryByText("inventoryStatus.warningBadge")).not.toBeInTheDocument();
});

it("reveals hidden allergen chips via popover (+N is not inert) (#185)", async () => {
  const rich = {
    ...item,
    allergens: ["gluten", "dairy", "eggs", "treenuts"],
  } as unknown as MenuItem;
  render(<MenuItemCard {...baseProps} item={rich} viewMode="grid" />);
  const overflow = screen.getByTestId("menu-item-hidden-chips");
  expect(overflow).toHaveTextContent("+1");
  fireEvent.click(overflow);
  // Popover content lists the hidden allergen label(s).
  expect(
    await screen.findByText("items.allergenNames.treenuts"),
  ).toBeInTheDocument();
});
