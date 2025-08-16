import { axiosInstance } from "@/api";
import { UserInterface } from "@/interface";

// Function to get user profile
export const getUserProfile = async (
    address: string,
): Promise<UserInterface | null> => {
    try {
        const response = await axiosInstance.get<UserInterface>(
            `/inside/get_user/${address}`,
        );
        return response.status === 200 ? response.data : null;
    } catch (error) {
        // @ts-ignore
        if (error.response?.status === 401) {
            return null;
        }
        console.error("Error getting user profile:", error);
        return null;
    }
};
