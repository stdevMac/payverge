import { timeclockApi } from "@/api/timeclock";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn(), post: jest.fn(), patch: jest.fn() },
}));

const mocked = axiosInstance as unknown as {
  get: jest.Mock;
  post: jest.Mock;
  patch: jest.Mock;
};

describe("timeclockApi", () => {
  beforeEach(() => jest.clearAllMocks());

  it("clocks in without a shift (empty body) and unwraps the entry", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 1, status: "open" } } });
    const out = await timeclockApi.clockIn("42");
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/me/clock-in", {});
    expect(out.id).toBe(1);
    expect(out.status).toBe("open");
  });

  it("clocks in tied to a shift (shift_id body)", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 2, shift_id: 9 } } });
    const out = await timeclockApi.clockIn("42", 9);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/me/clock-in", {
      shift_id: 9,
    });
    expect(out.shift_id).toBe(9);
  });

  it("clocks out (no body) and unwraps the pending entry", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 1, status: "pending_review" } } });
    const out = await timeclockApi.clockOut("42");
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/me/clock-out");
    expect(out.status).toBe("pending_review");
  });

  it("adds break minutes to the open entry", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 1, break_minutes: 30 } } });
    const out = await timeclockApi.addBreak("42", 30);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/me/break", {
      minutes: 30,
    });
    expect(out.break_minutes).toBe(30);
  });

  it("lists the caller's own timesheet", async () => {
    mocked.get.mockResolvedValue({ data: { data: [{ id: 1, status: "open" }] } });
    const out = await timeclockApi.myTimesheet("42");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/me/timesheet");
    expect(out[0].id).toBe(1);
  });

  it("lists the caller's own entries in a date range", async () => {
    mocked.get.mockResolvedValue({ data: { data: [{ id: 1, worked_minutes: 450, worked_hours: 7.5 }] } });
    const out = await timeclockApi.listMineRange("42", "2026-07-01", "2026-07-07", "approved");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/me/timesheet", {
      params: { from: "2026-07-01", to: "2026-07-07", status: "approved" },
    });
    expect(out[0].id).toBe(1);
    expect(out[0].worked_hours).toBe(7.5);
  });

  it("lists the review queue (no status => server default) ", async () => {
    mocked.get.mockResolvedValue({ data: { data: [{ id: 5, status: "pending_review" }] } });
    const out = await timeclockApi.listForReview("42");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/timesheets", {
      params: undefined,
    });
    expect(out[0].status).toBe("pending_review");
  });

  it("lists the review queue filtered by status", async () => {
    mocked.get.mockResolvedValue({ data: { data: [] } });
    await timeclockApi.listForReview("42", "approved");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/timesheets", {
      params: { status: "approved" },
    });
  });

  it("approves an entry by id", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 5, status: "approved" } } });
    const out = await timeclockApi.approve("42", 5);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/timesheets/5/approve");
    expect(out.status).toBe("approved");
  });

  it("fetches a paginated review page with an honest total", async () => {
    mocked.get.mockResolvedValue({
      data: { data: [{ id: 1 }, { id: 2 }], total: 42, offset: 0, limit: 20 },
    });
    const out = await timeclockApi.listForReviewPaged("42", { limit: 20, offset: 0 });
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/timesheets", {
      params: { status: undefined, offset: 0, limit: 20, from: undefined, to: undefined },
    });
    expect(out.total).toBe(42);
    expect(out.data).toHaveLength(2);
  });

  it("rejects an entry with a reason", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 5, status: "rejected" } } });
    const out = await timeclockApi.reject("42", 5, "wrong out time");
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/timesheets/5/reject", {
      reason: "wrong out time",
    });
    expect(out.status).toBe("rejected");
  });

  it("rejects with an empty body when no reason is given", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 5, status: "rejected" } } });
    await timeclockApi.reject("42", 5);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/timesheets/5/reject", {});
  });

  it("edits an entry via PATCH", async () => {
    mocked.patch.mockResolvedValue({ data: { data: { id: 5, status: "pending_review" } } });
    const input = {
      clock_in_at: "2026-07-01T09:00:00Z",
      clock_out_at: "2026-07-01T17:00:00Z",
      break_minutes: 30,
      note: "fixed",
    };
    const out = await timeclockApi.edit("42", 5, input);
    expect(mocked.patch).toHaveBeenCalledWith("/inside/businesses/42/timesheets/5", input);
    expect(out.status).toBe("pending_review");
  });

  it("creates a manual entry with the input body", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 8, source: "manager_manual" } } });
    const input = {
      staff_id: 7,
      clock_in_at: "2026-06-30T17:00:00Z",
      clock_out_at: "2026-06-30T23:00:00Z",
      break_minutes: 30,
      note: "Forgot to punch",
    };
    const out = await timeclockApi.createManual("42", input);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/time-entries", input);
    expect(out.source).toBe("manager_manual");
  });
});
