import { notificationsApi } from "@/api/notifications";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn(), post: jest.fn() },
}));

const mocked = axiosInstance as unknown as { get: jest.Mock; post: jest.Mock };

describe("notificationsApi", () => {
  beforeEach(() => jest.clearAllMocks());

  it("lists the caller's own notifications", async () => {
    mocked.get.mockResolvedValue({
      data: { data: [{ id: 2, kind: "shift.assigned", title: "T", body: "b", url: "/x", read_at: null }] },
    });
    const out = await notificationsApi.list("42");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/me/notifications", {
      params: { limit: undefined, before: undefined },
    });
    expect(out[0].id).toBe(2);
  });

  it("reads the unread count", async () => {
    mocked.get.mockResolvedValue({ data: { data: { count: 3 } } });
    expect(await notificationsApi.unreadCount("42")).toBe(3);
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/me/notifications/unread-count");
  });

  it("marks all read", async () => {
    mocked.post.mockResolvedValue({ data: { data: { updated: 3 } } });
    expect(await notificationsApi.markRead("42", { all: true })).toBe(3);
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/me/notifications/read", { all: true });
  });
});
