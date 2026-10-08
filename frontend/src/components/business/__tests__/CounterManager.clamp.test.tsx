/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

const mockGetBusinessCounters = jest.fn();
const mockUpdateCounterSettings = jest.fn();

jest.mock("../../../api/counters", () => ({
  getBusinessCounters: (...a: unknown[]) => mockGetBusinessCounters(...a),
  updateCounterSettings: (...a: unknown[]) => mockUpdateCounterSettings(...a),
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
jest.mock("@/components/ui/spinners/PrimarySpinner", () => ({
  PrimarySpinner: () => null,
}));

import CounterManager from "../CounterManager";

beforeEach(() => {
  jest.clearAllMocks();
  // L2-31/L2-36: enablement comes from business.counter_enabled, never from
  // the counter row count — the mock must carry the business block for the
  // count input and SaveBar to render.
  mockGetBusinessCounters.mockResolvedValue({
    counters: [{ id: 1, name: "C1", is_active: true }],
    business: {
      counter_enabled: true,
      counter_count: 3,
      counter_prefix: "C",
    },
  });
  mockUpdateCounterSettings.mockResolvedValue({});
});

it("clamps an out-of-range counter_count to the max on save (F11)", async () => {
  render(<CounterManager businessId={1} />);

  // The count input is labelled via the settings.counterCount key.
  const input = (await screen.findByLabelText(
    "businessDashboard.dashboard.counterManager.settings.counterCount",
  )) as HTMLInputElement;

  // Type an absurd value; the upper bound clamps to 20 immediately.
  fireEvent.change(input, { target: { value: "500" } });
  expect(input.value).toBe("20");

  // Save via the bottom SaveBar (Counter is a settings-style tab — no header
  // save button, same floating bar as Settings/Business Page).
  const saveBtn = await screen.findByText("businessSettings.saveBar.save");
  fireEvent.click(saveBtn);

  await waitFor(() => expect(mockUpdateCounterSettings).toHaveBeenCalled());
  const sentSettings = mockUpdateCounterSettings.mock.calls[0][1];
  expect(sentSettings.counter_count).toBe(20);
});

it("uses the SaveBar, not a header save button, and flags unsaved edits", async () => {
  render(<CounterManager businessId={1} />);

  const input = (await screen.findByLabelText(
    "businessDashboard.dashboard.counterManager.settings.counterCount",
  )) as HTMLInputElement;

  // The old header CTA is gone; the floating SaveBar is the one save surface.
  expect(
    screen.queryByText(
      "businessDashboard.dashboard.counterManager.buttons.saveSettings",
    ),
  ).toBeNull();
  expect(screen.getByText("businessSettings.saveBar.save")).toBeInTheDocument();

  // Pristine: honest idle copy, never an autosave claim (#380).
  expect(screen.queryByText(/saved automatically/i)).toBeNull();
  expect(screen.getByText("businessSettings.saveBar.clean")).toBeInTheDocument();

  // Pristine: no unsaved-changes pill. Edit -> pill appears.
  expect(
    screen.queryByText("businessSettings.saveBar.unsaved"),
  ).toBeNull();
  fireEvent.change(input, { target: { value: "5" } });
  expect(
    screen.getByText("businessSettings.saveBar.unsaved"),
  ).toBeInTheDocument();
});
