import { demoLogin, fetchDemoTables } from "@/api/demo";
import { API_BASE_URL } from "@/api/tools/baseUrl";

describe("demo api", () => {
  const fetchMock = jest.fn();
  beforeEach(() => {
    fetchMock.mockReset();
    global.fetch = fetchMock;
  });

  it("posts the role with credentials and returns the owner redirect", async () => {
    fetchMock.mockResolvedValue({ ok: true, json: async () => ({ redirect: "/dashboard", token: "t" }) });
    await expect(demoLogin("owner")).resolves.toEqual({ kind: "owner", redirect: "/dashboard" });
    expect(fetchMock).toHaveBeenCalledWith(`${API_BASE_URL}/auth/demo/login`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ role: "owner" }),
    });
  });

  it("never follows an off-site owner redirect", async () => {
    fetchMock.mockResolvedValue({ ok: true, json: async () => ({ redirect: "//evil.example" }) });
    await expect(demoLogin("owner")).resolves.toEqual({ kind: "owner", redirect: "/dashboard" });
  });

  it("returns the staff payload for staff roles", async () => {
    const staff = { id: 4, name: "Tomás", email: "t@x", role: "kitchen", business_id: 9 };
    fetchMock.mockResolvedValue({ ok: true, json: async () => ({ staff }) });
    await expect(demoLogin("kitchen")).resolves.toEqual({ kind: "staff", staff });
  });

  it("surfaces the server message (404 off a demo install)", async () => {
    fetchMock.mockResolvedValue({ ok: false, json: async () => ({ error: "Not found" }) });
    await expect(demoLogin("waiter")).rejects.toThrow("Not found");
  });

  it("keeps only safe table codes", async () => {
    fetchMock.mockResolvedValue({
      ok: true,
      json: async () => ({
        venues: [
          { name: "Bodegón", custom_url: "bodegon", tables: [{ name: "Mesa 1", code: "abc-1" }, { name: "x", code: "../admin" }] },
          null,
        ],
      }),
    });
    await expect(fetchDemoTables()).resolves.toEqual([
      { name: "Bodegón", custom_url: "bodegon", tables: [{ name: "Mesa 1", code: "abc-1" }] },
    ]);
  });
});
