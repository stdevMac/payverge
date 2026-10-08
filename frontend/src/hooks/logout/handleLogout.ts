"use client";
import { useCallback } from "react";
import { useDisconnect } from "wagmi";
import { StoreMenu } from "@/store";
import { useUserStore } from "@/store/useUserStore";
import { signOutFromSession } from "@/api";
import { apiCache } from "@/utils/cache";
import { logError } from '@/utils/errorLogger';
import { clearAllMutationQueues } from "@/lib/mutationQueue";

export const useLogout = () => {
    const closeMenu = StoreMenu((state) => state.closeSideMenu);
    const { disconnect } = useDisconnect();
    const { clearUser } = useUserStore();

    const logout = useCallback(async () => {
        const clearAllState = async () => {
            // Clear localStorage wagmi state
            localStorage.removeItem('wagmi.wallet');
            localStorage.removeItem('wagmi.connected');
            localStorage.removeItem('wagmi.account');
            localStorage.removeItem('wagmi.network');
            localStorage.removeItem('persist-web3-login');

            // Reset all stores
            clearUser();
            clearAllMutationQueues();

            // Clear API cache
            apiCache.endSession();
        };

        try {
            // First disconnect Wagmi
            disconnect();

            // Call backend signout (clears httpOnly cookies server-side)
            await signOutFromSession();

            // Clear all state
            await clearAllState();

            // Close menu
            closeMenu();

            // Force reload to ensure clean state
            // eslint-disable-next-line @next/next/no-location-assign-relative-destination -- full reload on logout drops wallet and client state
            window.location.href = '/';
        } catch (error) {
            void logError(error instanceof Error ? error : String(error), 'useLogout', 'logout');
            console.error("Error during logout:", error);
            // Still proceed with cleanup
            disconnect();
            await clearAllState();
            closeMenu();
            // eslint-disable-next-line @next/next/no-location-assign-relative-destination -- full reload on logout drops wallet and client state
            window.location.href = '/';
        }
    }, [disconnect, closeMenu, clearUser]);

    return { logout };
};
