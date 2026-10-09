/**
 * #664: the "week starts on" picker must label each option with the weekday it
 * actually stores. The labels are built from a UTC-anchored date, so they have
 * to be formatted in UTC too — otherwise every device behind UTC shifts the
 * whole list back a day and a stored Monday (week_start_day=1) is presented as
 * "Sunday" while the grid correctly renders MON→SUN.
 *
 * @jest-environment jsdom
 */

// Force a negative-offset device timezone so the test is independent of the
// machine it runs on. Must happen before the component formats any date.
const ORIGINAL_TZ = process.env.TZ;
process.env.TZ = "America/Buenos_Aires";

import React from "react";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import ScheduleSettingsModal from "../ScheduleSettingsModal";

jest.mock("@/api/scheduleSettings", () => ({
  scheduleSettingsApi: {
    get: () =>
      Promise.resolve({
        week_start_day: 1, // Monday — what the venue has persisted
        default_shift_minutes: 480,
        reminder_lead_hours: 2,
        quiet_hours_start_min: null,
        quiet_hours_end_min: null,
        overtime_weekly_minutes: 2400,
        posted_lead_days: 7,
        minor_cutoff_min: null,
      }),
    update: jest.fn(),
  },
}));

let mockLocale = "en";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: mockLocale, setLocale: jest.fn() }),
  getTranslation: (_locale: string, key: string) => key,
}));

jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: jest.fn(),
    showError: jest.fn(),
    showInfo: jest.fn(),
    showWarning: jest.fn(),
  }),
}));

function renderModal() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ScheduleSettingsModal businessId="1" onClose={jest.fn()} />
    </QueryClientProvider>,
  );
}

afterAll(() => {
  process.env.TZ = ORIGINAL_TZ;
});

beforeEach(() => {
  mockLocale = "en";
});

describe("#664 schedule week-start option labels", () => {
  it("labels every option with the weekday it stores on a device behind UTC", async () => {
    const { container } = renderModal();
    await screen.findByTestId("schedule-settings-week");

    const hidden = container.querySelector("select");
    expect(hidden).not.toBeNull();

    const labelFor = (key: string) =>
      hidden!.querySelector(`option[value="${key}"]`)?.textContent;

    expect(labelFor("0")).toBe("Sunday");
    expect(labelFor("1")).toBe("Monday");
    expect(labelFor("6")).toBe("Saturday");
  });

  it("shows the persisted week start (Monday) as the selected value", async () => {
    const { container } = renderModal();
    const fieldset = await screen.findByTestId("schedule-settings-week");

    const hidden = container.querySelector("select") as HTMLSelectElement;
    expect(hidden.value).toBe("1");

    const trigger = fieldset.querySelector('[data-slot="value"]');
    expect(trigger?.textContent).toBe("Monday");
  });

  it("labels a persisted Monday as lunes, not domingo, in Spanish on ART", async () => {
    mockLocale = "es";
    const { container } = renderModal();
    const fieldset = await screen.findByTestId("schedule-settings-week");

    const hidden = container.querySelector("select") as HTMLSelectElement;
    expect(hidden.value).toBe("1");
    expect(hidden.querySelector('option[value="1"]')?.textContent).toMatch(
      /^lunes$/i,
    );
    expect(hidden.querySelector('option[value="1"]')?.textContent).not.toMatch(
      /domingo/i,
    );

    const trigger = fieldset.querySelector('[data-slot="value"]');
    expect(trigger?.textContent).toMatch(/^lunes$/i);
    expect(trigger?.textContent).not.toMatch(/domingo/i);
  });
});
