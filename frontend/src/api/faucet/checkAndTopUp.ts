import { axiosInstance } from "../tools/instance";

export interface TopUpResponse {
  status: string;
  tx_hash?: string;
  to_address?: string;
  amount_sent?: string;
  balance?: string;
  error?: string;
}

/**
 * Checks if an Ethereum address needs ETH and tops it up if needed
 * @param address The Ethereum address to check and potentially top up
 * @returns Response with status and transaction details if topped up
 */
export const checkAndTopUp = async (address: string): Promise<TopUpResponse> => {
  try {
    const response = await axiosInstance.post("/inside/faucet", {
      address,
    });
    return response.data;
  } catch (error: any) {
    // Handle different error responses
    if (error.response) {
      // The request was made and the server responded with a status code
      // that falls out of the range of 2xx
      return error.response.data;
    } else {
      // Something happened in setting up the request that triggered an Error
      return {
        status: "error",
        error: error.message || "Failed to check and top up address",
      };
    }
  }
};
