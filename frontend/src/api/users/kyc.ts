import { axiosInstance } from "@/api";
import axios, { AxiosError } from "axios";
import { KYCInterface } from "@/interface/users/kyc-interface";

axios.defaults.timeout = 15000;

export const submitKYCToBackend = async (
    data: KYCInterface,
): Promise<boolean> => {
    try {
        const response = await axiosInstance.post(`/inside/kyc_submit`, data);

        if (response.status === 200) {
            return true;
        } else {
            return false;
        }
    } catch (error: unknown) {
        return false;
    }
};
export const approveKYC = async (address: string): Promise<boolean> => {
    try {
        const response = await axiosInstance.post(
            `/admin/approve_kyc/${address}`,
        );

        if (response.status === 200) {
            return true;
        } else {
            return false;
        }
    } catch (error: unknown) {
        return false;
    }
};

export const rejectKYC = async (address: string): Promise<boolean> => {
    try {
        const response = await axiosInstance.post(
            `/admin/reject_kyc/${address}`,
        );

        if (response.status === 200) {
            return true;
        } else {
            return false;
        }
    } catch (error: unknown) {
        return false;
    }
};

export const initiateKYC = async (address: string, email: string) => {
    try {
        const response = await axiosInstance.post("/inside/kyc/start", {
            address,
            email,
        });
        const { applicant_id, websdk_link, message } = response.data;
        // Use the data as needed
        return { applicant_id, websdk_link, message };
    } catch (error) {
        throw error;
    }
};
export const refreshAccessToken = async (address: string) => {
    try {
        const response = await axios.post("/api/kyc/refreshToken", {
            address,
        });
        return response.data; // Should contain new accessToken
    } catch (error) {
        throw error;
    }
};
