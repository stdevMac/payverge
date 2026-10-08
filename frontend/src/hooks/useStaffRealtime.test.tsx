/** @jest-environment jsdom */
import { renderHook, act } from "@testing-library/react";
import { useStaffRealtime } from "@/hooks/useStaffRealtime";
import { useSSEEvents } from "@/hooks/useSSEEvents";

jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: jest.fn(),
}));

const mockUseSSEEvents = useSSEEvents as jest.Mock;

function lastOptions() {
  return mockUseSSEEvents.mock.calls[mockUseSSEEvents.mock.calls.length - 1][0];
}

describe("useStaffRealtime", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockUseSSEEvents.mockReturnValue({
      degraded: false,
      blocked: false,
      reconnect: jest.fn(),
    });
  });

  it("fans named events out to the matching handler", () => {
    const onSchedulePublished = jest.fn();
    renderHook(() =>
      useStaffRealtime({ businessId: 7, onSchedulePublished }),
    );

    const { onEvent } = lastOptions();
    act(() => {
      onEvent({ type: "schedule.published", data: { schedule_id: 3 } });
    });

    expect(onSchedulePublished).toHaveBeenCalledWith({ schedule_id: 3 });
  });

  it("fans the timeclock.entry event to onTimeclockEntry", () => {
    const onTimeclockEntry = jest.fn();
    renderHook(() => useStaffRealtime({ businessId: 7, onTimeclockEntry }));

    const { onEvent } = lastOptions();
    act(() => {
      onEvent({ type: "timeclock.entry", data: { entry_id: 5, status: "pending_review" } });
    });

    expect(onTimeclockEntry).toHaveBeenCalledWith({ entry_id: 5, status: "pending_review" });
  });

  describe("SSE reconnect healing", () => {
    it("registers an onReconnect callback with the shared SSE hook", () => {
      renderHook(() => useStaffRealtime({ businessId: 7 }));
      expect(typeof lastOptions().onReconnect).toBe("function");
    });

    it("invokes the consumer's onReconnect handler after a recovered drop", () => {
      const onReconnect = jest.fn();
      renderHook(() => useStaffRealtime({ businessId: 7, onReconnect }));

      act(() => {
        lastOptions().onReconnect();
      });

      expect(onReconnect).toHaveBeenCalledTimes(1);
    });

    it("is a safe no-op when the consumer registers no onReconnect", () => {
      renderHook(() => useStaffRealtime({ businessId: 7 }));
      expect(() => lastOptions().onReconnect()).not.toThrow();
    });

    it("reads the LATEST handler through the ref (stable subscription, fresh closure)", () => {
      const first = jest.fn();
      const second = jest.fn();
      const { rerender } = renderHook(
        ({ handler }: { handler: () => void }) =>
          useStaffRealtime({ businessId: 7, onReconnect: handler }),
        { initialProps: { handler: first } },
      );

      const captured = lastOptions().onReconnect;
      rerender({ handler: second });

      act(() => {
        captured();
      });

      expect(first).not.toHaveBeenCalled();
      expect(second).toHaveBeenCalledTimes(1);
    });
  });
});
