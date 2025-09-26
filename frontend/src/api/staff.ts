import { axiosInstance } from "@/api";
import axios from "axios";

// Types
export interface StaffMember {
  id: number;
  name: string;
  email: string;
  role: 'manager' | 'server' | 'host' | 'kitchen';
  business_id: number;
  created_at: string;
  updated_at: string;
  last_login_at?: string;
}

export interface StaffInvitation {
  id: number;
  email: string;
  name: string;
  role: 'manager' | 'server' | 'host' | 'kitchen';
  business_id: number;
  token: string;
  expires_at: string;
  created_at: string;
  status?: 'pending' | 'accepted' | 'expired' | 'revoked';
}

export interface InviteStaffRequest {
  email: string;
  name: string;
  role: 'manager' | 'server' | 'host' | 'kitchen';
}

export interface StaffResponse {
  staff: StaffMember[];
  pending_invitations: StaffInvitation[];
}

export interface InviteStaffResponse {
  message: string;
  invitation_id: number;
  expires_at: string;
}

export interface AcceptInvitationRequest {
  token: string;
  name: string;
}

export interface LoginCodeRequest {
  email: string;
}

export interface VerifyCodeRequest {
  email: string;
  code: string;
}

export interface StaffLoginResponse {
  token: string;
  staff: StaffMember;
}

// Staff Management API (Business Owner Functions)
export const inviteStaff = async (businessId: string, data: InviteStaffRequest): Promise<InviteStaffResponse> => {
  try {
    const response = await axiosInstance.post(`/inside/businesses/${businessId}/staff/invite`, data);
    return response.data;
  } catch (error) {
    if (axios.isAxiosError(error)) {
      throw new Error(error.response?.data?.error || 'Failed to invite staff');
    }
    throw error;
  }
};

export const getBusinessStaff = async (businessId: string): Promise<StaffResponse> => {
  try {
    const response = await axiosInstance.get(`/inside/businesses/${businessId}/staff`);
    return response.data;
  } catch (error) {
    if (axios.isAxiosError(error)) {
      throw new Error(error.response?.data?.error || 'Failed to get business staff');
    }
    throw error;
  }
};

export const removeStaff = async (businessId: string, staffId: number): Promise<{ message: string }> => {
  try {
    const response = await axiosInstance.delete(`/inside/businesses/${businessId}/staff/${staffId}`);
    return response.data;
  } catch (error) {
    if (axios.isAxiosError(error)) {
      throw new Error(error.response?.data?.error || 'Failed to remove staff');
    }
    throw error;
  }
};

export const resendInvitation = async (businessId: string, invitationId: number): Promise<InviteStaffResponse> => {
  try {
    const response = await axiosInstance.post(`/inside/businesses/${businessId}/staff/invitations/${invitationId}/resend`);
    return response.data;
  } catch (error) {
    if (axios.isAxiosError(error)) {
      throw new Error(error.response?.data?.error || 'Failed to resend invitation');
    }
    throw error;
  }
};

// Staff Authentication API (Public Functions)
export const acceptInvitation = async (data: AcceptInvitationRequest): Promise<StaffLoginResponse> => {
  try {
    const response = await axiosInstance.post('/staff/accept-invitation', data);
    return response.data;
  } catch (error) {
    if (axios.isAxiosError(error)) {
      throw new Error(error.response?.data?.error || 'Failed to accept invitation');
    }
    throw error;
  }
};

export const requestLoginCode = async (data: LoginCodeRequest): Promise<{ message: string }> => {
  try {
    const response = await axiosInstance.post('/staff/request-login-code', data);
    return response.data;
  } catch (error) {
    if (axios.isAxiosError(error)) {
      throw new Error(error.response?.data?.error || 'Failed to request login code');
    }
    throw error;
  }
};

export const verifyLoginCode = async (data: VerifyCodeRequest): Promise<StaffLoginResponse> => {
  try {
    const response = await axiosInstance.post('/staff/verify-login-code', data);
    return response.data;
  } catch (error) {
    if (axios.isAxiosError(error)) {
      throw new Error(error.response?.data?.error || 'Failed to verify login code');
    }
    throw error;
  }
};
