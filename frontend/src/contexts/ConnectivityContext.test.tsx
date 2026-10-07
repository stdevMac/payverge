/** @jest-environment jsdom */
import { render, act, screen, waitFor } from "@testing-library/react";
import React from "react";

import { ConnectivityProvider, useConnectivity } from "./ConnectivityContext";

function Probe() {
  const { isOnline } = useConnectivity();
  return <span data-testid="status">{isOnline ? "online" : "offline"}</span>;
}

// Regression: confirmOnline must probe the same-origin "/api/health" route (which
// exists and returns 200), NOT the backend path "/api/v1/health/live" — that path
// does not exist on the frontend origin (no /api/v1 rewrite), so it 404'd and the
// offline banner never cleared after the browser reconnected.
describe("ConnectivityProvider connectivity probe", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    jest.useRealTimers();
    global.fetch = originalFetch;
  });

  it("probes the same-origin /api/health route (not the backend /api/v1 path)", async () => {
    jest.useFakeTimers();
    const fetchMock = jest.fn().mockResolvedValue({ ok: true } as Response);
    global.fetch = fetchMock as unknown as typeof fetch;

    render(
      <ConnectivityProvider>
        <div />
      </ConnectivityProvider>,
    );

    // Simulate the browser firing the "online" event, then pass the 500ms debounce.
    act(() => {
      window.dispatchEvent(new Event("online"));
    });
    await act(async () => {
      jest.advanceTimersByTime(600);
      await Promise.resolve();
    });

    expect(fetchMock).toHaveBeenCalled();
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/health");
    expect(opts).toEqual(expect.objectContaining({ method: "HEAD" }));
  });
});

describe("ConnectivityProvider reconnect retry", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    jest.runOnlyPendingTimers();
    jest.useRealTimers();
    global.fetch = originalFetch;
  });

  async function flush() {
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
  }

  it("retries a failed confirmation probe instead of latching offline forever", async () => {
    jest.useFakeTimers();
    const fetchMock = jest
      .fn()
      .mockRejectedValueOnce(new Error("network")) // first probe fails
      .mockResolvedValueOnce({ ok: true } as Response); // retry succeeds
    global.fetch = fetchMock as unknown as typeof fetch;

    render(
      <ConnectivityProvider>
        <Probe />
      </ConnectivityProvider>,
    );

    // Drop offline so the banner is showing.
    act(() => {
      window.dispatchEvent(new Event("offline"));
      jest.advanceTimersByTime(500);
    });
    expect(screen.getByTestId("status").textContent).toBe("offline");

    // Reconnect: first probe (after the 500ms debounce) fails.
    act(() => {
      window.dispatchEvent(new Event("online"));
      jest.advanceTimersByTime(500);
    });
    await flush();
    expect(screen.getByTestId("status").textContent).toBe("offline");

    // A bounded backoff retry must be scheduled; advance past the first delay.
    await act(async () => {
      jest.advanceTimersByTime(1000);
      await Promise.resolve();
    });
    await flush();

    await waitFor(() => {
      expect(screen.getByTestId("status").textContent).toBe("online");
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
