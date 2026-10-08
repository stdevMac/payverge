import type { ReadonlyURLSearchParams } from "next/navigation";

type RouteParams = Record<string, string | string[]> | null;

export function getRouteParam(params: RouteParams, key: string): string {
  const value = params?.[key];

  if (Array.isArray(value)) {
    return value[0] ?? "";
  }

  return value ?? "";
}

export function getSearchParam(
  searchParams: ReadonlyURLSearchParams | null,
  key: string,
): string | null {
  return searchParams?.get(key) ?? null;
}
