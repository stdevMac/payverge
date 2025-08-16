import { useState, useCallback } from "react";
import toast from "react-hot-toast";
import { waitForTransactionReceipt } from "wagmi/actions";
import { config } from "@/config";
import { CHAIN_ID } from "@/utils/network";

interface UseTransactionStateProps {
  onSuccess?: () => Promise<void>;
  onError?: (error: any) => void;
}

export const useTransactionState = ({
  onSuccess,
  onError,
}: UseTransactionStateProps = {}) => {
  const [isProcessing, setIsProcessing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const startProcessing = useCallback(() => {
    setIsProcessing(true);
    setError(null);
  }, []);

  const stopProcessing = useCallback(() => {
    setIsProcessing(false);
  }, []);

  const handleTransaction = useCallback(
    async (
      transactionFunction: () => Promise<`0x${string}`>,
      afterFunction?: () => Promise<void>
    ) => {
      startProcessing();
      let loadingToast: string | undefined;

      try {
        const tx = await transactionFunction();

        loadingToast = toast.loading("Processing transaction...");

        if (tx !== "0x0") {
          // Only wait for receipt if it's a real transaction
          const receipt = await waitForTransactionReceipt(config.wagmiConfig, {
            chainId: CHAIN_ID,
            hash: tx,
          });

          if (receipt.status !== "success") {
            throw new Error("Transaction failed");
          }
        }

        await afterFunction?.();
        await onSuccess?.();

        toast.success("Operation completed successfully");
        return true;
      } catch (err: any) {
        const errorMessage = err.message || "Operation failed";
        setError(errorMessage);
        toast.error(`Error: ${errorMessage}`);

        onError?.(err);

        return false;
      } finally {
        stopProcessing();
        if (loadingToast) {
          toast.dismiss(loadingToast);
        }
      }
    },
    [onSuccess, onError, startProcessing, stopProcessing]
  );

  return {
    isProcessing,
    error,
    handleTransaction,
    startProcessing,
    stopProcessing,
  };
};
