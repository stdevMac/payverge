import fs from "fs";
import path from "path";
import { getTranslation } from "@/i18n/getTranslation";
import {
  kitchenAllergenLabel,
  kitchenItemsToPrepareLabel,
  kitchenNoteLabels,
  localizeKitchenOrderNote,
} from "./kitchenTicketCopy";

const LOCALES = ["en", "es", "es-AR"] as const;

describe("kitchenAllergenLabel (#776)", () => {
  it("matches menu-builder operator labels for Harvest Bowl leftovers", () => {
    expect(kitchenAllergenLabel("gluten", "en")).toBe("Gluten");
    expect(kitchenAllergenLabel("sesame", "en")).toBe("Sesame");
    expect(kitchenAllergenLabel("gluten", "es")).toBe("Gluten");
    expect(kitchenAllergenLabel("sesame", "es")).toBe("Sésamo");
    expect(kitchenAllergenLabel("gluten", "es-AR")).toBe("Gluten");
    expect(kitchenAllergenLabel("sesame", "es-AR")).toBe("Sésamo");
  });

  it("never renders the raw token or CSS-style GLUTEN/SESAME", () => {
    for (const locale of LOCALES) {
      expect(kitchenAllergenLabel("gluten", locale)).not.toBe("gluten");
      expect(kitchenAllergenLabel("sesame", locale)).not.toBe("sesame");
      expect(kitchenAllergenLabel("gluten", locale)).not.toBe("GLUTEN");
      expect(kitchenAllergenLabel("sesame", locale)).not.toBe("SESAME");
    }
  });
});

describe("localizeKitchenOrderNote (#776)", () => {
  it("rewrites leftover English suffixes and the seed table/counter prefix", () => {
    expect(
      localizeKitchenOrderNote("Table 1 - Additional Items", kitchenNoteLabels("es")),
    ).toBe("Mesa 1 - Artículos adicionales");
    expect(
      localizeKitchenOrderNote("Table 1 - Initial Order", kitchenNoteLabels("es")),
    ).toBe("Mesa 1 - Pedido inicial");
    expect(
      localizeKitchenOrderNote("Counter 2 - Initial Order", kitchenNoteLabels("es")),
    ).toBe("Barra 2 - Pedido inicial");
    expect(
      localizeKitchenOrderNote("Table 1 - Additional Items", kitchenNoteLabels("en")),
    ).toBe("Table 1 - Additional Items");
    expect(
      localizeKitchenOrderNote("Table 1 - Additional Items", kitchenNoteLabels("es-AR")),
    ).toBe("Mesa 1 - Artículos adicionales");
  });

  it("keeps custom table names in chrome notes untouched", () => {
    // Only English `<Table|Counter> <digits>` seeds are remapped — a renamed
    // table keeps its stored identity on every operator locale.
    expect(
      localizeKitchenOrderNote("Patio A - Additional Items", kitchenNoteLabels("es")),
    ).toBe("Patio A - Artículos adicionales");
    expect(
      localizeKitchenOrderNote(
        "Tablecloth Corner - Initial Order",
        kitchenNoteLabels("es"),
      ),
    ).toBe("Tablecloth Corner - Pedido inicial");
  });

  it("keeps freeform cook notes and Chopper notes untouched", () => {
    expect(
      localizeKitchenOrderNote("No onions — extra spicy", kitchenNoteLabels("es")),
    ).toBe("No onions — extra spicy");
    expect(
      localizeKitchenOrderNote("Chopper notes", kitchenNoteLabels("es")),
    ).toBe("Chopper notes");
    expect(
      localizeKitchenOrderNote("Chopper notes", kitchenNoteLabels("en")),
    ).toBe("Chopper notes");
  });

  it("resolves kitchenDisplay note keys in every operator locale", () => {
    for (const locale of LOCALES) {
      const additional = getTranslation(
        "kitchenDisplay.notes.additionalItems",
        locale,
      );
      const initial = getTranslation("kitchenDisplay.notes.initialOrder", locale);
      expect(typeof additional).toBe("string");
      expect(typeof initial).toBe("string");
      expect(additional).not.toMatch(/^kitchenDisplay\./);
      expect(initial).not.toMatch(/^kitchenDisplay\./);
    }
    expect(getTranslation("kitchenDisplay.notes.additionalItems", "es")).toBe(
      "Artículos adicionales",
    );
    expect(getTranslation("kitchenDisplay.notes.additionalItems", "es")).not.toBe(
      getTranslation("kitchenDisplay.notes.additionalItems", "en"),
    );
  });
});

describe("kitchen ticket item count pluralization (#827)", () => {
  it("uses a singular form for one item to prepare", () => {
    expect(kitchenItemsToPrepareLabel(1, "en")).toBe("1 item to prepare");
    expect(kitchenItemsToPrepareLabel(2, "en")).toBe("2 items to prepare");
    expect(kitchenItemsToPrepareLabel(1, "es")).toBe("1 artículo para preparar");
    expect(kitchenItemsToPrepareLabel(3, "es")).toBe(
      "3 artículos para preparar",
    );
    expect(kitchenItemsToPrepareLabel(1, "es-AR")).toBe(
      "1 artículo para preparar",
    );
  });

  it("Kitchen cards pick the plural form at the use sites", () => {
    const kitchen = fs.readFileSync(
      path.join(process.cwd(), "src/components/business/Kitchen.tsx"),
      "utf8",
    );
    expect(kitchen).toMatch(/kitchenItemsToPrepareLabel\(/);
    expect(kitchen).toMatch(/countKey\(\s*"needsApproval\.items"/);
    expect(kitchen).toMatch(/countKey\(\s*"order\.moreItems"/);
  });
});

describe("kitchen surfaces wire the shared helpers (#776)", () => {
  it("Kitchen list and KDS both localize chips and leftover note chrome", () => {
    const kitchen = fs.readFileSync(
      path.join(process.cwd(), "src/components/business/Kitchen.tsx"),
      "utf8",
    );
    const kds = fs.readFileSync(
      path.join(
        process.cwd(),
        "src/components/business/KitchenDisplayMode.tsx",
      ),
      "utf8",
    );
    expect(kitchen).toMatch(/KitchenAllergenChips/);
    expect(kitchen).toMatch(/localizeKitchenOrderNote/);
    expect(kds).toMatch(/KitchenAllergenChips/);
    expect(kds).toMatch(/localizeKitchenOrderNote/);
    expect(kitchen).not.toMatch(/uppercase tracking-wide/);
    expect(kds).not.toMatch(/uppercase tracking-wide/);
  });
});
