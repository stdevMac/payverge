# Code tour

Eight stops, about twenty minutes, for a reviewer who wants to know whether
this codebase can be trusted with money. Each stop names a file, what to
read in it, and what to notice. Search for the quoted names rather than
scrolling; several of these files are long.

For the bigger picture first, read
[architecture/overview.md](architecture/overview.md).

## 1. The composition root (3 minutes)

**Open** [backend/cmd/app/main.go](../backend/cmd/app/main.go).

- Search `ClassifyDatabase`: startup refuses a database it cannot verify,
  bootstraps an empty one from the genesis baseline, then applies numbered
  migrations and checks the result.
- Search `r := gin.New()`: the global middleware chain, in order.
- Search `protectedRoutes.POST("/businesses/:id/menu"`: a typical route,
  with its subscription check and RBAC permission (`menu:write`) inline.

**Notice:** every service, route and scheduler is wired here, explicitly. It
is long, but there is no hidden registration apart from payment plugins.

## 2. Who is calling (3 minutes)

**Open** [server/middleware.go](../backend/internal/server/middleware.go)
at `HybridAuthenticationMiddleware`, then
[server/rbac.go](../backend/internal/server/rbac.go) at `hasPermissions`.

**Notice:** a valid JWT is not enough. `validateSession` also checks a
server-side session row, so sign-out and revocation are immediate. When the
URL names a business, the caller must own it, work there or be a platform
admin. Permissions are plain strings such as `menu:write`, and the same
resolver decides which realtime events a staff member receives.

## 3. Money primitives (2 minutes)

**Open** [money/bill_totals.go](../backend/internal/money/bill_totals.go),
[money/minorunits.go](../backend/internal/money/minorunits.go) and
[database/models_json.go](../backend/internal/database/models_json.go).

**Notice:** amounts are `int64` cents. Tax is computed to a thousandth of a
percent and rounded once. Provider calls convert to each currency's real
minor unit (yen has none). JSON responses carry dollars, converted in one
place. See [ADR 0003](adr/0003-money-as-int64-cents.md).

## 4. A payment becomes "paid" exactly once (4 minutes)

**Open** [handlers/plugin_handlers.go](../backend/internal/handlers/plugin_handlers.go)
at `handlePaymentWebhook`, then
[database/business.go](../backend/internal/database/business.go) at
`ApplyConfirmedPayment`.

**Notice the order:** resolve the business, verify the provider's signature,
claim the event in an idempotency table, and only then touch money. Inside,
one transaction locks the bill row, refuses a paid, closed or voided bill
and any amount larger than what is still owed, records the payment under the
provider's transaction ID (so a replay changes nothing), and, when the bill
becomes paid, enqueues the fiscal receipt job in the same commit. A capture that cannot settle (wrong currency,
share already paid, bill closed) is refunded automatically
(`autoRefundUnsettleablePluginCapture`). See
[architecture/money-flow.md](architecture/money-flow.md).

## 5. Splitting a bill (2 minutes)

**Open** [splitting/service.go](../backend/internal/splitting/service.go) at
`allocateCentsEqual` and `allocateProportionalCents`, then
[database/bill_split.go](../backend/internal/database/bill_split.go).

**Notice:** shares are integer cents and always sum to the bill, with the
remainder on the last share. When guests pay their own share, the share is
*held* for five minutes first, so two phones cannot pay the same cents.

## 6. Realtime (2 minutes)

**Open** [events/hub.go](../backend/internal/events/hub.go) at `Publish`,
then [events/sse_handler.go](../backend/internal/events/sse_handler.go).

**Notice:** server-sent events, not WebSockets. Each event has a sequence
number and a process epoch, so a reconnecting client either replays what it
missed or is told to resync. A slow client loses events instead of blocking
the publisher. Topics are filtered by permission, and staff access is
re-checked every 15 seconds. See
[architecture/realtime.md](architecture/realtime.md).

## 7. Payment plugins (2 minutes)

**Open** [plugins/interface.go](../backend/internal/plugins/interface.go),
then [plugins/payment_contract.go](../backend/internal/plugins/payment_contract.go).

**Notice:** a provider implements the provider-specific calls only.
Settlement is shared (stop 4). The contract manifest lists what every
production provider must guarantee, a shared test runner checks it, and each
provider is off in production until its switch is set. See
[architecture/plugins.md](architecture/plugins.md).

## 8. The AI cannot invent a price (2 minutes)

**Open** [server/ai_waiter_v2.go](../backend/internal/server/ai_waiter_v2.go)
at `FinalizeWaiterV2`, then
[llm/call_budget.go](../backend/internal/llm/call_budget.go) at
`ReserveForRequest`.

**Notice:** the waiter's answer is rebuilt from the menu snapshot and from
cart calls the server has validated. Model prose never becomes a price, a
dish or an action. Every model call reserves its worst-case cost against
daily dollar ceilings in Postgres before it runs. With no model configured,
the product still works. See [ai/README.md](ai/README.md).

## Where next

| If you want to | Read |
|---|---|
| Run it | [deploy/README.md](../deploy/README.md) |
| Follow one request end to end | [architecture/request-flow.md](architecture/request-flow.md) |
| See why things are the way they are | [adr/](adr/README.md) |
| See how the AI is tested | [ai/evals.md](ai/evals.md), and the scenarios in [aicontract/testdata/scenarios.yaml](../backend/internal/aicontract/testdata/scenarios.yaml) |
| Find a file for a task | [ONBOARDING.md](ONBOARDING.md), [CODEMAPS/](CODEMAPS/) |
