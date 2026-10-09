import { messages } from "../getTranslation";

/**
 * Key-existence gate for the wave-g modal/overlay copy.
 *
 * Why this file exists: every component test for these surfaces stubs the
 * translation function (`t: (k) => k`), so a component can reference a key that
 * exists in NEITHER en nor es and every suite still passes. In production
 * `getTranslation` NEVER returns a falsy value — on a total miss it returns
 * `sentenceCaseLeaf(key)` — so the miss renders as English-looking debug copy
 * ("Loading", "Discard config") in every locale instead of throwing. That is
 * exactly how the discardConfig / discardConfirm / crm.loading gaps shipped.
 *
 * This gate reads the REAL merged operator trees (the same objects
 * SimpleTranslationProvider serves) and fails when a referenced key is absent.
 *
 * es-AR is the es tree plus a thin voseo override layer, so it can never have a
 * gap relative to es — it is asserted here anyway to pin that invariant.
 */

// Dotted keys referenced by wave-g modal/overlay components, with the call site
// that reads them. Add a row whenever an overlay starts reading a new key.
const REQUIRED_KEYS: ReadonlyArray<readonly [key: string, callSite: string]> = [
  // PluginManager.tsx — plugin config drawer dirty-discard ConfirmationModal.
  [
    "businessDashboard.dashboard.pluginManager.discardConfig.title",
    "PluginManager.tsx",
  ],
  [
    "businessDashboard.dashboard.pluginManager.discardConfig.description",
    "PluginManager.tsx",
  ],
  [
    "businessDashboard.dashboard.pluginManager.discardConfig.confirm",
    "PluginManager.tsx",
  ],
  [
    "businessDashboard.dashboard.pluginManager.discardConfig.cancel",
    "PluginManager.tsx",
  ],

  // DetailDrawer.tsx discard confirm — copy is supplied by each accounting
  // drawer via the `discardConfirm` prop (the shell ships no English literals).
  [
    "businessDashboard.accountingDashboard.discardConfirm.confirm",
    "DetailDrawer via accounting drawers",
  ],
  [
    "businessDashboard.accountingDashboard.discardConfirm.cancel",
    "DetailDrawer via accounting drawers",
  ],
  [
    "businessDashboard.accountingDashboard.discardConfirm.entry.title",
    "EntryFormDrawer.tsx",
  ],
  [
    "businessDashboard.accountingDashboard.discardConfirm.entry.description",
    "EntryFormDrawer.tsx",
  ],
  [
    "businessDashboard.accountingDashboard.discardConfirm.payroll.title",
    "PayrollRunFormDrawer.tsx",
  ],
  [
    "businessDashboard.accountingDashboard.discardConfirm.payroll.description",
    "PayrollRunFormDrawer.tsx",
  ],
  [
    "businessDashboard.accountingDashboard.discardConfirm.issueInvoice.title",
    "IssueInvoiceDrawer.tsx",
  ],
  [
    "businessDashboard.accountingDashboard.discardConfirm.issueInvoice.description",
    "IssueInvoiceDrawer.tsx",
  ],

  // CustomersTab.tsx — customer-details modal loading status line.
  ["businessDashboard.crm.loading", "CustomersTab.tsx"],
];

function getNested(root: unknown, dotted: string): unknown {
  return dotted.split(".").reduce<unknown>((acc, part) => {
    if (acc && typeof acc === "object" && part in (acc as object)) {
      return (acc as Record<string, unknown>)[part];
    }
    return undefined;
  }, root);
}

describe("wave-g modal/overlay operator keys exist in the real message files", () => {
  const locales = ["en", "es", "es-AR"] as const;

  it.each(locales)("%s resolves every referenced key to a string", (locale) => {
    const missing: string[] = [];
    for (const [key, callSite] of REQUIRED_KEYS) {
      const value = getNested(messages[locale], key);
      if (typeof value !== "string" || value.trim().length === 0) {
        missing.push(`${key} (referenced by ${callSite})`);
      }
    }
    expect(missing).toEqual([]);
  });

  it("keeps en and es structurally aligned for these keys", () => {
    const drift = REQUIRED_KEYS.filter(([key]) => {
      const en = getNested(messages.en, key);
      const es = getNested(messages.es, key);
      return typeof en !== typeof es;
    }).map(([key]) => key);
    expect(drift).toEqual([]);
  });
});
