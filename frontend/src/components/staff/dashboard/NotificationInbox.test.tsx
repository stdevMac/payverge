/** @jest-environment jsdom */

import React from "react";
import { render, fireEvent, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NotificationInbox } from "./NotificationInbox";
import { notificationsApi } from "@/api/notifications";

jest.mock("@/api/notifications");

const labels = {
  bellAria: "Notifications",
  title: "Notifications",
  empty: "No notifications",
  markAll: "Mark all read",
};

function wrap(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe("NotificationInbox a11y dismissal", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (notificationsApi.list as jest.Mock).mockResolvedValue([]);
    (notificationsApi.markRead as jest.Mock).mockResolvedValue(0);
  });

  it("closes on Escape key", () => {
    const onClose = jest.fn();
    wrap(<NotificationInbox businessId="1" labels={labels} locale="en" onClose={onClose} />);
    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("closes on outside click (pointerdown on document.body)", () => {
    const onClose = jest.fn();
    wrap(<NotificationInbox businessId="1" labels={labels} locale="en" onClose={onClose} />);
    fireEvent.pointerDown(document.body);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("does NOT close on pointerdown inside the dialog", () => {
    const onClose = jest.fn();
    const { getByRole } = wrap(
      <NotificationInbox businessId="1" labels={labels} locale="en" onClose={onClose} />,
    );
    const dialog = getByRole("dialog");
    fireEvent.pointerDown(dialog);
    expect(onClose).not.toHaveBeenCalled();
  });

  it("is money-free", async () => {
    const onClose = jest.fn();
    (notificationsApi.list as jest.Mock).mockResolvedValue([
      { id: 1, kind: "shift.assigned", title: "T", body: "b", url: "/x", read_at: null, created_at: new Date().toISOString() },
    ]);
    const { container } = wrap(
      <NotificationInbox businessId="1" labels={labels} locale="en" onClose={onClose} />,
    );
    await waitFor(() => expect(container.textContent || "").not.toMatch(/\$/));
  });
});
