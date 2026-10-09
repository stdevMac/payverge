"use client";

import { getSafeApiErrorMessage } from "@/utils/apiError";

import { useCallback, useEffect, useRef, useState } from "react";

import { InventoryMovement, inventoryApi } from "@/api/inventory";

const PAGE_SIZE = 25;

export interface InventoryMovementsState {
  movements: InventoryMovement[];
  total: number;
  page: number;
  totalPages: number;
  loading: boolean;
  error: string | null;
  setPage: (page: number) => void;
  reload: () => void;
}

interface Options {
  businessId: number;
  /** "" = all items. */
  itemFilter: string;
  /** "" = all types. */
  typeFilter: string;
  /** Only fetch while the Activity tab is the active surface. */
  enabled: boolean;
}

/**
 * Server-paginated inventory movement ledger for the Activity tab. Filters and
 * paging are resolved on the backend (item_id, movement_type, offset) instead
 * of slicing 25 global rows in the browser (audit §3.5 HIGH/C1). A request-id
 * guard drops stale responses when the operator changes filters mid-flight.
 */
export function useInventoryMovements({
  businessId,
  itemFilter,
  typeFilter,
  enabled,
}: Options): InventoryMovementsState {
  const [movements, setMovements] = useState<InventoryMovement[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Monotonic request id: only the latest fetch may commit to state.
  const requestId = useRef(0);
  const [reloadKey, setReloadKey] = useState(0);

  // Reset to page 1 whenever the filters change so a filter switch never leaves
  // the operator stranded on a page that no longer exists.
  useEffect(() => {
    setPage(1);
  }, [itemFilter, typeFilter]);

  useEffect(() => {
    if (!enabled) return;
    const id = ++requestId.current;
    setLoading(true);
    setError(null);
    inventoryApi
      .listMovementsPage(businessId, {
        itemId: itemFilter ? Number(itemFilter) : undefined,
        movementType: typeFilter || undefined,
        limit: PAGE_SIZE,
        offset: (page - 1) * PAGE_SIZE,
      })
      .then((res) => {
        if (id !== requestId.current) return;
        setMovements(res.movements);
        setTotal(res.total);
      })
      .catch((err: unknown) => {
        if (id !== requestId.current) return;
        setError(getSafeApiErrorMessage(err, "load_failed"));
      })
      .finally(() => {
        if (id !== requestId.current) return;
        setLoading(false);
      });
  }, [businessId, itemFilter, typeFilter, page, enabled, reloadKey]);

  const reload = useCallback(() => setReloadKey((k) => k + 1), []);

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return {
    movements,
    total,
    page,
    totalPages,
    loading,
    error,
    setPage,
    reload,
  };
}

const ITEM_HISTORY_PAGE = 10;

export interface ItemMovementHistoryState {
  movements: InventoryMovement[];
  total: number;
  loading: boolean;
  error: string | null;
  hasMore: boolean;
  loadMore: () => void;
}

/**
 * Per-item movement history for the ItemDetailDrawer's "Recent movements"
 * section: queries the server scoped to one item and appends pages via
 * load-more, so physical counts and adjustments are auditable past the old
 * 25-global-row window (audit §3.5 HIGH/C1). Resets whenever the drawer opens
 * on a different item.
 */
export function useItemMovementHistory(
  businessId: number,
  itemId: number | null,
): ItemMovementHistoryState {
  const [movements, setMovements] = useState<InventoryMovement[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [limit, setLimit] = useState(ITEM_HISTORY_PAGE);
  const requestId = useRef(0);

  // Reset the window whenever the selected item changes (drawer reopened).
  useEffect(() => {
    setLimit(ITEM_HISTORY_PAGE);
    setMovements([]);
    setTotal(0);
    setError(null);
  }, [itemId]);

  useEffect(() => {
    if (itemId == null) return;
    const id = ++requestId.current;
    setLoading(true);
    setError(null);
    // Fetch the first `limit` rows for this item in one call (offset 0, growing
    // limit). Simpler than accumulating pages and always consistent with the
    // server's newest-first ordering.
    inventoryApi
      .listMovementsPage(businessId, {
        itemId,
        limit,
        offset: 0,
      })
      .then((res) => {
        if (id !== requestId.current) return;
        setMovements(res.movements);
        setTotal(res.total);
      })
      .catch((err: unknown) => {
        if (id !== requestId.current) return;
        setError(getSafeApiErrorMessage(err, "load_failed"));
      })
      .finally(() => {
        if (id !== requestId.current) return;
        setLoading(false);
      });
  }, [businessId, itemId, limit]);

  const loadMore = useCallback(
    () => setLimit((l) => Math.min(l + ITEM_HISTORY_PAGE, 200)),
    [],
  );

  return {
    movements,
    total,
    loading,
    error,
    hasMore: movements.length < total,
    loadMore,
  };
}
