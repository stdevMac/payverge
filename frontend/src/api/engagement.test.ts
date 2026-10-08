import { checklistsApi, documentsApi, recognitionApi, pollsApi } from "@/api/engagement";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn(), post: jest.fn(), put: jest.fn() },
}));

const mocked = axiosInstance as unknown as { get: jest.Mock; post: jest.Mock; put: jest.Mock };

beforeEach(() => jest.clearAllMocks());

describe("checklistsApi", () => {
  it("lists the caller's runs", async () => {
    mocked.get.mockResolvedValue({ data: { data: [{ id: 1, status: "pending" }] } });
    const out = await checklistsApi.listRuns("42");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/checklists/runs");
    expect(out[0].status).toBe("pending");
  });

  it("lists business-wide runs with the scope=business param", async () => {
    mocked.get.mockResolvedValue({
      data: {
        data: [
          { id: 1, status: "pending", template_name: "Opening", assigned_staff_name: "Ana" },
        ],
      },
    });
    const out = await checklistsApi.listBusinessRuns("42");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/checklists/runs", {
      params: { scope: "business" },
    });
    expect(out[0].template_name).toBe("Opening");
    expect(out[0].assigned_staff_name).toBe("Ana");
  });

  it("reads a run's detail (items + completion state)", async () => {
    mocked.get.mockResolvedValue({
      data: { data: { run: { id: 5, status: "in_progress" }, items: [{ item_id: 9, done: false }] } },
    });
    const out = await checklistsApi.getRunDetail("42", 5);
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/checklists/runs/5");
    expect(out.run.status).toBe("in_progress");
    expect(out.items[0].item_id).toBe(9);
  });

  it("ticks an item on a run", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 1, status: "in_progress" } } });
    await checklistsApi.tickItem("42", 5, 9, true);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/checklists/runs/5/items/9", {
      done: true,
      note: "",
    });
  });

  it("creates a template and a run", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 3 } } });
    await checklistsApi.createTemplate("42", {
      name: "Opening",
      kind: "opening",
      items: [{ label: "Unlock doors", sort_order: 0, is_required: true }],
    });
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/checklists/templates", {
      name: "Opening",
      kind: "opening",
      items: [{ label: "Unlock doors", sort_order: 0, is_required: true }],
    });
    await checklistsApi.createRun("42", { template_id: 3, assigned_staff_id: 7 });
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/checklists/runs", {
      template_id: 3,
      assigned_staff_id: 7,
    });
  });
});

describe("documentsApi", () => {
  it("lists documents with the caller's per-document ack state", async () => {
    mocked.get.mockResolvedValue({
      data: { data: [{ id: 11, title: "Handbook" }], acked: { "11": true } },
    });
    const out = await documentsApi.list("42");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/documents");
    expect(out.documents[0].title).toBe("Handbook");
    expect(out.acked["11"]).toBe(true);
  });

  it("defaults acked to an empty map when the server omits it", async () => {
    mocked.get.mockResolvedValue({ data: { data: [] } });
    const out = await documentsApi.list("42");
    expect(out.acked).toEqual({});
  });

  it("acknowledges a document", async () => {
    mocked.post.mockResolvedValue({ data: { success: true } });
    await documentsApi.ack("42", 11);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/documents/11/ack", {});
  });

  it("updates a document, bumping its version", async () => {
    mocked.put.mockResolvedValue({ data: { data: { id: 11, version: 3 } } });
    const out = await documentsApi.update("42", 11, {
      title: "Handbook",
      content: "v2 text",
      require_ack: true,
      audience_filter: "dept:BOH",
    });
    expect(mocked.put).toHaveBeenCalledWith("/inside/businesses/42/documents/11", {
      title: "Handbook",
      content: "v2 text",
      require_ack: true,
      audience_filter: "dept:BOH",
    });
    expect(out.version).toBe(3);
  });
});

describe("recognitionApi", () => {
  it("lists shout-outs", async () => {
    mocked.get.mockResolvedValue({ data: { data: [{ id: 1, message: "great close" }] } });
    const out = await recognitionApi.list("42");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/shoutouts");
    expect(out[0].message).toBe("great close");
  });

  it("sends a shout-out", async () => {
    mocked.post.mockResolvedValue({ data: { data: { id: 1 } } });
    await recognitionApi.send("42", { to_staff_id: 3, message: "great close", emoji: "🙌", visibility: "team" });
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/shoutouts", {
      to_staff_id: 3,
      message: "great close",
      emoji: "🙌",
      visibility: "team",
    });
  });
});

describe("pollsApi", () => {
  it("lists polls with options and the caller's vote", async () => {
    mocked.get.mockResolvedValue({
      data: { data: [{ id: 5, question: "Pizza night?", options: [], my_vote: null }] },
    });
    const out = await pollsApi.list("42");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/polls");
    expect(out[0].question).toBe("Pizza night?");
  });

  it("casts a vote and reads back the results", async () => {
    mocked.post.mockResolvedValue({ data: { data: { poll_id: 5, options: [] } } });
    const res = await pollsApi.vote("42", 5, 12);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/polls/5/vote", { option_id: 12 });
    expect(res.poll_id).toBe(5);
  });

  it("fetches results and closes a poll", async () => {
    mocked.get.mockResolvedValue({ data: { data: { poll_id: 5, options: [] } } });
    await pollsApi.results("42", 5);
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/polls/5/results");
    mocked.post.mockResolvedValue({ data: { success: true } });
    await pollsApi.close("42", 5);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/polls/5/close", {});
  });
});
