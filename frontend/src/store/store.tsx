"use client";

import React, { createContext, useContext, useState, useEffect } from "react";
import {
  getCookieJSON,
  setCookieJSON,
} from "../config/aws-s3/cookie-management/store.helpers";

const initialState = {
  account: "",
  balance: null,
  isConnected: false,
};

type StoreState = typeof initialState;

type StoreContextType = StoreState & {
  setStore: React.Dispatch<React.SetStateAction<StoreState>>;
};

// Initialize context with default values to avoid null during SSR
const defaultContextValue: StoreContextType = {
  ...initialState,
  setStore: () => null, // This will be replaced by the actual setter in the provider
};

const StoreContext = createContext<StoreContextType>(defaultContextValue);

type StoreContextProviderProps = {
  children: React.ReactNode;
};

const StoreContextProvider = ({ children }: StoreContextProviderProps) => {
  // Only run getCookieJSON on the client side
  const [store, setStore] = useState<StoreState>(() => {
    if (typeof window === 'undefined') return initialState;
    return getCookieJSON<StoreState>("persist-web3-login") || initialState;
  });

  useEffect(() => {
    // Only run setCookieJSON on the client side
    if (typeof window !== 'undefined') {
      setCookieJSON("persist-web3-login", store, 7);
    }
  }, [store]);

  const contextValue = React.useMemo(() => ({
    ...store,
    setStore,
  }), [store]);

  return (
    <StoreContext.Provider value={contextValue}>
      {children}
    </StoreContext.Provider>
  );
};

const useStoreContext = (): StoreContextType => {
  const context = useContext(StoreContext);
  return context; // No need to check for null since we provide default values
};

export { StoreContextProvider, useStoreContext, initialState };
