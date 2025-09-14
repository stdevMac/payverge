// TypeScript interfaces for PayvergePayments smart contract
export interface Bill {
  id: string;
  businessAddress: string;
  tippingAddress: string;
  totalAmount: bigint;
  paidAmount: bigint;
  tipAmount: bigint;
  createdAt: bigint;
  lastPaymentAt: bigint;
  status: BillStatus;
  metadata: string;
}

export interface Payment {
  id: string;
  billId: string;
  payer: string;
  amount: bigint;
  tipAmount: bigint;
  platformFee: bigint;
  timestamp: bigint;
  status: PaymentStatus;
  transactionHash: string;
}

export interface BusinessInfo {
  name: string;
  owner: string;
  paymentAddress: string;
  tippingAddress: string;
  registrationTime: bigint;
  isActive: boolean;
  totalVolume: bigint;
  totalPayments: bigint;
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

// Contract interaction parameters
export interface CreateBillParams {
  billId: string;
  totalAmount: bigint;
  metadata: string;
}

export interface ProcessPaymentParams {
  billId: string;
  amount: bigint;
  tipAmount: bigint;
}

export interface VerifyBusinessParams {
  businessAddress: string;
  name: string;
  paymentAddress: string;
  tippingAddress: string;
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

export interface BusinessVerifiedEvent {
  businessAddress: string;
  owner: string;
}

// Contract configuration is defined in config.ts

// Transaction options
export interface TransactionOptions {
  gasLimit?: bigint;
  gasPrice?: bigint;
  value?: bigint;
}
