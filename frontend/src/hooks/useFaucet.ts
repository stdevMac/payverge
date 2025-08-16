"use client";

import { useState } from "react";
import { checkAndTopUp, TopUpResponse } from "@/api/faucet/checkAndTopUp";
import { useTranslation } from "@/i18n/useTranslation";

interface UseFaucetReturn {
  isLoading: boolean;
  error: string | null;
  response: TopUpResponse | null;
  checkAndTopUpAddress: (address: string) => Promise<TopUpResponse>;
  reset: () => void;
}

/**
 * Hook for checking and topping up an Ethereum address with ETH
 * 
 * @returns {UseFaucetReturn} Object containing loading state, error, response, and functions
 */
export const useFaucet = (): UseFaucetReturn => {
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [response, setResponse] = useState<TopUpResponse | null>(null);
  const { t } = useTranslation();

  /**
   * Checks the balance of an Ethereum address and tops it up if needed
   * 
   * @param address - The Ethereum address to check and potentially top up
   * @returns The response from the faucet service
   */
  const checkAndTopUpAddress = async (address: string): Promise<TopUpResponse> => {
    setIsLoading(true);
    setError(null);
    
    try {
      const result = await checkAndTopUp(address);
      setResponse(result);
      
      // Handle different response statuses
      if (result.status === "error") {
        setError(result.error || t("shared.investmentProcessModal.faucet.unknownError"));
      }
      
      return result;
    } catch (err: any) {
      const errorMessage = err.message || t("shared.investmentProcessModal.faucet.requestFailed");
      setError(errorMessage);
      return {
        status: "error",
        error: errorMessage
      };
    } finally {
      setIsLoading(false);
    }
  };

  /**
   * Resets the hook state
   */
  const reset = () => {
    setIsLoading(false);
    setError(null);
    setResponse(null);
  };

  return {
    isLoading,
    error,
    response,
    checkAndTopUpAddress,
    reset
  };
};
