/** @jest-environment jsdom */
import React from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { accountingApi } from "@/api/accounting";
import {
  accountingKeys,
  useEntries,
  useVoidEntry,
} from "./useAccountingQueries";

jest.mock("@/api/accounting", () => ({
  accountingApi: {
    listEntries: jest.fn(),
    voidEntry: jest.fn(),
    getSummary: jest.fn(),
    getTimeseries: jest.fn(),
    listPayrollRunsPage: jest.fn(),
    getPayrollRun: jest.fn(),
    createEntry: jest.fn(),
    createPayrollRun: jest.fn(),
    markPayrollRunPaid: jest.fn(),
    voidPayrollRun: jest.fn(),
    deletePayrollRun: jest.fn(),
  },
}));

jest.mock("@/api/fiscal", () => ({
  fiscalApi: {
    listReceiptsPage: jest.fn(),
    listIssuableBills: jest.fn(),
    issueReceipt: jest.fn(),
    retryDeliveryTask: jest.fn(),
    creditFiscalReceipt: jest.fn(),
    resendFiscalReceipt: jest.fn(),
  },
}));

function createWrapper(queryClient?: QueryClient) {
  const client =
    queryClient ??
    new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
  };
}

describe("useEntries", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("refetches when params change", async () => {
    (accountingApi.listEntries as jest.Mock)
      .mockResolvedValueOnce({
        entries: [{ id: 1 }],
        total: 1,
        page: 1,
        page_size: 20,
        total_pages: 1,
      })
      .mockResolvedValueOnce({
        entries: [{ id: 2 }],
        total: 1,
        page: 2,
        page_size: 20,
        total_pages: 2,
      });

    const params1 = {
      start: "2026-01-01",
      end: "2026-01-31",
      page: 1,
    };
    const { result, rerender } = renderHook(
      ({ params }) => useEntries(42, params),
      {
        wrapper: createWrapper(),
        initialProps: { params: params1 },
      },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(accountingApi.listEntries).toHaveBeenCalledWith("42", params1);
    expect(result.current.data?.entries[0].id).toBe(1);

    const params2 = { ...params1, page: 2 };
    rerender({ params: params2 });

    await waitFor(() => expect(result.current.data?.entries[0].id).toBe(2));
    expect(accountingApi.listEntries).toHaveBeenCalledWith("42", params2);
    expect(accountingApi.listEntries).toHaveBeenCalledTimes(2);
  });
});

describe("useVoidEntry", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("invalidates accounting root key on success", async () => {
    (accountingApi.voidEntry as jest.Mock).mockResolvedValue(undefined);
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    const invalidateSpy = jest.spyOn(queryClient, "invalidateQueries");

    const { result } = renderHook(() => useVoidEntry(42), {
      wrapper: createWrapper(queryClient),
    });

    await act(async () => {
      await result.current.mutateAsync(7);
    });

    expect(accountingApi.voidEntry).toHaveBeenCalledWith("42", 7);
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: accountingKeys.root(42),
    });
  });
});

describe("accountingKeys", () => {
  it("nests summary/entries under the root key", () => {
    expect(accountingKeys.root(42)).toEqual(["accounting", 42]);
    expect(accountingKeys.summary(42, { start: "2026-01-01", end: "2026-01-31" })).toEqual([
      "accounting",
      42,
      "summary",
      { start: "2026-01-01", end: "2026-01-31" },
    ]);
    expect(
      accountingKeys.entries(42, {
        start: "2026-01-01",
        end: "2026-01-31",
        page: 1,
      }),
    ).toEqual([
      "accounting",
      42,
      "entries",
      { start: "2026-01-01", end: "2026-01-31", page: 1 },
    ]);
  });
});
