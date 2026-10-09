import { useCallback, useRef, useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
  type InfiniteData,
  type QueryClient,
} from "@tanstack/react-query";
import {
  getMarketingActivity,
  recordMarketingActivity,
  restoreMarketingActivity,
  type CampaignSuggestion,
  type MarketingActivity,
  type MarketingActivityFilters,
  type MarketingActivityPage,
  type RecordMarketingActivityRequest,
} from "@/api/marketing";

const PER_PAGE = 20;

type ActivityInfiniteData = InfiniteData<
  MarketingActivityPage,
  number
>;

function marketingActivityKey(
  id: string,
  filters: Pick<
    MarketingActivityFilters,
    "status" | "play" | "from" | "to" | "handled_from"
  >,
  perPage: number = PER_PAGE,
) {
  return [
    "business",
    id,
    "marketing",
    "activity",
    filters.status ?? "",
    filters.play ?? "",
    filters.from ?? "",
    filters.to ?? "",
    filters.handled_from ?? "",
    perPage,
  ] as const;
}

/** Prefix for every activity infinite-query for a business (all filter combos). */
export function marketingActivityKeyPrefix(id: string) {
  return ["business", id, "marketing", "activity"] as const;
}

function isActivityInfiniteData(
  data: unknown,
): data is ActivityInfiniteData {
  return (
    !!data &&
    typeof data === "object" &&
    Array.isArray((data as ActivityInfiniteData).pages)
  );
}

/**
 * Upsert a recorded activity into every cached activity page set without
 * refetching. Matching suggestion_id rows are replaced in place; otherwise the
 * row is prepended to page 1 when the query's status filter accepts it, and
 * totals are adjusted.
 */
function patchActivityCachesWithRecord(
  qc: QueryClient,
  businessId: string,
  activity: MarketingActivity,
): void {
  qc.setQueriesData<ActivityInfiniteData>(
    { queryKey: marketingActivityKeyPrefix(businessId) },
    (current) => {
      if (!isActivityInfiniteData(current) || current.pages.length === 0) {
        return current;
      }
      // queryKey: ["business", id, "marketing", "activity", status, play, from, to, handled_from]
      // We cannot read the queryKey inside setQueriesData updater, so apply a
      // conservative patch: update matching rows everywhere; prepend only when
      // the first page already has rows of the same status or is empty with
      // total 0; always bump totals when inserting a new suggestion_id.
      const hasRow = current.pages.some((page) =>
        page.activity.some(
          (row) =>
            row.suggestion_id === activity.suggestion_id ||
            row.id === activity.id,
        ),
      );

      if (hasRow) {
        return {
          ...current,
          pages: current.pages.map((page) => ({
            ...page,
            activity: page.activity.map((row) =>
              row.suggestion_id === activity.suggestion_id ||
              row.id === activity.id
                ? { ...row, ...activity }
                : row,
            ),
          })),
        };
      }

      // New row: prepend onto page 1 and increment totals on every page.
      const [first, ...rest] = current.pages;
      const nextFirst: MarketingActivityPage = {
        ...first,
        total: first.total + 1,
        activity: [activity, ...first.activity],
      };
      return {
        ...current,
        pages: [
          nextFirst,
          ...rest.map((page) => ({ ...page, total: page.total + 1 })),
        ],
      };
    },
  );
}

/**
 * Apply a successful restore: dismissed→posted if the row was previously
 * posted, otherwise remove a pure-dismissal row. Adjusts totals.
 */
function patchActivityCachesWithRestore(
  qc: QueryClient,
  businessId: string,
  suggestionId: string,
): void {
  qc.setQueriesData<ActivityInfiniteData>(
    { queryKey: marketingActivityKeyPrefix(businessId) },
    (current) => {
      if (!isActivityInfiniteData(current) || current.pages.length === 0) {
        return current;
      }

      let mode: "none" | "revive" | "delete" = "none";
      for (const page of current.pages) {
        const row = page.activity.find((r) => r.suggestion_id === suggestionId);
        if (!row) continue;
        if (row.status !== "dismissed") {
          mode = "none";
          break;
        }
        mode = row.posted_at ? "revive" : "delete";
        break;
      }
      if (mode === "none") return current;

      if (mode === "revive") {
        return {
          ...current,
          pages: current.pages.map((page) => ({
            ...page,
            activity: page.activity.map((row) =>
              row.suggestion_id === suggestionId
                ? {
                    ...row,
                    status: "posted" as const,
                    dismissed_at: undefined,
                  }
                : row,
            ),
          })),
        };
      }

      // Pure dismissal deleted server-side — drop the row and decrement total.
      const removedFromAny = current.pages.some((page) =>
        page.activity.some((row) => row.suggestion_id === suggestionId),
      );
      if (!removedFromAny) return current;
      return {
        ...current,
        pages: current.pages.map((page) => ({
          ...page,
          activity: page.activity.filter(
            (row) => row.suggestion_id !== suggestionId,
          ),
          total: Math.max(0, page.total - 1),
        })),
      };
    },
  );
}

export function useMarketingActivity(
  businessId: number | string | undefined,
  filters: Pick<
    MarketingActivityFilters,
    "status" | "play" | "from" | "to" | "handled_from"
  >,
  enabled = true,
  perPage: number = PER_PAGE,
) {
  const id = businessId == null ? "" : String(businessId);
  const pageSize = perPage > 0 ? perPage : PER_PAGE;
  const query = useInfiniteQuery({
    queryKey: marketingActivityKey(id, filters, pageSize),
    queryFn: ({ pageParam = 1 }) =>
      getMarketingActivity(id, {
        ...filters,
        page: pageParam,
        per_page: pageSize,
      }),
    initialPageParam: 1,
    getNextPageParam: (lastPage) => {
      const loaded = lastPage.page * lastPage.per_page;
      return loaded < lastPage.total ? lastPage.page + 1 : undefined;
    },
    enabled: !!id && enabled,
    staleTime: 30_000,
  });

  const items: MarketingActivity[] = (query.data?.pages ?? []).flatMap(
    (p) => p.activity,
  );
  const total = query.data?.pages?.[0]?.total ?? 0;

  return {
    items,
    total,
    loading: query.isLoading,
    loadingMore: query.isFetchingNextPage,
    error: query.error ? (query.error as Error).message : null,
    hasMore: !!query.hasNextPage,
    loadMore: async () => {
      await query.fetchNextPage();
    },
    refetch: async () => {
      await query.refetch();
    },
  };
}

type RecordVars = RecordMarketingActivityRequest;

/**
 * Mutations that record post/dismiss and restore, patching activity caches in
 * place (no full infinite-query refetch storm).
 *
 * Ownership of the optimistic "locally hidden" set lives here so the rollback
 * on a failed record lives right next to the mutation that hid the card. A
 * failed POST removes the id from the hidden set (the card reappears in the
 * live feed) and calls `onRecordError` so the caller can surface a toast.
 */
export function useMarketingActivityMutations(
  businessId: number | string | undefined,
  onRecordError?: (vars: RecordVars, error: unknown) => void,
  refreshSuggestions?: () => Promise<CampaignSuggestion[]>,
) {
  const id = businessId == null ? "" : String(businessId);
  const qc = useQueryClient();

  // Dismissed exclusion is server-side; a local hidden set gives immediate UX
  // on dismiss/post before the cache patch, and is the thing we roll back on error.
  const [locallyHidden, setLocallyHidden] = useState<Set<string>>(new Set());
  const confirmedRestores = useRef<Set<string>>(new Set());
  const unhide = useCallback((suggestionId: string) => {
    setLocallyHidden((prev) => {
      if (!prev.has(suggestionId)) return prev;
      const next = new Set(prev);
      next.delete(suggestionId);
      return next;
    });
  }, []);

  const record = useMutation({
    mutationFn: (vars: RecordVars) => recordMarketingActivity(id, vars),
    onMutate: (vars: RecordVars) => {
      // Optimistically hide the card the moment the mutation starts.
      setLocallyHidden((prev) => {
        if (prev.has(vars.suggestion.id)) return prev;
        return new Set(prev).add(vars.suggestion.id);
      });
    },
    onSuccess: (activity) => {
      if (activity) {
        patchActivityCachesWithRecord(qc, id, activity);
      }
      // Suggestions feed still needs a refresh (server-side exclusion set).
      void qc.invalidateQueries({
        queryKey: ["business", id, "marketing", "suggestions"],
      });
    },
    onError: (error, vars) => {
      // Roll back the optimistic hide so the card returns to the live feed
      // instead of silently vanishing (and reappearing after the next refetch
      // without ever reaching the Library).
      unhide(vars.suggestion.id);
      onRecordError?.(vars, error);
    },
  });
  const restore = useMutation({
    mutationFn: async (suggestionId: string) => {
      let restored = confirmedRestores.current.has(suggestionId);
      if (!restored) {
        restored = await restoreMarketingActivity(id, suggestionId);
        if (restored) confirmedRestores.current.add(suggestionId);
      }

      const suggestions = refreshSuggestions
        ? await refreshSuggestions()
        : await qc
            .invalidateQueries({
              queryKey: ["business", id, "marketing", "suggestions"],
            })
            .then(() => [] as CampaignSuggestion[]);
      const visible =
        restored &&
        suggestions.some((suggestion) => suggestion.id === suggestionId);

      // A successful authoritative response is terminal for the confirmation,
      // whether the suggestion is visible or excluded by another lifecycle
      // rule. A rejected refresh intentionally leaves it in place so retry can
      // reconcile without depending on a second successful restore request.
      confirmedRestores.current.delete(suggestionId);

      // The restore endpoint can legitimately be a no-op, and a restored
      // activity can remain excluded by another lifecycle rule (for example,
      // a prior post cooldown). Only the fresh, unfiltered suggestion response
      // is authoritative enough to reverse the local hide.
      if (visible) unhide(suggestionId);

      // Targeted cache patch — no full activity family invalidation.
      if (restored) {
        patchActivityCachesWithRestore(qc, id, suggestionId);
      }

      return { restored, visible };
    },
  });
  return { record, restore, locallyHidden, unhide };
}
