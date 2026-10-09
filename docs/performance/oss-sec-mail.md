# oss-sec-mail: outbound mail budget performance

> Point-in-time evidence. Migration numbers cited here (0002xx) predate the
> squash into `backend/schema/genesis/current_schema.sql`; that schema now
> lives in the genesis baseline, and numbered migrations restart at 000001.

Date: 2026-10-03
Branch: `oss/sec-mail` (base `e26f72b6d`)

This stream adds a per-tenant outbound email budget to the email dispatch
choke point (`emails.EmailServer.dispatch` → `claimTenantMail`). Tenant-
triggered sends now touch the database before they are queued or sent, so the
Backend Performance Gate applies to that path. No other hot path gained
queries; the remaining changes in the stream are listed under "Query shape" below.

## Query shape

A budgeted send is any send stamped with `ForBusiness`, `ForBill`,
`ForReservation`, `ForReservationRequest` or `ForReservationFollowUp`, and any
notification whose `MailOrigin` names a business (delivery mail through
`EmailServerDispatcher`, since FIX round 2). For such a send, one transaction
runs:

| Step | Statement | When |
| --- | --- | --- |
| Dedupe | 1 upsert into `auth_attempts` (`kind = tenant_mail_dedupe`) | dedupe window > 0 and the message has a content hash |
| Tier | 1 narrow `SELECT is_demo, kind FROM businesses WHERE id = ?` (the trial tier and its `subscription_status` read were removed with hosted billing) | always |
| Business day | 1 upsert | business cap > 0 (a cap of 0 refuses without writing; a negative cap skips the step) |
| Booking lane day | 1 upsert | guest-booking lane only (`ForReservationRequest`, a `ForReservationFollowUp` for a guest-made booking, or the public delivery checkout's order-received mail) |
| Recipient, cross-tenant all-purpose (`to:<hash>`, `tenant_mail_recipient_global_day`) | 1 upsert per recipient | always (follow-up F-1) |
| Recipient, cross-tenant lane (`to:<hash>`, `tenant_mail_lane_recipient_global_day`) | 1 upsert per recipient | guest-booking lane only |
| Recipient, per business | 1 upsert per recipient | always |
| Booking lane recipient | 1 upsert per recipient | guest-booking lane only |

Each counter is a single `INSERT … ON CONFLICT (principal, kind) DO UPDATE …
RETURNING count`. No row is read and then written back, and nothing is
preloaded. Rows are locked in a fixed order, so concurrent claims cannot
deadlock.

The "daily" caps are counted per 24-hour window, one window per counter. A
window opens with the counter's first counted send and resets 24 hours later.
It is not a UTC calendar day and not a sliding window
(`TestTenantMailBudget_WindowIsNotACalendarDay`).

The stream adds no table. The ledger reuses `auth_attempts`, whose unique
`(principal, kind)` index comes from migration 000091. A janitor deletes
expired rows every hour.

A typical single-recipient guest receipt or operator send costs 1 select and
3 upserts in one transaction (it was 4 before FIX round 1 made the
cross-tenant recipient key lane-only). A single-recipient guest-booking lane
send costs 1 select and 6 upserts.

Unstamped mail runs no queries and behaves exactly as before. This covers
system mail (password reset, verification, login codes), platform lifecycle
and subscription mail, admin sends, and the delivery order notices to the
venue's own contact address (new order, driver assigned, cancelled), which
wait on an owner decision. Since FIX round 2, the staff, wallet and
booking-approval notices to that address are stamped, and so is all delivery
mail to the guest (see below).

The other changes in this stream add no query on a hot path:

- **Reservation field caps:** CPU-only rune checks.
- **Guest-text scrub in the reservation senders:** fewer template keys.
- **Receipt display-string sanitizers:** CPU-only.
- **Legacy `GET /unsubscribe`:** one fewer query. The old `DELETE` is gone, and the route now runs no query.
- **Token `POST /email/unsubscribe`:** one extra `UPDATE subscribers SET deleted_at = …`. This is a rare, rate-limited public form, not a hot path.

## Benchmark

`BenchmarkDispatchTenantBudget` is in `backend/internal/emails/tenant_budget_test.go`.

It drives `ForBill(7, 1).SendCustomEmail` through the real `dispatch` against
a stub provider and in-memory SQLite, with a fresh recipient on every
iteration:

- **`without`:** the budget is not attached. This is the pre-change path, since `claimTenantMail` returns early.
- **`with`:** the real `GormTenantMailBudget`, with caps set to `1<<30`, on a receipt (`ForBill`) send. Every iteration performs the dedupe, tier, business and per-business recipient sequence and never refuses.
- **`with-booking-lane`** (added in FIX round 1): the same budget on a public booking-form (`ForReservationRequest`) send, which also takes the lane day, cross-tenant recipient and lane recipient keys.

Command (hermetic env prefix from the workstream brief):

```bash
cd backend
go test -run '^$' -bench BenchmarkDispatchTenantBudget -benchmem -count=3 ./internal/emails/
```

Results (Apple M3, darwin, Go 1.26):

| Variant | ns/op (3 runs) | B/op | allocs/op |
| --- | --- | ---: | ---: |
| without (before) | 750.9 / 758.5 / 788.7 | ~785 | 15 |
| with (after) | 110,462 / 110,022 / 108,985 | ~43,825 | 612 |

The delta is about 109 µs, 43 KB and 597 allocations per budgeted send on
SQLite.

FIX round 1 rerun (same command, same machine, 2026-10-03, after the
guest-booking lane split). The machine was shared with other workstreams at a
load average near 30 on 8 cores, so ns/op is inflated across the board: the
unchanged `without` baseline itself ran 2.5–6x slower than above. B/op and
allocs/op do not depend on load and are the comparable numbers.

| Variant | ns/op (3 runs, loaded host) | B/op | allocs/op |
| --- | --- | ---: | ---: |
| without | 1,945 / 3,570 / 4,522 | ~795 | 14–15 |
| with (receipt / operator) | 524,574 / 729,548 / 416,272 | ~35,170 | 496 |
| with-booking-lane | 583,895 / 528,978 / 270,069 | ~60,130 | 841 |

The receipt path dropped from 612 to 496 allocs/op and from about 43.8 KB to
35.2 KB per send, which matches the one upsert it no longer runs. The lane
path costs 7 statements instead of 4. It runs only on the rate-limited public
booking form and its follow-ups. A quiet-host ns/op rerun is still owed.

On Postgres a receipt or operator claim is one transaction of 4 statements
(5 before round 1), and a lane claim is 7. At typical in-cluster round-trip
latency that adds about 1–2 ms, or about 2–3 ms on the lane.

That cost sits in front of either an outbox insert or a provider HTTP call
(Resend/Postmark, usually 100+ ms), and only on tenant-triggered sends. It is
therefore not on any request path that has a latency budget tighter than the
send itself. The guest receipt endpoint is the one request-path caller. It
already does an idempotency/outbox write and is rate-limited per bill.

Follow-up F-1 (2026-10-04) puts back an all-purpose cross-tenant recipient key
(`EMAIL_TENANT_RECIPIENT_GLOBAL_ALL_DAILY_CAP`, default 50) on every tenant
send, so sybil businesses cannot each mail one victim up to the per-business
recipient cap through invites or receipts. The lane keeps its own, stricter
key (`EMAIL_TENANT_RECIPIENT_GLOBAL_DAILY_CAP`, default 10). Same command,
quiet host, before = oss/main 14ff9974c, after = oss/followups-be:

| Variant | before ns/op (3 runs) | before B/op, allocs | after ns/op (3 runs) | after B/op, allocs |
| --- | --- | --- | --- | --- |
| without | 773.8 / 773.4 / 786.1 | ~794, 15 | 1,006 / 1,030 / 1,547 | ~783, 15 |
| with (receipt / operator) | 87,587 / 88,127 / 87,001 | ~35,470, 497 | 112,258 / 112,823 / 112,039 | ~43,850, 612 |
| with-booking-lane | 158,265 / 158,082 / 158,808 | ~60,630, 844 | 182,463 / 185,143 / 203,835 | ~68,870, 958 |

The receipt/operator path is back to one select and 4 upserts (the cost it had
before FIX round 1 made the key lane-only), and the lane path is 1 select and
7 upserts. The extra upsert is the price of the sybil bound; the path still
sits in front of an outbox insert or a provider HTTP call.

Not done: the Postgres (Testcontainers) variant of this benchmark. Possible
follow-up: cache the business tier per process for a short TTL, which saves
the one select in every claim. The tier changes rarely, and it would need invalidation
on subscription change. This is listed in the workstream backlog.

FIX round 2 (2026-10-03) stamps four more call sites: staff added
(`ForBusiness`), staff removed (`ForBusiness`), wallet changed (`ForBusiness`)
and the approval reminder (`ForReservationFollowUp`). None of them is a hot
path. The first three follow a rare owner action. The reminder is sent by the
approval sweeper at most once per pending booking. Each runs a claim whose
shape is already measured above: the operator shape (`with`) for the three
owner-action notices and a staff-entered booking's reminder, and the lane
shape (`with-booking-lane`) for a guest-made booking's reminder. No benchmark
was added and no rerun was needed. The staff-added notice is now skipped when
the invite itself was refused for budget, which removes one claim from that
path.

FIX round 2 also stamps the delivery mail to the guest, which goes through
`EmailServerDispatcher.DispatchNotification` rather than a `For*` helper.
Before this, `ForDeliveryOrder` existed but had no caller, and every delivery
mail was unbudgeted. That included the order-received confirmation that the
public checkout (`POST /businesses/:business_id/delivery/orders`, 30/min per
IP) sends to whatever address the caller typed. Now:

- the order-received confirmation of a public checkout is lane mail
  (`MailPurposeGuestDeliveryRequest`), the lane shape;
- accepted, status change, expired, cancelled and payment received are
  operator mail stamped with the business and the delivery order, the
  operator shape;
- the venue notices to `business.Email` are unchanged (owner decision).

The public checkout is a guest route, so this was benchmarked.
`BenchmarkDispatchNotificationTenantBudget` is in
`backend/internal/emails/dispatcher_budget_test.go`. It drives
`DispatchNotification` with the real budget attached (caps `1<<30`), a stub
provider, SQLite, and production template caching (outside production mode
every render re-reads the template tree from disk, which costs about 13 ms and
10 MB per send and would hide the delta). `unstamped` is exactly what every
delivery notification did before, so it is the baseline.

```bash
cd backend
go test -run '^$' -bench BenchmarkDispatchNotificationTenantBudget -benchmem -count=3 ./internal/emails/
```

Results (Apple M3, 2026-10-03, load average about 6 on 8 cores, so ns/op is
somewhat noisy):

| Variant | ns/op (3 runs) | B/op | allocs/op |
| --- | --- | ---: | ---: |
| unstamped (before) | 24,605 / 26,154 / 26,967 | ~34,505 | 258 |
| delivery-operator (after) | 133,335 / 122,337 / 120,617 | ~73,306 | 742 |
| delivery-guest-lane (after) | 222,724 / 201,544 / 202,322 | ~98,353 | 1,088 |

The deltas (about +97 µs, +38.8 KB, +484 allocs for an operator send and
about +180 µs, +63.8 KB, +830 allocs for a lane send) match the receipt and
booking-lane deltas measured above, so the dispatcher adds nothing beyond the
claim itself. On Postgres the checkout gains one 7-statement transaction
(about 2–3 ms) in front of an outbox insert or a provider call that it
already made.

## Regression tests

Every test below passed with `go test -short -count=1` and the hermetic
environment prefix.

`internal/emails`, budget tests in `tenant_budget_test.go`:

- caps at and over each boundary: business, trial, demo, recipient, recipient-global, booking-form business and booking-form recipient;
- the guest-booking lane is capped at half the tier cap and cannot starve operator mail, guest-made follow-ups stay in the lane, and the cross-tenant recipient key is lane-only;
- an explicit `ReportingDuplicates()` origin gets `ErrTenantMailDuplicate` inside the dedupe window, and other sends still skip it silently;
- every `EMAIL_TENANT_*` key is forwarded by `docker-compose.yml` (the only compose file the open-source tree ships) and documented in `.env.example` with the code defaults;
- cap 0 blocks and a negative cap is unlimited;
- dedupe window;
- release on a provider failure, and an outbox duplicate as a silent no-op;
- unstamped system mail is never budgeted;
- a notification's `MailOrigin` is claimed by `EmailServerDispatcher`: the delivery checkout confirmation is lane mail and is refused past the lane recipient cap, operator delivery mail still goes out, and an unstamped notification is never budgeted (`TestEmailServerDispatcher_ClaimsNotificationMailOrigin`, FIX round 2);
- the tier lookup fails closed for an unknown business;
- `PurgeExpired`, which the janitor runs, keeps live rows and other kinds' rows.

`internal/emails`, reservation guest-text tests in `reservation_guest_text_test.go`.

`internal/fiscal/...`: bill-scoped receipt emailer attribution. A receipt the tenant budget refuses is deferred (no attempt burned) and dead-letters only after 72 hours.

`internal/services`:

- reservation field caps, boundaries included (80/81, 32/33, 300/301), for create and update;
- the approval reminder is tenant mail and stays in the guest-booking lane for a public-form booking (`TestApprovalReminderIsTenantMailInTheBookingLane`, FIX round 2);
- the public delivery checkout's order-received mail is stamped into the guest lane and the venue notice is left unstamped (`TestGuestDeliveryCheckout_ReceivedMailIsInTheGuestLane`, FIX round 2);
- accepted, status change, expired, payment received and cancelled delivery mail to the guest is stamped as the venue's operator mail (`TestDeliveryCustomerMailIsStampedForTheVenue`, FIX round 2).

`internal/server`:

- a public reservation with oversized fields or control characters gets 400;
- the booking form sends through `ForReservationRequest`;
- staff invite and resend disclose a budget refusal;
- the staff-added, staff-removed and wallet-change notices to `business.Email` are claimed against the business, none reaches the provider once the budget is spent, and the staff-added notice is skipped when the invite was refused (`TestOwnerNotices_CountAgainstTenantBudget`, FIX round 2);
- the legacy `/unsubscribe` route is neutral, ignores bad tokens and redirects on a valid token;
- the token POST soft-deletes;
- an opt-out is sticky;
- the subscribe form answers new, listed and opted-out addresses with the same neutral body;
- fiscal customer fields refuse control and bidi-override characters on the guest and operator routes.

`internal/handlers`:

- a receipt over budget gets 429 and its slot is released;
- a receipt resend inside the dedupe window answers `already_sent: true` and its slot is released;
- receipt display strings are sanitized.

`internal/database`:

- the static shape of migration 000224;
- with `-tags integration_postgres` and Docker:
  - `TestRollbackRehearsal_EachMigration` passed, including the 000224 down then up;
  - `TestSchemaFingerprint_*` passed only with a regenerated genesis snapshot (see the integrator notes).
