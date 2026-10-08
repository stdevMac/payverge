# Showcase seed — "Trattoria Bella Vista"

A hand-curated, single-business fixture used to capture marketing screenshots
(features page, FirstRunWizard previews) against a running Payverge stack.

The general-purpose perf seed in `../` produces random "Item 1 / Item 2"
fixtures suitable for benchmarking. This one is the opposite: one realistic
restaurant, every surface populated, every row deliberate.

## What it seeds

| Surface | Count | Notes |
|---|---|---|
| Business | 1 | `business_id = showcase-bellavista`, full profile, AI settings, design |
| Operating hours | 7 | Closed Mon, lunch + dinner Tue–Sun |
| Gallery images | 4 | Unsplash placeholders (replace from dashboard) |
| Special features | 5 | "Outdoor seating", "Wine pairing", etc. |
| Menu categories | 6 | Antipasti / Pasta / Pizza / Mains / Dolci / Drinks |
| Menu items | ~34 | Named, priced, allergens, dietary tags |
| Tables | 19 | Patio, Window, Bar, Booth, Private Dining |
| Staff | 9 | Marco, Sofia, Giulia, etc. — PIN `showcase` |
| Loyalty | 4 tiers | Welcome → Cucina → Famiglia → Maestro |
| Reservation settings | 1 | 60-day window, 30-min slots, 18-max party |
| Reservations | 12 | 8 upcoming, 4 past (varied statuses) |
| Customers | 15 | CRM rows w/ visit counts, tags, allergies |
| Offers | 3 | Aperitivo Hour, Famiglia Sunday, Birthday Tiramisù |
| Historical bills | ~640 | Every day in past 90 (2 Mon, 5 Tue-Wed, 7 Thu, 11 Fri, 12 Sat, 8 Sun) with lunch/dinner spread |
| Active bills/orders | 6 | Mixed kitchen states (approved/in_kitchen/ready) |

Everything is keyed on stable natural identifiers (`showcase-*`) so re-runs are
no-ops on existing rows.

## How to run

### Option A — Docker compose (recommended)

```bash
# from repo root — placeholder owner
./scripts/seed-showcase.sh

# attach the business to an existing user so it shows up in their dashboard
./scripts/seed-showcase.sh --owner-email=you@example.com
```

That wraps:

```bash
docker compose --env-file .env --profile seed run --rm showcase-seed [--owner-email=...]
```

`profiles: [seed]` keeps the service out of regular `docker compose up`, so
this only runs when explicitly invoked.

The `--owner-email` user must already exist in the `users` table (register
via `/business/register` or OAuth first). If lookup fails the seed logs a
warning and continues with the placeholder owner — re-run later after the
user is created.

### Option B — Against a host-exposed Postgres

If you're running Postgres outside compose (e.g. local dev DB):

```bash
DSN='postgres://payverge:payverge_password@localhost:5432/payverge?sslmode=disable' \
  ./scripts/seed-showcase.sh --host
```

### Option C — Direct go run

```bash
cd backend
DATABASE_URL='postgres://...' go run ./perf/seed/showcase
```

## Idempotency / regenerating

Every entity is keyed on a unique column:

| Entity | Key |
|---|---|
| Business | `business_id = 'showcase-bellavista'` |
| Menu | one row per business (FirstOrCreate) |
| Staff | `email LIKE '%@trattoriabellavista.example'` |
| Tables | `table_code LIKE 'showcase-bellavista-%'` |
| Bills | `bill_number LIKE 'BV-%'` |
| Payments | `tx_hash LIKE '0xshowcase%'` |
| Reservations | `confirmation_code LIKE 'BV-RES-%'` |

To wipe and re-seed:

```sql
DELETE FROM businesses WHERE business_id = 'showcase-bellavista';
```

Cascades cover most child rows; some (customers, gallery, hours) have to be
cleared manually if you want a fully clean slate.

## Login as the operator

Easiest: pass `--owner-email=<your-account-email>` when running the seed (see
the Run section above). The seeder looks up the user by email and writes
`user_id` (and `owner_address` if the user has a wallet linked) on the
business row. Then log in normally with that account and the showcase
business appears in your dashboard.

If the user doesn't exist yet:

1. Register the account at `/business/register` (creates the `users` row).
2. Re-run `./scripts/seed-showcase.sh --owner-email=...` — it's idempotent
   and the second run will pick up the now-existing user.

If you prefer SQL:

```sql
UPDATE businesses
   SET owner_address = '<your-wallet-address>',
       user_id       = (SELECT id FROM users WHERE email = '<your-email>')
 WHERE business_id   = 'showcase-bellavista';
```

For staff PIN login, every staff row uses PIN `showcase`.

## Capturing screenshots

The README and `site/` screenshots come from `tools/screenshots/capture.mjs`,
which plays one dinner service against a local `--demo` install and writes
PNGs to `docs/assets/screenshots/` and a WebP subset to
`site/assets/screenshots/`. The header of that script lists the environment
variables it reads. It targets the `--demo` venue, not this showcase seed.

## Notes

* The seeder relies on the schema already being migrated (`golang-migrate up`).
  The backend container auto-migrates on boot, so running this against a stack
  that has booted at least once is enough.
* All money is stored as `int64` cents per the money-wire contract; the
  seeder converts from the human-readable USD `Price` floats on menu items.
* `tax_rate` on the business is **0.0875** (8.75% SF county) and historical
  bill totals reflect that plus a flat 18% tip.
