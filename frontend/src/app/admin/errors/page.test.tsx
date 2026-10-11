/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import ErrorLogsPage from "./page";
import type { ErrorLog } from "@/api/errorLogs";

const mockGetErrors = jest.fn();

jest.mock("@/api/errorLogs", () => ({
  errorLogsAPI: {
    getErrors: (...args: unknown[]) => mockGetErrors(...args),
  },
}));

function makeError(id: number, msg: string): ErrorLog {
  return {
    id,
    message: msg,
    source: "backend",
    component: "payments",
    function: "Charge",
    timestamp: "2025-05-01T10:00:00Z",
    created_at: "2025-05-01T10:00:00Z",
    stack: `stack-${id}`,
  } as ErrorLog;
}

// 25 rows -> 2 pages at PAGE_SIZE=20
const PAGE1 = Array.from({ length: 20 }, (_, i) => makeError(i + 1, `err ${i + 1}`));
const PAGE2 = Array.from({ length: 5 }, (_, i) => makeError(i + 21, `err ${i + 21}`));

beforeEach(() => {
  jest.clearAllMocks();
  // Make scrollIntoView a no-op (jsdom lacks layout).
  window.HTMLElement.prototype.scrollIntoView = jest.fn();
  mockGetErrors.mockImplementation(
    async ({ offset }: { offset: number }) => ({
      errors: offset === 0 ? PAGE1 : PAGE2,
      total: 25,
    }),
  );
});

describe("ErrorLogsPage — selection UX", () => {
  it("scrolls the detail panel into view when an error is selected", async () => {
    const scrollSpy = window.HTMLElement.prototype
      .scrollIntoView as jest.Mock;
    render(<ErrorLogsPage />);

    const row = await screen.findByText("err 1");
    fireEvent.click(row);

    await waitFor(() => {
      expect(screen.getByText("stack-1")).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(scrollSpy).toHaveBeenCalled();
    });
  });

  it("clears the selection when the page changes so the detail can't silently desync", async () => {
    render(<ErrorLogsPage />);

    // Select a row on page 1.
    const row = await screen.findByText("err 1");
    fireEvent.click(row);

    await waitFor(() => {
      expect(screen.getByText("stack-1")).toBeInTheDocument();
    });

    // Go to page 2.
    const next = screen.getByRole("button", { name: /next/i });
    fireEvent.click(next);

    await waitFor(() => {
      expect(screen.getByText("err 21")).toBeInTheDocument();
    });

    // Selection cleared: no detail panel AND no lingering row-selection.
    expect(screen.queryByText("stack-1")).not.toBeInTheDocument();
    const selectedRows = document.querySelectorAll(
      'tr[data-selected="true"]',
    );
    expect(selectedRows.length).toBe(0);
  });
});
