import { axiosInstance } from "@/api";
import axios from "axios";
import { 
  CheckCodeRequest, 
  CheckCodeResponse, 
  UseCodeRequest, 
  UseCodeResponse 
} from "@/interface/codes";

// Set global timeout for axios requests
axios.defaults.timeout = 15000;

/**
 * Checks if a reward code is valid, unused, and not expired
 * @param req Object containing the code to check
 * @returns Response with code status and details if available
 */
export const checkCode = async (req: CheckCodeRequest): Promise<CheckCodeResponse> => {
  try {
    const response = await axiosInstance.post<CheckCodeResponse>(
      "/inside/check_code",
      {
        code: req.code
      }
    );
    return response.data;
  } catch (error: any) {
    handleApiError(error);
    // Instead of throwing the error, return a response indicating an error
    if (error.response && error.response.status === 404) {
      return {
        status: "invalid",
        message: "Invalid code",
        code: undefined,
        amount: undefined
      };
    }
    // For other errors, still return a response to prevent UI crashes
    return {
      status: "error",
      message: error.message || "Error checking code",
      error: error.message || "Error checking code"
    };
  }
};

/**
 * Redeems a valid code to receive a USDC transfer
 * @param req Object containing the code and Ethereum address
 * @returns Response with transaction details if successful
 */
export const useCode = async (req: UseCodeRequest): Promise<UseCodeResponse> => {
  try {
    const response = await axiosInstance.put<UseCodeResponse>(
      "/inside/use_code",
      {
        code: req.code,
        address: req.address
      }
    );
    return response.data;
  } catch (error: any) {
    handleApiError(error);
    throw error;
  }
};

// Function to handle API errors
function handleApiError(error: any) {
  if (error.response) {
    console.error("Response error:", error.response.data);
  } else if (error.request) {
    console.error("Request error:", error.request);
  } else {
    console.error("Error:", error.message);
  }
}
