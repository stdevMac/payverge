import { axiosInstance } from "@/api/tools/instance";
import type { Dollars } from "@/types/money";

// Admin statistics interfaces
export interface MonthlyGrowth {
  month: string;
  count: number;
  value?: number; // For revenue/volume data
}

export interface AdminAction {
  id: number;
  admin_user_id: number;
  target_user_id: number;
  action_type: string;
  details: Record<string, any>;
  created_at: string;
}

export interface ErrorLogSummary {
  id: number;
  created_at: string;
  message: string;
  source: string;
  component: string;
}

export interface AdminStats {
  // Business metrics
  total_businesses: number;
  active_businesses: number;
  inactive_businesses: number;
  business_growth: MonthlyGrowth[];

  // User metrics
  total_users: number;
  users_by_role: Record<string, number>;
  user_growth: MonthlyGrowth[];

  // Payment metrics (dollars — backend divides cents by 100 before emit)
  total_payment_volume: number;
  payment_volume_growth: MonthlyGrowth[];
  average_transaction_size: number;
  /** ISO code of the volume figures; empty when venues use different currencies. */
  payment_volume_currency?: string;
  /** Every currency that contributed to the (unconverted) volume sums. */
  payment_volume_currencies?: string[];

  failed_webhooks_count: number;

  // Merchandise volume (restaurant guest takings — NOT platform revenue)
  gross_merchandise_volume: number;
  revenue_growth: MonthlyGrowth[];

  // Bill metrics
  total_bills?: number;
  recognized_bills?: number;
  bills_by_status?: Record<string, number>;
  bill_growth?: MonthlyGrowth[];

  // Operational
  recent_admin_actions: AdminAction[];
  recent_errors: ErrorLogSummary[];
}

export interface AdminUserListItem {
  id: number;
  name: string;
  email: string;
  business_name: string;
  status: string;
  joined_at: string;
  can_merge?: boolean;
  merge_with?: number[];
}

export interface AdminUserListResponse {
  users: AdminUserListItem[];
  total: number;
  page: number;
  limit: number;
}

// API functions
export const getAdminStats = async (): Promise<AdminStats> => {
  const response = await axiosInstance.get<AdminStats>('/admin/stats', {
    _useCache: false,
  });
  return response.data;
};

export const getUserList = async (params: {
  page?: number;
  limit?: number;
  search?: string;
  role?: string;
  status?: string;
}): Promise<AdminUserListResponse> => {
  const response = await axiosInstance.get<AdminUserListResponse>('/admin/users', {
    params,
    _useCache: false,
  });
  return { ...response.data, users: response.data.users || [] };
};

// Admin User Detail types
export interface AdminUserBusinessSummary {
  id: number;
  name: string;
  is_active: boolean;
  closed_at: string | null;
}

export interface AdminUserDetail {
  user: {
    id: number;
    email: string;
    name: string;
    role: string;
    auth_method: string;
    email_verified: boolean;
    created_at: string;
    picture: string;
  };
  business: {
    id: number;
    business_id: string;
    name: string;
    is_active: boolean;
    closed_at: string | null;
    closed_reason: string;
  } | null;
  admin_actions: {
    id: number;
    admin_email: string;
    action_type: string;
    details: Record<string, any>;
    created_at: string;
  }[];
  businesses?: AdminUserBusinessSummary[];
}

export const adminUserAPI = {
  getDetail: (userId: number, params?: { business_id?: number }) =>
    axiosInstance.get<AdminUserDetail>(`/admin/users/${userId}/detail`, {
      params,
      _useCache: false,
    }).then((r) => r.data),

  close: (userId: number, data: { reason: string; business_id?: number }) =>
    axiosInstance.post(`/admin/users/${userId}/close`, data).then(r => r.data),

  resetPassword: (userId: number) =>
    axiosInstance.post(`/admin/users/${userId}/reset-password`).then(r => r.data),
};
