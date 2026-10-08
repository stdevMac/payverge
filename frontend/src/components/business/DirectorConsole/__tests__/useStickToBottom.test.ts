/** @jest-environment jsdom */
import React from "react";
import { render, renderHook, act, fireEvent } from "@testing-library/react";
import { useStickToBottom } from "../useStickToBottom";

function mockScroller(opts: {
  scrollTop: number;
  clientHeight: number;
  scrollHeight: number;
}): HTMLDivElement {
  const el = document.createElement("div");
  Object.defineProperties(el, {
    scrollTop: {
      configurable: true,
      get: () => opts.scrollTop,
      set: (v: number) => {
        opts.scrollTop = v;
      },
    },
    clientHeight: {
      configurable: true,
      get: () => opts.clientHeight,
    },
    scrollHeight: {
      configurable: true,
      get: () => opts.scrollHeight,
    },
  });
  return el;
}

/** Mount a div so the callback ref attaches and scroll listeners bind. */
function attachScroller(
  result: { current: ReturnType<typeof useStickToBottom> },
  el: HTMLDivElement,
) {
  act(() => {
    result.current.scrollRef(el);
  });
}

describe("useStickToBottom", () => {
  it("starts stuck to bottom and does not show jump chip", () => {
    const { result } = renderHook(() => useStickToBottom({ threshold: 100 }));
    expect(result.current.stickToBottom).toBe(true);
    expect(result.current.showJumpToLatest).toBe(false);
  });

  it("scrolls the container when stuck and content grows", () => {
    const metrics = { scrollTop: 400, clientHeight: 200, scrollHeight: 600 };
    const el = mockScroller(metrics);
    const { result } = renderHook(() => useStickToBottom({ threshold: 100 }));
    attachScroller(result, el);

    metrics.scrollHeight = 800;
    act(() => {
      result.current.onContentChange({ force: false });
    });
    // Hook assigns scrollTop = scrollHeight (browsers clamp to max).
    expect(metrics.scrollTop).toBe(800);
    expect(result.current.showJumpToLatest).toBe(false);
  });

  it("force-scrolls on operator send even if not near bottom", () => {
    const metrics = { scrollTop: 0, clientHeight: 200, scrollHeight: 800 };
    const el = mockScroller(metrics);
    const { result } = renderHook(() => useStickToBottom({ threshold: 100 }));
    attachScroller(result, el);

    // User scrolled up → leave stick mode
    act(() => {
      fireEvent.scroll(el);
    });
    expect(result.current.stickToBottom).toBe(false);

    metrics.scrollHeight = 900;
    act(() => {
      result.current.onContentChange({ force: false });
    });
    expect(metrics.scrollTop).toBe(0);
    expect(result.current.showJumpToLatest).toBe(true);

    act(() => {
      result.current.onContentChange({ force: true });
    });
    expect(metrics.scrollTop).toBe(900);
    expect(result.current.showJumpToLatest).toBe(false);
    expect(result.current.stickToBottom).toBe(true);
  });

  it("jumpToLatest scrolls, hides chip, re-enables stick", () => {
    const metrics = { scrollTop: 0, clientHeight: 200, scrollHeight: 800 };
    const el = mockScroller(metrics);
    const { result } = renderHook(() => useStickToBottom({ threshold: 100 }));
    attachScroller(result, el);

    act(() => {
      fireEvent.scroll(el);
    });
    act(() => {
      result.current.onContentChange({ force: false });
    });
    expect(result.current.showJumpToLatest).toBe(true);

    act(() => {
      result.current.jumpToLatest();
    });
    expect(metrics.scrollTop).toBe(800);
    expect(result.current.showJumpToLatest).toBe(false);
    expect(result.current.stickToBottom).toBe(true);
  });

  it("exposes a callback scrollRef usable on a DOM node", () => {
    function Harness() {
      const { scrollRef } = useStickToBottom();
      return React.createElement("div", {
        ref: scrollRef,
        "data-testid": "scroller",
      });
    }
    const { getByTestId } = render(React.createElement(Harness));
    expect(getByTestId("scroller")).toBeTruthy();
  });
});
