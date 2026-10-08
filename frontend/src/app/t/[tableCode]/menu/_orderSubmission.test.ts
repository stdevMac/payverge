import {
  buildGuestOrderItems,
  buildCartSignature,
  cartItemContainsBlockedMenuItem,
} from "./_orderSubmission";
import type { CartItem } from "./_types";

const cart: CartItem[] = [
  {
    itemType: "menu_item",
    name: "Burger",
    price: 10,
    quantity: 2,
    menuItemId: "item-1",
    addOns: [{ name: "Extra cheese", price: 1.5 }],
    specialRequests: "no onions",
  },
  {
    itemType: "bundle",
    name: "Combo",
    price: 15,
    quantity: 1,
    bundleId: 7,
    menuItemId: "bundle:7",
  },
];

describe("buildGuestOrderItems", () => {
  it("maps cart lines to the wire payload (dollars, options, type metadata)", () => {
    const items = buildGuestOrderItems(cart, () => undefined);
    expect(items[0]).toMatchObject({
      menu_item_name: "Burger",
      menu_item_id: "item-1",
      quantity: 2,
      price: 10,
      item_type: "menu_item",
      special_requests: "no onions",
    });
    expect(items[0].options).toHaveLength(1);
    expect(items[0].options[0]).toMatchObject({
      name: "Extra cheese",
      price_change: 1.5,
      is_required: false,
    });
    expect(typeof items[0].options[0].id).toBe("string");
    expect(items[1]).toMatchObject({
      item_type: "bundle",
      bundle_id: 7,
      menu_item_id: "bundle:7",
    });
  });

  it("resolves a missing menu_item_id through the lookup", () => {
    const items = buildGuestOrderItems(
      [
        {
          itemType: "menu_item",
          name: "Soup",
          price: 5,
          quantity: 1,
          menuItemId: "",
        },
      ],
      (lowerName) => (lowerName === "soup" ? "item-soup" : undefined),
    );
    expect(items[0].menu_item_id).toBe("item-soup");
  });
});

describe("buildCartSignature (G-3)", () => {
  it("derives from cart content only — no path/bill/notes discriminators", () => {
    const items = buildGuestOrderItems(cart, () => undefined);
    const signature = buildCartSignature("T1", items);
    expect(signature).not.toContain('"type"');
    expect(signature).not.toContain('"billId"');
    expect(signature).not.toContain('"notes"');
    // Same cart → same signature, whichever submit path runs.
    expect(
      buildCartSignature(
        "T1",
        buildGuestOrderItems(cart, () => undefined),
      ),
    ).toBe(signature);
    // Different table or different cart → different signature.
    expect(buildCartSignature("T2", items)).not.toBe(signature);
  });
});

describe("cartItemContainsBlockedMenuItem", () => {
  it("re-evaluates the current cart after indexes shift and removes only authoritative blocked lines", () => {
    const blockedIds = new Set(["item-burger"]);
    const bundleChildren = new Map<number, string[]>([
      [7, ["item-burger", "item-fries"]],
    ]);
    const isBlocked = (item: CartItem) =>
      cartItemContainsBlockedMenuItem(
        item,
        blockedIds,
        (bundleId) => bundleChildren.get(bundleId) || [],
      );

    const cartWhenToastOpened: CartItem[] = [
      {
        itemType: "menu_item",
        name: "Burger",
        price: 10,
        quantity: 1,
        menuItemId: "item-burger",
      },
      {
        itemType: "menu_item",
        name: "Salad",
        price: 8,
        quantity: 1,
        menuItemId: "item-salad",
      },
    ];
    expect(
      cartWhenToastOpened.filter(isBlocked).map((item) => item.name),
    ).toEqual(["Burger"]);

    // A cart edit inserts a line before the old blocked index and adds a bundle
    // containing the same blocked child before the toast action is clicked.
    const cartAtActionTime: CartItem[] = [
      {
        itemType: "menu_item",
        name: "Fries",
        price: 4,
        quantity: 1,
        menuItemId: "item-fries",
      },
      ...cartWhenToastOpened,
      {
        itemType: "bundle",
        name: "Burger Combo",
        price: 15,
        quantity: 1,
        bundleId: 7,
        menuItemId: "bundle:7",
      },
    ];

    expect(
      cartAtActionTime
        .filter((item) => !isBlocked(item))
        .map((item) => item.name),
    ).toEqual(["Fries", "Salad"]);
  });

  it("resolves a restored direct cart line without an id before matching blocked ids", () => {
    const restoredLine = {
      itemType: "menu_item" as const,
      name: "Burger",
      price: 10,
      quantity: 1,
      menuItemId: "",
    };

    expect(
      cartItemContainsBlockedMenuItem(
        restoredLine,
        new Set(["item-burger"]),
        () => [],
        (lowerName) => (lowerName === "burger" ? "item-burger" : undefined),
      ),
    ).toBe(true);
  });
});
