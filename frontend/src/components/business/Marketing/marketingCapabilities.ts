import { hasPerm } from "@/constants/permissions";

export interface MarketingCapabilities {
  canView: boolean;
  canEdit: boolean;
  canGenerate: boolean;
  canUpload: boolean;
  /**
   * S2-E: owner may approve creatives marked ready. Staff with marketing:write
   * craft and mark ready only; UI mirrors backend owner gate.
   */
  canApprove: boolean;
  /** Owner principal flag for post-from-ready without prior approve. */
  isOwner: boolean;
}

/**
 * Mirrors backend authorization for UI visibility only. Route middleware remains
 * authoritative for every read and creative mutation.
 */
export function getMarketingCapabilities(
  permissions: readonly string[] | null | undefined,
  isOwner = false,
): MarketingCapabilities {
  if (isOwner) {
    return {
      canView: true,
      canEdit: true,
      canGenerate: true,
      canUpload: true,
      canApprove: true,
      isOwner: true,
    };
  }

  const canWrite = hasPerm(permissions, "marketing:write");
  return {
    canView: hasPerm(permissions, "marketing:read"),
    canEdit: canWrite,
    canGenerate: canWrite,
    canUpload: canWrite,
    canApprove: false,
    isOwner: false,
  };
}
