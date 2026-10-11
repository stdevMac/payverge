"use client";

import { getPublicConfig } from "@/config/publicConfig";
import React, {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  useRef,
} from "react";
import { crmAPI, Customer } from "@/api/crm";
import { axiosInstance } from "@/api/tools/instance";
import { logError } from "@/utils/errorLogger";
import { startTokenRefreshTimer } from "@/utils/tokenRefresh";
import {
  CUSTOMER_SESSION_EXPIRED_EVENT,
  CUSTOMER_SESSION_HINT_KEY,
  refreshCustomerAuthSession,
  type RefreshCustomerAuthResult,
} from "@/utils/refreshCustomerAuth";
import { clearSentryIdentity, setSentryIdentity } from "@/lib/sentry/reporting";
import SessionTimeoutWarning from "@/components/auth/SessionTimeoutWarning";

interface CustomerAuthContextType {
  customer: Customer | null;
  customerId: number | null;
  isAuthenticated: boolean;
  loading: boolean;
  login: (email: string, password: string) => Promise<Customer>;
  register: (email: string, password: string, name: string) => Promise<Customer>;
  logout: () => Promise<void>;
  refreshCustomer: () => Promise<void>;
}

interface CustomerSessionInfo {
  authenticated: boolean;
  type?: string;
  customer_id?: number;
  email?: string;
  name?: string;
}

const CustomerAuthContext = createContext<CustomerAuthContextType | undefined>(
  undefined,
);

type CustomerSessionHintState = "present" | "absent" | "unavailable";

class CustomerAuthTransitionSuperseded extends Error {
  constructor() {
    super("Customer authentication transition was superseded");
    this.name = "CustomerAuthTransitionSuperseded";
  }
}

const readCustomerSessionHint = (): CustomerSessionHintState => {
  if (typeof window === "undefined") return "unavailable";
  try {
    return window.localStorage.getItem(CUSTOMER_SESSION_HINT_KEY) === "1"
      ? "present"
      : "absent";
  } catch {
    // Storage policy cannot disprove an HttpOnly refresh cookie. The caller
    // performs one bounded cookie-backed refresh before deciding anonymously.
    return "unavailable";
  }
};

const writeCustomerSessionHint = (present: boolean): void => {
  if (typeof window === "undefined") return;
  try {
    if (present) {
      window.localStorage.setItem(CUSTOMER_SESSION_HINT_KEY, "1");
    } else {
      window.localStorage.removeItem(CUSTOMER_SESSION_HINT_KEY);
    }
  } catch {
    // The httpOnly cookie remains authoritative when storage is unavailable.
  }
};

export function CustomerAuthProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const [customer, setCustomer] = useState<Customer | null>(null);
  const [customerId, setCustomerId] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [isSessionExpired, setIsSessionExpired] = useState(false);
  const refreshCleanupRef = useRef<(() => void) | null>(null);
  const hasCustomerSessionRef = useRef(false);
  const authEpochRef = useRef(0);
  const mountedRef = useRef(true);

  const beginAuthTransition = useCallback((): number => {
    authEpochRef.current += 1;
    return authEpochRef.current;
  }, []);

  const isAuthEpochCurrent = useCallback(
    (epoch: number): boolean =>
      mountedRef.current && authEpochRef.current === epoch,
    [],
  );

  // The interceptor's toSanitizedError preserves both `status` and
  // `response.status`; read either so we work with sanitized and raw shapes.
  const statusOf = (error: unknown): number | undefined => {
    if (typeof error !== "object" || error === null) return undefined;
    const e = error as { status?: unknown; response?: { status?: unknown } };
    if (typeof e.status === "number") return e.status;
    if (typeof e.response?.status === "number") return e.response.status;
    return undefined;
  };

  const performRefresh = useCallback(async (): Promise<RefreshCustomerAuthResult> => {
    return refreshCustomerAuthSession(getPublicConfig().apiUrl);
  }, []);

  const clearCustomerState = useCallback(() => {
    const hadCustomerSession = hasCustomerSessionRef.current;
    hasCustomerSessionRef.current = false;
    writeCustomerSessionHint(false);
    setCustomer(null);
    setCustomerId(null);
    // Only clear the customer Sentry identity if one was actually applied, so
    // we don't stomp a higher-priority (staff/user/web3) identity on mount.
    if (hadCustomerSession) {
      clearSentryIdentity("customer");
    }
  }, []);

  const setAuthenticatedCustomer = useCallback((nextCustomer: Customer) => {
    hasCustomerSessionRef.current = true;
    writeCustomerSessionHint(true);
    setCustomer(nextCustomer);
    setCustomerId(nextCustomer.id);
    setSentryIdentity({
      authSource: "customer",
      customerId: nextCustomer.id,
    });
  }, []);

  const stopRefreshTimer = useCallback(() => {
    refreshCleanupRef.current?.();
    refreshCleanupRef.current = null;
  }, []);

  const scheduleRefreshTimer = useCallback(() => {
    stopRefreshTimer();
    setIsSessionExpired(false);
    refreshCleanupRef.current = startTokenRefreshTimer({
      refreshFn: performRefresh,
      onRefreshSuccess: () => {},
      onSessionExpired: () => {
        if (!mountedRef.current) return;
        beginAuthTransition();
        stopRefreshTimer();
        setIsSessionExpired(true);
        setLoading(false);
        clearCustomerState();
      },
    });
  }, [beginAuthTransition, clearCustomerState, performRefresh, stopRefreshTimer]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      beginAuthTransition();
      stopRefreshTimer();
    };
  }, [beginAuthTransition, stopRefreshTimer]);

  // Load customer from auth cookie-backed session on mount.
  useEffect(() => {
    const bootstrapEpoch = authEpochRef.current;
    let disposed = false;
    const isBootstrapCurrent = (): boolean =>
      !disposed && isAuthEpochCurrent(bootstrapEpoch);

    const loadCustomer = async () => {
      try {
        let { data: sessionInfo } =
          await axiosInstance.get<CustomerSessionInfo>(
            "/customer/session-info",
            { _skipAuthRefresh: true },
          );
        if (!isBootstrapCurrent()) return;

        if (!sessionInfo.authenticated) {
          if (readCustomerSessionHint() === "absent") {
            stopRefreshTimer();
            clearCustomerState();
            return;
          }

          const apiUrl = getPublicConfig().apiUrl;
          const refreshResult = await refreshCustomerAuthSession(apiUrl);
          if (!isBootstrapCurrent()) return;

          if (!refreshResult.ok) {
            if (refreshResult.sessionDead) {
              stopRefreshTimer();
              clearCustomerState();
            }
            return;
          }

          const refreshedSession =
            await axiosInstance.get<CustomerSessionInfo>(
              "/customer/session-info",
              { _skipAuthRefresh: true },
            );
          if (!isBootstrapCurrent()) return;

          sessionInfo = refreshedSession.data;
          if (!sessionInfo.authenticated) {
            // A successful refresh followed by an inconclusive probe is not
            // proof of logout. Keep the hint for the next bounded recovery.
            return;
          }
        }

        if (sessionInfo.type !== "customer") {
          stopRefreshTimer();
          clearCustomerState();
          return;
        }

        hasCustomerSessionRef.current = true;
        writeCustomerSessionHint(true);
        const customerData = await crmAPI.getProfile();
        if (!isBootstrapCurrent()) return;

        setAuthenticatedCustomer(customerData);
        scheduleRefreshTimer();
      } catch (error) {
        if (!isBootstrapCurrent()) return;

        void logError(
          error instanceof Error ? error : String(error),
          "CustomerAuth",
          "loadCustomer",
        );
        const status = statusOf(error);
        if (status === 401) {
          // Only an authentication failure proves the cookie-backed session is
          // dead. A 403 may be an origin/policy failure and must preserve state.
          stopRefreshTimer();
          clearCustomerState();
        }
        // A 5xx or transport failure is NOT proof of logout. Preserve any
        // existing customer state and retry on the next mount/refresh path.
      } finally {
        if (isBootstrapCurrent()) {
          setLoading(false);
        }
      }
    };

    loadCustomer().catch((err) => console.error("loadCustomer failed:", err));
    return () => {
      disposed = true;
    };
  }, [clearCustomerState, isAuthEpochCurrent, scheduleRefreshTimer, setAuthenticatedCustomer, stopRefreshTimer]);

  // Clear state on session expiry
  useEffect(() => {
    const handleExpired = () => {
      if (!hasCustomerSessionRef.current) {
        return;
      }

      beginAuthTransition();
      stopRefreshTimer();
      setIsSessionExpired(true);
      setLoading(false);
      clearCustomerState();
    };
    window.addEventListener(CUSTOMER_SESSION_EXPIRED_EVENT, handleExpired);
    return () =>
      window.removeEventListener(CUSTOMER_SESSION_EXPIRED_EVENT, handleExpired);
  }, [beginAuthTransition, clearCustomerState, stopRefreshTimer]);

  const login = useCallback(async (email: string, password: string): Promise<Customer> => {
    const transitionEpoch = beginAuthTransition();
    try {
      const response = await crmAPI.login(email, password);
      if (!isAuthEpochCurrent(transitionEpoch)) {
        throw new CustomerAuthTransitionSuperseded();
      }
      setAuthenticatedCustomer(response.customer);
      scheduleRefreshTimer();
      return response.customer;
    } catch (error) {
      if (error instanceof CustomerAuthTransitionSuperseded) throw error;
      void logError(error instanceof Error ? error : String(error), "CustomerAuth", "login");
      throw error;
    } finally {
      if (isAuthEpochCurrent(transitionEpoch)) {
        setLoading(false);
      }
    }
  }, [beginAuthTransition, isAuthEpochCurrent, scheduleRefreshTimer, setAuthenticatedCustomer]);

  const register = useCallback(async (email: string, password: string, name: string): Promise<Customer> => {
    const transitionEpoch = beginAuthTransition();
    try {
      await crmAPI.register(email, password, name);
      if (!isAuthEpochCurrent(transitionEpoch)) {
        throw new CustomerAuthTransitionSuperseded();
      }
      const response = await crmAPI.login(email, password);
      if (!isAuthEpochCurrent(transitionEpoch)) {
        throw new CustomerAuthTransitionSuperseded();
      }
      setAuthenticatedCustomer(response.customer);
      scheduleRefreshTimer();
      return response.customer;
    } catch (error) {
      if (error instanceof CustomerAuthTransitionSuperseded) throw error;
      void logError(error instanceof Error ? error : String(error), "CustomerAuth", "register");
      throw error;
    } finally {
      if (isAuthEpochCurrent(transitionEpoch)) {
        setLoading(false);
      }
    }
  }, [beginAuthTransition, isAuthEpochCurrent, scheduleRefreshTimer, setAuthenticatedCustomer]);

  const logout = useCallback(async () => {
    const transitionEpoch = beginAuthTransition();
    stopRefreshTimer();
    setIsSessionExpired(false);
    try {
      await crmAPI.logout();
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), "CustomerAuth", "logout");
    }
    if (!isAuthEpochCurrent(transitionEpoch)) return;

    clearCustomerState();
    setLoading(false);
    if (typeof window !== "undefined") {
      // eslint-disable-next-line @next/next/no-location-assign-relative-destination -- full reload on customer logout drops all client state
      window.location.href = "/";
    }
  }, [beginAuthTransition, clearCustomerState, isAuthEpochCurrent, stopRefreshTimer]);

  const refreshCustomer = useCallback(async () => {
    const transitionEpoch = beginAuthTransition();
    try {
      const customerData = await crmAPI.getProfile();
      if (!isAuthEpochCurrent(transitionEpoch)) return;

      setAuthenticatedCustomer(customerData);
      if (!refreshCleanupRef.current) {
        scheduleRefreshTimer();
      }
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), "CustomerAuth", "refreshCustomer");
      const status = statusOf(error);
      if (isAuthEpochCurrent(transitionEpoch) && status === 401) {
        clearCustomerState();
      }
    } finally {
      if (isAuthEpochCurrent(transitionEpoch)) {
        setLoading(false);
      }
    }
  }, [beginAuthTransition, clearCustomerState, isAuthEpochCurrent, scheduleRefreshTimer, setAuthenticatedCustomer]);

  const value: CustomerAuthContextType = {
    customer,
    customerId,
    isAuthenticated: !!customer,
    loading,
    login,
    register,
    logout,
    refreshCustomer,
  };

  return (
    <CustomerAuthContext.Provider value={value}>
      {children}
      <SessionTimeoutWarning
        isExpired={isSessionExpired}
        onLogin={() => {
          if (typeof window !== "undefined") {
            window.location.reload();
          }
        }}
      />
    </CustomerAuthContext.Provider>
  );
}

export function useCustomerAuth() {
  const context = useContext(CustomerAuthContext);
  if (context === undefined) {
    throw new Error(
      "useCustomerAuth must be used within a CustomerAuthProvider",
    );
  }
  return context;
}
