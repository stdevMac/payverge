// TypeScript interfaces for Payverge ecosystem smart contracts

// ============ PayvergePayments Types ============
export interface Bill {
  businessAddress: string;
  isPaid: boolean;
  isCancelled: boolean;
  participantCount: number;
  createdAt: bigint;
  lastPaymentAt: bigint;
  totalAmount: bigint;
  paidAmount: bigint;
  nonce: string;
}

export interface Payment {
  id: string;
  billId: string;
  payer: string;
  timestamp: bigint;
  amount: bigint;
  tipAmount: bigint;
  platformFee: bigint;
}

export interface Participant {
  paidAmount: bigint;
  paymentCount: number;
  lastPaymentTime: number;
}

export interface BusinessInfo {
  paymentAddress: string;
  tippingAddress: string;
  isActive: boolean;
  registrationDate: bigint;
  totalVolume: bigint;
  totalTips: bigint;
}

export enum BillStatus {
  OPEN = 0,
  PAID = 1,
  CANCELLED = 2,
  EXPIRED = 3
}

export enum PaymentStatus {
  PENDING = 0,
  COMPLETED = 1,
  FAILED = 2,
  REFUNDED = 3
}

// Contract interaction parameters - Updated for new contract
export interface CreateBillParams {
  billId: string;
  businessAddress: string;
  totalAmount: bigint;
  metadata: string;
  nonce: string;
}

export interface ProcessPaymentParams {
  billId: string;
  amount: bigint;
  tipAmount: bigint;
}

export interface RegisterBusinessParams {
  name: string;
  paymentAddress: string;
  tippingAddress: string;
  referralCode?: string; // Optional referral code
}

export interface ClaimableAmounts {
  payments: bigint;
  tips: bigint;
}

// Event types
export interface BillCreatedEvent {
  billId: string;
  businessAddress: string;
  totalAmount: bigint;
  metadata: string;
}

export interface PaymentProcessedEvent {
  paymentId: string;
  billId: string;
  payer: string;
  amount: bigint;
  tipAmount: bigint;
  platformFee: bigint;
}

export interface BusinessRegisteredEvent {
  businessAddress: string;
  name: string;
  paymentAddress: string;
  tippingAddress: string;
}

// Contract configuration is defined in config.ts

// Transaction options
export interface TransactionOptions {
  gasLimit?: bigint;
  gasPrice?: bigint;
  value?: bigint;
}

// ============ PayvergeReferrals Types ============

export interface Referrer {
  referrerAddress: string;
  tier: number; // 1 = Basic, 2 = Premium
  isActive: boolean;
  registrationDate: bigint;
  totalReferrals: bigint;
  totalCommissions: bigint;
  claimableCommissions: bigint;
  lastClaimedAt: bigint;
  referralCode: string;
}

export interface ReferralRecord {
  id: string;
  referrer: string;
  business: string;
  timestamp: bigint;
  registrationFee: bigint;
  discount: bigint;
  commission: bigint;
  commissionPaid: boolean;
}

export interface RegisterReferrerParams {
  referralCode: string;
  tier: number; // 1 = Basic, 2 = Premium
}

export interface ProcessReferralParams {
  business: string;
  referralCode: string;
  registrationFee: bigint;
}

// ============ PayvergeProfitSplit Types ============

export interface Beneficiary {
  beneficiaryAddress: string;
  name: string;
  percentage: number; // Basis points (e.g., 1000 = 10%)
  isActive: boolean;
  totalReceived: bigint;
  lastDistributionAt: bigint;
}

export interface Distribution {
  id: string;
  totalAmount: bigint;
  beneficiaryCount: number;
  timestamp: bigint;
  triggeredBy: string;
}

export interface AddBeneficiaryParams {
  beneficiary: string;
  name: string;
  percentage: number; // Basis points
}

export interface DistributeProfitsParams {
  amount: bigint;
}

// ============ Event Types for New Contracts ============

export interface ReferrerRegisteredEvent {
  referrer: string;
  tier: number;
  referralCode: string;
  fee: bigint;
}

export interface ReferralProcessedEvent {
  referrer: string;
  business: string;
  discount: bigint;
  commission: bigint;
  referralCode: string;
}

export interface CommissionClaimedEvent {
  referrer: string;
  amount: bigint;
  timestamp: bigint;
}

export interface BeneficiaryAddedEvent {
  beneficiary: string;
  name: string;
  percentage: number;
  addedBy: string;
}

export interface ProfitDistributedEvent {
  distributionId: string;
  totalAmount: bigint;
  beneficiaryCount: number;
  triggeredBy: string;
}

export interface BeneficiaryPayoutEvent {
  distributionId: string;
  beneficiary: string;
  amount: bigint;
  percentage: number;
}
