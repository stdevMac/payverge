/** @jest-environment jsdom */
import React from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useMarketingSuggestions } from "./useMarketingSuggestions";
import * as api from "@/api/marketing";

jest.mock("@/api/marketing");

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const Wrapper = ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
  Wrapper.displayName = "TestQueryWrapper";
  return Wrapper;
}

it("surfaces the paused flag from the response", async () => {
  (api.getMarketingSuggestionsWithState as jest.Mock).mockResolvedValue({
    suggestions: [],
    paused: true,
    empty_reason: "paused",
  });
  const { result } = renderHook(() => useMarketingSuggestions(42), {
    wrapper: wrapper(),
  });
  await waitFor(() => expect(result.current.paused).toBe(true));
  expect(result.current.suggestions).toEqual([]);
  expect(result.current.emptyReason).toBe("paused");
  expect(result.current.inventoryBlocked).toEqual([]);
});

it("surfaces inventory-blocked dish names from the response", async () => {
  (api.getMarketingSuggestionsWithState as jest.Mock).mockResolvedValue({
    suggestions: [],
    paused: false,
    empty_reason: "",
    inventory_blocked: ["Steak Plate"],
  });
  const { result } = renderHook(() => useMarketingSuggestions(42), {
    wrapper: wrapper(),
  });
  await waitFor(() =>
    expect(result.current.inventoryBlocked).toEqual(["Steak Plate"]),
  );
});

it("returns the authoritative suggestion list from an explicit refetch", async () => {
  const restored = {
    id: "restored",
    play: "featured_dish" as const,
    title: "Restored",
    why_data: "Fresh response",
    source: "menu_engineering",
    copy_angle: "Angle",
    rank: 1,
  };
  (api.getMarketingSuggestionsWithState as jest.Mock)
    .mockResolvedValueOnce({ suggestions: [], paused: false, empty_reason: "" })
    .mockResolvedValueOnce({
      suggestions: [restored],
      paused: false,
      empty_reason: "",
    });
  const { result } = renderHook(() => useMarketingSuggestions(42), {
    wrapper: wrapper(),
  });
  await waitFor(() => expect(result.current.loading).toBe(false));

  let refreshed: unknown;
  await act(async () => {
    refreshed = await result.current.refetch();
  });

  expect(refreshed).toEqual([restored]);
});
