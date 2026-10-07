import {
  getAdminEscalations,
  parseEscalationTranscriptTarget,
  patchEscalation,
} from "@/api/adminEscalations";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    patch: jest.fn(),
  },
}));

const mockedGet = axiosInstance.get as unknown as jest.Mock;
const mockedPatch = axiosInstance.patch as unknown as jest.Mock;

beforeEach(() => {
  mockedGet.mockReset();
  mockedPatch.mockReset();
});

describe("getAdminEscalations", () => {
  it("hits the paginated endpoint and unwraps escalations + total", async () => {
    mockedGet.mockResolvedValueOnce({
      data: {
        escalations: [{ id: 9, source: "ops", issue: "billing", status: "open" }],
        total: 3,
      },
    });
    const result = await getAdminEscalations();
    expect(mockedGet).toHaveBeenCalledWith(
      "/admin/escalations",
      expect.objectContaining({ params: { page: 1, limit: 50 } }),
    );
    expect(result.total).toBe(3);
    expect(result.escalations[0].source).toBe("ops");
  });

  it("defaults missing arrays/totals to empty", async () => {
    mockedGet.mockResolvedValueOnce({ data: {} });
    const result = await getAdminEscalations();
    expect(result.escalations).toEqual([]);
    expect(result.total).toBe(0);
  });
});

describe("patchEscalation", () => {
  it("patches status and returns the updated row", async () => {
    mockedPatch.mockResolvedValueOnce({
      data: { escalation: { id: 1, status: "resolved" } },
    });
    const row = await patchEscalation(1, { status: "resolved" });
    expect(mockedPatch).toHaveBeenCalledWith("/admin/escalations/1", {
      status: "resolved",
    });
    expect(row.status).toBe("resolved");
  });
});

describe("parseEscalationTranscriptTarget", () => {
  it("resolves an ops thread reference", () => {
    expect(parseEscalationTranscriptTarget("thread:42", 7)).toEqual({
      businessId: 7,
      threadId: 42,
    });
  });

  it("has no transcript for non-thread refs or a missing business", () => {
    expect(parseEscalationTranscriptTarget("sess-abc", 7)).toBeNull();
    expect(parseEscalationTranscriptTarget("thread:42", null)).toBeNull();
    expect(parseEscalationTranscriptTarget("thread:x", 7)).toBeNull();
    expect(parseEscalationTranscriptTarget("", 7)).toBeNull();
  });
});
