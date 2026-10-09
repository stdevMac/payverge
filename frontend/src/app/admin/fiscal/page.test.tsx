/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import AdminFiscalPage from "./page";

const mockGetSummary = jest.fn();
const mockGetJobs = jest.fn();
const mockGetReceipts = jest.fn();
const mockRequeue = jest.fn();

jest.mock("@/api/adminFiscal", () => ({
  getAdminFiscalSummary: (...args: unknown[]) => mockGetSummary(...args),
  getAdminFiscalJobs: (...args: unknown[]) => mockGetJobs(...args),
  getAdminFiscalReceipts: (...args: unknown[]) => mockGetReceipts(...args),
  requeueFiscalJob: (...args: unknown[]) => mockRequeue(...args),
  isFiscalJobRequeueable: (status: string) =>
    status === "failed_retryable" || status === "failed_permanent",
}));

jest.mock("next/link", () => {
  return function MockLink({
    children,
    href,
  }: {
    children: React.ReactNode;
    href: string;
  }) {
    return <a href={href}>{children}</a>;
  };
});

beforeEach(() => {
  jest.clearAllMocks();
  mockGetSummary.mockResolvedValue({
    businesses_with_fiscal: 1,
    jobs_by_status: {},
    receipts_by_status: {},
    due_jobs: 0,
    failed_retryable_jobs: 1,
    failed_permanent_jobs: 0,
  });
  mockGetReceipts.mockResolvedValue({ receipts: [], total: 0 });
  mockGetJobs.mockResolvedValue({
    jobs: [
      {
        id: 42,
        business_id: 7,
        business_name: "Real Bistro",
        bill_id: 99,
        action: "issue_receipt",
        status: "failed_retryable",
        attempts: 2,
        max_attempts: 5,
        last_error_message: "AFIP timeout",
        created_at: "2026-07-01T00:00:00Z",
        updated_at: "2026-07-01T01:00:00Z",
      },
      {
        id: 43,
        business_id: 8,
        business_name: "Other Cafe",
        bill_id: 100,
        action: "issue_receipt",
        status: "pending",
        attempts: 0,
        max_attempts: 5,
        created_at: "2026-07-01T00:00:00Z",
        updated_at: "2026-07-01T00:00:00Z",
      },
    ],
    total: 2,
  });
});

describe("AdminFiscalPage — Task 15 operable queue", () => {
  it("renders the business name (not only a raw #id)", async () => {
    render(<AdminFiscalPage />);
    expect(await screen.findByText("Real Bistro")).toBeInTheDocument();
    // Must not be the only presentation of the business as "#7"
    const nameCell = screen.getByTestId("fiscal-job-business-42");
    expect(nameCell).toHaveTextContent("Real Bistro");
    expect(nameCell).not.toHaveTextContent(/^#\d+$/);
  });

  it("shows an enabled Requeue control for failed jobs", async () => {
    render(<AdminFiscalPage />);
    const requeue = await screen.findByTestId("fiscal-job-requeue-42");
    expect(requeue).toBeInTheDocument();
    expect(requeue).toBeEnabled();
    expect(requeue).toHaveTextContent(/Requeue/i);

    // Pending jobs do not get a requeue action.
    expect(screen.queryByTestId("fiscal-job-requeue-43")).not.toBeInTheDocument();
  });

  it("requeues a retryable job plainly and a permanent job with allow_permanent", async () => {
    mockGetJobs.mockResolvedValue({
      jobs: [
        {
          id: 42, business_id: 7, business_name: "Real Bistro", bill_id: 99,
          action: "issue_receipt", status: "failed_retryable", attempts: 2, max_attempts: 5,
          created_at: "2026-07-01T00:00:00Z", updated_at: "2026-07-01T01:00:00Z",
        },
        {
          id: 44, business_id: 7, business_name: "Real Bistro", bill_id: 101,
          action: "issue_receipt", status: "failed_permanent", attempts: 5, max_attempts: 5,
          created_at: "2026-07-01T00:00:00Z", updated_at: "2026-07-01T01:00:00Z",
        },
      ],
      total: 2,
    });
    mockRequeue.mockResolvedValue({});
    render(<AdminFiscalPage />);
    fireEvent.click(await screen.findByTestId("fiscal-job-requeue-42"));
    await waitFor(() => expect(mockRequeue).toHaveBeenCalledWith(42, false));
    fireEvent.click(await screen.findByTestId("fiscal-job-requeue-44"));
    await waitFor(() => expect(mockRequeue).toHaveBeenCalledWith(44, true));
  });

  it("loads jobs with the default kind=real scope (no kind=all)", async () => {
    render(<AdminFiscalPage />);
    await waitFor(() => expect(mockGetJobs).toHaveBeenCalled());
    const call = mockGetJobs.mock.calls[0]?.[0] ?? {};
    // Default list is production queue — never request kind=all / demo.
    expect(call.kind).not.toBe("all");
    expect(call.kind).not.toBe("demo");
    expect(call.kind).not.toBe("test");
  });
});
