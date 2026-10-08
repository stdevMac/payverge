import { getPublicConfig } from "@/config/publicConfig";
import { useCallback, useEffect, useRef, useState } from "react";
import { refreshAuthSession } from "@/api/tools/instance";

export interface SSEEvent {
  type: string;
  data: Record<string, unknown>;
}

const namedSSEEventTypes = [
  "connected",
  "order.created",
  "order.updated",
  "order.cancelled",
  "print.bill_available",
  "print.receipt_available",
  "print.kitchen_available",
  "bill.created",
  "bill.updated",
  "bill.closed",
  "bill.stuck",
  "payment.received",
  // Guest initiated a payment (request/crypto flow); financial:read. Surfaces a
  // subtle "awaiting confirmation" badge on the bill row — never a toast/sound.
  "payment.pending",
  // An operator cancelled or rejected a pending request; clears the pending
  // badge without ever triggering received-payment celebration.
  "payment.request.resolved",
  "reservation.new",
  "reservation.updated",
  "delivery.created",
  "delivery.updated",
  "delivery.cancelled",
  "alert.created",
  "alert.updated",
  "alert.claimed",
  "alert.resolved",
  "alert.dismissed",
  "alert.reopened",
  // Staff scheduling (Slice 2). Mirrors backend sse_permissions.go (schedule:read).
  "schedule.published",
  "shift.assigned",
  "shift.updated",
  // Availability & time-off (Slice 3). timeoff.requested → managers (schedule:approve);
  // timeoff.decided → the requester (schedule:self). See backend sse_permissions.go.
  "timeoff.requested",
  "timeoff.decided",
  // Coverage — open shifts & swaps (Slice 5). claimed/requested/accepted → managers
  // (schedule:approve); decided → the requester/claimant (schedule:self). Mirrors
  // backend sse_permissions.go; feeds CoverageBoard + the operator ApprovalsPanel.
  "openshift.claimed",
  "openshift.decided",
  "shift.swap.requested",
  "shift.swap.accepted",
  "shift.swap.decided",
  // Staff chat & announcements (Slice 7). Content-free nudges (ids only — never
  // the message text or a DM body): chat:read is universal across staff roles and
  // the hub fans per-business, so a body would leak DMs. chat.message /
  // chat.announcement → chat:read holders. Mirrors backend sse_permissions.go.
  "chat.message",
  "chat.announcement",
  // Staff "Time & Team" Phase 1: self-targeted inbox nudge (per-staff notification).
  // Fired by notifyStaff chokepoint; consumers filter on their own staff_id.
  "notification.new",
  // Spaces phone-scan session progress (tables:read). Desktop modal polls as
  // fallback; SSE is preferred for live waiting_for_phone → review_ready.
  "space.scan.updated",
  // Restart-epoch control frame (SSE-EPOCH): the backend rebuilt its in-memory
  // replay ring (deploy/restart) or the ring can no longer serve our resume
  // point, so our Last-Event-ID is stale. Discard it and full-heal via the
  // onReconnect refetch path. Not a domain event — never dispatched to consumers.
  "sync.reset",
  "ping",
] as const;

const namedSSEEventTypeSet = new Set<string>(namedSSEEventTypes);

function normalizeSSEData(data: unknown): Record<string, unknown> {
  if (data && typeof data === "object" && !Array.isArray(data)) {
    return data as Record<string, unknown>;
  }
  return { value: data };
}

// Number of consecutive reconnect failures we tolerate silently before
// surfacing the degraded ("Real-time updates disconnected") banner. We keep
// retrying forever regardless — this only gates the UI hint.
const GRACE_RETRIES = 4;
// Upper bound on the backoff between reconnect attempts. The stream "should
// always be active", so we never give up — we just stop escalating the delay.
const MAX_BACKOFF_MS = 30_000;
// Consecutive reconnect failures before we suspect a stale session cookie and
// proactively refresh it. A bare HTTP 401 (expired 15-min session_token) is
// indistinguishable from a network drop at the EventSource layer — both fire
// `onerror` with no readable body — so the only signal we have is "reconnects
// keep failing". Refreshing through the shared singleton and reconnecting with
// the fresh cookie is the only thing that can break the loop; retrying the dead
// cookie forever just latches the disconnected banner.
const REFRESH_AFTER_RETRIES = 2;

interface SSESubscriber {
  onEvent: (event: SSEEvent) => void;
  onConnectionChange?: (connected: boolean) => void;
  onReconnect?: () => void;
  onDegradedChange?: (degraded: boolean) => void;
  onBlockedChange?: (blocked: boolean, reason?: string | null) => void;
}

/**
 * A single EventSource shared by every consumer of the same business stream.
 * The dashboard page, the Kitchen board, and the dispatch console all read the
 * same `/events` channel; without sharing they each opened their own connection
 * (2–3 per tab), multiplying server load and burning the edge rate-limit budget
 * — which made reconnect-storm failures (and the disconnected banner) far more
 * likely. One connection fans every event out to all subscribers instead.
 *
 * Reconnection retries indefinitely with capped, jittered exponential backoff,
 * so a transient outage (deploy, proxy blip, brief upstream 5xx) always
 * self-heals rather than latching a permanent "disconnected" state.
 */
class SharedSSEConnection {
  private es: EventSource | null = null;
  private readonly subscribers = new Set<SSESubscriber>();
  private retries = 0;
  private wasDisconnected = false;
  private degraded = false;
  // Latched when the server sends a terminal `error` event (a permanent gate
  // denial: business suspended or closed, or access revoked). Unlike `degraded`
  // (transient, auto-clears on reconnect), `blocked` stops reconnection
  // entirely — retrying a 403-equivalent only hammers the edge rate-limit.
  private blocked = false;
  private blockedReason: string | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private closed = false;
  // Guards against firing more than one in-flight /auth/refresh from this
  // connection while reconnects are failing. (The shared singleton dedupes
  // globally too, but this avoids redundant scheduling churn.)
  private refreshing = false;
  // Highest event id seen on this stream. Sent as last_event_id on reconnect so
  // the backend replays ONLY events missed during the drop — recovering lost
  // orders/payments while avoiding a full-buffer replay storm (duplicate sounds
  // and notifications). Empty on first connect (initial hydrate covers that).
  private lastEventId = "";

  constructor(private readonly url: string) {
    // Reconnect promptly when the tab/network wakes. Reconnection is otherwise
    // driven only by a setTimeout backoff, which browsers throttle (background
    // tabs) or freeze (sleep) — so without these the operator can sit on a
    // stale "disconnected" banner long after connectivity actually returns.
    if (typeof window !== "undefined") {
      window.addEventListener("online", this.wake);
      window.addEventListener("focus", this.wake);
    }
    if (typeof document !== "undefined") {
      document.addEventListener(
        "visibilitychange",
        this.handleVisibilityChange,
      );
    }
  }

  get subscriberCount(): number {
    return this.subscribers.size;
  }

  subscribe(sub: SSESubscriber): void {
    this.subscribers.add(sub);
    // Sync current degraded state into the freshly-mounted consumer.
    sub.onDegradedChange?.(this.degraded);
    sub.onBlockedChange?.(this.blocked, this.blockedReason);
    if (!this.es && !this.reconnectTimer) {
      this.connect();
    }
  }

  unsubscribe(sub: SSESubscriber): void {
    this.subscribers.delete(sub);
  }

  /** Resets backoff and reconnects immediately (manual "Reconnect" action). */
  reconnectNow(): void {
    this.retries = 0;
    this.closed = false;
    this.setBlocked(false, null);
    this.setDegraded(false);
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.es?.close();
    this.es = null;
    this.connect();
  }

  close(): void {
    this.closed = true;
    if (typeof window !== "undefined") {
      window.removeEventListener("online", this.wake);
      window.removeEventListener("focus", this.wake);
    }
    if (typeof document !== "undefined") {
      document.removeEventListener(
        "visibilitychange",
        this.handleVisibilityChange,
      );
    }
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.es?.close();
    this.es = null;
  }

  /**
   * Force an immediate reconnect when the tab/network wakes — but only if the
   * stream is actually down (no live EventSource, a pending backoff retry, or a
   * surfaced degraded state). A healthy connection is left untouched so wake
   * events don't needlessly churn it. Bound as a field so it can be added/
   * removed as an event listener. A terminally `blocked` stream is never woken.
   */
  private wake = (): void => {
    if (this.closed || this.blocked) return;
    const isDown = !this.es || this.degraded || this.reconnectTimer !== null;
    if (!isDown) return;
    this.reconnectNow();
  };

  private handleVisibilityChange = (): void => {
    if (
      typeof document !== "undefined" &&
      document.visibilityState !== "visible"
    ) {
      return;
    }
    this.wake();
  };

  /**
   * A stale 15-min session cookie 401s the reconnect, which looks identical to
   * a transport drop here. After sustained failures, refresh the session
   * through the SHARED singleton (so concurrent callers dedupe a single
   * /auth/refresh) and, on success, abandon the backoff wait and reconnect
   * immediately with the fresh cookie. A genuine network/refresh outage just
   * resolves false and the already-scheduled backoff retry proceeds unchanged.
   */
  private attemptAuthRefresh(): void {
    if (this.refreshing) return;
    this.refreshing = true;
    void refreshAuthSession(getPublicConfig().apiUrl)
      .then((refreshed) => {
        this.refreshing = false;
        if (!refreshed || this.closed || this.blocked) return;
        this.retries = 0;
        if (this.reconnectTimer) {
          clearTimeout(this.reconnectTimer);
          this.reconnectTimer = null;
        }
        this.es?.close();
        this.es = null;
        this.connect();
      })
      .catch(() => {
        this.refreshing = false;
      });
  }

  private setDegraded(value: boolean): void {
    if (this.degraded === value) return;
    this.degraded = value;
    for (const sub of this.subscribers) sub.onDegradedChange?.(value);
  }

  private setBlocked(value: boolean, reason?: string | null): void {
    const nextReason = value ? (reason ?? this.blockedReason) : null;
    if (this.blocked === value && this.blockedReason === nextReason) return;
    this.blocked = value;
    this.blockedReason = nextReason;
    for (const sub of this.subscribers) sub.onBlockedChange?.(value, nextReason);
  }

  private notifyConnection(connected: boolean): void {
    for (const sub of this.subscribers) sub.onConnectionChange?.(connected);
  }

  private dispatch(event: SSEEvent): void {
    for (const sub of this.subscribers) sub.onEvent(event);
  }

  private connect(): void {
    if (this.closed) return;

    // On reconnect, resume from the last event we saw so the server replays
    // only what we missed (bounded), not its entire ring buffer.
    let url = this.url;
    if (this.lastEventId) {
      url +=
        (this.url.includes("?") ? "&" : "?") +
        `last_event_id=${encodeURIComponent(this.lastEventId)}`;
    }

    const es = new EventSource(url, { withCredentials: true });
    this.es = es;

    for (const type of namedSSEEventTypes) {
      es.addEventListener(type, (e: MessageEvent) => {
        try {
          if (type === "sync.reset") {
            // Our resume point is unrecoverable (server ring rebuilt or evicted).
            // Drop the stale id so the next reconnect starts fresh, and fire the
            // full-heal path so consumers refetch domain state for the gap.
            this.lastEventId = "";
            for (const sub of this.subscribers) sub.onReconnect?.();
            return;
          }
          if (e.lastEventId) this.lastEventId = e.lastEventId;
          const data = JSON.parse(e.data);
          if (type === "connected") {
            const isRecoveryFromDrop = this.wasDisconnected;
            this.retries = 0;
            this.wasDisconnected = false;
            this.setDegraded(false);
            this.notifyConnection(true);
            // Only resync on recovery, not the first connect — the initial
            // hydrate is already covered by each consumer's on-mount fetch.
            if (isRecoveryFromDrop) {
              for (const sub of this.subscribers) sub.onReconnect?.();
            }
          }
          this.dispatch({ type, data });
        } catch {
          // Ignore parse errors for malformed events
        }
      });
    }

    // Terminal gate-denial frame. The server returns a 200 SSE stream with a
    // single `error` event instead of an EventSource-unreadable 403, so we can
    // distinguish a permanent denial (stop retrying) from a transient drop.
    es.addEventListener("error", (e: MessageEvent) => {
      try {
        const payload = JSON.parse(e.data) as { code?: unknown };
        if (typeof payload?.code !== "string") return;
        this.setBlocked(true, payload.code);
        this.setDegraded(false);
        this.notifyConnection(false);
        // Fully tear down: detach wake listeners, clear timer, close socket,
        // and evict this entry from the module-level connection map so that
        // wake events (online/focus/visibilitychange) no longer reference a
        // permanently-blocked connection. (SSE-LEAK-2)
        this.close();
        connections.delete(this.url);
      } catch {
        // A bodyless transport error fires native EventSource onerror with no
        // parseable data — that path is handled by es.onerror (keep retrying).
      }
    });

    es.onmessage = (e: MessageEvent) => {
      try {
        if (e.lastEventId) this.lastEventId = e.lastEventId;
        const envelope = JSON.parse(e.data) as {
          type?: unknown;
          data?: unknown;
        };
        if (
          !envelope ||
          typeof envelope.type !== "string" ||
          namedSSEEventTypeSet.has(envelope.type)
        ) {
          return;
        }

        this.dispatch({
          type: envelope.type,
          data: normalizeSSEData(envelope.data),
        });
      } catch {
        // Ignore parse errors for malformed generic events
      }
    };

    es.onerror = () => {
      this.wasDisconnected = true;
      this.notifyConnection(false);
      es.close();
      if (this.es === es) this.es = null;

      if (this.closed) return;

      this.retries += 1;
      if (this.retries >= GRACE_RETRIES) {
        this.setDegraded(true);
      }

      // Capped exponential backoff with full jitter. The cap keeps a long
      // outage from stretching reconnect attempts minutes apart; the jitter
      // spreads reconnects out so many clients/streams don't stampede the
      // backend in lockstep after a shared outage (e.g. a deploy).
      const ceiling = Math.min(
        1000 * Math.pow(2, this.retries - 1),
        MAX_BACKOFF_MS,
      );
      const delay = ceiling / 2 + Math.random() * (ceiling / 2);
      this.reconnectTimer = setTimeout(() => {
        this.reconnectTimer = null;
        this.connect();
      }, delay);

      // Sustained failures may be a stale session cookie 401'ing the reconnect
      // rather than a network drop. Refresh and, on success, reconnect at once
      // (cancelling the backoff above) — otherwise the backoff retry stands.
      if (this.retries >= REFRESH_AFTER_RETRIES) {
        this.attemptAuthRefresh();
      }
    };
  }
}

// Active connections keyed by stream URL (one per business). Module-scoped so
// every hook instance across the tab shares the same connection objects.
const connections = new Map<string, SharedSSEConnection>();

function acquireConnection(url: string): SharedSSEConnection {
  let conn = connections.get(url);
  if (!conn) {
    conn = new SharedSSEConnection(url);
    connections.set(url, conn);
  }
  return conn;
}

function releaseConnection(url: string): void {
  const conn = connections.get(url);
  if (conn && conn.subscriberCount === 0) {
    conn.close();
    connections.delete(url);
  }
}

interface UseSSEEventsOptions {
  businessId: number;
  enabled: boolean;
  onEvent: (event: SSEEvent) => void;
  onConnectionChange?: (connected: boolean) => void;
  /**
   * IMP-01: called when the SSE connection is re-established AFTER a drop.
   * Not fired on the initial hydrate. Consumers should re-fetch their domain
   * state here (open bills, kitchen tickets, etc.) — events that fired
   * during the outage were not buffered server-side and are otherwise lost.
   */
  onReconnect?: () => void;
}

export interface UseSSEEventsResult {
  /**
   * True when reconnect attempts have failed past the grace window and the
   * stream is currently down. Auto-clears the moment the connection recovers —
   * it is never a permanent terminal state (the hook always keeps retrying).
   */
  degraded: boolean;
  /**
   * True when the server issued a terminal `error` frame (permanent gate
   * denial: business suspended or closed, or access revoked). Unlike `degraded`,
   * reconnection has STOPPED — surface an upgrade/permission prompt, not a
   * "reconnecting" hint. Cleared by calling `reconnect()`.
   */
  blocked: boolean;
  /**
   * Terminal SSE error code when `blocked` is true (`business_suspended`,
   * `access_denied`, `session_revoked`). Null when the stream is not blocked.
   */
  blockedReason: string | null;
  /** Resets the backoff and re-establishes the SSE connection immediately. */
  reconnect: () => void;
}

/**
 * Subscribes to the shared business SSE stream. Multiple consumers on the same
 * business reuse one underlying EventSource. Fires onEvent for each event,
 * onReconnect after a recovered drop, and reports degraded state for UI hints.
 */
export function useSSEEvents({
  businessId,
  enabled,
  onEvent,
  onConnectionChange,
  onReconnect,
}: UseSSEEventsOptions): UseSSEEventsResult {
  const onEventRef = useRef(onEvent);
  const onConnectionChangeRef = useRef(onConnectionChange);
  const onReconnectRef = useRef(onReconnect);
  const connRef = useRef<SharedSSEConnection | null>(null);
  const [degraded, setDegraded] = useState(false);
  const [blocked, setBlocked] = useState(false);
  const [blockedReason, setBlockedReason] = useState<string | null>(null);

  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);

  useEffect(() => {
    onConnectionChangeRef.current = onConnectionChange;
  }, [onConnectionChange]);

  useEffect(() => {
    onReconnectRef.current = onReconnect;
  }, [onReconnect]);

  useEffect(() => {
    if (!enabled || !businessId) {
      setDegraded(false);
      setBlocked(false);
      setBlockedReason(null);
      return;
    }

    const url = `${getPublicConfig().apiUrl}/inside/businesses/${businessId}/events`;
    const conn = acquireConnection(url);
    connRef.current = conn;

    const sub: SSESubscriber = {
      onEvent: (event) => onEventRef.current(event),
      onConnectionChange: (connected) =>
        onConnectionChangeRef.current?.(connected),
      onReconnect: () => onReconnectRef.current?.(),
      onDegradedChange: (value) => setDegraded(value),
      onBlockedChange: (value, reason) => {
        setBlocked(value);
        setBlockedReason(value ? reason ?? null : null);
      },
    };
    conn.subscribe(sub);

    return () => {
      conn.unsubscribe(sub);
      releaseConnection(url);
      connRef.current = null;
    };
  }, [businessId, enabled]);

  const reconnect = useCallback(() => {
    connRef.current?.reconnectNow();
  }, []);

  return { degraded, blocked, blockedReason, reconnect };
}
