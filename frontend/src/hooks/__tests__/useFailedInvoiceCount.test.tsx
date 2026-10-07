/** @jest-environment jsdom */
import React from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useFailedInvoiceCount } from "@/hooks/useFailedInvoiceCount";
import { fiscalApi } from "@/api/fiscal";

jest.mock("@/api/fiscal", () => ({
  fiscalApi: { listReceiptsPage: jest.fn() },
}));

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("useFailedInvoiceCount", () => {
  beforeEach(() => jest.clearAllMocks());

  it("counts failures via one paginated probe (server total, comma statuses)", async () => {
    (fiscalApi.listReceiptsPage as jest.Mock).mockResolvedValue({
      receipts: [],
      total: 3,
      page: 1,
      page_size: 1,
      total_pages: 3,
    });
    const { result } = renderHook(() => useFailedInvoiceCount(42, true), {
      wrapper,
    });
    await waitFor(() => expect(result.current).toBe(3));
    expect(fiscalApi.listReceiptsPage).toHaveBeenCalledWith(42, {
      status: "failed_retryable,failed_permanent,rejected,cancelled",
      page: 1,
      page_size: 1,
    });
  });

  it("returns 0 on fetch failure instead of throwing", async () => {
    (fiscalApi.listReceiptsPage as jest.Mock).mockRejectedValue(
      new Error("boom"),
    );
    const { result } = renderHook(() => useFailedInvoiceCount(42, true), {
      wrapper,
    });
    await waitFor(() =>
      expect(fiscalApi.listReceiptsPage).toHaveBeenCalledTimes(1),
    );
    expect(result.current).toBe(0);
  });

  it("returns 0 and does not fetch when disabled", async () => {
    const { result } = renderHook(() => useFailedInvoiceCount(42, false), {
      wrapper,
    });
    expect(result.current).toBe(0);
    expect(fiscalApi.listReceiptsPage).not.toHaveBeenCalled();
  });
});
