/**
 * @jest-environment jsdom
 */

import { render, act, cleanup } from "@testing-library/react";
import { useSSEEvents, type SSEEvent } from "@/hooks/useSSEEvents";

// The hook reuses the app's shared refresh singleton so concurrent callers
// dedupe a single /auth/refresh. Mock it so the heavy axios module (toast,
// cache, mutation queue) never loads under jsdom and we can drive outcomes.
jest.mock("@/api/tools/instance", () => ({
  refreshAuthSession: jest.fn().mockResolvedValue(false),
}));

const mockRefreshAuthSession = (
  jest.requireMock("@/api/tools/instance") as {
    refreshAuthSession: jest.Mock;
  }
).refreshAuthSession;

type EventHandler = (event: MessageEvent) => void;

class MockEventSource {
  static instances: MockEventSource[] = [];

  listeners = new Map<string, EventHandler[]>();
  onerror: EventHandler | null = null;
  onmessage: EventHandler | null = null;
  url: string;
  withCredentials?: boolean;
  closed = false;

  constructor(url: string, init?: EventSourceInit) {
    this.url = url;
    this.withCredentials = init?.withCredentials;
    MockEventSource.instances.push(this);
  }

  addEventListener(type: string, handler: EventHandler) {
    const handlers = this.listeners.get(type) ?? [];
    handlers.push(handler);
    this.listeners.set(type, handlers);
  }

  close() {
    this.closed = true;
  }

  emit(type: string, data: unknown, lastEventId = "") {
    const event = { data: JSON.stringify(data), lastEventId } as MessageEvent;
    for (const handler of this.listeners.get(type) ?? []) {
      handler(event);
    }
  }

  emitMessage(data: unknown) {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent);
  }
}

const latest = () =>
  MockEventSource.instances[MockEventSource.instances.length - 1];

function Harness({
  onEvent,
  onReconnect,
  businessId = 42,
  enabled = true,
}: {
  onEvent: (event: SSEEvent) => void;
  onReconnect?: () => void;
  businessId?: number;
  enabled?: boolean;
}) {
  useSSEEvents({ businessId, enabled, onEvent, onReconnect });
  return null;
}

// Captures the hook's `degraded`/`blocked` returns on every render.
let capturedDegraded: boolean | undefined;
let capturedBlocked: boolean | undefined;
let capturedBlockedReason: string | null | undefined;
function StateHarness({ onEvent }: { onEvent: (event: SSEEvent) => void }) {
  const { degraded, blocked, blockedReason } = useSSEEvents({
    businessId: 42,
    enabled: true,
    onEvent,
  });
  capturedDegraded = degraded;
  capturedBlocked = blocked;
  capturedBlockedReason = blockedReason;
  return null;
}

describe("useSSEEvents", () => {
  beforeEach(() => {
    MockEventSource.instances = [];
    capturedDegraded = undefined;
    capturedBlocked = undefined;
    capturedBlockedReason = undefined;
    global.EventSource = MockEventSource as never;
    process.env.API_URL = "/api/v1";
    mockRefreshAuthSession.mockReset().mockResolvedValue(false);
  });

  afterEach(() => {
    cleanup();
  });

  it("dispatches generic fallback messages for unknown backend event types", () => {
    const events: SSEEvent[] = [];

    render(<Harness onEvent={(event) => events.push(event)} />);

    act(() => {
      MockEventSource.instances[0].emitMessage({
        type: "inventory.low",
        data: { item_id: 12 },
      });
    });

    expect(events).toEqual([{ type: "inventory.low", data: { item_id: 12 } }]);
  });

  it("dispatches all permission-scoped print wake events", () => {
    const events: SSEEvent[] = [];
    render(<Harness onEvent={(event) => events.push(event)} />);
    for (const type of [
      "print.bill_available",
      "print.receipt_available",
      "print.kitchen_available",
    ]) {
      act(() => latest().emit(type, { printer_id: 7 }));
    }
    expect(events.map((event) => event.type)).toEqual([
      "print.bill_available",
      "print.receipt_available",
      "print.kitchen_available",
    ]);
  });

  it("fires reconnect resync only after a dropped connection recovers", () => {
    jest.useFakeTimers();
    const onReconnect = jest.fn();

    render(<Harness onEvent={jest.fn()} onReconnect={onReconnect} />);

    act(() => {
      MockEventSource.instances[0].emit("connected", { business_id: 42 });
    });
    expect(onReconnect).not.toHaveBeenCalled();

    act(() => {
      MockEventSource.instances[0].onerror?.({ data: "" } as MessageEvent);
      jest.runOnlyPendingTimers();
    });

    act(() => {
      MockEventSource.instances[1].emit("connected", { business_id: 42 });
    });

    expect(onReconnect).toHaveBeenCalledTimes(1);
    jest.useRealTimers();
  });

  it("opens a single shared EventSource for multiple consumers of the same business", () => {
    const a = jest.fn();
    const b = jest.fn();

    render(
      <>
        <Harness onEvent={a} />
        <Harness onEvent={b} />
      </>,
    );

    expect(MockEventSource.instances).toHaveLength(1);

    act(() => {
      MockEventSource.instances[0].emitMessage({
        type: "inventory.low",
        data: { item_id: 7 },
      });
    });

    // The single stream fans out to every subscriber.
    expect(a).toHaveBeenCalledWith({
      type: "inventory.low",
      data: { item_id: 7 },
    });
    expect(b).toHaveBeenCalledWith({
      type: "inventory.low",
      data: { item_id: 7 },
    });
  });

  it("keeps the shared stream open until the last consumer unmounts", () => {
    const { rerender } = render(
      <>
        <Harness onEvent={jest.fn()} />
        <Harness onEvent={jest.fn()} />
      </>,
    );
    const stream = MockEventSource.instances[0];
    expect(stream.closed).toBe(false);

    // Drop one consumer — the stream must stay open for the other.
    rerender(
      <>
        <Harness onEvent={jest.fn()} />
      </>,
    );
    expect(stream.closed).toBe(false);
    expect(MockEventSource.instances).toHaveLength(1);

    // Drop the last consumer — now the stream closes.
    rerender(<></>);
    expect(stream.closed).toBe(true);
  });

  it("retries forever with degraded state instead of permanently giving up", () => {
    jest.useFakeTimers();

    render(<StateHarness onEvent={jest.fn()} />);
    expect(capturedDegraded).toBe(false);

    const failOnce = () => {
      act(() => {
        latest().onerror?.({ data: "" } as MessageEvent);
        // Advance past the maximum capped backoff to fire the scheduled retry.
        jest.advanceTimersByTime(30_000);
      });
    };

    // Far beyond the old 5-retry cap — the hook must keep opening new streams.
    for (let i = 0; i < 10; i++) failOnce();

    // 1 initial + 10 reconnects = 11 streams. The old implementation stopped at 6.
    expect(MockEventSource.instances.length).toBeGreaterThanOrEqual(11);
    // Sustained failure surfaces the degraded banner...
    expect(capturedDegraded).toBe(true);

    // ...and a successful (re)connect clears it automatically.
    act(() => {
      latest().emit("connected", { business_id: 42 });
    });
    expect(capturedDegraded).toBe(false);

    jest.useRealTimers();
  });

  it("stops reconnecting and reports blocked when the server sends a terminal error frame", () => {
    jest.useFakeTimers();

    render(<StateHarness onEvent={jest.fn()} />);
    expect(MockEventSource.instances).toHaveLength(1);
    expect(capturedBlocked).toBe(false);

    // Server gate denial: a terminal `error` event on the 200 stream.
    act(() => {
      latest().emit("error", { code: "business_suspended", message: "x" });
    });

    expect(capturedBlocked).toBe(true);
    expect(capturedBlockedReason).toBe("business_suspended");

    // Even after a transport drop + backoff window, NO new stream opens — the
    // denial is permanent until the consumer re-mounts (e.g. after upgrading).
    act(() => {
      latest().onerror?.({ data: "" } as MessageEvent);
      jest.advanceTimersByTime(60_000);
    });
    expect(MockEventSource.instances).toHaveLength(1);

    jest.useRealTimers();
  });

  it("detaches wake listeners and removes the connection from the map on terminal block", () => {
    // Regression for SSE-LEAK-2: a terminal `error` frame left window/document
    // event listeners live and the connection in the module map, so
    // online/focus/visibilitychange events would still fire against a
    // permanently-stopped connection. The wake listeners must be removed and
    // the shared connection map entry must be deleted when blocked.
    jest.useFakeTimers();

    const addSpy = jest.spyOn(window, "addEventListener");
    const removeSpy = jest.spyOn(window, "removeEventListener");

    render(<StateHarness onEvent={jest.fn()} />);
    // Baseline: constructor registered the wake listeners.
    expect(addSpy).toHaveBeenCalledWith("online", expect.any(Function));
    expect(addSpy).toHaveBeenCalledWith("focus", expect.any(Function));

    // Terminal server gate-denial.
    act(() => {
      latest().emit("error", { code: "business_suspended", message: "x" });
    });

    expect(capturedBlocked).toBe(true);
    // The wake listeners must have been detached.
    expect(removeSpy).toHaveBeenCalledWith("online", expect.any(Function));
    expect(removeSpy).toHaveBeenCalledWith("focus", expect.any(Function));

    // After teardown, a subsequent online event must NOT trigger a new stream.
    const countBefore = MockEventSource.instances.length;
    act(() => {
      window.dispatchEvent(new Event("online"));
    });
    expect(MockEventSource.instances.length).toBe(countBefore);

    addSpy.mockRestore();
    removeSpy.mockRestore();
    jest.useRealTimers();
  });

  it("keeps retrying (not blocked) on a transient transport error", () => {
    jest.useFakeTimers();

    render(<StateHarness onEvent={jest.fn()} />);
    act(() => {
      latest().onerror?.({ data: "" } as MessageEvent);
      jest.advanceTimersByTime(30_000);
    });

    // A transport drop reconnects and never sets blocked.
    expect(MockEventSource.instances.length).toBeGreaterThanOrEqual(2);
    expect(capturedBlocked).toBe(false);
    expect(capturedBlockedReason).toBeNull();

    jest.useRealTimers();
  });

  it("preserves access_denied as the blocked reason, not business_suspended", () => {
    render(<StateHarness onEvent={jest.fn()} />);
    act(() => {
      latest().emit("error", { code: "access_denied", message: "Access denied" });
    });
    expect(capturedBlocked).toBe(true);
    expect(capturedBlockedReason).toBe("access_denied");
  });

  it("resumes from the last seen event id on reconnect", () => {
    jest.useFakeTimers();

    render(<Harness onEvent={jest.fn()} />);

    // First connect carries no resume param (asserted fully in the next test).
    const firstUrl = MockEventSource.instances[0].url;

    // Establish the stream, then receive a real named event carrying id="7".
    act(() => {
      MockEventSource.instances[0].emit("connected", { business_id: 42 });
      MockEventSource.instances[0].emit("order.updated", { order_id: 1 }, "7");
    });

    // Drop the connection; the scheduled backoff reconnect opens a new stream.
    act(() => {
      MockEventSource.instances[0].onerror?.({ data: "" } as MessageEvent);
      jest.runOnlyPendingTimers();
    });

    // The new EventSource must resume from id 7 so the backend replays ONLY
    // missed events (recovering lost orders, avoiding a full-buffer storm).
    const resumeUrl = MockEventSource.instances[1].url;
    expect(resumeUrl).toContain("last_event_id=7");
    expect(resumeUrl).toBe(`${firstUrl}?last_event_id=7`);

    jest.useRealTimers();
  });

  it("full-heals and discards the stale id on a sync.reset control frame", () => {
    jest.useFakeTimers();

    const onReconnect = jest.fn();
    render(<Harness onEvent={jest.fn()} onReconnect={onReconnect} />);

    const firstUrl = MockEventSource.instances[0].url;

    // Stream is live and has seen event id 7.
    act(() => {
      MockEventSource.instances[0].emit("connected", { business_id: 42 });
      MockEventSource.instances[0].emit(
        "order.updated",
        { order_id: 1 },
        "abc:7",
      );
    });

    // The backend signals a restart-epoch reset: our resume point is gone.
    act(() => {
      MockEventSource.instances[0].emit("sync.reset", {
        epoch: "def",
        reason: "resume_unavailable",
      });
    });

    // sync.reset must trigger the full-heal refetch path.
    expect(onReconnect).toHaveBeenCalled();

    // And the stale id must be dropped: a later transport drop reconnects WITHOUT
    // a last_event_id param.
    act(() => {
      MockEventSource.instances[0].onerror?.({ data: "" } as MessageEvent);
      jest.runOnlyPendingTimers();
    });
    const resumeUrl =
      MockEventSource.instances[MockEventSource.instances.length - 1].url;
    expect(resumeUrl).toBe(firstUrl);
    expect(resumeUrl).not.toContain("last_event_id");

    jest.useRealTimers();
  });

  it("omits last_event_id on the very first connect", () => {
    render(<Harness onEvent={jest.fn()} />);

    // No event has been seen yet, so the first EventSource URL must be the
    // bare stream URL — the initial hydrate (on-mount fetch) covers state,
    // and an empty resume param would needlessly ask the backend to replay.
    const firstUrl = MockEventSource.instances[0].url;
    expect(firstUrl).toBe("/api/v1/inside/businesses/42/events");
    expect(firstUrl).not.toContain("last_event_id");
  });

  // A stale 15-min session cookie 401s the reconnect, which is indistinguishable
  // from a network drop at the EventSource layer (both fire onerror with no
  // readable body). Without an auth-aware path the hook would retry the dead
  // cookie forever and latch the "disconnected" banner permanently. It must
  // instead refresh the session and reconnect with the fresh cookie.
  it("refreshes the session and reconnects immediately after sustained reconnect failures", async () => {
    jest.useFakeTimers();
    mockRefreshAuthSession.mockResolvedValue(true);

    render(<StateHarness onEvent={jest.fn()} />);
    expect(MockEventSource.instances).toHaveLength(1);

    // First failure is below the refresh threshold — backoff only, no refresh.
    await act(async () => {
      latest().onerror?.({ data: "" } as MessageEvent);
    });
    expect(mockRefreshAuthSession).not.toHaveBeenCalled();

    // Fire the scheduled backoff reconnect, then fail again. Two consecutive
    // failed reconnects look like a stale-cookie 401 → trigger a refresh.
    await act(async () => {
      jest.advanceTimersByTime(5_000);
    });
    const beforeRefresh = MockEventSource.instances.length;
    await act(async () => {
      latest().onerror?.({ data: "" } as MessageEvent);
    });
    expect(mockRefreshAuthSession).toHaveBeenCalled();

    // The refresh resolves true → the hook abandons the backoff wait and opens
    // a fresh stream right away (no timer advance), carrying the new cookie.
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(MockEventSource.instances.length).toBe(beforeRefresh + 1);

    // The fresh stream's connected frame clears any degraded state.
    await act(async () => {
      latest().emit("connected", { business_id: 42 });
    });
    expect(capturedDegraded).toBe(false);

    jest.useRealTimers();
  });

  it("does not refresh on a single transient blip that recovers on its own", async () => {
    jest.useFakeTimers();

    render(<StateHarness onEvent={jest.fn()} />);
    await act(async () => {
      latest().onerror?.({ data: "" } as MessageEvent);
      jest.advanceTimersByTime(5_000);
    });
    // One failure is below the threshold — no /auth/refresh hammering.
    expect(mockRefreshAuthSession).not.toHaveBeenCalled();

    await act(async () => {
      latest().emit("connected", { business_id: 42 });
    });
    expect(mockRefreshAuthSession).not.toHaveBeenCalled();

    jest.useRealTimers();
  });

  // Reconnect is otherwise driven only by a setTimeout backoff, which the
  // browser throttles/freezes in background tabs and during sleep. On wake the
  // operator would stare at a stale banner until the frozen timer eventually
  // fires. A network/visibility/focus wake must force an immediate reconnect.
  it("reconnects immediately when the browser comes back online after a drop", () => {
    jest.useFakeTimers();

    render(<StateHarness onEvent={jest.fn()} />);
    // Drop → the hook enters backoff (down) without opening a new stream yet.
    act(() => {
      latest().onerror?.({ data: "" } as MessageEvent);
    });
    expect(MockEventSource.instances).toHaveLength(1);

    // Network restored: reconnect now rather than waiting out the backoff.
    act(() => {
      window.dispatchEvent(new Event("online"));
    });
    expect(MockEventSource.instances).toHaveLength(2);

    jest.useRealTimers();
  });

  it("does not churn a healthy stream on wake events", () => {
    render(<StateHarness onEvent={jest.fn()} />);
    act(() => {
      latest().emit("connected", { business_id: 42 });
    });
    expect(MockEventSource.instances).toHaveLength(1);

    // A healthy, connected stream must be left untouched on wake.
    act(() => {
      window.dispatchEvent(new Event("online"));
      window.dispatchEvent(new Event("focus"));
    });
    expect(MockEventSource.instances).toHaveLength(1);
  });
});
