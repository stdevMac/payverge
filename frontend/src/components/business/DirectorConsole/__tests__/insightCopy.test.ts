import {
  toPreShiftCard,
  toPreShiftCards,
  toBriefingChips,
  sortInsightCardsBySeverity,
  greetingKey,
  briefingReadKey,
  briefingHealthKey,
  briefingPlayKey,
  briefingPlayCtaKey,
  briefingWinKey,
  ownerFirstName,
} from "../insightCopy";
import type { ProactiveInsightDTO } from "@/api/directorConsole";

function dto(partial: Partial<ProactiveInsightDTO>): ProactiveInsightDTO {
  return {
    id: "x",
    type: "inventory_out_of_stock",
    params: {},
    cta: { tab: "inventory" },
    ...partial,
  };
}

describe("toPreShiftCard", () => {
  it("maps out-of-stock to an urgent card with count, names and tab", () => {
    const card = toPreShiftCard(
      dto({
        id: "a",
        type: "inventory_out_of_stock",
        params: { count: 3, item_names: ["Branzino", "Risotto", "Tiramisu", "Extra"] },
        cta: { tab: "inventory" },
      }),
    );
    expect(card).not.toBeNull();
    expect(card!.id).toBe("a");
    expect(card!.tone).toBe("urgent");
    // count>1 -> the plural ".other" branch
    expect(card!.copyKey).toBe("outOfStock.other");
    expect(card!.params.count).toBe(3);
    // names are joined and capped at 3
    expect(card!.params.names).toBe("Branzino, Risotto, Tiramisu");
    expect(card!.tab).toBe("inventory");
  });

  // L1-22: negative stock must not use the "at zero" copy key.
  it("maps oversold inventory (has_oversold) to oversold copy keys", () => {
    const card = toPreShiftCard(
      dto({
        id: "o",
        type: "inventory_out_of_stock",
        params: {
          count: 1,
          item_names: ["Aceite"],
          has_oversold: true,
          oversold_count: 1,
        },
        cta: { tab: "inventory" },
      }),
    );
    expect(card).not.toBeNull();
    expect(card!.copyKey).toBe("oversold.one");
    expect(card!.copyKey).not.toMatch(/outOfStock/);
  });

  // R2-7: has_oversold is a boolean OR across the set. With 1 oversold + 1
  // at-zero item the card claimed "2 inventory items are oversold: Oversold
  // Oil, Zero Flour" — false for Zero Flour, and it threw away oversold_count.
  describe("mixed oversold + at-zero sets (R2-7)", () => {
    const mixed = dto({
      id: "m",
      type: "inventory_out_of_stock",
      params: {
        count: 2,
        item_names: ["Oversold Oil", "Zero Flour"],
        has_oversold: true,
        oversold_count: 1,
        oversold_names: ["Oversold Oil"],
        zero_names: ["Zero Flour"],
      },
      cta: { tab: "inventory" },
    });

    it("uses the mixed copy key with each group's own count and names", () => {
      const card = toPreShiftCard(mixed);
      expect(card).not.toBeNull();
      expect(card!.copyKey).toBe("outOfStockMixed");
      expect(card!.params.count).toBe(2);
      expect(card!.params.oversoldCount).toBe(1);
      expect(card!.params.oversoldNames).toBe("Oversold Oil");
      expect(card!.params.zeroCount).toBe(1);
      expect(card!.params.zeroNames).toBe("Zero Flour");
      expect(card!.tone).toBe("urgent");
      expect(card!.tab).toBe("inventory");
    });

    it("keeps the single-group oversold key when the whole set is oversold", () => {
      const card = toPreShiftCard(
        dto({
          type: "inventory_out_of_stock",
          params: {
            count: 2,
            item_names: ["Oil", "Butter"],
            has_oversold: true,
            oversold_count: 2,
            oversold_names: ["Oil", "Butter"],
            zero_names: [],
          },
        }),
      );
      expect(card!.copyKey).toBe("oversold.other");
      expect(card!.params.names).toBe("Oil, Butter");
    });

    it("falls back to the single-group key when the backend omits the split", () => {
      // Version skew: a new frontend against a backend that predates the split
      // must not render a mixed template with empty name slots.
      const card = toPreShiftCard(
        dto({
          type: "inventory_out_of_stock",
          params: {
            count: 2,
            item_names: ["Oil", "Flour"],
            has_oversold: true,
          },
        }),
      );
      expect(card!.copyKey).toBe("oversold.other");
    });
  });


  // The interpolation engine has no ICU plural support, so each card type that
  // the backend can emit at count===1 must carry count-branched .one/.other
  // keys — otherwise the card renders "1 open bill(s) have sat" on the most
  // common trigger. These four are reachable at count===1.
  describe("count-branched copy keys", () => {
    const cases: Array<{ type: string; base: string; tab: string }> = [
      { type: "inventory_out_of_stock", base: "outOfStock", tab: "inventory" },
      { type: "stale_open_bills", base: "staleBills", tab: "bills" },
      { type: "ai_conversations_pending", base: "aiPending", tab: "ai" },
      { type: "food_cost_high", base: "foodCostHigh", tab: "accounting" },
    ];

    cases.forEach(({ type, base, tab }) => {
      it(`branches ${base} to .one at count===1`, () => {
        const card = toPreShiftCard(dto({ type, params: { count: 1 }, cta: { tab } }));
        expect(card!.copyKey).toBe(`${base}.one`);
        expect(card!.params.count).toBe(1);
      });

      it(`branches ${base} to .other (never .one) at count===2`, () => {
        const card = toPreShiftCard(dto({ type, params: { count: 2 }, cta: { tab } }));
        expect(card!.copyKey).toBe(`${base}.other`);
        expect(card!.copyKey).not.toMatch(/\.one$/);
        expect(card!.params.count).toBe(2);
      });
    });

    // lowStock is deliberately NOT count-branched: the backend only emits
    // inventory_low_stock at count>=3 (director_console_handler.go
    // len(lowStock)>=3 gate), so the plural-only wording is always safe. Lock
    // the single-key contract so a future count-branch (or a dropped backend
    // threshold leaking count===1) trips this test instead of shipping
    // "1 item(s)" copy. Asserting at count===1 also makes the test non-vacuous:
    // a pluralKey() branch here would yield "lowStock.one" and fail.
    it("keeps lowStock a single non-branched key (backend gates it at count>=3)", () => {
      expect(toPreShiftCard(dto({ type: "inventory_low_stock", params: { count: 1 }, cta: { tab: "inventory" } }))!.copyKey).toBe("lowStock");
      expect(toPreShiftCard(dto({ type: "inventory_low_stock", params: { count: 4 }, cta: { tab: "inventory" } }))!.copyKey).toBe("lowStock");
    });
  });

  it("rounds waste amount and keeps the ingredient", () => {
    const card = toPreShiftCard(
      dto({ type: "waste_high", params: { amount: 179.6, ingredient: "salmon trim" }, cta: { tab: "accounting" } }),
    );
    expect(card!.copyKey).toBe("wasteHigh");
    expect(card!.params.amount).toBe(180);
    expect(card!.params.ingredient).toBe("salmon trim");
    expect(card!.tab).toBe("accounting");
  });

  it("maps marketing_posts to an info Library card with plural keys", () => {
    const card = toPreShiftCard(
      dto({
        id: "m1",
        type: "marketing_posts",
        params: {
          count: 2,
          period_days: 7,
          recent_titles: ["Ribeye", "Happy hour"],
          channels: [],
        },
        cta: { tab: "marketing" },
      }),
    );
    expect(card).not.toBeNull();
    expect(card!.tone).toBe("info");
    expect(card!.copyKey).toBe("marketingPosts.other");
    expect(card!.params.count).toBe(2);
    expect(card!.params.days).toBe(7);
    expect(card!.params.titles).toBe("Ribeye, Happy hour");
    expect(card!.tab).toBe("marketing");
  });

  it("uses marketingPostsWithChannel when freeform channels are present", () => {
    const card = toPreShiftCard(
      dto({
        type: "marketing_posts",
        params: {
          count: 1,
          period_days: 7,
          channels: ["Instagram Stories", "WhatsApp"],
        },
        cta: { tab: "marketing" },
      }),
    );
    expect(card!.copyKey).toBe("marketingPostsWithChannel.one");
    expect(card!.params.channels).toBe("Instagram Stories, WhatsApp");
  });

  it("converts labor pct fraction to a whole percent", () => {
    const card = toPreShiftCard(dto({ type: "labor_high", params: { pct: 0.34, amount: 310 }, cta: { tab: "accounting" } }));
    expect(card!.copyKey).toBe("laborHigh");
    expect(card!.params.pct).toBe(34);
    expect(card!.params.amount).toBe(310);
  });

  it("falls back to the overview tab when cta tab is missing", () => {
    const card = toPreShiftCard(dto({ type: "food_cost_high", params: { count: 2 }, cta: { tab: "" } }));
    expect(card!.tab).toBe("overview");
  });

  it("drops unknown insight types instead of inventing copy", () => {
    expect(toPreShiftCard(dto({ type: "totally_new_signal" }))).toBeNull();
  });
});

describe("toPreShiftCards", () => {
  // Cold-start guard: the briefing endpoint serializes an empty insight set as
  // JSON `null` (Go nil slice), so a fresh business hands us `null` here. This
  // must degrade to an empty list, never throw — otherwise the whole Director
  // Console tab unmounts into the error boundary ("Something went wrong").
  it("returns [] for null or undefined insights", () => {
    expect(toPreShiftCards(null)).toEqual([]);
    expect(toPreShiftCards(undefined)).toEqual([]);
  });

  // The drawer shows the FULL list now (the old 3-card cap moved to the strip
  // via toBriefingChips), so toPreShiftCards no longer truncates — it only
  // filters unknowns.
  it("filters unknowns and keeps ALL known insights (no cap)", () => {
    const cards = toPreShiftCards([
      dto({ id: "1", type: "inventory_out_of_stock", params: { count: 1 } }),
      dto({ id: "2", type: "labor_high", params: { pct: 0.3, amount: 100 } }),
      dto({ id: "3", type: "weird" }),
      dto({ id: "4", type: "waste_high", params: { amount: 50, ingredient: "x" } }),
      dto({ id: "5", type: "food_cost_high", params: { count: 4 } }),
      dto({ id: "6", type: "stale_open_bills", params: { count: 2 } }),
    ]);
    expect(cards.map((c) => c.id)).toEqual(["1", "2", "4", "5", "6"]);
  });
});

describe("sortInsightCardsBySeverity", () => {
  it("orders urgent → watch → info and is stable within a tone", () => {
    const cards = toPreShiftCards([
      dto({ id: "info", type: "ai_conversations_pending", params: { count: 1 } }),
      dto({ id: "watch1", type: "stale_open_bills", params: { count: 2 } }),
      dto({ id: "urgent", type: "inventory_out_of_stock", params: { count: 1 } }),
      dto({ id: "watch2", type: "labor_high", params: { pct: 0.3, amount: 100 } }),
    ]);
    const sorted = sortInsightCardsBySeverity(cards);
    expect(sorted.map((c) => c.id)).toEqual(["urgent", "watch1", "watch2", "info"]);
  });

  it("does not mutate the input array", () => {
    const cards = toPreShiftCards([
      dto({ id: "info", type: "ai_conversations_pending", params: { count: 1 } }),
      dto({ id: "urgent", type: "inventory_out_of_stock", params: { count: 1 } }),
    ]);
    const before = cards.map((c) => c.id);
    sortInsightCardsBySeverity(cards);
    expect(cards.map((c) => c.id)).toEqual(before);
  });
});

describe("toBriefingChips", () => {
  it("sorts by severity and caps at 5", () => {
    const chips = toBriefingChips(
      [
        dto({ id: "info", type: "ai_conversations_pending", params: { count: 1 } }),
        dto({ id: "w1", type: "stale_open_bills", params: { count: 2 } }),
        dto({ id: "u1", type: "inventory_out_of_stock", params: { count: 1 } }),
        dto({ id: "w2", type: "labor_high", params: { pct: 0.3, amount: 100 } }),
        dto({ id: "w3", type: "waste_high", params: { amount: 50, ingredient: "x" } }),
        dto({ id: "w4", type: "food_cost_high", params: { count: 4 } }),
        dto({ id: "u2", type: "inventory_out_of_stock", params: { count: 3 } }),
      ],
      5,
    );
    // 2 urgent first, then watch, capped at 5 total.
    expect(chips.map((c) => c.id)).toEqual(["u1", "u2", "w1", "w2", "w3"]);
    expect(chips).toHaveLength(5);
  });

  it("tolerates null/undefined and applies the default cap of 5", () => {
    expect(toBriefingChips(null)).toEqual([]);
    expect(toBriefingChips(undefined)).toEqual([]);
  });
});

describe("greetingKey", () => {
  it("buckets the hour into morning/afternoon/evening", () => {
    expect(greetingKey(6)).toBe("morning");
    expect(greetingKey(11)).toBe("morning");
    expect(greetingKey(12)).toBe("afternoon");
    expect(greetingKey(16)).toBe("afternoon");
    expect(greetingKey(17)).toBe("evening");
    expect(greetingKey(18)).toBe("evening");
    expect(greetingKey(23)).toBe("evening");
  });
});

describe("briefingReadKey", () => {
  it("returns the learning read for the learning state regardless of pace", () => {
    expect(briefingReadKey("learning", null)).toBe("preShift.briefing.learning");
    expect(briefingReadKey("learning", 12)).toBe("preShift.briefing.learning");
  });

  // The misleading-morning-pace guard: a null pace must NEVER render a pace
  // clause — it falls to the no-comparison read.
  it("returns the no-pace read when pacePct is null (active)", () => {
    expect(briefingReadKey("active", null)).toBe("preShift.briefing.read.noPace");
  });

  it("returns the ahead read for non-negative pace and the behind read for negative", () => {
    expect(briefingReadKey("active", 0)).toBe("preShift.briefing.read.withPace");
    expect(briefingReadKey("active", 12)).toBe("preShift.briefing.read.withPace");
    expect(briefingReadKey("active", -8)).toBe("preShift.briefing.read.withPaceBehind");
  });
});

describe("briefingHealthKey", () => {
  it("picks both / food / labor / none by which percentages are present", () => {
    expect(briefingHealthKey(0.28, 0.24)).toBe("preShift.briefing.health.both");
    expect(briefingHealthKey(0.28, null)).toBe("preShift.briefing.health.food");
    expect(briefingHealthKey(null, 0.24)).toBe("preShift.briefing.health.labor");
    expect(briefingHealthKey(null, null)).toBeNull();
  });

  it("treats 0 as present (a real measured zero, not absence)", () => {
    expect(briefingHealthKey(0, null)).toBe("preShift.briefing.health.food");
  });
});

describe("briefing play/win key builders", () => {
  it("keys the play sentence + CTA by kind", () => {
    expect(briefingPlayKey("reprice_up")).toBe("preShift.briefing.play.reprice_up");
    expect(briefingPlayKey("promote")).toBe("preShift.briefing.play.promote");
    expect(briefingPlayCtaKey("reprice_up")).toBe("preShift.briefing.play.cta.reprice_up");
    expect(briefingPlayCtaKey("promote")).toBe("preShift.briefing.play.cta.promote");
  });

  it("keys the win sentence by kind", () => {
    expect(briefingWinKey("revenue_up_wow")).toBe("preShift.briefing.win.revenue_up_wow");
  });
});

describe("ownerFirstName", () => {
  it("returns the first token of a human name", () => {
    expect(ownerFirstName("María González")).toBe("María");
    expect(ownerFirstName("  José  ")).toBe("José");
  });

  it("falls back to null for empty / wallet-address / non-letter names", () => {
    expect(ownerFirstName(undefined)).toBeNull();
    expect(ownerFirstName("")).toBeNull();
    expect(ownerFirstName("0x1f3bA9c2")).toBeNull();
    expect(ownerFirstName("123")).toBeNull();
  });

  it("rejects QA / test account handles so venues are not greeted as QA", () => {
    expect(ownerFirstName("QA")).toBeNull();
    expect(ownerFirstName("qa")).toBeNull();
    expect(ownerFirstName("TEST")).toBeNull();
    expect(ownerFirstName("demo")).toBeNull();
  });
});
