/**
 * One sub-tab URL convention: `sub` is the only parameter.
 * Unknown tab/sub keys surface a not-found state naming the bad key —
 * never a silent redirect to a default.
 *
 * @jest-environment node
 */
import {
  SUB_PARAM,
  resolveSubTab,
  buildSubSearchParams,
  isAllowedSub,
} from "./subTabs";

const ANALYTICS_SUBS = ["today", "payments", "revenue", "menu", "service"] as const;

describe("subTabs — parameter name", () => {
  it("exports sub as the only sub-tab parameter name", () => {
    expect(SUB_PARAM).toBe("sub");
  });
});

describe("resolveSubTab", () => {
  it("returns the default when sub is absent", () => {
    const result = resolveSubTab({ sub: null }, ANALYTICS_SUBS, "today");
    expect(result).toEqual({
      status: "ok",
      sub: "today",
      needsDefaultInUrl: true,
    });
  });

  it("honors an allowed sub param", () => {
    const result = resolveSubTab(
      { sub: "revenue" },
      ANALYTICS_SUBS,
      "today",
    );
    expect(result).toEqual({
      status: "ok",
      sub: "revenue",
      needsDefaultInUrl: false,
    });
  });

  it("returns unknown naming the bad key for an invalid sub (no silent fallback)", () => {
    const result = resolveSubTab(
      { sub: "not-a-panel" },
      ANALYTICS_SUBS,
      "today",
    );
    expect(result).toEqual({
      status: "unknown",
      badKey: "not-a-panel",
      param: "sub",
    });
  });

  it("maps staff sub=tips and sub=tip onto People (issue 354)", () => {
    const team = ["people", "positions", "communication"] as const;
    const aliases: Record<string, (typeof team)[number]> = {
      tips: "people",
      tip: "people",
    };
    for (const raw of ["tips", "tip"] as const) {
      const result = resolveSubTab({ sub: raw }, team, "people", aliases);
      expect(result.status).toBe("ok");
      if (result.status === "ok") {
        expect(result.sub).toBe("people");
        expect(result.needsDefaultInUrl).toBe(true);
      }
    }
  });

  it("still names a genuinely unknown staff sub", () => {
    const team = ["people", "positions", "communication"] as const;
    const result = resolveSubTab({ sub: "payroll" }, team, "people", {
      tips: "people",
    });
    expect(result).toEqual({
      status: "unknown",
      badKey: "payroll",
      param: "sub",
    });
  });
});

describe("buildSubSearchParams", () => {
  it("writes sub and leaves unrelated params, including a leftover view", () => {
    const next = buildSubSearchParams(
      new URLSearchParams("tab=analytics&period=week&view=service"),
      "revenue",
    );
    expect(next.get("sub")).toBe("revenue");
    expect(next.get("view")).toBe("service");
    expect(next.get("period")).toBe("week");
    expect(next.get("tab")).toBe("analytics");
  });
});

describe("isAllowedSub", () => {
  it("type-guards members of the allowed list", () => {
    expect(isAllowedSub("today", ANALYTICS_SUBS)).toBe(true);
    expect(isAllowedSub("nope", ANALYTICS_SUBS)).toBe(false);
    expect(isAllowedSub(null, ANALYTICS_SUBS)).toBe(false);
  });
});
