/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import AvailabilityEditor, { type AvailabilityEditorLabels } from "./AvailabilityEditor";
import { availabilityApi } from "@/api/availability";
import type { StaffData } from "@/utils/staffAuth";

jest.mock("@/api/availability", () => ({
  availabilityApi: { getMine: jest.fn(), putMine: jest.fn() },
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

const mocked = availabilityApi as unknown as { getMine: jest.Mock; putMine: jest.Mock };

const staff: StaffData = {
  id: 7,
  name: "Dana",
  email: "dana@example.com",
  role: "server",
  business_id: 42,
  business_name: "Cafe 42",
  is_active: true,
};

const labels: AvailabilityEditorLabels = {
  title: "Availability",
  subtitle: "Let your manager know when you can work",
  loading: "Loading your availability",
  error: "Could not load availability",
  emptyTitle: "No availability set",
  emptySubtitle: "Add the windows when you prefer to work",
  addTitle: "Add a window",
  weekdayLabel: "Day",
  kindLabel: "Type",
  kindPreferred: "Preferred",
  kindUnavailable: "Unavailable",
  startLabel: "From",
  endLabel: "To",
  add: "Add window",
  remove: "Remove window",
  invalidRange: "End must be after start",
  save: "Save availability",
  saving: "Saving",
  saved: "Availability saved",
  saveError: "Couldn't save",
};

function renderEditor() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <AvailabilityEditor staff={staff} labels={labels} locale="en" />
    </QueryClientProvider>,
  );
}

const oneWindow = {
  id: 1,
  business_id: 42,
  staff_id: 7,
  weekday: 1, // Monday
  start_min: 540, // 9:00 AM
  end_min: 1020, // 5:00 PM
  kind: "preferred" as const,
  created_at: "",
  updated_at: "",
};

beforeEach(() => jest.clearAllMocks());

describe("AvailabilityEditor", () => {
  it("renders existing windows as friendly time labels (no dollars)", async () => {
    mocked.getMine.mockResolvedValue([oneWindow]);
    renderEditor();

    // The remove control proves a window row rendered.
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Remove window" })).toBeInTheDocument(),
    );
    // Friendly minute-of-day → time labels (not the raw HH:MM input value).
    expect(document.body.textContent).toMatch(/9:00/);
    expect(document.body.textContent).toMatch(/5:00/);
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("shows the empty state when no windows are set", async () => {
    mocked.getMine.mockResolvedValue([]);
    renderEditor();

    await waitFor(() =>
      expect(screen.getByText("No availability set")).toBeInTheDocument(),
    );
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("saves the current windows via putMine (full replace)", async () => {
    mocked.getMine.mockResolvedValue([oneWindow]);
    mocked.putMine.mockResolvedValue([oneWindow]);
    renderEditor();

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Remove window" })).toBeInTheDocument(),
    );
    await userEvent.click(screen.getByRole("button", { name: "Save availability" }));

    await waitFor(() =>
      expect(mocked.putMine).toHaveBeenCalledWith("42", [
        { weekday: 1, start_min: 540, end_min: 1020, kind: "preferred" },
      ]),
    );
    await waitFor(() => expect(mockShowSuccess).toHaveBeenCalledWith("Availability saved"));
  });

  it("seeds from a fetch after mount, never from rows already in the cache", async () => {
    let resolveFetch: (rows: (typeof oneWindow)[]) => void = () => {};
    mocked.getMine.mockReturnValue(
      new Promise((resolve) => {
        resolveFetch = resolve;
      }),
    );
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    // Rows left in the cache by an earlier session.
    qc.setQueryData(["availability", "42", "mine", "7"], [
      { ...oneWindow, id: 99, start_min: 660, end_min: 720 },
    ]);
    render(
      <QueryClientProvider client={qc}>
        <AvailabilityEditor staff={staff} labels={labels} locale="en" />
      </QueryClientProvider>,
    );

    expect(screen.queryByRole("button", { name: "Remove window" })).toBeNull();
    resolveFetch([oneWindow]);

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Remove window" })).toBeInTheDocument(),
    );
    expect(document.body.textContent).toMatch(/9:00/);
    expect(document.body.textContent).not.toMatch(/11:00/);
  });
});
