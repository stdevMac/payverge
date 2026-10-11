/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { MenuItemCard } from "../MenuItemCard";
import type { MenuItem } from "@/api/business";
import { asDollars } from "@/types/money";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));

const baseItem: MenuItem = {
  id: "steak",
  name: "Steak Plate",
  description: "Grilled",
  price: asDollars(42),
  is_available: true,
  orderability_state: "available",
};

const baseProps = {
  item: baseItem,
  originalCategoryIndex: 0,
  originalItemIndex: 0,
  tString: (key: string) => key,
  handleEditItem: jest.fn(),
  handleDeleteItem: jest.fn(),
  handleToggleEightySix: jest.fn(),
  viewMode: "list" as const,
};

describe("MenuItemCard eighty-six action", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("exposes a one-tap 86 control without opening edit", () => {
    render(<MenuItemCard {...baseProps} />);
    const mark = screen.getByRole("button", { name: /eightySix.markAria/ });
    expect(screen.getByText("eightySix.mark")).toBeVisible();
    fireEvent.click(mark);
    expect(baseProps.handleToggleEightySix).toHaveBeenCalledWith(0, 0);
    expect(baseProps.handleEditItem).not.toHaveBeenCalled();
  });

  it("keeps the labeled 86 control on grid cards", () => {
    render(<MenuItemCard {...baseProps} viewMode="grid" />);
    expect(screen.getByRole("button", { name: /eightySix.markAria/ })).toBeVisible();
    expect(screen.getByText("eightySix.mark")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: /eightySix.markAria/ }));
    expect(baseProps.handleToggleEightySix).toHaveBeenCalledWith(0, 0);
    expect(baseProps.handleEditItem).not.toHaveBeenCalled();
  });

  it("shows restore when the dish is manually 86'd", () => {
    render(
      <MenuItemCard
        {...baseProps}
        item={{
          ...baseItem,
          is_available: false,
          orderability_state: "manual_disabled",
        }}
      />,
    );
    expect(screen.getByText("eightySix.badge")).toBeVisible();
    fireEvent.click(
      screen.getByRole("button", { name: /eightySix.restoreAria/ }),
    );
    expect(baseProps.handleToggleEightySix).toHaveBeenCalledWith(0, 0);
  });

  it("86s steak from item_orderability even when the summary chip is missing", () => {
    render(
      <MenuItemCard
        {...baseProps}
        item={{
          ...baseItem,
          id: "demo-steak",
          name: "Steak Plate",
          is_available: true,
          orderability_state: "inventory_out",
          inventory_status: "out_of_stock",
        }}
        inventoryStatus={{
          menu_item_id: "demo-steak",
          menu_item_name: "Steak Plate",
          category_name: "Mains",
          manual_available: true,
          has_recipe: true,
          status: "ok",
          max_possible_servings: 1,
          recommended_available: true,
          shows_warning: false,
          blocks_sale: false,
          affected_inventory: [],
          warning_inventory: [],
        }}
      />,
    );
    expect(screen.getByText("inventoryStatus.outBadge")).toBeVisible();
    // #727: the dish inventory already blocks must not offer the plain
    // "Mark 86" affordance, which reads as "this is currently sellable".
    expect(screen.queryByText("eightySix.mark")).not.toBeInTheDocument();
    expect(screen.getByText("eightySix.markInventoryOut")).toBeVisible();
  });

  it("does not 86 Harvest Bowl from leftover catalog inventory_status when orderability is available", () => {
    render(
      <MenuItemCard
        {...baseProps}
        item={{
          ...baseItem,
          id: "demo-bowl",
          name: "Harvest Bowl",
          is_available: true,
          orderability_state: "available",
          inventory_status: "out_of_stock",
          dietary_tags: ["vegetarian"],
        }}
      />,
    );
    expect(screen.queryByText("inventoryStatus.outBadge")).not.toBeInTheDocument();
    expect(screen.queryByText("inventoryStatus.warningBadge")).not.toBeInTheDocument();
    expect(screen.getByText("eightySix.mark")).toBeVisible();
  });

  it("does not 86 Harvest Bowl from an inverted summary row when orderability is available", () => {
    render(
      <MenuItemCard
        {...baseProps}
        item={{
          ...baseItem,
          id: "demo-bowl",
          name: "Harvest Bowl",
          is_available: true,
          orderability_state: "available",
          inventory_status: "out_of_stock",
          dietary_tags: ["vegetarian"],
        }}
        inventoryStatus={{
          menu_item_id: "demo-bowl",
          menu_item_name: "Harvest Bowl",
          category_name: "Mains",
          manual_available: true,
          has_recipe: true,
          status: "out_of_stock",
          max_possible_servings: 0,
          recommended_available: false,
          shows_warning: true,
          blocks_sale: true,
          affected_inventory: ["Premium Beef"],
          warning_inventory: [],
        }}
      />,
    );
    expect(screen.queryByText("inventoryStatus.outBadge")).not.toBeInTheDocument();
    expect(screen.queryByText("inventoryStatus.warningBadge")).not.toBeInTheDocument();
    expect(screen.getByText("eightySix.mark")).toBeVisible();
  });

  it("does not paint a sellable dish as 86'd when inventory is empty", () => {
    render(
      <MenuItemCard
        {...baseProps}
        item={{
          ...baseItem,
          is_available: true,
          orderability_state: "inventory_out",
        }}
      />,
    );
    expect(screen.queryByText("eightySix.badge")).not.toBeInTheDocument();
    expect(screen.queryByText("status.unavailable")).not.toBeInTheDocument();
    expect(screen.getByText("inventoryStatus.outBadge")).toBeVisible();
    expect(screen.getByText("eightySix.markInventoryOut")).toBeVisible();
  });

  // #727 live repro: business 86 operator menu returns Steak Plate as
  //   parsed_categories[].is_available = true      (manual 86 flag, untouched)
  //   item_orderability["demo-steak"] = { orderable:false, state:"inventory_out" }
  // Menu Builder painted the inventory pill but still labelled its one-tap
  // action "Mark 86", so the card read as "sellable, tap to pull it".
  it("names the inventory block on the 86 control instead of offering a plain Mark 86", () => {
    render(
      <MenuItemCard
        {...baseProps}
        item={{
          ...baseItem,
          id: "demo-steak",
          name: "Steak Plate",
          is_available: true,
          orderability_state: "inventory_out",
        }}
      />,
    );
    const control = screen.getByRole("button", {
      name: /eightySix.markInventoryOutAria/,
    });
    expect(control).toBeVisible();
    // Tooltip carries the "why", not just the button label.
    expect(control).toHaveAttribute("title", "eightySix.markInventoryOutAria");
    expect(
      screen.queryByRole("button", { name: /eightySix.markAria/ }),
    ).not.toBeInTheDocument();
    // Still a real manual-86 action: pinning the dish off survives a restock.
    fireEvent.click(control);
    expect(baseProps.handleToggleEightySix).toHaveBeenCalledWith(0, 0);
  });

  it("ships operator copy that names the blocked dish", () => {
    const en = require("@/i18n/messages/en/businessDashboard.json");
    const copy = en.dashboard.menuBuilder.eightySix;
    expect(copy.markInventoryOut).toBeTruthy();
    expect(copy.markInventoryOut).not.toBe(copy.mark);
    expect(copy.markInventoryOutAria).toContain("{name}");
  });

  it("keeps the plain Mark 86 control on a dish inventory does not block", () => {
    render(
      <MenuItemCard
        {...baseProps}
        item={{
          ...baseItem,
          id: "demo-bowl",
          name: "Harvest Bowl",
          is_available: true,
          orderability_state: "available",
          dietary_tags: ["vegetarian"],
        }}
      />,
    );
    expect(screen.getByText("eightySix.mark")).toBeVisible();
    expect(
      screen.queryByText("eightySix.markInventoryOut"),
    ).not.toBeInTheDocument();
  });

  it("keeps restore, not the inventory label, once the dish is manually 86'd too", () => {
    render(
      <MenuItemCard
        {...baseProps}
        item={{
          ...baseItem,
          id: "demo-steak",
          name: "Steak Plate",
          is_available: false,
          orderability_state: "inventory_out",
        }}
      />,
    );
    expect(screen.getByText("eightySix.restore")).toBeVisible();
    expect(
      screen.queryByText("eightySix.markInventoryOut"),
    ).not.toBeInTheDocument();
  });

  it("disables 86 when the translated view cannot mutate", () => {
    render(
      <MenuItemCard
        {...baseProps}
        canMutate={false}
        editLanguageName="English"
      />,
    );
    const mark = screen.getByRole("button", { name: /eightySix.markAria/ });
    expect(mark).toBeDisabled();
    fireEvent.click(mark);
    expect(baseProps.handleToggleEightySix).not.toHaveBeenCalled();
  });
  // Payload after #727: the operator menu reports the EFFECTIVE flag, so an
  // inventory-blocked dish arrives as is_available:false + manual_available:true
  // — the same is_available guests get. The card must still tell the two apart:
  // inventory pulled this dish, the operator did not.
  describe("effective is_available with manual_available (post-#727 payload)", () => {
    const inventoryBlocked: MenuItem = {
      ...baseItem,
      id: "demo-bife",
      name: "Bife de chorizo",
      is_available: false,
      manual_available: true,
      orderability_state: "inventory_out",
      inventory_status: "out_of_stock",
    };

    it("reads as inventory-blocked, not hand-pulled", () => {
      render(<MenuItemCard {...baseProps} item={inventoryBlocked} />);
      expect(screen.getByText("inventoryStatus.outBadge")).toBeVisible();
      expect(screen.queryByText("eightySix.badge")).not.toBeInTheDocument();
      expect(screen.queryByText("status.unavailable")).not.toBeInTheDocument();
    });

    it("names the block on the control instead of offering restore", () => {
      render(<MenuItemCard {...baseProps} item={inventoryBlocked} />);
      expect(screen.getByText("eightySix.markInventoryOut")).toBeVisible();
      expect(screen.queryByText("eightySix.mark")).not.toBeInTheDocument();
      // The regression this guards: reading the effective flag as the manual
      // one turns the control into "Back on", which would write is_available
      // true on a dish whose operator switch was never off.
      expect(screen.queryByText("eightySix.restore")).not.toBeInTheDocument();
    });

    it("keeps the inventory badge visible on grid cards", () => {
      render(<MenuItemCard {...baseProps} item={inventoryBlocked} viewMode="grid" />);
      expect(screen.getByText("inventoryStatus.outBadge")).toBeVisible();
    });

    it("still reports a hand-pulled dish as manually 86'd", () => {
      render(
        <MenuItemCard
          {...baseProps}
          item={{
            ...inventoryBlocked,
            manual_available: false,
            orderability_state: "manual_disabled",
            inventory_status: undefined,
          }}
        />,
      );
      expect(screen.getByText("eightySix.badge")).toBeVisible();
      expect(screen.getByText("eightySix.restore")).toBeVisible();
    });
  });
});
