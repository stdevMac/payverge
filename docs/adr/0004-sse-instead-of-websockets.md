# 0004. Server-sent events, not WebSockets, for realtime

- Status: Accepted
- Date: 2026-10-03 (records a decision already in effect)

## Context

Kitchen screens, the floor view, guest split screens and AI chats need
updates without a reload. Almost all of that traffic flows from server to
client; clients act through ordinary REST calls that already carry
authentication, origin checks, rate limits and audit. A second, bidirectional
protocol would need its own copy of that pipeline, its own authorization per
message and its own resume logic, and must survive the same reverse proxies.

## Decision

Realtime uses **server-sent events** over plain HTTP. There is no WebSocket
endpoint; an earlier one was removed.

- An in-process hub per backend publishes typed events per business, with a
  sequence number, a short replay ring and a per-process epoch, so a client
  can resume with `Last-Event-ID` or is told to resync (`sync.reset`).
- Each subscriber only receives the event types its permissions allow, and
  staff streams re-check access every 15 seconds.
- Guest streams are keyed by unguessable capabilities and carry minimal data.
- AI answers stream back on the same `POST` that asked the question.
- Clients treat events as hints and re-read state over REST. Polling remains
  as a fallback.

## Consequences

- **Easier:** one auth and middleware path for everything. `EventSource`
  reconnects on its own. Works through ordinary HTTP proxies once response
  buffering is off.
- **Easier:** writes stay REST, so idempotency, validation and audit apply
  unchanged.
- **Harder:** the backend's HTTP server cannot use read or write timeouts,
  and each open dashboard or guest phone holds a connection.
- **Accepted limitation:** hubs are per process. Postgres `LISTEN/NOTIFY`
  fan-out across replicas is not implemented, so with several replicas
  clients rely on polling for events published elsewhere.

See [architecture/realtime.md](../architecture/realtime.md).
