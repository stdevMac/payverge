/** @jest-environment jsdom */
/**
 * D1 / L6-12: Entradas "Exportar CSV" must forward active filters (status,
 * category, type, q) and locale into entriesExportUrl — not start/end alone.
 */
import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { accountingApi } from "@/api/accounting";
import { useEntries } from "@/hooks/accounting/useAccountingQueries";
import EntriesTab from "./EntriesTab";

jest.mock("@/hooks/accounting/useAccountingQueries", () => ({
  useEntries: jest.fn(),
  useVoidEntry: jest.fn(() => ({ mutate: jest.fn(), isPending: false })),
  useCreateEntry: jest.fn(() => ({ mutate: jest.fn(), isPending: false })),
}));

jest.mock("@/api/accounting", () => ({
  accountingApi: {
    entriesExportUrl: jest.fn(
      (_id: string, params: Record<string, string>) => {
        const q = new URLSearchParams(
          Object.entries(params).map(([k, v]) => [k, String(v)]),
        );
        return `https://api.test/entries/export.csv?${q.toString()}`;
      },
    ),
    listRecurringTemplates: jest.fn().mockResolvedValue([]),
    listAccountingCategories: jest.fn().mockResolvedValue({
      defaults: [],
      custom: [],
    }),
  },
}));

const useEntriesMock = useEntries as jest.Mock;

describe("EntriesTab L6-12 export filters DOM (D1)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.useFakeTimers();
    useEntriesMock.mockReturnValue({
      data: {
        entries: [],
        total: 0,
        page: 1,
        page_size: 20,
        total_pages: 1,
      },
      isLoading: false,
      isFetching: false,
      isSuccess: true,
    });
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it("rebuilds export href with status voided + locale when filter changes", async () => {
    render(
      <EntriesTab
        businessId="42"
        start="2026-01-01"
        end="2026-01-31"
        locale="es"
        currency="USD"
        canWrite
        t={(k) => k}
      />,
    );

    // Baseline call includes lang=es
    expect(accountingApi.entriesExportUrl).toHaveBeenCalledWith(
      "42",
      expect.objectContaining({
        start: "2026-01-01",
        end: "2026-01-31",
        lang: "es",
      }),
    );

    const statusTrigger = screen.getByTestId("entries-status-filter");
    fireEvent.click(statusTrigger);
    fireEvent.click(
      screen.getByRole("option", { name: "entries.filters.statusVoided" }),
    );

    await waitFor(() => {
      expect(accountingApi.entriesExportUrl).toHaveBeenCalledWith(
        "42",
        expect.objectContaining({
          status: "voided",
          lang: "es",
          start: "2026-01-01",
          end: "2026-01-31",
        }),
      );
    });

    const link = screen.getByRole("link", { name: /entries\.exportCsv/i });
    const href = link.getAttribute("href") || "";
    expect(href).toMatch(/status=voided/);
    expect(href).toMatch(/lang=es/);
    // notes column is owned by the server manifest (ENTRIES_CSV_COLUMNS includes notes)
  });

  it("forwards debounced search q into export URL", async () => {
    render(
      <EntriesTab
        businessId="42"
        start="2026-01-01"
        end="2026-01-31"
        locale="en"
        currency="USD"
        canWrite
        t={(k) => k}
      />,
    );

    const search = screen.getByLabelText("entries.filters.searchPlaceholder");
    fireEvent.change(search, { target: { value: "flour" } });

    act(() => {
      jest.advanceTimersByTime(300);
    });

    await waitFor(() => {
      expect(accountingApi.entriesExportUrl).toHaveBeenCalledWith(
        "42",
        expect.objectContaining({ q: "flour", lang: "en" }),
      );
    });

    const link = screen.getByRole("link", { name: /entries\.exportCsv/i });
    expect(link.getAttribute("href")).toMatch(/q=flour/);
  });
});
