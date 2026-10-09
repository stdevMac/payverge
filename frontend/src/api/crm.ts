import { axiosInstance } from "@/api/tools/instance";
import type { Business } from "@/api/business";

// Customer Types
export interface Customer {
  id: number;
  email: string;
  name: string;
  phone?: string;
  wallet_address?: string;
  birthday?: string;
  profile_image_url?: string;
  is_active: boolean;
  email_verified: boolean;
  last_login_at?: string;
  created_at: string;
  updated_at: string;
  preferences?: CustomerPreferences;
}

interface CustomerPreferences {
  id: number;
  customer_id: number;
  preferred_language: string;
  preferred_currency: string;
  receive_promotions: boolean;
  receive_newsletters: boolean;
  receive_birthday_offers: boolean;
  share_data_with_businesses: boolean;
  created_at: string;
  updated_at: string;
}

export interface CustomerBusiness {
  id: number;
  customer_id: number;
  business_id: number;
  loyalty_points: number;
  loyalty_tier?: string;
  total_spent: number;
  visit_count: number;
  last_visit_at?: string;
  first_visit_at: string;
  opt_in_marketing: boolean;
  opt_in_sms: boolean;
  opt_in_email: boolean;
  favorite_items?: string;
  dietary_preferences?: string;
  allergies?: string;
  notes?: string;
  tags?: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
  customer?: Customer;
  business?: Business;
}

/** Whole-base CRM aggregates returned in the customer-list response meta. */
export interface BusinessCustomerSummary {
  total_customers: number;
  active_this_month: number;
  /** Average lifetime spend in dollars (money wire contract). */
  avg_lifetime_spend: number;
  top_tier_count: number;
}

export interface BusinessCustomersResponse {
  customers: CustomerBusiness[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
  summary?: BusinessCustomerSummary;
}


// Customer Authentication API
export const crmAPI = {
  // Customer Registration & Authentication
  register: async (email: string, password: string, name: string) => {
    const response = await axiosInstance.post("/crm/register", {
      email,
      password,
      name,
    });
    return response.data;
  },

  login: async (email: string, password: string) => {
    const response = await axiosInstance.post("/crm/login", {
      email,
      password,
    });
    return response.data;
  },

  connectToBusiness: async (
    businessId: number,
    optInMarketing: boolean = false
  ) => {
    const response = await axiosInstance.post(
      `/customer/connect-business`,
      {
        business_id: businessId,
        opt_in_marketing: optInMarketing,
      }
    );
    return response.data;
  },

  // Customer Profile Management
  getProfile: async (): Promise<Customer> => {
    const response = await axiosInstance.get(`/customer/profile`);
    return response.data;
  },

  getBusinesses: async (): Promise<CustomerBusiness[]> => {
    const response = await axiosInstance.get(`/customer/businesses`);
    return response.data;
  },

  updateProfile: async (
    updates: {
      name?: string;
      phone?: string;
      birthday?: string;
      profile_image_url?: string;
    }
  ) => {
    const response = await axiosInstance.put(`/customer/profile`, updates);
    return response.data;
  },

  updatePreferences: async (
    updates: {
      preferred_language?: string;
      preferred_currency?: string;
      receive_promotions?: boolean;
      receive_newsletters?: boolean;
      receive_birthday_offers?: boolean;
      share_data_with_businesses?: boolean;
    }
  ) => {
    const response = await axiosInstance.put(`/customer/preferences`, updates);
    return response.data;
  },

  linkWallet: async (walletAddress: string) => {
    const response = await axiosInstance.post(`/customer/link-wallet`, {
      wallet_address: walletAddress,
    });
    return response.data;
  },

  deleteAccount: async () => {
    const response = await axiosInstance.delete(`/customer/account`);
    return response.data;
  },

  logout: async () => {
    const response = await axiosInstance.post(`/customer/logout`);
    return response.data;
  },
};

// Business CRM Management API
export const businessCRMAPI = {
  // CRM Status
  getCRMStatus: async (businessId: number) => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/crm/status`
    );
    return response.data;
  },

  // CRM Toggle
  toggleCRM: async (businessId: number, enabled: boolean) => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/crm/toggle`,
      { enabled }
    );
    return response.data;
  },

  // Customer Management
  getCustomers: async (
    businessId: number,
    page: number = 1,
    pageSize: number = 20,
    search: string = "",
    tier: string = "",
    opts: {
      /** Whitelisted server sort column (fix 6). */
      sortBy?: string;
      sortDir?: "asc" | "desc";
      /** Behavioral segment drilldown (fix 7): lapsed/vip/new/at-risk. */
      segment?: string;
    } = {}
  ): Promise<BusinessCustomersResponse> => {
    const params: Record<string, string | number> = {
      page,
      page_size: pageSize,
      search,
    };
    // "All" is the UI sentinel for no tier filter — omit it so the server
    // returns every tier.
    if (tier && tier !== "All") {
      params.tier = tier;
    }
    if (opts.segment) {
      params.segment = opts.segment;
    } else if (opts.sortBy) {
      // Segment scopes the whole base; the server ignores sort when a segment is
      // present, so only send sort params when NOT drilling into a segment.
      params.sort_by = opts.sortBy;
      params.sort_dir = opts.sortDir ?? "desc";
    }
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/crm/customers`,
      { params }
    );
    return response.data;
  },

  getCustomerDetails: async (
    businessId: number,
    customerBusinessId: number
  ) => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/crm/customers/${customerBusinessId}`
    );
    return response.data;
  },

  updateCustomerNotes: async (
    businessId: number,
    customerBusinessId: number,
    notes: string
  ) => {
    const response = await axiosInstance.put(
      `/inside/businesses/${businessId}/crm/customers/${customerBusinessId}/notes`,
      { notes }
    );
    return response.data;
  },

  updateCustomerAllergies: async (
    businessId: number,
    customerBusinessId: number,
    allergies: string
  ) => {
    const response = await axiosInstance.put(
      `/inside/businesses/${businessId}/crm/customers/${customerBusinessId}/allergies`,
      { allergies }
    );
    return response.data;
  },

  adjustCustomerLoyaltyPoints: async (
    businessId: number,
    customerBusinessId: number,
    loyaltyPoints: number,
    reason: string = ""
  ) => {
    const response = await axiosInstance.put(
      `/inside/businesses/${businessId}/crm/customers/${customerBusinessId}/loyalty-points`,
      { loyalty_points: loyaltyPoints, reason }
    );
    return response.data as { message: string; loyalty_points: number };
  },

  updateCustomerTags: async (
    businessId: number,
    customerBusinessId: number,
    tags: string
  ) => {
    const response = await axiosInstance.put(
      `/inside/businesses/${businessId}/crm/customers/${customerBusinessId}/tags`,
      { tags }
    );
    return response.data;
  },

  addCustomer: async (
    businessId: number,
    customer: { email: string; name: string; phone?: string },
  ) => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/crm/customers`,
      customer,
    );
    return response.data;
  },

  /** Soft-unlink from this business only (not a global account delete). */
  unlinkCustomer: async (businessId: number, customerBusinessId: number) => {
    const response = await axiosInstance.delete(
      `/inside/businesses/${businessId}/crm/customers/${customerBusinessId}`,
    );
    return response.data;
  },

  exportCustomers: async (businessId: number, lang?: string) => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/crm/export`,
      {
        params: lang ? { lang } : undefined,
        responseType: "blob",
      }
    );
    return response.data;
  },
};

export async function getSegments(
  businessId: number,
): Promise<Record<string, number>> {
  // Errors propagate so an outage renders a retryable error card, not a
  // false "no segments yet" empty state (audit §3.5 MED, fix 8). The previous
  // catch-to-zeros swallowed failures into a misleading empty state.
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/crm/segments`,
  );
  return response.data;
}
