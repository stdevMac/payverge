import { positionsApi } from "@/api/positions";
import { axiosInstance } from "@/api/tools/instance";
import type { Dollars } from "@/types/money";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn(), put: jest.fn() },
}));

const mocked = axiosInstance as unknown as {
  get: jest.Mock; post: jest.Mock; patch: jest.Mock; delete: jest.Mock; put: jest.Mock;
};

describe("positionsApi", () => {
  beforeEach(() => jest.clearAllMocks());

  it("lists positions for a business", async () => {
    mocked.get.mockResolvedValue({ data: { data: [{ id: 1, name: "Server" }] } });
    const out = await positionsApi.list("42");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/positions");
    expect(out[0].name).toBe("Server");
  });

  it("assigns a position as primary", async () => {
    mocked.post.mockResolvedValue({ data: { success: true } });
    await positionsApi.assign("42", 7, 3, true);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/staff/7/positions", {
      position_id: 3,
      is_primary: true,
    });
  });

  it("reads an owner-only pay rate in dollars", async () => {
    mocked.get.mockResolvedValue({ data: { data: { pay_rate: 18.5 } } });
    const rate = await positionsApi.getRate("42", 7, 3);
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/staff/7/positions/3/rate");
    expect(rate).toBe(18.5);
  });

  it("writes an owner-only pay rate in dollars", async () => {
    mocked.put.mockResolvedValue({ data: { success: true } });
    await positionsApi.setRate("42", 7, 3, 18.5 as Dollars);
    expect(mocked.put).toHaveBeenCalledWith("/inside/businesses/42/staff/7/positions/3/rate", {
      pay_rate: 18.5,
    });
  });

  it("removes (retires) a position by concatenated id", async () => {
    mocked.delete.mockResolvedValue({ data: { success: true } });
    await positionsApi.remove("42", 3);
    expect(mocked.delete).toHaveBeenCalledWith("/inside/businesses/42/positions/3");
  });

  it("unassigns a position from a staff member", async () => {
    mocked.delete.mockResolvedValue({ data: { success: true } });
    await positionsApi.unassign("42", 7, 3);
    expect(mocked.delete).toHaveBeenCalledWith("/inside/businesses/42/staff/7/positions/3");
  });

  it("creates a position with the input body", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 5, name: "Server" } } });
    const input = { name: "Server" };
    await positionsApi.create("42", input);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/positions", input);
  });

  it("updates a position with the input body", async () => {
    mocked.patch.mockResolvedValue({ data: { success: true } });
    const input = { name: "Waiter" };
    await positionsApi.update("42", 3, input);
    expect(mocked.patch).toHaveBeenCalledWith("/inside/businesses/42/positions/3", input);
  });

  it("lists positions assigned to a staff member", async () => {
    mocked.get.mockResolvedValue({ data: { data: [{ id: 1, staff_id: 7, position_id: 3 }] } });
    const out = await positionsApi.listForStaff("42", 7);
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/staff/7/positions");
    expect(out[0].position_id).toBe(3);
  });
});
