import type { Business } from "@/api/business";
import { getBusinessDashboardPath } from "@/utils/businessUrl";
import { isSafeRedirectUrl } from "@/utils/safeRedirect";
import type { PwaIdentity } from "./installState";

const DASHBOARD_PATH = /^\/business\/[^/?#]+\/dashboard$/;
const SAFE_STAFF_BUSINESS_KEY = /^[A-Za-z0-9_-]+$/;

interface LaunchStaffData {
  business_id: number;
  business_slug?: string;
}

interface LaunchArgs {
  identity: PwaIdentity;
  rememberedPath: string | null;
  staffData: LaunchStaffData | null;
  loadBusinesses(): Promise<Pick<Business, "id" | "business_id">[]>;
  onResolutionError?(): void;
}

function isCandidateDashboardPath(path: string | null): path is string {
  return Boolean(path && isSafeRedirectUrl(path) && DASHBOARD_PATH.test(path));
}

function signalResolutionError(
  callback: LaunchArgs["onResolutionError"],
): void {
  try {
    callback?.();
  } catch {
    // Resolution must remain available when optional telemetry fails.
  }
}

export async function resolvePwaLaunchPath({
  identity,
  rememberedPath,
  staffData,
  loadBusinesses,
  onResolutionError,
}: LaunchArgs): Promise<string> {
  if (identity.roleType === "staff") {
    if (
      !staffData ||
      !Number.isSafeInteger(staffData.business_id) ||
      staffData.business_id <= 0
    ) {
      signalResolutionError(onResolutionError);
      return "/dashboard";
    }

    const slug = staffData.business_slug?.trim();
    if (
      staffData.business_slug !== undefined &&
      (!slug || !SAFE_STAFF_BUSINESS_KEY.test(slug))
    ) {
      signalResolutionError(onResolutionError);
    }
    const businessKey =
      slug && SAFE_STAFF_BUSINESS_KEY.test(slug)
        ? slug
        : String(staffData.business_id);
    return `/business/${businessKey}/dashboard`;
  }

  if (!rememberedPath) return "/dashboard";
  if (!isCandidateDashboardPath(rememberedPath)) {
    signalResolutionError(onResolutionError);
    return "/dashboard";
  }

  try {
    const businesses = await loadBusinesses();
    const owned = businesses.some(
      (business) => getBusinessDashboardPath(business) === rememberedPath,
    );
    if (!owned) signalResolutionError(onResolutionError);
    return owned ? rememberedPath : "/dashboard";
  } catch {
    signalResolutionError(onResolutionError);
    return "/dashboard";
  }
}
