/** @jest-environment jsdom */
import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { accountingApi, type ManualLedgerEntry } from "@/api/accounting";
import { useEntries } from "@/hooks/accounting/useAccountingQueries";
import EntriesTab from "./EntriesTab";

jest.mock("@/hooks/accounting/useAccountingQueries", () => ({
  useEntries: jest.fn(),
  useVoidEntry: jest.fn(() => ({ mutate: jest.fn(), isPending: false })),
  useCreateEntry: jest.fn(() => ({ mutate: jest.fn(), isPending: false })),
}));

const mockListAccountingCategories = jest.fn().mockResolvedValue({
  defaults: [],
  custom: [],
});

jest.mock("@/api/accounting", () => ({
  accountingApi: {
    entriesExportUrl: jest.fn(
      () => "https://api.test/entries/export.csv?start=2026-01-01&end=2026-01-31",
    ),
    listRecurringTemplates: jest.fn().mockResolvedValue([]),
    listAccountingCategories: (...a: unknown[]) =>
      mockListAccountingCategories(...a),
  },
}));

const useEntriesMock = useEntries as jest.Mock;

const t = (key: string) => key;

function entry(
  overrides: Partial<ManualLedgerEntry> & { id: number },
): ManualLedgerEntry {
  return {
    business_id: 42,
    entry_type: "expense",
    category: "inventory",
    amount: 40 as ManualLedgerEntry["amount"],
    currency: "USD",
    occurred_at: "2026-01-15T12:00:00Z",
    description: "Inventory restock",
    voided_at: null,
    ...overrides,
  };
}

function mockPage(
  entries: ManualLedgerEntry[],
  overrides: { total?: number; page?: number; page_size?: number } = {},
) {
  useEntriesMock.mockReturnValue({
    data: {
      entries,
      total: overrides.total ?? entries.length,
      page: overrides.page ?? 1,
      page_size: overrides.page_size ?? 20,
      total_pages: Math.max(
        1,
        Math.ceil((overrides.total ?? entries.length) / (overrides.page_size ?? 20)),
      ),
    },
    isLoading: false,
    isFetching: false,
    isSuccess: true,
  });
}

function renderTab(
  props: Partial<React.ComponentProps<typeof EntriesTab>> = {},
) {
  return render(
    <EntriesTab
      businessId="42"
      start="2026-01-01"
      end="2026-01-31"
      locale="en"
      currency="USD"
      canWrite
      t={t}
      {...props}
    />,
  );
}

describe("EntriesTab", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.useFakeTimers();
    mockPage([
      entry({ id: 1, description: "Inventory restock" }),
      entry({
        id: 2,
        description: "Voided rent",
        category: "rent",
        voided_at: "2026-01-20T12:00:00Z",
      }),
    ]);
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it("renders entry rows from useEntries", () => {
    renderTab();
    expect(screen.getByText("Inventory restock")).toBeInTheDocument();
    expect(screen.getByText("Voided rent")).toBeInTheDocument();
    expect(useEntriesMock).toHaveBeenCalled();
  });

  // L6-14: category filter must include custom categories, not only hardcoded defaults.
  it("includes active custom categories in the category filter options", async () => {
    jest.useRealTimers(); // category list is an async effect; fake timers starve it.
    mockListAccountingCategories.mockResolvedValue({
      defaults: [{ key: "rent", label: "Rent", entry_type: "expense", source: "default" }],
      custom: [
        {
          id: 9,
          key: "wine_club",
          label: "Wine club dues",
          entry_type: "income",
          source: "custom",
          active: true,
        },
      ],
    });
    renderTab();
    await waitFor(() =>
      expect(mockListAccountingCategories).toHaveBeenCalledWith("42"),
    );

    // Open category filter via the stable test id (NextUI mirrors a hidden select).
    fireEvent.click(screen.getByTestId("entries-category-filter"));
    expect(
      await screen.findByRole("option", { name: /Wine club dues/i }),
    ).toBeInTheDocument();
  });

  it("applies line-through to voided description cells", () => {
    renderTab();
    const voided = screen.getByText("Voided rent");
    expect(voided.className).toMatch(/line-through/);
    const active = screen.getByText("Inventory restock");
    expect(active.className).not.toMatch(/line-through/);
  });

  it("refires useEntries with status voided when status filter changes", async () => {
    renderTab();

    // Prefer the visible trigger (data-testid) — NextUI also renders a hidden
    // native <select> with the same label.
    const statusTrigger = screen.getByTestId("entries-status-filter");
    fireEvent.click(statusTrigger);

    const voidedOption = await screen.findByRole("option", {
      name: "entries.filters.statusVoided",
    });
    fireEvent.click(voidedOption);

    await waitFor(() => {
      const lastCall = useEntriesMock.mock.calls.at(-1);
      expect(lastCall?.[1]).toEqual(
        expect.objectContaining({ status: "voided" }),
      );
    });
  });

  it("wires pagination onPageChange into the next useEntries page", async () => {
    mockPage(
      [entry({ id: 1, description: "Page one row" })],
      { total: 45, page: 1, page_size: 20 },
    );
    renderTab();

    expect(screen.getByText("Page one row")).toBeInTheDocument();

    mockPage(
      [entry({ id: 21, description: "Page two row" })],
      { total: 45, page: 2, page_size: 20 },
    );

    const nextBtn = screen.getByRole("button", { name: "Next page" });
    fireEvent.click(nextBtn);

    await waitFor(() => {
      const lastCall = useEntriesMock.mock.calls.at(-1);
      expect(lastCall?.[1]).toEqual(expect.objectContaining({ page: 2 }));
    });
  });

  it("debounces search 250ms into the q param", async () => {
    renderTab();

    const search = screen.getByLabelText("entries.filters.searchPlaceholder");
    fireEvent.change(search, { target: { value: "flour" } });

    // Still within debounce window — q must not be applied yet.
    act(() => {
      jest.advanceTimersByTime(200);
    });
    const midCalls = useEntriesMock.mock.calls.map((c) => c[1] as { q?: string });
    expect(midCalls.every((p) => !p.q || p.q !== "flour")).toBe(true);

    act(() => {
      jest.advanceTimersByTime(100);
    });

    await waitFor(() => {
      const lastCall = useEntriesMock.mock.calls.at(-1);
      expect(lastCall?.[1]).toEqual(expect.objectContaining({ q: "flour" }));
    });
  });

  it("hides New entry when canWrite is false", () => {
    renderTab({ canWrite: false });
    expect(
      screen.queryByRole("button", { name: /entries\.newEntry/i }),
    ).not.toBeInTheDocument();
  });

  it("shows Export CSV as a link using entriesExportUrl", () => {
    renderTab();
    const link = screen.getByRole("link", { name: /entries\.exportCsv/i });
    expect(link).toHaveAttribute(
      "href",
      expect.stringContaining("export.csv"),
    );
  });

  // The streamed CSV localizes its headers server-side. Without an explicit
  // lang the backend falls back to Accept-Language, so an es browser on an
  // English dashboard downloads a Spanish CSV. The tab must send the operator
  // locale it is already rendering with.
  it("passes the operator locale to entriesExportUrl", () => {
    renderTab({ locale: "es" });
    expect(accountingApi.entriesExportUrl).toHaveBeenCalledWith(
      "42",
      expect.objectContaining({ lang: "es" }),
    );
  });

  it("passes es-AR through to entriesExportUrl unchanged", () => {
    renderTab({ locale: "es-AR" });
    expect(accountingApi.entriesExportUrl).toHaveBeenCalledWith(
      "42",
      expect.objectContaining({ lang: "es-AR" }),
    );
  });
});
