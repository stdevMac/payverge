"use client";

import { useQuery } from "@tanstack/react-query";
import { getStaffPermissions } from "@/api/rbac";
import { queryKeys } from "@/api/queryKeys";

/**
 * Fetch a staff member's effective permissions plus role defaults and custom grants.
 *
 * Pass `enabled` so the query only runs when the consumer is ready (e.g. modal open).
 */
export function useStaffPermissions(
  businessId: string | undefined,
  staffId: number | string | undefined,
  enabled: boolean,
) {
  return useQuery({
    queryKey: queryKeys.staff.permissions(businessId ?? "", staffId ?? ""),
    queryFn: () => getStaffPermissions(String(businessId), String(staffId)),
    enabled: Boolean(enabled && businessId && staffId),
    staleTime: 30_000,
  });
}
