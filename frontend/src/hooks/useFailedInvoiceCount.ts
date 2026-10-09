"use client";

import { useQuery } from "@tanstack/react-query";
import { fiscalApi } from "@/api/fiscal";
import { queryKeys } from "@/api/queryKeys";
import { INVOICE_FAILURE_STATUSES } from "@/components/business/fiscal/invoiceStatus";

/**
 * Server-side count of failed fiscal invoices for the persistent sidebar
 * badge: one paginated probe (page_size:1, comma status list) whose envelope
 * `total` is the count. Replaces the legacy unpaginated listReceipts scan,
 * which pulled every receipt row (with delivery embeds) just to count a
 * handful of failures client-side — and silently capped at the backend's
 * 50-row legacy limit. Cached 60s on its own React Query key. `enabled` lets
 * callers skip the request for businesses with no fiscal access.
 */
export function useFailedInvoiceCount(
  businessId: number,
  enabled: boolean,
): number {
  const shouldFetch = enabled && businessId > 0;
  const { data } = useQuery({
    queryKey: [
      ...queryKeys.fiscal.receipts(String(businessId)),
      "failed-count",
    ],
    queryFn: () =>
      fiscalApi
        .listReceiptsPage(businessId, {
          status: INVOICE_FAILURE_STATUSES.join(","),
          page: 1,
          page_size: 1,
        })
        .then((page) => page.total)
        .catch(() => 0),
    enabled: shouldFetch,
    staleTime: 60_000,
  });
  return data ?? 0;
}
