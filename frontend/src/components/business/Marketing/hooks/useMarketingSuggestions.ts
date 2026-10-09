import { useEffect, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  getMarketingSuggestionsWithState,
  type CampaignSuggestion,
  type MarketingEmptyReason,
} from "@/api/marketing";
import { marketingActivityKeyPrefix } from "./useMarketingActivity";

export interface UseMarketingSuggestionsResult {
  suggestions: CampaignSuggestion[];
  paused: boolean;
  emptyReason: MarketingEmptyReason;
  inventoryBlocked: string[];
  loading: boolean;
  error: string | null;
  refetch: () => Promise<CampaignSuggestion[]>;
}

export function useMarketingSuggestions(
  businessId: number | string | undefined,
  enabled = true,
): UseMarketingSuggestionsResult {
  const id = businessId == null ? "" : String(businessId);
  const qc = useQueryClient();
  const { data, isLoading, error, refetch, dataUpdatedAt } = useQuery({
    queryKey: ["business", id, "marketing", "suggestions"] as const,
    queryFn: () => getMarketingSuggestionsWithState(id),
    enabled: !!id && enabled,
    staleTime: 5 * 60 * 1000, // suggestions derive from analytics; 5 min is fresh enough
  });

  // Serving suggestions is also what writes the inventory-hidden ideas into
  // the activity ledger (the engine persists them as dismissed rows). The
  // activity queries are cached for 30s and are never told about that
  // write, so Historial -> Ocultos kept rendering the pre-write page and the
  // "BLOCKED BY INVENTORY" ideas were unrecoverable from the UI that claims
  // to list them (#692). Refresh the ledger whenever a response reports one.
  const blockedSignature = (data?.inventory_blocked ?? []).join("\u0000");
  const syncedRef = useRef<string | null>(null);
  useEffect(() => {
    if (!id || !blockedSignature) return;
    const stamp = `${dataUpdatedAt}:${blockedSignature}`;
    if (syncedRef.current === stamp) return;
    syncedRef.current = stamp;
    void qc.invalidateQueries({ queryKey: marketingActivityKeyPrefix(id) });
  }, [id, blockedSignature, dataUpdatedAt, qc]);

  return {
    suggestions: data?.suggestions ?? [],
    paused: data?.paused ?? false,
    emptyReason: data?.empty_reason ?? "",
    inventoryBlocked: data?.inventory_blocked ?? [],
    loading: isLoading,
    error: error ? (error as Error).message : null,
    refetch: async () => {
      const result = await refetch({ throwOnError: true });
      return result.data?.suggestions ?? [];
    },
  };
}
