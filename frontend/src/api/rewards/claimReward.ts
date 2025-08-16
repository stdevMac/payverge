import { axiosInstance } from "@/api";
import axios from "axios";

axios.defaults.timeout = 15000;

export interface RewardInterface {
    address: string;
    code: string;
}

export interface ClaimRewardResponse {
    message: string;
    points: number;
}

export const claim_reward = async (
    data: RewardInterface,
): Promise<ClaimRewardResponse | null> => {
    try {
        const response = await axiosInstance.post<ClaimRewardResponse>(
            `/inside/claim_reward`,
            data,
        );

        if (response.status === 200) {
            return response.data;
        } else {
            return null;
        }
    } catch (error: unknown) {
        return null;
    }
};
