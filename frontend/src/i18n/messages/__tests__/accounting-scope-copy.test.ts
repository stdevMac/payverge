/** @jest-environment node */
import enDashboard from "../en/businessDashboard.json";
import esDashboard from "../es/businessDashboard.json";
import esArDashboard from "../es-ar/businessDashboard.json";

type AccountingDashboard = {
  overview?: {
    kpi?: {
      collectionGapHint?: string;
      collectionGapScope?: string;
    };
  };
  outstanding?: {
    asOf?: string;
    empty?: string;
  };
};

function accounting(tree: {
  accountingDashboard?: AccountingDashboard;
}): AccountingDashboard {
  return tree.accountingDashboard ?? {};
}

function effectiveEsAr(): AccountingDashboard {
  const es = accounting(esDashboard);
  const ar = accounting(esArDashboard);
  return {
    overview: {
      kpi: {
        ...es.overview?.kpi,
        ...ar.overview?.kpi,
      },
    },
    outstanding: {
      ...es.outstanding,
      ...ar.outstanding,
    },
  };
}

/**
 * #222 / #770: Overview collection gap is range-created leftover remainings;
 * Outstanding is as-of the end date and may include older leftover remainings.
 * en / es / es-AR must say so — empty-state "as of this period end" is not enough.
 */
describe("accounting collection-gap vs outstanding scope copy (#222)", () => {
  const locales: Array<[string, AccountingDashboard]> = [
    ["en", accounting(enDashboard)],
    ["es", accounting(esDashboard)],
    ["es-AR", effectiveEsAr()],
  ];

  it.each(locales)(
    "%s Overview discloses range-created debt and that Outstanding includes older bills",
    (_name, dash) => {
      const hint = dash.overview?.kpi?.collectionGapHint ?? "";
      const scope = dash.overview?.kpi?.collectionGapScope ?? "";
      expect(hint.length).toBeGreaterThan(0);
      expect(hint).toMatch(/date range|rango de fechas/i);
      expect(scope.length).toBeGreaterThan(0);
      expect(scope).toMatch(/\{end\}/);
      expect(scope).toMatch(/\{start\}/);
    },
  );

  it.each(locales)(
    "%s non-empty Outstanding states as-of the end date and older debt",
    (_name, dash) => {
      const asOf = dash.outstanding?.asOf ?? "";
      expect(asOf.length).toBeGreaterThan(0);
      expect(asOf).toMatch(/\{end\}/);
      expect(asOf).toMatch(/before this range|anteriores a este rango/i);
    },
  );
});
