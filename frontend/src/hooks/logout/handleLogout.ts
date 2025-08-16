"use client";
import { useCallback } from "react";
import { useRouter } from "next/navigation";
import { useStoreContext } from "@/store/store";
import { destroyCookie } from "nookies";
import { useDisconnect } from "wagmi";
import { StoreMenu } from "@/store";
import { useUserStore } from "@/store/useUserStore";
import { signOutFromSession } from "@/api";
import { siweConfig } from "@/config/aws-s3/siwe-config/useSiweConfig";
import { useAppKit } from "@reown/appkit/react";

export const useLogout = () => {
    const closeMenu = StoreMenu((state) => state.closeSideMenu);
    const { disconnect } = useDisconnect();
    const { setStore } = useStoreContext();
    const { clearUser } = useUserStore();
    const { close: closeWeb3Modal } = useAppKit();
    const router = useRouter();

    const logout = useCallback(async () => {
        const clearAllState = async () => {
            // Clear all cookies
            destroyCookie(null, "token", { path: "/" });
            destroyCookie(null, "session_token", { path: "/" });
            destroyCookie(null, "persist-web3-login", { path: "/" });
            
            // Clear localStorage SIWE state
            localStorage.removeItem('wagmi.wallet');
            localStorage.removeItem('wagmi.connected');
            localStorage.removeItem('wagmi.account');
            localStorage.removeItem('wagmi.network');
            localStorage.removeItem('siwe.session');

            // Reset all stores
            setStore({
                isConnected: false,
                account: "",
                balance: null,
            });
            clearUser();
        };

        try {
            // First disconnect Wagmi
            disconnect();
            
            // Then close Web3Modal
            await closeWeb3Modal();

            // Call backend signout and SIWE signout in parallel
            await Promise.all([
                signOutFromSession(),
                siweConfig().signOut()
            ]);

            // Clear all state
            await clearAllState();

            // Close menu
            closeMenu();

            // Force reload to ensure clean state
            window.location.href = '/';
        } catch (error) {
            console.error("Error during logout:", error);
            // Still proceed with cleanup
            disconnect();
            await clearAllState();
            closeMenu();
            window.location.href = '/';
        }
    }, [disconnect, closeWeb3Modal, closeMenu, setStore, clearUser]);

    return { logout };
};
