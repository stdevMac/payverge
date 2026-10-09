import { getTranslation } from "@/i18n/getTranslation";
import { countKey } from "@/i18n/countForm";

const locales = ["en", "es", "es-AR"] as const;

function copy(
  base: string,
  locale: (typeof locales)[number],
  count: number,
  params: Record<string, string | number> = { count },
): string {
  return String(getTranslation(countKey(base, count), locale, params));
}

describe("exact-count operator copy (#483)", () => {
  it.each(locales)("%s singularizes one active bill and pluralizes 0/2", (locale) => {
    const zero = copy(
      "businessDashboard.overview.quickActions.activeBills.description",
      locale,
      0,
    );
    const one = copy(
      "businessDashboard.overview.quickActions.activeBills.description",
      locale,
      1,
    );
    const two = copy(
      "businessDashboard.overview.quickActions.activeBills.description",
      locale,
      2,
    );

    if (locale === "en") {
      expect(zero).toMatch(/0 bills are still active/);
      expect(one).toMatch(/1 bill is still active/);
      expect(two).toMatch(/2 bills are still active/);
    } else {
      expect(zero).toMatch(/0 cuentas siguen activas/);
      expect(one).toMatch(/1 cuenta sigue activa/);
      expect(two).toMatch(/2 cuentas siguen activas/);
    }

    if (locale === "es-AR") {
      expect(one).toMatch(/revisala y cobrá/);
      expect(two).toMatch(/revisalas y cobrá/);
    }
  });

  it.each(locales)("%s singularizes sidebar bill and kitchen badge names", (locale) => {
    const billsZero = copy("businessDashboard.sidebar.billsAlertBadge", locale, 0);
    const billsOne = copy("businessDashboard.sidebar.billsAlertBadge", locale, 1);
    const billsTwo = copy("businessDashboard.sidebar.billsAlertBadge", locale, 2);
    const kitchenZero = copy(
      "businessDashboard.sidebar.kitchenReadyBadge",
      locale,
      0,
    );
    const kitchenOne = copy(
      "businessDashboard.sidebar.kitchenReadyBadge",
      locale,
      1,
    );
    const kitchenTwo = copy(
      "businessDashboard.sidebar.kitchenReadyBadge",
      locale,
      2,
    );
    const occupiedZero = copy(
      "businessDashboard.sidebar.occupiedTablesBadge",
      locale,
      0,
    );
    const occupiedOne = copy(
      "businessDashboard.sidebar.occupiedTablesBadge",
      locale,
      1,
    );
    const occupiedTwo = copy(
      "businessDashboard.sidebar.occupiedTablesBadge",
      locale,
      2,
    );

    if (locale === "en") {
      expect(billsZero).toBe("0 orders awaiting approval");
      expect(billsOne).toBe("1 order awaiting approval");
      expect(billsTwo).toBe("2 orders awaiting approval");
      expect(kitchenZero).toBe("0 tickets to cook");
      expect(kitchenOne).toBe("1 ticket to cook");
      expect(kitchenTwo).toBe("2 tickets to cook");
      expect(occupiedZero).toBe("0 occupied tables");
      expect(occupiedOne).toBe("1 occupied table");
      expect(occupiedTwo).toBe("2 occupied tables");
    } else if (locale === "es-AR") {
      expect(billsOne).toBe("1 pedido por aprobar");
      expect(billsTwo).toBe("2 pedidos por aprobar");
      expect(kitchenOne).toBe("1 comanda por cocinar");
      expect(kitchenTwo).toBe("2 comandas por cocinar");
      expect(occupiedOne).toBe("1 mesa ocupada");
      expect(occupiedTwo).toBe("2 mesas ocupadas");
    } else {
      expect(billsOne).toBe("1 pedido por aprobar");
      expect(billsTwo).toBe("2 pedidos por aprobar");
      expect(kitchenOne).toBe("1 comanda por cocinar");
      expect(kitchenTwo).toBe("2 comandas por cocinar");
      expect(occupiedOne).toBe("1 mesa ocupada");
      expect(occupiedTwo).toBe("2 mesas ocupadas");
    }
  });

  it.each(locales)("%s singularizes AI proposal count labels at 1", (locale) => {
    const kind = locale === "en" ? "Price adjustment" : "Ajuste de precios";
    const titleZero = copy(
      "directorConsole.proposal.titleWithCount",
      locale,
      0,
      { kind, count: 0 },
    );
    const titleOne = copy(
      "directorConsole.proposal.titleWithCount",
      locale,
      1,
      { kind, count: 1 },
    );
    const titleTwo = copy(
      "directorConsole.proposal.titleWithCount",
      locale,
      2,
      { kind, count: 2 },
    );
    const descZero = copy("directorConsole.proposal.descAffected", locale, 0);
    const descOne = copy("directorConsole.proposal.descAffected", locale, 1);
    const descTwo = copy("directorConsole.proposal.descAffected", locale, 2);
    const summaryZero = copy(
      "directorConsole.proposal.summaryAffected",
      locale,
      0,
    );
    const summaryOne = copy(
      "directorConsole.proposal.summaryAffected",
      locale,
      1,
    );
    const summaryTwo = copy(
      "directorConsole.proposal.summaryAffected",
      locale,
      2,
    );
    const affectedZero = copy(
      "directorConsole.proposal.affectedCount",
      locale,
      0,
    );
    const affectedOne = copy(
      "directorConsole.proposal.affectedCount",
      locale,
      1,
    );
    const affectedTwo = copy(
      "directorConsole.proposal.affectedCount",
      locale,
      2,
    );

    for (const value of [
      titleZero,
      titleOne,
      titleTwo,
      descZero,
      descOne,
      descTwo,
      summaryZero,
      summaryOne,
      summaryTwo,
      affectedZero,
      affectedOne,
      affectedTwo,
    ]) {
      expect(value).not.toMatch(/\(s\)/);
    }

    if (locale === "en") {
      expect(titleZero).toBe("Price adjustment · 0 items");
      expect(titleOne).toBe("Price adjustment · 1 item");
      expect(titleTwo).toBe("Price adjustment · 2 items");
      expect(descZero).toBe("Applies to 0 items.");
      expect(descOne).toBe("Applies to 1 item.");
      expect(descTwo).toBe("Applies to 2 items.");
      expect(summaryZero).toBe("Preview of 0 items.");
      expect(summaryOne).toBe("Preview of 1 item.");
      expect(summaryTwo).toBe("Preview of 2 items.");
      expect(affectedZero).toBe("0 items affected");
      expect(affectedOne).toBe("1 item affected");
      expect(affectedTwo).toBe("2 items affected");
    } else {
      expect(titleZero).toBe("Ajuste de precios · 0 ítems");
      expect(titleOne).toBe("Ajuste de precios · 1 ítem");
      expect(titleTwo).toBe("Ajuste de precios · 2 ítems");
      expect(descZero).toBe("Se aplica a 0 ítems.");
      expect(descOne).toBe("Se aplica a 1 ítem.");
      expect(descTwo).toBe("Se aplica a 2 ítems.");
      expect(summaryZero).toBe("Vista previa de 0 elementos.");
      expect(summaryOne).toBe("Vista previa de 1 elemento.");
      expect(summaryTwo).toBe("Vista previa de 2 elementos.");
      expect(affectedZero).toBe("0 artículos afectados");
      expect(affectedOne).toBe("1 artículo afectado");
      expect(affectedTwo).toBe("2 artículos afectados");
      expect(titleOne).not.toMatch(/1 ítems/);
      expect(descOne).not.toMatch(/1 ítems/);
      expect(summaryOne).not.toMatch(/1 elementos/);
    }
  });
});
