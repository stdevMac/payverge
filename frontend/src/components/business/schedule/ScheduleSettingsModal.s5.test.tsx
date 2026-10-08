/** @jest-environment jsdom */
/**
 * L5-33 — Schedule settings number fields: form noValidate + text inputs
 * (no native English min/max/step bubbles).
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import ScheduleSettingsModal from "./ScheduleSettingsModal";
import { scheduleSettingsApi, type ScheduleSettings } from "@/api/scheduleSettings";

const COPY: Record<string, string> = {
  title: "Schedule settings",
  subtitle: "Defaults",
  close: "Close",
  cancel: "Cancel",
  save: "Save",
  saving: "Saving…",
  saved: "Saved",
  saveError: "Error",
  sectionWeek: "Week & shifts",
  weekStart: "Week starts on",
  shiftLength: "Default shift length",
  hoursUnit: "hours",
  sectionReminders: "Reminders",
  reminderLead: "Remind staff before a shift",
  quietHours: "Quiet hours",
  quietHint: "No reminders",
  from: "From",
  to: "To",
  sectionCompliance: "Overtime & posting",
  overtime: "Overtime after",
  hoursPerWeek: "hours / week",
  postedLead: "Publish schedules ahead",
  daysUnit: "days",
  minorCurfew: "Minor curfew",
  minorHint: "Block shifts",
  noShiftsPast: "No shifts past",
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) =>
    COPY[key.replace("dashboardScheduleSettings.", "")] ?? key,
}));
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: jest.fn(),
    showError: jest.fn(),
    showInfo: jest.fn(),
    showWarning: jest.fn(),
    showToast: jest.fn(),
  }),
}));
jest.mock("@/api/scheduleSettings", () => ({
  scheduleSettingsApi: { get: jest.fn(), update: jest.fn() },
}));

const baseSettings: ScheduleSettings = {
  id: 1,
  business_id: 42,
  week_start_day: 1,
  default_shift_minutes: 480,
  reminder_lead_hours: 3,
  overtime_weekly_minutes: 2400,
  posted_lead_days: 7,
  minor_cutoff_min: null,
  quiet_hours_start_min: null,
  quiet_hours_end_min: null,
  created_at: "",
  updated_at: "",
};

describe("L5-33 ScheduleSettingsModal number validation", () => {
  beforeEach(() => {
    (scheduleSettingsApi.get as jest.Mock).mockResolvedValue(baseSettings);
    (scheduleSettingsApi.update as jest.Mock).mockResolvedValue(baseSettings);
  });

  it("form has noValidate and number fields are type=text", async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { container } = render(
      <QueryClientProvider client={qc}>
        <ScheduleSettingsModal businessId="42" onClose={jest.fn()} />
      </QueryClientProvider>,
    );

    // Wait until settings form is hydrated (Save appears only after load).
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /Save/i })).toBeInTheDocument();
    });

    const form = container.querySelector("form");
    expect(form).not.toBeNull();
    expect(form).toHaveAttribute("novalidate");

    // Number fields must not be native type=number (English step/min bubbles).
    const numberInputs = container.querySelectorAll('input[type="number"]');
    expect(numberInputs.length).toBe(0);
  });
});
