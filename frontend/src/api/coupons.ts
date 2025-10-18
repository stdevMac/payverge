import { axiosInstance } from "@/api";

// Types for coupon management
export interface CouponDetails {
  code: string;
  hash: string;
  discountAmount: string;
  expiryTime?: string;
  isActive: boolean;
  isUsed: boolean;
  usedBy?: string;
  usedAt?: string;
  createdAt: string;
}

export interface CouponCreateRequest {
  code: string;
  amount: string;
  expiry?: string;
}

export interface CouponValidationResponse {
  valid: boolean;
  code?: string;
  discountAmount?: string;
  hash?: string;
  message: string;
  status?: 'available' | 'used' | 'expired' | 'inactive' | 'invalid';
}

export interface CouponStats {
  total: number;
  active: number;
  used: number;
  expired: number;
  deactivated: number;
}

// API Response types
interface ApiResponse<T> {
  success: boolean;
  data?: T;
  message?: string;
  error?: string;
}

interface CouponResponse extends ApiResponse<CouponDetails> {
  coupon?: CouponDetails;
}

interface CouponsResponse extends ApiResponse<CouponDetails[]> {
  coupons?: CouponDetails[];
}

/**
 * Create a new coupon (admin only)
 * Note: This only creates the database record. The smart contract transaction
 * must be completed separately using the smart contract hooks.
 */
export const createCoupon = async (req: CouponCreateRequest): Promise<CouponResponse> => {
  try {
    const response = await axiosInstance.post<CouponResponse>("/admin/coupons", req);
    return response.data;
  } catch (error: any) {
    throw new Error(error.response?.data?.error || "Failed to create coupon");
  }
};

/**
 * Get a specific coupon by code or hash
 */
export const getCoupon = async (codeOrHash: string): Promise<CouponResponse> => {
  try {
    const response = await axiosInstance.get<CouponResponse>(`/admin/coupons/${codeOrHash}`);
    return response.data;
  } catch (error: any) {
    throw new Error(error.response?.data?.error || "Failed to get coupon");
  }
};

/**
 * Get all coupons (admin only)
 */
export const getAllCoupons = async (): Promise<CouponsResponse> => {
  try {
    const response = await axiosInstance.get<CouponsResponse>("/admin/coupons");
    return response.data;
  } catch (error: any) {
    throw new Error(error.response?.data?.error || "Failed to get coupons");
  }
};

/**
 * Validate a coupon for use (public endpoint)
 */
export const validateCoupon = async (code: string): Promise<CouponValidationResponse> => {
  try {
    const response = await axiosInstance.get<CouponValidationResponse>(`/coupons/validate/${code}`);
    return response.data;
  } catch (error: any) {
    throw new Error(error.response?.data?.error || "Failed to validate coupon");
  }
};

/**
 * Mark a coupon as used (called by blockchain event handlers)
 */
export const markCouponUsed = async (hash: string, usedBy: string, txHash?: string): Promise<ApiResponse<void>> => {
  try {
    const response = await axiosInstance.post<ApiResponse<void>>("/admin/coupons/mark-used", {
      hash,
      usedBy,
      txHash,
    });
    return response.data;
  } catch (error: any) {
    throw new Error(error.response?.data?.error || "Failed to mark coupon as used");
  }
};

/**
 * Deactivate a coupon (admin only)
 * Note: This only updates the database. The smart contract transaction
 * must be completed separately using the smart contract hooks.
 */
export const deactivateCoupon = async (code: string): Promise<ApiResponse<void>> => {
  try {
    const response = await axiosInstance.delete<ApiResponse<void>>(`/admin/coupons/${code}`);
    return response.data;
  } catch (error: any) {
    throw new Error(error.response?.data?.error || "Failed to deactivate coupon");
  }
};

/**
 * Get coupon usage statistics (admin only)
 */
export const getCouponStats = async (): Promise<ApiResponse<CouponStats>> => {
  try {
    const response = await axiosInstance.get<ApiResponse<CouponStats>>("/admin/coupons/stats");
    return response.data;
  } catch (error: any) {
    throw new Error(error.response?.data?.error || "Failed to get coupon stats");
  }
};

/**
 * Sync all coupons with blockchain state (admin only)
 */
export const syncCouponsWithBlockchain = async (): Promise<ApiResponse<{ syncedCount: number; totalCount: number }>> => {
  try {
    const response = await axiosInstance.post<ApiResponse<{ syncedCount: number; totalCount: number }>>("/admin/coupons/sync");
    return response.data;
  } catch (error: any) {
    throw new Error(error.response?.data?.error || "Failed to sync coupons");
  }
};

// Utility functions
export const formatCouponStatus = (coupon: CouponDetails): string => {
  if (coupon.isUsed) return "Used";
  if (!coupon.isActive) return "Deactivated";
  if (coupon.expiryTime && new Date(coupon.expiryTime) < new Date()) return "Expired";
  return "Active";
};

export const getCouponStatusColor = (coupon: CouponDetails): "success" | "danger" | "warning" | "primary" => {
  if (coupon.isUsed) return "success";
  if (!coupon.isActive) return "danger";
  if (coupon.expiryTime && new Date(coupon.expiryTime) < new Date()) return "warning";
  return "primary";
};

export const formatAddress = (address?: string): string => {
  if (!address) return "—";
  return `${address.slice(0, 6)}...${address.slice(-4)}`;
};

export const formatExpiryTime = (expiryTime?: string): string => {
  if (!expiryTime) return "No expiry";
  return new Date(expiryTime).toLocaleDateString();
};
