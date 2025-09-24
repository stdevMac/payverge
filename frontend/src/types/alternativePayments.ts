export enum PaymentMethod {
  CRYPTO = 0,
  CASH = 1,
  CARD = 2,
  VENMO = 3,
  OTHER = 4
}

export interface AlternativePayment {
  participant: string;        // Ethereum address
  amount: string;            // Amount in USDC (as string to handle precision)
  timestamp: number;         // Unix timestamp
  methodType: PaymentMethod; // Payment method enum
  verified: boolean;         // Always true for confirmed payments
}

export interface BillPaymentBreakdown {
  totalAmount: string;       // Total bill amount
  cryptoPaid: string;        // Amount paid via crypto
  alternativePaid: string;   // Amount paid via alternative methods
  remaining: string;         // Remaining amount to be paid
  isComplete: boolean;       // Whether bill is fully paid
}

export interface AlternativePaymentRequest {
  billId: string;
  participantAddress: string;
  amount: string;
  paymentMethod: PaymentMethod;
  businessConfirmation: boolean;
}

export interface AlternativePaymentResponse {
  success: boolean;
  transactionHash?: string;
  message: string;
  paymentBreakdown: BillPaymentBreakdown;
}

export interface PendingAlternativePayment {
  id: string;
  billId: string;
  participantName?: string;
  participantAddress: string;
  amount: string;
  paymentMethod: PaymentMethod;
  timestamp: number;
  status: 'pending' | 'confirmed' | 'failed';
}

// UI-specific types
export interface PaymentMethodOption {
  value: PaymentMethod;
  label: string;
  icon: string;
  description: string;
}

export const PAYMENT_METHOD_OPTIONS: PaymentMethodOption[] = [
  {
    value: PaymentMethod.CRYPTO,
    label: 'Crypto (USDC)',
    icon: '₿',
    description: 'Pay with USDC on blockchain'
  },
  {
    value: PaymentMethod.CASH,
    label: 'Cash',
    icon: '💵',
    description: 'Pay with physical cash'
  },
  {
    value: PaymentMethod.CARD,
    label: 'Credit/Debit Card',
    icon: '💳',
    description: 'Pay with credit or debit card'
  },
  {
    value: PaymentMethod.VENMO,
    label: 'Venmo/PayPal',
    icon: '📱',
    description: 'Pay with Venmo or PayPal'
  },
  {
    value: PaymentMethod.OTHER,
    label: 'Other',
    icon: '🔄',
    description: 'Other payment method'
  }
];

export function getPaymentMethodLabel(method: PaymentMethod): string {
  const option = PAYMENT_METHOD_OPTIONS.find(opt => opt.value === method);
  return option?.label || 'Unknown';
}

export function getPaymentMethodIcon(method: PaymentMethod): string {
  const option = PAYMENT_METHOD_OPTIONS.find(opt => opt.value === method);
  return option?.icon || '❓';
}

// Utility functions for amount formatting
export function formatUSDCAmount(amount: string): string {
  const numAmount = parseFloat(amount) / 1_000_000; // Convert from micro USDC to USDC
  return numAmount.toFixed(2);
}

export function parseUSDCAmount(amount: string): string {
  const numAmount = parseFloat(amount) * 1_000_000; // Convert from USDC to micro USDC
  return Math.floor(numAmount).toString();
}
