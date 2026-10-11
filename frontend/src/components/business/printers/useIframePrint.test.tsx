/** @jest-environment jsdom */
import { act, renderHook } from "@testing-library/react";

import { useIframePrint } from "./useIframePrint";

afterEach(() => {
  jest.restoreAllMocks();
  jest.useRealTimers();
  document
    .querySelectorAll("iframe[data-print-hidden]")
    .forEach((el) => el.remove());
});

it("inserts an iframe, calls print(), then removes it after the deferred timer", async () => {
  jest.useFakeTimers();

  const { result } = renderHook(() => useIframePrint());

  const printSpy = jest.fn();
  const origCreate = document.createElement.bind(document);
  jest.spyOn(document, "createElement").mockImplementation((tag) => {
    const el = origCreate(tag);
    if (tag === "iframe") {
      Object.defineProperty(el, "contentWindow", {
        configurable: true,
        get: () => ({
          print: printSpy,
          document: { open() {}, write() {}, close() {} },
          focus() {},
          addEventListener: (_event: string, handler: () => void) => {
            setTimeout(handler, 0);
          },
        }),
      });
      // simulate immediate load
      setTimeout(() => el.dispatchEvent(new Event("load")), 0);
    }
    return el;
  });

  let resolved = false;
  result.current.print("<html><body>hi</body></html>").then(() => {
    resolved = true;
  });

  // Advance past the load dispatch and the afterprint handler it schedules
  // (both 0-delay); stays well under the 500ms deferred-removal timer.
  await act(async () => {
    jest.advanceTimersByTime(1);
    await Promise.resolve(); // let the resolve continuation run
  });

  expect(resolved).toBe(true);
  expect(printSpy).toHaveBeenCalled();
  // Promise resolves promptly, but the iframe is still attached until the deferred removal.
  expect(document.querySelectorAll("iframe[data-print-hidden]")).toHaveLength(1);

  await act(async () => {
    jest.advanceTimersByTime(500);
  });
  expect(document.querySelectorAll("iframe[data-print-hidden]")).toHaveLength(0);
});

it("restores focus to the parent window when finishing", async () => {
  jest.useFakeTimers();
  const focusSpy = jest.spyOn(window, "focus").mockImplementation(() => {});

  const { result } = renderHook(() => useIframePrint());

  const origCreate = document.createElement.bind(document);
  jest.spyOn(document, "createElement").mockImplementation((tag) => {
    const el = origCreate(tag);
    if (tag === "iframe") {
      Object.defineProperty(el, "contentWindow", {
        configurable: true,
        get: () => ({
          print: jest.fn(),
          document: { open() {}, write() {}, close() {} },
          focus() {},
          addEventListener: (_event: string, handler: () => void) => {
            setTimeout(handler, 0);
          },
        }),
      });
      setTimeout(() => el.dispatchEvent(new Event("load")), 0);
    }
    return el;
  });

  result.current.print("<html><body>hi</body></html>");
  await act(async () => {
    jest.advanceTimersByTime(1);
    await Promise.resolve();
  });

  expect(focusSpy).toHaveBeenCalled();

  await act(async () => {
    jest.advanceTimersByTime(500);
  });
});

it("survives the dialog-cancel path where afterprint fires during print()", async () => {
  jest.useFakeTimers();

  const { result } = renderHook(() => useIframePrint());

  let afterPrint: (() => void) | null = null;
  // print() itself synchronously invokes the registered afterprint handler,
  // mirroring Chrome's dialog-cancel behavior (afterprint dispatched while
  // print() is still on the call stack).
  const printSpy = jest.fn(() => {
    afterPrint?.();
  });
  const origCreate = document.createElement.bind(document);
  jest.spyOn(document, "createElement").mockImplementation((tag) => {
    const el = origCreate(tag);
    if (tag === "iframe") {
      Object.defineProperty(el, "contentWindow", {
        configurable: true,
        get: () => ({
          print: printSpy,
          document: { open() {}, write() {}, close() {} },
          focus() {},
          addEventListener: (event: string, handler: () => void) => {
            if (event === "afterprint") afterPrint = handler;
          },
          removeEventListener: jest.fn(),
        }),
      });
      setTimeout(() => el.dispatchEvent(new Event("load")), 0);
    }
    return el;
  });

  let resolved = false;
  const promise = result.current
    .print("<html><body>hi</body></html>")
    .then(() => {
      resolved = true;
    });

  // Trigger the load handler → print() → synchronous afterprint. Must not throw.
  await act(async () => {
    jest.advanceTimersByTime(0);
  });
  await promise;

  expect(printSpy).toHaveBeenCalled();
  expect(resolved).toBe(true);
  // Frame stays attached immediately after cancel — removing it mid-dispatch hangs Chrome.
  expect(document.querySelectorAll("iframe[data-print-hidden]")).toHaveLength(1);

  await act(async () => {
    jest.advanceTimersByTime(500);
  });
  expect(document.querySelectorAll("iframe[data-print-hidden]")).toHaveLength(0);
});

it("keeps the print iframe until afterprint, then removes it via deferred cleanup", async () => {
  jest.useFakeTimers();

  const { result } = renderHook(() => useIframePrint());

  let afterPrint: (() => void) | null = null;
  const printSpy = jest.fn();
  const origCreate = document.createElement.bind(document);
  jest.spyOn(document, "createElement").mockImplementation((tag) => {
    const el = origCreate(tag);
    if (tag === "iframe") {
      Object.defineProperty(el, "contentWindow", {
        configurable: true,
        get: () => ({
          print: printSpy,
          document: { open() {}, write() {}, close() {} },
          focus() {},
          addEventListener: (event: string, handler: () => void) => {
            if (event === "afterprint") afterPrint = handler;
          },
          removeEventListener: jest.fn(),
        }),
      });
      setTimeout(() => el.dispatchEvent(new Event("load")), 0);
    }
    return el;
  });

  const promise = result.current.print("<html><body>hi</body></html>");

  await act(async () => {
    jest.advanceTimersByTime(0);
  });
  expect(printSpy).toHaveBeenCalled();
  expect(document.querySelectorAll("iframe[data-print-hidden]")).toHaveLength(1);

  await act(async () => {
    jest.advanceTimersByTime(300);
  });
  // Still present: afterprint hasn't fired, and the 5s fallback hasn't elapsed.
  expect(document.querySelectorAll("iframe[data-print-hidden]")).toHaveLength(1);

  await act(async () => {
    afterPrint?.();
  });
  await promise;

  // afterprint resolved the promise but deferred the removal.
  expect(document.querySelectorAll("iframe[data-print-hidden]")).toHaveLength(1);

  await act(async () => {
    jest.advanceTimersByTime(500);
  });
  expect(document.querySelectorAll("iframe[data-print-hidden]")).toHaveLength(0);
});

it("falls back to finishing after 5s when afterprint never fires", async () => {
  jest.useFakeTimers();

  const { result } = renderHook(() => useIframePrint());

  const printSpy = jest.fn();
  const origCreate = document.createElement.bind(document);
  jest.spyOn(document, "createElement").mockImplementation((tag) => {
    const el = origCreate(tag);
    if (tag === "iframe") {
      Object.defineProperty(el, "contentWindow", {
        configurable: true,
        get: () => ({
          print: printSpy,
          document: { open() {}, write() {}, close() {} },
          focus() {},
          // Never registers/fires afterprint.
          addEventListener: () => {},
          removeEventListener: jest.fn(),
        }),
      });
      setTimeout(() => el.dispatchEvent(new Event("load")), 0);
    }
    return el;
  });

  const promise = result.current.print("<html><body>hi</body></html>");

  await act(async () => {
    jest.advanceTimersByTime(0);
  });
  expect(printSpy).toHaveBeenCalled();
  expect(document.querySelectorAll("iframe[data-print-hidden]")).toHaveLength(1);

  // Fallback timer (5s) fires finish(), which resolves + schedules deferred removal.
  await act(async () => {
    jest.advanceTimersByTime(5000);
  });
  await promise;
  expect(document.querySelectorAll("iframe[data-print-hidden]")).toHaveLength(1);

  await act(async () => {
    jest.advanceTimersByTime(500);
  });
  expect(document.querySelectorAll("iframe[data-print-hidden]")).toHaveLength(0);
});
