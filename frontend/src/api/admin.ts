import { axiosInstance } from './index';

// Admin statistics interfaces
export interface MonthlyGrowth {
  month: string;
  count: number;
  value?: number; // For revenue/volume data
}

export interface TopReferrer {
  wallet_address: string;
  referral_code: string;
  tier: string;
  total_referrals: number;
  total_commissions: number;
}

export interface AdminStats {
  // Business metrics
  total_businesses: number;
  active_businesses: number;
  inactive_businesses: number;
  business_growth: MonthlyGrowth[];
  businesses_by_referral: Record<string, number>;

  // User metrics
  total_users: number;
  users_by_role: Record<string, number>;
  user_growth: MonthlyGrowth[];

  // Payment metrics
  total_payment_volume: number;
  total_crypto_payments: number;
  total_alternative_payments: number;
  payment_volume_growth: MonthlyGrowth[];
  average_transaction_size: number;

  // Bill metrics
  total_bills: number;
  bills_by_status: Record<string, number>;
  bill_growth: MonthlyGrowth[];

  // Subscription metrics
  total_subscribers: number;
  subscriber_growth: MonthlyGrowth[];

  // Referral metrics
  total_referrers: number;
  referrers_by_tier: Record<string, number>;
  total_commissions_paid: number;
  top_referrers: TopReferrer[];

  // Revenue metrics
  total_revenue: number;
  revenue_growth: MonthlyGrowth[];
}

export interface Business {
  id: number;
  owner_address: string;
  name: string;
  logo: string;
  address: {
    street: string;
    city: string;
    state: string;
    postal_code: string;
    country: string;
  };
  settlement_address: string;
  tipping_address: string;
  tax_rate: number;
  service_fee_rate: number;
  tax_inclusive: boolean;
  service_inclusive: boolean;
  is_active: boolean;
  description: string;
  custom_url: string;
  phone: string;
  email: string;
  website: string;
  social_media: string;
  banner_images: string;
  business_page_enabled: boolean;
  show_reviews: boolean;
  google_reviews_enabled: boolean;
  referred_by_code: string;
  counter_enabled: boolean;
  counter_count: number;
  counter_prefix: string;
  default_currency: string;
  display_currency: string;
  default_language: string;
  created_at: string;
  updated_at: string;
}

export interface User {
  id: number;
  address: string;
  referral_code: string;
  token_id?: string;
  role: string;
  notification_preferences: {
    email_enabled: boolean;
    news_enabled: boolean;
    updates_enabled: boolean;
    transactional_enabled: boolean;
    security_enabled: boolean;
    reports_enabled: boolean;
    statistics_enabled: boolean;
  };
  created_at: string;
  updated_at: string;
}

export interface BusinessListResponse {
  businesses: Business[];
  total: number;
  page: number;
  limit: number;
}

export interface UserListResponse {
  users: User[];
  total: number;
  page: number;
  limit: number;
}

// API functions
export const getAdminStats = async (): Promise<AdminStats> => {
  const response = await axiosInstance.get<AdminStats>('/admin/stats');
  return response.data;
};

export const getBusinessList = async (params: {
  page?: number;
  limit?: number;
  search?: string;
}): Promise<BusinessListResponse> => {
  const response = await axiosInstance.get<BusinessListResponse>('/admin/businesses', {
    params
  });
  return response.data;
};

export const getUserList = async (params: {
  page?: number;
  limit?: number;
  search?: string;
  role?: string;
}): Promise<UserListResponse> => {
  const response = await axiosInstance.get<UserListResponse>('/admin/users', {
    params
  });
  return response.data;
};

// Smart contract data interfaces (for frontend hooks)
export interface ContractRevenue {
  total_fees_collected: bigint;
  registration_fees: bigint;
  platform_fees: bigint;
  total_businesses_registered: bigint;
  total_bills_created: bigint;
  total_payments_processed: bigint;
}

export interface ReferralContractStats {
  total_referrers: bigint;
  basic_referrers: bigint;
  premium_referrers: bigint;
  total_commissions_paid: bigint;
  total_referral_records: bigint;
}

export interface ProfitSplitStats {
  total_distributions: bigint;
  total_amount_distributed: bigint;
  active_beneficiaries: number;
  pending_distributions: bigint;
}
