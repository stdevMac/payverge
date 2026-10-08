import {
  adminUserAPI,
  getAdminStats,
  getUserList,
  type AdminStats,
  type AdminUserDetail,
} from "@/api/admin";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

describe("admin api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("forwards the lifecycle status filter to the user list", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        users: [{ id: 1, email: "owner@example.com", status: "suspended" }],
        total: 1,
        page: 1,
        limit: 10,
      },
    });

    const result = await getUserList({
      page: 1,
      limit: 10,
      search: "Needle",
      status: "suspended",
    });

    expect(axiosInstance.get).toHaveBeenCalledWith("/admin/users", {
      params: { page: 1, limit: 10, search: "Needle", status: "suspended" },
      _useCache: false,
    });
    expect(result.users).toEqual([
      expect.objectContaining({ id: 1, status: "suspended" }),
    ]);
  });

  it("returns the admin user detail with the business lifecycle", async () => {
    const payload: AdminUserDetail = {
      user: {
        id: 2,
        email: "owner@example.com",
        name: "Owner",
        role: "user",
        auth_method: "email",
        email_verified: true,
        created_at: "2026-01-01T00:00:00Z",
        picture: "",
      },
      business: {
        id: 9,
        business_id: "biz",
        name: "Biz",
        is_active: false,
        closed_at: null,
        closed_reason: "",
      },
      admin_actions: [],
    };
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: payload });

    const detail = await adminUserAPI.getDetail(2);

    expect(axiosInstance.get).toHaveBeenCalledWith("/admin/users/2/detail", {
      params: undefined,
      _useCache: false,
    });
    expect(detail).toEqual(payload);
  });

  it("passes through the full backend AdminStats payload including payment/revenue fields", async () => {
    const backendStats: AdminStats = {
      total_businesses: 3,
      active_businesses: 2,
      inactive_businesses: 1,
      business_growth: [{ month: "May 2026", count: 1 }],
      total_users: 5,
      users_by_role: { user: 4, admin: 1 },
      user_growth: [{ month: "May 2026", count: 2 }],
      total_payment_volume: 12_345.67,
      payment_volume_growth: [{ month: "May 2026", count: 10, value: 12_345.67 }],
      average_transaction_size: 42.5,
      failed_webhooks_count: 0,
      gross_merchandise_volume: 9_876.54,
      revenue_growth: [{ month: "May 2026", count: 10, value: 9_876.54 }],
      recent_admin_actions: [],
      recent_errors: [],
    };
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: backendStats });

    const stats = await getAdminStats();

    expect(axiosInstance.get).toHaveBeenCalledWith("/admin/stats", {
      _useCache: false,
    });
    expect(stats.total_payment_volume).toBe(12_345.67);
    expect(stats.payment_volume_growth[0].value).toBe(12_345.67);
    expect(stats.average_transaction_size).toBe(42.5);
    expect(stats.gross_merchandise_volume).toBe(9_876.54);
    expect(stats).not.toHaveProperty("mrr");
    expect(stats.revenue_growth[0].value).toBe(9_876.54);
    expect(stats).not.toHaveProperty("total_revenue");
  });
});
