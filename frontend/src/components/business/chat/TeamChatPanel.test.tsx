/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import TeamChatPanel from "./TeamChatPanel";
import { chatApi } from "@/api/chat";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: jest.fn(),
    showError: jest.fn(),
    showInfo: jest.fn(),
    showWarning: jest.fn(),
    showToast: jest.fn(),
  }),
}));
jest.mock("@/hooks/useStaffRealtime", () => ({
  useStaffRealtime: () => ({ degraded: false, blocked: false, reconnect: jest.fn() }),
}));
jest.mock("@/api/chat", () => ({
  chatApi: {
    listChannels: jest.fn(),
    listMessages: jest.fn(),
    postMessage: jest.fn(),
    deleteMessage: jest.fn(),
    markRead: jest.fn(),
  },
}));

const mocked = chatApi as unknown as {
  listChannels: jest.Mock;
  listMessages: jest.Mock;
  postMessage: jest.Mock;
  deleteMessage: jest.Mock;
  markRead: jest.Mock;
};

const roleChannel = {
  id: 7,
  business_id: 42,
  type: "role",
  name: "Servers",
  ref_key: "role:server",
  is_archived: false,
  created_by_staff_id: null,
  created_at: "",
  updated_at: "",
};
const dmChannel = {
  id: 8,
  business_id: 42,
  type: "direct",
  name: "",
  ref_key: "dm:1:2",
  is_archived: false,
  created_by_staff_id: null,
  created_at: "",
  updated_at: "",
};

function renderPanel(canModerate = true, channels = [roleChannel]) {
  mocked.listChannels.mockResolvedValue({ channels, unread: {}, previews: {} });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <TeamChatPanel businessId="42" canModerate={canModerate} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  mocked.markRead.mockResolvedValue({
    channel_id: 7,
    staff_id: 3,
    business_id: 42,
    last_read_message_id: 100,
    last_read_at: "",
  });
  mocked.listMessages.mockResolvedValue([
    {
      id: 100,
      business_id: 42,
      channel_id: 7,
      sender_staff_id: 3,
      sender_name: "Mara",
      content: "hello team",
      parent_id: null,
      attachment_url: "",
      created_at: "2026-07-01T20:00:00Z",
      updated_at: "",
    },
  ]);
});

describe("TeamChatPanel", () => {
  it("shows the unread COUNT on a channel row, not just a dot", async () => {
    mocked.listChannels.mockResolvedValue({
      channels: [roleChannel],
      unread: { "7": 3 },
    });
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={qc}>
        <TeamChatPanel businessId="42" canModerate />
      </QueryClientProvider>,
    );
    await waitFor(() =>
      expect(screen.getByText("dashboardChat.chat.roles.server")).toBeInTheDocument(),
    );
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("shows the latest-message preview on a channel row, kind label as fallback", async () => {
    mocked.listChannels.mockResolvedValue({
      channels: [roleChannel, dmChannel],
      unread: {},
      previews: {
        "7": {
          snippet: "see you at the pass",
          sender_name: "Ana",
          created_at: "2026-07-03T12:00:00Z",
        },
      },
    });
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={qc}>
        <TeamChatPanel businessId="42" canModerate />
      </QueryClientProvider>,
    );
    await waitFor(() =>
      expect(screen.getByText("dashboardChat.chat.roles.server")).toBeInTheDocument(),
    );
    // The row with a preview speaks its last message…
    expect(screen.getByText(/Ana: see you at the pass/)).toBeInTheDocument();
    // …the row without one falls back to its kind label.
    expect(screen.getByText("dashboardChat.chat.kind.direct")).toBeInTheDocument();
  });

  it("opens a role channel and moderates a message (chat:moderate), no dollars", async () => {
    mocked.deleteMessage.mockResolvedValue(undefined);

    renderPanel(true);

    // Channel list resolves the localized role-channel name.
    const channelBtn = await screen.findByText("dashboardChat.chat.roles.server");
    await userEvent.click(channelBtn);

    await waitFor(() => expect(screen.getByText("hello team")).toBeInTheDocument());

    // Moderator delete affordance is present on a non-DM channel.
    const del = screen.getByRole("button", { name: "dashboardChat.chat.delete" });
    await userEvent.click(del);

    // Wave 4: shared ConfirmationModal replaces window.confirm.
    const confirm = await screen.findByRole("button", {
      name: "dashboardChat.chat.deleteConfirmAction",
    });
    await userEvent.click(confirm);

    await waitFor(() => expect(mocked.deleteMessage).toHaveBeenCalledWith("42", 100));
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("hides the delete affordance on a DM channel even for a moderator", async () => {
    renderPanel(true, [dmChannel]);

    const channelBtn = await screen.findByText("dashboardChat.chat.fallback.direct");
    await userEvent.click(channelBtn);

    await waitFor(() => expect(screen.getByText("hello team")).toBeInTheDocument());
    // No delete button on a DM thread (the server 403s a DM moderation delete).
    expect(
      screen.queryByRole("button", { name: "dashboardChat.chat.delete" }),
    ).not.toBeInTheDocument();
  });

  it("hides the delete affordance for a non-moderator", async () => {
    renderPanel(false);

    const channelBtn = await screen.findByText("dashboardChat.chat.roles.server");
    await userEvent.click(channelBtn);

    await waitFor(() => expect(screen.getByText("hello team")).toBeInTheDocument());
    expect(
      screen.queryByRole("button", { name: "dashboardChat.chat.delete" }),
    ).not.toBeInTheDocument();
  });
});
