import { useState, useCallback, useEffect } from "react";
import { getUserProfile, updateUser } from "@/api";
import { UserInterface } from "@/interface";
import toast from "react-hot-toast";

export const useProfileData = (userId: string) => {
    const [user, setUser] = useState<UserInterface | null>(null);
    const [loading, setLoading] = useState<boolean>(true);
    const [formData, setFormData] = useState<UserInterface>({
        address: "",
        joined_at: "",
        email: "",
        notifications: [],
        referees: {},
        language_selected: "",
        referral_code: "",
        referrer: "",
        role: "",
        username: "",
    });

    const fetchUser = useCallback(async () => {
        try {
            const userData = await getUserProfile(userId);
            if (userData) {
                setUser(userData);
                setFormData(userData);
            } else {
                console.error("Failed to fetch user data");
            }
        } catch (error) {
            console.error("Error fetching user data:", error);
        } finally {
            setLoading(false);
        }
    }, [userId]);

    const updateUserProfile = async (newData: Partial<UserInterface>) => {
        setLoading(true);
        try {
            const updated = await updateUser({
                address: user?.address || "",
                username: newData.username || formData.username,
                email: user?.email || "",
            });

            if (updated && user) {
                setUser({
                    ...user,
                    ...newData,
                });
                toast.success("Profile updated successfully!");
                return true;
            } else {
                toast.error("Failed to update profile");
                return false;
            }
        } catch (error) {
            console.error("Error updating profile:", error);
            toast.error("Failed to update profile");
            return false;
        } finally {
            setLoading(false);
        }
    };

    const refetchUser = useCallback(async () => {
        try {
            const userData = await getUserProfile(userId);
            if (userData) {
                setUser(userData);
                setFormData(userData);
                return userData;
            }
            return null;
        } catch (error) {
            console.error("Error fetching user data:", error);
            return null;
        }
    }, [userId]);

    useEffect(() => {
        fetchUser();
    }, [fetchUser]);

    return {
        user,
        loading,
        updateUserProfile,
        refetchUser,
        formData,
        setFormData,
    };
};
