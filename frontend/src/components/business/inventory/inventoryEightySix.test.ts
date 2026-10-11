import type { MenuCategory } from "@/api/business";
import type { InventoryItem, InventoryMenuItemStatus, InventoryRecipe } from "@/api/inventory";
import { asDollars } from "@/types/money";
import {
  buildMenuItemIndex,
  canOfferEightySix,
  dishesUsingInventoryItem,
  inventoryStatusForMenuItem,
  mappedSellableDishes,
  resolveRecipeMenuItemId,
} from "./inventoryEightySix";

function item(p: Partial<InventoryItem>): InventoryItem {
  return {
    id: p.id ?? 1,
    business_id: 42,
    name: p.name ?? "Premium Beef",
    unit: "kg",
    current_quantity: p.current_quantity ?? 0,
    reorder_threshold: p.reorder_threshold ?? 2,
    cost_per_unit: p.cost_per_unit ?? 21.5,
    is_active: true,
    created_at: "",
    updated_at: "",
  };
}

function status(
  p: Partial<InventoryMenuItemStatus> & Pick<InventoryMenuItemStatus, "menu_item_id">,
): InventoryMenuItemStatus {
  return {
    menu_item_id: p.menu_item_id,
    menu_item_name: p.menu_item_name ?? p.menu_item_id,
    category_name: p.category_name ?? "Mains",
    manual_available: p.manual_available ?? true,
    has_recipe: p.has_recipe ?? true,
    status: p.status ?? "out_of_stock",
    max_possible_servings: p.max_possible_servings ?? 0,
    recommended_available: p.recommended_available ?? false,
    shows_warning: p.shows_warning ?? true,
    blocks_sale: p.blocks_sale ?? true,
    affected_inventory: p.affected_inventory ?? ["Premium Beef"],
    warning_inventory: p.warning_inventory ?? [],
  };
}

const recipes: InventoryRecipe[] = [
  {
    id: 10,
    business_id: 42,
    menu_item_id: "demo-steak",
    menu_item_name: "Steak Plate",
    inventory_item_id: 1,
    quantity_required: 0.35,
    created_at: "",
    updated_at: "",
  },
];

describe("inventoryEightySix", () => {
  it("indexes menu items by id with category/item positions", () => {
    const categories: MenuCategory[] = [
      {
        id: "cat-mains",
        name: "Mains",
        description: "",
        items: [
          {
            id: "demo-steak",
            name: "Steak Plate",
            description: "Grilled steak",
            price: asDollars(24),
            is_available: true,
          },
        ],
      },
    ];
    const index = buildMenuItemIndex(categories);
    expect(index["demo-steak"]).toMatchObject({
      categoryId: "cat-mains",
      categoryIndex: 0,
      itemIndex: 0,
      itemId: "demo-steak",
    });
  });

  it("offers 86 only for zero-qty SKUs that still have sellable mapped dishes", () => {
    const beef = item({ current_quantity: 0 });
    const dishes = mappedSellableDishes(beef, recipes, [
      status({ menu_item_id: "demo-steak", menu_item_name: "Steak Plate" }),
      status({
        menu_item_id: "demo-bowl",
        menu_item_name: "Harvest Bowl",
        has_recipe: false,
        status: "untracked",
      }),
    ]);
    expect(dishes.map((d) => d.menu_item_id)).toEqual(["demo-steak"]);
    expect(canOfferEightySix(beef, dishes)).toBe(true);
    expect(canOfferEightySix(item({ current_quantity: 2 }), dishes)).toBe(false);
  });

  it("does not offer 86 when the mapped dish is already marked unavailable", () => {
    const dishes = mappedSellableDishes(item({}), recipes, [
      status({
        menu_item_id: "demo-steak",
        manual_available: false,
        status: "manual_unavailable",
      }),
    ]);
    expect(dishes).toEqual([]);
    expect(canOfferEightySix(item({}), dishes)).toBe(false);
  });

  it("does not invent a mapping when no recipe links the SKU", () => {
    const dishes = mappedSellableDishes(item({ id: 9 }), recipes, [
      status({ menu_item_id: "demo-steak" }),
    ]);
    expect(dishes).toEqual([]);
  });

  // #727 B2: the stored id is the strong key — a conflicted id/name pair stays
  // on the id's dish (it may be that dish's second ingredient with a drifted
  // name). The name only decides when the id matches no live dish.
  it("trusts the stored recipe id over a drifted name", () => {
    const live = [
      { id: "demo-bowl", name: "Harvest Bowl" },
      { id: "demo-steak", name: "Steak Plate" },
    ];
    expect(
      resolveRecipeMenuItemId(
        { menu_item_id: "demo-bowl", menu_item_name: "Steak Plate" },
        live,
      ),
    ).toBe("demo-bowl");
    expect(
      resolveRecipeMenuItemId(
        { menu_item_id: "demo-steak", menu_item_name: "Steak Plate" },
        live,
      ),
    ).toBe("demo-steak");
    // Dead id: the name is the only live referent and wins.
    expect(
      resolveRecipeMenuItemId(
        { menu_item_id: "gone-id", menu_item_name: "Steak Plate" },
        live,
      ),
    ).toBe("demo-steak");
  });

  // #727 B2 mirror: a multi-ingredient bowl owns two rows sharing its id; the
  // second row's name drifted onto the recipe-less steak. Both rows stay the
  // bowl's — the SKU's Blocks label must point at the bowl, never invent a
  // mapping onto the steak.
  it("keeps a drifted-name row on its stored-id dish (multi-ingredient bowl)", () => {
    const liveMenu = [
      { id: "demo-bowl", name: "Harvest Bowl" },
      { id: "demo-steak", name: "Steak Plate" },
    ];
    const bowlRecipes: InventoryRecipe[] = [
      {
        id: 9,
        business_id: 42,
        menu_item_id: "demo-bowl",
        menu_item_name: "Harvest Bowl",
        inventory_item_id: 2,
        quantity_required: 0.2,
        created_at: "",
        updated_at: "",
      },
      {
        id: 10,
        business_id: 42,
        menu_item_id: "demo-bowl",
        menu_item_name: "Steak Plate",
        inventory_item_id: 1,
        quantity_required: 0.05,
        created_at: "",
        updated_at: "",
      },
    ];
    const statuses = [
      status({
        menu_item_id: "demo-bowl",
        menu_item_name: "Harvest Bowl",
        affected_inventory: ["House Dressing"],
      }),
      status({
        menu_item_id: "demo-steak",
        menu_item_name: "Steak Plate",
        has_recipe: false,
        status: "untracked",
        blocks_sale: false,
        recommended_available: true,
        shows_warning: false,
        affected_inventory: [],
      }),
    ];
    const dressing = item({ id: 1, name: "House Dressing", current_quantity: 0 });
    expect(
      mappedSellableDishes(dressing, bowlRecipes, statuses, liveMenu).map(
        (dish) => dish.menu_item_id,
      ),
    ).toEqual(["demo-bowl"]);
    expect(
      dishesUsingInventoryItem(dressing, bowlRecipes, statuses, liveMenu).map(
        (dish) => dish.menu_item_id,
      ),
    ).toEqual(["demo-bowl"]);
  });

  // #727 shape B: the beef recipe keeps the CORRECT id but its NAME drifted to
  // "Harvest Bowl". The stored id wins (#727 B2), so the Blocks label must
  // still point at the steak, matching the backend.
  it("does not point Blocks at Harvest Bowl when the beef recipe name drifted", () => {
    const liveMenu = [
      { id: "demo-bowl", name: "Harvest Bowl" },
      { id: "demo-steak", name: "Steak Plate" },
    ];
    const driftedRecipes: InventoryRecipe[] = [
      {
        id: 9,
        business_id: 42,
        menu_item_id: "demo-bowl",
        menu_item_name: "Harvest Bowl",
        inventory_item_id: 2,
        quantity_required: 0.25,
        created_at: "",
        updated_at: "",
      },
      {
        id: 10,
        business_id: 42,
        menu_item_id: "demo-steak",
        menu_item_name: "Harvest Bowl",
        inventory_item_id: 1,
        quantity_required: 0.35,
        created_at: "",
        updated_at: "",
      },
    ];
    const statuses = [
      status({ menu_item_id: "demo-steak", menu_item_name: "Steak Plate" }),
      status({
        menu_item_id: "demo-bowl",
        menu_item_name: "Harvest Bowl",
        has_recipe: true,
        status: "ok",
        blocks_sale: false,
        recommended_available: true,
        shows_warning: false,
        affected_inventory: ["Mixed Greens"],
      }),
    ];
    const beef = item({ current_quantity: 0 });
    expect(
      mappedSellableDishes(beef, driftedRecipes, statuses, liveMenu).map(
        (dish) => dish.menu_item_id,
      ),
    ).toEqual(["demo-steak"]);
    expect(
      dishesUsingInventoryItem(beef, driftedRecipes, statuses, liveMenu).map(
        (dish) => dish.menu_item_id,
      ),
    ).toEqual(["demo-steak"]);
  });

  it("trusts the id when Menu Builder names are translated", () => {
    const liveMenu = [
      { id: "demo-bowl", name: "Bowl de la cosecha" },
      { id: "demo-steak", name: "Plato de bistec" },
    ];
    const honestById: Record<string, InventoryMenuItemStatus> = {
      "demo-bowl": status({
        menu_item_id: "demo-bowl",
        menu_item_name: "Harvest Bowl",
        status: "ok",
        blocks_sale: false,
        recommended_available: true,
        shows_warning: false,
        affected_inventory: ["Mixed Greens"],
      }),
      "demo-steak": status({
        menu_item_id: "demo-steak",
        menu_item_name: "Steak Plate",
      }),
    };
    expect(
      inventoryStatusForMenuItem(
        { id: "demo-bowl", name: "Bowl de la cosecha" },
        honestById,
        liveMenu,
      )?.status,
    ).toBe("ok");
    expect(
      inventoryStatusForMenuItem(
        { id: "demo-steak", name: "Plato de bistec" },
        honestById,
        liveMenu,
      )?.status,
    ).toBe("out_of_stock");
  });

  it("rewrites a steak chip so 86 CTA cannot toggle Harvest Bowl", () => {
    const liveMenu = [
      { id: "demo-bowl", name: "Harvest Bowl" },
      { id: "demo-steak", name: "Steak Plate" },
    ];
    const staleStatuses = [
      status({
        menu_item_id: "demo-bowl",
        menu_item_name: "Steak Plate",
      }),
    ];
    const staleById: Record<string, InventoryMenuItemStatus> = {
      "demo-bowl": staleStatuses[0],
    };
    expect(
      inventoryStatusForMenuItem(
        { id: "demo-bowl", name: "Harvest Bowl" },
        staleById,
        liveMenu,
      ),
    ).toBeUndefined();
    expect(
      inventoryStatusForMenuItem(
        { id: "demo-steak", name: "Steak Plate" },
        staleById,
        liveMenu,
      )?.menu_item_id,
    ).toBe("demo-steak");
    // #727 B2: the recipe's stored id owns the row — the drifted name no
    // longer moves the mapping onto the steak.
    expect(
      mappedSellableDishes(
        item({ current_quantity: 0 }),
        [
          {
            id: 10,
            business_id: 42,
            menu_item_id: "demo-bowl",
            menu_item_name: "Steak Plate",
            inventory_item_id: 1,
            quantity_required: 0.35,
            created_at: "",
            updated_at: "",
          },
        ],
        staleStatuses,
        liveMenu,
      ).map((dish) => dish.menu_item_id),
    ).toEqual(["demo-bowl"]);
  });
});
