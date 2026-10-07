/**
 * Node-only half of the peer stamp (see peerStamp.ts). Subscribes to the
 * `http.server.request.start` diagnostics channel, which Node publishes
 * synchronously before the server's 'request' event, so the stamp is on the
 * request before Next reads any header. Called once from src/instrumentation.ts.
 */
import { randomBytes } from "node:crypto";
import diagnosticsChannel from "node:diagnostics_channel";
import type { IncomingMessage } from "node:http";
import type { Socket } from "node:net";

import {
  PEER_STAMP_HEADER,
  formatPeerStamp,
  getPeerStampNonce,
  setPeerStampNonce,
} from "./peerStamp";

const CHANNEL = "http.server.request.start";

type RequestStart = { request?: IncomingMessage; socket?: Socket };

type Subscriber = (message: unknown) => void;

// On globalThis, like the nonce, so a second module instance (a separate
// bundle, a dev reload) finds the subscription instead of adding another.
const SUBSCRIBER_KEY = Symbol.for("payverge.peerStamp.subscriber");
type SubscriberGlobal = typeof globalThis & { [SUBSCRIBER_KEY]?: Subscriber };

/** Idempotent per process; returns the process nonce. */
export function installPeerStamp(): string {
  const g = globalThis as SubscriberGlobal;
  const existing = getPeerStampNonce();
  if (g[SUBSCRIBER_KEY] && existing) return existing;
  const stale = g[SUBSCRIBER_KEY];
  if (stale) diagnosticsChannel.unsubscribe(CHANNEL, stale);

  const nonce = randomBytes(18).toString("base64url");
  const onRequestStart = (message: unknown) => {
    const { request, socket } = (message ?? {}) as RequestStart;
    if (!request) return;
    const peer = socket ?? request.socket;
    const address = peer?.remoteAddress;
    if (!address) {
      // Never let a client-supplied stamp survive an unstamped request.
      delete request.headers[PEER_STAMP_HEADER];
      return;
    }
    const encrypted = Boolean((peer as Socket & { encrypted?: boolean }).encrypted);
    request.headers[PEER_STAMP_HEADER] = formatPeerStamp(
      nonce,
      encrypted ? "https" : "http",
      address,
    );
  };
  diagnosticsChannel.subscribe(CHANNEL, onRequestStart);
  g[SUBSCRIBER_KEY] = onRequestStart;
  setPeerStampNonce(nonce);
  return nonce;
}

/** Test helper: remove the subscription and forget the nonce. */
export function uninstallPeerStamp(): void {
  const g = globalThis as SubscriberGlobal;
  const subscriber = g[SUBSCRIBER_KEY];
  if (subscriber) diagnosticsChannel.unsubscribe(CHANNEL, subscriber);
  delete g[SUBSCRIBER_KEY];
  setPeerStampNonce(null);
}
