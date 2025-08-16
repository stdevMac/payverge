import { axiosInstance } from "@/api";
import axios from "axios";
import { 
  CreateCodeRequest, 
  CreateCodeResponse, 
  GetCodeRequest, 
  GetCodeResponse, 
  UpdateCodeRequest, 
  UpdateCodeResponse, 
  DeleteCodeRequest, 
  DeleteCodeResponse,
  GetAllCodesResponse
} from "@/interface/codes";

// Set global timeout for axios requests
axios.defaults.timeout = 15000;

/**
 * Generates a new code for USDC reward
 * @param req Object containing amount and optional expiry date
 * @returns Response with the generated code
 */
export const createCode = async (req: CreateCodeRequest): Promise<CreateCodeResponse> => {
  try {
    const response = await axiosInstance.post<CreateCodeResponse>(
      "/admin/create_code",
      {
        amount: req.amount,
        expiry: req.expiry
      }
    );
    return response.data;
  } catch (error: any) {
    handleApiError(error);
    throw error;
  }
};

/**
 * Retrieves details about a specific code
 * @param req Object containing the code to retrieve
 * @returns Response with code details
 */
export const getCode = async (req: GetCodeRequest): Promise<GetCodeResponse> => {
  try {
    const response = await axiosInstance.get<GetCodeResponse>(
      "/admin/get_code",
      {
        data: {
          code: req.code
        }
      }
    );
    return response.data;
  } catch (error: any) {
    handleApiError(error);
    throw error;
  }
};

/**
 * Updates an existing code's attributes
 * @param req Object containing the code and attributes to update
 * @returns Response indicating success or failure
 */
export const updateCode = async (req: UpdateCodeRequest): Promise<UpdateCodeResponse> => {
  try {
    const response = await axiosInstance.put<UpdateCodeResponse>(
      "/admin/update_code",
      {
        code: req.code,
        amount: req.amount,
        expiry: req.expiry,
        used: req.used
      }
    );
    return response.data;
  } catch (error: any) {
    handleApiError(error);
    throw error;
  }
};

/**
 * Deletes a specific code
 * @param req Object containing the code to delete
 * @returns Response indicating success or failure
 */
export const deleteCode = async (req: DeleteCodeRequest): Promise<DeleteCodeResponse> => {
  try {
    const response = await axiosInstance.delete<DeleteCodeResponse>(
      "/admin/delete_code",
      {
        data: {
          code: req.code
        }
      }
    );
    return response.data;
  } catch (error: any) {
    handleApiError(error);
    throw error;
  }
};

/**
 * Retrieves a list of all codes in the system
 * @returns Array of code details or error object
 */
export const getAllCodes = async (): Promise<GetAllCodesResponse> => {
  try {
    const response = await axiosInstance.get<GetAllCodesResponse>(
      "/admin/get_all_codes"
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
