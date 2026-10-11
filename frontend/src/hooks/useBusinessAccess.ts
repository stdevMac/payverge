'use client';

import { useQuery, useQueryClient } from '@tanstack/react-query';
import { getBusiness } from '@/api/business';
import { logError } from '@/utils/errorLogger';
import { queryKeys } from '@/api/queryKeys';
import { useInstance } from './useInstance';

/**
 * The only lock a business has: the server administrator's lifecycle.
 * `suspended` = is_active false, `closed` = closed_at set. Both make the
 * dashboard read-only; the backend answers 403 business_suspended /
 * business_closed on every operational write.
 */
type BusinessLockState = 'active' | 'suspended' | 'closed';

interface BusinessAccessInfo {
  is_active: boolean;
  closed_at: string | null;
}

export interface UseBusinessAccessResult {
  access: BusinessAccessInfo | null;
  loading: boolean;
  error: string | null;
  /** True when the access fetch failed — distinct from a locked business. */
  isError: boolean;
  /** True once the server confirmed the business is operational. */
  hasAccess: boolean;
  /** True when a server administrator suspended or closed the business. */
  isSuspended: boolean;
  lockState: BusinessLockState;
  /**
   * False only when the server confirmed no LLM provider is configured, so AI
   * surfaces can say "configure a provider". True while loading.
   */
  aiConfigured: boolean;
  refetch: () => Promise<void>;
}

function lockStateOf(access: BusinessAccessInfo | null): BusinessLockState {
  if (!access) return 'active';
  if (access.closed_at) return 'closed';
  if (!access.is_active) return 'suspended';
  return 'active';
}

/**
 * Lock state of one business for dashboard gating.
 */
export function useBusinessAccess(
  businessId: number | string | undefined,
): UseBusinessAccessResult {
  const queryClient = useQueryClient();
  const { isOff } = useInstance();

  // Numeric ids and slugs resolve to the same business server-side; pick one
  // stable cache key so dashboard and overview share a single request.
  const requestId = businessId == null ? '' : String(businessId);
  const cacheKey = /^\d+$/.test(requestId) ? String(parseInt(requestId, 10)) : requestId;

  const {
    data: access = null,
    isLoading: loading,
    isError,
    error: queryError,
    refetch: refetchQuery,
  } = useQuery({
    queryKey: queryKeys.business.access(cacheKey),
    queryFn: async (): Promise<BusinessAccessInfo> => {
      try {
        const business = await getBusiness(requestId);
        return {
          is_active: business.is_active !== false,
          closed_at: business.closed_at ?? null,
        };
      } catch (err) {
        // 404 and 401 are expected (a slug the user no longer owns, an
        // expired session); logging them drowns real failures.
        const status =
          err && typeof err === 'object' && 'response' in err
            ? (err as { response?: { status?: number } }).response?.status
            : undefined;
        if (status !== 404 && status !== 401) {
          void logError(
            err instanceof Error ? err : String(err),
            'useBusinessAccess',
            'fetchAccess',
          );
        }
        throw new Error('Failed to load business access');
      }
    },
    enabled: !!cacheKey,
    staleTime: 10 * 60 * 1000,
  });

  const lockState = lockStateOf(access);

  const refetch = async (): Promise<void> => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.business.access(cacheKey) });
    await refetchQuery();
  };

  return {
    access,
    loading,
    error: queryError ? (queryError as Error).message : null,
    isError,
    hasAccess: access !== null && lockState === 'active',
    isSuspended: lockState !== 'active',
    lockState,
    aiConfigured: !isOff('ai'),
    refetch,
  };
}
