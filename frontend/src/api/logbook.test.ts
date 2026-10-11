import { logbookApi } from "@/api/logbook";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn(), post: jest.fn() },
}));

const mocked = axiosInstance as unknown as { get: jest.Mock; post: jest.Mock };

describe("logbookApi", () => {
  beforeEach(() => jest.clearAllMocks());

  it("lists notes for a given date", async () => {
    mocked.get.mockResolvedValue({ data: { data: [{ id: 1, category: "sales", content: "Busy" }] } });
    const out = await logbookApi.list("42", "2026-06-30");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/shift-notes?date=2026-06-30");
    expect(out[0].content).toBe("Busy");
  });

  it("lists today's notes when no date is passed", async () => {
    mocked.get.mockResolvedValue({ data: { data: [] } });
    await logbookApi.list("42");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/shift-notes");
  });

  it("creates a note with the input body", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 5, category: "guests", content: "VIP at 8" } } });
    const out = await logbookApi.create("42", { category: "guests", content: "VIP at 8" });
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/shift-notes", {
      category: "guests",
      content: "VIP at 8",
    });
    expect(out.id).toBe(5);
  });
});
