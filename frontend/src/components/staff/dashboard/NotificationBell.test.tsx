/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import NotificationBell from "./NotificationBell";
import { notificationsApi } from "@/api/notifications";

jest.mock("@/api/notifications");
jest.mock("@/hooks/useSSEEvents", () => ({ useSSEEvents: () => ({ degraded: false, blocked: false, reconnect: jest.fn() }) }));

const labels = { bellAria: "Notifications", empty: "No notifications", markAll: "Mark all read", title: "Notifications" };

function wrap(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

it("renders the unread count badge", async () => {
  (notificationsApi.unreadCount as jest.Mock).mockResolvedValue(4);
  wrap(<NotificationBell businessId="1" staffId={7} labels={labels} locale="en" />);
  await waitFor(() => expect(screen.getByText("4")).toBeInTheDocument());
});

it("is money-free", async () => {
  (notificationsApi.unreadCount as jest.Mock).mockResolvedValue(0);
  const { container } = wrap(<NotificationBell businessId="1" staffId={7} labels={labels} locale="en" />);
  await waitFor(() => expect(container.textContent || "").not.toMatch(/\$\d/));
});

it("closes the inbox when the bell is clicked while open (regression: trigger inside boundary)", async () => {
  (notificationsApi.unreadCount as jest.Mock).mockResolvedValue(3);
  (notificationsApi.list as jest.Mock).mockResolvedValue([
    { id: 1, kind: "shift.assigned", title: "Shift", body: "b", url: "/x", read_at: null, created_at: new Date().toISOString() },
  ]);
  (notificationsApi.markRead as jest.Mock).mockResolvedValue(1);

  wrap(<NotificationBell businessId="1" staffId={7} labels={labels} locale="en" />);

  // Wait for the bell to render with unread badge.
  const bell = await screen.findByLabelText(labels.bellAria);

  // First click: open the inbox.
  fireEvent.click(bell);
  await screen.findByRole("dialog");

  // Second click: simulate the real browser event sequence (pointerDown first,
  // then click). The pointerDown triggers the outside-click a11y handler which
  // closes the inbox, and without the fix the bell's click handler re-toggles
  // it open. This must FAIL before the containerRef fix.
  fireEvent.pointerDown(bell);
  fireEvent.click(bell);
  await waitFor(() => {
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});
