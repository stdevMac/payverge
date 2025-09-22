// TypeScript interfaces for PayvergePayments smart contract - Updated for v5.0.0-unified-simple
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
