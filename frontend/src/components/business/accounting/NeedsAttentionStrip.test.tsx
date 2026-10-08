/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import type { AccountingSummary } from "@/api/accounting";
import {
  usePayrollRuns,
  useReceipts,
  useSummary,
} from "@/hooks/accounting/useAccountingQueries";
import NeedsAttentionStrip from "./NeedsAttentionStrip";

jest.mock("@/hooks/accounting/useAccountingQueries", () => ({
  useSummary: jest.fn(),
  usePayrollRuns: jest.fn(),
  useReceipts: jest.fn(),
}));

jest.mock("../premium", () => ({
  PremiumPanel: ({
    children,
    tone,
    className,
    // Strip non-DOM PremiumPanel props from the mock host element.
    withTexture: _withTexture,
    interactive: _interactive,
    ...rest
  }: {
    children: React.ReactNode;
    tone?: string;
    className?: string;
    withTexture?: boolean;
    interactive?: boolean;
  }) => {
    void _withTexture;
    void _interactive;
    return (
      <div
        data-testid="premium-panel"
        data-tone={tone}
        className={className}
        {...rest}
      >
        {children}
      </div>
    );
  },
}));

const useSummaryMock = useSummary as jest.Mock;
const usePayrollRunsMock = usePayrollRuns as jest.Mock;
const useReceiptsMock = useReceipts as jest.Mock;

/** Templates mirror i18n so tWith can interpolate in unit tests. */
const T_WITH_TEMPLATES: Record<string, string> = {
  "attention.draftRuns": "{count} draft payroll runs waiting",
  "attention.failedDeliveries": "{count} invoices need attention",
  "attention.collectionGap": "{amount} billed but not collected",
};

const t = (key: string) => key;
const tWith = (key: string, replacements: Record<string, string | number>) => {
  let value = T_WITH_TEMPLATES[key] ?? key;
  Object.entries(replacements).forEach(([name, replacement]) => {
    value = value.replace(
      new RegExp(`\\{${name}\\}`, "g"),
      String(replacement),
    );
  });
  return value;
};
const fmtMoney = (value: number, currency: string) =>
  `${currency} ${value.toFixed(2)}`;

function summary(
  overrides: Partial<AccountingSummary> = {},
): AccountingSummary {
  return {
    start_date: "2026-01-01",
    end_date: "2026-01-31",
    currency: "USD",
    auto_income_total: 0 as AccountingSummary["auto_income_total"],
    manual_income_total: 0 as AccountingSummary["manual_income_total"],
    expense_total: 0 as AccountingSummary["expense_total"],
    payroll_total: 0 as AccountingSummary["payroll_total"],
    billed_total: 0 as AccountingSummary["billed_total"],
    collected_total: 0 as AccountingSummary["collected_total"],
    collection_gap: 0 as AccountingSummary["collection_gap"],
    income_breakdown: [],
    expense_breakdown: [],
    payroll_summary: {
      paid_runs: 0,
      total_gross: 0 as AccountingSummary["payroll_total"],
      total_bonus: 0 as AccountingSummary["payroll_total"],
      total_deduction: 0 as AccountingSummary["payroll_total"],
      total_net: 0 as AccountingSummary["payroll_total"],
    },
    ...overrides,
  };
}

function renderStrip(
  props: Partial<React.ComponentProps<typeof NeedsAttentionStrip>> = {},
) {
  return render(
    <NeedsAttentionStrip
      businessId="42"
      start="2026-01-01"
      end="2026-01-31"
      t={t}
      tWith={tWith}
      fmtMoney={fmtMoney}
      {...props}
    />,
  );
}

describe("NeedsAttentionStrip", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    useSummaryMock.mockReturnValue({
      data: summary(),
      isLoading: false,
      isError: false,
    });
    usePayrollRunsMock.mockReturnValue({
      data: { runs: [], total: 0, page: 1, page_size: 1, total_pages: 0 },
      isLoading: false,
    });
    useReceiptsMock.mockReturnValue({
      data: { receipts: [], total: 0, page: 1, page_size: 1, total_pages: 0 },
      isLoading: false,
    });
  });

  it("renders nothing when all signals are clear", () => {
    const { container } = renderStrip();
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByTestId("needs-attention-strip")).toBeNull();
  });

  it("probes draft payrolls, needs-attention invoices, and summary collection gap", () => {
    renderStrip();

    expect(useSummaryMock).toHaveBeenCalledWith("42", {
      start: "2026-01-01",
      end: "2026-01-31",
    });
    // Probes must carry the page's date range: unscoped probes counted
    // all-time drafts/attention against a period-scoped Overview.
    expect(usePayrollRunsMock).toHaveBeenCalledWith("42", {
      start: "2026-01-01",
      end: "2026-01-31",
      status: "draft",
      page: 1,
      page_size: 1,
    });
    expect(useReceiptsMock).toHaveBeenCalledWith("42", {
      start: "2026-01-01",
      end: "2026-01-31",
      needs_attention: true,
      page: 1,
      page_size: 1,
    });
  });

  it("renders a draft-payrolls row with count and Review deep-link", () => {
    usePayrollRunsMock.mockReturnValue({
      data: { runs: [], total: 3, page: 1, page_size: 1, total_pages: 3 },
      isLoading: false,
    });

    renderStrip();

    expect(screen.getByTestId("needs-attention-strip")).toBeInTheDocument();
    expect(screen.getByText("attention.title")).toBeInTheDocument();
    expect(screen.getByTestId("attention-draft-runs")).toHaveTextContent(
      "3 draft payroll runs waiting",
    );

    const link = screen.getByTestId("attention-draft-runs-review");
    expect(link).toHaveAttribute(
      "href",
      expect.stringContaining("sub=payroll"),
    );
    expect(link).toHaveAttribute(
      "href",
      expect.stringContaining("status=draft"),
    );
    expect(link).toHaveTextContent("attention.action.review");
  });

  it("renders a failed-invoices row with needs_attention deep-link", () => {
    useReceiptsMock.mockReturnValue({
      data: { receipts: [], total: 2, page: 1, page_size: 1, total_pages: 2 },
      isLoading: false,
    });

    renderStrip();

    expect(screen.getByTestId("attention-failed-deliveries")).toHaveTextContent(
      "2 invoices need attention",
    );

    const link = screen.getByTestId("attention-failed-deliveries-review");
    expect(link).toHaveAttribute(
      "href",
      expect.stringContaining("sub=invoices"),
    );
    expect(link).toHaveAttribute(
      "href",
      expect.stringContaining("filter=needs_attention"),
    );
  });

  it("renders a collection-gap row when gap > 0 with outstanding deep-link", () => {
    useSummaryMock.mockReturnValue({
      data: summary({
        collection_gap: 150 as AccountingSummary["collection_gap"],
        currency: "USD",
      }),
      isLoading: false,
      isError: false,
    });

    renderStrip();

    const row = screen.getByTestId("attention-collection-gap");
    expect(row).toHaveTextContent("USD 150.00 billed but not collected");

    const link = screen.getByTestId("attention-collection-gap-review");
    expect(link).toHaveAttribute(
      "href",
      expect.stringContaining("sub=outstanding"),
    );
  });

  it("renders every present signal together", () => {
    useSummaryMock.mockReturnValue({
      data: summary({
        collection_gap: 50 as AccountingSummary["collection_gap"],
      }),
      isLoading: false,
    });
    usePayrollRunsMock.mockReturnValue({
      data: { runs: [], total: 1, page: 1, page_size: 1, total_pages: 1 },
      isLoading: false,
    });
    useReceiptsMock.mockReturnValue({
      data: { receipts: [], total: 4, page: 1, page_size: 1, total_pages: 4 },
      isLoading: false,
    });

    renderStrip();

    expect(screen.getByTestId("attention-draft-runs")).toBeInTheDocument();
    expect(
      screen.getByTestId("attention-failed-deliveries"),
    ).toBeInTheDocument();
    expect(screen.getByTestId("attention-collection-gap")).toBeInTheDocument();
    expect(screen.getAllByText("attention.action.review")).toHaveLength(3);
  });

  it("uses amber/urgent panel tone when signals exist", () => {
    usePayrollRunsMock.mockReturnValue({
      data: { runs: [], total: 1, page: 1, page_size: 1, total_pages: 1 },
      isLoading: false,
    });

    renderStrip();

    const panel = screen.getByTestId("needs-attention-strip");
    expect(panel).toHaveAttribute("data-tone", "urgent");
  });
});
