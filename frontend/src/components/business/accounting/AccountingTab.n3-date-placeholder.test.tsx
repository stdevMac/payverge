/** @jest-environment jsdom */
/**
 * Accounting date controls match Bills (NextUI DatePicker) — no raw
 * "Format: mm/dd/yyyy" helper text under the fields.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SimpleTranslationProvider } from "@/i18n/SimpleTranslationProvider";
import { AccountingTab } from "./AccountingTab";

jest.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(),
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    isSuspended: false,
    loading: false,
  }),
}));

jest.mock("@/hooks/accounting/useAccountingQueries", () => ({
  accountingKeys: {
    summary: () => ["summary"],
  },
  useSummary: () => ({
    data: null,
    isLoading: false,
    isError: false,
  }),
}));

jest.mock("@/api/accounting", () => ({
  accountingApi: {
    listEntries: jest.fn().mockResolvedValue({ entries: [], total: 0 }),
    listPayrollRuns: jest.fn().mockResolvedValue({ runs: [], total: 0 }),
    listPayrollRunsPage: jest.fn().mockResolvedValue({ runs: [], total: 0 }),
    listEntriesPage: jest.fn().mockResolvedValue({ entries: [], total: 0 }),
  },
}));

jest.mock("@/api/fiscal", () => ({
  fiscalApi: {
    listReceiptsPage: jest.fn().mockResolvedValue({ receipts: [], total: 0 }),
  },
}));

// Heavy sub-tabs are out of scope for the date-filter chrome assertion.
jest.mock("./OverviewTab", () => () => null);
jest.mock("./EntriesTab", () => () => null);
jest.mock("./PayrollTab", () => () => null);
jest.mock("./InvoicesTab", () => () => null);
jest.mock("./ReportsTab", () => () => null);
jest.mock("./OutstandingTab", () => () => null);

function renderAccounting(locale: "en" | "es" = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <SimpleTranslationProvider initialLocale={locale}>
        <AccountingTab businessId="1" />
      </SimpleTranslationProvider>
    </QueryClientProvider>,
  );
}

describe("AccountingTab date filters", () => {
  it("uses DatePicker controls without Format: mm/dd/yyyy helper text", async () => {
    renderAccounting("en");
    // NextUI DatePicker exposes a group with aria-label (not a native input).
    expect(
      await screen.findByRole("group", { name: /start date/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("group", { name: /end date/i }),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Format:\s*mm\/dd\/yyyy/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Formato:\s*dd\/mm\/yyyy/i)).not.toBeInTheDocument();
    expect(document.querySelector('input[type="date"]')).toBeNull();
  });

  it("keeps Spanish labels without exposing raw ISO format helpers", async () => {
    renderAccounting("es");
    expect(
      await screen.findByRole("group", { name: /fecha de inicio/i }),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Formato:/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/mm\/dd\/yyyy/i)).not.toBeInTheDocument();
    expect(document.querySelector('input[type="date"]')).toBeNull();
  });
});
