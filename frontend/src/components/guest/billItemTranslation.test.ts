import { resolveBillItemTranslatedName } from "./billItemTranslation";
import type { BillItem } from "@/api/bills";
import { asDollars } from "@/types/money";

function item(
  overrides: Partial<BillItem> & Pick<BillItem, "id" | "name">,
): BillItem {
  return {
    menu_item_id: "",
    price: asDollars(10),
    quantity: 1,
    options: [],
    subtotal: asDollars(10),
    ...overrides,
  };
}

describe("resolveBillItemTranslatedName", () => {
  const translations = {
    "demo-tacos": { name: "Tacos del mercado" },
    "demo-tea": { name: "Té helado" },
    "2": { name: "Legacy position name" },
    "bundle:9": { name: "Combo del mercado" },
    "demo-child": { name: "Guacamole" },
  };

  it("prefers menu_item_id over bill-line id when both are present", () => {
    const billLine = item({
      id: "uuid-bill-line-1",
      menu_item_id: "demo-tacos",
      name: "Market Tacos",
    });
    // Even if the bill-line UUID accidentally collides with a translation key,
    // menu_item_id must win so bill names match the table menu.
    const withCollision = {
      ...translations,
      "uuid-bill-line-1": { name: "Wrong bill-line key" },
    };
    expect(resolveBillItemTranslatedName(billLine, withCollision)).toBe(
      "Tacos del mercado",
    );
  });

  it("falls back to bill-line id for legacy payloads without menu_item_id", () => {
    expect(
      resolveBillItemTranslatedName(
        item({ id: "2", name: "Iced Tea", menu_item_id: "" }),
        translations,
      ),
    ).toBe("Legacy position name");
  });

  it("translates normal menu items, bundles, and bundle children", () => {
    expect(
      resolveBillItemTranslatedName(
        item({
          id: "line-a",
          menu_item_id: "demo-tea",
          name: "Iced Tea",
          item_type: "menu_item",
        }),
        translations,
      ),
    ).toBe("Té helado");

    expect(
      resolveBillItemTranslatedName(
        item({
          id: "line-b",
          menu_item_id: "bundle:9",
          name: "Market Combo",
          item_type: "bundle",
        }),
        translations,
      ),
    ).toBe("Combo del mercado");

    expect(
      resolveBillItemTranslatedName(
        item({
          id: "line-b2",
          menu_item_id: "bundle:1",
          name: "Date Night for Two",
          item_type: "bundle",
        }),
        { "1": { name: "Cita romántica para dos" } },
      ),
    ).toBe("Cita romántica para dos");

    expect(
      resolveBillItemTranslatedName(
        item({
          id: "line-c",
          menu_item_id: "demo-child",
          name: "Guacamole",
          item_type: "bundle_item",
        }),
        translations,
      ),
    ).toBe("Guacamole");
  });

  it("keeps stored names for discounts and missing translations", () => {
    expect(
      resolveBillItemTranslatedName(
        item({
          id: "disc-1",
          menu_item_id: "offer-1",
          name: "Happy Hour −10%",
          item_type: "discount",
        }),
        { "offer-1": { name: "Hora feliz −10%" } },
      ),
    ).toBe("Happy Hour −10%");

    expect(
      resolveBillItemTranslatedName(
        item({
          id: "line-missing",
          menu_item_id: "demo-unknown",
          name: "Market Tacos",
        }),
        translations,
      ),
    ).toBe("Market Tacos");
  });
});
