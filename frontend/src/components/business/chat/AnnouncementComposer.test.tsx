/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import AnnouncementComposer from "./AnnouncementComposer";
import { chatApi } from "@/api/chat";
import { positionsApi } from "@/api/positions";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));
const mockShowSuccess = jest.fn();
const mockShowError = jest.fn();
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: mockShowSuccess,
    showError: mockShowError,
    showInfo: jest.fn(),
    showWarning: jest.fn(),
    showToast: jest.fn(),
  }),
}));
jest.mock("@/api/chat", () => ({
  chatApi: {
    createAnnouncement: jest.fn(),
    listAnnouncements: jest.fn(),
    announcementAcks: jest.fn(),
    announcementAckSummaries: jest.fn(),
    updateAnnouncement: jest.fn(),
    deleteAnnouncement: jest.fn(),
  },
}));
jest.mock("@/api/positions", () => ({ positionsApi: { list: jest.fn() } }));
// Capture the composer's realtime handlers so tests can simulate SSE nudges.
let realtimeHandlers: {
  onChatAnnouncementAck?: (p: { announcement_id?: number }) => void;
} = {};
jest.mock("@/hooks/useStaffRealtime", () => ({
  useStaffRealtime: (opts: typeof realtimeHandlers) => {
    realtimeHandlers = opts;
    return { degraded: false, blocked: false, reconnect: jest.fn() };
  },
}));

const mocked = chatApi as unknown as {
  createAnnouncement: jest.Mock;
  listAnnouncements: jest.Mock;
  announcementAcks: jest.Mock;
  announcementAckSummaries: jest.Mock;
  updateAnnouncement: jest.Mock;
  deleteAnnouncement: jest.Mock;
};
const mockedPositions = positionsApi as unknown as { list: jest.Mock };

// A "just now" ISO so require_ack notices are within the 14-day poll window.
const recentIso = () => new Date(Date.now() - 60_000).toISOString();

function renderComposer() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <AnnouncementComposer businessId="42" />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  mocked.listAnnouncements.mockResolvedValue({ announcements: [], acked: {} });
  mocked.announcementAckSummaries.mockResolvedValue({});
  mockedPositions.list.mockResolvedValue([
    { id: 9, business_id: 42, name: "Server", color_hex: "#1a6b6a", department: "FOH", is_active: true, sort_order: 0 },
  ]);
});

describe("AnnouncementComposer", () => {
  it("posts an announcement with the composed body + require_ack", async () => {
    mocked.createAnnouncement.mockResolvedValue({ id: 1, title: "Shift change" });

    renderComposer();
    await waitFor(() => expect(mocked.listAnnouncements).toHaveBeenCalled());

    fireEvent.change(
      screen.getByPlaceholderText("dashboardChat.announce.titlePlaceholder"),
      { target: { value: "Shift change" } },
    );
    fireEvent.change(
      screen.getByPlaceholderText("dashboardChat.announce.contentPlaceholder"),
      { target: { value: "Come in early" } },
    );
    // Require confirmation.
    await userEvent.click(
      screen.getByRole("switch", { name: "dashboardChat.announce.requireAck" }),
    );

    await userEvent.click(
      screen.getByRole("button", { name: "dashboardChat.announce.post" }),
    );

    await waitFor(() =>
      expect(mocked.createAnnouncement).toHaveBeenCalledWith("42", {
        title: "Shift change",
        content: "Come in early",
        require_ack: true,
        audience_filter: "all",
      }),
    );
    await waitFor(() => expect(mockShowSuccess).toHaveBeenCalled());
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("loads the ack summaries in ONE batch call for visible require_ack notices", async () => {
    mocked.listAnnouncements.mockResolvedValue({
      announcements: [
        {
          id: 40,
          business_id: 42,
          author_staff_id: 1,
          title: "Dinner rush",
          content: "VIP at 7:30",
          require_ack: true,
          audience_filter: "all",
          created_at: recentIso(),
          updated_at: "",
        },
        {
          id: 41,
          business_id: 42,
          author_staff_id: 1,
          title: "No ack needed",
          content: "fyi",
          require_ack: false,
          audience_filter: "all",
          created_at: recentIso(),
          updated_at: "",
        },
      ],
      acked: {},
    });
    mocked.announcementAckSummaries.mockResolvedValue({
      "40": { announcement_id: 40, acked: 1, total_eligible: 5, first_acker_names: ["Ana"] },
    });

    renderComposer();
    await waitFor(() =>
      expect(mocked.announcementAckSummaries).toHaveBeenCalledWith("42", [40]),
    );
    // The batch call carries ONLY the require_ack notice (41 is excluded).
    // The per-announcement roster read is NOT fired on mount (loads on expand).
    expect(mocked.announcementAcks).not.toHaveBeenCalled();
    expect(await screen.findByText("dashboardChat.announce.ackRoster")).toBeInTheDocument();
  });

  it("refetches the batch summaries when an ack SSE nudge lands", async () => {
    mocked.listAnnouncements.mockResolvedValue({
      announcements: [
        {
          id: 40,
          business_id: 42,
          author_staff_id: 1,
          title: "Dinner rush",
          content: "VIP at 7:30",
          require_ack: true,
          audience_filter: "all",
          created_at: recentIso(),
          updated_at: "",
        },
      ],
      acked: {},
    });
    mocked.announcementAckSummaries.mockResolvedValue({
      "40": { announcement_id: 40, acked: 1, total_eligible: 5, first_acker_names: ["Ana"] },
    });

    renderComposer();
    await waitFor(() => expect(mocked.announcementAckSummaries).toHaveBeenCalled());
    const before = mocked.announcementAckSummaries.mock.calls.length;

    realtimeHandlers.onChatAnnouncementAck?.({ announcement_id: 40 });
    await waitFor(() =>
      expect(mocked.announcementAckSummaries.mock.calls.length).toBeGreaterThan(before),
    );
  });

  it("edits an announcement in place via the edit action", async () => {
    mocked.listAnnouncements.mockResolvedValue({
      announcements: [
        {
          id: 7,
          business_id: 42,
          author_staff_id: 1,
          title: "Old title",
          content: "old body",
          require_ack: false,
          audience_filter: "all",
          created_at: recentIso(),
          updated_at: "",
        },
      ],
      acked: {},
    });
    mocked.updateAnnouncement.mockResolvedValue({ id: 7, title: "New title" });

    renderComposer();
    await userEvent.click(await screen.findByRole("button", { name: "dashboardChat.announce.edit" }));

    // The form is pre-filled; change the title and save.
    fireEvent.change(
      screen.getByPlaceholderText("dashboardChat.announce.titlePlaceholder"),
      { target: { value: "New title" } },
    );
    await userEvent.click(
      screen.getByRole("button", { name: "dashboardChat.announce.saveEdit" }),
    );

    await waitFor(() =>
      expect(mocked.updateAnnouncement).toHaveBeenCalledWith("42", 7, {
        title: "New title",
        content: "old body",
        require_ack: false,
        audience_filter: "all",
      }),
    );
  });

  it("confirms before deleting an announcement", async () => {
    mocked.listAnnouncements.mockResolvedValue({
      announcements: [
        {
          id: 9,
          business_id: 42,
          author_staff_id: 1,
          title: "Doomed",
          content: "x",
          require_ack: false,
          audience_filter: "all",
          created_at: recentIso(),
          updated_at: "",
        },
      ],
      acked: {},
    });
    mocked.deleteAnnouncement.mockResolvedValue(undefined);

    renderComposer();
    await userEvent.click(await screen.findByRole("button", { name: "dashboardChat.announce.delete" }));

    // The confirm modal is open; delete only fires on confirm.
    expect(mocked.deleteAnnouncement).not.toHaveBeenCalled();
    const confirmBtn = await screen.findByRole("button", {
      name: "dashboardChat.announce.deleteConfirmAction",
    });
    await userEvent.click(confirmBtn);
    await waitFor(() => expect(mocked.deleteAnnouncement).toHaveBeenCalledWith("42", 9));
  });

  it("fetches a one-shot summary for require-ack announcements older than 14 days", async () => {
    const oldIso = new Date(Date.now() - 15 * 24 * 60 * 60 * 1000).toISOString();
    mocked.listAnnouncements.mockResolvedValue({
      announcements: [
        {
          id: 99,
          business_id: 42,
          author_staff_id: 1,
          title: "Demo dinner rush",
          content: "old notice",
          require_ack: true,
          audience_filter: "all",
          created_at: oldIso,
          updated_at: "",
        },
      ],
      acked: {},
    });
    mocked.announcementAckSummaries.mockResolvedValue({
      "99": {
        announcement_id: 99,
        acked: 3,
        total_eligible: 5,
        first_acker_names: ["Ana"],
      },
    });

    renderComposer();
    await waitFor(() =>
      expect(mocked.announcementAckSummaries).toHaveBeenCalledWith("42", [99]),
    );
    expect(await screen.findByText("dashboardChat.announce.ackRoster")).toBeInTheDocument();
    expect(screen.queryByText("dashboardChat.announce.feedLoading")).not.toBeInTheDocument();

    const before = mocked.announcementAckSummaries.mock.calls.length;
    await act(async () => {
      await new Promise((r) => setTimeout(r, 35));
    });
    expect(mocked.announcementAckSummaries.mock.calls.length).toBe(before);
  });

  it("settles an old require-ack card when the one-shot summary has no row", async () => {
    const oldIso = new Date(Date.now() - 20 * 24 * 60 * 60 * 1000).toISOString();
    mocked.listAnnouncements.mockResolvedValue({
      announcements: [
        {
          id: 77,
          business_id: 42,
          author_staff_id: 1,
          title: "Ancient notice",
          content: "x",
          require_ack: true,
          audience_filter: "all",
          created_at: oldIso,
          updated_at: "",
        },
      ],
      acked: {},
    });
    mocked.announcementAckSummaries.mockResolvedValue({});

    renderComposer();
    expect(
      await screen.findByText("dashboardChat.announce.ackHistoryUnavailable"),
    ).toBeInTheDocument();
    expect(screen.queryByText("dashboardChat.announce.feedLoading")).not.toBeInTheDocument();
  });

  it("explains the missing department options when no departments exist", async () => {
    mockedPositions.list.mockResolvedValue([
      { id: 9, business_id: 42, name: "Server", color_hex: "#1a6b6a", department: "", is_active: true, sort_order: 0 },
    ]);
    renderComposer();
    await waitFor(() =>
      expect(
        screen.getByText("dashboardChat.announce.noDeptsHint"),
      ).toBeInTheDocument(),
    );
  });

  it("blocks an empty title and never calls the API", async () => {
    renderComposer();
    await waitFor(() => expect(mocked.listAnnouncements).toHaveBeenCalled());

    await userEvent.click(
      screen.getByRole("button", { name: "dashboardChat.announce.post" }),
    );

    expect(
      screen.getByText("dashboardChat.announce.titleRequired"),
    ).toBeInTheDocument();
    expect(mocked.createAnnouncement).not.toHaveBeenCalled();
  });
});
