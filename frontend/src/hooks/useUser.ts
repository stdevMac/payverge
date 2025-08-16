import { useAccount } from "wagmi";
import { useUserStore } from "@/store/useUserStore";
import { UserInterface } from "@/interface";

export const useUser = () => {
    const { isConnected } = useAccount();
    const { user, clearUser } = useUserStore();

    return {
        user: isConnected && user ? (user as UserInterface) : null,
        isLoading: isConnected && !user,
        isAuthenticated: isConnected && !!user,
        logout: clearUser,
    };
};
