import {
  applyDirectorAction,
  archiveDirectorThread,
  askDirectorStreamURL,
  directorActionErrorInfo,
  exportDirectorThreadURL,
  getDirectorThreadMessages,
  listDirectorAppliedActions,
  listDirectorThreads,
  restoreDirectorThread,
  submitDirectorFeedback,
  undoDirectorAction,
} from "@/api/directorConsole";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
    patch: jest.fn(),
    delete: jest.fn(),
  },
}));

describe("director console api", () => {
  const originalApiURL = process.env.API_URL;

  beforeEach(() => {
    jest.clearAllMocks();
    process.env.API_URL = "https://api.example.com/api/v1";
  });

  afterEach(() => {
    process.env.API_URL = originalApiURL;
  });

  it("requests a bounded thread transcript by default", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { messages: [] } });

    await getDirectorThreadMessages(42, 7);

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/ai/director/threads/7/messages?limit=200",
    );
  });

  it("lists active threads by default and archived threads on demand", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { threads: [] } });

    await listDirectorThreads(42);
    expect(axiosInstance.get).toHaveBeenLastCalledWith(
      "/inside/businesses/42/ai/director/threads",
    );

    await listDirectorThreads(42, { archived: true });
    expect(axiosInstance.get).toHaveBeenLastCalledWith(
      "/inside/businesses/42/ai/director/threads?archived=1",
    );
  });

  it("archives via PATCH and restores via POST — never a permanent delete", async () => {
    (axiosInstance.patch as jest.Mock).mockResolvedValue({ data: {} });
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: {} });

    await archiveDirectorThread(7, 11);
    expect(axiosInstance.patch).toHaveBeenCalledWith(
      "/inside/businesses/7/ai/director/threads/11/archive",
    );

    await restoreDirectorThread(7, 11);
    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/7/ai/director/threads/11/restore",
    );
  });

  it("lists applied director actions with undo eligibility", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        actions: [
          {
            proposal_id: "pa_1",
            kind: "menu.adjust_prices",
            title: "Reprice fries",
            applied_at: "2026-07-21T10:00:00Z",
            undone_at: null,
            can_undo: true,
          },
        ],
      },
    });

    const res = await listDirectorAppliedActions(7);
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/7/ai/director/actions/applied",
    );
    expect(res.actions[0].proposal_id).toBe("pa_1");
    expect(res.actions[0].can_undo).toBe(true);
  });

  it("builds absolute direct-fetch URLs with the API base", () => {
    expect(askDirectorStreamURL(7)).toBe(
      "https://api.example.com/api/v1/inside/businesses/7/ai/director/ask/stream",
    );
    expect(exportDirectorThreadURL(7, 11)).toBe(
      "https://api.example.com/api/v1/inside/businesses/7/ai/director/threads/11/export?format=md",
    );
  });

  it("posts feedback payloads to director routes", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValueOnce({
      data: { message: { id: 3 } },
    });

    await submitDirectorFeedback(7, 3, "up");

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/7/ai/director/messages/3/feedback",
      { vote: "up" },
    );
  });

  it("posts apply with the proposal public id and reconfirm flag", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        applied: true,
        result: { new_menu_version: 4, proposal_id: "pa_1", audit_id: 9, kind: "menu.adjust_prices" },
      },
    });

    const res = await applyDirectorAction(7, "pa_1");
    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/7/ai/director/actions/apply",
      { proposal_id: "pa_1", reconfirm: false },
    );
    expect(res.result.new_menu_version).toBe(4);

    await applyDirectorAction(7, "pa_1", true);
    expect(axiosInstance.post).toHaveBeenLastCalledWith(
      "/inside/businesses/7/ai/director/actions/apply",
      { proposal_id: "pa_1", reconfirm: true },
    );
  });

  it("posts undo with the proposal public id", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        undone: true,
        result: { new_menu_version: 5, proposal_id: "pa_1", audit_id: 9, kind: "menu.adjust_prices" },
      },
    });

    const res = await undoDirectorAction(7, "pa_1");
    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/7/ai/director/actions/undo",
      { proposal_id: "pa_1" },
    );
    expect(res.undone).toBe(true);
  });

  it("extracts status + machine code from axios-shaped apply errors", () => {
    expect(
      directorActionErrorInfo({
        response: { status: 409, data: { code: "menu_changed", error: "The menu changed" } },
      }),
    ).toEqual({ status: 409, code: "menu_changed" });
    expect(directorActionErrorInfo(new Error("network down"))).toEqual({
      status: undefined,
      code: undefined,
    });
  });
});
