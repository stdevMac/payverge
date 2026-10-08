/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import * as api from "@/api/print";

import RecentPrintJobs from "./RecentPrintJobs";

jest.mock("@/api/print", () => {
  const actual = jest.requireActual("@/api/print");
  return {
    ...actual,
    fetchJobs: jest.fn(),
    cancelJob: jest.fn(),
    reprintJob: jest.fn(),
    rerouteJob: jest.fn(),
  };
});

const mockToastSuccess = jest.fn();
const mockToastError = jest.fn();
jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: {
    success: (...args: unknown[]) => mockToastSuccess(...args),
    error: (...args: unknown[]) => mockToastError(...args),
  },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (
    key: string,
    _locale?: string,
    params?: Record<string, string | number>,
  ) => {
    const map: Record<string, string> = {
      "printers.jobs.title": "Recent jobs",
      "printers.jobs.subtitle": "Print queue",
      "printers.jobs.refresh": "Refresh",
      "printers.jobs.loading": "Loading…",
      "printers.jobs.emptyTitle": "No print jobs yet",
      "printers.jobs.emptyBody": "Jobs show up here.",
      "printers.jobs.jobLabel": "Job #{id} · {kind}",
      "printers.jobs.jobLabelShort": "#{id} · {kind}",
      "printers.jobs.tableWord": "Table",
      "printers.jobs.counterWord": "Counter",
      "printers.jobs.stalledQueued": "Stalled — check print station",
      "printers.jobs.stationOfflineBanner":
        "Queued jobs are waiting for {printers} but no print station is active on this browser.",
      "printers.jobs.billLabel": "Bill {number}",
      "printers.jobs.billIdLabel": "Bill #{id}",
      "printers.jobs.orderLabel": "Order #{id}",
      "printers.jobs.unknownPrinter": "Unknown printer",
      "printers.jobs.unassignedPrinter": "No printer assigned",
      "printers.jobs.needsPrinter": "Needs printer",
      "printers.jobs.unassignedRecoveryHint": "Assign this job.",
      "printers.jobs.unassignedAddPrinterHint": "Add a printer.",
      "printers.jobs.assignPrinter": "Assign printer",
      "printers.jobs.addPrinterCta": "Add printer",
      "printers.jobs.cancel": "Cancel job",
      "printers.jobs.reprint": "Reprint",
      "printers.jobs.reprintSuccess": "Reprint queued",
      "printers.jobs.reprintFailed": "Could not reprint",
      "printers.jobs.rerouteSuccess": "Printer assigned",
      "printers.jobs.rerouteFailed": "Could not assign a printer",
      "printers.jobs.cancelSuccess": "Print job cancelled",
      "printers.jobs.cancelConfirmTitle": "Cancel print job?",
      "printers.jobs.cancelConfirmBody": "Cancel job #{id}?",
      "printers.jobs.filterStatus": "Status",
      "printers.jobs.filterKind": "Kind",
      "printers.jobs.filters.allStatuses": "All statuses",
      "printers.jobs.filters.allKinds": "All kinds",
      "printers.jobs.filters.unassigned": "Unassigned",
      "printers.jobs.unknownDay": "Unknown date",
      "printers.jobs.previous": "Previous",
      "printers.jobs.next": "Next",
      "printers.jobs.pageRange": "Showing {from}–{to} of {total}",
      "printers.jobs.pageOf": "Page {page} of {pages}",
      "printers.loadError": "Could not load: {error}",
      "printers.status.failed": "Failed",
      "printers.status.queued": "Queued",
      "printers.status.printed": "Printed",
      "printers.status.pending": "Pending",
      "printers.kindLabels.bill": "Bill",
      "printers.kindLabels.receipt": "Receipt",
      "printers.kindLabels.kitchen": "Kitchen",
      "printers.kindLabels.bar": "Bar",
    };
    let out = map[key] ?? key;
    if (params) {
      Object.entries(params).forEach(([k, v]) => {
        out = out.replace(new RegExp(`\\{${k}\\}`, "g"), String(v));
      });
    }
    return out;
  },
}));

const printers = [
  {
    id: 1,
    name: "Kitchen",
    role: "kitchen",
    transport: "browser" as const,
    enabled: true,
    paper_width_mm: 80,
    code_page: "CP858",
    business_id: 42,
    location_id: null,
    cloudprnt_last_seen_at: null,
  },
  {
    id: 2,
    name: "Front",
    role: "bill",
    transport: "browser" as const,
    enabled: true,
    paper_width_mm: 80,
    code_page: "CP858",
    business_id: 42,
    location_id: null,
    cloudprnt_last_seen_at: null,
  },
];

describe("RecentPrintJobs", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("renders recent jobs with status, printer, bill/table, and error", async () => {
    (api.fetchJobs as jest.Mock).mockResolvedValueOnce({
      items: [
        {
          id: 99,
          business_id: 42,
          printer_id: 1,
          kind: "bill",
          source_type: "bill",
          source_id: 10,
          status: "failed",
          payload_html: null,
          language: "en",
          created_at: new Date().toISOString(),
          printed_at: null,
          last_error: "printer offline",
          bill_number: "B-10",
          table_name: "T4",
        },
      ],
      total: 1,
    });

    render(<RecentPrintJobs businessId={42} printers={printers} />);
    expect(
      // O7: custom names keep their stored identity, no type-word prefix.
      await screen.findByText(/#99 · Bill · T4 · Bill B-10/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Kitchen", { selector: "span" }),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Failed").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("printer offline")).toBeInTheDocument();
  });

  it("cancels a non-terminal job after confirm", async () => {
    (api.fetchJobs as jest.Mock).mockResolvedValue({
      items: [
        {
          id: 11,
          business_id: 42,
          printer_id: 1,
          kind: "bill",
          source_type: "bill",
          source_id: 1,
          status: "routed",
          payload_html: null,
          language: "en",
          created_at: new Date().toISOString(),
          printed_at: null,
        },
      ],
      total: 1,
    });
    (api.cancelJob as jest.Mock).mockResolvedValueOnce(undefined);

    render(<RecentPrintJobs businessId={42} printers={printers} />);
    fireEvent.click(await screen.findByRole("button", { name: /cancel job/i }));
    fireEvent.click(
      await screen.findByRole("button", { name: /^cancel job$/i }),
    );

    await waitFor(() => expect(api.cancelJob).toHaveBeenCalledWith(42, 11));
  });

  it("reprints bill jobs and shows a confirmation toast", async () => {
    (api.fetchJobs as jest.Mock).mockResolvedValue({
      items: [
        {
          id: 22,
          business_id: 42,
          printer_id: 1,
          kind: "bill",
          source_type: "bill",
          source_id: 2,
          status: "printed",
          payload_html: null,
          language: "en",
          created_at: new Date().toISOString(),
          printed_at: new Date().toISOString(),
        },
      ],
      total: 1,
    });
    (api.reprintJob as jest.Mock).mockResolvedValueOnce({ id: 23 });

    render(<RecentPrintJobs businessId={42} printers={printers} />);
    fireEvent.click(await screen.findByRole("button", { name: /reprint/i }));
    await waitFor(() => expect(api.reprintJob).toHaveBeenCalledWith(42, 22));
    expect(mockToastSuccess).toHaveBeenCalledWith("Reprint queued");
  });

  it("offers assign-printer recovery for unassigned pending jobs", async () => {
    (api.fetchJobs as jest.Mock).mockResolvedValue({
      items: [
        {
          id: 44,
          business_id: 42,
          printer_id: null,
          kind: "receipt",
          source_type: "bill",
          source_id: 9,
          status: "pending",
          payload_html: null,
          language: "en",
          created_at: new Date(Date.now() - 4 * 60 * 1000).toISOString(),
          printed_at: null,
        },
      ],
      total: 1,
    });
    (api.rerouteJob as jest.Mock).mockResolvedValueOnce({
      id: 44,
      printer_id: 2,
      status: "routed",
    });

    render(<RecentPrintJobs businessId={42} printers={printers} />);
    expect(await screen.findByText("Needs printer")).toBeInTheDocument();
    fireEvent.click(
      await screen.findByRole("button", { name: /assign printer/i }),
    );
    await waitFor(() => expect(api.rerouteJob).toHaveBeenCalledWith(42, 44));
    expect(mockToastSuccess).toHaveBeenCalledWith("Printer assigned");
  });

  it("offers reprint only for the terminally failed receipt", async () => {
    (api.fetchJobs as jest.Mock).mockResolvedValue({
      items: [
        {
          id: 31,
          business_id: 42,
          printer_id: 1,
          kind: "receipt",
          source_type: "bill",
          source_id: 3,
          status: "routed",
          payload_html: null,
          language: "en",
          created_at: new Date().toISOString(),
          printed_at: null,
        },
        {
          id: 32,
          business_id: 42,
          printer_id: 1,
          kind: "receipt",
          source_type: "bill",
          source_id: 4,
          status: "failed",
          payload_html: null,
          language: "en",
          created_at: new Date().toISOString(),
          printed_at: null,
          last_error: "station offline",
        },
      ],
      total: 2,
    });
    (api.reprintJob as jest.Mock).mockResolvedValueOnce({ id: 33 });

    render(<RecentPrintJobs businessId={42} printers={printers} />);

    expect(await screen.findByText("Queued")).toBeInTheDocument();
    expect(screen.getAllByText("Failed").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("station offline")).toBeInTheDocument();
    const reprintButtons = screen.getAllByRole("button", { name: /reprint/i });
    expect(reprintButtons).toHaveLength(1);
    fireEvent.click(reprintButtons[0]);
    await waitFor(() => expect(api.reprintJob).toHaveBeenCalledWith(42, 32));
  });

  it("does not double the word Table for seed table names (#825)", async () => {
    (api.fetchJobs as jest.Mock).mockResolvedValueOnce({
      items: [
        {
          id: 1115,
          business_id: 42,
          printer_id: 1,
          kind: "kitchen",
          source_type: "order",
          source_id: 7,
          status: "routed",
          payload_html: null,
          language: "en",
          created_at: new Date().toISOString(),
          printed_at: null,
          table_name: "Table 1",
        },
      ],
      total: 1,
    });

    render(<RecentPrintJobs businessId={42} printers={printers} />);
    expect(
      await screen.findByText(/#1115 · Kitchen · Table 1/),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Table Table/)).not.toBeInTheDocument();
  });

  it("keeps counter seeds as Counter and custom names raw (#825 / O7)", async () => {
    (api.fetchJobs as jest.Mock).mockResolvedValueOnce({
      items: [
        {
          id: 1116,
          business_id: 42,
          printer_id: 1,
          kind: "kitchen",
          source_type: "order",
          source_id: 8,
          status: "routed",
          payload_html: null,
          language: "en",
          created_at: new Date().toISOString(),
          printed_at: null,
          table_name: "Counter 3",
        },
        {
          id: 1117,
          business_id: 42,
          printer_id: 1,
          kind: "kitchen",
          source_type: "order",
          source_id: 9,
          status: "routed",
          payload_html: null,
          language: "en",
          created_at: new Date().toISOString(),
          printed_at: null,
          table_name: "Patio A",
        },
      ],
      total: 2,
    });

    render(<RecentPrintJobs businessId={42} printers={printers} />);
    expect(
      await screen.findByText(/#1116 · Kitchen · Counter 3/),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Table 3/)).not.toBeInTheDocument();
    // Custom names keep their stored identity, without a type-word prefix.
    expect(screen.getByText(/#1117 · Kitchen · Patio A/)).toBeInTheDocument();
    expect(screen.queryByText(/Table Patio A/)).not.toBeInTheDocument();
  });

  it("flags stale queued browser jobs and warns the station is not armed here (#833)", async () => {
    const thirtyThreeMinAgo = new Date(Date.now() - 33 * 60000).toISOString();
    (api.fetchJobs as jest.Mock).mockResolvedValueOnce({
      items: [
        {
          id: 1114,
          business_id: 42,
          printer_id: 1,
          kind: "kitchen",
          source_type: "order",
          source_id: 8,
          status: "routed",
          payload_html: null,
          language: "en",
          created_at: thirtyThreeMinAgo,
          printed_at: null,
          table_name: "Table 1",
        },
      ],
      total: 1,
    });

    render(<RecentPrintJobs businessId={42} printers={printers} />);
    expect(
      await screen.findByText("Stalled — check print station"),
    ).toBeInTheDocument();
    // No station armed in this browser → operator is told explicitly.
    expect(
      screen.getByText(/no print station is active on this browser/i),
    ).toBeInTheDocument();
  });

  it("keeps fresh queued jobs quiet and trusts an armed station (#833)", async () => {
    window.localStorage.setItem("payverge_print_station:42", "1");
    const fortyMinAgo = new Date(Date.now() - 40 * 60000).toISOString();
    (api.fetchJobs as jest.Mock).mockResolvedValueOnce({
      items: [
        {
          id: 5,
          business_id: 42,
          printer_id: 1,
          kind: "kitchen",
          source_type: "order",
          source_id: 9,
          status: "routed",
          payload_html: null,
          language: "en",
          created_at: new Date().toISOString(),
          printed_at: null,
        },
        {
          id: 6,
          business_id: 42,
          printer_id: 1,
          kind: "kitchen",
          source_type: "order",
          source_id: 10,
          status: "routed",
          payload_html: null,
          language: "en",
          created_at: fortyMinAgo,
          printed_at: null,
        },
      ],
      total: 2,
    });

    try {
      render(<RecentPrintJobs businessId={42} printers={printers} />);
      // Stale job still gets its chip even with the station armed here…
      expect(
        await screen.findByText("Stalled — check print station"),
      ).toBeInTheDocument();
      // …but the offline banner stays away: this browser IS the station.
      expect(
        screen.queryByText(/no print station is active on this browser/i),
      ).not.toBeInTheDocument();
    } finally {
      window.localStorage.removeItem("payverge_print_station:42");
    }
  });
});
