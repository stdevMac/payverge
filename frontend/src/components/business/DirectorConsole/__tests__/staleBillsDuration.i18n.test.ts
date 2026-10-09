import { getTranslation } from "@/i18n/getTranslation";
import type { Locale } from "@/i18n/localeRegistry";
import type { ProactiveInsightDTO } from "@/api/directorConsole";
import {
  formatInsightCardLine,
  toPreShiftCard,
} from "../insightCopy";

const FORTY_DAY_MINUTES = 40 * 24 * 60;
const ENGLISH_UNITS = /\b(days?|hours?|minutes?)\b/i;

const FORTY_DAY_ALERT: Record<Locale, string> = {
  en: "2 bills open longer than 40 days — check if guests slipped out.",
  es: "2 cuentas abiertas desde hace más de 40 días — fíjate si los clientes se fueron.",
  "es-AR":
    "2 cuentas abiertas desde hace más de 40 días — fijate si los clientes se fueron.",
};

function directorT(locale: Locale) {
  return (key: string, params?: Record<string, string | number>): string => {
    const value = getTranslation(`directorConsole.${key}`, locale, params);
    return Array.isArray(value) ? value[0] || key : String(value);
  };
}

function staleInsight(
  params: Record<string, unknown>,
): ProactiveInsightDTO {
  return {
    id: "stale-open-bills",
    type: "stale_open_bills",
    params,
    cta: { tab: "bills" },
  };
}

function staleAlert(
  locale: Locale,
  params: Record<string, unknown>,
  t?: (key: string, p?: Record<string, string | number>) => string,
): string {
  const card = toPreShiftCard(staleInsight(params), t);
  if (!card) {
    throw new Error("expected stale_open_bills card");
  }
  return formatInsightCardLine(card, directorT(locale));
}

describe("Director Console stale-bills 40-day duration (#581)", () => {
  const locales: Locale[] = ["en", "es", "es-AR"];

  it.each(locales)(
    "%s localizes 40 days from oldest_minutes, never English units in Spanish",
    (locale) => {
      const line = staleAlert(locale, {
        count: 2,
        duration: "40 days",
        oldest_minutes: FORTY_DAY_MINUTES,
      });

      expect(line).toBe(FORTY_DAY_ALERT[locale]);
      if (locale === "en") {
        expect(line).toContain("40 days");
      } else {
        expect(line).toContain("40 días");
        expect(line).not.toMatch(ENGLISH_UNITS);
      }
    },
  );

  it.each(locales)(
    "%s localizes 40 days when oldest_minutes is missing and only English duration is present",
    (locale) => {
      const line = staleAlert(locale, {
        count: 2,
        duration: "40 days",
      });

      expect(line).toBe(FORTY_DAY_ALERT[locale]);
      if (locale !== "en") {
        expect(line).not.toMatch(ENGLISH_UNITS);
      }
    },
  );

  it.each(locales)(
    "%s localizes 40 days when oldest_minutes arrives as a numeric string",
    (locale) => {
      const line = staleAlert(locale, {
        count: 2,
        duration: "40 days",
        oldest_minutes: String(FORTY_DAY_MINUTES),
      });

      expect(line).toBe(FORTY_DAY_ALERT[locale]);
    },
  );

  it("toPreShiftCard with a translator never stores English duration for es-AR", () => {
    const card = toPreShiftCard(
      staleInsight({
        count: 2,
        duration: "40 days",
        oldest_minutes: FORTY_DAY_MINUTES,
      }),
      directorT("es-AR"),
    );

    expect(card?.params.duration).toBe("40 días");
    expect(card?.params.duration).not.toMatch(/days/i);
    expect(formatInsightCardLine(card!, directorT("es-AR"))).toBe(
      FORTY_DAY_ALERT["es-AR"],
    );
  });

  it.each(["es", "es-AR"] as const)(
    "%s sibling insight cards do not interpolate raw English duration units",
    (locale) => {
      const t = directorT(locale);
      const siblings: ProactiveInsightDTO[] = [
        staleInsight({
          count: 2,
          duration: "40 days",
          oldest_minutes: FORTY_DAY_MINUTES,
        }),
        staleInsight({ count: 2, duration: "3 hours" }),
        {
          id: "m1",
          type: "marketing_posts",
          params: { count: 2, period_days: 7 },
          cta: { tab: "marketing" },
        },
        {
          id: "o1",
          type: "inventory_out_of_stock",
          params: { count: 2, item_names: ["Pan"] },
          cta: { tab: "inventory" },
        },
        {
          id: "l1",
          type: "inventory_low_stock",
          params: { count: 3, item_names: ["Pan", "Leche", "Huevos"] },
          cta: { tab: "inventory" },
        },
        {
          id: "a1",
          type: "ai_conversations_pending",
          params: { count: 2 },
          cta: { tab: "ai-waiter" },
        },
        {
          id: "f1",
          type: "food_cost_high",
          params: { count: 2, item_names: ["Milanesa"] },
          cta: { tab: "accounting" },
        },
        {
          id: "w1",
          type: "waste_high",
          params: { amount: 80, ingredient: "salmon" },
          cta: { tab: "accounting" },
        },
        {
          id: "lb1",
          type: "labor_high",
          params: { pct: 0.4, amount: 100 },
          cta: { tab: "accounting" },
        },
      ];

      for (const insight of siblings) {
        const card = toPreShiftCard(insight, t);
        expect(card).not.toBeNull();
        const line = formatInsightCardLine(card!, t);
        expect(line).not.toMatch(ENGLISH_UNITS);
      }
    },
  );
});
