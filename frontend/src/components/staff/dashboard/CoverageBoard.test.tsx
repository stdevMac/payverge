/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import CoverageBoard, { type CoverageBoardLabels } from "./CoverageBoard";

const labels: CoverageBoardLabels = {
  title: "Coverage",
  openShifts: "Open shifts",
  swapInbox: "Swap inbox",
  myRequests: "My requests",
  claim: "Claim",
  accept: "Accept",
  cancel: "Cancel",
  rowSwap: "Swap request",
  rowGiveup: "Give-up request",
  rowClaim: "Open-shift claim",
  statusPending: "Pending",
  statusAccepted: "Accepted",
  statusPendingApproval: "Awaiting approval",
  statusApproved: "Approved",
  statusDenied: "Denied",
  statusCancelled: "Cancelled",
  statusWithdrawn: "Withdrawn",
  emptyOpenTitle: "No open shifts",
  emptyOpenSubtitle: "Open shifts show up here.",
  emptyInboxTitle: "No swaps to cover",
  emptyInboxSubtitle: "Swaps to cover show up here.",
  emptyMineTitle: "No requests yet",
  emptyMineSubtitle: "Your requests show up here.",
  loading: "Loading…",
};

test("claims an open shift and shows no dollars", () => {
  const onClaim = jest.fn();
  render(
    <CoverageBoard
      open={{
        open_shifts: [
          {
            shift_id: 10,
            position_id: 9,
            starts_at: "2026-07-01T16:00:00Z",
            ends_at: "2026-07-01T22:00:00Z",
            break_minutes: 30,
          },
        ],
        swap_inbox: [],
        pending_approvals: { swaps: [], claims: [] },
      }}
      mine={{ claims: [], swaps: [] }}
      loading={false}
      busyId={null}
      cancelBusyKey={null}
      onClaim={onClaim}
      onAccept={jest.fn()}
      onCancel={jest.fn()}
      positionName={() => "Server"}
      formatRange={() => "4:00 PM – 10:00 PM"}
      labels={labels}
    />,
  );
  expect(screen.getByText("Server")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Claim" }));
  expect(onClaim).toHaveBeenCalledWith(10);
  expect(document.body.textContent).not.toMatch(/\$\d/);
});

test("accepts a swap from the inbox", () => {
  const onAccept = jest.fn();
  render(
    <CoverageBoard
      open={{
        open_shifts: [],
        swap_inbox: [
          {
            swap_id: 5,
            shift_id: 12,
            requesting_staff_id: 3,
            position_id: 9,
            starts_at: "2026-07-02T16:00:00Z",
            ends_at: "2026-07-02T22:00:00Z",
          },
        ],
        pending_approvals: { swaps: [], claims: [] },
      }}
      mine={{ claims: [], swaps: [] }}
      loading={false}
      busyId={null}
      cancelBusyKey={null}
      onClaim={jest.fn()}
      onAccept={onAccept}
      onCancel={jest.fn()}
      positionName={() => "Server"}
      formatRange={() => "4:00 PM – 10:00 PM"}
      labels={labels}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "Accept" }));
  expect(onAccept).toHaveBeenCalledWith(5);
  expect(document.body.textContent).not.toMatch(/\$\d/);
});

test("renders my requests with status chips and no dollars", () => {
  render(
    <CoverageBoard
      open={{ open_shifts: [], swap_inbox: [], pending_approvals: { swaps: [], claims: [] } }}
      mine={{
        claims: [{ id: 1, shift_id: 10, status: "pending", created_at: "" }],
        swaps: [
          {
            id: 2,
            shift_id: 11,
            kind: "giveup",
            status: "pending_approval",
            requesting_staff_id: 7,
            accepting_staff_id: null,
            created_at: "",
          },
        ],
      }}
      loading={false}
      busyId={null}
      cancelBusyKey={null}
      onClaim={jest.fn()}
      onAccept={jest.fn()}
      onCancel={jest.fn()}
      positionName={() => "Server"}
      formatRange={() => "4:00 PM – 10:00 PM"}
      labels={labels}
    />,
  );
  expect(screen.getByText("Give-up request")).toBeInTheDocument();
  expect(screen.getByText("Open-shift claim")).toBeInTheDocument();
  expect(screen.getByText("Awaiting approval")).toBeInTheDocument();
  expect(screen.getByText("Pending")).toBeInTheDocument();
  expect(document.body.textContent).not.toMatch(/\$\d/);
});

test("cancels a live request; a terminal request shows no cancel button", () => {
  const onCancel = jest.fn();
  render(
    <CoverageBoard
      open={{ open_shifts: [], swap_inbox: [], pending_approvals: { swaps: [], claims: [] } }}
      mine={{
        claims: [{ id: 1, shift_id: 10, status: "pending", created_at: "" }],
        swaps: [
          {
            id: 2,
            shift_id: 11,
            kind: "giveup",
            status: "pending_approval",
            requesting_staff_id: 7,
            accepting_staff_id: null,
            created_at: "",
          },
          // Terminal — no cancel affordance.
          {
            id: 3,
            shift_id: 12,
            kind: "swap",
            status: "approved",
            requesting_staff_id: 7,
            accepting_staff_id: 8,
            created_at: "",
          },
        ],
      }}
      loading={false}
      busyId={null}
      cancelBusyKey={null}
      onClaim={jest.fn()}
      onAccept={jest.fn()}
      onCancel={onCancel}
      positionName={() => "Server"}
      formatRange={() => "4:00 PM – 10:00 PM"}
      labels={labels}
    />,
  );
  // Two cancellable rows (the pending_approval giveup + the pending claim); the
  // approved swap is terminal, so only two Cancel buttons render.
  const cancels = screen.getAllByRole("button", { name: "Cancel" });
  expect(cancels).toHaveLength(2);

  // Cancelling the giveup passes the swap id + its kind.
  fireEvent.click(cancels[0]);
  expect(onCancel).toHaveBeenCalledWith(2, "giveup");
});

test("renders skeleton while loading", () => {
  render(
    <CoverageBoard
      open={null}
      mine={null}
      loading
      busyId={null}
      cancelBusyKey={null}
      onClaim={jest.fn()}
      onAccept={jest.fn()}
      onCancel={jest.fn()}
      positionName={() => ""}
      formatRange={() => ""}
      labels={labels}
    />,
  );
  expect(screen.getByRole("status")).toHaveAccessibleName("Loading…");
});
