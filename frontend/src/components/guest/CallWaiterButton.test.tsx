/** @jest-environment jsdom */
import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";

import CallWaiterButton from "@/components/guest/CallWaiterButton";
import {
  createServiceCall,
  getServiceCallStatus,
  ServiceCallCooldownError,
} from "@/api/bills";

jest.mock("@/api/bills", () => {
  class MockServiceCallCooldownError extends Error {
    retryAfterSeconds: number;
    constructor(retryAfterSeconds: number) {
      super("Service call is cooling down");
      this.name = "ServiceCallCooldownError";
      this.retryAfterSeconds = retryAfterSeconds;
    }
  }
  return {
    createServiceCall: jest.fn(),
    getServiceCallStatus: jest.fn(),
    ServiceCallCooldownError: MockServiceCallCooldownError,
  };
});

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string, params?: { reason?: string }) => {
      const table: Record<string, string> = {
        "serviceCall.button": "Call waiter",
        "serviceCall.reasonWater": "Water",
        "serviceCall.reasonOrder": "Ready to order",
        "serviceCall.reasonCheck": "Check, please",
        "serviceCall.waiting": "We've let the team know",
        "serviceCall.onTheWay": "Someone's on the way",
        "serviceCall.resolved": "All taken care of",
        "serviceCall.cooldown": "Just a moment — the team was just here",
        "serviceCall.dismiss": "Dismiss",
        "serviceCall.askedFor": "You asked for {reason}",
        "serviceCall.closedHint":
          "Staff may still help with your bill or questions.",
      };
      const value = table[key] ?? key;
      return params?.reason
        ? value.replace("{reason}", String(params.reason))
        : value;
    },
  }),
}));

const mockCreate = createServiceCall as jest.Mock;
const mockGetStatus = getServiceCallStatus as jest.Mock;

describe("CallWaiterButton", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGetStatus.mockResolvedValue({ status: "none", reason: null });
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  test("shows closed-hours hint only when businessClosed", () => {
    const { rerender } = render(<CallWaiterButton tableCode="T42" />);
    expect(screen.queryByTestId("call-waiter-closed-hint")).toBeNull();

    rerender(<CallWaiterButton tableCode="T42" businessClosed />);
    expect(screen.getByTestId("call-waiter-closed-hint")).toHaveTextContent(
      /bill or questions/i,
    );
  });

  test("tapping call-waiter with a reason posts and shows waiting state", async () => {
    mockCreate.mockResolvedValue("open");

    render(<CallWaiterButton tableCode="T42" />);

    fireEvent.click(screen.getByRole("button", { name: /call waiter/i }));

    fireEvent.click(await screen.findByRole("button", { name: "Water" }));

    await waitFor(() =>
      expect(mockCreate).toHaveBeenCalledWith("T42", "water"),
    );
    expect(
      await screen.findByText(/We've let the team know/),
    ).toBeInTheDocument();
    expect(screen.getByTestId("call-waiter-reason-echo")).toHaveTextContent(
      "You asked for Water",
    );
    expect(screen.getByRole("button", { name: "Water" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Ready to order" }),
    ).toBeInTheDocument();
  });

  test("acknowledged status flips to on-the-way copy", async () => {
    jest.useFakeTimers();
    mockCreate.mockResolvedValue("open");
    mockGetStatus.mockResolvedValue({ status: "none", reason: null });

    render(<CallWaiterButton tableCode="T42" />);

    fireEvent.click(screen.getByRole("button", { name: /call waiter/i }));
    fireEvent.click(
      await screen.findByRole("button", { name: "Check, please" }),
    );
    expect(
      await screen.findByText(/We've let the team know/),
    ).toBeInTheDocument();
    expect(screen.getByTestId("call-waiter-reason-echo")).toHaveTextContent(
      "You asked for Check, please",
    );
    expect(
      screen.getByRole("button", { name: "Ready to order" }),
    ).toBeInTheDocument();

    mockGetStatus.mockResolvedValue({
      status: "acknowledged",
      reason: "check",
    });

    // Advance past one 10s poll tick.
    await act(async () => {
      jest.advanceTimersByTime(10_000);
    });

    expect(await screen.findByText(/Someone's on the way/)).toBeInTheDocument();
    expect(screen.getByTestId("call-waiter-reason-echo")).toHaveTextContent(
      "You asked for Check, please",
    );
    expect(screen.getByRole("button", { name: "Water" })).toBeInTheDocument();
  });

  test("keeps chips open so a second reason can be sent after ack", async () => {
    mockCreate.mockResolvedValue("open");

    render(<CallWaiterButton tableCode="T42" />);

    fireEvent.click(screen.getByRole("button", { name: /call waiter/i }));
    fireEvent.click(await screen.findByRole("button", { name: "Water" }));

    expect(
      await screen.findByText(/We've let the team know/),
    ).toBeInTheDocument();
    expect(screen.getByTestId("call-waiter-reason-echo")).toHaveTextContent(
      "You asked for Water",
    );

    fireEvent.click(screen.getByRole("button", { name: "Ready to order" }));

    await waitFor(() =>
      expect(mockCreate).toHaveBeenLastCalledWith("T42", "order"),
    );
    expect(mockCreate).toHaveBeenCalledTimes(2);
    expect(screen.getByTestId("call-waiter-reason-echo")).toHaveTextContent(
      "You asked for Ready to order",
    );
    expect(screen.getByRole("button", { name: "Check, please" })).toBeEnabled();
  });

  test("resumes an acknowledged call with the echoed reason and chips", async () => {
    mockGetStatus.mockResolvedValue({
      status: "acknowledged",
      reason: "order",
    });

    render(<CallWaiterButton tableCode="T42" />);

    expect(
      await screen.findByText(/Someone's on the way/),
    ).toBeInTheDocument();
    expect(screen.getByTestId("call-waiter-reason-echo")).toHaveTextContent(
      "You asked for Ready to order",
    );
    expect(screen.getByRole("button", { name: "Water" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Check, please" }),
    ).toBeInTheDocument();
  });

  test("cooldown (429) shows the cooldown copy", async () => {
    mockCreate.mockRejectedValue(new ServiceCallCooldownError(30));

    render(<CallWaiterButton tableCode="T42" />);

    fireEvent.click(screen.getByRole("button", { name: /call waiter/i }));
    fireEvent.click(
      await screen.findByRole("button", { name: "Ready to order" }),
    );

    expect(
      await screen.findByText("Just a moment — the team was just here"),
    ).toBeInTheDocument();
  });

  test("dismiss control returns the chooser to idle", async () => {
    render(<CallWaiterButton tableCode="T42" />);

    fireEvent.click(screen.getByRole("button", { name: /call waiter/i }));
    await screen.findByRole("button", { name: "Water" });

    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));

    expect(
      screen.getByRole("button", { name: /call waiter/i }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Water" }),
    ).not.toBeInTheDocument();
    expect(mockCreate).not.toHaveBeenCalled();
  });

  test("keeps the chooser open when the surrounding menu rerenders", async () => {
    const { rerender } = render(<CallWaiterButton tableCode="T42" />);
    fireEvent.click(screen.getByRole("button", { name: /call waiter/i }));
    await screen.findByRole("button", { name: "Water" });

    rerender(<CallWaiterButton tableCode="T42" />);

    expect(screen.getByRole("button", { name: "Water" })).toBeInTheDocument();
    expect(mockCreate).not.toHaveBeenCalled();
  });

  test("Escape on the chooser returns to idle", async () => {
    render(<CallWaiterButton tableCode="T42" />);

    fireEvent.click(screen.getByRole("button", { name: /call waiter/i }));
    const chip = await screen.findByRole("button", { name: "Water" });

    fireEvent.keyDown(chip, { key: "Escape" });

    expect(
      screen.getByRole("button", { name: /call waiter/i }),
    ).toBeInTheDocument();
    expect(mockCreate).not.toHaveBeenCalled();
  });

  test("focus moves to the first reason chip when entering choosing", async () => {
    render(<CallWaiterButton tableCode="T42" />);

    fireEvent.click(screen.getByRole("button", { name: /call waiter/i }));
    const chip = await screen.findByRole("button", { name: "Water" });

    await waitFor(() => expect(chip).toHaveFocus());
  });

  test("cooldown panel is a live status region", async () => {
    mockCreate.mockRejectedValue(new ServiceCallCooldownError(30));

    render(<CallWaiterButton tableCode="T42" />);

    fireEvent.click(screen.getByRole("button", { name: /call waiter/i }));
    fireEvent.click(
      await screen.findByRole("button", { name: "Ready to order" }),
    );

    const panel = await screen.findByRole("status");
    expect(panel).toHaveTextContent("Just a moment — the team was just here");
  });

  test("resolved status returns to idle after a brief resolved beat", async () => {
    jest.useFakeTimers();
    mockCreate.mockResolvedValue("open");
    mockGetStatus.mockResolvedValue({ status: "none", reason: null });

    render(<CallWaiterButton tableCode="T42" />);

    fireEvent.click(screen.getByRole("button", { name: /call waiter/i }));
    fireEvent.click(await screen.findByRole("button", { name: "Water" }));
    expect(
      await screen.findByText(/We've let the team know/),
    ).toBeInTheDocument();

    mockGetStatus.mockResolvedValue({ status: "resolved", reason: null });

    await act(async () => {
      jest.advanceTimersByTime(10_000);
    });

    // During the resolved beat the guest sees DISTINCT resolved copy — not
    // the stale "on the way" line (the call is already handled).
    expect(screen.getByText("All taken care of")).toBeInTheDocument();
    expect(
      screen.queryByText(/Someone's on the way/),
    ).not.toBeInTheDocument();

    // Brief resolved beat, then back to the idle button.
    await act(async () => {
      jest.advanceTimersByTime(4_000);
    });

    expect(
      screen.getByRole("button", { name: /call waiter/i }),
    ).toBeInTheDocument();
  });
});
