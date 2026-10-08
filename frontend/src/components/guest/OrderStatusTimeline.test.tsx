/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import OrderStatusTimeline from "@/components/guest/OrderStatusTimeline";

const t = (key: string) => key;

describe("OrderStatusTimeline", () => {
  it("renders all four steps", () => {
    render(<OrderStatusTimeline status="pending" t={t} />);
    expect(screen.getByText("orders.statusSent")).toBeInTheDocument();
    expect(screen.getByText("orders.statusAccepted")).toBeInTheDocument();
    expect(screen.getByText("orders.statusInKitchen")).toBeInTheDocument();
    expect(screen.getByText("orders.statusReady")).toBeInTheDocument();
  });

  it.each([
    ["pending", "orders.statusSent"],
    ["approved", "orders.statusAccepted"],
    ["in_kitchen", "orders.statusInKitchen"],
    ["ready", "orders.statusReady"],
  ])("marks the %s step as current", (status, expectedLabel) => {
    render(<OrderStatusTimeline status={status} t={t} />);
    const current = screen.getByText(expectedLabel);
    expect(current).toHaveAttribute("aria-current", "step");
  });

  it("announces when the order becomes ready", () => {
    const { rerender } = render(<OrderStatusTimeline status="in_kitchen" t={t} />);
    const live = screen.getByTestId("order-status-live");
    expect(live).toHaveAttribute("role", "status");
    expect(live).toHaveAttribute("aria-live", "polite");
    expect(live).toHaveTextContent("");

    rerender(<OrderStatusTimeline status="ready" t={t} />);
    expect(live).toHaveTextContent("orders.readyAnnouncement");
  });

  it("uses text-xs and ink-600 for unreached step labels and badges", () => {
    render(<OrderStatusTimeline status="pending" t={t} />);
    const label = screen.getByText("orders.statusReady");
    expect(label.className).toContain("text-xs");
    expect(label.className).toContain("text-ink-600");
    expect(label.className).not.toContain("text-[10px]");
    expect(label.className).not.toContain("text-ink-400");

    const badge = label.parentElement?.querySelector(
      "span[aria-hidden='true']",
    );
    expect(badge?.className).toContain("text-ink-600");
    expect(badge?.className).not.toContain("text-ink-400");
  });
});

