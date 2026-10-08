import { axiosInstance } from "@/api/tools/instance";
import { AdminAction } from "@/api/admin";
import { asDollars, type Dollars } from "@/types/money";

interface AdminBusinessAddress {
  street?: string;
  city?: string;
  state?: string;
  postal_code?: string;
  country?: string;
}

/** Registry kind for admin business list (backend businesses.kind). */
export type AdminBusinessKind = "real" | "demo" | "test" | "all";

export interface AdminBusinessListItem {
  id: number;
  name: string;
  owner_name: string;
  owner_email: string;
  /** active | suspended | closed — the admin lifecycle. */
  status: string;
  /** real | demo | test — defaults to real on the wire when omitted. */
  kind?: string;
  created_at: string;
  last_active_at: string | null;
}

export interface AdminBusinessDetail {
  business: {
    id: number;
    name: string;
    slug?: string;
    address?: string;
    /** active | suspended | closed — the admin lifecycle. */
    status: string;
    is_active: boolean;
    closed_at: string | null;
    created_at: string;
    has_settlement_address: boolean;
    has_tipping_address: boolean;
  };
  owner: {
    id: number;
    email_domain: string;
    created_at: string;
  };
  staff: { id: number; name: string; email_domain: string; role: string }[];
  recent_activity: {
    recent_orders: number;
    recent_payments: number;
    /** Revenue in dollars (see @/types/money wire contract). */
    total_revenue: Dollars;
    /** Tips in dollars (see @/types/money wire contract). */
    total_tips: Dollars;
  };
  admin_actions: AdminAction[];
}

interface AdminBusinessDetailResponse {
  business?: {
    id?: number;
    name?: string;
    slug?: string;
    address?: AdminBusinessAddress | string | null;
    created_at?: string;
    is_active?: boolean;
    closed_at?: string | null;
    has_settlement_address?: boolean;
    has_tipping_address?: boolean;
  } | null;
  owner?: {
    id?: number;
    email_domain?: string;
    created_at?: string;
  } | null;
  staff?: {
    id: number;
    name?: string;
    email_domain?: string;
    role?: string;
  }[] | null;
  activity?: {
    recent_order_count?: number;
    recent_payment_count?: number;
    total_revenue?: number;
    total_tips?: number;
  } | null;
  recent_activity?: {
    recent_orders?: number;
    recent_payments?: number;
    total_revenue?: number;
    total_tips?: number;
  } | null;
  admin_actions?: AdminAction[] | null;
}

interface RawAdminBusinessActivity {
  recent_order_count?: number;
  recent_payment_count?: number;
  recent_orders?: number;
  recent_payments?: number;
  total_revenue?: number;
  total_tips?: number;
}

function formatBusinessAddress(
  address?: AdminBusinessAddress | string | null,
): string | undefined {
  if (!address) {
    return undefined;
  }

  if (typeof address === "string") {
    const trimmed = address.trim();
    return trimmed || undefined;
  }

  const parts = [
    address.street,
    address.city,
    address.state,
    address.postal_code,
    address.country,
  ]
    .map((part) => part?.trim())
    .filter(Boolean);

  return parts.length > 0 ? parts.join(", ") : undefined;
}

function normalizeAdminBusinessDetail(
  data: AdminBusinessDetailResponse,
): AdminBusinessDetail {
  const business = data.business ?? {};
  const owner = data.owner ?? {};
  const activity = (data.recent_activity ?? data.activity ?? {}) as RawAdminBusinessActivity;

  return {
    business: {
      id: business.id ?? 0,
      name: business.name ?? "",
      slug: business.slug || undefined,
      address: formatBusinessAddress(business.address),
      status: business.closed_at
        ? "closed"
        : business.is_active === false
          ? "suspended"
          : "active",
      is_active: business.is_active !== false,
      closed_at: business.closed_at ?? null,
      created_at: business.created_at ?? "",
      has_settlement_address: business.has_settlement_address === true,
      has_tipping_address: business.has_tipping_address === true,
    },
    owner: {
      id: owner.id ?? 0,
      email_domain: owner.email_domain ?? "",
      created_at: owner.created_at ?? "",
    },
    staff: (data.staff ?? []).map((member) => ({
      id: member.id,
      name: member.name ?? "",
      email_domain: member.email_domain ?? "",
      role: member.role ?? "",
    })),
    recent_activity: {
      recent_orders:
        activity.recent_orders ?? activity.recent_order_count ?? 0,
      recent_payments:
        activity.recent_payments ?? activity.recent_payment_count ?? 0,
      total_revenue: asDollars(activity.total_revenue ?? 0),
      total_tips: asDollars(activity.total_tips ?? 0),
    },
    admin_actions: data.admin_actions ?? [],
  };
}

export const adminBusinessAPI = {
  getList: async (params: {
    page?: number;
    limit?: number;
    search?: string;
    status?: string;
    /** Defaults to real on the backend when omitted. */
    kind?: AdminBusinessKind;
  }) => {
    const response = await axiosInstance.get("/admin/businesses", {
      params,
      _useCache: false,
    });
    return {
      ...response.data,
      businesses: (response.data?.businesses ?? []) as AdminBusinessListItem[],
    };
  },
  getDetail: async (id: number): Promise<AdminBusinessDetail> => {
    const response = await axiosInstance.get(
      `/admin/businesses/${id}/detail`,
      { _useCache: false },
    );
    return normalizeAdminBusinessDetail(response.data);
  },
  suspend: async (id: number, reason: string) => {
    const response = await axiosInstance.post(
      `/admin/businesses/${id}/suspend`,
      { reason }
    );
    return response.data;
  },
  reactivate: async (id: number, reason: string) => {
    const response = await axiosInstance.post(
      `/admin/businesses/${id}/reactivate`,
      { reason }
    );
    return response.data;
  },
};
