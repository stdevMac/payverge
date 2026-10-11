/**
 * D1 / L3-45: RecentPrintJobs age chips must use humanizeDurationMinutes
 * (e.g. "2d"), never raw multi-day hour walls like "48h".
 *
 * Pure printJobAgeLabel unit tests pass even if the list never calls them.
 * This mounts RecentPrintJobs with a 2-day-old job and asserts the chip DOM.
 *
 * Revert-proof: return `${minutes}h` from printJobAgeLabel → chip shows "2880h"
 * (or similar) and this fails.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

import * as api from "@/api/print";
import RecentPrintJobs from "./RecentPrintJobs";

jest.mock("@/api/print", () => {
  const actual = jest.requireActual("@/api/print");
  return {
    ...actual,
    fetchJobs: jest.fn(),
    cancelJob: jest.fn(),
    reprintJob: jest.fn(),
  };
});

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (
    key: string,
    _locale?: string,
    params?: Record<string, string | number>,
  ) => {
    const map: Record<string, string> = {
      "printers.jobs.title": "Recent jobs",
      "printers.jobs.subtitle": "Last 50 jobs",
      "printers.jobs.refresh": "Refresh",
      "printers.jobs.loading": "Loading…",
      "printers.jobs.emptyTitle": "No print jobs yet",
      "printers.jobs.emptyBody": "Jobs show up here.",
      "printers.jobs.jobLabel": "Job #{id} · {kind}",
      "printers.jobs.unknownPrinter": "Unknown printer",
      "printers.jobs.unassignedPrinter": "Unassigned",
      "printers.jobs.cancel": "Cancel job",
      "printers.jobs.reprint": "Reprint",
      "printers.jobs.cancelConfirmTitle": "Cancel print job?",
      "printers.jobs.cancelConfirmBody": "Cancel job #{id}?",
      "printers.loadError": "Could not load: {error}",
      "printers.status.failed": "Failed",
      "printers.status.queued": "Queued",
      "printers.status.printed": "Printed",
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
];

describe("RecentPrintJobs L3-45 age chip (DOM)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("renders multi-day ages as humanized days, never raw hour walls", async () => {
    const twoDaysAgo = new Date(Date.now() - 2 * 24 * 60 * 60_000).toISOString();
    (api.fetchJobs as jest.Mock).mockResolvedValueOnce({
      items: [
        {
          id: 77,
          business_id: 42,
          printer_id: 1,
          kind: "bill",
          source_type: "bill",
          source_id: 10,
          status: "failed",
          payload_html: null,
          language: "en",
          created_at: twoDaysAgo,
          printed_at: null,
          last_error: null,
        },
      ],
    });

    render(<RecentPrintJobs businessId={42} printers={printers} />);

    const ageChip = await screen.findByTestId("job-age-77");
    await waitFor(() => {
      expect(ageChip.textContent).toMatch(/2d/);
    });
    // Raw multi-day hour walls (audit N-6/N-7 class) must not appear.
    expect(ageChip.textContent).not.toMatch(/\d{2,}h/);
    expect(ageChip.textContent).not.toMatch(/2880|48h|2880h/);
  });
});
