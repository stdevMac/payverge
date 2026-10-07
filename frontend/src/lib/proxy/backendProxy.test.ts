import http from "node:http";
import type { AddressInfo } from "node:net";
import zlib from "node:zlib";

import {
  backendTarget,
  createProxyHandlers,
  proxyToBackend,
  upstreamUrlFor,
} from "./backendProxy";
import { PEER_STAMP_HEADER, formatPeerStamp, setPeerStampNonce } from "./peerStamp";

type Seen = {
  method: string;
  url: string;
  headers: http.IncomingHttpHeaders;
  body: string;
};

let server: http.Server;
let backendUrl: string;
let seen: Seen[] = [];
let releaseSecondEvent: (() => void) | null = null;

beforeAll(async () => {
  server = http.createServer((req, res) => {
    const chunks: Buffer[] = [];
    req.on("data", (chunk: Buffer) => chunks.push(chunk));
    req.on("end", () => {
      const body = Buffer.concat(chunks).toString("utf8");
      seen.push({
        method: req.method ?? "",
        url: req.url ?? "",
        headers: req.headers,
        body,
      });
      const path = (req.url ?? "").split("?")[0];

      if (path === "/api/v1/events") {
        res.writeHead(200, {
          "content-type": "text/event-stream",
          "cache-control": "no-cache",
        });
        res.write("data: first\n\n");
        releaseSecondEvent = () => {
          res.write("data: second\n\n");
          res.end();
        };
        return;
      }
      if (path === "/api/v1/cookies") {
        res.setHeader("set-cookie", [
          "session=abc; Path=/; HttpOnly",
          "csrf=xyz; Path=/",
        ]);
        res.setHeader("connection", "x-internal-hop");
        res.setHeader("x-internal-hop", "drop-me");
        res.setHeader("keep-alive", "timeout=5");
        res.setHeader("x-end-to-end", "keep-me");
        res.end("ok");
        return;
      }
      if (path === "/api/v1/redirect-internal") {
        res.writeHead(302, {
          location: `${backendUrl}/api/v1/landing?x=1`,
        });
        res.end();
        return;
      }
      if (path === "/api/v1/redirect-external") {
        res.writeHead(302, { location: "https://accounts.example.test/auth" });
        res.end();
        return;
      }
      if (path === "/api/v1/gzip") {
        const gz = zlib.gzipSync(Buffer.from("compressed-payload"));
        res.writeHead(200, {
          "content-type": "text/plain",
          "content-encoding": "gzip",
          "content-length": String(gz.length),
        });
        res.end(gz);
        return;
      }
      if (path === "/api/v1/empty") {
        res.writeHead(204);
        res.end();
        return;
      }
      if (path === "/api/v1/hang") {
        return; // never answers
      }
      res.setHeader("content-type", "application/json");
      res.end(
        JSON.stringify({ method: req.method, url: req.url, body }),
      );
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address() as AddressInfo;
  backendUrl = `http://127.0.0.1:${port}`;
});

afterAll(async () => {
  server.closeAllConnections?.();
  await new Promise<void>((resolve) => server.close(() => resolve()));
});

beforeEach(() => {
  seen = [];
  releaseSecondEvent = null;
});

const api = (env?: Record<string, string>) =>
  createProxyHandlers({
    prefix: "/api/v1/",
    env: env ?? { BACKEND_INTERNAL_URL: backendUrl },
  });

function req(path: string, init: RequestInit & { duplex?: "half" } = {}) {
  return new Request(`https://pos.example.test${path}`, init);
}

describe("enablement", () => {
  it("is a 404 and never calls upstream when BACKEND_INTERNAL_URL is unset", async () => {
    const fetchImpl = jest.fn();
    const res = await proxyToBackend(req("/api/v1/menu"), {
      prefix: "/api/v1/",
      env: {},
      fetchImpl,
    });
    expect(res.status).toBe(404);
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it("refuses malformed or credentialed targets", () => {
    expect(backendTarget({ BACKEND_INTERNAL_URL: "backend:8080" })).toBeNull();
    expect(
      backendTarget({ BACKEND_INTERNAL_URL: "http://u:p@backend:8080" }),
    ).toBeNull();
    expect(backendTarget({ BACKEND_INTERNAL_URL: "ftp://backend" })).toBeNull();
    expect(
      backendTarget({ BACKEND_INTERNAL_URL: "http://backend:8080/" })?.origin,
    ).toBe("http://backend:8080");
  });
});

describe("methods and bodies", () => {
  it.each(["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"] as const)(
    "forwards %s with path, query and body",
    async (method) => {
      const handlers = api();
      const withBody = !["GET", "OPTIONS"].includes(method);
      const res = await handlers[method](
        req("/api/v1/orders/42?expand=items", {
          method,
          body: withBody ? JSON.stringify({ n: 1 }) : undefined,
          headers: withBody ? { "content-type": "application/json" } : {},
        }),
      );
      expect(res.status).toBe(200);
      const echoed = await res.json();
      expect(echoed.method).toBe(method);
      expect(echoed.url).toBe("/api/v1/orders/42?expand=items");
      expect(echoed.body).toBe(withBody ? '{"n":1}' : "");
    },
  );

  it("forwards HEAD without a body", async () => {
    const res = await api().HEAD(req("/api/v1/orders/1", { method: "HEAD" }));
    expect(res.status).toBe(200);
    expect(res.body).toBeNull();
    expect(seen[0].method).toBe("HEAD");
  });

  it("streams a chunked request body through", async () => {
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode("part-1,"));
        controller.enqueue(new TextEncoder().encode("part-2"));
        controller.close();
      },
    });
    const res = await api().POST(
      req("/api/v1/upload", { method: "POST", body, duplex: "half" }),
    );
    expect((await res.json()).body).toBe("part-1,part-2");
  });

  it("passes a 204 through with a null body", async () => {
    const res = await api().DELETE(req("/api/v1/empty", { method: "DELETE" }));
    expect(res.status).toBe(204);
    expect(res.body).toBeNull();
  });
});

describe("size limit", () => {
  it("rejects a declared Content-Length over the cap before calling upstream", async () => {
    const fetchImpl = jest.fn();
    const res = await proxyToBackend(
      req("/api/v1/upload", {
        method: "POST",
        body: "x".repeat(32),
        headers: { "content-length": "32" },
      }),
      {
        prefix: "/api/v1/",
        env: { BACKEND_INTERNAL_URL: backendUrl },
        maxBodyBytes: 16,
        fetchImpl,
      },
    );
    expect(res.status).toBe(413);
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it("rejects the default 15 MB cap by Content-Length", async () => {
    const fetchImpl = jest.fn();
    const res = await proxyToBackend(
      new Request("https://pos.example.test/api/v1/upload", {
        method: "POST",
        headers: { "content-length": String(15 * 1024 * 1024 + 1) },
        body: "tiny",
      }),
      { prefix: "/api/v1/", env: { BACKEND_INTERNAL_URL: backendUrl }, fetchImpl },
    );
    expect(res.status).toBe(413);
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it("aborts a chunked body once it exceeds the cap", async () => {
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        for (let i = 0; i < 4; i += 1) {
          controller.enqueue(new TextEncoder().encode("0123456789"));
        }
        controller.close();
      },
    });
    const res = await proxyToBackend(
      req("/api/v1/upload", { method: "POST", body, duplex: "half" }),
      {
        prefix: "/api/v1/",
        env: { BACKEND_INTERNAL_URL: backendUrl },
        maxBodyBytes: 16,
      },
    );
    expect(res.status).toBe(413);
  });
});

describe("headers", () => {
  it("forwards cookies, auth and X-Forwarded-*, strips hop-by-hop", async () => {
    await api().GET(
      req("/api/v1/me", {
        headers: {
          cookie: "session=abc",
          authorization: "Bearer t0k3n",
          "x-request-id": "req-123",
          connection: "keep-alive, x-secret-hop",
          "keep-alive": "timeout=5",
          "x-secret-hop": "must-not-forward",
          te: "trailers",
          "proxy-authorization": "Basic Zm9v",
          "x-middleware-subrequest": "middleware",
          "accept-encoding": "gzip, br",
        },
      }),
    );
    const h = seen[0].headers;
    expect(h.cookie).toBe("session=abc");
    expect(h.authorization).toBe("Bearer t0k3n");
    // Unstamped: the scheme is the Next server's own (http), never the
    // request URL's, which Next derives from a client X-Forwarded-Proto.
    expect(h["x-forwarded-proto"]).toBe("http");
    expect(h["x-forwarded-host"]).toBe("pos.example.test");
    expect(h["x-request-id"]).toBe("req-123");
    expect(h["x-secret-hop"]).toBeUndefined();
    expect(h["proxy-authorization"]).toBeUndefined();
    expect(h["keep-alive"]).toBeUndefined();
    expect(h.te).toBeUndefined();
    expect(h["x-middleware-subrequest"]).toBeUndefined();
    expect(h["accept-encoding"]).toBe("identity");
    expect(h.host).toBe(new URL(backendUrl).host);
  });

  it("ignores a client-supplied X-Forwarded-Proto/Host", async () => {
    await api().GET(
      new Request("http://pos.example.test/api/v1/me", {
        headers: {
          "x-forwarded-proto": "https",
          "x-forwarded-host": "orders.example.test",
        },
      }),
    );
    expect(seen[0].headers["x-forwarded-host"]).toBe("pos.example.test");
    expect(seen[0].headers["x-forwarded-proto"]).toBe("http");
  });

  it("mints an X-Request-Id when missing or unsafe", async () => {
    await api().GET(req("/api/v1/me"));
    await api().GET(
      req("/api/v1/me", { headers: { "x-request-id": "bad id\twith spaces" } }),
    );
    for (const entry of seen) {
      expect(entry.headers["x-request-id"]).toMatch(
        /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/,
      );
    }
  });

  it("keeps every Set-Cookie and strips hop-by-hop response headers", async () => {
    const res = await api().GET(req("/api/v1/cookies"));
    expect(res.headers.getSetCookie()).toEqual([
      "session=abc; Path=/; HttpOnly",
      "csrf=xyz; Path=/",
    ]);
    expect(res.headers.get("x-end-to-end")).toBe("keep-me");
    expect(res.headers.get("x-internal-hop")).toBeNull();
    expect(res.headers.get("keep-alive")).toBeNull();
    expect(res.headers.get("connection")).toBeNull();
  });

  it("hands on a decoded body without a stale Content-Encoding", async () => {
    const res = await api().GET(req("/api/v1/gzip"));
    expect(res.headers.get("content-encoding")).toBeNull();
    expect(await res.text()).toBe("compressed-payload");
  });
});

describe("redirects", () => {
  it("never follows and rewrites backend-origin Locations to relative", async () => {
    const res = await api().GET(req("/api/v1/redirect-internal"));
    expect(res.status).toBe(302);
    expect(res.headers.get("location")).toBe("/api/v1/landing?x=1");
    expect(seen).toHaveLength(1);
  });

  it("leaves third-party Locations untouched", async () => {
    const res = await api().GET(req("/api/v1/redirect-external"));
    expect(res.status).toBe(302);
    expect(res.headers.get("location")).toBe("https://accounts.example.test/auth");
  });
});

describe("streaming", () => {
  it("delivers SSE events as they arrive (no buffering)", async () => {
    const res = await api().GET(
      req("/api/v1/events", { headers: { accept: "text/event-stream" } }),
    );
    expect(res.headers.get("content-type")).toBe("text/event-stream");
    expect(res.headers.get("cache-control")).toBe("no-cache, no-transform");
    expect(res.headers.get("x-accel-buffering")).toBe("no");

    const reader = res.body!.getReader();
    const decoder = new TextDecoder();
    const first = await reader.read();
    // The backend has not sent the second event yet; the first one already
    // reached us, so nothing in between is buffering the stream.
    expect(decoder.decode(first.value)).toBe("data: first\n\n");
    expect(releaseSecondEvent).not.toBeNull();
    releaseSecondEvent!();
    let rest = "";
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      rest += decoder.decode(value);
    }
    expect(rest).toBe("data: second\n\n");
  });

  it("keeps SSE open past the request timeout", async () => {
    const res = await proxyToBackend(req("/api/v1/events"), {
      prefix: "/api/v1/",
      env: { BACKEND_INTERNAL_URL: backendUrl },
      timeoutMs: 30,
    });
    const reader = res.body!.getReader();
    await reader.read();
    await new Promise((resolve) => setTimeout(resolve, 80));
    releaseSecondEvent!();
    const next = await reader.read();
    expect(new TextDecoder().decode(next.value)).toBe("data: second\n\n");
    await reader.cancel();
  });
});

describe("failures", () => {
  it("answers 504 when the backend exceeds the timeout", async () => {
    const res = await proxyToBackend(req("/api/v1/hang"), {
      prefix: "/api/v1/",
      env: { BACKEND_INTERNAL_URL: backendUrl },
      timeoutMs: 50,
    });
    expect(res.status).toBe(504);
  });

  it("answers 502 when the backend is unreachable", async () => {
    const res = await proxyToBackend(req("/api/v1/menu"), {
      prefix: "/api/v1/",
      env: { BACKEND_INTERNAL_URL: "http://127.0.0.1:1" },
    });
    expect(res.status).toBe(502);
  });
});

describe("upstream URL safety", () => {
  const backend = new URL("http://backend:8080");

  it("keeps the backend origin and the route prefix", () => {
    expect(
      upstreamUrlFor(
        new URL("https://pos.example.test/api/v1/a/b?c=d"),
        backend,
        "/api/v1/",
      )?.href,
    ).toBe("http://backend:8080/api/v1/a/b?c=d");
    expect(
      upstreamUrlFor(new URL("https://pos.example.test/metrics"), backend, "/api/v1/"),
    ).toBeNull();
    expect(
      upstreamUrlFor(
        new URL("https://pos.example.test//evil.example/api/v1/x"),
        backend,
        "/api/v1/",
      ),
    ).toBeNull();
  });

  it("keeps percent-encoded segments encoded", () => {
    expect(
      upstreamUrlFor(
        new URL("https://pos.example.test/media/a%2Fb.png"),
        backend,
        "/media/",
      )?.pathname,
    ).toBe("/media/a%2Fb.png");
  });

  it("supports a backend mounted under a path", () => {
    expect(
      upstreamUrlFor(
        new URL("https://pos.example.test/api/v1/x"),
        new URL("http://backend:8080/core/"),
        "/api/v1/",
      )?.href,
    ).toBe("http://backend:8080/core/api/v1/x");
  });

  it("rejects disallowed methods with 405 when a route restricts them", async () => {
    const res = await proxyToBackend(req("/media/a.png", { method: "POST", body: "x" }), {
      prefix: "/media/",
      methods: ["GET", "HEAD"],
      env: { BACKEND_INTERNAL_URL: backendUrl },
    });
    expect(res.status).toBe(405);
    expect(res.headers.get("allow")).toBe("GET, HEAD");
  });
});

describe("client identity (X-Forwarded-For spoofing)", () => {
  const NONCE = "test-nonce-0123456789";
  const stamp = (address: string, protocol: "http" | "https" = "http") =>
    formatPeerStamp(NONCE, protocol, address);
  const spoofed = {
    "x-forwarded-for": "1.2.3.4",
    "x-real-ip": "1.2.3.5",
    forwarded: "for=1.2.3.6",
    "cf-connecting-ip": "1.2.3.7",
    "true-client-ip": "1.2.3.8",
    "x-client-ip": "1.2.3.9",
  };
  const identity = (h: http.IncomingHttpHeaders) => ({
    xff: h["x-forwarded-for"],
    realIp: h["x-real-ip"],
    forwarded: h.forwarded,
    cf: h["cf-connecting-ip"],
    trueClient: h["true-client-ip"],
    clientIp: h["x-client-ip"],
    stamp: h[PEER_STAMP_HEADER],
  });

  beforeEach(() => setPeerStampNonce(NONCE));
  afterEach(() => setPeerStampNonce(null));

  it("never forwards client-chosen addresses from an untrusted peer", async () => {
    await api().GET(
      req("/api/v1/auth/login", {
        headers: { ...spoofed, [PEER_STAMP_HEADER]: stamp("198.51.100.9") },
      }),
    );
    expect(identity(seen[0].headers)).toEqual({
      xff: "198.51.100.9",
      realIp: undefined,
      forwarded: undefined,
      cf: undefined,
      trueClient: undefined,
      clientIp: undefined,
      stamp: undefined,
    });
  });

  it("derives X-Forwarded-Proto/Host from the socket and Host, not the client", async () => {
    await api().GET(
      req("/api/v1/me", {
        headers: {
          host: "lan.example.test:3000",
          "x-forwarded-proto": "https",
          "x-forwarded-host": "evil.example.test",
          [PEER_STAMP_HEADER]: stamp("198.51.100.9", "http"),
        },
      }),
    );
    expect(seen[0].headers["x-forwarded-proto"]).toBe("http");
    expect(seen[0].headers["x-forwarded-host"]).toBe("lan.example.test:3000");
  });

  it("forwards no address at all without a valid stamp", async () => {
    await api().GET(req("/api/v1/me", { headers: spoofed }));
    await api().GET(
      req("/api/v1/me", {
        headers: {
          ...spoofed,
          [PEER_STAMP_HEADER]: formatPeerStamp("guessed", "http", "1.2.3.4"),
        },
      }),
    );
    setPeerStampNonce(null); // stamp not installed in this process
    await api().GET(
      req("/api/v1/me", {
        headers: { ...spoofed, [PEER_STAMP_HEADER]: stamp("1.2.3.4") },
      }),
    );
    expect(seen).toHaveLength(3);
    for (const entry of seen) {
      expect(identity(entry.headers)).toEqual({
        xff: undefined,
        realIp: undefined,
        forwarded: undefined,
        cf: undefined,
        trueClient: undefined,
        clientIp: undefined,
        stamp: undefined,
      });
    }
  });

  it("keeps a trusted edge's chain and appends the edge", async () => {
    const env = {
      BACKEND_INTERNAL_URL: backendUrl,
      FRONTEND_TRUSTED_PROXIES: "10.0.0.0/8, fd00::/8",
    };
    await api(env).GET(
      req("/api/v1/me", {
        headers: {
          "x-forwarded-for": "203.0.113.7",
          "x-real-ip": "203.0.113.7",
          "x-forwarded-proto": "https",
          "x-forwarded-host": "orders.example.test",
          [PEER_STAMP_HEADER]: stamp("::ffff:10.1.2.3"),
        },
      }),
    );
    // Next filled a missing X-Forwarded-For with the peer itself.
    await api(env).GET(
      req("/api/v1/me", {
        headers: {
          "x-forwarded-for": "fd00::5",
          [PEER_STAMP_HEADER]: stamp("fd00::5"),
        },
      }),
    );
    const [withChain, nextFilled] = seen.map((entry) => entry.headers);
    expect(withChain["x-forwarded-for"]).toBe("203.0.113.7, 10.1.2.3");
    expect(withChain["x-real-ip"]).toBe("203.0.113.7");
    expect(withChain["x-forwarded-proto"]).toBe("https");
    expect(withChain["x-forwarded-host"]).toBe("orders.example.test");
    expect(withChain[PEER_STAMP_HEADER]).toBeUndefined();
    expect(nextFilled["x-forwarded-for"]).toBe("fd00::5");
  });

  it("treats a peer outside FRONTEND_TRUSTED_PROXIES (or a /0 entry) as a client", async () => {
    for (const trusted of ["10.0.0.0/8", "0.0.0.0/0", "::/0"]) {
      await api({
        BACKEND_INTERNAL_URL: backendUrl,
        FRONTEND_TRUSTED_PROXIES: trusted,
      }).GET(
        req("/api/v1/me", {
          headers: { ...spoofed, [PEER_STAMP_HEADER]: stamp("192.168.1.50") },
        }),
      );
    }
    for (const entry of seen) {
      expect(entry.headers["x-forwarded-for"]).toBe("192.168.1.50");
      expect(entry.headers["x-real-ip"]).toBeUndefined();
    }
  });
});
