import { chatApi } from "@/api/chat";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn(), post: jest.fn(), put: jest.fn(), delete: jest.fn() },
}));

const mockGet = axiosInstance.get as jest.Mock;
const mockPost = axiosInstance.post as jest.Mock;
const mockPut = axiosInstance.put as jest.Mock;
const mockDelete = axiosInstance.delete as jest.Mock;

describe("chatApi", () => {
  beforeEach(() => jest.clearAllMocks());

  it("lists channels with the unread-badge map (missing key → 0)", async () => {
    mockGet.mockResolvedValue({
      data: { data: [{ id: 1, type: "role" }], unread: { "1": 2 } },
    });
    const res = await chatApi.listChannels("42");
    expect(mockGet).toHaveBeenCalledWith("/inside/businesses/42/chat/channels");
    expect(res.channels[0].id).toBe(1);
    expect(res.unread["1"]).toBe(2);
  });

  it("defaults the unread map to {} when the server omits it", async () => {
    mockGet.mockResolvedValue({ data: { data: [{ id: 9 }] } });
    const res = await chatApi.listChannels("42");
    expect(res.unread).toEqual({});
  });

  it("lists messages with the keyset cursor + limit params", async () => {
    mockGet.mockResolvedValue({ data: { data: [{ id: 9 }] } });
    const res = await chatApi.listMessages("42", 7, { cursor: 20, limit: 50 });
    expect(mockGet).toHaveBeenCalledWith(
      "/inside/businesses/42/chat/channels/7/messages",
      { params: { cursor: 20, limit: 50 } },
    );
    expect(res[0].id).toBe(9);
  });

  it("posts a message with a content body", async () => {
    mockPost.mockResolvedValue({ data: { data: { id: 3, content: "hi" } } });
    const res = await chatApi.postMessage("42", 7, "hi");
    expect(mockPost).toHaveBeenCalledWith(
      "/inside/businesses/42/chat/channels/7/messages",
      { content: "hi" },
    );
    expect(res.content).toBe("hi");
  });

  it("marks a channel read, defaulting an empty id to 0 (newest)", async () => {
    mockPost.mockResolvedValue({
      data: { data: { channel_id: 7, last_read_message_id: 9 } },
    });
    await chatApi.markRead("42", 7);
    expect(mockPost).toHaveBeenCalledWith(
      "/inside/businesses/42/chat/channels/7/read",
      { last_read_message_id: 0 },
    );
  });

  it("opens a DM with content (201 channel + message)", async () => {
    mockPost.mockResolvedValue({
      data: { data: { channel: { id: 5 }, message: { id: 8 } } },
    });
    const res = await chatApi.dm("42", 3, "yo");
    expect(mockPost).toHaveBeenCalledWith("/inside/businesses/42/chat/dm/3", {
      content: "yo",
    });
    expect(res.message?.id).toBe(8);
  });

  it("ensures a DM channel with no content (empty body, 200)", async () => {
    mockPost.mockResolvedValue({ data: { data: { channel: { id: 5 } } } });
    const res = await chatApi.dm("42", 3);
    expect(mockPost).toHaveBeenCalledWith("/inside/businesses/42/chat/dm/3", {});
    expect(res.channel.id).toBe(5);
    expect(res.message).toBeUndefined();
  });

  it("moderates (soft-deletes) a message", async () => {
    mockDelete.mockResolvedValue({ data: { success: true } });
    await chatApi.deleteMessage("42", 99);
    expect(mockDelete).toHaveBeenCalledWith("/inside/businesses/42/chat/messages/99");
  });

  it("creates an announcement with the full body", async () => {
    mockPost.mockResolvedValue({ data: { data: { id: 1, title: "T" } } });
    const res = await chatApi.createAnnouncement("42", {
      title: "T",
      content: "C",
      require_ack: true,
      audience_filter: "role:server",
    });
    expect(mockPost).toHaveBeenCalledWith("/inside/businesses/42/announcements", {
      title: "T",
      content: "C",
      require_ack: true,
      audience_filter: "role:server",
    });
    expect(res.title).toBe("T");
  });

  it("lists announcements (audience-filtered feed + caller ack state)", async () => {
    mockGet.mockResolvedValue({ data: { data: [{ id: 2 }], acked: { "2": true } } });
    const res = await chatApi.listAnnouncements("42");
    expect(mockGet).toHaveBeenCalledWith("/inside/businesses/42/announcements");
    expect(res.announcements[0].id).toBe(2);
    expect(res.acked["2"]).toBe(true);
  });

  it("defaults the ack map to empty when the server omits it", async () => {
    mockGet.mockResolvedValue({ data: { data: [{ id: 3 }] } });
    const res = await chatApi.listAnnouncements("42");
    expect(res.announcements[0].id).toBe(3);
    expect(res.acked).toEqual({});
  });

  it("acks an announcement (idempotent, no body)", async () => {
    mockPost.mockResolvedValue({ data: { data: { announcement_id: 2, staff_id: 7 } } });
    const res = await chatApi.ackAnnouncement("42", 2);
    expect(mockPost).toHaveBeenCalledWith("/inside/businesses/42/announcements/2/ack");
    expect(res.staff_id).toBe(7);
  });

  it("fetches the ack roster (X of Y + unacked)", async () => {
    mockGet.mockResolvedValue({
      data: {
        data: {
          announcement_id: 2,
          acked: 3,
          total_eligible: 5,
          unacked: [{ staff_id: 9, name: "Mara" }],
        },
      },
    });
    const res = await chatApi.announcementAcks("42", 2);
    expect(mockGet).toHaveBeenCalledWith("/inside/businesses/42/announcements/2/acks");
    expect(res.total_eligible).toBe(5);
    expect(res.unacked[0].name).toBe("Mara");
  });

  it("fetches batch ack summaries for a bounded id list (one call)", async () => {
    mockGet.mockResolvedValue({
      data: {
        data: {
          "2": { announcement_id: 2, acked: 1, total_eligible: 5, first_acker_names: ["Ana"] },
          "3": { announcement_id: 3, acked: 0, total_eligible: 5, first_acker_names: [] },
        },
      },
    });
    const res = await chatApi.announcementAckSummaries("42", [2, 3]);
    expect(mockGet).toHaveBeenCalledWith("/inside/businesses/42/announcements/ack-summaries", {
      params: { ids: "2,3" },
    });
    expect(res["2"].acked).toBe(1);
    expect(res["2"].first_acker_names).toEqual(["Ana"]);
    expect(res["3"].total_eligible).toBe(5);
  });

  it("short-circuits an empty ack-summary id list without a request", async () => {
    const res = await chatApi.announcementAckSummaries("42", []);
    expect(mockGet).not.toHaveBeenCalled();
    expect(res).toEqual({});
  });

  it("edits an announcement in place via PUT", async () => {
    mockPut.mockResolvedValue({ data: { data: { id: 8, title: "New" } } });
    const res = await chatApi.updateAnnouncement("42", 8, {
      title: "New",
      content: "body",
      require_ack: true,
      audience_filter: "role:server",
    });
    expect(mockPut).toHaveBeenCalledWith("/inside/businesses/42/announcements/8", {
      title: "New",
      content: "body",
      require_ack: true,
      audience_filter: "role:server",
    });
    expect(res.title).toBe("New");
  });

  it("deletes an announcement", async () => {
    mockDelete.mockResolvedValue({ data: { success: true } });
    await chatApi.deleteAnnouncement("42", 8);
    expect(mockDelete).toHaveBeenCalledWith("/inside/businesses/42/announcements/8");
  });
});
