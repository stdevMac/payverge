/** @jest-environment jsdom */

import React from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { AssistantViewport } from "../AssistantViewport";
import type { ContentChange } from "../useAssistantScroll";

type MutableMetrics = {
  scrollTop: number;
  scrollHeight: number;
  clientHeight: number;
};

class ResizeObserverMock {
  static instances: ResizeObserverMock[] = [];

  readonly observe = jest.fn();
  readonly unobserve = jest.fn();
  readonly disconnect = jest.fn();

  constructor(private readonly callback: ResizeObserverCallback) {
    ResizeObserverMock.instances.push(this);
  }

  fire(target: Element): void {
    this.callback(
      [{ target } as ResizeObserverEntry],
      this as unknown as ResizeObserver,
    );
  }
}

function installMetrics(
  element: HTMLElement,
  initial: MutableMetrics,
): MutableMetrics {
  const metrics = { ...initial };
  Object.defineProperties(element, {
    scrollTop: {
      configurable: true,
      get: () => metrics.scrollTop,
      set: (value: number) => {
        const maximum = Math.max(
          0,
          metrics.scrollHeight - metrics.clientHeight,
        );
        metrics.scrollTop = Math.max(0, Math.min(value, maximum));
      },
    },
    scrollHeight: {
      configurable: true,
      get: () => metrics.scrollHeight,
    },
    clientHeight: {
      configurable: true,
      get: () => metrics.clientHeight,
    },
  });
  return metrics;
}

function setRect(element: HTMLElement, top: number, height: number): void {
  jest.spyOn(element, "getBoundingClientRect").mockReturnValue({
    top,
    bottom: top + height,
    height,
    left: 0,
    right: 400,
    width: 400,
    x: 0,
    y: top,
    toJSON: () => ({}),
  });
}

function viewport(change?: ContentChange) {
  return (
    <AssistantViewport
      conversationLabel="Conversation"
      jumpToLatestLabel="Jump to latest"
      contentChange={change}
      contentChangeOrigin={change ? "live" : undefined}
    >
      <p>Latest user turn</p>
      <article>First assistant paragraph</article>
      <p>Continued assistant growth</p>
    </AssistantViewport>
  );
}

const originalResizeObserver = globalThis.ResizeObserver;
const originalMatchMedia = window.matchMedia;
const originalScrollTo = HTMLElement.prototype.scrollTo;

beforeEach(() => {
  ResizeObserverMock.instances = [];
  globalThis.ResizeObserver =
    ResizeObserverMock as unknown as typeof ResizeObserver;
  window.matchMedia = jest.fn().mockReturnValue({
    matches: false,
    media: "(prefers-reduced-motion: reduce)",
    onchange: null,
    addListener: jest.fn(),
    removeListener: jest.fn(),
    addEventListener: jest.fn(),
    removeEventListener: jest.fn(),
    dispatchEvent: jest.fn(),
  });
});

afterEach(() => {
  if (originalResizeObserver) {
    globalThis.ResizeObserver = originalResizeObserver;
  } else {
    Reflect.deleteProperty(globalThis, "ResizeObserver");
  }
  window.matchMedia = originalMatchMedia;
  if (originalScrollTo) {
    HTMLElement.prototype.scrollTo = originalScrollTo;
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, "scrollTo");
  }
  jest.restoreAllMocks();
});

describe("assistant long-response viewport", () => {
  it("anchors a submitted user turn at the viewport top", () => {
    const { rerender } = render(viewport());
    const scroller = screen.getByRole("log", { name: "Conversation" });
    const metrics = installMetrics(scroller, {
      scrollTop: 0,
      scrollHeight: 1_400,
      clientHeight: 400,
    });

    rerender(
      viewport({
        turnId: "turn-user",
        responseStart: null,
        responseComplete: false,
      }),
    );

    expect(metrics.scrollTop).toBe(1_000);
    expect(screen.getByText("Latest user turn")).toBeVisible();
  });

  it("shows the first content of a tall response without jumping to its end", () => {
    const responseStart = document.createElement("article");
    const { rerender } = render(viewport());
    const scroller = screen.getByRole("log", { name: "Conversation" });
    const metrics = installMetrics(scroller, {
      scrollTop: 500,
      scrollHeight: 1_800,
      clientHeight: 400,
    });
    setRect(scroller, 100, 400);
    setRect(responseStart, 620, 760);

    rerender(
      viewport({
        turnId: "turn-long",
        responseStart,
        responseComplete: false,
      }),
    );

    expect(screen.getByText("First assistant paragraph")).toBeVisible();
    expect(metrics.scrollTop).toBe(1_008);
    expect(metrics.scrollTop).toBeLessThan(
      metrics.scrollHeight - metrics.clientHeight,
    );
  });

  it("follows growth near the bottom, freezes after scroll-up, and jumps to latest", () => {
    const responseStart = document.createElement("article");
    const change: ContentChange = {
      turnId: "turn-streaming",
      responseStart,
      responseComplete: false,
    };
    const { rerender } = render(viewport());
    const scroller = screen.getByRole("log", { name: "Conversation" });
    const metrics = installMetrics(scroller, {
      scrollTop: 0,
      scrollHeight: 900,
      clientHeight: 400,
    });
    setRect(scroller, 0, 400);
    setRect(responseStart, 260, 120);

    rerender(viewport(change));
    expect(metrics.scrollTop).toBe(500);
    fireEvent.scroll(scroller);

    metrics.scrollHeight = 1_100;
    act(() => ResizeObserverMock.instances[0].fire(responseStart));
    expect(metrics.scrollTop).toBe(700);

    metrics.scrollTop = 300;
    fireEvent.wheel(scroller);
    fireEvent.scroll(scroller);
    metrics.scrollHeight = 1_400;
    act(() => ResizeObserverMock.instances[0].fire(responseStart));

    expect(metrics.scrollTop).toBe(300);
    fireEvent.click(screen.getByRole("button", { name: "Jump to latest" }));
    expect(metrics.scrollTop).toBe(1_000);
  });

  it("uses instant scrolling when reduced motion is requested", () => {
    window.matchMedia = jest.fn().mockReturnValue({
      matches: true,
      media: "(prefers-reduced-motion: reduce)",
      onchange: null,
      addListener: jest.fn(),
      removeListener: jest.fn(),
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
      dispatchEvent: jest.fn(),
    });
    const scrollTo = jest.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollTo", {
      configurable: true,
      value: scrollTo,
    });
    const { rerender } = render(viewport());
    const scroller = screen.getByRole("log", { name: "Conversation" });
    installMetrics(scroller, {
      scrollTop: 0,
      scrollHeight: 900,
      clientHeight: 400,
    });
    scrollTo.mockClear();

    rerender(
      viewport({
        turnId: "turn-reduced",
        responseStart: null,
        responseComplete: false,
      }),
    );

    expect(scrollTo).toHaveBeenCalledWith({ top: 900, behavior: "auto" });
  });
});
