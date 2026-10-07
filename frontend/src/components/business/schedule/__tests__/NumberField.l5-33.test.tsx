/** @jest-environment jsdom */
/**
 * L5-33: ScheduleSettingsModal NumberField (production) must allow "8." /
 * "8.5" for step 0.5 and must not wipe a trailing comma to "0".
 * Rule 3: reverts of NumberField to native number parse-on-change fail here.
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import ScheduleSettingsModal from "../ScheduleSettingsModal";
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
  default_shift_minutes: 480, // 8h — step 0.5 NumberField
  reminder_lead_hours: 3,
  overtime_weekly_minutes: 2400,
  posted_lead_days: 7,
  minor_cutoff_min: null,
  quiet_hours_start_min: null,
  quiet_hours_end_min: null,
  created_at: "",
  updated_at: "",
};

function typeChars(el: HTMLElement, chars: string) {
  let acc = (el as HTMLInputElement).value || "";
  for (const ch of chars) {
    acc = acc + ch;
    fireEvent.change(el, { target: { value: acc } });
  }
}

function renderModal() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <ScheduleSettingsModal businessId="42" onClose={jest.fn()} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  (scheduleSettingsApi.get as jest.Mock).mockResolvedValue(baseSettings);
  (scheduleSettingsApi.update as jest.Mock).mockResolvedValue(baseSettings);
});

describe("L5-33 ScheduleSettingsModal NumberField (real component)", () => {
  it("allows intermediate 8. for step 0.5 shift length", async () => {
    renderModal();
    // Hydrated shift length from 480 minutes → "8"
    const input = (await screen.findByDisplayValue("8")) as HTMLInputElement;
    expect(input).toHaveAttribute("type", "text");
    expect(input).toHaveAttribute("inputmode", "decimal");

    fireEvent.change(input, { target: { value: "" } });
    typeChars(input, "8.");
    expect(input.value).toBe("8.");
    typeChars(input, "5");
    expect(input.value).toBe("8.5");
    fireEvent.blur(input);

    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(scheduleSettingsApi.update).toHaveBeenCalledWith(
        "42",
        expect.objectContaining({ default_shift_minutes: 510 }), // 8.5 * 60
      ),
    );
  });

  it("does not wipe the field when typing a trailing comma", async () => {
    renderModal();
    // reminder lead seeds as "3"
    const input = (await screen.findByDisplayValue("3")) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "" } });
    typeChars(input, "7,");
    // Broken path: Number("7,") → NaN → 0 → field shows "0".
    expect(input.value).toBe("7,");
    expect(input.value).not.toBe("0");
  });
});
