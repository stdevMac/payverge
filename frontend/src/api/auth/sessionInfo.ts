import { axiosInstance } from "@/api/tools/instance";

export interface SessionInfo {
  authenticated: boolean;
  type?: "web3" | "user" | "staff" | "customer";
  user_id?: number;
  email?: string;
  address?: string;
  role?: string;
  picture?: string;
  staff_id?: number;
  business_id?: number;
  business_name?: string;
  business_slug?: string;
  staff_name?: string;
  customer_id?: number;
  // Whether the user's email/password auth is verified. Absent for pre-update
  // backends; OAuth/web3 users are reported as provider-verified (true).
  email_verified?: boolean;
}

export async function getSessionInfo(): Promise<SessionInfo> {
  const response = await axiosInstance.get("/auth/session-info", {
    _useCache: false,
    _skipErrorToast: true,
  } as any);
  return response.data;
}
