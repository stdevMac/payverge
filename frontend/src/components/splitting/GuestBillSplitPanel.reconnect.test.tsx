/** @jest-environment jsdom */
import React from "react";
import { act, render } from "@testing-library/react";
import { asDollars } from "@/types/money";
import GuestBillSplitPanel from "./GuestBillSplitPanel";

// Focused suite for the guest split stream's reconnect behavior: a fatal
// EventSource close must NOT freeze the guest's split view — the panel owns
// its retry loop (bounded exponential backoff) and refetches on recovery.

jest.mock("@/api/splitting", () => ({
  __esModule: true,
  ...(() => {
    const api = {
      createSplitHold: jest.fn(),
      executeHeldShare: jest.fn(),
      getSplitShareReceipt: jest.fn(),
      getMySplitShares: jest.fn().mockResolvedValue([]),
      releaseHeldShare: jest.fn(),
    };
    return {
      SplittingAPI: api,
      useSplittingAPI: () => api,
      getSplitEventsURL: (billToken: string) => `/split-events/${billToken}`,
    };
  })(),
}));

jest.mock("@/api/alternativePayments", () => ({
  requestAlternativePayment: jest.fn(),
}));

jest.mock("../guest/PaymentSection", () => function MockPaymentSection() {
  return <div data-testid="payment-section" />;
});

jest.mock("../../i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key,
    currentLanguage: undefined,
  }),
}));

interface EmittedSource {
  url: string;
  close: jest.Mock;
  onerror: ((event: Event) => void) | null;
  emit: (type: string, payload: unknown) => void;
}

const sources: MockEventSource[] = [];

class MockEventSource implements EmittedSource {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSED = 2;
  url: string;
  withCredentials = false;
  readyState = 1;
  onerror: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onopen: ((event: Event) => void) | null = null;
  close = jest.fn();
  private listeners = new Map<string, Array<(event: MessageEvent) => void>>();

  constructor(url: string) {
    this.url = url;
    sources.push(this);
  }

  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    const existing = this.listeners.get(type) ?? [];
    existing.push(listener);
    this.listeners.set(type, existing);
  }

  removeEventListener() {}

  dispatchEvent() {
    return true;
  }

  emit(type: string, payload: unknown) {
    const event = { data: JSON.stringify(payload) } as MessageEvent;
    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }
}

const originalEventSource = globalThis.EventSource;

function renderPanel(onBillRefresh: jest.Mock) {
  return render(
    <GuestBillSplitPanel
      billId={1}
      billToken="B42"
      businessId={2}
      businessName="Test Resto"
      businessAddress="0xsettlement"
      tipAddress="0xtip"
      tableCode="T1"
      remainingAmount={asDollars(50)}
      defaultCurrency="USD"
      displayCurrency="USD"
      items={[]}
      onPaymentComplete={jest.fn()}
      onBillRefresh={onBillRefresh}
    />,
  );
}

describe("GuestBillSplitPanel stream reconnect", () => {
  beforeEach(() => {
    sources.length = 0;
    jest.useFakeTimers();
    Object.defineProperty(globalThis, "EventSource", {
      configurable: true,
      value: MockEventSource,
    });
  });

  afterEach(() => {
    jest.useRealTimers();
    Object.defineProperty(globalThis, "EventSource", {
      configurable: true,
      value: originalEventSource,
    });
  });

  it("recreates the EventSource after a fatal close (bounded backoff)", () => {
    const onBillRefresh = jest.fn();
    renderPanel(onBillRefresh);

    expect(sources).toHaveLength(1);

    // Fatal close: the stream dies. Pre-fix this was permanent.
    act(() => {
      sources[0].onerror?.(new Event("error"));
    });
    expect(sources[0].close).toHaveBeenCalled();
    // The immediate error still triggers a bill refetch (existing behavior).
    expect(onBillRefresh).toHaveBeenCalledTimes(1);
    // Not recreated synchronously — backoff first.
    expect(sources).toHaveLength(1);

    // First retry lands within the 1s ceiling.
    act(() => {
      jest.advanceTimersByTime(1_000);
    });
    expect(sources).toHaveLength(2);
    expect(sources[1].url).toBe("/split-events/B42");
  });

  it("refetches split/bill state when the stream recovers after a drop", () => {
    const onBillRefresh = jest.fn();
    renderPanel(onBillRefresh);

    act(() => {
      sources[0].onerror?.(new Event("error"));
      jest.advanceTimersByTime(1_000);
    });
    expect(sources).toHaveLength(2);
    onBillRefresh.mockClear();

    // Recovery: `connected` on the NEW source → refetch missed state.
    act(() => {
      sources[1].emit("connected", { state: { bill_number: "B42", shares: [] } });
    });
    expect(onBillRefresh).toHaveBeenCalledTimes(1);
  });

  it("refetches the bill when the server sends sync.reset after a lost frame", () => {
    const onBillRefresh = jest.fn();
    renderPanel(onBillRefresh);
    act(() => {
      sources[0].emit("connected", { state: { bill_number: "B42", shares: [] } });
    });
    onBillRefresh.mockClear();

    act(() => {
      sources[0].emit("sync.reset", { bill_number: "B42", state: { bill_number: "B42", shares: [] } });
    });
    expect(onBillRefresh).toHaveBeenCalledTimes(1);
    expect(sources).toHaveLength(1);
    expect(sources[0].close).not.toHaveBeenCalled();
  });

  it("does not tear down EventSource when onBillRefresh identity changes", () => {
    const first = jest.fn();
    const { rerender } = renderPanel(first);
    expect(sources).toHaveLength(1);

    rerender(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(50)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
      />,
    );

    expect(sources).toHaveLength(1);
    expect(sources[0].close).not.toHaveBeenCalled();
  });

  it("does not refetch on the FIRST connect (no drop happened)", () => {
    const onBillRefresh = jest.fn();
    renderPanel(onBillRefresh);

    act(() => {
      sources[0].emit("connected", { state: { bill_number: "B42", shares: [] } });
    });
    expect(onBillRefresh).not.toHaveBeenCalled();
  });

  it("escalates the backoff across consecutive failures instead of hammering", () => {
    renderPanel(jest.fn());

    // Failure 1 → retry within 1s.
    act(() => {
      sources[0].onerror?.(new Event("error"));
    });
    act(() => {
      jest.advanceTimersByTime(1_000);
    });
    expect(sources).toHaveLength(2);

    // Failure 2 → delay is in (1s, 2s]; 900ms is always too early.
    act(() => {
      sources[1].onerror?.(new Event("error"));
    });
    act(() => {
      jest.advanceTimersByTime(900);
    });
    expect(sources).toHaveLength(2);
    act(() => {
      jest.advanceTimersByTime(1_100);
    });
    expect(sources).toHaveLength(3);
  });

  it("unmount cancels the pending retry and closes the live source", () => {
    const { unmount } = renderPanel(jest.fn());

    act(() => {
      sources[0].onerror?.(new Event("error"));
    });

    unmount();

    act(() => {
      jest.advanceTimersByTime(60_000);
    });
    // No zombie reconnect after unmount.
    expect(sources).toHaveLength(1);
  });

  it("unmount while connected closes the EventSource", () => {
    const { unmount } = renderPanel(jest.fn());
    unmount();
    expect(sources[0].close).toHaveBeenCalled();
  });
});
