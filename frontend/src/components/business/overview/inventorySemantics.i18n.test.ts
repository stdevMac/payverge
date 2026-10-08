import { getTranslation } from "@/i18n/getTranslation";

const locales = ["en", "es", "es-AR"] as const;
const falseSaleClaims =
  /can't be sold|cannot be sold|hide|still on the menu|no se pueden vender|ocult|siguen? (?:visible )?en el menú/i;

describe("inventory insight semantics", () => {
  it.each(locales)(
    "%s describes depleted inventory without inferring sale or menu state",
    (locale) => {
      const overview = getTranslation(
        "businessDashboard.overview.proactive.types.inventory_out_of_stock.summary",
        locale,
        { items: "Premium Beef" },
      );
      const director = getTranslation(
        "directorConsole.preShift.cards.outOfStock.one",
        locale,
        { count: 1, names: "Premium Beef" },
      );

      for (const copy of [overview, director]) {
        expect(copy).toContain("Premium Beef");
        expect(copy).not.toMatch(falseSaleClaims);
      }
    },
  );

  it.each(locales)(
    "%s labels overview counts as inventory inputs",
    (locale) => {
      const copy = getTranslation(
        "businessDashboard.overview.quickActions.inventoryAlerts.description",
        locale,
        { out: 1, low: 2 },
      );

      expect(copy).toMatch(/ingredient|inventory|insumo|inventario/i);
    },
  );

  it.each(locales)("%s pluralizes quick inventory alerts", (locale) => {
    const zero = getTranslation(
      "businessDashboard.overview.quickActions.inventoryAlerts.descriptionOutZero",
      locale,
      { out: 0, low: 2 },
    );
    const one = getTranslation(
      "businessDashboard.overview.quickActions.inventoryAlerts.descriptionOutOne",
      locale,
      { out: 1, low: 2 },
    );
    const many = getTranslation(
      "businessDashboard.overview.quickActions.inventoryAlerts.descriptionOutMany",
      locale,
      { out: 2, low: 2 },
    );

    expect(zero).toMatch(/0 (items|insumos)/i);
    expect(one).toMatch(/1 (item|insumo)/i);
    expect(many).toMatch(/2 (items|insumos)/i);
  });

  it.each(locales)(
    "%s formats stale bill duration in the active locale",
    (locale) => {
      const duration = getTranslation(
        "businessDashboard.overview.proactive.duration.days_other",
        locale,
        { count: 26 },
      );

      expect(duration).toMatch(locale === "en" ? /26 days/ : /26 días/);
    },
  );

  it.each(locales)(
    "%s interpolates a 40-day stale-bills overview title without English units in Spanish",
    (locale) => {
      const duration = getTranslation(
        "businessDashboard.overview.proactive.duration.days_other",
        locale,
        { count: 40 },
      );
      const title = getTranslation(
        "businessDashboard.overview.proactive.types.stale_open_bills.title_other",
        locale,
        { count: 2, duration: Array.isArray(duration) ? duration.join(" ") : duration },
      );

      expect(title).toContain("2");
      if (locale === "en") {
        expect(title).toBe("2 bills open longer than 40 days");
      } else {
        expect(title).toContain("40 días");
        expect(String(title)).not.toMatch(/\bdays\b/);
      }
    },
  );
});
