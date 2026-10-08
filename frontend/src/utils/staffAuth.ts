import { axiosInstance } from "@/api/tools/instance";
import { hasPerm } from "@/constants/permissions";
import {
  resolveStaffTabs,
  type StaffTabResolveOpts,
} from "@/constants/staffTabAccess";
import { apiCache } from "@/utils/cache";

export interface StaffData {
  id: number;
  name: string;
  email: string;
  role: "manager" | "server" | "host" | "kitchen";
  business_id: number;
  business_name?: string;
  business_slug?: string;
  is_active: boolean;
  last_login_at?: string;
}

export interface StaffSession {
  staff: StaffData;
  business: {
    id: number;
    name: string;
    business_id?: string;
  };
}

// Staff session data lives in HybridAuthProvider (hydrated from the
// /auth/session-info httpOnly-cookie endpoint). setStaffData is retained
// only for the post-login flow in StaffLogin/AcceptInvitation which hand
// data off to the provider via its own context. There is no getter here;
// components must read from useAuth().staffData instead.
let _staffData: StaffData | null = null;

export const setStaffData = (staffData: StaffData): void => {
  _staffData = staffData;
};

const clearStaffData = (): void => {
  _staffData = null;
};

// Clear all staff session data (backend clears httpOnly cookie)
export const clearStaffSession = async (): Promise<void> => {
  try {
    await axiosInstance.post("/staff/logout");
  } catch (error) {
    console.error("Error calling staff logout:", error);
  }
  clearStaffData();
  apiCache.endSession();
};

/**
 * Domain-level staff permission check against effective permission keys.
 * True if the principal has `{domain}:read` or any `{domain}:*` key.
 */
export function hasStaffPermission(
  permissions: string[] | null | undefined,
  domain:
    | "menu"
    | "tables"
    | "bills"
    | "financial"
    | "staff"
    | "analytics"
    | "settings"
    | "plugins"
    | "counter"
    | "inventory",
): boolean {
  const p = permissions ?? [];
  return p.some((x) => x === `${domain}:read` || x.startsWith(`${domain}:`));
}

/** @deprecated Prefer resolveStaffTabs(permissions). Temporary shim for non-dashboard call sites. */
export function getAllowedStaffTabs(
  role: string,
  ctx?: StaffTabResolveOpts & { permissions?: string[] },
): string[] {
  void role;
  if (ctx?.permissions) {
    return resolveStaffTabs(ctx.permissions, ctx);
  }
  // Fail closed — do not invent fat role maps
  return ["overview"];
}

interface PermissionCheckedActiveTabParams {
  activeTab: string;
  allowedTabs: string[];
  authResolved: boolean;
  fallbackTab?: string;
}

export const getPermissionCheckedActiveTab = ({
  activeTab,
  allowedTabs,
  authResolved,
  fallbackTab = "overview",
}: PermissionCheckedActiveTabParams): string => {
  if (authResolved && !allowedTabs.includes(activeTab)) {
    return fallbackTab;
  }
  return activeTab;
};

interface DashboardPrincipalResolutionParams {
  isStaffUser: boolean;
  hasStaffData: boolean;
  staffPermissionsLoading: boolean;
  isWeb3User: boolean;
  isOAuthUser: boolean;
}

/**
 * A staff principal is not permission-resolved until both its session profile
 * and effective permissions have hydrated. Treating the cookie alone as a
 * resolved principal can reject a valid cold dashboard deep link against the
 * temporary overview-only allowlist and permanently rewrite its URL.
 */
export const isDashboardPrincipalResolved = ({
  isStaffUser,
  hasStaffData,
  staffPermissionsLoading,
  isWeb3User,
  isOAuthUser,
}: DashboardPrincipalResolutionParams): boolean => {
  if (isStaffUser) {
    return hasStaffData && !staffPermissionsLoading;
  }
  return isWeb3User || isOAuthUser;
};

export const getRoleColor = (role: string): string => {
  const roleColors = {
    manager: "primary",
    server: "success",
    host: "warning",
    kitchen: "secondary",
  };

  return roleColors[role as keyof typeof roleColors] || "default";
};

export interface ReservationCapabilities {
  canRead: boolean;
  canCreate: boolean;
  canEdit: boolean; // backend: reservations:write
  canDelete: boolean; // backend: reservations:delete
  canManageSettings: boolean; // backend: reservations:settings
}

/** Capability flags from effective permissions (reservations:*). */
export function getReservationCapabilities(
  permissions: string[] | null | undefined,
): ReservationCapabilities {
  const p = permissions ?? [];
  return {
    canRead: hasPerm(p, "reservations:read"),
    canCreate: hasPerm(p, "reservations:create"),
    canEdit: hasPerm(p, "reservations:write"),
    canDelete: hasPerm(p, "reservations:delete"),
    canManageSettings: hasPerm(p, "reservations:settings"),
  };
}

export interface KitchenCapabilities {
  /** Can the user activate kitchen/orders when feature is off? settings:write. */
  canActivate: boolean;
  /** Can the user move orders through kitchen status states? orders:status. */
  canUpdateOrderStatus: boolean;
}

export interface BillPaymentCapabilities {
  /** Read payment breakdown, ledger, and pending guest requests (bills:read). */
  canViewPayments: boolean;
  /** Record or confirm in-person tender (bills:payment). */
  canRecordPayment: boolean;
}

/** Bill payment capabilities. Owners are special-cased full access. */
export function getBillPaymentCapabilities(
  permissions: string[] | null | undefined,
  isOwner: boolean,
): BillPaymentCapabilities {
  if (isOwner) {
    return { canViewPayments: true, canRecordPayment: true };
  }
  const p = permissions ?? [];
  return {
    canViewPayments: hasPerm(p, "bills:read"),
    canRecordPayment: hasPerm(p, "bills:payment"),
  };
}

/** Kitchen capabilities from effective permissions. */
export function getKitchenCapabilities(
  permissions: string[] | null | undefined,
): KitchenCapabilities {
  const p = permissions ?? [];
  return {
    canActivate: hasPerm(p, "settings:write"),
    canUpdateOrderStatus: hasPerm(p, "orders:status"),
  };
}

export interface AiWaiterCapabilities {
  canViewConfig: boolean; // owner only
  canViewConversations: boolean;
  canViewInsights: boolean;
  canReply: boolean;
  canClose: boolean;
  canForceRelease: boolean;
}

/**
 * AI Waiter capabilities from effective permissions.
 * Owners (OAuth/Web3 principal, no staff role) get full access including config.
 */
export function getAiWaiterCapabilities(
  permissions: string[] | null | undefined,
  isOwner: boolean,
): AiWaiterCapabilities {
  if (isOwner) {
    return {
      canViewConfig: true,
      canViewConversations: true,
      canViewInsights: true,
      canReply: true,
      canClose: true,
      canForceRelease: true,
    };
  }
  const p = permissions ?? [];
  return {
    canViewConfig: false,
    canViewConversations: hasPerm(p, "ai_waiter:read"),
    canViewInsights: hasPerm(p, "ai_waiter:insights"),
    canReply: hasPerm(p, "ai_waiter:reply"),
    canClose: hasPerm(p, "ai_waiter:write"),
    canForceRelease: hasPerm(p, "ai_waiter:write"),
  };
}
