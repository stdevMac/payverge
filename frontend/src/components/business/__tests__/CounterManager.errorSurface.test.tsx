/** @jest-environment jsdom */
/**
 * Blocker #2: a failed settings save must SURFACE the backend error inline.
 * CounterManager.tsx called surfaceBackendError (a pure function) and threw the
 * result away, then set the old generic string — swallowing backend validation
 * copy like "counter_prefix must be at most 5 characters".
 *
 * surfaceBackendError is intentionally NOT mocked.
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

const mockGetBusinessCounters = jest.fn();
const mockUpdateCounterSettings = jest.fn();

jest.mock("../../../api/counters", () => ({
  getBusinessCounters: (...a: unknown[]) => mockGetBusinessCounters(...a),
  updateCounterSettings: (...a: unknown[]) => mockUpdateCounterSettings(...a),
  COUNTER_PREFIX_MAX_LENGTH: 5,
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({ hasAccess: true, loading: false }),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("../CounterToggle", () => ({ CounterToggle: () => null }));
jest.mock("../CounterSkeleton", () => ({ CounterSkeleton: () => null }));
jest.mock("../DashboardLockedTabView", () => ({
  __esModule: true,
  default: () => null,
}));

import CounterManager from "../CounterManager";

const backendMessage = "counter_prefix must be at most 5 characters";

beforeEach(() => {
  jest.clearAllMocks();
  mockGetBusinessCounters.mockResolvedValue({
    counters: [{ id: 1, name: "C1", is_active: true }],
    business: {
      counter_enabled: true,
      counter_count: 3,
      counter_prefix: "C",
    },
  });
  mockUpdateCounterSettings.mockRejectedValue({
    response: { status: 400, data: { error: backendMessage } },
  });
});

it("shows the backend error message when saving counter settings fails", async () => {
  render(<CounterManager businessId={1} />);

  const input = (await screen.findByLabelText(
    "businessDashboard.dashboard.counterManager.settings.counterCount",
  )) as HTMLInputElement;
  fireEvent.change(input, { target: { value: "5" } });

  fireEvent.click(await screen.findByText("businessSettings.saveBar.save"));

  await waitFor(() => expect(mockUpdateCounterSettings).toHaveBeenCalled());
  await waitFor(() =>
    expect(screen.getByText(backendMessage)).toBeInTheDocument(),
  );
});
