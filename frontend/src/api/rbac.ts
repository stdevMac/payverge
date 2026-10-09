import { axiosInstance } from "@/api/tools/instance";

export interface RBACAuditLog {
  id: number;
  staff_id: number;
  business_id: number;
  action: string;
  old_role?: string;
  new_role?: string;
  old_permissions?: string;
  new_permissions?: string;
  changed_by: string;
  reason?: string;
  ip_address?: string;
  user_agent?: string;
  created_at: string;
  staff?: {
    id: number;
    name: string;
    email: string;
  };
}

export interface StaffPermissions {
  permissions: string[];
  role_permissions: string[];
  custom_grants: string[];
  /** Explicit deny overrides (subtracted from role ∪ grants for effective set). */
  custom_denies: string[];
}

export interface ChangeRoleRequest {
  new_role: 'kitchen' | 'host' | 'server' | 'manager';
  reason?: string;
}

export interface PermissionRequest {
  permission: string;
  reason?: string;
}

export interface DeactivateRequest {
  reason?: string;
}

// Get staff permissions (effective + role defaults + custom grants)
export const getStaffPermissions = async (
  businessId: string,
  staffId: string,
): Promise<StaffPermissions> => {
  const response = await axiosInstance.get<StaffPermissions>(
    `/inside/businesses/${businessId}/staff/${staffId}/permissions`,
  );
  const data = response.data;
  return {
    permissions: data.permissions ?? [],
    role_permissions: data.role_permissions ?? [],
    custom_grants: data.custom_grants ?? [],
    custom_denies: data.custom_denies ?? [],
  };
};

// Change staff role
export const changeStaffRole = async (businessId: string, staffId: string, request: ChangeRoleRequest): Promise<{ message: string }> => {
  const response = await axiosInstance.put(`/inside/businesses/${businessId}/staff/${staffId}/role`, request);
  return response.data;
};

// Grant custom permission
export const grantCustomPermission = async (businessId: string, staffId: string, request: PermissionRequest): Promise<{ message: string }> => {
  const response = await axiosInstance.post(`/inside/businesses/${businessId}/staff/${staffId}/permissions`, request);
  return response.data;
};

// Revoke custom permission
export const revokeCustomPermission = async (businessId: string, staffId: string, request: PermissionRequest): Promise<{ message: string }> => {
  const response = await axiosInstance.delete(`/inside/businesses/${businessId}/staff/${staffId}/permissions`, { data: request });
  return response.data;
};

// Set an explicit permission deny (does not remove grants; effective set subtracts).
export const denyPermission = async (
  businessId: string,
  staffId: string,
  request: PermissionRequest,
): Promise<{ message: string }> => {
  const response = await axiosInstance.post(
    `/inside/businesses/${businessId}/staff/${staffId}/permission-denies`,
    request,
  );
  return response.data;
};

// Clear an explicit permission deny.
export const removePermissionDeny = async (
  businessId: string,
  staffId: string,
  request: PermissionRequest,
): Promise<{ message: string }> => {
  const response = await axiosInstance.delete(
    `/inside/businesses/${businessId}/staff/${staffId}/permission-denies`,
    { data: request },
  );
  return response.data;
};

/** Owner-adjacent / non-delegable keys that need explicit confirmation in the editor. */
export const SENSITIVE_PERMISSIONS = new Set([
  "bills:refund",
  "payroll:write",
  "fiscal:credit",
  "fiscal:credentials",
  "director:read",
  "director:write",
  "financial:withdraw",
  "financial:write",
  "staff:permissions",
  "staff:deactivate",
]);

// Deactivate staff
export const deactivateStaff = async (businessId: string, staffId: string, request: DeactivateRequest): Promise<{ message: string }> => {
  const response = await axiosInstance.post(`/inside/businesses/${businessId}/staff/${staffId}/deactivate`, request);
  return response.data;
};

// Reactivate staff
export const reactivateStaff = async (businessId: string, staffId: string, request: DeactivateRequest): Promise<{ message: string }> => {
  const response = await axiosInstance.post(`/inside/businesses/${businessId}/staff/${staffId}/reactivate`, request);
  return response.data;
};

// Get staff audit log
export const getStaffAuditLog = async (businessId: string, staffId: string, limit = 20, offset = 0): Promise<{ audit_logs: RBACAuditLog[] }> => {
  const response = await axiosInstance.get(`/inside/businesses/${businessId}/staff/${staffId}/audit?limit=${limit}&offset=${offset}`);
  return response.data;
};
