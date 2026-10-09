import { coverageApi } from "@/api/coverage";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn(), post: jest.fn() },
}));

const mockGet = axiosInstance.get as jest.Mock;
const mockPost = axiosInstance.post as jest.Mock;

describe("coverageApi", () => {
  beforeEach(() => jest.clearAllMocks());

  it("lists open coverage and unwraps res.data.data", async () => {
    mockGet.mockResolvedValue({
      data: { data: { open_shifts: [], swap_inbox: [], pending_approvals: { swaps: [], claims: [] } } },
    });
    const res = await coverageApi.listOpen("42");
    expect(mockGet).toHaveBeenCalledWith("/inside/businesses/42/coverage/open");
    expect(res.swap_inbox).toEqual([]);
    expect(res.pending_approvals.claims).toEqual([]);
  });

  it("lists my coverage and unwraps res.data.data", async () => {
    mockGet.mockResolvedValue({ data: { data: { claims: [], swaps: [{ id: 3 }] } } });
    const res = await coverageApi.listMine("42");
    expect(mockGet).toHaveBeenCalledWith("/inside/businesses/42/coverage/mine");
    expect(res.swaps[0].id).toBe(3);
  });

  it("claims an open shift with an empty body", async () => {
    mockPost.mockResolvedValue({ data: { data: { id: 1, status: "pending" } } });
    const res = await coverageApi.claim("42", 10);
    expect(mockPost).toHaveBeenCalledWith("/inside/businesses/42/shifts/10/claim");
    expect(res.status).toBe("pending");
  });

  it("requests a swap with the kind discriminator body", async () => {
    mockPost.mockResolvedValue({ data: { data: { id: 5, status: "open", kind: "swap" } } });
    const res = await coverageApi.requestSwap("42", 10, { kind: "swap", target: "all_in_role" });
    expect(mockPost).toHaveBeenCalledWith("/inside/businesses/42/shifts/10/swap", {
      kind: "swap",
      target: "all_in_role",
    });
    expect(res.kind).toBe("swap");
  });

  it("requests a give-up with the kind discriminator body", async () => {
    mockPost.mockResolvedValue({ data: { data: { id: 6, status: "pending_approval", kind: "giveup" } } });
    await coverageApi.requestSwap("42", 11, { kind: "giveup" });
    expect(mockPost).toHaveBeenCalledWith("/inside/businesses/42/shifts/11/swap", { kind: "giveup" });
  });

  it("accepts a swap with an empty body", async () => {
    mockPost.mockResolvedValue({ data: { data: { id: 5, status: "accepted" } } });
    const res = await coverageApi.acceptSwap("42", 5);
    expect(mockPost).toHaveBeenCalledWith("/inside/businesses/42/swaps/5/accept");
    expect(res.status).toBe("accepted");
  });

  it("decides a swap with the kind discriminator", async () => {
    mockPost.mockResolvedValue({ data: { data: { id: 1, status: "approved" } } });
    await coverageApi.decide("42", 1, { kind: "swap", decision: "approve" });
    expect(mockPost).toHaveBeenCalledWith("/inside/businesses/42/swaps/1/decision", {
      kind: "swap",
      decision: "approve",
    });
  });

  it("decides an open claim against the same route", async () => {
    mockPost.mockResolvedValue({ data: { data: { id: 7, status: "denied" } } });
    await coverageApi.decide("42", 7, { kind: "open_claim", decision: "deny" });
    expect(mockPost).toHaveBeenCalledWith("/inside/businesses/42/swaps/7/decision", {
      kind: "open_claim",
      decision: "deny",
    });
  });

  it("cancels a swap/giveup against the coverage cancel route", async () => {
    mockPost.mockResolvedValue({ data: { data: { id: 2, status: "cancelled" } } });
    const res = await coverageApi.cancel("42", 2, "giveup");
    expect(mockPost).toHaveBeenCalledWith("/inside/businesses/42/coverage/2/cancel", {
      kind: "giveup",
    });
    expect(res.status).toBe("cancelled");
  });

  it("withdraws an open claim against the same cancel route", async () => {
    mockPost.mockResolvedValue({ data: { data: { id: 1, status: "withdrawn" } } });
    await coverageApi.cancel("42", 1, "open_claim");
    expect(mockPost).toHaveBeenCalledWith("/inside/businesses/42/coverage/1/cancel", {
      kind: "open_claim",
    });
  });

  it("lists resolved coverage history with an optional limit", async () => {
    mockGet.mockResolvedValue({
      data: {
        data: [
          {
            kind: "swap",
            request_id: 5,
            shift_id: 10,
            position_id: 9,
            starts_at: "2026-07-01T16:00:00Z",
            ends_at: "2026-07-01T22:00:00Z",
            status: "approved",
            requester_staff_id: 3,
            decider_staff_id: 2,
            resolved_at: "2026-07-01T12:00:00Z",
          },
        ],
      },
    });
    const res = await coverageApi.history("42", 25);
    expect(mockGet).toHaveBeenCalledWith("/inside/businesses/42/coverage/history", {
      params: { limit: 25 },
    });
    expect(res[0].status).toBe("approved");
    expect(res[0].decider_staff_id).toBe(2);
  });
});
