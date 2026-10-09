// Staff profile API client
import { axiosInstance } from "@/api/tools/instance";
import { StaffSession } from '@/utils/staffAuth';
import { getSafeApiErrorMessage } from "@/utils/apiError";

// Get current staff profile
export const getStaffProfile = async (): Promise<StaffSession> => {
  try {
    const response = await axiosInstance.get('/staff/profile');
    return response.data;
  } catch (error: unknown) {
    const axiosError = error as { response?: { status?: number; data?: { error?: string } }; message?: string };
    // Preserve the HTTP status on the thrown error so callers can tell a real
    // auth failure (401) apart from a transient network/interceptor error and
    // avoid tearing down a still-valid session.
    const wrapped = new Error(
      getSafeApiErrorMessage(error, "Failed to fetch staff profile"),
    ) as Error & { status?: number };
    wrapped.status = axiosError.response?.status;
    throw wrapped;
  }
};

// Staff logout (if needed for server-side cleanup)
export const staffLogout = async (): Promise<void> => {
  try {
    await axiosInstance.post('/staff/logout');
  } catch (error: unknown) {
    // Don't throw error for logout - we'll clear local data anyway
    const axiosError = error as { message?: string };
    console.warn('Staff logout API call failed:', axiosError.message);
  }
};
