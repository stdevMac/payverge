/**
 * NR-2: ScheduleSettingsModal four numeric fields must reject garbage and
 * out-of-range values with a visible error and disabled Save — never leave
 * Save enabled while the field shows "abc" or "999".
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import ScheduleSettingsModal from "../ScheduleSettingsModal";

const mockUpdate = jest.fn();

jest.mock("@/api/scheduleSettings", () => {
  const sample = {
    week_start_day: 1,
    default_shift_minutes: 480, // 8h
    reminder_lead_hours: 2,
    quiet_hours_start_min: null,
    quiet_hours_end_min: null,
    overtime_weekly_minutes: 2400, // 40h
    posted_lead_days: 7,
    minor_cutoff_min: null,
  };
  return {
    scheduleSettingsApi: {
      get: () => Promise.resolve(sample),
      update: (...args: unknown[]) => mockUpdate(...args),
    },
  };
});

jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: jest.fn(),
    showError: jest.fn(),
    showInfo: jest.fn(),
    showWarning: jest.fn(),
  }),
}));

function renderModal() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={qc}>
      <ScheduleSettingsModal businessId="1" onClose={jest.fn()} />
    </QueryClientProvider>,
  );
}

function typeChars(el: HTMLElement, chars: string) {
  let acc = (el as HTMLInputElement).value || "";
  for (const ch of chars) {
    acc = acc + ch;
    fireEvent.change(el, { target: { value: acc } });
  }
}

beforeEach(() => {
  mockUpdate.mockReset();
  mockUpdate.mockImplementation(() =>
    Promise.resolve({
      week_start_day: 1,
      default_shift_minutes: 480,
      reminder_lead_hours: 2,
      quiet_hours_start_min: null,
      quiet_hours_end_min: null,
      overtime_weekly_minutes: 2400,
      posted_lead_days: 7,
      minor_cutoff_min: null,
    }),
  );
});

describe("NR-2 ScheduleSettingsModal number validation", () => {
  it("shift hours: abc is aria-invalid and Save stays disabled", async () => {
    renderModal();
    const input = await screen.findByTestId("schedule-shift-hours");
    fireEvent.change(input, { target: { value: "" } });
    typeChars(input, "abc");
    await waitFor(() => {
      expect(input).toHaveAttribute("aria-invalid", "true");
    });
    // Field keeps garbage visible (no silent collapse to 0).
    expect((input as HTMLInputElement).value).toBe("abc");
    const save = screen.getByRole("button", { name: /^save$/i });
    expect(save).toBeDisabled();
    expect(mockUpdate).not.toHaveBeenCalled();
  });

  it("shift hours: 999 clamps to 168 on blur", async () => {
    renderModal();
    const input = await screen.findByTestId("schedule-shift-hours");
    fireEvent.change(input, { target: { value: "" } });
    typeChars(input, "999");
    expect((input as HTMLInputElement).value).toBe("999");
    // Out of range before blur → invalid.
    await waitFor(() => {
      expect(input).toHaveAttribute("aria-invalid", "true");
    });
    fireEvent.blur(input);
    await waitFor(() => {
      expect((input as HTMLInputElement).value).toMatch(/^168(\.00)?$/);
    });
  });

  it("shift hours: accepts 2,5 per keystroke (comma decimal)", async () => {
    renderModal();
    const input = await screen.findByTestId("schedule-shift-hours");
    fireEvent.change(input, { target: { value: "" } });
    typeChars(input, "2");
    expect((input as HTMLInputElement).value).toBe("2");
    typeChars(input, ",");
    expect((input as HTMLInputElement).value).toBe("2,");
    typeChars(input, "5");
    expect((input as HTMLInputElement).value).toBe("2,5");
    fireEvent.blur(input);
    await waitFor(() => {
      // Normalized display may use fixed decimals.
      expect((input as HTMLInputElement).value).toMatch(/2[,.]5/);
    });
  });

  it("all four numeric fields expose test ids and reject garbage", async () => {
    renderModal();
    const ids = [
      "schedule-shift-hours",
      "schedule-reminder-lead",
      "schedule-overtime-hours",
      "schedule-posted-lead",
    ];
    for (const id of ids) {
      const input = await screen.findByTestId(id);
      fireEvent.change(input, { target: { value: "" } });
      typeChars(input, "xyz");
      await waitFor(() => {
        expect(input).toHaveAttribute("aria-invalid", "true");
      });
      expect((input as HTMLInputElement).value).toBe("xyz");
    }
    expect(screen.getByRole("button", { name: /^save$/i })).toBeDisabled();
  });
});
