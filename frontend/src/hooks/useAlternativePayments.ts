import { useCallback } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  AlternativePayment,
  BillPaymentBreakdown,
  MoneyString,
  PaymentMethod,
  PendingAlternativePayment,
} from "@/types/alternativePayments";
import alternativePaymentsAPI from "@/api/alternativePayments";
import { logError } from "@/utils/errorLogger";
import { queryKeys } from "@/api/queryKeys";

interface AlternativePaymentsData {
  paymentBreakdown: BillPaymentBreakdown | null;
  alternativePayments: AlternativePayment[];
  pendingPayments: PendingAlternativePayment[];
}

async function fetchBusinessAlternativePaymentsData(
  billId: string,
): Promise<AlternativePaymentsData> {
  const [breakdown, pending] = await Promise.all([
    alternativePaymentsAPI.getBillPaymentBreakdownInside(billId),
    alternativePaymentsAPI.getPendingAlternativePayments(billId),
  ]);
  return {
    paymentBreakdown: breakdown,
    alternativePayments: [],
    pendingPayments: pending,
  };
}

function useBusinessAlternativePaymentsData(billId: string) {
  const {
    data,
    isLoading,
    error: queryError,
    refetch,
  } = useQuery({
    queryKey: queryKeys.payments.alternativeInside(billId),
    queryFn: () => fetchBusinessAlternativePaymentsData(billId),
    enabled: !!billId,
    refetchInterval: 10_000,
    staleTime: 0,
  });

  const error = queryError
    ? (() => {
        logError(queryError, "useAlternativePayments", "loadData");
        return "Failed to load payment data";
      })()
    : null;

  const refresh = useCallback(() => {
    refetch();
  }, [refetch]);

  return {
    paymentBreakdown: data?.paymentBreakdown ?? null,
    alternativePayments: data?.alternativePayments ?? [],
    pendingPayments: data?.pendingPayments ?? [],
    loading: isLoading,
    error,
    refresh,
  };
}

// Hook specifically for business owners
export function useBusinessAlternativePayments(billId: string) {
  const queryClient = useQueryClient();
  const baseHook = useBusinessAlternativePaymentsData(billId);

  const markPaymentMutation = useMutation({
    mutationFn: async ({
      participantAddress,
      participantName,
      amount,
      tipAmount,
      paymentMethod,
      requestId,
      idempotencyKey,
    }: {
      participantAddress?: string;
      participantName?: string;
      amount: MoneyString;
      tipAmount?: MoneyString;
      paymentMethod: PaymentMethod;
      requestId?: string;
      idempotencyKey?: string;
    }) => {
      return alternativePaymentsAPI.markAlternativePayment({
        billId,
        participantAddress,
        participantName,
        amount,
        tipAmount,
        paymentMethod,
        businessConfirmation: true,
        requestId,
        idempotencyKey,
      });
    },
    onSuccess: (response) => {
      if (response.success && response.paymentBreakdown) {
        queryClient.invalidateQueries({
          queryKey: queryKeys.payments.alternativeInside(billId),
        });
        queryClient.invalidateQueries({
          queryKey: ["payments", "alternative", "guest"],
        });
        queryClient.invalidateQueries({ queryKey: ["bills"] });
      }
    },
    onError: (error) => {
      logError(
        error instanceof Error ? error : String(error),
        "useBusinessAlternativePayments",
        "markPayment",
      );
    },
  });

  const rejectPaymentMutation = useMutation({
    mutationFn: (requestId: string) =>
      alternativePaymentsAPI.rejectPendingAlternativePayment(billId, requestId),
    onSuccess: (response) => {
      if (response.success) {
        void queryClient.invalidateQueries({
          queryKey: queryKeys.payments.alternativeInside(billId),
        });
        void queryClient.invalidateQueries({
          queryKey: ["payments", "alternative", "guest"],
        });
        void queryClient.invalidateQueries({ queryKey: ["bills"] });
      }
    },
    onError: (error) => {
      logError(
        error instanceof Error ? error : String(error),
        "useBusinessAlternativePayments",
        "rejectPayment",
      );
    },
  });

  const markPayment = useCallback(
    async (
      participantAddress: string | undefined,
      amount: MoneyString,
      paymentMethod: PaymentMethod,
      requestId?: string,
      options?: {
        participantName?: string;
        tipAmount?: MoneyString;
        idempotencyKey?: string;
      },
    ) => {
      try {
        return await markPaymentMutation.mutateAsync({
          participantAddress,
          participantName: options?.participantName,
          amount,
          tipAmount: options?.tipAmount,
          paymentMethod,
          requestId,
          idempotencyKey: options?.idempotencyKey,
        });
      } catch (error) {
        logError(
          error instanceof Error ? error : String(error),
          "useBusinessAlternativePayments",
          "markPayment",
        );
        // Empty message so the (locale-aware) caller falls back to its own
        // translated error toast instead of this English sentinel (LOW).
        return {
          success: false,
          message: "",
          paymentBreakdown: baseHook.paymentBreakdown || {
            totalAmount: "0",
            cryptoPaid: "0",
            alternativePaid: "0",
            remaining: "0",
            isComplete: false,
          },
        };
      }
    },
    [markPaymentMutation, baseHook.paymentBreakdown],
  );

  const rejectPayment = useCallback(
    async (requestId: string) => {
      try {
        return await rejectPaymentMutation.mutateAsync(requestId);
      } catch (error) {
        logError(
          error instanceof Error ? error : String(error),
          "useBusinessAlternativePayments",
          "rejectPayment",
        );
        return { success: false, status: "rejected" as const };
      }
    },
    [rejectPaymentMutation],
  );

  return {
    ...baseHook,
    markPayment,
    rejectPayment,
  };
}
