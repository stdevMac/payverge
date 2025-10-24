// API functions for referral system
import { axiosInstance } from './index';

// Types for referral system
export interface Referrer {
  id: number;
  wallet_address: string;
  referral_code: string;
  tier: 'basic' | 'premium';
  status: 'active' | 'inactive' | 'suspended';
  total_referrals: number;
  total_commissions: string;
  claimable_commissions: string;
  last_claimed_at?: string;
  registration_tx_hash: string;
  created_at: string;
  updated_at: string;
}

export interface ReferralRecord {
  id: number;
  referrer_id: number;
  business_id: number;
  registration_fee: string;
  discount: string;
  commission: string;
  commission_paid: boolean;
  commission_tx_hash?: string;
  processed_tx_hash: string;
  created_at: string;
  updated_at: string;
  referrer?: Referrer;
  business?: any; // Business type from business.ts
}

export interface ReferralStats {
  total_referrers: number;
  total_referrals: number;
  total_commissions: string;
  basic_referrers: number;
  premium_referrers: number;
}

export interface RegisterReferrerRequest {
  wallet_address: string;
  referral_code: string;
  tier: 'basic' | 'premium';
  registration_tx_hash: string;
}

export interface CheckReferralCodeRequest {
  referral_code: string;
}

export interface CheckReferralCodeResponse {
  available: boolean;
  error?: string;
  message?: string;
}

export interface ClaimCommissionRequest {
  wallet_address: string;
  amount: string;
  tx_hash: string;
}

export interface ProcessReferralRequest {
  business_id: number;
  referral_code: string;
  registration_fee: string;
  discount: string;
  commission: string;
  processed_tx_hash: string;
}

export interface UpdateReferralCodeRequest {
  new_referral_code: string;
}

// Public API functions (no authentication required)

/**
 * Check if a referral code is available
 */
export const checkReferralCodeAvailability = async (
  referralCode: string
): Promise<CheckReferralCodeResponse> => {
  const response = await axiosInstance.post('/referrals/check-code', {
    referral_code: referralCode,
  });
  return response.data;
};

/**
 * Get overall referral system statistics
 */
export const getReferralStats = async (): Promise<ReferralStats> => {
  const response = await axiosInstance.get('/referrals/stats');
  return response.data;
};

/**
 * Get referrer information by referral code (public)
 */
export const getReferrerByCode = async (referralCode: string): Promise<Referrer> => {
  const response = await axiosInstance.get(`/referrals/referrer/code/${referralCode}`);
  return response.data;
};

// Protected API functions (require authentication)

/**
 * Register as a referrer
 */
export const registerReferrer = async (
  request: RegisterReferrerRequest
): Promise<{ message: string; referrer: Referrer }> => {
  const response = await axiosInstance.post('/inside/referrals/register', request);
  return response.data;
};

/**
 * Get referrer information by wallet address
 */
export const getReferrer = async (walletAddress: string): Promise<Referrer> => {
  const response = await axiosInstance.get(`/inside/referrals/referrer/${walletAddress}`);
  return response.data;
};

/**
 * Get all referrals made by a referrer
 */
export const getReferrerReferrals = async (
  walletAddress: string
): Promise<{ referrer: Referrer; referrals: ReferralRecord[] }> => {
  const response = await axiosInstance.get(`/inside/referrals/referrer/${walletAddress}/referrals`);
  return response.data;
};

/**
 * Process a referral when a business registers
 */
export const processReferral = async (
  request: ProcessReferralRequest
): Promise<{ message: string; referral_record: ReferralRecord }> => {
  const response = await axiosInstance.post('/inside/referrals/process', request);
  return response.data;
};

/**
 * Claim commission rewards
 */
export const claimCommission = async (
  request: ClaimCommissionRequest
): Promise<{ message: string; claim: any }> => {
  const response = await axiosInstance.post('/inside/referrals/claim', request);
  return response.data;
};

/**
 * Update referral code
 */
export const updateReferralCode = async (
  walletAddress: string,
  request: UpdateReferralCodeRequest
): Promise<{ message: string; referrer: Referrer }> => {
  const response = await axiosInstance.put(`/inside/referrals/referrer/${walletAddress}/code`, request);
  return response.data;
};

// Admin API functions (require admin authentication)

/**
 * Deactivate a referrer (admin only)
 */
export const deactivateReferrer = async (
  walletAddress: string
): Promise<{ message: string; referrer: Referrer }> => {
  const response = await axiosInstance.put(`/admin/referrals/referrer/${walletAddress}/deactivate`);
  return response.data;
};

// Utility functions

/**
 * Generate a random referral code
 */
export const generateReferralCode = (length: number = 8): string => {
  const characters = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
  let result = '';
  for (let i = 0; i < length; i++) {
    result += characters.charAt(Math.floor(Math.random() * characters.length));
  }
  return result;
};

/**
 * Validate referral code format
 */
export const validateReferralCode = (code: string): { valid: boolean; error?: string } => {
  if (code.length < 6 || code.length > 12) {
    return { valid: false, error: 'Referral code must be 6-12 characters long' };
  }
  
  if (!/^[A-Za-z0-9]+$/.test(code)) {
    return { valid: false, error: 'Referral code must contain only letters and numbers' };
  }
  
  return { valid: true };
};

/**
 * Format USDC amount for display
 */
export const formatUsdcAmount = (amount: string): string => {
  const num = parseFloat(amount);
  if (isNaN(num)) return '0.00';
  return (num / 1000000).toFixed(2); // Convert from 6 decimals to display format
};

/**
 * Parse USDC amount for blockchain
 */
export const parseUsdcAmount = (amount: string): string => {
  const num = parseFloat(amount);
  if (isNaN(num)) return '0';
  return Math.floor(num * 1000000).toString(); // Convert to 6 decimals
};

/**
 * Calculate referral discount based on tier
 */
export const calculateReferralDiscount = (
  registrationFee: string,
  tier: 'basic' | 'premium'
): { discount: string; discountedFee: string } => {
  const fee = parseFloat(registrationFee) / 1000000; // Convert from 6 decimals
  const discountRate = tier === 'premium' ? 0.15 : 0.10; // 15% for premium, 10% for basic
  
  const discount = fee * discountRate;
  const discountedFee = fee - discount;
  
  return {
    discount: Math.floor(discount * 1000000).toString(), // Convert back to 6 decimals
    discountedFee: Math.floor(discountedFee * 1000000).toString(),
  };
};

/**
 * Calculate referral commission based on tier
 */
export const calculateReferralCommission = (
  registrationFee: string,
  tier: 'basic' | 'premium'
): string => {
  const fee = parseFloat(registrationFee) / 1000000; // Convert from 6 decimals
  const commissionRate = tier === 'premium' ? 0.15 : 0.10; // 15% for premium, 10% for basic
  
  const commission = fee * commissionRate;
  return Math.floor(commission * 1000000).toString(); // Convert back to 6 decimals
};

/**
 * Get referral tier pricing
 */
export const getReferralTierPricing = () => {
  return {
    basic: {
      registrationFee: '10000000', // $10 USDC (6 decimals)
      discountRate: 10, // 10%
      commissionRate: 10, // 10%
      name: 'Basic Referrer',
      description: 'Earn 10% commission on every business you refer',
    },
    premium: {
      registrationFee: '25000000', // $25 USDC (6 decimals)
      discountRate: 15, // 15%
      commissionRate: 15, // 15%
      name: 'Premium Referrer',
      description: 'Earn 15% commission on every business you refer',
    },
  };
};

const referralsApi = {
  // Public functions
  checkReferralCodeAvailability,
  getReferralStats,
  getReferrerByCode,
  
  // Protected functions
  registerReferrer,
  getReferrer,
  getReferrerReferrals,
  processReferral,
  claimCommission,
  updateReferralCode,
  
  // Admin functions
  deactivateReferrer,
  
  // Utility functions
  generateReferralCode,
  validateReferralCode,
  formatUsdcAmount,
  parseUsdcAmount,
  calculateReferralDiscount,
  calculateReferralCommission,
  getReferralTierPricing,
};

export default referralsApi;
