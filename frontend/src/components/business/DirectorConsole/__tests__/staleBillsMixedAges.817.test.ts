/**
 * #817 — the demo venue has TWO open checks with very different ages:
 * bill 1143 (Table 1, ~1.16 days) and bill 761 (Table 9, ~7.39 days).
 *
 * The copy shape "{count} bills open longer than {duration}" can only carry
 * ONE age, so it has to either smear the oldest age over both checks (the
 * original bug: "2 bills open longer than 6 days") or drop the younger check
 * from the count entirely (f6648da6e: "1 bill open longer than 7 days", and
 * 1143 vanishes from the briefing).
 *
 * Neither is honest. When the backend says the duration bucket holds fewer
 * bills than the stale set (stale_count > count), the card must name the real
 * total and attribute the age to the oldest one only.
 */
import { getTranslation } from "@/i18n/getTranslation";
import type { Locale } from "@/i18n/localeRegistry";
import type { ProactiveInsightDTO } from "@/api/directorConsole";
import { formatInsightCardLine, toPreShiftCard } from "../insightCopy";

const DAY = 24 * 60;
// Live payload for business 86: oldest is bill 761 at ~7.39 days, and only
// that one clears the "7 days" bucket floor, but two bills are open.
const LIVE_PARAMS = {
  count: 1,
  stale_count: 2,
  oldest_minutes: Math.round(7.39 * DAY),
  duration: "7 days",
  threshold_minutes: 120,
};

function directorT(locale: Locale) {
  return (key: string, params?: Record<string, string | number>): string => {
    const value = getTranslation(`directorConsole.${key}`, locale, params);
    return Array.isArray(value) ? value[0] || key : String(value);
  };
}

function line(locale: Locale, params: Record<string, unknown>): string {
  const dto: ProactiveInsightDTO = {
    id: "stale-open-bills",
    type: "stale_open_bills",
    params,
    cta: { tab: "bills" },
  };
  const card = toPreShiftCard(dto, directorT(locale));
  if (!card) throw new Error("expected stale_open_bills card");
  return formatInsightCardLine(card, directorT(locale));
}

describe("#817 stale-bills card with mixed ages", () => {
  it("names both open checks and pins the age to the oldest one", () => {
    const en = line("en", LIVE_PARAMS);
    // The real total, not the bucket count.
    expect(en).toMatch(/\b2 bills\b/);
    // The age belongs to the oldest bill, not to both.
    expect(en).toContain("7 days");
    // The smear and the erasure are both gone.
    expect(en).not.toMatch(/2 bills open longer than/);
    expect(en).not.toMatch(/^1 bill\b/);
  });

  it("keeps the plain wording when every stale bill is in the bucket", () => {
    const en = line("en", {
      count: 2,
      stale_count: 2,
      oldest_minutes: 7 * DAY,
      duration: "7 days",
    });
    expect(en).toBe(
      "2 bills open longer than 7 days — check if guests slipped out.",
    );
  });

  it("keeps the plain wording when the backend omits stale_count", () => {
    // Older payloads and any consumer that predates #817.
    const en = line("en", { count: 2, oldest_minutes: 7 * DAY, duration: "7 days" });
    expect(en).toBe(
      "2 bills open longer than 7 days — check if guests slipped out.",
    );
  });

  it.each(["es", "es-AR"] as const)(
    "%s renders the mixed line localized, with no English units",
    (locale) => {
      const out = line(locale, LIVE_PARAMS);
      expect(out).toMatch(/\b2 cuentas\b/);
      expect(out).toContain("7 días");
      expect(out).not.toMatch(/\b(days?|hours?|minutes?|bills?)\b/i);
      // Neither locale may fall through to the raw key.
      expect(out).not.toContain("preShift.cards");
      expect(out).not.toContain("staleBillsOldest");
    },
  );

  it("es-AR keeps the voseo imperative", () => {
    expect(line("es-AR", LIVE_PARAMS)).toContain("Fijate");
    expect(line("es", LIVE_PARAMS)).toContain("Fíjate");
  });
});
