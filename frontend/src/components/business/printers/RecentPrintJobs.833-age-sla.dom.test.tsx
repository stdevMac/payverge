/**
 * #833: on the live demo venue five kitchen tickets sat `routed` for 21h-74h.
 * The 21h pair rendered danger-red; the 57h/62h/74h trio crossed the 24h
 * "calm" band in elapsedUrgency and rendered NEUTRAL GREY - the three oldest
 * never-printed tickets looked like the healthiest rows in the list.
 *
 * Revert-proof at the call site: drop `job.status` from the
 * printJobAgeChipColor(...) argument list in RecentPrintJobs and the printed
 * job below turns danger; restore the 24h calm band for in-flight jobs and
 * the routed job below turns default.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen } from "@testing-library/react";

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
  getTranslation: (key: string) => key,
}));

const printers = [
  {
    id: 13,
    name: "Demo Kitchen Printer",
    role: "kitchen",
    transport: "browser" as const,
    enabled: true,
    paper_width_mm: 80,
    code_page: "CP858",
    business_id: 86,
    location_id: null,
    cloudprnt_last_seen_at: null,
  },
];

const hoursAgo = (h: number) => new Date(Date.now() - h * 60 * 60_000).toISOString();

const job = (over: Record<string, unknown>) => ({
  business_id: 86,
  printer_id: 13,
  kind: "kitchen",
  source_type: "order",
  source_id: 1,
  payload_html: null,
  language: "en",
  printed_at: null,
  last_error: null,
  ...over,
});

describe("#833 print job age SLA", () => {
  beforeEach(() => jest.clearAllMocks());

  it("keeps a 74h routed ticket loud and lets a 180h printed one go calm", async () => {
    (api.fetchJobs as jest.Mock).mockResolvedValueOnce({
      items: [
        job({ id: 1110, status: "routed", created_at: hoursAgo(74.6) }),
        job({ id: 1107, status: "printed", created_at: hoursAgo(180), printed_at: hoursAgo(180) }),
      ],
    });

    render(<RecentPrintJobs businessId={86} printers={printers} />);

    const routedAge = await screen.findByTestId("job-age-1110");
    expect(routedAge.className).toContain("bg-danger");
    expect(routedAge.className).not.toContain("bg-default");

    const printedAge = screen.getByTestId("job-age-1107");
    expect(printedAge.className).toContain("bg-default");
    expect(printedAge.className).not.toContain("bg-danger");
  });
});
