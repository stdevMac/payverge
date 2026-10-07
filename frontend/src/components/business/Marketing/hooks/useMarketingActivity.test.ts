/** @jest-environment jsdom */
import React from "react";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  useMarketingActivity,
  useMarketingActivityMutations,
} from "./useMarketingActivity";
import * as api from "@/api/marketing";

jest.mock("@/api/marketing");

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const Wrapper = ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
  Wrapper.displayName = "TestQueryWrapper";
  return Wrapper;
}

beforeEach(() => jest.clearAllMocks());

it("fetches the first page with the active filters", async () => {
  (api.getMarketingActivity as jest.Mock).mockResolvedValue({
    activity: [{ id: 1, suggestion_id: "1:offer:o1", status: "posted" }],
    total: 1,
    page: 1,
    per_page: 20,
  });
  const { result } = renderHook(
    () => useMarketingActivity(42, { status: "posted" }),
    { wrapper: wrapper() },
  );
  await waitFor(() => expect(result.current.items).toHaveLength(1));
  // The hook stringifies businessId for query-key stability before calling the API.
  expect(api.getMarketingActivity).toHaveBeenCalledWith(
    "42",
    expect.objectContaining({ status: "posted", page: 1 }),
  );
});

it("forwards the handled lifecycle cutoff independently from library created-at filters", async () => {
  (api.getMarketingActivity as jest.Mock).mockResolvedValue({
    activity: [],
    total: 7,
    page: 1,
    per_page: 20,
  });
  const { result } = renderHook(
    () =>
      useMarketingActivity(42, {
        from: "2026-01-01",
        handled_from: "2026-06-15T12:00:00Z",
      }),
    { wrapper: wrapper() },
  );
  await waitFor(() => expect(result.current.total).toBe(7));
  expect(api.getMarketingActivity).toHaveBeenCalledWith("42", {
    from: "2026-01-01",
    handled_from: "2026-06-15T12:00:00Z",
    page: 1,
    per_page: 20,
  });
});

it("loadMore appends the next page and stops at total", async () => {
  (api.getMarketingActivity as jest.Mock)
    .mockResolvedValueOnce({
      activity: [{ id: 1 }, { id: 2 }],
      total: 3,
      page: 1,
      per_page: 2,
    })
    .mockResolvedValueOnce({
      activity: [{ id: 3 }],
      total: 3,
      page: 2,
      per_page: 2,
    });
  const { result } = renderHook(() => useMarketingActivity(42, {}), {
    wrapper: wrapper(),
  });
  await waitFor(() => expect(result.current.items).toHaveLength(2));
  expect(result.current.hasMore).toBe(true);
  await act(async () => {
    await result.current.loadMore();
  });
  await waitFor(() => expect(result.current.items).toHaveLength(3));
  expect(result.current.hasMore).toBe(false);
});

it("posts the exact creative snapshot only when the explicit post mutation is invoked", async () => {
  (api.recordMarketingActivity as jest.Mock).mockResolvedValue({ id: 9 });
  const { result } = renderHook(() => useMarketingActivityMutations(42), {
    wrapper: wrapper(),
  });
  const creative = {
    caption: "Exact caption",
    image_url: "https://cdn/c.jpg",
    image_source: "menu" as const,
    template: "editorial" as const,
    aspect: "4:5" as const,
    slots: { dishName: "Carbonara" },
    crop: { x: 0.5, y: 0.5, zoom: 1 },
  };
  const suggestion = {
    id: "a",
    play: "featured_dish" as const,
    title: "Feature Carbonara",
    target_name: "Carbonara",
  };

  expect(api.recordMarketingActivity).not.toHaveBeenCalled();
  await act(async () => {
    await result.current.record.mutateAsync({
      action: "post",
      suggestion,
      creative_snapshot: creative,
    });
  });

  expect(api.recordMarketingActivity).toHaveBeenCalledWith("42", {
    action: "post",
    suggestion,
    creative_snapshot: creative,
  });
});

it("patches activity caches on record instead of invalidating the whole family", async () => {
  const activity = {
    id: 99,
    business_id: 42,
    suggestion_id: "a",
    play: "featured_dish" as const,
    title: "Feature Carbonara",
    target_name: "Carbonara",
    status: "posted" as const,
    image_url: "https://cdn/c.jpg",
    caption: "Exact caption",
    creative_snapshot: null,
    created_by: "owner",
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
  };
  (api.recordMarketingActivity as jest.Mock).mockResolvedValue(activity);

  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const invalidateQueries = jest.spyOn(qc, "invalidateQueries");
  const setQueriesData = jest.spyOn(qc, "setQueriesData");
  const Wrapper = ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);

  // Seed a multi-page infinite cache that would be expensive to refetch.
  qc.setQueryData(
    ["business", "42", "marketing", "activity", "", "", "", "", "", 20],
    {
      pages: [
        {
          activity: [{ id: 1, suggestion_id: "old", status: "posted" }],
          total: 2,
          page: 1,
          per_page: 20,
        },
        {
          activity: [{ id: 2, suggestion_id: "older", status: "dismissed" }],
          total: 2,
          page: 2,
          per_page: 20,
        },
      ],
      pageParams: [1, 2],
    },
  );

  const { result } = renderHook(() => useMarketingActivityMutations(42), {
    wrapper: Wrapper,
  });

  await act(async () => {
    await result.current.record.mutateAsync({
      action: "post",
      suggestion: {
        id: "a",
        play: "featured_dish",
        title: "Feature Carbonara",
        target_name: "Carbonara",
      },
      creative_snapshot: {
        caption: "Exact caption",
        image_url: "https://cdn/c.jpg",
        image_source: "menu",
        template: "editorial",
        aspect: "4:5",
        slots: { dishName: "Carbonara" },
        crop: { x: 0.5, y: 0.5, zoom: 1 },
      },
    });
  });

  expect(setQueriesData).toHaveBeenCalled();
  expect(invalidateQueries).not.toHaveBeenCalledWith({
    queryKey: ["business", "42", "marketing", "activity"],
  });
  // Suggestions still refresh (server exclusion).
  expect(invalidateQueries).toHaveBeenCalledWith({
    queryKey: ["business", "42", "marketing", "suggestions"],
  });

  const cached = qc.getQueryData<{
    pages: Array<{ activity: Array<{ id: number }>; total: number }>;
  }>(["business", "42", "marketing", "activity", "", "", "", "", "", 20]);
  expect(cached?.pages[0].activity[0].id).toBe(99);
  expect(cached?.pages[0].total).toBe(3);
  // Second page kept as-is (no refetch), only total bumped.
  expect(cached?.pages[1].activity).toHaveLength(1);
  expect(cached?.pages[1].total).toBe(3);
});

it("allows a per_page override so count-only callers can request 1 row", async () => {
  (api.getMarketingActivity as jest.Mock).mockResolvedValue({
    activity: [{ id: 1 }],
    total: 42,
    page: 1,
    per_page: 1,
  });
  const { result } = renderHook(
    () => useMarketingActivity(42, { handled_from: "2026-01-01" }, true, 1),
    { wrapper: wrapper() },
  );
  await waitFor(() => expect(result.current.total).toBe(42));
  expect(api.getMarketingActivity).toHaveBeenCalledWith("42", {
    handled_from: "2026-01-01",
    page: 1,
    per_page: 1,
  });
  expect(result.current.items).toHaveLength(1);
});

it("optimistically hides only after mutation and rolls back with retry context on failure", async () => {
  const error = new Error("record failed");
  (api.recordMarketingActivity as jest.Mock).mockRejectedValue(error);
  const onRecordError = jest.fn();
  const { result } = renderHook(
    () => useMarketingActivityMutations(42, onRecordError),
    { wrapper: wrapper() },
  );
  const vars = {
    action: "post" as const,
    suggestion: {
      id: "a",
      play: "featured_dish" as const,
      title: "Feature Carbonara",
    },
    creative_snapshot: {
      caption: "Feature Carbonara",
      image_url: "https://cdn.example.com/carbonara.jpg",
      image_source: "menu" as const,
      template: "editorial" as const,
      aspect: "4:5" as const,
      slots: { dishName: "Carbonara" },
      crop: { x: 0.5, y: 0.5, zoom: 1 },
    },
  };

  expect(result.current.locallyHidden.has("a")).toBe(false);
  act(() => result.current.record.mutate(vars));
  await waitFor(() => expect(result.current.locallyHidden.has("a")).toBe(true));
  await waitFor(() =>
    expect(result.current.locallyHidden.has("a")).toBe(false),
  );
  expect(onRecordError).toHaveBeenCalledWith(vars, error);
});

const locallyDismissedSuggestion = {
  id: "same-mount",
  play: "featured_dish" as const,
  title: "Feature Carbonara",
  why_data: "Top-selling dish",
  source: "menu_engineering",
  copy_angle: "Feature the guest favorite",
  rank: 1,
};

async function dismissLocally(result: {
  current: ReturnType<typeof useMarketingActivityMutations>;
}) {
  await act(async () => {
    await result.current.record.mutateAsync({
      action: "dismiss",
      suggestion: locallyDismissedSuggestion,
    });
  });
  expect(result.current.locallyHidden.has("same-mount")).toBe(true);
}

it("keeps a locally dismissed suggestion hidden when restore is a false no-op", async () => {
  (api.recordMarketingActivity as jest.Mock).mockResolvedValue({ id: 9 });
  (api.restoreMarketingActivity as jest.Mock).mockResolvedValue(false);
  const refreshSuggestions = jest
    .fn()
    .mockResolvedValue([locallyDismissedSuggestion]);
  const { result } = renderHook(
    () => useMarketingActivityMutations(42, undefined, refreshSuggestions),
    { wrapper: wrapper() },
  );

  await dismissLocally(result);
  await act(async () => {
    await result.current.restore.mutateAsync("same-mount");
  });

  expect(refreshSuggestions).toHaveBeenCalledTimes(1);
  expect(api.restoreMarketingActivity).toHaveBeenCalledTimes(1);
  expect(result.current.locallyHidden.has("same-mount")).toBe(true);
});

it("keeps a previously posted/cooldown suggestion hidden after restore refetch", async () => {
  (api.recordMarketingActivity as jest.Mock).mockResolvedValue({ id: 9 });
  (api.restoreMarketingActivity as jest.Mock)
    .mockResolvedValueOnce(true)
    .mockResolvedValueOnce(false);
  const refreshSuggestions = jest
    .fn()
    .mockResolvedValueOnce([])
    .mockResolvedValueOnce([locallyDismissedSuggestion]);
  const { result } = renderHook(
    () => useMarketingActivityMutations(42, undefined, refreshSuggestions),
    { wrapper: wrapper() },
  );

  await dismissLocally(result);
  await act(async () => {
    await result.current.restore.mutateAsync("same-mount");
  });

  expect(refreshSuggestions).toHaveBeenCalledTimes(1);
  expect(result.current.locallyHidden.has("same-mount")).toBe(true);

  await act(async () => {
    await result.current.restore.mutateAsync("same-mount");
  });
  expect(api.restoreMarketingActivity).toHaveBeenCalledTimes(2);
  expect(result.current.locallyHidden.has("same-mount")).toBe(true);
});

it("retries authoritative reconciliation without reposting a confirmed restore", async () => {
  (api.recordMarketingActivity as jest.Mock).mockResolvedValue({ id: 9 });
  (api.restoreMarketingActivity as jest.Mock).mockResolvedValue(true);
  const refreshError = new Error("suggestions refresh failed");
  const refreshSuggestions = jest
    .fn()
    .mockRejectedValueOnce(refreshError)
    .mockResolvedValueOnce([locallyDismissedSuggestion]);
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const invalidateQueries = jest.spyOn(qc, "invalidateQueries");
  const Wrapper = ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
  const { result } = renderHook(
    () => useMarketingActivityMutations(42, undefined, refreshSuggestions),
    { wrapper: Wrapper },
  );

  await dismissLocally(result);
  invalidateQueries.mockClear();
  await act(async () => {
    await expect(
      result.current.restore.mutateAsync("same-mount"),
    ).rejects.toThrow(refreshError.message);
  });

  expect(api.restoreMarketingActivity).toHaveBeenCalledTimes(1);
  expect(result.current.locallyHidden.has("same-mount")).toBe(true);
  expect(result.current.restore.isError).toBe(true);
  expect(result.current.restore.variables).toBe("same-mount");
  expect(invalidateQueries).not.toHaveBeenCalled();

  await act(async () => {
    await result.current.restore.mutateAsync("same-mount");
  });

  expect(api.restoreMarketingActivity).toHaveBeenCalledTimes(1);
  expect(refreshSuggestions).toHaveBeenCalledTimes(2);
  expect(result.current.locallyHidden.has("same-mount")).toBe(false);
  expect(result.current.restore.isError).toBe(false);
  // Restore patches activity caches in place — does not invalidate the family.
  expect(invalidateQueries).not.toHaveBeenCalledWith({
    queryKey: ["business", "42", "marketing", "activity"],
  });
});

it("unhides a genuine restore only after authoritative suggestions return it", async () => {
  (api.recordMarketingActivity as jest.Mock).mockResolvedValue({ id: 9 });
  (api.restoreMarketingActivity as jest.Mock).mockResolvedValue(true);
  let resolveSuggestions!: (
    suggestions: (typeof locallyDismissedSuggestion)[],
  ) => void;
  const refreshSuggestions = jest.fn(
    () =>
      new Promise<(typeof locallyDismissedSuggestion)[]>((resolve) => {
        resolveSuggestions = resolve;
      }),
  );
  const { result } = renderHook(
    () => useMarketingActivityMutations(42, undefined, refreshSuggestions),
    { wrapper: wrapper() },
  );

  await dismissLocally(result);
  let restorePromise!: Promise<unknown>;
  act(() => {
    restorePromise = result.current.restore.mutateAsync("same-mount");
  });
  await waitFor(() => expect(refreshSuggestions).toHaveBeenCalledTimes(1));

  expect(result.current.restore.isPending).toBe(true);
  expect(result.current.locallyHidden.has("same-mount")).toBe(true);

  await act(async () => {
    resolveSuggestions([locallyDismissedSuggestion]);
    await restorePromise;
  });
  expect(result.current.locallyHidden.has("same-mount")).toBe(false);
  expect(result.current.restore.isPending).toBe(false);
});
