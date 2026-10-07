/** @jest-environment jsdom */

import { act, renderHook } from "@testing-library/react";

import { sparseRestaurantMenu } from "../../../../lib/menuPrint/__fixtures__/restaurantMenus";
import { planFixtureDocument } from "../../../../lib/menuPrint/__fixtures__/plannedDocuments";
import { renderPlannedMenuHtml } from "../../../../lib/menuPrint/renderPlannedHtml";

import { useMenuPrintWindow } from "./useMenuPrintWindow";

const HTML = "<!doctype html><html><body>Menu</body></html>";

type Completion = "afterprint-sync" | "never";

function createMockPrintWindow(
  input: {
    completion?: Completion;
    ready?: boolean;
  } = {},
) {
  let written = "";
  let jobId = "";
  let lifecycle: HTMLScriptElement | null = null;
  const completion = input.completion ?? "never";

  const child = {
    closed: false,
    focus: jest.fn(),
    print: jest.fn(() => {
      if (completion === "afterprint-sync") emit("complete");
    }),
    close: jest.fn(() => {
      child.closed = true;
    }),
    document: {
      open: jest.fn(() => {
        written = "";
        lifecycle = null;
      }),
      write: jest.fn((value: string) => {
        written += value;
      }),
      close: jest.fn(() => {
        jobId =
          written.match(/name="payverge-print-job" content="([^"]+)"/)?.[1] ??
          "";
      }),
      createElement: jest.fn((tagName: string) =>
        document.createElement(tagName),
      ),
      body: {
        appendChild: jest.fn((node: HTMLScriptElement) => {
          lifecycle = node;
          if (input.ready !== false) queueMicrotask(() => emit("ready"));
          return node;
        }),
      },
    },
  };

  function emit(
    type: "ready" | "complete",
    overrides: Record<string, unknown> = {},
  ) {
    window.dispatchEvent(
      new MessageEvent("message", {
        origin: window.location.origin,
        source: child as unknown as Window,
        data: {
          source: "payverge-menu-print",
          jobId,
          type,
          ...overrides,
        },
      }),
    );
  }

  return {
    window: child as unknown as Window,
    child,
    emit,
    get jobId() {
      return jobId;
    },
    get written() {
      if (!lifecycle) return written;
      return written.replace(/<\/body\s*>/i, `${lifecycle.outerHTML}</body>`);
    },
    get lifecycle() {
      return lifecycle;
    },
  };
}

afterEach(() => {
  jest.restoreAllMocks();
  jest.useRealTimers();
  document
    .querySelectorAll("[data-menu-print-nonce-test]")
    .forEach((element) => element.remove());
});

it("opens a dedicated named same-origin print window synchronously", async () => {
  const child = createMockPrintWindow({ completion: "afterprint-sync" });
  const open = jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() => useMenuPrintWindow());

  const promise = result.current.print(HTML);

  expect(open).toHaveBeenCalledWith(
    "",
    "payverge-menu-print",
    "popup,width=1200,height=900",
  );
  await expect(promise).resolves.toEqual({ status: "complete" });
});

it("returns blocked without changing parent focus when window.open is denied", async () => {
  jest.spyOn(window, "open").mockReturnValue(null);
  const focus = jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() => useMenuPrintWindow());

  await expect(
    result.current.print("<!doctype html><body>Menu</body>"),
  ).resolves.toEqual({ status: "blocked" });
  expect(focus).not.toHaveBeenCalled();
});

it("resolves synchronous cancellation, restores focus, and permits immediate repeat print", async () => {
  jest.useFakeTimers();
  const first = createMockPrintWindow({ completion: "afterprint-sync" });
  const second = createMockPrintWindow({ completion: "afterprint-sync" });
  jest
    .spyOn(window, "open")
    .mockReturnValueOnce(first.window)
    .mockReturnValueOnce(second.window);
  const focus = jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() => useMenuPrintWindow());

  const firstPromise = result.current.print(HTML);
  await act(async () => jest.runAllTicks());
  await expect(firstPromise).resolves.toEqual({ status: "complete" });
  const secondPromise = result.current.print(HTML);
  await act(async () => jest.runAllTicks());
  await expect(secondPromise).resolves.toEqual({ status: "complete" });

  expect(focus).toHaveBeenCalledTimes(2);
  expect(first.child.print).toHaveBeenCalledTimes(1);
  expect(second.child.print).toHaveBeenCalledTimes(1);
  expect(first.jobId).not.toBe(second.jobId);

  await act(async () => jest.advanceTimersByTime(500));
  expect(first.child.close).toHaveBeenCalledTimes(1);
  expect(second.child.close).toHaveBeenCalledTimes(1);
});

it("does not let a deferred close from the prior job close a reused named window", async () => {
  jest.useFakeTimers();
  const child = createMockPrintWindow({ completion: "never" });
  child.child.print
    .mockImplementationOnce(() => child.emit("complete"))
    .mockImplementationOnce(() => undefined);
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() => useMenuPrintWindow());

  const first = result.current.print(HTML);
  await act(async () => jest.runAllTicks());
  await expect(first).resolves.toEqual({ status: "complete" });
  const second = result.current.print(HTML);
  await act(async () => jest.runAllTicks());
  expect(child.child.print).toHaveBeenCalledTimes(2);

  await act(async () => jest.advanceTimersByTime(500));
  expect(child.child.close).not.toHaveBeenCalled();
  expect(child.child.closed).toBe(false);

  child.emit("complete");
  await expect(second).resolves.toEqual({ status: "complete" });
  await act(async () => jest.advanceTimersByTime(500));
  expect(child.child.close).toHaveBeenCalledTimes(1);
});

it.each(["different", "reused"] as const)(
  "supersedes an unready job exactly once when the next print uses a %s child",
  async (childMode) => {
    jest.useFakeTimers();
    const firstChild = createMockPrintWindow({ ready: false });
    const secondChild =
      childMode === "reused"
        ? firstChild
        : createMockPrintWindow({ ready: false });
    jest
      .spyOn(window, "open")
      .mockReturnValueOnce(firstChild.window)
      .mockReturnValueOnce(secondChild.window);
    jest.spyOn(window, "focus").mockImplementation(() => {});
    const { result } = renderHook(() => useMenuPrintWindow());
    const firstSettled = jest.fn();

    const first = result.current.print(HTML).then((value) => {
      firstSettled(value);
      return value;
    });
    const firstJobId = firstChild.jobId;
    const second = result.current.print(HTML);

    await expect(first).resolves.toEqual({ status: "superseded" });
    expect(firstSettled).toHaveBeenCalledTimes(1);
    expect(firstChild.child.print).not.toHaveBeenCalled();

    firstChild.emit("ready", { jobId: firstJobId });
    expect(firstChild.child.print).not.toHaveBeenCalled();
    secondChild.emit("ready");
    expect(secondChild.child.print).toHaveBeenCalledTimes(1);
    secondChild.emit("complete");
    await expect(second).resolves.toEqual({ status: "complete" });

    firstChild.emit("complete");
    await act(async () => jest.advanceTimersByTime(500));
    expect(firstSettled).toHaveBeenCalledTimes(1);
  },
);

it("starts the completion timeout only after validated readiness invokes print", async () => {
  jest.useFakeTimers();
  const child = createMockPrintWindow({ ready: false });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() =>
    useMenuPrintWindow({ completionTimeoutMs: 100, readyTimeoutMs: 1_000 }),
  );
  const settled = jest.fn();

  const promise = result.current.print(HTML).then((value) => {
    settled(value);
    return value;
  });
  await act(async () => jest.advanceTimersByTime(500));
  expect(settled).not.toHaveBeenCalled();
  expect(child.child.print).not.toHaveBeenCalled();

  child.emit("ready");
  expect(child.child.print).toHaveBeenCalledTimes(1);
  await act(async () => jest.advanceTimersByTime(99));
  expect(settled).not.toHaveBeenCalled();
  await act(async () => jest.advanceTimersByTime(1));
  await expect(promise).resolves.toEqual({ status: "timeout" });
});

it("bounds a child that never reports readiness with a parent watchdog grace", async () => {
  jest.useFakeTimers();
  const child = createMockPrintWindow({ ready: false });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() =>
    useMenuPrintWindow({ completionTimeoutMs: 10, readyTimeoutMs: 100 }),
  );
  const settled = jest.fn();

  const promise = result.current.print(HTML).then((value) => {
    settled(value);
    return value;
  });
  await act(async () => jest.advanceTimersByTime(349));
  expect(settled).not.toHaveBeenCalled();
  expect(child.child.print).not.toHaveBeenCalled();
  await act(async () => jest.advanceTimersByTime(1));
  await expect(promise).resolves.toEqual({ status: "timeout" });
});

it("uses a bounded fallback when afterprint never arrives", async () => {
  jest.useFakeTimers();
  const child = createMockPrintWindow({ completion: "never" });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() =>
    useMenuPrintWindow({ completionTimeoutMs: 30_000 }),
  );

  const promise = result.current.print(HTML);
  await act(async () => {
    await Promise.resolve();
    jest.advanceTimersByTime(30_000);
  });

  await expect(promise).resolves.toEqual({ status: "timeout" });
  expect(child.child.print).toHaveBeenCalledTimes(1);
});

it("finishes when the child window closes without an afterprint event", async () => {
  jest.useFakeTimers();
  const child = createMockPrintWindow({ completion: "never" });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() =>
    useMenuPrintWindow({ closedPollMs: 100, completionTimeoutMs: 30_000 }),
  );

  const promise = result.current.print(HTML);
  await act(async () => {
    await Promise.resolve();
    child.child.closed = true;
    jest.advanceTimersByTime(100);
  });

  await expect(promise).resolves.toEqual({ status: "complete" });
});

it("ignores duplicate, stale, forged, and unsupported lifecycle messages", async () => {
  jest.useFakeTimers();
  const child = createMockPrintWindow({ ready: false });
  const stranger = createMockPrintWindow({ ready: false });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() => useMenuPrintWindow());

  const promise = result.current.print(HTML);
  const validData = {
    source: "payverge-menu-print",
    jobId: child.jobId,
    type: "ready",
  };
  window.dispatchEvent(
    new MessageEvent("message", {
      origin: "https://forged.example",
      source: child.window,
      data: validData,
    }),
  );
  window.dispatchEvent(
    new MessageEvent("message", {
      origin: window.location.origin,
      source: stranger.window,
      data: validData,
    }),
  );
  child.emit("ready", { source: "someone-else" });
  child.emit("ready", { jobId: "stale-job" });
  child.emit("ready", { type: "launch" });
  expect(child.child.print).not.toHaveBeenCalled();

  child.emit("ready");
  child.emit("ready");
  expect(child.child.print).toHaveBeenCalledTimes(1);
  child.emit("complete");
  child.emit("complete");
  await expect(promise).resolves.toEqual({ status: "complete" });

  await act(async () => jest.advanceTimersByTime(500));
});

it("cleans an active job on unmount and resolves it exactly once", async () => {
  jest.useFakeTimers();
  const child = createMockPrintWindow({ ready: false });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  const focus = jest.spyOn(window, "focus").mockImplementation(() => {});
  const removeListener = jest.spyOn(window, "removeEventListener");
  const { result, unmount } = renderHook(() => useMenuPrintWindow());
  const settled = jest.fn();

  const promise = result.current.print(HTML).then((value) => {
    settled(value);
    return value;
  });
  unmount();

  await expect(promise).resolves.toEqual({ status: "complete" });
  expect(settled).toHaveBeenCalledTimes(1);
  expect(child.child.close).toHaveBeenCalledTimes(1);
  expect(removeListener).toHaveBeenCalledWith("message", expect.any(Function));
  expect(focus).toHaveBeenCalledTimes(1);

  child.emit("complete");
  await act(async () => jest.runOnlyPendingTimers());
  expect(settled).toHaveBeenCalledTimes(1);
});

it("lets the owning modal cancel an active print job immediately", async () => {
  const child = createMockPrintWindow({ ready: false });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() => useMenuPrintWindow());

  const promise = result.current.print(HTML);
  act(() => result.current.cancel());

  await expect(promise).resolves.toEqual({ status: "superseded" });
  expect(child.child.close).toHaveBeenCalledTimes(1);
});

it.each([
  ["body only", "<!doctype html><body><h1>Menu</h1></body>"],
  ["head only", "<html><head><title>Menu</title></head><p>Menu</p></html>"],
  ["unclosed head", "<html><head><title>Menu</title>"],
  ["fragment", "<h1>Menu</h1>"],
])("injects safe lifecycle markup into %s HTML", async (_label, html) => {
  const child = createMockPrintWindow({ completion: "afterprint-sync" });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() => useMenuPrintWindow());

  await expect(result.current.print(html)).resolves.toEqual({
    status: "complete",
  });

  expect(child.written).toMatch(/^<!doctype html>/i);
  expect(child.written.match(/<html[\s>]/gi)).toHaveLength(1);
  expect(child.written.match(/<head[\s>]/gi)).toHaveLength(1);
  expect(child.written.match(/<body[\s>]/gi)).toHaveLength(1);
  expect(child.written.toLowerCase().indexOf("<head")).toBeLessThan(
    child.written.toLowerCase().indexOf("<body"),
  );
  expect(
    child.written.match(/name="payverge-print-job" content="[^"]+"/g),
  ).toHaveLength(1);
  expect(child.written).toContain("document.fonts");
  expect(child.written).toContain("document.images");
  expect(child.written).toContain("Promise.allSettled");
  expect(child.written).toContain("Promise.race");
  expect(child.written).toContain('addEventListener("afterprint"');
  expect(child.written).toContain('addEventListener("pagehide"');
  expect(child.written.indexOf("payverge-print-lifecycle")).toBeLessThan(
    child.written.toLowerCase().lastIndexOf("</body>"),
  );
});

it("clamps fractional readiness durations to a positive millisecond", async () => {
  const child = createMockPrintWindow({ completion: "afterprint-sync" });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() =>
    useMenuPrintWindow({ readyTimeoutMs: 0.5 }),
  );

  await result.current.print(HTML);
  expect(child.written).toContain("const READY_TIMEOUT_MS = 1;");
});

it("replaces a caller-supplied job marker and does not execute injected job data", async () => {
  const child = createMockPrintWindow({ completion: "afterprint-sync" });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() => useMenuPrintWindow());

  await result.current.print(
    '<html><head><meta name="payverge-print-job" content="forged"></head><body>Menu</body></html>',
  );

  expect(child.written).not.toContain('content="forged"');
  expect(child.written.match(/name="payverge-print-job"/g)).toHaveLength(1);
  expect(child.written).not.toContain("</script><script>");
});

it.each(["script", "style"] as const)(
  "copies the active parent CSP nonce from a trusted %s element",
  async (tagName) => {
    const nonce = "ZmFrZS1wYXl2ZXJnZS1ub25jZQ==";
    const trusted = document.createElement(tagName);
    trusted.setAttribute("data-menu-print-nonce-test", "true");
    trusted.nonce = nonce;
    document.head.appendChild(trusted);
    const child = createMockPrintWindow({ completion: "afterprint-sync" });
    jest.spyOn(window, "open").mockReturnValue(child.window);
    jest.spyOn(window, "focus").mockImplementation(() => {});
    const { result } = renderHook(() => useMenuPrintWindow());

    await expect(result.current.print(HTML)).resolves.toEqual({
      status: "complete",
    });

    expect(child.lifecycle?.nonce).toBe(nonce);
    expect(child.child.document.body.appendChild).toHaveBeenCalledWith(
      child.lifecycle,
    );
  },
);

it("keeps the nonce on the live appended node when serialization hides it", async () => {
  const nonce = "Y3NwLW5vbmNlLWhpZGRlbi1hdHRy";
  const trusted = document.createElement("script");
  trusted.setAttribute("data-menu-print-nonce-test", "true");
  trusted.nonce = nonce;
  document.head.appendChild(trusted);
  const outerHtml = Object.getOwnPropertyDescriptor(
    Element.prototype,
    "outerHTML",
  )?.get;
  if (!outerHtml) throw new Error("outerHTML getter unavailable");
  jest
    .spyOn(Element.prototype, "outerHTML", "get")
    .mockImplementation(function (this: Element) {
      return String(outerHtml.call(this)).replace(
        `nonce="${nonce}"`,
        'nonce=""',
      );
    });
  const child = createMockPrintWindow({ completion: "afterprint-sync" });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() => useMenuPrintWindow());

  await result.current.print(HTML);

  expect(child.lifecycle?.nonce).toBe(nonce);
  expect(child.written).toContain('nonce=""');
  expect(child.child.document.body.appendChild).toHaveBeenCalledWith(
    child.lifecycle,
  );
});

it("leaves the lifecycle nonce-free when the active document has no CSP nonce", async () => {
  const child = createMockPrintWindow({ completion: "afterprint-sync" });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() => useMenuPrintWindow());

  await result.current.print(HTML);

  expect(child.lifecycle?.hasAttribute("nonce")).toBe(false);
});

it("rejects an unsafe parent nonce and never serializes it into the child", async () => {
  const unsafeNonce = 'bad-nonce"><script>alert(1)</script>';
  const trusted = document.createElement("script");
  trusted.setAttribute("data-menu-print-nonce-test", "true");
  trusted.nonce = unsafeNonce;
  document.head.appendChild(trusted);
  const child = createMockPrintWindow({ completion: "afterprint-sync" });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() => useMenuPrintWindow());

  await result.current.print(
    '<html><head><script nonce="source-forgery">alert(2)</script></head><body>Menu</body></html>',
  );

  expect(child.lifecycle?.hasAttribute("nonce")).toBe(false);
  expect(child.written).not.toContain("bad-nonce");
  expect(child.written).not.toContain("source-forgery");
});

it("constructs lifecycle nodes structurally despite raw-text closing-tag strings", async () => {
  const child = createMockPrintWindow({ completion: "afterprint-sync" });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() => useMenuPrintWindow());
  const hostileHtml = `<!doctype html><html><head>
    <meta name="payverge-print-job" content="forged">
    <style>body::after { content: "</head></body>"; }</style>
    <script id="payverge-print-lifecycle">window.__forged = true;</script>
  </head><body>
    <script>window.__untrusted = "</body>";</script>
    <main>Restaurant menu</main>
  </body></html>`;

  await expect(result.current.print(hostileHtml)).resolves.toEqual({
    status: "complete",
  });

  const parsed = new DOMParser().parseFromString(child.written, "text/html");
  const lifecycle = parsed.querySelector("#payverge-print-lifecycle");
  expect(
    parsed.querySelectorAll('meta[name="payverge-print-job"]'),
  ).toHaveLength(1);
  expect(parsed.querySelectorAll("script")).toHaveLength(1);
  expect(lifecycle?.parentElement).toBe(parsed.body);
  expect(parsed.querySelector("style")).toBeNull();
  expect(child.written).not.toContain("window.__forged");
  expect(child.written).not.toContain("window.__untrusted");
});

it("removes active-content bypass vectors before injecting the lifecycle", async () => {
  const child = createMockPrintWindow({ completion: "afterprint-sync" });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() => useMenuPrintWindow());
  const bypassHtml = `<!doctype html><html><head>
    <meta http-equiv="refresh" content="0;url=javascript:alert(1)">
    <base href="https://attacker.example/">
    <link rel="stylesheet" href="javascript:alert(2)">
  </head><body>
    <img id="encoded-js" src="java&#10;script:alert(3)" onerror="alert(4)">
    <iframe srcdoc="<script>alert(5)</script>"></iframe>
    <object data="data:text/html,<script>alert(6)</script>"></object>
    <embed src="https://attacker.example/payload">
    <form action="javascript:alert(7)"><button formaction="javascript:alert(8)">Go</button></form>
    <svg><a xlink:href="javascript:alert(9)">svg</a></svg>
    <math href="javascript:alert(10)"></math>
  </body></html>`;

  await expect(result.current.print(bypassHtml)).resolves.toEqual({
    status: "complete",
  });

  const parsed = new DOMParser().parseFromString(child.written, "text/html");
  expect(
    parsed.querySelectorAll(
      "iframe, object, embed, form, button, input, svg, math, base, link",
    ),
  ).toHaveLength(0);
  expect(parsed.querySelector("meta[http-equiv]")).toBeNull();
  expect(child.written).not.toContain("0;url=");
  expect(parsed.querySelector("#encoded-js")).toBeNull();
  expect(child.written.toLowerCase()).not.toContain("javascript:");
  expect(child.written.toLowerCase()).not.toContain("srcdoc");
  expect(child.written.toLowerCase()).not.toContain("formaction");
  expect(child.written.toLowerCase()).not.toContain("xlink:href");
  expect(parsed.querySelectorAll("script")).toHaveLength(1);
  expect(parsed.querySelector("#payverge-print-lifecycle")?.parentElement).toBe(
    parsed.body,
  );
});

it("preserves planned menu structure, font CSS, semantic attributes, and safe images", async () => {
  const child = createMockPrintWindow({ completion: "afterprint-sync" });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() => useMenuPrintWindow());
  const plannedHtml = `<!doctype html><html lang="es" dir="ltr"><head>
    <meta charset="utf-8"><meta name="viewport" content="width=device-width">
    <title>Casa Verde — Menu</title>
    <style>@font-face { font-family: "DM Sans"; src: url("/fonts/dm-sans/menu.woff2") format("woff2"); }
      .print-page { color: #111; }</style>
  </head><body class="menu-document" data-family="atelier">
    <section class="print-page" data-page="1" aria-label="Menu page">
      <article><h1>Casa Verde</h1><h2>Starters</h2><h3>Soup</h3>
      <div style="left: 10mm"><p>Seasonal soup</p><span>$12</span></div></article>
      <figure><img src="https://images.example/dish.jpg" alt="Dish"><figcaption>Dish</figcaption></figure>
      <img src="/images/logo.png" alt="Logo">
      <img src="data:image/png;base64,iVBORw0KGgo=" alt="QR">
    </section>
  </body></html>`;

  await expect(result.current.print(plannedHtml)).resolves.toEqual({
    status: "complete",
  });

  const parsed = new DOMParser().parseFromString(child.written, "text/html");
  expect(parsed.title).toBe("Casa Verde — Menu");
  expect(parsed.documentElement.lang).toBe("es");
  expect(parsed.body.className).toBe("menu-document");
  expect(parsed.body.dataset.family).toBe("atelier");
  expect(parsed.querySelector("section")?.dataset.page).toBe("1");
  expect(parsed.querySelector("section")?.getAttribute("aria-label")).toBe(
    "Menu page",
  );
  expect(parsed.querySelector("style")?.textContent).toContain(
    "/fonts/dm-sans/menu.woff2",
  );
  expect(
    Array.from(parsed.querySelectorAll("img"), (image) =>
      image.getAttribute("src"),
    ),
  ).toEqual([
    "https://images.example/dish.jpg",
    "/images/logo.png",
    "data:image/png;base64,iVBORw0KGgo=",
  ]);
  expect(parsed.querySelector("div")?.getAttribute("style")).toContain(
    "left: 10mm",
  );
});

it("removes escaped, commented, remote, protocol-relative, and data CSS vectors", async () => {
  const child = createMockPrintWindow({ completion: "afterprint-sync" });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result } = renderHook(() => useMenuPrintWindow());
  const unsafeRules = [
    String.raw`@\69mport url("/fonts/print/dm-sans.woff2");`,
    String.raw`@\\69mport url("/fonts/print/dm-sans.woff2");`,
    String.raw`.escaped-js { background: url(j\61vascript:alert(1)); }`,
    String.raw`.commented-js { background: u/**/rl(jav/**/ascript:alert(2)); }`,
    `.remote { src: url("https://attacker.example/font.woff2"); }`,
    `.protocol-relative { src: url("//attacker.example/font.woff2"); }`,
    `.same-origin-image { background: url("${window.location.origin}/images/pixel.png"); }`,
    `.root-relative-image { background: url("/images/pixel.png"); }`,
    String.raw`.data-html { background: url(d\61ta:text/html;base64,PHNjcmlwdD4=); }`,
  ];
  const unsafeHtml = `<!doctype html><html><head>
    ${unsafeRules.map((rule) => `<style>${rule}</style>`).join("\n")}
  </head><body>
    <div class="remote-inline" style="background: url('https://attacker.example/pixel.png')">Menu</div>
  </body></html>`;

  await expect(result.current.print(unsafeHtml)).resolves.toEqual({
    status: "complete",
  });

  const parsed = new DOMParser().parseFromString(child.written, "text/html");
  expect(parsed.querySelectorAll("style")).toHaveLength(0);
  expect(parsed.querySelector(".remote-inline")?.hasAttribute("style")).toBe(
    false,
  );
  expect(child.written).not.toContain("attacker.example");
  expect(child.written).not.toContain("PHNjcmlwdD4=");
});

it.each([
  ["same-origin", window.location.origin],
  ["root-relative", ""],
] as const)(
  "preserves the real planned renderer stylesheet with %s font assets",
  async (_urlKind, origin) => {
    const child = createMockPrintWindow({ completion: "afterprint-sync" });
    jest.spyOn(window, "open").mockReturnValue(child.window);
    jest.spyOn(window, "focus").mockImplementation(() => {});
    const { result } = renderHook(() => useMenuPrintWindow());
    const plannedHtml = renderPlannedMenuHtml(
      sparseRestaurantMenu,
      planFixtureDocument({
        model: sparseRestaurantMenu,
        familyId: "atelier",
      }),
      { origin, qrCaption: "Scan for the live menu" },
    );
    const sourceCss = new DOMParser()
      .parseFromString(plannedHtml, "text/html")
      .querySelector("style")?.textContent;

    await expect(result.current.print(plannedHtml)).resolves.toEqual({
      status: "complete",
    });

    const printedCss = new DOMParser()
      .parseFromString(child.written, "text/html")
      .querySelector("style")?.textContent;
    expect(printedCss).toBe(sourceCss);
    expect(printedCss).toContain("@font-face");
    expect(printedCss).toContain(`src: url("${origin}/fonts/`);
    expect(printedCss).toContain(".print-page");
  },
);

it("does not mutate the parent DOM or a preview iframe", async () => {
  const preview = document.createElement("iframe");
  preview.dataset.testid = "menu-preview";
  document.body.appendChild(preview);
  const child = createMockPrintWindow({ completion: "afterprint-sync" });
  jest.spyOn(window, "open").mockReturnValue(child.window);
  jest.spyOn(window, "focus").mockImplementation(() => {});
  const { result, unmount } = renderHook(() => useMenuPrintWindow());
  const initialBody = document.body.innerHTML;

  await result.current.print(HTML);
  expect(document.body.innerHTML).toBe(initialBody);

  unmount();
  preview.remove();
});
