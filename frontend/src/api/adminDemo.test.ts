import {
  appendAdminDemoDay,
  ensureAdminDemo,
  getAdminDemo,
  resetAdminDemo,
  verifyAdminDemo,
  type AdminDemoSummary,
} from "@/api/adminDemo";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

const summary: AdminDemoSummary = {
  instance: {
    id: 1,
    admin_user_id: 7,
    primary_business_id: 10,
    secondary_business_id: 11,
    status: "ready",
    seed_version: "test",
    baseline_start_date: "2026-06-03T00:00:00Z",
    last_simulated_business_date: "2026-07-02T00:00:00Z",
    timezone: "America/New_York",
    created_at: "2026-07-02T00:00:00Z",
    updated_at: "2026-07-02T00:00:00Z",
  },
  businesses: [
    {
      id: 10,
      name: "Core Demo",
      business_id: "demo-core",
      is_demo: true,
    },
    {
      id: 11,
      name: "AI Demo",
      business_id: "demo-ai",
      is_demo: true,
    },
  ],
  access: [],
  runs: [],
  verification: {
    status: "passed",
    errors: [],
    coverage: [{ key: "menu_images", status: "passed", count: 2 }],
  },
};

describe("adminDemo api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("loads the authenticated admin demo summary", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: summary });

    await expect(getAdminDemo()).resolves.toBe(summary);

    expect(axiosInstance.get).toHaveBeenCalledWith("/admin/demo", {
      _useCache: false,
    });
  });

  it("posts manual demo actions to the admin demo endpoints", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: summary });

    await ensureAdminDemo();
    await resetAdminDemo();
    await appendAdminDemoDay();

    expect(axiosInstance.post).toHaveBeenNthCalledWith(1, "/admin/demo/ensure");
    expect(axiosInstance.post).toHaveBeenNthCalledWith(2, "/admin/demo/reset");
    expect(axiosInstance.post).toHaveBeenNthCalledWith(
      3,
      "/admin/demo/append-day",
    );
  });

  it("returns verification from the verify endpoint", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { verification: summary.verification },
    });

    await expect(verifyAdminDemo()).resolves.toBe(summary.verification);

    expect(axiosInstance.post).toHaveBeenCalledWith("/admin/demo/verify");
  });
});
