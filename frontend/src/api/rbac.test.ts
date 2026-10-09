import {
  changeStaffRole,
  denyPermission,
  getStaffPermissions,
  removePermissionDeny,
  revokeCustomPermission,
} from "@/api/rbac";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    delete: jest.fn(),
    get: jest.fn(),
    post: jest.fn(),
    put: jest.fn(),
  },
}));

describe("rbac API", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("gets staff permissions and normalizes missing fields to empty arrays", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        permissions: ["bills:read", "menu:read"],
        // role_permissions / custom_grants / custom_denies omitted — should default to []
      },
    });

    const result = await getStaffPermissions("42", "7");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/staff/7/permissions",
    );
    expect(result).toEqual({
      permissions: ["bills:read", "menu:read"],
      role_permissions: [],
      custom_grants: [],
      custom_denies: [],
    });
  });

  it("gets staff permissions with full effective/role/custom/deny payload", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        permissions: ["bills:read", "analytics:sales"],
        role_permissions: ["bills:read", "menu:read"],
        custom_grants: ["analytics:sales"],
        custom_denies: ["menu:read"],
      },
    });

    const result = await getStaffPermissions("42", "7");

    expect(result).toEqual({
      permissions: ["bills:read", "analytics:sales"],
      role_permissions: ["bills:read", "menu:read"],
      custom_grants: ["analytics:sales"],
      custom_denies: ["menu:read"],
    });
  });

  it("changes staff roles using the live RBAC payload contract", async () => {
    (axiosInstance.put as jest.Mock).mockResolvedValue({
      data: { message: "Staff role updated successfully" },
    });

    await changeStaffRole("42", "7", {
      new_role: "manager",
      reason: "Promoted to shift manager",
    });

    expect(axiosInstance.put).toHaveBeenCalledWith(
      "/inside/businesses/42/staff/7/role",
      { new_role: "manager", reason: "Promoted to shift manager" },
    );
  });

  it("revokes custom permissions through a DELETE request body", async () => {
    (axiosInstance.delete as jest.Mock).mockResolvedValue({
      data: { message: "Permission revoked successfully" },
    });

    await revokeCustomPermission("42", "7", {
      permission: "analytics:sales",
      reason: "Temporary grant ended",
    });

    expect(axiosInstance.delete).toHaveBeenCalledWith(
      "/inside/businesses/42/staff/7/permissions",
      {
        data: {
          permission: "analytics:sales",
          reason: "Temporary grant ended",
        },
      },
    );
  });

  it("sets a permission deny via POST permission-denies", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { message: "Permission denied successfully" },
    });

    await denyPermission("42", "7", {
      permission: "menu:write",
      reason: "kitchen only",
    });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/staff/7/permission-denies",
      { permission: "menu:write", reason: "kitchen only" },
    );
  });

  it("clears a permission deny via DELETE permission-denies", async () => {
    (axiosInstance.delete as jest.Mock).mockResolvedValue({
      data: { message: "Permission deny removed successfully" },
    });

    await removePermissionDeny("42", "7", {
      permission: "menu:write",
      reason: "restored",
    });

    expect(axiosInstance.delete).toHaveBeenCalledWith(
      "/inside/businesses/42/staff/7/permission-denies",
      {
        data: {
          permission: "menu:write",
          reason: "restored",
        },
      },
    );
  });
});
