/** @jest-environment jsdom */

import { act, renderHook } from "@testing-library/react";
import { type ContentChange, useAssistantScroll } from "../useAssistantScroll";

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
  initial: Partial<MutableMetrics> = {},
): MutableMetrics {
  const metrics: MutableMetrics = {
    scrollTop: initial.scrollTop ?? 0,
    scrollHeight: initial.scrollHeight ?? 600,
    clientHeight: initial.clientHeight ?? 300,
  };

  Object.defineProperties(element, {
    scrollTop: {
      configurable: true,
      get: () => metrics.scrollTop,
      set: (value: number) => {
        metrics.scrollTop = value;
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
    right: 300,
    width: 300,
    x: 0,
    y: top,
    toJSON: () => ({}),
  });
}

function change(
  turnId: string,
  responseStart: HTMLElement,
  responseComplete = false,
): ContentChange {
  return { turnId, responseStart, responseComplete };
}

const originalResizeObserver = globalThis.ResizeObserver;
const originalMatchMedia = window.matchMedia;
const originalScrollIntoView = HTMLElement.prototype.scrollIntoView;

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
  if (originalScrollIntoView) {
    HTMLElement.prototype.scrollIntoView = originalScrollIntoView;
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
  }
  jest.restoreAllMocks();
});

describe("useAssistantScroll", () => {
  it("opens restored history at the bottom of the scroll container", () => {
    const scroller = document.createElement("div");
    const metrics = installMetrics(scroller, {
      scrollTop: 0,
      scrollHeight: 1400,
      clientHeight: 400,
    });
    const { result } = renderHook(() => useAssistantScroll());

    act(() => result.current.setScroller(scroller));

    expect(metrics.scrollTop).toBe(1400);
    expect(result.current.showJumpToLatest).toBe(false);
  });

  it("keeps a short response at the bottom while the reader is following", () => {
    const scroller = document.createElement("div");
    const response = document.createElement("article");
    scroller.append(response);
    const metrics = installMetrics(scroller, {
      scrollTop: 304,
      scrollHeight: 680,
      clientHeight: 300,
    });
    setRect(scroller, 20, 300);
    setRect(response, 240, 80);
    const { result } = renderHook(() => useAssistantScroll());

    act(() => result.current.setScroller(scroller));
    metrics.scrollTop = 304;
    act(() => result.current.onContentChange(change("turn-1", response)));

    expect(metrics.scrollTop).toBe(680);
    expect(result.current.showJumpToLatest).toBe(false);
  });

  it("aligns the start of a long response relative to the scroll container", () => {
    const scroller = document.createElement("div");
    const response = document.createElement("article");
    scroller.append(response);
    const metrics = installMetrics(scroller, {
      scrollTop: 500,
      scrollHeight: 1800,
      clientHeight: 400,
    });
    setRect(scroller, 100, 400);
    setRect(response, 620, 760);
    const { result } = renderHook(() => useAssistantScroll());

    act(() => result.current.setScroller(scroller));
    metrics.scrollTop = 500;
    act(() => result.current.onContentChange(change("turn-2", response)));

    expect(metrics.scrollTop).toBe(1008);
    expect(result.current.showJumpToLatest).toBe(true);
    expect(result.current.userScrolledAwayTransition).toBe(0);
  });

  it("preserves position after intentional scroll-up beyond the 96px threshold", () => {
    const scroller = document.createElement("div");
    const response = document.createElement("article");
    scroller.append(response);
    const metrics = installMetrics(scroller, {
      scrollTop: 300,
      scrollHeight: 1000,
      clientHeight: 400,
    });
    setRect(scroller, 0, 400);
    setRect(response, 240, 120);
    const { result } = renderHook(() => useAssistantScroll());

    act(() => result.current.setScroller(scroller));
    metrics.scrollTop = 300;
    act(() => scroller.dispatchEvent(new Event("wheel")));
    act(() => scroller.dispatchEvent(new Event("scroll")));
    act(() => result.current.onContentChange(change("turn-3", response)));

    expect(metrics.scrollTop).toBe(300);
    expect(result.current.showJumpToLatest).toBe(true);
    expect(result.current.userScrolledAwayTransition).toBe(1);

    act(() => result.current.onContentChange(change("turn-3", response, true)));
    expect(result.current.userScrolledAwayTransition).toBe(1);
  });

  it("does not drag a long-answer reader when late content growth is observed", () => {
    const scroller = document.createElement("div");
    const response = document.createElement("article");
    scroller.append(response);
    const metrics = installMetrics(scroller, {
      scrollTop: 500,
      scrollHeight: 1700,
      clientHeight: 400,
    });
    setRect(scroller, 100, 400);
    setRect(response, 620, 720);
    const { result } = renderHook(() => useAssistantScroll());

    act(() => result.current.setScroller(scroller));
    metrics.scrollTop = 500;
    act(() => result.current.onContentChange(change("turn-4", response)));
    expect(metrics.scrollTop).toBe(1008);

    metrics.scrollHeight = 2100;
    act(() => ResizeObserverMock.instances[0].fire(scroller));

    expect(metrics.scrollTop).toBe(1008);
    expect(result.current.showJumpToLatest).toBe(true);
  });

  it("jumpToLatest returns to bottom and resumes resize following", () => {
    const scroller = document.createElement("div");
    const response = document.createElement("article");
    scroller.append(response);
    const metrics = installMetrics(scroller, {
      scrollTop: 200,
      scrollHeight: 900,
      clientHeight: 400,
    });
    setRect(scroller, 0, 400);
    setRect(response, 250, 120);
    const { result } = renderHook(() => useAssistantScroll());

    act(() => result.current.setScroller(scroller));
    metrics.scrollTop = 200;
    act(() => scroller.dispatchEvent(new Event("wheel")));
    act(() => scroller.dispatchEvent(new Event("scroll")));
    act(() => result.current.onContentChange(change("turn-5", response)));
    expect(result.current.showJumpToLatest).toBe(true);

    act(() => result.current.jumpToLatest());
    expect(metrics.scrollTop).toBe(900);
    expect(result.current.showJumpToLatest).toBe(false);

    // The browser can emit the scroll event after layout has already grown.
    // The event still belongs to the controller's jump, not to the user.
    metrics.scrollHeight = 1100;
    act(() => scroller.dispatchEvent(new Event("scroll")));
    act(() => ResizeObserverMock.instances[0].fire(scroller));
    expect(metrics.scrollTop).toBe(1100);
  });

  it("keeps following after jumping past an already aligned long response", () => {
    const scroller = document.createElement("div");
    const response = document.createElement("article");
    scroller.append(response);
    const metrics = installMetrics(scroller, {
      scrollTop: 500,
      scrollHeight: 1700,
      clientHeight: 400,
    });
    setRect(scroller, 100, 400);
    setRect(response, 620, 720);
    const { result } = renderHook(() => useAssistantScroll());

    act(() => result.current.setScroller(scroller));
    metrics.scrollTop = 500;
    act(() => result.current.onContentChange(change("turn-long", response)));
    expect(metrics.scrollTop).toBe(1008);

    act(() => result.current.jumpToLatest());
    expect(metrics.scrollTop).toBe(1700);
    expect(result.current.showJumpToLatest).toBe(false);

    metrics.scrollHeight = 2100;
    act(() => ResizeObserverMock.instances[0].fire(response));

    expect(metrics.scrollTop).toBe(2100);
    expect(result.current.showJumpToLatest).toBe(false);
  });

  it("uses auto behavior for reduced motion and never scrolls the document", () => {
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
    const scroller = document.createElement("div");
    const response = document.createElement("article");
    scroller.append(response);
    installMetrics(scroller, {
      scrollTop: 0,
      scrollHeight: 900,
      clientHeight: 400,
    });
    setRect(scroller, 0, 400);
    setRect(response, 260, 100);
    const scrollTo = jest.fn();
    Object.defineProperty(scroller, "scrollTo", {
      configurable: true,
      value: scrollTo,
    });
    const windowScrollTo = jest.spyOn(window, "scrollTo").mockImplementation();
    const scrollIntoView = jest.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      configurable: true,
      value: scrollIntoView,
    });
    const { result } = renderHook(() => useAssistantScroll());

    act(() => result.current.setScroller(scroller));
    act(() => result.current.onContentChange(change("turn-6", response)));
    act(() => result.current.jumpToLatest());

    expect(scrollTo).toHaveBeenCalledWith({ top: 900, behavior: "auto" });
    expect(windowScrollTo).not.toHaveBeenCalled();
    expect(scrollIntoView).not.toHaveBeenCalled();
  });

  it("treats exactly 96px from bottom as following and 97px as user intent", () => {
    const scroller = document.createElement("div");
    const response = document.createElement("article");
    scroller.append(response);
    const metrics = installMetrics(scroller, {
      scrollTop: 504,
      scrollHeight: 1000,
      clientHeight: 400,
    });
    setRect(scroller, 0, 400);
    setRect(response, 300, 80);
    const { result } = renderHook(() => useAssistantScroll());

    act(() => result.current.setScroller(scroller));
    metrics.scrollTop = 504;
    act(() => scroller.dispatchEvent(new Event("wheel")));
    act(() => scroller.dispatchEvent(new Event("scroll")));
    act(() => result.current.onContentChange(change("turn-7", response)));
    expect(metrics.scrollTop).toBe(1000);

    metrics.scrollTop = 503;
    act(() => scroller.dispatchEvent(new Event("wheel")));
    act(() => scroller.dispatchEvent(new Event("scroll")));
    metrics.scrollHeight = 1100;
    act(() => result.current.onContentChange(change("turn-7", response, true)));
    expect(metrics.scrollTop).toBe(503);
    expect(result.current.showJumpToLatest).toBe(true);
  });

  it.each(["wheel", "touchmove"])(
    "ignores intermediate smooth scrolling until explicit %s intent",
    (intentEvent) => {
      const scroller = document.createElement("div");
      const metrics = installMetrics(scroller, {
        scrollTop: 200,
        scrollHeight: 1000,
        clientHeight: 400,
      });
      const scrollTo = jest.fn();
      Object.defineProperty(scroller, "scrollTo", {
        configurable: true,
        value: scrollTo,
      });
      const { result } = renderHook(() => useAssistantScroll());

      act(() => result.current.setScroller(scroller));
      act(() => result.current.jumpToLatest());
      expect(scrollTo).toHaveBeenLastCalledWith({
        top: 1000,
        behavior: "smooth",
      });

      metrics.scrollTop = 450;
      act(() => scroller.dispatchEvent(new Event("scroll")));
      expect(result.current.showJumpToLatest).toBe(false);

      act(() => scroller.dispatchEvent(new Event(intentEvent)));
      metrics.scrollTop = 400;
      act(() => scroller.dispatchEvent(new Event("scroll")));
      expect(result.current.showJumpToLatest).toBe(true);
    },
  );

  it.each(["pointerdown", "touchstart"])(
    "keeps a smooth jump active through a nested %s tap",
    (tapEvent) => {
      const scroller = document.createElement("div");
      const response = document.createElement("article");
      const nestedAction = document.createElement("a");
      nestedAction.href = "/pricing";
      response.append(nestedAction);
      scroller.append(response);
      const metrics = installMetrics(scroller, {
        scrollTop: 200,
        scrollHeight: 1000,
        clientHeight: 400,
      });
      const scrollTo = jest.fn();
      Object.defineProperty(scroller, "scrollTo", {
        configurable: true,
        value: scrollTo,
      });
      const { result } = renderHook(() => useAssistantScroll());

      act(() => result.current.setScroller(scroller));
      act(() => result.current.jumpToLatest());
      act(() =>
        nestedAction.dispatchEvent(new Event(tapEvent, { bubbles: true })),
      );

      metrics.scrollTop = 450;
      act(() => scroller.dispatchEvent(new Event("scroll")));
      expect(result.current.showJumpToLatest).toBe(false);

      metrics.scrollHeight = 1200;
      act(() => ResizeObserverMock.instances[0].fire(response));
      expect(scrollTo).toHaveBeenLastCalledWith({
        top: 1200,
        behavior: "smooth",
      });
      expect(result.current.showJumpToLatest).toBe(false);
    },
  );

  it.each([
    ["button child", "button", " "],
    ["link child", "link", "ArrowDown"],
    ["input", "input", "Home"],
    ["textarea", "textarea", "PageDown"],
    ["contenteditable child", "contenteditable", "End"],
  ])(
    "keeps a smooth jump active for %s keyboard input",
    (_label, controlKind, key) => {
      const scroller = document.createElement("div");
      const control = document.createElement(
        controlKind === "link"
          ? "a"
          : controlKind === "contenteditable"
            ? "div"
            : controlKind,
      );
      if (controlKind === "link") {
        control.setAttribute("href", "/pricing");
      }
      if (controlKind === "contenteditable") {
        control.setAttribute("contenteditable", "true");
      }
      const usesChildTarget = ["button", "link", "contenteditable"].includes(
        controlKind,
      );
      const eventTarget = usesChildTarget
        ? document.createElement("span")
        : control;
      if (usesChildTarget) {
        control.append(eventTarget);
      }
      scroller.append(control);
      const metrics = installMetrics(scroller, {
        scrollTop: 200,
        scrollHeight: 1000,
        clientHeight: 400,
      });
      Object.defineProperty(scroller, "scrollTo", {
        configurable: true,
        value: jest.fn(),
      });
      const { result } = renderHook(() => useAssistantScroll());

      act(() => result.current.setScroller(scroller));
      act(() => result.current.jumpToLatest());
      act(() =>
        eventTarget.dispatchEvent(
          new KeyboardEvent("keydown", { key, bubbles: true }),
        ),
      );

      metrics.scrollTop = 450;
      act(() => scroller.dispatchEvent(new Event("scroll")));
      expect(result.current.showJumpToLatest).toBe(false);
    },
  );

  it("treats keyboard navigation on the scroll background as scroll intent", () => {
    const scroller = document.createElement("div");
    const metrics = installMetrics(scroller, {
      scrollTop: 200,
      scrollHeight: 1000,
      clientHeight: 400,
    });
    Object.defineProperty(scroller, "scrollTo", {
      configurable: true,
      value: jest.fn(),
    });
    const { result } = renderHook(() => useAssistantScroll());

    act(() => result.current.setScroller(scroller));
    act(() => result.current.jumpToLatest());
    act(() =>
      scroller.dispatchEvent(
        new KeyboardEvent("keydown", { key: "ArrowUp", bubbles: true }),
      ),
    );

    metrics.scrollTop = 400;
    act(() => scroller.dispatchEvent(new Event("scroll")));
    expect(result.current.showJumpToLatest).toBe(true);
  });

  it("counts a native scrollbar drag from the scroll background once", () => {
    const scroller = document.createElement("div");
    const metrics = installMetrics(scroller, {
      scrollTop: 600,
      scrollHeight: 1000,
      clientHeight: 400,
    });
    const { result } = renderHook(() => useAssistantScroll());

    act(() => result.current.setScroller(scroller));
    metrics.scrollTop = 300;
    act(() => scroller.dispatchEvent(new Event("pointerdown")));
    act(() => scroller.dispatchEvent(new Event("scroll")));
    act(() => scroller.dispatchEvent(new Event("scroll")));

    expect(result.current.showJumpToLatest).toBe(true);
    expect(result.current.userScrolledAwayTransition).toBe(1);
  });

  it.each(["pointerdown", "touchstart", "wheel"])(
    "does not treat a nested %s without scroll movement as scroll-away intent",
    (eventType) => {
      const scroller = document.createElement("div");
      const response = document.createElement("article");
      const nestedAction = document.createElement("a");
      nestedAction.href = "/pricing";
      response.append(nestedAction);
      scroller.append(response);
      const metrics = installMetrics(scroller, {
        scrollTop: 500,
        scrollHeight: 900,
        clientHeight: 400,
      });
      setRect(scroller, 0, 400);
      setRect(response, 260, 100);
      const { result } = renderHook(() => useAssistantScroll());

      act(() => result.current.setScroller(scroller));
      act(() =>
        nestedAction.dispatchEvent(new Event(eventType, { bubbles: true })),
      );
      metrics.scrollHeight = 1100;
      act(() =>
        result.current.onContentChange(change("turn-action", response)),
      );

      expect(metrics.scrollTop).toBe(1100);
      expect(result.current.showJumpToLatest).toBe(false);
    },
  );

  it("enters long-answer reading mode when observed streaming growth crosses the viewport", () => {
    const scroller = document.createElement("div");
    const response = document.createElement("article");
    scroller.append(response);
    const metrics = installMetrics(scroller, {
      scrollTop: 500,
      scrollHeight: 1300,
      clientHeight: 400,
    });
    let responseHeight = 120;
    jest.spyOn(scroller, "getBoundingClientRect").mockReturnValue({
      top: 100,
      bottom: 500,
      height: 400,
      left: 0,
      right: 300,
      width: 300,
      x: 0,
      y: 100,
      toJSON: () => ({}),
    });
    jest.spyOn(response, "getBoundingClientRect").mockImplementation(() => ({
      top: 120,
      bottom: 120 + responseHeight,
      height: responseHeight,
      left: 0,
      right: 300,
      width: 300,
      x: 0,
      y: 120,
      toJSON: () => ({}),
    }));
    const { result } = renderHook(() => useAssistantScroll());

    act(() => result.current.setScroller(scroller));
    metrics.scrollTop = 500;
    act(() => result.current.onContentChange(change("turn-9", response)));
    expect(metrics.scrollTop).toBe(1300);

    responseHeight = 720;
    metrics.scrollHeight = 1900;
    act(() => ResizeObserverMock.instances[0].fire(response));
    expect(metrics.scrollTop).toBe(1308);
    expect(result.current.showJumpToLatest).toBe(true);

    metrics.scrollHeight = 2200;
    act(() => ResizeObserverMock.instances[0].fire(response));
    expect(metrics.scrollTop).toBe(1308);
  });

  it("observes the active response when earlier sibling messages are static", () => {
    const scroller = document.createElement("div");
    const oldMessage = document.createElement("article");
    const response = document.createElement("article");
    scroller.append(oldMessage, response);
    installMetrics(scroller, {
      scrollTop: 500,
      scrollHeight: 1000,
      clientHeight: 400,
    });
    setRect(scroller, 0, 400);
    setRect(response, 300, 100);
    const { result } = renderHook(() => useAssistantScroll());

    act(() => result.current.setScroller(scroller));
    act(() => result.current.onContentChange(change("turn-10", response)));

    expect(
      ResizeObserverMock.instances[0].observe.mock.calls.some(
        ([observed]) => observed === response,
      ),
    ).toBe(true);
  });

  it("unobserves the previous active response when it changes or clears", () => {
    const scroller = document.createElement("div");
    const firstResponse = document.createElement("article");
    const nextResponse = document.createElement("article");
    scroller.append(firstResponse, nextResponse);
    installMetrics(scroller, {
      scrollTop: 500,
      scrollHeight: 1000,
      clientHeight: 400,
    });
    setRect(scroller, 0, 400);
    setRect(firstResponse, 200, 100);
    setRect(nextResponse, 300, 100);
    const { result } = renderHook(() => useAssistantScroll());

    act(() => result.current.setScroller(scroller));
    act(() =>
      result.current.onContentChange(change("turn-first", firstResponse)),
    );
    act(() =>
      result.current.onContentChange(change("turn-next", nextResponse)),
    );
    expect(ResizeObserverMock.instances[0].unobserve).toHaveBeenCalledWith(
      firstResponse,
    );

    act(() =>
      result.current.onContentChange({
        turnId: "turn-loading",
        responseStart: null,
        responseComplete: false,
      }),
    );
    expect(ResizeObserverMock.instances[0].unobserve).toHaveBeenCalledWith(
      nextResponse,
    );
  });

  it("reveals a newly sent turn and loading state even after reading older content", () => {
    const scroller = document.createElement("div");
    const metrics = installMetrics(scroller, {
      scrollTop: 200,
      scrollHeight: 1000,
      clientHeight: 400,
    });
    const { result } = renderHook(() => useAssistantScroll());

    act(() => result.current.setScroller(scroller));
    metrics.scrollTop = 200;
    act(() => scroller.dispatchEvent(new Event("wheel")));
    act(() => scroller.dispatchEvent(new Event("scroll")));
    expect(result.current.showJumpToLatest).toBe(true);

    act(() =>
      result.current.onContentChange({
        turnId: "turn-11",
        responseStart: null,
        responseComplete: false,
      }),
    );

    expect(metrics.scrollTop).toBe(1000);
    expect(result.current.showJumpToLatest).toBe(false);
  });
});
