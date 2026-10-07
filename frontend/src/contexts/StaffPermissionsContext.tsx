"use client";

import React from "react";
import { useStaffPermissions } from "@/hooks/useStaffPermissions";

type Ctx = {
  permissions: string[];
  rolePermissions: string[];
  customGrants: string[];
  isLoading: boolean;
  isError: boolean;
  refetch: () => void;
};

const StaffPermissionsContext = React.createContext<Ctx | null>(null);

export function StaffPermissionsProvider({
  businessId,
  staffId,
  enabled,
  children,
}: {
  businessId: string;
  staffId?: number;
  enabled: boolean;
  children: React.ReactNode;
}) {
  const q = useStaffPermissions(businessId, staffId, enabled);
  const value: Ctx = {
    permissions: q.data?.permissions ?? [],
    rolePermissions: q.data?.role_permissions ?? [],
    customGrants: q.data?.custom_grants ?? [],
    isLoading: q.isLoading,
    isError: q.isError,
    refetch: () => {
      void q.refetch();
    },
  };
  return (
    <StaffPermissionsContext.Provider value={value}>
      {children}
    </StaffPermissionsContext.Provider>
  );
}

/**
 * Read staff effective permissions from the nearest provider.
 * Outside a provider (or when disabled for owners) returns empty / idle —
 * callers that special-case owners should not rely on these keys alone.
 */
export function useStaffPermissionsContext(): Ctx {
  const ctx = React.useContext(StaffPermissionsContext);
  if (!ctx) {
    return {
      permissions: [],
      rolePermissions: [],
      customGrants: [],
      isLoading: false,
      isError: false,
      refetch: () => {},
    };
  }
  return ctx;
}
