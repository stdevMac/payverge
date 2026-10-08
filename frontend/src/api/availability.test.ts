import { availabilityApi } from "@/api/availability";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn(), put: jest.fn(), post: jest.fn() },
}));

const mocked = axiosInstance as unknown as {
  get: jest.Mock;
  put: jest.Mock;
  post: jest.Mock;
};

describe("availabilityApi", () => {
  beforeEach(() => jest.clearAllMocks());

  it("gets the caller's own availability windows", async () => {
    mocked.get.mockResolvedValue({
      data: { data: [{ id: 1, weekday: 1, start_min: 540, end_min: 1020, kind: "preferred" }] },
    });
    const out = await availabilityApi.getMine("42");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/me/availability");
    expect(out[0].id).toBe(1);
    expect(out[0].kind).toBe("preferred");
  });

  it("replaces availability with a windows body (full replace)", async () => {
    mocked.put.mockResolvedValue({ data: { data: [{ id: 2 }] } });
    const windows = [
      { weekday: 2, start_min: 600, end_min: 1080, kind: "unavailable" as const },
    ];
    const out = await availabilityApi.putMine("42", windows);
    expect(mocked.put).toHaveBeenCalledWith("/inside/businesses/42/me/availability", {
      windows,
    });
    expect(out[0].id).toBe(2);
  });

  it("clears availability with an empty windows array", async () => {
    mocked.put.mockResolvedValue({ data: { data: [] } });
    const out = await availabilityApi.putMine("42", []);
    expect(mocked.put).toHaveBeenCalledWith("/inside/businesses/42/me/availability", {
      windows: [],
    });
    expect(out).toEqual([]);
  });

  it("creates a pending time-off request with the input body", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 9, status: "pending" } } });
    const input = {
      starts_at: "2026-07-01T00:00:00Z",
      ends_at: "2026-07-02T00:00:00Z",
      reason: "Family event",
    };
    const out = await availabilityApi.createTimeOff("42", input);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/me/time-off", input);
    expect(out.status).toBe("pending");
  });

  it("lists time-off filtered by status", async () => {
    mocked.get.mockResolvedValue({ data: { data: [{ id: 9, status: "pending" }] } });
    const out = await availabilityApi.listTimeOff("42", "pending");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/time-off", {
      params: { status: "pending" },
    });
    expect(out[0].id).toBe(9);
  });

  it("lists time-off with no status param when omitted", async () => {
    mocked.get.mockResolvedValue({ data: { data: [] } });
    await availabilityApi.listTimeOff("42");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/time-off", {
      params: undefined,
    });
  });

  it("decides a request (approve) by id", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 9, status: "approved" } } });
    const out = await availabilityApi.decide("42", 9, true, "Enjoy");
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/time-off/9/decision", {
      approve: true,
      reason: "Enjoy",
    });
    expect(out.status).toBe("approved");
  });
});
