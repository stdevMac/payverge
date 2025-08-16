import { axiosInstance } from "@/api";
import { SubscriberInterface } from "@/interface/subscribers/subscriber-interface";

export const getAllSubscribers = async (): Promise<SubscriberInterface[]> => {
    try {
        const response = await axiosInstance.get<SubscriberInterface[]>(
            "/admin/get_subscribers",
        );
        if (response.status === 200) {
            return response.data;
        } else {
            return [];
        }
    } catch (error: any) {
        console.error("Error fetching subscribers:", error);
        return [];
    }
};
