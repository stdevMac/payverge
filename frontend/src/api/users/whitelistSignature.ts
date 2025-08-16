import { axiosInstance } from "@/api";
import axios, { AxiosError } from "axios";
import { res } from "pino-std-serializers";

export interface WhitelistSignatureRequest {
    contract_address: string;
    user_address: string;
    amount: string;
}

export interface WhitelistSignatureResponse {
    signature: string;
    nonce: number;
    expiry: number;
}

export interface WhitelistSignatureResult {
    signature: string;
    nonce: number;
    expiry: number;
    success: boolean;
}

export interface WhitelistSubmittedRequest {
    address: string;
    transaction_hash: string;
}

export const whitelistSignature = async (
    data: WhitelistSignatureRequest,
): Promise<WhitelistSignatureResult> => {
    try {
        const response = await axiosInstance.post<WhitelistSignatureResponse>(
            `/inside/whitelist/signature`,
            data,
        );

        return {
            signature: response.data.signature,
            nonce: response.data.nonce,
            expiry: response.data.expiry,
            success: response.status == 200,
        };
    } catch (error: unknown) {
        if (axios.isAxiosError(error)) {
            return {
                success: false,
                signature: "",
                nonce: 0,
                expiry: 0,
            };
        }

        return {
            success: false,
            signature: "",
            nonce: 0,
            expiry: 0,
        };
    }
};

export const whitelistSubmit = async (
    data: WhitelistSubmittedRequest,
): Promise<boolean> => {
    try {
        const response = await axiosInstance.post<WhitelistSignatureResponse>(
            `/inside/whitelist/submit`,
            data,
        );

        return response.status == 200;
    } catch (error: unknown) {
        if (axios.isAxiosError(error)) {
            return false;
        }

        return false;
    }
};
