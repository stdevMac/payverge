import { useState, useEffect, useCallback } from 'react';
import { 
  AlternativePayment, 
  BillPaymentBreakdown, 
  PendingAlternativePayment 
} from '@/types/alternativePayments';
import alternativePaymentsAPI from '@/api/alternativePayments';

interface AlternativePaymentUpdate {
  type: 'payment_marked' | 'payment_requested' | 'bill_completed';
  billId: string;
  payment?: AlternativePayment;
  breakdown?: BillPaymentBreakdown;
  message?: string;
}

export function useAlternativePayments(billId: string) {
  const [paymentBreakdown, setPaymentBreakdown] = useState<BillPaymentBreakdown | null>(null);
  const [alternativePayments, setAlternativePayments] = useState<AlternativePayment[]>([]);
  const [pendingPayments, setPendingPayments] = useState<PendingAlternativePayment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Load initial data
  const loadData = useCallback(async () => {
    if (!billId) return;
    
    setLoading(true);
    setError(null);
    
    try {
      const [breakdown, altPayments, pending] = await Promise.all([
        alternativePaymentsAPI.getBillPaymentBreakdown(billId),
        alternativePaymentsAPI.getBillAlternativePayments(billId),
        alternativePaymentsAPI.getPendingAlternativePayments(billId).catch(() => []) // May fail for guests
      ]);
      
      setPaymentBreakdown(breakdown);
      setAlternativePayments(altPayments);
      setPendingPayments(pending);
    } catch (err) {
      console.error('Error loading alternative payment data:', err);
      setError('Failed to load payment data');
    } finally {
      setLoading(false);
    }
  }, [billId]);

  // WebSocket connection for real-time updates
  useEffect(() => {
    if (!billId) return;

    let cleanup: (() => void) | undefined;

    try {
      cleanup = alternativePaymentsAPI.subscribeToBillUpdates(billId, (update: AlternativePaymentUpdate) => {
        console.log('Alternative payment update received:', update);
        
        switch (update.type) {
          case 'payment_marked':
            if (update.payment) {
              setAlternativePayments(prev => [...prev, update.payment!]);
            }
            if (update.breakdown) {
              setPaymentBreakdown(update.breakdown);
            }
            // Remove from pending if it was there
            if (update.payment) {
              setPendingPayments(prev => 
                prev.filter(p => p.participantAddress !== update.payment!.participant)
              );
            }
            break;
            
          case 'payment_requested':
            // Refresh pending payments
            alternativePaymentsAPI.getPendingAlternativePayments(billId)
              .then(setPendingPayments)
              .catch(console.error);
            break;
            
          case 'bill_completed':
            if (update.breakdown) {
              setPaymentBreakdown(update.breakdown);
            }
            break;
        }
      });
    } catch (err) {
      console.error('Error setting up WebSocket connection:', err);
    }

    // Initial data load
    loadData();

    // Cleanup WebSocket on unmount
    return cleanup;
  }, [billId, loadData]);

  // Manual refresh function
  const refresh = useCallback(() => {
    loadData();
  }, [loadData]);

  return {
    paymentBreakdown,
    alternativePayments,
    pendingPayments,
    loading,
    error,
    refresh
  };
}

// Hook specifically for business owners
export function useBusinessAlternativePayments(billId: string) {
  const baseHook = useAlternativePayments(billId);
  
  const markPayment = useCallback(async (
    participantAddress: string,
    amount: string,
    paymentMethod: any
  ) => {
    try {
      const response = await alternativePaymentsAPI.markAlternativePayment({
        billId,
        participantAddress,
        amount,
        paymentMethod,
        businessConfirmation: true
      });
      
      if (response.success) {
        // Update local state immediately for better UX
        if (response.paymentBreakdown) {
          baseHook.refresh(); // Refresh all data
        }
      }
      
      return response;
    } catch (error) {
      console.error('Error marking alternative payment:', error);
      return {
        success: false,
        message: 'Failed to mark payment',
        paymentBreakdown: baseHook.paymentBreakdown || {
          totalAmount: '0',
          cryptoPaid: '0',
          alternativePaid: '0',
          remaining: '0',
          isComplete: false
        }
      };
    }
  }, [billId, baseHook]);

  return {
    ...baseHook,
    markPayment
  };
}

// Hook for guests to request alternative payments
export function useGuestAlternativePayments(billId: string) {
  const baseHook = useAlternativePayments(billId);
  
  const requestPayment = useCallback(async (
    amount: string,
    paymentMethod: any,
    participantName?: string
  ) => {
    try {
      const { requestAlternativePayment } = await import('@/api/alternativePayments');
      const response = await requestAlternativePayment(billId, amount, paymentMethod, participantName);
      
      if (response.success) {
        // Refresh data to show the new request
        setTimeout(() => baseHook.refresh(), 1000);
      }
      
      return response;
    } catch (error) {
      console.error('Error requesting alternative payment:', error);
      return {
        success: false,
        message: 'Failed to request payment'
      };
    }
  }, [billId, baseHook]);

  return {
    ...baseHook,
    requestPayment
  };
}
