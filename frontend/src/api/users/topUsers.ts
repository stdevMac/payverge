import { axiosInstance } from "@/api";
import { TopUserInterface } from "@/interface";
import axios, { AxiosError } from "axios";

axios.defaults.timeout = 15000;

// getTopUsers gets the top 10 users with the most points
export const getTopUsers = async (): Promise<TopUserInterface[]> => {
    try {
        const response =
            await axiosInstance.get<TopUserInterface[]>(`/top_users`);

        // Verify if the response has the status code 200 (OK)
        if (response.status === 200) {
            return response.data;
        } else {
            return [];
        }
    } catch (error: unknown) {
        throw error;
    }
};
