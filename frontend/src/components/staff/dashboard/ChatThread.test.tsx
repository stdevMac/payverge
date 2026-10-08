/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import ChatThread, { type ChatThreadLabels } from "./ChatThread";
import { chatApi } from "@/api/chat";

jest.mock("@/api/chat", () => ({
  chatApi: {
    listMessages: jest.fn(),
    postMessage: jest.fn(),
    markRead: jest.fn(),
  },
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
// Avoid opening a real EventSource in jsdom.
jest.mock("@/hooks/useStaffRealtime", () => ({
  useStaffRealtime: () => ({ degraded: false, blocked: false, reconnect: jest.fn() }),
}));

const mocked = chatApi as unknown as {
  listMessages: jest.Mock;
  postMessage: jest.Mock;
  markRead: jest.Mock;
};

const labels: ChatThreadLabels = {
  back: "Channels",
  loading: "Loading messages…",
  error: "Could not load",
  emptyTitle: "No messages yet",
  emptySubtitle: "Say something",
  loadOlder: "Load older messages",
  you: "You",
  composerPlaceholder: "Write a message…",
  send: "Send",
  sending: "Sending…",
  sendError: "Could not send",
};

function renderThread() {
  return render(
    <ChatThread
      businessId="42"
      channelId={7}
      currentStaffId={5}
      title="Servers"
      onBack={jest.fn()}
      locale="en"
      labels={labels}
    />,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  mocked.markRead.mockResolvedValue({});
});

describe("ChatThread", () => {
  it("renders messages oldest→newest, labels own messages, marks read, no dollars", async () => {
    // Backend returns newest-first; the thread reverses to a transcript.
    mocked.listMessages.mockResolvedValue([
      { id: 11, sender_staff_id: 9, sender_name: "Mara", content: "newer one", created_at: "2026-07-01T20:00:00Z" },
      { id: 10, sender_staff_id: 5, sender_name: "Self", content: "older mine", created_at: "2026-07-01T19:00:00Z" },
    ]);

    renderThread();

    await waitFor(() => expect(screen.getByText("newer one")).toBeInTheDocument());
    expect(screen.getByText("older mine")).toBeInTheDocument();
    // My own message (sender_staff_id === currentStaffId) is labelled "You".
    expect(screen.getByText("You")).toBeInTheDocument();
    expect(screen.getByText("Mara")).toBeInTheDocument();
    // Viewing the channel marks it read.
    expect(mocked.markRead).toHaveBeenCalledWith("42", 7);
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("sends a message via the composer (chat:send)", async () => {
    mocked.listMessages.mockResolvedValue([]);
    mocked.postMessage.mockResolvedValue({
      id: 99,
      sender_staff_id: 5,
      sender_name: "Self",
      content: "hello team",
      created_at: "2026-07-01T21:00:00Z",
    });

    renderThread();

    await waitFor(() => expect(screen.getByText("No messages yet")).toBeInTheDocument());

    fireEvent.change(screen.getByLabelText("Write a message…"), {
      target: { value: "hello team" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(mocked.postMessage).toHaveBeenCalledWith("42", 7, "hello team"),
    );
    await waitFor(() => expect(screen.getByText("hello team")).toBeInTheDocument());
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("loads an older page via the keyset cursor", async () => {
    // A full page (50) marks hasMore → the "Load older" affordance appears.
    const firstPage = Array.from({ length: 50 }, (_, i) => ({
      id: 100 - i, // newest-first: 100…51
      sender_staff_id: 9,
      sender_name: "Mara",
      content: `m${100 - i}`,
      created_at: "2026-07-01T20:00:00Z",
    }));
    const olderPage = [
      {
        id: 50,
        sender_staff_id: 9,
        sender_name: "Mara",
        content: "the oldest",
        created_at: "2026-06-30T20:00:00Z",
      },
    ];
    mocked.listMessages.mockResolvedValueOnce(firstPage).mockResolvedValueOnce(olderPage);

    renderThread();

    await waitFor(() => expect(screen.getByText("m100")).toBeInTheDocument());
    // Oldest displayed is id 51 → the keyset cursor for the older page.
    fireEvent.click(screen.getByRole("button", { name: "Load older messages" }));

    await waitFor(() =>
      expect(mocked.listMessages).toHaveBeenLastCalledWith("42", 7, {
        cursor: 51,
        limit: 50,
      }),
    );
    await waitFor(() => expect(screen.getByText("the oldest")).toBeInTheDocument());
  });
});
