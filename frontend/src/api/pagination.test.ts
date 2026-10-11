import { fetchAllPages } from "./pagination";
import type { PaginatedResponse } from "./pagination";

describe("fetchAllPages", () => {
  it("caps by maxPages and reports the last fetched page", async () => {
    const fetchPage = jest.fn(async (page: number) => ({
      items: [page],
      total: 20,
      page,
      page_size: 1,
      total_pages: 20,
    }));

    const result = await fetchAllPages<number>(
      fetchPage,
      (response) => response.items as number[],
      { pageSize: 1, maxPages: 10 },
    );

    expect(result.items).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);
    expect(result.capped).toBe(true);
    expect(result.warning).toBe("live_data_cap_reached");
    expect(result.metadata).toEqual({
      total: 20,
      page: 10,
      page_size: 1,
      total_pages: 20,
    });
    expect(fetchPage).toHaveBeenCalledTimes(10);
  });

  it("trims by maxRows and returns a warning", async () => {
    const fetchPage = jest.fn(async (page: number) => ({
      items: [page * 10 + 1, page * 10 + 2],
      total: 4,
      page,
      page_size: 2,
      total_pages: 2,
    }));

    const result = await fetchAllPages<number>(
      fetchPage,
      (response) => response.items as number[],
      { pageSize: 2, maxRows: 3 },
    );

    expect(result.items).toEqual([11, 12, 21]);
    expect(result.capped).toBe(true);
    expect(result.warning).toBe("live_data_cap_reached");
    expect(result.metadata.page).toBe(2);
  });

  it("handles malformed metadata without looping indefinitely", async () => {
    const fetchPage = jest.fn(async () => ({
      items: [1],
      total: "invalid",
      total_pages: "invalid",
    }) as unknown as PaginatedResponse<number>);

    const result = await fetchAllPages<number>(
      fetchPage,
      (response) => response.items as number[],
      { pageSize: 0, maxPages: 5 },
    );

    expect(result.items).toEqual([1]);
    expect(result.capped).toBe(false);
    expect(result.metadata).toEqual({
      total: 1,
      page: 1,
      page_size: 100,
      total_pages: 1,
    });
    expect(fetchPage).toHaveBeenCalledTimes(1);
  });

  it("reports the backend-normalized page size when it differs from the request", async () => {
    const fetchPage = jest.fn(async () => ({
      items: [1],
      total: 1,
      page: 1,
      page_size: 100,
      total_pages: 1,
    }));

    const result = await fetchAllPages<number>(
      fetchPage,
      (response) => response.items as number[],
      { pageSize: 500 },
    );

    expect(result.metadata.page_size).toBe(100);
  });
});
