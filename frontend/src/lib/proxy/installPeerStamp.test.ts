import http from "node:http";
import type { AddressInfo } from "node:net";

import { installPeerStamp, uninstallPeerStamp } from "./installPeerStamp";
import {
  PEER_STAMP_HEADER,
  formatPeerStamp,
  getPeerStampNonce,
  normalizePeerAddress,
  readPeerStamp,
} from "./peerStamp";

afterEach(() => uninstallPeerStamp());

/** The stamp header a real Node server sees for one request. */
async function stampSeenByServer(
  headers: Record<string, string> = {},
): Promise<string | null> {
  const server = http.createServer((req, res) => {
    res.end(JSON.stringify({ stamp: req.headers[PEER_STAMP_HEADER] ?? null }));
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address() as AddressInfo;
  try {
    const res = await fetch(`http://127.0.0.1:${port}/api/v1/me`, { headers });
    return ((await res.json()) as { stamp: string | null }).stamp;
  } finally {
    server.closeAllConnections?.();
    await new Promise<void>((resolve) => server.close(() => resolve()));
  }
}

const asHeaders = (stamp: string | null) =>
  new Headers(stamp === null ? {} : { [PEER_STAMP_HEADER]: stamp });

describe("installPeerStamp", () => {
  it("stamps the socket peer before any handler runs", async () => {
    installPeerStamp();
    const stamp = await stampSeenByServer();
    expect(readPeerStamp(asHeaders(stamp))).toEqual({
      address: "127.0.0.1",
      protocol: "http",
    });
  });

  it("overwrites a client-supplied stamp", async () => {
    const nonce = installPeerStamp();
    const stamp = await stampSeenByServer({
      [PEER_STAMP_HEADER]: formatPeerStamp(nonce, "https", "1.2.3.4"),
    });
    expect(readPeerStamp(asHeaders(stamp))).toEqual({
      address: "127.0.0.1",
      protocol: "http",
    });
  });

  it("is idempotent", () => {
    const first = installPeerStamp();
    expect(installPeerStamp()).toBe(first);
    expect(getPeerStampNonce()).toBe(first);
  });

  it("trusts nothing when not installed", async () => {
    const forged = formatPeerStamp("guess", "http", "1.2.3.4");
    const stamp = await stampSeenByServer({ [PEER_STAMP_HEADER]: forged });
    expect(stamp).toBe(forged);
    expect(readPeerStamp(asHeaders(stamp))).toBeNull();
  });
});

describe("readPeerStamp", () => {
  it("rejects a wrong nonce, protocol or address", () => {
    const nonce = installPeerStamp();
    const read = (value: string) => readPeerStamp(asHeaders(value));
    expect(read(formatPeerStamp("other", "http", "10.0.0.1"))).toBeNull();
    expect(read(`${nonce};ftp;10.0.0.1`)).toBeNull();
    expect(read(`${nonce};http;not an ip`)).toBeNull();
    expect(read(`${nonce};http;10.0.0.1;extra`)).toBeNull();
    expect(read(`${nonce};https;::ffff:10.0.0.1`)).toEqual({
      address: "10.0.0.1",
      protocol: "https",
    });
  });
});

describe("normalizePeerAddress", () => {
  it("reduces IPv4-mapped IPv6 and drops zones", () => {
    expect(normalizePeerAddress("::ffff:192.168.1.5")).toBe("192.168.1.5");
    expect(normalizePeerAddress("fe80::1%eth0")).toBe("fe80::1");
    expect(normalizePeerAddress("2001:db8::7")).toBe("2001:db8::7");
    expect(normalizePeerAddress("evil.example.test")).toBeNull();
    expect(normalizePeerAddress("")).toBeNull();
  });
});
