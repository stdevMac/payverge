/**
 * One sub-tab URL convention for the operator dashboard.
 *
 * Every multi-panel tab deep-links via `?sub=<key>` only.
 *
 * Unknown keys never silently fall back: callers render a not-found state that
 * names the bad key so typos and stale links stay visible.
 */

"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";

export const SUB_PARAM = "sub" as const;

type SubTabOk<T extends string> = {
  status: "ok";
  sub: T;
  /** True when the default is active but absent from the URL — write it so a shared link is reproducible. */
  needsDefaultInUrl: boolean;
};

type SubTabUnknown = {
  status: "unknown";
  badKey: string;
  param: typeof SUB_PARAM;
};

export type SubTabResult<T extends string> = SubTabOk<T> | SubTabUnknown;

export function isAllowedSub<T extends string>(
  value: string | null | undefined,
  allowed: readonly T[],
): value is T {
  return !!value && (allowed as readonly string[]).includes(value);
}

/**
 * Resolve the active sub-tab from the `sub` URL param.
 */
export function resolveSubTab<T extends string>(
  params: { sub: string | null },
  allowed: readonly T[],
  defaultSub: T,
  aliases?: Record<string, T>,
): SubTabResult<T> {
  const aliasOf = (raw: string | null): { value: string | null; from?: string } => {
    if (!raw || !aliases?.[raw]) return { value: raw };
    return { value: aliases[raw], from: raw };
  };
  const mappedSub = aliasOf(params.sub);
  const hasSub = mappedSub.value != null && mappedSub.value !== "";

  if (!hasSub) {
    return {
      status: "ok",
      sub: defaultSub,
      needsDefaultInUrl: true,
    };
  }

  if (isAllowedSub(mappedSub.value, allowed)) {
    return {
      status: "ok",
      sub: mappedSub.value,
      needsDefaultInUrl: Boolean(mappedSub.from),
    };
  }
  return { status: "unknown", badKey: params.sub as string, param: SUB_PARAM };
}

/**
 * Write `sub` onto a copy of the current search params.
 */
export function buildSubSearchParams(
  current: URLSearchParams,
  sub: string,
): URLSearchParams {
  const next = new URLSearchParams(current.toString());
  next.set(SUB_PARAM, sub);
  return next;
}

/**
 * Client hook: read/write the `sub` param for a tab's allowed keys.
 * Pure resolve lives above so unit tests don't need next/navigation.
 */
export function useSubTab<T extends string>(
  _tabKey: string,
  allowed: readonly T[],
  defaultSub: T,
): {
  sub: T | null;
  unknownSub: string | null;
  setSub: (next: T) => void;
} {
  const searchParams = useSearchParams();
  const router = useRouter();

  const subRaw = searchParams?.get(SUB_PARAM) ?? null;

  const resolved = useMemo(
    () => resolveSubTab({ sub: subRaw }, allowed, defaultSub),
    [subRaw, allowed, defaultSub],
  );

  // Optimistic selection so a click updates the panel before router.replace
  // lands (and so tests that mock replace without mutating searchParams still
  // observe the switch). Cleared whenever the URL-derived resolve changes.
  const [optimistic, setOptimistic] = useState<T | null>(null);
  useEffect(() => {
    setOptimistic(null);
  }, [subRaw]);

  // Write the default sub into the URL when it is absent.
  useEffect(() => {
    if (resolved.status !== "ok") return;
    if (!resolved.needsDefaultInUrl) return;
    if (typeof window === "undefined") return;
    const next = buildSubSearchParams(
      new URLSearchParams(searchParams?.toString() ?? ""),
      resolved.sub,
    );
    router.replace(`?${next.toString()}`, { scroll: false });
  }, [resolved, router, searchParams]);

  const setSub = useCallback(
    (next: T) => {
      const current =
        optimistic ?? (resolved.status === "ok" ? resolved.sub : null);
      if (next === current && resolved.status === "ok") return;
      setOptimistic(next);
      const sp = buildSubSearchParams(
        new URLSearchParams(searchParams?.toString() ?? ""),
        next,
      );
      router.replace(`?${sp.toString()}`, { scroll: false });
    },
    [optimistic, resolved, router, searchParams],
  );

  if (resolved.status === "unknown" && optimistic == null) {
    return { sub: null, unknownSub: resolved.badKey, setSub };
  }
  const sub =
    optimistic ?? (resolved.status === "ok" ? resolved.sub : defaultSub);
  return { sub, unknownSub: null, setSub };
}
