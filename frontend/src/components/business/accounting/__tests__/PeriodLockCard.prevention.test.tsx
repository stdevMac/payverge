/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import PeriodLockCard from "@/components/business/accounting/PeriodLockCard";

const mockGetPeriodLock = jest.fn();
const mockPostPeriodLock = jest.fn();
const mockListEntries = jest.fn();

jest.mock("@/api/accounting", () => ({
  accountingApi: {
    getPeriodLock: (...a: unknown[]) => mockGetPeriodLock(...a),
    postPeriodLock: (...a: unknown[]) => mockPostPeriodLock(...a),
    listEntries: (...a: unknown[]) => mockListEntries(...a),
  },
}));

const t = (key: string, params?: Record<string, string | number>) => {
  if (key === "periodLock.freezePreview" && params) {
    return `Will freeze ${params.count} entries from ${params.start} through ${params.end}.`;
  }
  const map: Record<string, string> = {
    "periodLock.title": "Close the books",
    "periodLock.open": "Books are open",
    "periodLock.closedThrough": "Books closed through {date}",
    "periodLock.closeDate": "Close through",
    "periodLock.note": "Note (required)",
    "periodLock.noteRequired": "A note is required.",
    "periodLock.dateRequired": "periodLock.dateRequired",
    "periodLock.confirmCloseTitle": "periodLock.confirmCloseTitle",
    "periodLock.confirmCloseAction": "periodLock.confirmCloseAction",
    "periodLock.closeBooks": "Close books",
    "periodLock.reopen": "Reopen",
    "periodLock.loadError": "Could not load period lock.",
    "periodLock.saveError": "Could not update period lock.",
  };
  return map[key] || key;
};

describe("PeriodLockCard — preventive close-books UX", () => {
  beforeEach(() => {
    mockGetPeriodLock.mockReset();
    mockPostPeriodLock.mockReset();
    mockListEntries.mockReset();
    mockGetPeriodLock.mockResolvedValue({ locked_through: null });
    mockListEntries.mockResolvedValue({
      entries: [],
      total: 12,
      page: 1,
      page_size: 1,
      total_pages: 12,
    });
  });

  it("marks the note as required and disables submit while blank", async () => {
    render(
      <PeriodLockCard businessId="1" canOwn t={t} />,
    );
    await waitFor(() =>
      expect(mockGetPeriodLock).toHaveBeenCalled(),
    );

    const note = screen.getByLabelText(/note/i);
    // isRequired surfaces as required attribute / aria-required on NextUI Input.
    expect(
      note.getAttribute("aria-required") === "true" ||
        (note as HTMLInputElement).required === true ||
        note.closest("[data-required]") !== null ||
        note.getAttribute("required") !== null,
    ).toBe(true);

    const closeBtn = screen.getByTestId("period-lock-close");
    expect(closeBtn).toBeDisabled();

    fireEvent.change(note, { target: { value: "Month-end close" } });
    await waitFor(() => expect(closeBtn).not.toBeDisabled());
  });

  it("shows a freeze preview with date range and entry count", async () => {
    render(
      <PeriodLockCard businessId="1" canOwn t={t} />,
    );
    await waitFor(() => expect(mockListEntries).toHaveBeenCalled());

    const preview = await screen.findByTestId("period-lock-freeze-preview");
    expect(preview.textContent).toMatch(/12/);
    expect(preview.textContent).toMatch(/through/i);
  });

  it("keeps note.trim() validation as a backstop on submit", async () => {
    render(
      <PeriodLockCard businessId="1" canOwn t={t} />,
    );
    await waitFor(() => expect(mockGetPeriodLock).toHaveBeenCalled());

    const note = screen.getByLabelText(/note/i);
    // Whitespace-only still fails the trim() backstop even if the button were
    // force-enabled; type spaces then force-click is covered by the handler.
    fireEvent.change(note, { target: { value: "   " } });
    // Button stays disabled while blank-after-trim.
    expect(screen.getByTestId("period-lock-close")).toBeDisabled();
  });

  // L6-24: empty close date must not reopen the period; close needs confirm.
  it("does not post period lock on close until confirmed, and rejects empty date", async () => {
    mockPostPeriodLock.mockResolvedValue({ locked_through: "2026-08-01" });
    render(<PeriodLockCard businessId="1" canOwn t={t} />);
    await waitFor(() => expect(mockGetPeriodLock).toHaveBeenCalled());

    const note = screen.getByLabelText(/note/i);
    fireEvent.change(note, { target: { value: "Month-end close" } });

    // Empty the close date — must surface dateRequired and never call API.
    const dateInput = screen.getByLabelText(/close through/i);
    fireEvent.change(dateInput, { target: { value: "" } });
    fireEvent.click(screen.getByTestId("period-lock-close"));
    expect(
      await screen.findByText("periodLock.dateRequired"),
    ).toBeInTheDocument();
    expect(mockPostPeriodLock).not.toHaveBeenCalled();

    // Restore a date; close still requires ConfirmationModal before POST.
    fireEvent.change(dateInput, { target: { value: "2026-08-01" } });
    fireEvent.click(screen.getByTestId("period-lock-close"));
    expect(
      await screen.findByText("periodLock.confirmCloseTitle"),
    ).toBeInTheDocument();
    expect(mockPostPeriodLock).not.toHaveBeenCalled();

    fireEvent.click(screen.getByText("periodLock.confirmCloseAction"));
    await waitFor(() =>
      expect(mockPostPeriodLock).toHaveBeenCalledWith("1", {
        locked_through: "2026-08-01",
        note: "Month-end close",
      }),
    );
  });
});
