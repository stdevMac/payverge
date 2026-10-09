import { axiosInstance } from "@/api/tools/instance";
import { apiErrorDetail, getApiErrorCode } from "@/utils/apiError";

/**
 * Wave 4: rethrow API failures WITHOUT flattening. The shared axiosInstance
 * rejects with a sanitized Error carrying response.data.{error,code,params};
 * getLocalizedApiError / useApiErrorMessage need that structure to localize
 * and to route coded flows (invalid vs expired login code). Flattening into
 * `new Error(string)` — the old pattern here — silently degraded every staff
 * toast to the generic message. Non-API throws (a real TypeError, an offline
 * axios reject without response) get the descriptive fallback.
 */
function rethrowStaffApiError(error: unknown, fallback: string): never {
  if (apiErrorDetail(error) || getApiErrorCode(error)) {
    throw error;
  }
  throw new Error(fallback);
}

// Types
export interface StaffMember {
  id: number;
  name: string;
  email: string;
  role: "manager" | "server" | "host" | "kitchen";
  business_id: number;
  is_active: boolean;
  created_at: string;
  updated_at: string;
  last_login_at?: string;
}

export interface StaffInvitation {
  id: number;
  email: string;
  name: string;
  role: "manager" | "server" | "host" | "kitchen";
  business_id: number;
  // Tokens are never returned on staff list responses (staff:read is held by
  // kitchen/server/host). Use getInvitationLink / invite / resend for URLs.
  token?: string;
  expires_at: string;
  created_at: string;
  status?: "pending" | "accepted" | "expired" | "revoked";
}

export interface InviteStaffRequest {
  email: string;
  name: string;
  role: "manager" | "server" | "host" | "kitchen";
}

export interface StaffResponse {
  staff: StaffMember[];
  pending_invitations: StaffInvitation[];
}

export interface InviteStaffResponse {
  message: string;
  invitation_id: number;
  expires_at: string;
  // P2-21: false when the invite email failed or email is unconfigured; the
  // inviter should hand invitation_url to the invitee out-of-band.
  email_sent?: boolean;
  invitation_url?: string;
}

export interface AcceptInvitationRequest {
  token: string;
  name: string;
}

export interface StaffInvitationPreviewResponse {
  email: string;
  name: string;
  role: StaffMember["role"];
  business_id: number;
  business_name?: string;
  business_custom_url?: string;
  /** Operator-route identifier (businesses.business_id); custom_url is storefront-only. */
  business_slug?: string;
  expires_at: string;
}

export interface LoginCodeRequest {
  email: string;
  business_id?: number;
}

export type VerifyCodeRequest =
  | { email: string; code: string; business_id?: number }
  | { selection_token: string; business_id: number };

export interface StaffLoginResponse {
  token: string;
  staff: StaffMember;
  role?: StaffMember["role"];
  business_name?: string;
}

export interface StaffMembershipChoice {
  business_id: number;
  business_name: string;
  role: StaffMember["role"];
}

export interface StaffMembershipSelectionResponse {
  message: string;
  membership_selection_required: true;
  selection_token: string;
  memberships: StaffMembershipChoice[];
}

export type VerifyLoginCodeResponse =
  | StaffLoginResponse
  | StaffMembershipSelectionResponse;

export const isMembershipSelectionResponse = (
  response: VerifyLoginCodeResponse,
): response is StaffMembershipSelectionResponse =>
  "membership_selection_required" in response &&
  response.membership_selection_required === true;

export interface StaffOAuthURLResponse {
  url: string;
  state: string;
}

// Staff Management API (Business Owner Functions)
export const inviteStaff = async (
  businessId: string,
  data: InviteStaffRequest,
): Promise<InviteStaffResponse> => {
  try {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/staff/invite`,
      data,
    );
    return response.data;
  } catch (error) {
    rethrowStaffApiError(error, "Failed to invite staff");
  }
};

export const getBusinessStaff = async (
  businessId: string,
): Promise<StaffResponse> => {
  try {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/staff`,
    );
    return response.data;
  } catch (error) {
    rethrowStaffApiError(error, "Failed to get business staff");
  }
};

export const removeStaff = async (
  businessId: string,
  staffId: number,
): Promise<{ message: string }> => {
  try {
    const response = await axiosInstance.delete(
      `/inside/businesses/${businessId}/staff/${staffId}`,
    );
    return response.data;
  } catch (error) {
    rethrowStaffApiError(error, "Failed to remove staff");
  }
};

export const resendInvitation = async (
  businessId: string,
  invitationId: number,
): Promise<InviteStaffResponse> => {
  try {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/staff/invitations/${invitationId}/resend`,
    );
    return response.data;
  } catch (error) {
    rethrowStaffApiError(error, "Failed to resend invitation");
  }
};

// staff:invite-gated: re-materialize the accept URL for copy-link without
// rotating the token or sending email.
export const getInvitationLink = async (
  businessId: string,
  invitationId: number,
): Promise<{
  invitation_id: number;
  invitation_url: string;
  expires_at: string;
}> => {
  try {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/staff/invitations/${invitationId}/link`,
    );
    return response.data;
  } catch (error) {
    rethrowStaffApiError(error, "Failed to get invitation link");
  }
};

// Revoke a pending/expired invitation: the server rotates its token (the original
// link dies) and marks it revoked. An accepted invitation is rejected server-side.
export const revokeInvitation = async (
  businessId: string,
  invitationId: number,
): Promise<{ message: string; invitation_id: number }> => {
  try {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/staff/invitations/${invitationId}/revoke`,
    );
    return response.data;
  } catch (error) {
    rethrowStaffApiError(error, "Failed to revoke invitation");
  }
};

// Staff Authentication API (Public Functions)
export const acceptInvitation = async (
  data: AcceptInvitationRequest,
): Promise<StaffLoginResponse> => {
  try {
    const response = await axiosInstance.post("/staff/accept-invitation", data);
    return response.data;
  } catch (error) {
    rethrowStaffApiError(error, "Failed to accept invitation");
  }
};

export const getInvitationPreview = async (
  token: string,
): Promise<StaffInvitationPreviewResponse> => {
  try {
    const response = await axiosInstance.get("/staff/invitation-preview", {
      params: { token },
    });
    return response.data;
  } catch (error) {
    rethrowStaffApiError(error, "Failed to validate invitation");
  }
};

export const requestLoginCode = async (
  data: LoginCodeRequest,
): Promise<{ message: string }> => {
  try {
    const response = await axiosInstance.post(
      "/staff/request-login-code",
      data,
    );
    return response.data;
  } catch (error) {
    rethrowStaffApiError(error, "Failed to request login code");
  }
};

export const verifyLoginCode = async (
  data: VerifyCodeRequest,
): Promise<VerifyLoginCodeResponse> => {
  try {
    const response = await axiosInstance.post("/staff/verify-login-code", data);
    return response.data;
  } catch (error) {
    rethrowStaffApiError(error, "Failed to verify login code");
  }
};

export const getStaffGoogleAuthURL = async (
  redirect?: string,
): Promise<StaffOAuthURLResponse> => {
  try {
    const response = await axiosInstance.get("/auth/google/staff", {
      params: redirect ? { redirect } : undefined,
    });
    return response.data;
  } catch (error) {
    rethrowStaffApiError(error, "Failed to initiate Google login");
  }
};
