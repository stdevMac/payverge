import {
  adminBusinessAPI,
  type AdminBusinessDetail,
} from "@/api/adminBusiness";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

describe("admin business api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("passes the lifecycle list through unchanged", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        businesses: [
          {
            id: 1,
            name: "Cafe",
            status: "active",
            last_active_at: "2026-05-20T09:30:00Z",
          },
        ],
        total: 1,
      },
    });

    const result = await adminBusinessAPI.getList({
      page: 1,
      limit: 10,
      status: "active",
    });

    expect(axiosInstance.get).toHaveBeenCalledWith("/admin/businesses", {
      params: { page: 1, limit: 10, status: "active" },
      _useCache: false,
    });
    expect(result.businesses[0]).toEqual(
      expect.objectContaining({
        id: 1,
        status: "active",
        last_active_at: "2026-05-20T09:30:00Z",
      }),
    );
  });

  it("normalizes the admin business detail payload used by the detail panel", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        business: {
          id: 42,
          name: "Cafe 42",
          address: {
            street: "123 Main St",
            city: "Austin",
            state: "TX",
            postal_code: "78701",
            country: "US",
          },
          is_active: false,
          closed_at: null,
          created_at: "2026-01-01T00:00:00Z",
        },
        owner: null,
        staff: null,
        activity: {
          recent_order_count: 12,
          recent_payment_count: 9,
          total_revenue: 456.78,
        },
        admin_actions: null,
      },
    });

    const detail = await adminBusinessAPI.getDetail(42);

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/admin/businesses/42/detail",
      { _useCache: false },
    );
    expect(detail).toEqual<AdminBusinessDetail>(
      expect.objectContaining({
        business: expect.objectContaining({
          id: 42,
          name: "Cafe 42",
          address: "123 Main St, Austin, TX, 78701, US",
          status: "suspended",
          is_active: false,
          closed_at: null,
          has_settlement_address: false,
          has_tipping_address: false,
        }),
        owner: {
          id: 0,
          email_domain: "",
          created_at: "",
        },
        recent_activity: {
          recent_orders: 12,
          recent_payments: 9,
          total_revenue: 456.78,
          total_tips: 0,
        },
        staff: [],
        admin_actions: [],
      }),
    );
  });

  it("projects owner and staff email domains and wallet flags", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        business: {
          id: 3,
          name: "Venue",
          slug: "venue",
          has_settlement_address: true,
          has_tipping_address: false,
          is_active: true,
          created_at: "2026-02-02T00:00:00Z",
        },
        owner: {
          id: 11,
          email_domain: "venue.example",
          created_at: "2026-01-02T00:00:00Z",
          email: "owner@venue.example",
        },
        staff: [
          {
            id: 4,
            name: "Ada",
            email_domain: "floor.example",
            role: "server",
            email: "ada@floor.example",
          },
        ],
      },
    });

    const detail = await adminBusinessAPI.getDetail(3);

    expect(detail.business).toEqual(
      expect.objectContaining({
        slug: "venue",
        has_settlement_address: true,
        has_tipping_address: false,
      }),
    );
    expect(detail.owner).toEqual({
      id: 11,
      email_domain: "venue.example",
      created_at: "2026-01-02T00:00:00Z",
    });
    expect(detail.staff).toEqual([
      { id: 4, name: "Ada", email_domain: "floor.example", role: "server" },
    ]);
    const wire = JSON.stringify(detail);
    expect(wire).not.toContain("owner@venue.example");
    expect(wire).not.toContain("ada@floor.example");
  });

  it("derives closed from closed_at ahead of is_active", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        business: {
          id: 8,
          name: "Closed Cafe",
          is_active: true,
          closed_at: "2026-06-01T00:00:00Z",
        },
      },
    });

    const detail = await adminBusinessAPI.getDetail(8);

    expect(detail.business.status).toBe("closed");
  });

  it("surfaces total_tips from the detail activity payload as branded dollars", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        business: { id: 7, name: "Tip Cafe", created_at: "2026-01-01T00:00:00Z" },
        owner: null,
        staff: null,
        activity: {
          recent_order_count: 4,
          recent_payment_count: 3,
          total_revenue: 200.5,
          total_tips: 33.25,
        },
        admin_actions: null,
      },
    });

    const detail = await adminBusinessAPI.getDetail(7);

    expect(detail.recent_activity.total_revenue).toBe(200.5);
    expect(detail.recent_activity.total_tips).toBe(33.25);
  });
});
