import { scheduleApi } from "@/api/schedule";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));

const mocked = axiosInstance as unknown as {
  get: jest.Mock;
  post: jest.Mock;
  patch: jest.Mock;
  delete: jest.Mock;
};

describe("scheduleApi", () => {
  beforeEach(() => jest.clearAllMocks());

  it("gets a week (schedule + shifts) scoped by the week param", async () => {
    mocked.get.mockResolvedValue({
      data: { data: { schedule: { id: 1, status: "published" }, shifts: [{ id: 9 }] } },
    });
    const out = await scheduleApi.get("42", "2026-06-29");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/schedule", {
      params: { week: "2026-06-29" },
    });
    expect(out.schedule?.id).toBe(1);
    expect(out.shifts[0].id).toBe(9);
  });

  it("creates (get-or-creates) a draft for a week", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 5, status: "draft" } } });
    const out = await scheduleApi.createDraft("42", "2026-06-29");
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/schedule", {
      week: "2026-06-29",
    });
    expect(out.id).toBe(5);
  });

  it("creates a shift with the input body", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 11 } } });
    const input = {
      schedule_id: 5,
      position_id: 3,
      staff_id: 7,
      starts_at: "2026-06-29T17:00:00Z",
      ends_at: "2026-06-29T23:00:00Z",
    };
    const out = await scheduleApi.createShift("42", input);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/shifts", input);
    expect(out.id).toBe(11);
  });

  // L5-26 decision #10: optional attempt-scoped Idempotency-Key so a lost-response
  // retry replays the cached 201 instead of double-creating the day.
  it("sends Idempotency-Key when createShift is given an optional key", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 11 } } });
    const input = {
      schedule_id: 5,
      position_id: 3,
      staff_id: 7,
      starts_at: "2026-06-29T17:00:00Z",
      ends_at: "2026-06-29T23:00:00Z",
    };
    await scheduleApi.createShift("42", input, {
      idempotencyKey: "attempt-uuid:2026-06-29",
    });
    expect(mocked.post).toHaveBeenCalledWith(
      "/inside/businesses/42/shifts",
      input,
      { headers: { "Idempotency-Key": "attempt-uuid:2026-06-29" } },
    );
  });

  it("omits Idempotency-Key when createShift has no key option", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 11 } } });
    const input = {
      schedule_id: 5,
      position_id: 3,
      starts_at: "2026-06-29T17:00:00Z",
      ends_at: "2026-06-29T23:00:00Z",
    };
    await scheduleApi.createShift("42", input);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/shifts", input);
    // No third-arg config — callers that never opted in must stay unchanged.
    expect(mocked.post.mock.calls[0].length).toBe(2);
  });

  it("patches a shift by id (staff_id:0 unassigns)", async () => {
    mocked.patch.mockResolvedValue({ data: { data: { id: 11, staff_id: null } } });
    const out = await scheduleApi.updateShift("42", 11, { staff_id: 0 });
    expect(mocked.patch).toHaveBeenCalledWith("/inside/businesses/42/shifts/11", {
      staff_id: 0,
    });
    expect(out.staff_id).toBeNull();
  });

  it("deletes a shift by id", async () => {
    mocked.delete.mockResolvedValue({ data: { success: true } });
    await scheduleApi.deleteShift("42", 11);
    expect(mocked.delete).toHaveBeenCalledWith("/inside/businesses/42/shifts/11");
  });

  it("publishes a schedule and unwraps schedule + shifts", async () => {
    mocked.post.mockResolvedValue({
      data: { data: { schedule: { id: 5, status: "published" }, shifts: [{ id: 11 }] } },
    });
    const out = await scheduleApi.publish("42", 5);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/schedule/5/publish");
    expect(out.schedule?.status).toBe("published");
    expect(out.shifts[0].id).toBe(11);
  });

  it("gets a labor preview with salesTarget in the query and unwraps data.data", async () => {
    const payload = {
      schedule_id: 9,
      can_see_dollars: true,
      total_hours: 7.5,
      labor_cost: 150,
      sales_target: 1000,
      labor_cost_pct: 0.15,
      positions: [{ position_id: 3, shift_count: 1, hours: 7.5, labor_cost: 150 }],
      warnings: [{ code: "overtime", staff_id: 7, detail: 2520 }],
    };
    mocked.get.mockResolvedValue({ data: { data: payload } });
    const out = await scheduleApi.laborPreview("42", 9, 1000);
    expect(mocked.get).toHaveBeenCalledWith(
      "/inside/businesses/42/schedule/9/labor-preview?salesTarget=1000",
    );
    expect(out).toEqual(payload);
  });

  it("omits the salesTarget query when no target is provided", async () => {
    mocked.get.mockResolvedValue({ data: { data: { schedule_id: 9 } } });
    await scheduleApi.laborPreview("42", 9);
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/schedule/9/labor-preview");
  });

  it("copies a week transactionally in one call", async () => {
    mocked.post.mockResolvedValue({ data: { data: { created: 5, skipped: 0 } } });
    const out = await scheduleApi.copyWeek("42", "2026-06-29", "2026-07-06");
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/schedule/copy-week", {
      from_week: "2026-06-29",
      to_week: "2026-07-06",
      only_empty_days: false,
    });
    expect(out).toEqual({ created: 5, skipped: 0 });
  });

  it("copies into empty days when onlyEmptyDays is set", async () => {
    mocked.post.mockResolvedValue({ data: { data: { created: 2, skipped: 3 } } });
    const out = await scheduleApi.copyWeek("42", "2026-06-29", "2026-07-06", true);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/schedule/copy-week", {
      from_week: "2026-06-29",
      to_week: "2026-07-06",
      only_empty_days: true,
    });
    expect(out.skipped).toBe(3);
  });
});
