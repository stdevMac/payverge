<!-- Generated: 2026-08-01 | Files scanned: ~80 | Token estimate: ~700 -->

# Data Layer

## Database
**PostgreSQL 15** (postgres:15-alpine). Internal to compose, not host-exposed. GORM v2 + `lib/pq` driver.

## Migrations
Schema = genesis baseline (`backend/schema/genesis/current_schema.sql`, version 0) + numbered pairs in `backend/migrations/` (`NNNNNN_name.up.sql` + `.down.sql`).

**0 migrations** currently: the open-source release squashed history into the baseline (version 0), and the next schema change is `000001`. Verify with `ls backend/migrations`.

### Migration Failure Semantics
- Main `database.RunMigrations` failure aborts startup.
- Empty-database genesis bootstrap and post-migration schema verification also fail fast.
- Production startup runs no GORM AutoMigrate or schema-changing ensure helper.
- Partial migration mode is rejected in production.

## Model Files
`backend/internal/database/`:
```
business.go              — businesses, owners, settings
business_revenue_aggregate.go
bundles.go               — menu bundles
currency.go              — currencies, FX
customer_address_models.go + _migration.go
customer_business.go     — customer ↔ business link
delivery_models.go + _migration.go
director_console.go      — director threads
director_tool_calls.go   — director tool invocations
marketing_activity.go    — posted history + exact creative snapshots
marketing_settings.go    — validated creative profile in business JSON settings
error_logs.go            — backend error capture
hospitality.go           — tables, reservations
ai_menu_models.go        — AI-generated menu data
ai_waiter.go             — AI waiter sessions
admin_actions.go         — admin audit trail
models.go / models_json.go — Bill, Payment, AlternativePayment, etc. + custom MarshalJSON
```

## Key Domains
| Domain | Tables (high-level) |
|---|---|
| Auth | users, user_auths, user_sessions, user_session_refresh_history, auth_attempts |
| Business | businesses, staff, staff_memberships, staff_invitations, staff_identities, staff_permission_denies, staff_login_codes, business_operating_hours, business_operating_exceptions, business_schedule_settings, business_alert_settings |
| Menu | menus, offers, bundles, menu_extraction_jobs, menu_extraction_images, menu_wizard_sessions, menu_wizard_messages (categories and items are JSON on `menus.categories`) |
| Tables/Reservations | tables, table_reservations, reservation_settings, reservation_status_histories, table_combinations, table_combination_members |
| Bills/Orders | bills, bill_items, bill_history_events, bill_split_shares, orders, payments, alternative_payments, payment_refunds, payment_refund_destinations, crypto_payment_quotes |
| Inventory | inventory_items, inventory_movements, inventory_recipes, inventory_settings, inventory_alert_logs |
| Delivery | delivery_orders, delivery_drivers, delivery_zones, delivery_settings, delivery_status_history |
| CRM | customers, customer_businesses, customer_addresses, customer_preferences, customer_visits, customer_communications (tags and notes are columns on `customer_businesses`) |
| Analytics | page_views, conversion_events, user_interactions, session_summaries |
| Plugins | plugins, business_plugins, plugin_translations, plugin_notification_deliveries, plugin_notification_delivery_attempts |
| Printers | printers, print_jobs, print_audit_log |
| Fiscal | business_fiscal_settings, fiscal_receipts, fiscal_jobs, fiscal_audit_events, fiscal_delivery_tasks |
| Director | director_console_threads, director_console_messages, director_tool_calls, director_proposed_actions, director_action_audits |
| Marketing | `marketing_activities` with `creative_snapshot` JSONB; creative profile remains in `businesses.marketing_settings` JSON |
| WhatsApp | `whatsapp_business_devices` is in the genesis baseline; `whatsmeow_*` session tables are created at runtime by whatsmeow's sqlstore, only in builds with `-tags whatsapp` |

## Money Wire Contract
- **DB**: `int64` cents.
- **JSON**: `float64` dollars (via custom `MarshalJSON` in `backend/internal/database/models_json.go`).
- Covered types: `Bill`, `Payment`, `AlternativePayment`, `WithdrawalHistory`, `ManualLedgerEntry`, `PayrollRun`, `PayrollLineItem`, `PaymentBreakdown`, `ReservationSettings`.
- Printer formatters consume the float64 wire shape — do **not** re-divide by 100.

## Object Storage
Local disk by default (`STORAGE_DRIVER=local`); optionally S3-compatible object storage with a public + protected bucket pair (AWS SDK v2). See `docs/self-hosting/storage.md`.

## Caches / Queues
No Redis. Background processing via cron (`robfig/cron/v3`). Print queue persisted in PG (`print_jobs` table).
