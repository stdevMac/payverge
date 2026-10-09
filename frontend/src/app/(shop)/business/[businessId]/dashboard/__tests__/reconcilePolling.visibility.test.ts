/** @jest-environment jsdom */
import { renderHook, act } from "@testing-library/react";
import { usePolling } from "@/hooks/usePolling";
import { shouldScheduleGlobalBillPoll } from "../pollPolicy";

function setDocumentHidden(hidden: boolean) {
  Object.defineProperty(document, "hidden", {
    configurable: true,
    get: () => hidden,
  });
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    get: () => (hidden ? "hidden" : "visible"),
  });
  act(() => {
    document.dispatchEvent(new Event("visibilitychange"));
  });
}

describe("dashboard reconcile poller visibility", () => {
  beforeEach(() => {
    jest.useFakeTimers({ advanceTimers: true });
    setDocumentHidden(false);
  });

  afterEach(() => {
    setDocumentHidden(false);
    jest.useRealTimers();
  });

  it("does not tick loadGlobalBills while the document is hidden", async () => {
    const loadGlobalBills = jest.fn();
    renderHook(() =>
      usePolling({
        callback: loadGlobalBills,
        interval: 60_000,
        enabled: shouldScheduleGlobalBillPoll("bills"),
        immediate: false,
        pauseWhenHidden: true,
      }),
    );

    setDocumentHidden(true);

    await act(async () => {
      jest.advanceTimersByTime(180_000);
      await Promise.resolve();
    });

    expect(loadGlobalBills).not.toHaveBeenCalled();
  });
});
