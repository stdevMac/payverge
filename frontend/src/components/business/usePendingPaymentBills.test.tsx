/** @jest-environment jsdom */
import { act, renderHook } from "@testing-library/react";

import { usePendingPaymentBills } from "./usePendingPaymentBills";
import type { SSEEvent } from "@/hooks/useSSEEvents";

let capturedOnEvent: ((event: SSEEvent) => void) | undefined;
jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: jest.fn((options: { onEvent: (e: SSEEvent) => void }) => {
    capturedOnEvent = options.onEvent;
    return { degraded: false, blocked: false, reconnect: jest.fn() };
  }),
}));

function emit(event: SSEEvent) {
  act(() => {
    capturedOnEvent?.(event);
  });
}

describe("usePendingPaymentBills", () => {
  beforeEach(() => {
    capturedOnEvent = undefined;
    jest.useFakeTimers();
  });
  afterEach(() => {
    act(() => {
      jest.runOnlyPendingTimers();
    });
    jest.useRealTimers();
  });

  it("adds a bill id on payment.pending", () => {
    const { result } = renderHook(() => usePendingPaymentBills(42, true));
    expect(result.current.has(7)).toBe(false);
    emit({
      type: "payment.pending",
      data: { bill_id: 7, amount: 5, method: "cash" },
    });
    expect(result.current.has(7)).toBe(true);
  });

  it("clears the badge on payment.received for the same bill", () => {
    const { result } = renderHook(() => usePendingPaymentBills(42, true));
    emit({ type: "payment.pending", data: { bill_id: 7 } });
    expect(result.current.has(7)).toBe(true);
    emit({ type: "payment.received", data: { bill_id: 7 } });
    expect(result.current.has(7)).toBe(false);
  });

  it("clears the badge when a pending request is cancelled or rejected", () => {
    const { result } = renderHook(() => usePendingPaymentBills(42, true));
    emit({ type: "payment.pending", data: { bill_id: 7 } });
    expect(result.current.has(7)).toBe(true);
    emit({
      type: "payment.request.resolved",
      data: { bill_id: 7, request_id: 17, status: "rejected" },
    });
    expect(result.current.has(7)).toBe(false);
  });

  it("clears the badge when the bill transitions to a paid status via bill.updated", () => {
    const { result } = renderHook(() => usePendingPaymentBills(42, true));
    emit({ type: "payment.pending", data: { bill_id: 7 } });
    expect(result.current.has(7)).toBe(true);
    emit({ type: "bill.updated", data: { id: 7, status: "paid" } });
    expect(result.current.has(7)).toBe(false);
  });

  it("keeps the badge on a non-terminal bill.updated (e.g. a menu edit)", () => {
    const { result } = renderHook(() => usePendingPaymentBills(42, true));
    emit({ type: "payment.pending", data: { bill_id: 7 } });
    emit({ type: "bill.updated", data: { id: 7, status: "partial" } });
    expect(result.current.has(7)).toBe(true);
  });

  it("self-expires the badge after the TTL", () => {
    const { result } = renderHook(() => usePendingPaymentBills(42, true));
    emit({ type: "payment.pending", data: { bill_id: 7 } });
    expect(result.current.has(7)).toBe(true);
    act(() => {
      jest.advanceTimersByTime(10 * 60 * 1000 + 1);
    });
    expect(result.current.has(7)).toBe(false);
  });
});
