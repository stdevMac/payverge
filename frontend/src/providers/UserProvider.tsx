"use client";
import { useEffect, useCallback, useRef } from "react";
import { useAccount } from "wagmi";
import { useUserStore } from "@/store/useUserStore";
import { getUserProfile } from "@/api/users/profile";
import { getCookie } from "@/config/aws-s3/cookie-management/store.helpers";
import { destroyCookie } from "nookies";
import { SIWE_VERIFIED_EVENT } from "@/config/aws-s3/siwe-config/siweEvents";
import { useRouter } from "next/navigation";
import { decodeJwt, isTokenValid } from "@/utils/jwt";
import { useLanguage } from "@/i18n/useLanguage";

interface SessionToken {
  address: string;
  exp: number;
  role: string;
}

export function UserProvider({ children }: { children: React.ReactNode }) {
  const { address, isConnected } = useAccount();
  const { setUser, clearUser, user } = useUserStore();
  const router = useRouter();
  const { setLanguage } = useLanguage();
  
  // Track the last fetch time and address to prevent duplicate requests
  const lastFetchRef = useRef<{ time: number; address: string | null }>({ time: 0, address: null });
  const fetchTimeoutRef = useRef<NodeJS.Timeout>();
  const isInitializedRef = useRef(false);

  // Check if we have a valid session without making API calls
  const hasValidSession = useCallback(() => {
    const token = getCookie("session_token");
    if (!token) return false;
    
    try {
      return isTokenValid(token);
    } catch (error) {
      console.error("Error validating session:", error);
      return false;
    }
  }, []);

  // Clear invalid session data
  const clearInvalidSession = useCallback(() => {
    destroyCookie(null, "token", { path: "/" });
    destroyCookie(null, "session_token", { path: "/" });
    destroyCookie(null, "persist-web3-login", { path: "/" });
    clearUser();
  }, [clearUser]);

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

      // Validate session token first
      if (!token || !isTokenValid(token)) {
        console.log('[UserProvider] No valid session token, clearing user data');
        clearInvalidSession();
        return;
      }

      // Only fetch if:
      // 1. We're forcing a fetch OR
      // 2. We don't have a user OR 
      // 3. The addresses don't match OR
      // 4. This is the first initialization
      if (!force && currentUser?.address === userAddress && isInitializedRef.current) {
        console.log('[UserProvider] User data already current, skipping fetch');
        return;
      }

      try {
        console.log('[UserProvider] Fetching user profile for:', userAddress);
        const userData = await getUserProfile(userAddress);
        console.log('[UserProvider] API response:', userData);
        if (userData) {
          // Get role from session token
          const tokenData = decodeJwt(token) as SessionToken;
          // Override the role from API with the one from session token
          userData.role = tokenData.role;
          
          // Set user data first
          setUser(userData);
          
          // Mark as initialized
          isInitializedRef.current = true;

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
        } else {
          console.log('[UserProvider] No user data returned from API');
        }
      } catch (error) {
        console.error('[UserProvider] Failed to fetch user profile:', error);
      }
    },
    [setUser, setLanguage, clearInvalidSession]
  );

  // Initialize session on app startup
  useEffect(() => {
    const initializeSession = async () => {
      // Only initialize once
      if (isInitializedRef.current) return;
      
      console.log('[UserProvider] Initializing session...');
      
      // Check if we have a valid session token
      const token = getCookie("session_token");
      if (!token || !isTokenValid(token)) {
        console.log('[UserProvider] No valid session token found during initialization');
        return;
      }

      // If we have a valid token but no connected wallet, don't fetch yet
      if (!isConnected || !address) {
        console.log('[UserProvider] Valid token found but wallet not connected yet');
        return;
      }

      // Validate that the token address matches the connected address
      try {
        const tokenData = decodeJwt(token);
        if (tokenData.address && tokenData.address.toLowerCase() === address.toLowerCase()) {
          console.log('[UserProvider] Session token matches connected wallet, fetching user data');
          await fetchUserProfile(address, true);
        } else {
          console.log('[UserProvider] Session token address mismatch, clearing session');
          clearInvalidSession();
        }
      } catch (error) {
        console.error('[UserProvider] Error validating session token:', error);
        clearInvalidSession();
      }
    };

    initializeSession();
  }, [isConnected, address, fetchUserProfile, clearInvalidSession]);

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
      "/referrals",
      "/referrals/buy",
      "/referrals/dashboard",
      "/b",
      "/t",
    ];
    const currentPath = window.location.pathname;
    const isPublicRoute = publicRoutes.some((route) =>
      currentPath.startsWith(route)
    );

    const handleConnection = async () => {
      if (!isConnected || !address) {
        console.log('[UserProvider] Wallet disconnected, clearing user data');
        clearInvalidSession();
        isInitializedRef.current = false;
        if (!isPublicRoute) {
          router.push("/");
        }
        return;
      }

      // Check if we have a valid session
      if (!hasValidSession()) {
        console.log('[UserProvider] No valid session found');
        // If we have a user in store but no valid session, clear it
        if (user) {
          console.log('[UserProvider] Clearing stale user data');
          clearInvalidSession();
          isInitializedRef.current = false;
        }
        return;
      }

      // If we have a valid session and the address matches, fetch user data
      if (mounted) {
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
  }, [address, isConnected, fetchUserProfile, router, clearInvalidSession, hasValidSession, user]);

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
