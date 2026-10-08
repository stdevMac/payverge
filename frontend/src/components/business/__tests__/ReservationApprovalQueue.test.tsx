/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import ReservationApprovalQueue, {
  approvalDeadline,
} from "../ReservationApprovalQueue";
import type { Reservation } from "@/api/reservations";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) =>
    key.endsWith(".showingOf")
      ? "Showing {shown} of {total}"
      : key.endsWith(".loadMore")
        ? "Load more requests"
        : key,
}));

const pending = (id: number, startMs: number): Reservation =>
  ({
    id,
    business_id: 1,
    customer_name: `Guest ${id}`,
    party_size: 2,
    reservation_time: new Date(Date.now() + startMs).toISOString(),
    duration: 90,
    status: "pending",
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  }) as Reservation;

describe("ReservationApprovalQueue", () => {
  it("renders one row per pending request with approve/decline, sorted by deadline", () => {
    const onApprove = jest.fn();
    const onDecline = jest.fn();
    // id 1 starts next week (deadline = created+24h); id 2 starts in 3h (deadline = reservation-2h, sooner)
    render(
      <ReservationApprovalQueue
        pending={[pending(1, 7 * 86_400_000), pending(2, 3 * 3_600_000)]}
        onApprove={onApprove}
        onDecline={onDecline}
        busyId={null}
      />,
    );
    const names = screen.getAllByText(/Guest \d/).map((n) => n.textContent);
    expect(names).toEqual(["Guest 2", "Guest 1"]); // soonest deadline first

    fireEvent.click(screen.getAllByRole("button", { name: /approve/i })[0]);
    expect(onApprove).toHaveBeenCalledWith(2);

    fireEvent.click(screen.getAllByRole("button", { name: /decline/i })[0]);
    expect(onDecline).toHaveBeenCalledWith(2);
  });

  it("renders nothing when there are no pending requests", () => {
    const { container } = render(
      <ReservationApprovalQueue
        pending={[]}
        onApprove={jest.fn()}
        onDecline={jest.fn()}
        busyId={null}
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("disables the busy row's buttons", () => {
    render(
      <ReservationApprovalQueue
        pending={[pending(3, 3 * 3_600_000)]}
        onApprove={jest.fn()}
        onDecline={jest.fn()}
        busyId={3}
      />,
    );
    expect(screen.getByRole("button", { name: /decline/i })).toBeDisabled();
  });

  describe("approvalDeadline (mirror of backend ReservationApprovalDeadline)", () => {
    const created = new Date("2026-06-10T12:00:00Z").toISOString();
    const at = (iso: string) => new Date(iso).getTime();

    it("uses reservation_time - 2h when within the created+30m..created+24h window", () => {
      // starts in 6h -> deadline = start - 2h = created + 4h
      const start = new Date("2026-06-10T18:00:00Z").toISOString();
      expect(approvalDeadline(created, start).getTime()).toBe(
        at("2026-06-10T16:00:00Z"),
      );
    });

    it("caps at created + 24h for far-future reservations", () => {
      const start = new Date("2026-06-20T18:00:00Z").toISOString();
      expect(approvalDeadline(created, start).getTime()).toBe(
        at("2026-06-11T12:00:00Z"),
      );
    });

    it("floors at created + 30m for imminent reservations", () => {
      const start = new Date("2026-06-10T13:00:00Z").toISOString();
      expect(approvalDeadline(created, start).getTime()).toBe(
        at("2026-06-10T12:30:00Z"),
      );
    });
  });

  it("surfaces showing X of N when the server total exceeds loaded rows", () => {
    const onLoadMore = jest.fn();
    render(
      <ReservationApprovalQueue
        pending={[pending(1, 3 * 3_600_000)]}
        onApprove={jest.fn()}
        onDecline={jest.fn()}
        busyId={null}
        total={150}
        onLoadMore={onLoadMore}
      />,
    );
    // Mock getTranslation returns the key path; replace still injects numbers.
    expect(screen.getByTestId("approval-queue-truncated")).toHaveTextContent("150");
    expect(screen.getByTestId("approval-queue-truncated")).toHaveTextContent("1");
    fireEvent.click(screen.getByTestId("approval-queue-load-more"));
    expect(onLoadMore).toHaveBeenCalled();
  });

});
