/** @jest-environment jsdom */
import { renderHook } from "@testing-library/react";
import { QueryClient } from "@tanstack/react-query";
import { useClearQueryCacheOnSessionChange } from "./queryCacheSession";
import { apiCache, stopCacheCleanup } from "@/utils/cache";
import { queryKeys } from "@/api/queryKeys";

afterAll(() => stopCacheCleanup());

describe("useClearQueryCacheOnSessionChange", () => {
  it("clears the React Query cache on logout", () => {
    const client = new QueryClient();
    const key = queryKeys.coverage.mine("42", 7);
    client.setQueryData(key, [{ id: 1 }]);
    renderHook(() => useClearQueryCacheOnSessionChange(client));

    apiCache.endSession();

    expect(client.getQueryData(key)).toBeUndefined();
    expect(client.getQueryCache().getAll()).toHaveLength(0);
  });

  it("clears the React Query cache when another principal signs in", () => {
    const client = new QueryClient();
    apiCache.setFingerprint("staff:7:42");
    renderHook(() => useClearQueryCacheOnSessionChange(client));
    client.setQueryData(queryKeys.timesheet.mine("42", 7), [{ id: 1 }]);

    apiCache.setFingerprint("staff:8:42");

    expect(client.getQueryCache().getAll()).toHaveLength(0);
    apiCache.endSession();
  });

  it("stops listening after unmount", () => {
    const client = new QueryClient();
    const { unmount } = renderHook(() => useClearQueryCacheOnSessionChange(client));
    unmount();
    client.setQueryData(["x"], 1);

    apiCache.endSession();

    expect(client.getQueryData(["x"])).toBe(1);
  });
});
