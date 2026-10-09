/** @jest-environment jsdom */
/**
 * Issue #692 — an idea hidden by inventory ("BLOCKED BY INVENTORY / Hidden
 * Steak Plate") never shows up under Historial -> Ocultos, so the operator has
 * no way to recover it from the UI that advertises the recover action.
 *
 * The dismissed ledger rows for those ideas are written server-side as a side
 * effect of serving GET /marketing/suggestions. Nothing on the client tells the
 * activity queries about that write, and they are cached for 30s, so Ocultos
 * keeps rendering the page it fetched before the rows existed.
 */
import React from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useMarketingSuggestions } from "./useMarketingSuggestions";
import { useMarketingActivity } from "./useMarketingActivity";
import * as api from "@/api/marketing";

jest.mock("@/api/marketing");

const BUSINESS_ID = 8;

const hiddenRow = {
  id: 12,
  business_id: BUSINESS_ID,
  suggestion_id: "8:featured_dish:steak-plate",
  play: "featured_dish" as const,
  raw_play: "featured_dish",
  title: "Feature the Steak Plate",
  target_name: "Steak Plate",
  status: "dismissed",
  created_by: "inventory",
  created_at: "2026-08-19T20:00:00Z",
};

function wrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const Wrapper = ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
  Wrapper.displayName = "TestQueryWrapper";
  return Wrapper;
}

beforeEach(() => {
  jest.clearAllMocks();
});

it("surfaces inventory-hidden ideas in the dismissed ledger after the feed reports them", async () => {
  (api.getMarketingSuggestionsWithState as jest.Mock).mockResolvedValue({
    suggestions: [],
    paused: false,
    empty_reason: "",
    inventory_blocked: ["Steak Plate"],
  });
  // The ledger page fetched before the engine persisted the hidden idea is
  // empty; every later read has the dismissed row.
  (api.getMarketingActivity as jest.Mock)
    .mockResolvedValueOnce({
      activity: [],
      total: 0,
      page: 1,
      per_page: 20,
    })
    .mockResolvedValue({
      activity: [hiddenRow],
      total: 1,
      page: 1,
      per_page: 20,
    });

  const { result } = renderHook(
    () => {
      const feed = useMarketingSuggestions(BUSINESS_ID);
      const ledger = useMarketingActivity(BUSINESS_ID, {
        status: "dismissed",
      });
      return { feed, ledger };
    },
    { wrapper: wrapper() },
  );

  await waitFor(() =>
    expect(result.current.feed.inventoryBlocked).toEqual(["Steak Plate"]),
  );
  // Ocultos must list the idea the feed says is hidden.
  await waitFor(() => expect(result.current.ledger.total).toBe(1));
  expect(result.current.ledger.items.map((row) => row.target_name)).toEqual([
    "Steak Plate",
  ]);
});

it("does not churn the ledger when nothing is inventory-blocked", async () => {
  (api.getMarketingSuggestionsWithState as jest.Mock).mockResolvedValue({
    suggestions: [],
    paused: false,
    empty_reason: "",
    inventory_blocked: [],
  });
  (api.getMarketingActivity as jest.Mock).mockResolvedValue({
    activity: [],
    total: 0,
    page: 1,
    per_page: 20,
  });

  const { result } = renderHook(
    () => {
      const feed = useMarketingSuggestions(BUSINESS_ID);
      const ledger = useMarketingActivity(BUSINESS_ID, {
        status: "dismissed",
      });
      return { feed, ledger };
    },
    { wrapper: wrapper() },
  );

  await waitFor(() => expect(result.current.ledger.loading).toBe(false));
  await waitFor(() => expect(result.current.feed.loading).toBe(false));
  expect(api.getMarketingActivity).toHaveBeenCalledTimes(1);
});
