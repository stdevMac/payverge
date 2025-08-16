"use client";
import { useEffect, useCallback, useRef } from "react";
import { useAccount } from "wagmi";
import { useUserStore } from "@/store/useUserStore";
import { getUserProfile } from "@/api/users/profile";
import { getCookie } from "@/config/aws-s3/cookie-management/store.helpers";
import { destroyCookie } from "nookies";
import { SIWE_VERIFIED_EVENT } from "@/config/aws-s3/siwe-config/siweEvents";
import { useRouter } from "next/navigation";
import { decodeJwt } from "@/utils/jwt";
import { useLanguage } from "@/i18n/useLanguage";

interface SessionToken {
  address: string;
  exp: number;
  role: string;
}

export function UserProvider({ children }: { children: React.ReactNode }) {
  const { address, isConnected } = useAccount();
  const { setUser, clearUser } = useUserStore();
  const router = useRouter();
  const { setLanguage } = useLanguage();
  
  // Track the last fetch time and address to prevent duplicate requests
  const lastFetchRef = useRef<{ time: number; address: string | null }>({ time: 0, address: null });
  const fetchTimeoutRef = useRef<NodeJS.Timeout>();

  const fetchUserProfile = useCallback(
    async (userAddress: string, force: boolean = false) => {
      // Prevent duplicate requests within 2 seconds unless forced
      const now = Date.now();
      if (!force && 
          lastFetchRef.current.address === userAddress && 
          now - lastFetchRef.current.time < 2000) {
        return;
      }
      
      // Update last fetch time and address
      lastFetchRef.current = { time: now, address: userAddress };
      const token = getCookie("session_token");
      const currentUser = useUserStore.getState().user;

      // Only fetch if:
      // 1. We have a session token
      // 2. Either we're forcing a fetch OR we don't have a user OR the addresses don't match
      if (!token || (!force && currentUser?.address === userAddress)) {
        return;
      }

      try {
        const userData = await getUserProfile(userAddress);
        if (userData) {
          // Get role from session token
          const tokenData = decodeJwt(token) as SessionToken;
          // Override the role from API with the one from session token
          userData.role = tokenData.role;

          console.log('[UserProvider] Got user data:', { 
            address: userData.address, 
            language: userData.language_selected 
          });
          
          // Set user data first
          setUser(userData);

          // Update language if user has a preference
          if (userData.language_selected && 
              (userData.language_selected === 'en' || userData.language_selected === 'es')) {
            console.log('[UserProvider] Setting language to:', userData.language_selected);
            // Force language update by updating localStorage first
            localStorage.setItem('language', userData.language_selected);
            setLanguage(userData.language_selected);
          } else {
            console.log('[UserProvider] No valid language preference found in user data');
          }
        }
      } catch (error) {
        console.error('Failed to fetch user profile:', error);
      }
    },
    [setUser, setLanguage]
  );

  // Handle disconnects and address changes
  useEffect(() => {
    let mounted = true;

    // Clear any pending fetch timeout
    if (fetchTimeoutRef.current) {
      clearTimeout(fetchTimeoutRef.current);
    }

    const publicRoutes = [
      "/referee",
      "/profile",
      "/fleet",
      "/car",
      "/mission",
      "/team",
      "/how-it-works",
      "/terms",
      "/privacy",
      "/drop",
    ];
    const currentPath = window.location.pathname;
    const isPublicRoute = publicRoutes.some((route) =>
      currentPath.startsWith(route)
    );

    const handleConnection = async () => {
      if (!isConnected || !address) {
        clearUser();
        destroyCookie(null, "token");
        destroyCookie(null, "session_token");
        destroyCookie(null, "persist-web3-login");
        if (!isPublicRoute) {
          router.push("/");
        }
        return;
      }

      // Only fetch if we have a session token
      const token = getCookie("session_token");
      if (token && mounted) {
        // Debounce the fetch call
        if (fetchTimeoutRef.current) {
          clearTimeout(fetchTimeoutRef.current);
        }
        fetchTimeoutRef.current = setTimeout(() => {
          fetchUserProfile(address, false);
        }, 100);
      }
    };

    handleConnection();

    return () => {
      mounted = false;
      if (fetchTimeoutRef.current) {
        clearTimeout(fetchTimeoutRef.current);
      }
    };
  }, [address, isConnected, fetchUserProfile, router, clearUser]);

  useEffect(() => {
    const handleSiweVerified = () => {
      if (address) {
        // Clear any pending fetch timeout
        if (fetchTimeoutRef.current) {
          clearTimeout(fetchTimeoutRef.current);
        }
        // Force immediate fetch for SIWE verification
        fetchUserProfile(address, true);
      }
    };

    window.addEventListener(SIWE_VERIFIED_EVENT, handleSiweVerified);
    return () => {
      window.removeEventListener(SIWE_VERIFIED_EVENT, handleSiweVerified);
    };
  }, [address, fetchUserProfile]);

  return <>{children}</>;
}
