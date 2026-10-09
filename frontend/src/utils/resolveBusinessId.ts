import { getBusiness } from "@/api/business";

/**
 * Route params under /business/[businessId] may be a numeric id OR a slug
 * (e.g. "demo-admin-1-ai-pro"). Blindly `Number(param)` on a slug yields NaN,
 * which then hits `/api/v1/businesses/NaN/...` → 404 and collapses the page.
 *
 * Returns the numeric id, or null when the value can't be resolved. Never NaN.
 *
 * NOTE: getBusiness is axios-based, which short-circuits under SSR. Call this
 * from client components/effects only.
 */
export async function resolveNumericBusinessId(
  param: string,
): Promise<number | null> {
  if (/^\d+$/.test(param)) return parseInt(param, 10);
  try {
    const business = await getBusiness(param);
    return typeof business?.id === "number" ? business.id : null;
  } catch {
    return null;
  }
}
