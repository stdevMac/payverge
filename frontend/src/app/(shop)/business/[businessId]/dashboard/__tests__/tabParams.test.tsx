import { TAB_PARAM_WHITELIST, nextTabSearchParams } from "../tabParams";

describe("nextTabSearchParams (audit M2: tab switches drop stale params)", () => {
  it("always sets the tab", () => {
    const next = nextTabSearchParams(new URLSearchParams(""), "inventory");
    expect(next.get("tab")).toBe("inventory");
  });

  it("drops analytics-only params when switching to inventory", () => {
    const current = new URLSearchParams("tab=analytics&sub=service");
    const next = nextTabSearchParams(current, "inventory");
    expect(next.get("tab")).toBe("inventory");
    expect(next.get("sub")).toBeNull();
    expect(next.get("view")).toBeNull();
  });

  it("keeps analytics sub when staying on analytics", () => {
    const current = new URLSearchParams("tab=analytics&sub=service");
    const next = nextTabSearchParams(current, "analytics");
    expect(next.get("sub")).toBe("service");
  });

  it("keeps explicitly passed deep-link params", () => {
    const next = nextTabSearchParams(
      new URLSearchParams("tab=overview"),
      "bills",
      "tableId=5",
    );
    expect(next.get("tableId")).toBe("5");
  });

  it("carries tableId forward for the bills tab even without extraQuery", () => {
    const current = new URLSearchParams("tab=bills&tableId=7");
    const next = nextTabSearchParams(current, "bills");
    expect(next.get("tableId")).toBe("7");
  });

  it("drops a stale tableId when leaving the bills tab", () => {
    const current = new URLSearchParams("tab=bills&tableId=7");
    const next = nextTabSearchParams(current, "menu");
    expect(next.get("tableId")).toBeNull();
  });

  it("preserves the sub param across sub-tab-owning tabs", () => {
    const current = new URLSearchParams("tab=accounting&sub=invoices");
    const next = nextTabSearchParams(current, "accounting");
    expect(next.get("sub")).toBe("invoices");
  });

  it("drops the sub param when switching to a tab that does not own it", () => {
    const current = new URLSearchParams("tab=crm&sub=segments");
    const next = nextTabSearchParams(current, "menu");
    expect(next.get("sub")).toBeNull();
  });

  it("preserves the settings section param on the settings tab", () => {
    const current = new URLSearchParams("tab=settings&section=notifications");
    const next = nextTabSearchParams(current, "settings");
    expect(next.get("section")).toBe("notifications");
  });

  it("drops the settings section param when leaving settings", () => {
    const current = new URLSearchParams("tab=settings&section=notifications");
    const next = nextTabSearchParams(current, "analytics");
    expect(next.get("section")).toBeNull();
  });

  it("lets an explicit deep-link param override a whitelisted carry-over", () => {
    const current = new URLSearchParams("tab=bills&tableId=7");
    const next = nextTabSearchParams(current, "bills", "tableId=9");
    expect(next.get("tableId")).toBe("9");
  });

  it("ignores unknown/unwhitelisted params entirely", () => {
    const current = new URLSearchParams(
      "tab=analytics&renewal=success&foo=bar",
    );
    const next = nextTabSearchParams(current, "analytics");
    expect(next.get("renewal")).toBeNull();
    expect(next.get("foo")).toBeNull();
  });

  it("keeps tablesView and spaceId on the tables tab for Spaces deep-links", () => {
    const current = new URLSearchParams(
      "tab=tables&tablesView=spaces&spaceId=7&tableSearch=T1",
    );
    const next = nextTabSearchParams(current, "tables");
    expect(next.get("tablesView")).toBe("spaces");
    expect(next.get("spaceId")).toBe("7");
    expect(next.get("tableSearch")).toBe("T1");
  });

  it("applies explicit tablesView deep-link from onboarding layout step", () => {
    const next = nextTabSearchParams(
      new URLSearchParams("tab=overview"),
      "tables",
      "tablesView=spaces",
    );
    expect(next.get("tab")).toBe("tables");
    expect(next.get("tablesView")).toBe("spaces");
  });

  it("declares sub under analytics, tableId under bills, sub under sub-tab tabs, section under settings", () => {
    expect(TAB_PARAM_WHITELIST.analytics).toContain("sub");
    expect(TAB_PARAM_WHITELIST.analytics).not.toContain("view");
    expect(TAB_PARAM_WHITELIST.analytics).toContain("period");
    expect(TAB_PARAM_WHITELIST.bills).toContain("tableId");
    expect(TAB_PARAM_WHITELIST.accounting).toContain("sub");
    expect(TAB_PARAM_WHITELIST.accounting).toContain("status");
    expect(TAB_PARAM_WHITELIST.accounting).toContain("filter");
    expect(TAB_PARAM_WHITELIST.staff).toContain("sub");
    expect(TAB_PARAM_WHITELIST.crm).toContain("sub");
    expect(TAB_PARAM_WHITELIST.settings).toContain("section");
    expect(TAB_PARAM_WHITELIST["business-page"]).toContain("section");
  });

  it("keeps section=contact when deep-linking from Settings onto Business Page (#225)", () => {
    const next = nextTabSearchParams(
      new URLSearchParams("tab=settings&section=profile"),
      "business-page",
      "section=contact",
    );
    expect(next.get("tab")).toBe("business-page");
    expect(next.get("section")).toBe("contact");
  });

  it("does not leak Settings section onto Business Page without extraQuery", () => {
    const next = nextTabSearchParams(
      new URLSearchParams("tab=settings&section=notifications"),
      "business-page",
    );
    expect(next.get("section")).toBeNull();
  });

  it("preserves accounting status/filter deep-link params when staying on accounting", () => {
    const current = new URLSearchParams(
      "tab=accounting&sub=payroll&status=draft",
    );
    const next = nextTabSearchParams(current, "accounting");
    expect(next.get("sub")).toBe("payroll");
    expect(next.get("status")).toBe("draft");
  });

  it("drops accounting status/filter when leaving the accounting tab", () => {
    const current = new URLSearchParams(
      "tab=accounting&sub=invoices&filter=needs_attention",
    );
    const next = nextTabSearchParams(current, "menu");
    expect(next.get("sub")).toBeNull();
    expect(next.get("filter")).toBeNull();
    expect(next.get("status")).toBeNull();
  });

  it("carries billId forward for the bills tab and drops it elsewhere (wave 4)", () => {
    const carried = nextTabSearchParams(
      new URLSearchParams("tab=bills&billId=42"),
      "bills",
    );
    expect(carried.get("billId")).toBe("42");

    const dropped = nextTabSearchParams(
      new URLSearchParams("tab=bills&billId=42"),
      "menu",
    );
    expect(dropped.get("billId")).toBeNull();
  });

  it("carries billCustomer forward for the bills tab (wave 4)", () => {
    const carried = nextTabSearchParams(
      new URLSearchParams("tab=bills&billCustomer=7"),
      "bills",
    );
    expect(carried.get("billCustomer")).toBe("7");
  });

  it("carries tableSearch forward for the tables tab (wave 4)", () => {
    const carried = nextTabSearchParams(
      new URLSearchParams("tab=tables&tableSearch=CORE-T01"),
      "tables",
    );
    expect(carried.get("tableSearch")).toBe("CORE-T01");
  });

  it("carries menuSearch and staffSearch forward (wave 4)", () => {
    expect(
      nextTabSearchParams(
        new URLSearchParams("tab=menu&menuSearch=Tacos"),
        "menu",
      ).get("menuSearch"),
    ).toBe("Tacos");
    expect(
      nextTabSearchParams(
        new URLSearchParams("tab=staff&sub=people&staffSearch=Ana"),
        "staff",
      ).get("staffSearch"),
    ).toBe("Ana");
  });

  it("keeps only the current rail's params across the full rail transition matrix", () => {
    const railParams: Record<string, Record<string, string>> = {
      analytics: { sub: "revenue", period: "week" },
      accounting: {
        sub: "payroll",
        status: "draft",
        filter: "needs_attention",
        period: "week",
      },
      staff: { sub: "people", staffSearch: "Ana" },
      crm: { sub: "customers", focus: "vip" },
      bills: {
        tableId: "7",
        billId: "42",
        billCustomer: "9",
        billTab: "history",
        billPage: "2",
        billSearch: "taco",
        billFrom: "2026-01-01",
        billTo: "2026-01-31",
        billStatus: "paid",
      },
      tables: {
        tableSearch: "T1",
        tablesView: "spaces",
        spaceId: "3",
        tableId: "12",
      },
      menu: { menuSearch: "Tacos" },
      settings: { section: "notifications" },
      delivery: { sub: "dispatch" },
      reservations: { reservationId: "99" },
    };
    const rails = Object.keys(railParams);

    for (const source of rails) {
      const current = new URLSearchParams({
        tab: source,
        ...railParams[source],
        unrelated: "remove-me",
      });

      for (const destination of rails) {
        const next = nextTabSearchParams(current, destination);
        const destAllowed = new Set(TAB_PARAM_WHITELIST[destination] ?? []);
        // Same-rail: full whitelist carry. Cross-rail: only CROSS_RAIL_SHARED
        // params that the destination also declares (L6-2 period).
        const allowed = new Set(
          destination === source
            ? [...destAllowed]
            : [...destAllowed].filter((k) =>
                // inline the shared set so this test file stays self-contained
                // if CROSS_RAIL_SHARED_PARAMS import is unavailable
                k === "period",
              ),
        );

        expect(next.get("tab")).toBe(destination);
        expect(next.get("unrelated")).toBeNull();
        for (const key of Object.keys(railParams[source])) {
          expect(next.has(key)).toBe(allowed.has(key));
        }
      }
    }
  });

  it("filters explicit deep-link params through the destination rail", () => {
    const next = nextTabSearchParams(
      new URLSearchParams("tab=analytics&sub=revenue&section=overview"),
      "accounting",
      "sub=payroll&status=draft&section=should-drop&unrelated=drop",
    );

    expect(next.toString()).toBe("tab=accounting&sub=payroll&status=draft");
  });
});

/**
 * Gate F-10 (audit §18): tabParams round-trips every param each rail actually
 * uses. Same-rail carry-over keeps whitelisted keys; cross-rail strips them;
 * extraQuery applies regardless of rail (deliberate asymmetry).
 */
describe("F-10 tabParams round-trip contract (Session P / L9-12)", () => {
  const RAIL_CONTRACT: Record<string, readonly string[]> = {
    analytics: ["sub", "period"],
    bills: [
      "tableId",
      "billId",
      "billCustomer",
      "billTab",
      "billPage",
      "billSearch",
      "billFrom",
      "billTo",
      "billStatus",
    ],
    accounting: ["sub", "status", "filter", "period"],
    staff: ["sub", "staffSearch"],
    crm: ["sub", "focus"],
    settings: ["section"],
    tables: ["tableSearch", "tablesView", "spaceId", "tableId"],
    menu: ["menuSearch"],
    delivery: ["sub"],
    reservations: ["reservationId", "reservationsView", "sub"],
    inventory: ["sub"],
    "business-page": ["section"],
  };

  it("declares every Session-P deep-link param under its owning rail", () => {
    for (const [rail, keys] of Object.entries(RAIL_CONTRACT)) {
      expect(TAB_PARAM_WHITELIST[rail]).toEqual(expect.arrayContaining([...keys]));
      // Whitelist must not grow silent undeclared keys without updating F-10
      expect([...TAB_PARAM_WHITELIST[rail]].sort()).toEqual([...keys].sort());
    }
  });

  it("round-trips every declared param on same-rail navigation", () => {
    for (const [rail, keys] of Object.entries(RAIL_CONTRACT)) {
      const current = new URLSearchParams({ tab: rail });
      for (const key of keys) {
        current.set(key, `val-${key}`);
      }
      const next = nextTabSearchParams(current, rail);
      expect(next.get("tab")).toBe(rail);
      for (const key of keys) {
        expect(next.get(key)).toBe(`val-${key}`);
      }
    }
  });

  it("strips every declared param on cross-rail navigation (no silent carry)", () => {
    for (const [source, keys] of Object.entries(RAIL_CONTRACT)) {
      const current = new URLSearchParams({ tab: source });
      for (const key of keys) {
        current.set(key, `val-${key}`);
      }
      // Pick a destination that is not the source
      const destination = source === "menu" ? "overview" : "menu";
      const next = nextTabSearchParams(current, destination);
      expect(next.get("tab")).toBe(destination);
      for (const key of keys) {
        expect(next.get(key)).toBeNull();
      }
    }
  });

  it("applies extraQuery on cross-rail while still filtering through destination whitelist (asymmetry)", () => {
    // Carry-over is same-rail only, but extraQuery applies regardless of rail —
    // so a deep-link like handleSetActiveTab("bills?billTab=history&billCustomer=7")
    // works from any source tab.
    const next = nextTabSearchParams(
      new URLSearchParams("tab=crm&sub=customers&focus=vip"),
      "bills",
      "billTab=history&billCustomer=7&billId=42&unrelated=drop",
    );
    expect(next.get("tab")).toBe("bills");
    expect(next.get("billTab")).toBe("history");
    expect(next.get("billCustomer")).toBe("7");
    expect(next.get("billId")).toBe("42");
    // CRM params must not leak
    expect(next.get("sub")).toBeNull();
    expect(next.get("focus")).toBeNull();
    expect(next.get("unrelated")).toBeNull();
  });

  it("L5-8: billTab survives nextTabSearchParams when landing on bills via extraQuery", () => {
    const next = nextTabSearchParams(
      new URLSearchParams("tab=crm"),
      "bills",
      "billTab=history&billCustomer=12",
    );
    expect(next.get("billTab")).toBe("history");
    expect(next.get("billCustomer")).toBe("12");
  });

  it("L3-43: delivery sub is whitelisted and round-trips", () => {
    const carried = nextTabSearchParams(
      new URLSearchParams("tab=delivery&sub=drivers"),
      "delivery",
    );
    expect(carried.get("sub")).toBe("drivers");
    const deep = nextTabSearchParams(
      new URLSearchParams("tab=overview"),
      "delivery",
      "sub=history",
    );
    expect(deep.get("sub")).toBe("history");
  });

  it("L6-2: period survives staying on accounting", () => {
    expect(
      nextTabSearchParams(
        new URLSearchParams("tab=accounting&period=week"),
        "accounting",
      ).get("period"),
    ).toBe("week");
  });

  it("F-cand-11: reservationId / tableId deep-link params round-trip", () => {
    expect(
      nextTabSearchParams(
        new URLSearchParams("tab=reservations&reservationId=55"),
        "reservations",
      ).get("reservationId"),
    ).toBe("55");
    expect(
      nextTabSearchParams(
        new URLSearchParams("tab=tables&tableId=8"),
        "tables",
      ).get("tableId"),
    ).toBe("8");
  });
});
