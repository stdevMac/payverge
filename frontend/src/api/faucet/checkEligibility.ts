import { axiosInstance } from "../tools/instance";

/**
 * Checks if an Ethereum address is eligible for a faucet top-up
 * @param address The Ethereum address to check eligibility
 * @returns Boolean indicating if the address is eligible for a top-up (true) or has sufficient balance (false)
 * @throws Error if the request fails or returns an unexpected status
 */
export const checkEligibility = async (address: string): Promise<boolean> => {
  try {
    const response = await axiosInstance.get(`/inside/faucet/check/${address}`);
    
    if (response.status === 200) {
      if (response.data.status === "eligible") {
        return true;
      } else if (response.data.status === "sufficient_balance") {
        return false;
      }
    }
    
    // If we get here, the response was unexpected
    throw new Error(`Unexpected response: ${JSON.stringify(response.data)}`);
  } catch (error: any) {
    // If the error has a response property, it's an axios error
    if (error.response) {
      throw new Error(`API error: ${error.response.status} - ${JSON.stringify(error.response.data)}`);
    }
    
    // Otherwise, it's a network or other error
    throw error;
  }
};
