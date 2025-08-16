import { axiosInstance } from "@/api";
import axios, { AxiosError } from "axios";

export interface AddRefereeInterface {
    address: string;
    referrer_code: string;
}

export interface ReferralResponse {
    message?: string;
    error?: string;
}

export interface ReferralResult {
    success: boolean;
    message: string;
}

export const set_referrer = async (
    data: AddRefereeInterface,
): Promise<ReferralResult> => {
    try {
        const response = await axiosInstance.post<ReferralResponse>(
            `/inside/set_referrer`,
            data,
        );

        return {
            success: response.status == 200,
            message: response.data.error || "Referral set successfully",
        };
    } catch (error: unknown) {
        if (axios.isAxiosError(error)) {
            const axiosError = error as AxiosError<ReferralResponse>;
            return {
                success: false,
                message:
                    axiosError.response?.data?.error ||
                    "An error occurred. Please try again.",
            };
        }

        return {
            success: false,
            message: "An unexpected error occurred. Please try again.",
        };
    }
};
