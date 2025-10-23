import { useState, useEffect, useCallback } from 'react';
import { useAppKitAccount } from '@reown/appkit/react';
import { useUserStore } from '@/store/useUserStore';
import { useAuth } from '@/hooks/useAuth';
import { getCookie } from '@/config/aws-s3/cookie-management/store.helpers';
import { isTokenValid, decodeJwt } from '@/utils/jwt';
import { getUserProfile } from '@/api/users/profile';
import { Business, getBusiness } from '@/api/business';
import { UserInterface as User } from '@/interface/users/users-interface';

export const useBusinessDashboard = (businessId: number) => {
  const [business, setBusiness] = useState<Business | null>(null);
  const [loading, setLoading] = useState(true);
  const [authLoading, setAuthLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showWelcome, setShowWelcome] = useState(false);
  const [isFirstVisit, setIsFirstVisit] = useState(false);

  const { address, isConnected } = useAppKitAccount();
  const { user } = useUserStore();
  const { isAuthenticated } = useAuth();

  // Authentication check
  useEffect(() => {
    const checkAuth = async () => {
      try {
        const token = getCookie("session_token");
        
        if (!token || !isTokenValid(token)) {
          setError("Please sign in to continue.");
          setAuthLoading(false);
          return;
        }

        const tokenData = decodeJwt(token);
        
        if (!tokenData.address) {
          setError("Invalid session token.");
          setAuthLoading(false);
          return;
        }

        const address = tokenData.address.toLowerCase();
        
        if (!user) {
          try {
            const userData = await getUserProfile(address);
            if (userData) {
              userData.role = tokenData.role || 'user';
              useUserStore.getState().setUser(userData);
            } else {
              const tempUser: User = {
                username: "",
                email: "",
                address: address,
                role: tokenData.role || 'user',
                joined_at: new Date().toISOString(),
                language_selected: "en",
                referral_code: "",
                referrer: "",
                referees: {},
                notifications: [],
              };
              useUserStore.getState().setUser(tempUser);
            }
          } catch (apiError) {
            const tempUser: User = {
              username: "",
              email: "",
              address: address,
              role: tokenData.role || 'user',
              joined_at: new Date().toISOString(),
              language_selected: "en",
              referral_code: "",
              referrer: "",
              referees: {},
              notifications: [],
            };
            useUserStore.getState().setUser(tempUser);
          }
        }
        
        setAuthLoading(false);
        setError(null);
      } catch (error) {
        setError("Authentication failed. Please try signing in again.");
        setAuthLoading(false);
      }
    };

    if (isConnected) {
      checkAuth();
    }
  }, [isConnected, user]);

  // Clear error when user successfully logs in
  useEffect(() => {
    if (user && isConnected && error !== null && typeof error === 'string') {
      const hasSignInError = error.indexOf("sign in") !== -1;
      const hasAuthFailedError = error.indexOf("Authentication failed") !== -1;
      if (hasSignInError || hasAuthFailedError) {
        setError(null);
      }
    }
  }, [user, isConnected, error]);

  // Fetch business data
  const fetchBusiness = useCallback(async () => {
    if (!businessId || !user) return;
    
    try {
      setLoading(true);
      const businessData = await getBusiness(businessId);
      setBusiness(businessData);
      
      // Check if this is the user's first visit
      const visitKey = `business_${businessId}_visited`;
      const hasVisited = localStorage.getItem(visitKey);
      
      if (!hasVisited) {
        setIsFirstVisit(true);
        setShowWelcome(true);
        localStorage.setItem(visitKey, 'true');
      }
      
    } catch (error: any) {
      console.error('Error fetching business:', error);
      if (error.message?.includes('403') || error.message?.includes('Forbidden')) {
        setError("You don't have permission to access this business.");
      } else if (error.message?.includes('404') || error.message?.includes('Not Found')) {
        setError("Business not found.");
      } else {
        setError("Failed to load business data. Please try again.");
      }
    } finally {
      setLoading(false);
    }
  }, [businessId, user]);

  useEffect(() => {
    if (!authLoading && user) {
      fetchBusiness();
    }
  }, [authLoading, user, fetchBusiness]);

  const refreshBusiness = useCallback(() => {
    fetchBusiness();
  }, [fetchBusiness]);

  return {
    business,
    loading,
    authLoading,
    error,
    showWelcome,
    isFirstVisit,
    setShowWelcome,
    refreshBusiness,
    user,
    isConnected,
    address
  };
};
