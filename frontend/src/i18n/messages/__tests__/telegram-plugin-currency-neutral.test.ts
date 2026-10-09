/** @jest-environment node */
import enDashboard from "../en/businessDashboard.json";
import esDashboard from "../es/businessDashboard.json";
import esArDashboard from "../es-ar/businessDashboard.json";

/**
 * #901 — the Telegram plugin config panel renders for every venue, and the
 * venue currency comes from `businesses.display_currency`. A hard-coded amount
 * in that copy ("orders above $100") is simply wrong on an ARS carta, so the
 * operator strings must stay currency-neutral.
 */
const HARD_CODED_MONEY =
  /(\$\s*\d|\d\s*(usd|ars|eur|gbp)\b|\b(usd|ars|eur|gbp)\s*\d)/i;

type Tree = Record<string, unknown>;

function telegramCopy(tree: unknown): Tree {
  const dashboard = (tree as Tree)?.dashboard as Tree | undefined;
  const pluginManager = dashboard?.pluginManager as Tree | undefined;
  const config = pluginManager?.config as Tree | undefined;
  return (config?.telegram as Tree) ?? {};
}

function flatten(node: unknown, path: string, out: Record<string, string>) {
  if (typeof node === "string") {
    out[path] = node;
    return;
  }
  if (node && typeof node === "object") {
    for (const [key, value] of Object.entries(node as Tree)) {
      flatten(value, path ? `${path}.${key}` : key, out);
    }
  }
}

// es-AR ships deltas only; the effective bundle is es with es-AR layered on top.
const effectiveEsAr: Tree = {
  ...telegramCopy(esDashboard),
  ...telegramCopy(esArDashboard),
};

describe("telegram plugin operator copy", () => {
  const bundles: Array<[string, Tree]> = [
    ["en", telegramCopy(enDashboard)],
    ["es", telegramCopy(esDashboard)],
    ["es-AR", effectiveEsAr],
  ];

  it.each(bundles)(
    "%s telegram copy names no hard-coded currency amount",
    (locale, bundle) => {
      const strings: Record<string, string> = {};
      flatten(bundle, "", strings);
      expect(Object.keys(strings).length).toBeGreaterThan(0);

      const offenders = Object.entries(strings).filter(([, value]) =>
        HARD_CODED_MONEY.test(value),
      );
      expect(
        offenders.map(([key, value]) => `${locale}:${key} = ${value}`),
      ).toEqual([]);
    },
  );

  it("keeps a currency-neutral hint for the high-value orders toggle", () => {
    for (const [, bundle] of bundles) {
      const hint = bundle.highValueOrdersHint;
      expect(typeof hint === "string" && hint.trim().length > 0).toBe(true);
      expect(HARD_CODED_MONEY.test(hint as string)).toBe(false);
      // The retired key carried the USD figure; it must not come back.
      expect(bundle.ordersAbove100).toBeUndefined();
    }
  });
});
