/** @jest-environment jsdom */
/**
 * D1 / L1-23: PreShiftCard must re-localize duration from oldest_minutes so a
 * Spanish operator never sees English "3 hours" injected by the backend.
 * Asserts mounted card text, not only formatInsightDurationMinutes helper math.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { Receipt } from "lucide-react";
import PreShiftCard from "../PreShiftCard";
import type { PreShiftCardModel } from "../insightCopy";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "es", setLocale: jest.fn() }),
  getTranslation: (
    key: string,
    _locale: string,
    params?: Record<string, string | number>,
  ) => {
    if (key === "directorConsole.duration.hours") {
      return `${params?.count} horas`;
    }
    if (key === "directorConsole.duration.minutes") {
      return `${params?.count} minutos`;
    }
    if (key === "directorConsole.duration.days") {
      return `${params?.count} días`;
    }
    if (key === "directorConsole.preShift.cards.staleBills.other") {
      return `${params?.count} cuentas abiertas desde hace más de ${params?.duration}`;
    }
    if (key === "directorConsole.preShift.action") return "Echar un vistazo";
    return params ? `${key} ${JSON.stringify(params)}` : key;
  },
}));

describe("PreShiftCard L1-23 Spanish duration DOM (D1)", () => {
  it("shows localized hours from oldest_minutes, not backend English duration", () => {
    const model: PreShiftCardModel = {
      id: "stale-open-bills",
      icon: Receipt,
      tone: "watch",
      copyKey: "staleBills.other",
      // Backend still sent English duration; card must prefer oldest_minutes.
      params: {
        count: 2,
        duration: "3 hours",
        oldest_minutes: 180,
      },
      tab: "bills",
    };

    render(<PreShiftCard model={model} onOpen={jest.fn()} currency="USD" />);

    const card = screen.getByTestId("preshift-card");
    expect(card.textContent).toMatch(/3 horas/);
    expect(card.textContent).not.toMatch(/3 hours/i);
    expect(card.textContent).toMatch(/2 cuentas abiertas/);
  });

  it("shows localized 40 days from oldest_minutes, not backend English '40 days'", () => {
    const model: PreShiftCardModel = {
      id: "stale-open-bills",
      icon: Receipt,
      tone: "watch",
      copyKey: "staleBills.other",
      params: {
        count: 2,
        duration: "40 days",
        oldest_minutes: 40 * 24 * 60,
      },
      tab: "bills",
    };

    render(<PreShiftCard model={model} onOpen={jest.fn()} currency="USD" />);

    const card = screen.getByTestId("preshift-card");
    expect(card.textContent).toMatch(/40 días/);
    expect(card.textContent).not.toMatch(/\b40 days\b/i);
  });

  it("re-localizes English duration when oldest_minutes is missing", () => {
    const model: PreShiftCardModel = {
      id: "stale-open-bills",
      icon: Receipt,
      tone: "watch",
      copyKey: "staleBills.other",
      params: {
        count: 2,
        duration: "40 days",
      },
      tab: "bills",
    };

    render(<PreShiftCard model={model} onOpen={jest.fn()} currency="USD" />);

    const card = screen.getByTestId("preshift-card");
    expect(card.textContent).toMatch(/40 días/);
    expect(card.textContent).not.toMatch(/\bdays\b/i);
  });
});
