/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import ScheduleSettingsModal from "./ScheduleSettingsModal";
import { scheduleSettingsApi, type ScheduleSettings } from "@/api/scheduleSettings";

const COPY: Record<string, string> = {
  title: "Schedule settings",
  subtitle: "Defaults for building and posting schedules",
  close: "Close",
  cancel: "Cancel",
  save: "Save",
  saving: "Saving…",
  saved: "Schedule settings saved",
  saveError: "Couldn't save schedule settings",
  sectionWeek: "Week & shifts",
  weekStart: "Week starts on",
  shiftLength: "Default shift length",
  hoursUnit: "hours",
  sectionReminders: "Reminders",
  reminderLead: "Remind staff before a shift",
  quietHours: "Quiet hours",
  quietHint: "No reminders are sent during these hours.",
  from: "From",
  to: "To",
  sectionCompliance: "Overtime & posting",
  overtime: "Overtime after",
  hoursPerWeek: "hours / week",
  postedLead: "Publish schedules ahead",
  daysUnit: "days",
  minorCurfew: "Minor curfew",
  minorHint: "Block shifts for minors past this time.",
  noShiftsPast: "No shifts past",
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => COPY[key.replace("dashboardScheduleSettings.", "")] ?? key,
}));
const mockShowSuccess = jest.fn();
const mockShowError = jest.fn();
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: mockShowSuccess,
    showError: mockShowError,
    showInfo: jest.fn(),
    showWarning: jest.fn(),
    showToast: jest.fn(),
  }),
}));
jest.mock("@/api/scheduleSettings", () => ({
  scheduleSettingsApi: { get: jest.fn(), update: jest.fn() },
}));

const mockedApi = scheduleSettingsApi as unknown as {
  get: jest.Mock;
  update: jest.Mock;
};

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

function renderModal() {
  const onClose = jest.fn();
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <ScheduleSettingsModal businessId="42" onClose={onClose} />
    </QueryClientProvider>,
  );
  return { onClose };
}

beforeEach(() => {
  jest.clearAllMocks();
  mockedApi.get.mockResolvedValue(baseSettings);
  mockedApi.update.mockResolvedValue(baseSettings);
});

test("loads settings then saves — disabled nullable fields send -1", async () => {
  const { onClose } = renderModal();
  // The reminder-lead field seeds from the loaded row (3h).
  const lead = (await screen.findByDisplayValue("3")) as HTMLInputElement;
  expect(lead).toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", { name: "Save" }));

  await waitFor(() => expect(mockedApi.update).toHaveBeenCalledTimes(1));
  const body = mockedApi.update.mock.calls[0][1];
  expect(body.week_start_day).toBe(1);
  expect(body.default_shift_minutes).toBe(480);
  expect(body.reminder_lead_hours).toBe(3);
  expect(body.quiet_hours_start_min).toBe(-1);
  expect(body.quiet_hours_end_min).toBe(-1);
  expect(body.minor_cutoff_min).toBe(-1);
  await waitFor(() => expect(onClose).toHaveBeenCalled());
  expect(mockShowSuccess).toHaveBeenCalled();
});

test("enabling quiet hours sends the minute-of-day window", async () => {
  renderModal();
  await screen.findByDisplayValue("3");

  fireEvent.click(screen.getByLabelText("Quiet hours"));
  // Time inputs appear seeded with the 22:00–07:00 defaults.
  expect(screen.getByDisplayValue("22:00")).toBeInTheDocument();
  expect(screen.getByDisplayValue("07:00")).toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(mockedApi.update).toHaveBeenCalledTimes(1));
  const body = mockedApi.update.mock.calls[0][1];
  expect(body.quiet_hours_start_min).toBe(1320); // 22:00
  expect(body.quiet_hours_end_min).toBe(420); // 07:00
});

test("save failure surfaces an error toast and keeps the modal open", async () => {
  mockedApi.update.mockRejectedValue(new Error("403"));
  const { onClose } = renderModal();
  await screen.findByDisplayValue("3");

  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(mockShowError).toHaveBeenCalled());
  expect(onClose).not.toHaveBeenCalled();
});

test("money-free: no dollar sign in the editor", async () => {
  const { container } = (() => {
    const onClose = jest.fn();
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
      <QueryClientProvider client={qc}>
        <ScheduleSettingsModal businessId="42" onClose={onClose} />
      </QueryClientProvider>,
    );
  })();
  await screen.findByDisplayValue("3");
  expect(container.textContent).not.toContain("$");
});

// L5-34: fieldsets use flex+gap+pt-1 so outside labels clear the legend.
test("fieldset sections use flex gap pt-1 spacing (L5-34)", async () => {
  renderModal();
  await screen.findByDisplayValue("3");
  for (const id of [
    "schedule-settings-week",
    "schedule-settings-reminders",
    "schedule-settings-compliance",
  ]) {
    const el = screen.getByTestId(id);
    expect(el).toHaveClass("flex", "flex-col", "gap-4", "pt-1");
  }
});
