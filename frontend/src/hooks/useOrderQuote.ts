"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  quoteBusinessOrder,
  quoteGuestOrder,
  type OrderDraft,
  type OrderQuote,
} from "@/api/orders";

type OrderQuoteTarget =
  | { tableCode: string; businessId?: never }
  | { businessId: number; tableCode?: never };

export type UseOrderQuoteOptions = OrderQuoteTarget & {
  draft: OrderDraft;
  enabled?: boolean;
  /** Bypass the edit debounce, for example when opening checkout. */
  immediate?: boolean;
};

export function useOrderQuote(options: UseOrderQuoteOptions) {
  const { draft, enabled = true, immediate = false } = options;
  const tableCode = "tableCode" in options ? options.tableCode : undefined;
  const businessId = "businessId" in options ? options.businessId : undefined;
  const itemCount = draft.items.length;
  const requestKey = JSON.stringify([
    tableCode ?? null,
    businessId ?? null,
    draft,
  ]);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const [quoteResult, setQuoteResult] = useState<{
    key: string;
    quote: OrderQuote;
  } | null>(null);
  const [isPending, setIsPending] = useState(false);
  const [errorResult, setErrorResult] = useState<{
    key: string;
    error: unknown;
  } | null>(null);
  const [refreshVersion, setRefreshVersion] = useState(0);
  const requestVersion = useRef(0);
  const handledRefreshVersion = useRef(0);

  const refresh = useCallback(
    () => setRefreshVersion((value) => value + 1),
    [],
  );

  useEffect(() => {
    if (!enabled || itemCount === 0) {
      requestVersion.current += 1;
      setQuoteResult(null);
      setIsPending(false);
      setErrorResult(null);
      return;
    }

    const version = ++requestVersion.current;
    const forceRefresh = refreshVersion !== handledRefreshVersion.current;
    handledRefreshVersion.current = refreshVersion;
    setIsPending(true);
    setErrorResult(null);
    const timer = window.setTimeout(
      async () => {
        try {
          const next =
            tableCode !== undefined
              ? await quoteGuestOrder(tableCode, draftRef.current)
              : await quoteBusinessOrder(
                  businessId as number,
                  draftRef.current,
                );
          if (requestVersion.current === version) {
            setQuoteResult({ key: requestKey, quote: next });
          }
        } catch (nextError) {
          if (requestVersion.current === version) {
            setErrorResult({ key: requestKey, error: nextError });
          }
        } finally {
          if (requestVersion.current === version) setIsPending(false);
        }
      },
      immediate || forceRefresh ? 0 : 250,
    );

    return () => window.clearTimeout(timer);
  }, [
    businessId,
    enabled,
    immediate,
    itemCount,
    refreshVersion,
    requestKey,
    tableCode,
  ]);

  const quote = quoteResult?.key === requestKey ? quoteResult.quote : null;
  const error = errorResult?.key === requestKey ? errorResult.error : null;
  const blockedLines =
    quote?.lines.filter((line) => line.orderability?.orderable === false) ?? [];
  const isValid =
    quote !== null && !isPending && error === null && blockedLines.length === 0;
  return { quote, isPending, error, isValid, blockedLines, refresh };
}
