import {
  formatInsightDurationMinutes,
  toPreShiftCard,
} from "../insightCopy";

describe("L1-23 insight duration localization", () => {
  it("formats minutes into localized hours/days", () => {
    const t = (key: string, params?: Record<string, string | number>) => {
      if (key === "duration.hours") return `${params?.count} horas`;
      if (key === "duration.minutes") return `${params?.count} minutos`;
      if (key === "duration.days") return `${params?.count} días`;
      return key;
    };
    expect(formatInsightDurationMinutes(180, t)).toBe("3 horas");
    expect(formatInsightDurationMinutes(45, t)).toBe("45 minutos");
    expect(formatInsightDurationMinutes(60 * 24 * 2, t)).toBe("2 días");
  });

  it("stale_open_bills card carries oldest_minutes for FE re-localization", () => {
    const card = toPreShiftCard({
      id: "stale-open-bills",
      type: "stale_open_bills",
      params: {
        count: 2,
        duration: "3 hours",
        oldest_minutes: 180,
      },
      cta: { tab: "bills" },
    } as any);
    expect(card?.params.oldest_minutes).toBe(180);
    // Raw English duration may still be present as fallback; the renderer
    // must prefer oldest_minutes (asserted via PreShiftCard / format helper).
    expect(card?.params.duration).toBeTruthy();
  });

  it("formats 40 days through the translator instead of English backend prose", () => {
    const t = (key: string, params?: Record<string, string | number>) => {
      if (key === "duration.days") return `${params?.count} días`;
      if (key === "duration.hours") return `${params?.count} horas`;
      return key;
    };
    expect(formatInsightDurationMinutes(40 * 24 * 60, t)).toBe("40 días");
    const card = toPreShiftCard(
      {
        id: "stale-open-bills",
        type: "stale_open_bills",
        params: { count: 2, duration: "40 days", oldest_minutes: 40 * 24 * 60 },
        cta: { tab: "bills" },
      } as any,
      t,
    );
    expect(card?.params.duration).toBe("40 días");
  });
});
