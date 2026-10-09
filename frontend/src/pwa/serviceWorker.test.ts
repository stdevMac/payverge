import fs from "node:fs";
import path from "node:path";
import vm from "node:vm";

type CacheRecord = Map<string, Response>;

function requestKey(request: { url: string } | string) {
  return typeof request === "string" ? new URL(request, "https://payverge.io").href : request.url;
}

function createWorkerHarness(options: { addAll?: jest.Mock } = {}) {
  const listeners = new Map<string, (event: any) => void>();
  const records = new Map<string, CacheRecord>();
  const fetch = jest.fn().mockResolvedValue(new Response("network"));
  const showNotification = jest.fn().mockResolvedValue(undefined);
  const claim = jest.fn().mockResolvedValue(undefined);
  const openWindow = jest.fn().mockResolvedValue(undefined);
  const skipWaiting = jest.fn().mockResolvedValue(undefined);
  const addAll = options.addAll ?? jest.fn(async (urls: string[], record: CacheRecord) => {
    urls.forEach((url) => record.set(requestKey(url), new Response(`cached:${url}`)));
  });

  const caches = {
    async open(name: string) {
      if (!records.has(name)) records.set(name, new Map());
      const record = records.get(name)!;
      return {
        addAll: async (urls: string[]) => addAll(urls, record),
        match: async (request: Request | string) => record.get(requestKey(request)),
        put: async (request: Request | string, response: Response) => {
          record.set(requestKey(request), response);
        },
      };
    },
    async keys() {
      return [...records.keys()];
    },
    async delete(name: string) {
      return records.delete(name);
    },
    async match(request: Request | string) {
      const key = requestKey(request);
      for (const record of records.values()) {
        const response = record.get(key);
        if (response) return response;
      }
      return undefined;
    },
  };

  const context = vm.createContext({
    URL,
    Request,
    Response,
    caches,
    fetch,
    self: {
      location: { origin: "https://payverge.io" },
      addEventListener: (type: string, listener: (event: any) => void) => listeners.set(type, listener),
      skipWaiting,
      clients: { claim, openWindow },
      registration: { showNotification },
    },
    clients: { openWindow },
  });
  const source = fs.readFileSync(path.join(process.cwd(), "public/sw.js"), "utf8");
  vm.runInContext(source, context, { filename: "sw.js" });

  async function dispatch(type: string, event: Record<string, unknown> = {}) {
    const work: Promise<unknown>[] = [];
    const eventWithLifecycle = {
      ...event,
      waitUntil: (promise: Promise<unknown>) => work.push(Promise.resolve(promise)),
    };
    listeners.get(type)?.(eventWithLifecycle);
    await Promise.all(work);
    return eventWithLifecycle;
  }

  async function dispatchFetch(request: { method: string; mode: string; url: string }) {
    let response: Promise<Response> | undefined;
    const event = await dispatch("fetch", {
      request,
      respondWith: (promise: Promise<Response>) => {
        response = Promise.resolve(promise);
      },
    });
    return { event, response };
  }

  return { addAll, caches, claim, dispatch, dispatchFetch, fetch, openWindow, records, showNotification, skipWaiting };
}

describe("PWA service worker", () => {
  it("does not intercept authenticated API GETs", async () => {
    const worker = createWorkerHarness();
    const { response } = await worker.dispatchFetch(
      { url: "https://payverge.io/api/v1/inside/businesses/1", method: "GET", mode: "cors" },
    );

    expect(response).toBeUndefined();
    expect(worker.fetch).not.toHaveBeenCalled();
  });

  it("precaches only static offline resources during install", async () => {
    const worker = createWorkerHarness();

    await worker.dispatch("install");

    expect(worker.skipWaiting).toHaveBeenCalledTimes(1);
    const offlineCache = worker.records.get("payverge-offline-v2");
    expect(offlineCache).toBeDefined();
    if (!offlineCache) return;
    expect([...offlineCache.keys()]).toEqual([
      "https://payverge.io/offline/en.html",
      "https://payverge.io/offline/es.html",
      "https://payverge.io/offline/es-ar.html",
      "https://payverge.io/android-chrome-192x192.png",
    ]);
  });

  it("does not skip waiting when offline precaching fails", async () => {
    const addAll = jest.fn().mockRejectedValue(new Error("precache failed"));
    const worker = createWorkerHarness({ addAll });

    await expect(worker.dispatch("install")).rejects.toThrow("precache failed");

    expect(addAll).toHaveBeenCalledTimes(1);
    expect(worker.skipWaiting).not.toHaveBeenCalled();
  });

  it("removes legacy Payverge caches but preserves unrelated caches on activation", async () => {
    const worker = createWorkerHarness();
    await worker.caches.open("payverge-api-v1");
    await worker.caches.open("payverge-static-v1");
    await worker.caches.open("payverge-offline-v2");
    await worker.caches.open("payverge-pwa-meta-v1");
    await worker.caches.open("third-party-cache");

    await worker.dispatch("activate");

    expect(await worker.caches.keys()).toEqual([
      "payverge-offline-v2",
      "payverge-pwa-meta-v1",
      "third-party-cache",
    ]);
    expect(worker.claim).toHaveBeenCalledTimes(1);
  });

  it("uses the persisted es-AR offline page when navigation fails", async () => {
    const worker = createWorkerHarness();
    await worker.dispatch("install");
    await worker.dispatch("message", { data: { type: "SET_OFFLINE_LOCALE", locale: "es-AR" } });
    worker.fetch.mockRejectedValueOnce(new Error("offline"));

    const { response } = await worker.dispatchFetch(
      { url: "https://payverge.io/app", method: "GET", mode: "navigate" },
    );

    expect(response).toBeDefined();
    if (!response) return;
    await expect((await response).text()).resolves.toBe("cached:/offline/es-ar.html");
  });

  it("does not cache successful dashboard navigation before serving a later static fallback", async () => {
    const worker = createWorkerHarness();
    const dashboardURL = "https://payverge.io/app/dashboard";
    await worker.dispatch("install");
    await worker.dispatch("message", { data: { type: "SET_OFFLINE_LOCALE", locale: "es-AR" } });
    worker.fetch.mockResolvedValueOnce(new Response("private dashboard"));

    const online = await worker.dispatchFetch({ url: dashboardURL, method: "GET", mode: "navigate" });
    expect(online.response).toBeDefined();
    if (!online.response) return;
    await expect((await online.response).text()).resolves.toBe("private dashboard");
    expect([...worker.records.values()].some((cache) => cache.has(dashboardURL))).toBe(false);

    worker.fetch.mockRejectedValueOnce(new Error("offline"));
    const offline = await worker.dispatchFetch({ url: dashboardURL, method: "GET", mode: "navigate" });
    expect(offline.response).toBeDefined();
    if (!offline.response) return;
    await expect((await offline.response).text()).resolves.toBe("cached:/offline/es-ar.html");
  });

  it("normalizes locale messages before persisting the offline fallback preference", async () => {
    const worker = createWorkerHarness();

    await worker.dispatch("message", { data: { type: "SET_OFFLINE_LOCALE", locale: "es-MX" } });
    const meta = await worker.caches.open("payverge-pwa-meta-v1");
    const locale = await meta.match("/__pwa_offline_locale__");

    expect(locale).toBeDefined();
    if (!locale) return;
    await expect(locale.text()).resolves.toBe("es");
  });

  it("continues to deliver push notifications with safe defaults", async () => {
    const worker = createWorkerHarness();

    await worker.dispatch("push", {
      data: { json: () => { throw new Error("invalid JSON"); } },
    });

    expect(worker.showNotification).toHaveBeenCalledWith("Payverge", {
      body: "",
      icon: "/android-chrome-192x192.png",
      badge: "/favicon-32x32.png",
      data: { url: "/app" },
    });
  });

  it("delivers a valid push payload unchanged", async () => {
    const worker = createWorkerHarness();

    await worker.dispatch("push", {
      data: { json: () => ({ title: "Order ready", body: "Table 12", url: "/app/orders/12" }) },
    });

    expect(worker.showNotification).toHaveBeenCalledWith("Order ready", {
      body: "Table 12",
      icon: "/android-chrome-192x192.png",
      badge: "/favicon-32x32.png",
      data: { url: "/app/orders/12" },
    });
  });

  it("opens the notification URL or app fallback after a click", async () => {
    const worker = createWorkerHarness();
    const close = jest.fn();

    await worker.dispatch("notificationclick", { notification: { close, data: {} } });

    expect(close).toHaveBeenCalledTimes(1);
    expect(worker.openWindow).toHaveBeenCalledWith("/app");
  });

  it("opens a valid relative notification URL after a click", async () => {
    const worker = createWorkerHarness();

    await worker.dispatch("notificationclick", { notification: { close: jest.fn(), data: { url: "/app/orders/12" } } });

    expect(worker.openWindow).toHaveBeenCalledWith("/app/orders/12");
  });
});
