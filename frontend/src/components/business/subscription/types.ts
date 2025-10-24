export interface SubscriptionData {
  status: 'active' | 'expired' | 'suspended' | 'cancelled';
  lastPaymentDate: string;
  subscriptionEndDate: string;
  lastPaymentAmount: string;
  totalPaid: string;
  yearlyFee: string; // The full yearly fee amount
  timeRemaining: number; // seconds remaining
  remindersSent: number;
}

export interface RenewalOption {
  months: number;
  suggestedAmount: string;
  description: string;
  popular?: boolean;
}

export interface SubscriptionManagementProps {
  subscriptionData: SubscriptionData;
  onRenewSubscription?: () => void;
  onToggleAutoRenewal?: () => void;
}

export type ProcessingStep = 'approval' | 'renewal' | 'complete';

export interface RenewalState {
  selectedOption: RenewalOption;
  customAmount: string;
  useCustomAmount: boolean;
  couponCode: string;
  useCoupon: boolean;
  isProcessing: boolean;
  processingStep: ProcessingStep;
}
