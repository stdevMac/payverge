import { useCallback } from 'react';
import { useUserStore } from '@/store/useUserStore';
import { getCookie } from '@/config/aws-s3/cookie-management/store.helpers';
import { isTokenValid } from '@/utils/jwt';

export const useAuth = () => {
  const { user, clearUser } = useUserStore();

  const isAuthenticated = useCallback(() => {
    // Check if we have a user in store
    if (!user) return false;

    // Check if we have a valid session token
    const token = getCookie("session_token");
    if (!token || !isTokenValid(token)) {
      // Token is invalid, clear user data
      clearUser();
      return false;
    }

    return true;
  }, [user, clearUser]);

  const getAuthToken = useCallback(() => {
    const token = getCookie("session_token");
    return token && isTokenValid(token) ? token : null;
  }, []);

  return {
    user,
    isAuthenticated,
    getAuthToken,
    clearUser
  };
};
