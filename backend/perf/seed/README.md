# Perf seed CLI

Idempotently seeds Postgres with fixture data so the Go benchmarks
(Tasks 7–8) and k6 load scripts (Tasks 10–11) have realistic data to
exercise the backend against.

## Safety

`make perf-seed` reads `DATABASE_URL` from the environment. If your shell
already has a dev `DATABASE_URL` (or sources `.env`), the seeder will
silently target whatever that points to. **Always pass the target DSN
explicitly:**

```bash
make perf-seed DATABASE_URL='postgres://...'   # safe — overrides the env
```

Don't run this against a production or shared staging database.

## Usage

```bash
cd backend
DATABASE_URL='postgres://user:pass@host:5432/db?sslmode=disable' make perf-seed
```

Or via `go run` with overrides:

```bash
cd backend
go run ./perf/seed --dsn='postgres://...' --businesses=10 --orders-history=200
```

Default counts:

| Flag                       | Default | Meaning                                                     |
| -------------------------- | ------- | ----------------------------------------------------------- |
| `--businesses=N`           | 20      | Number of seeded businesses                                  |
| `--products-per-business=N`| 50      | Menu items per business (lives inside `Menu.Categories` JSON)|
| `--staff-per-business=N`   | 5       | Staff users per business (roles round-robin)                |
| `--orders-history=N`       | 1000    | Historical (bill, payment, order) triplets per business     |
| `--dsn=...`                | (none)  | Overrides `DATABASE_URL`                                    |
| `--quiet`                  | false   | Suppress GORM info logging                                  |

## Idempotency

Re-running the CLI is a no-op for existing rows. Dedupe keys:

| Entity   | Unique key                                                  | Example                                |
| -------- | ----------------------------------------------------------- | -------------------------------------- |
| Business | `business_id` (also sets `custom_url` to the same value)    | `perf-seed-001`                        |
| Staff    | `email`                                                     | `perf-seed-staff-001-3@example.test`   |
| Table    | `table_code`                                                | `perf-seed-001-t01`                    |
| Bill     | `bill_number`                                               | `PERF-001-00042`                       |
| Payment  | `tx_hash`                                                   | `0xperfseed00100042`                   |
| Order    | composite `(bill_id, created_by, client_request_id)`        | `client_request_id = perfseed-001-00042` |
| Menu     | `FirstOrCreate` on `business_id` only — existing menus are **NOT** overwritten |                                        |

If you need to regenerate a menu after schema changes, delete the
existing row first:

```sql
DELETE FROM menus WHERE business_id IN (
  SELECT id FROM businesses WHERE business_id LIKE 'perf-seed-%'
);
```

## Schema prerequisites

Run migrations against the target database before the seed:

```bash
migrate -path backend/migrations -database "$DATABASE_URL" up
```

The seeder relies on these tables existing with their normal indexes:
`businesses`, `menus`, `staff`, `staff_identities`, `staff_memberships`,
`staff_login_codes`, `tables`, `bills`, `payments`, `orders`.

## Fixture characteristics

* Bill timestamps are spread across the past 90 days (deterministic via
  a fixed RNG seed) so time-bucketed analytics queries have something to
  group on.
* Order status is ~80% `delivered`, ~20% `cancelled` (every 5th by
  sequence number).
* All currency amounts are stored as `int64` cents in line with the
  money-wire contract; menu prices are deterministic per
  `(business_idx, item_idx)`.
* Wallet addresses use a single placeholder
  (`0xPERFSEED0000...`) so fixtures are easy to recognise in the DB.

## Notes

* `BusinessPageEnabled` and `SubscriptionStatus = 'active'` are set so
  the guest menu route (`GET /business/:customUrl/menu`) accepts
  requests against the seeded businesses.
* `CustomURL` is set to the same value as `BusinessId` — k6 scripts
  should use the slug `perf-seed-NNN` to hit the menu route.
* Staff PINs are bcrypt-hashed once at startup (constant `perfseed`),
  reused across all staff rows so the seeder doesn't pay 60–100 ms per
  hash.
* Each business has one deterministic open bill (`PERF-ACTIVE-NNN`) and its
  manager has a one-use login code (`86NNNN`). Rerunning the seed restores the
  open/orderable state and unconsumed code. Business one uses `860001`.
* Normalized staff identity/membership rows are seeded alongside legacy staff,
  so the real login-code route works on the current expand schema.
