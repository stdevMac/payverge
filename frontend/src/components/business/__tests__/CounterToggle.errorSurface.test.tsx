/** @jest-environment jsdom */
/**
 * Blocker #1: a failed toggle must SURFACE the backend error via toast.
 * CounterToggle.tsx previously called surfaceBackendError (pure) and
 * discarded the result, so validation failures like
 * "counter_prefix must be at most 5 characters" were silent.
 *
 * surfaceBackendError is intentionally NOT mocked — the assertion is that the
 * backend-derived message reaches toast.error.
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

const backendMessage = "counter_prefix must be at most 5 characters";
const mockUpdate = jest.fn(() =>
  Promise.reject({
    response: { status: 400, data: { error: backendMessage } },
  }),
);
jest.mock("@/api/counters", () => ({
  getBusinessCounters: jest.fn(() =>
    Promise.resolve({
      business: { counter_enabled: false, counter_count: 3, counter_prefix: "C" },
    }),
  ),
  updateCounterSettings: (...a: unknown[]) => mockUpdate(...(a as [])),
}));

import toast from "react-hot-toast";
import { CounterToggle } from "@/components/business/CounterToggle";

describe("CounterToggle failed toggle", () => {
  it("surfaces the backend error message via toast.error", async () => {
    render(
      <CounterToggle businessId={1} isLocked={false} variant="card" enabled={false} />,
    );

    fireEvent.click(
      screen.getByText("businessDashboard.dashboard.counterManager.toggle.enable"),
    );
    fireEvent.click(
      await screen.findByText(
        "businessDashboard.dashboard.counterManager.toggle.enableModal.confirm",
      ),
    );

    await waitFor(() => expect(mockUpdate).toHaveBeenCalled());
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(backendMessage));
  });
});
