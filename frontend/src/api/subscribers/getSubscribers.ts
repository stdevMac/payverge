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

export const subscribeToWaitlist = async (email: string, businessName: string, message: string): Promise<{ success: boolean; message: string }> => {
    try {
        const subscriptionData = {
            name: "", // Optional - we don't collect name in this form
            email: email,
            phone_number: "", // Optional - we don't collect phone
            company_name: businessName,
            contact_way: "email", // Default to email as preferred contact
            message: message,
            social_media: "", // Optional
            username: "" // Optional
        };

        const response = await axiosInstance.post('/subscribe', subscriptionData);
        
        if (response.status === 200) {
            return { success: true, message: "Thanks for joining our waitlist! We'll be in touch soon." };
        } else {
            return { success: false, message: "Something went wrong. Please try again." };
        }
    } catch (error: any) {
        console.error('Subscription error:', error);
        if (error.response?.data?.error) {
            return { success: false, message: error.response.data.error };
        } else {
            return { success: false, message: "Something went wrong. Please try again." };
        }
    }
};
