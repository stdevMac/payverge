---
name: configure-restaurant
description: Set up or change a restaurant on a running Payverge instance through the payverge-admin MCP tools, covering the business profile, opening hours, menu, tables and QR links, staff, reservations, the AI waiter and payment readiness. Use when an owner asks to configure their restaurant, load a menu, add tables, invite staff or check what is left to set up.
---

# Configure a restaurant

You act as the restaurant owner through the MCP server in
`tools/payverge-admin-mcp`. Every tool calls an endpoint the backend already
serves. Tool reference: `tools/payverge-admin-mcp/README.md`.

## 0. Preconditions

1. The MCP server must be registered with your client. If no
   `payverge_*` tools are available, stop and point the owner to the
   "Client configuration" section of `tools/payverge-admin-mcp/README.md`.
   Do not ask for the password in chat. The owner exports
   `PAYVERGE_OWNER_EMAIL` and `PAYVERGE_OWNER_PASSWORD` in their own shell
   before registering the server.
2. For a first look or an audit, suggest `PAYVERGE_MCP_READ_ONLY=true`.
   Previews still work, and every apply is refused.
3. If the instance hosts more than one restaurant, suggest
   `PAYVERGE_MCP_BUSINESS_IDS=<id>`. The MCP tools then refuse the other
   businesses. It is a guard in the MCP server, not a backend permission,
   and it does not narrow the instance-wide admin tools (health, error
   logs, failed webhooks, fiscal jobs). Leave `PAYVERGE_ADMIN_MCP_TOKEN`
   unset for restaurant setup.

## 1. Rules for every write

- Call the write tool once as a preview: `dry_run` defaults to `true`. Show
  the owner the `diff` and any `warnings` in plain language.
- Apply with `dry_run: false` only after the owner agrees with that preview.
- `payverge_import_menu` with `mode: "replace"` deletes the whole current
  menu. It needs `confirm: true` as well, and the owner must explicitly ask
  for a replace.
- Prices are in major units of the business currency (`12.5` means 12.50).
- Menu writes carry the menu version. The tool retries a version conflict
  once on its own. A `conflict` error means someone keeps editing the menu
  in the dashboard: re-read it with `payverge_get_menu`, redo the preview
  and ask again.
- On `sanitization_review_required`, the backend would drop fields (an
  unknown allergen, an out-of-range price). Show the owner the details. Fix
  the input, or pass `confirm_sanitization: true` only if they accept the
  cleaned version.
- If a bulk tool stops part-way (`import_stopped`, `bulk_create_stopped`),
  `partial` lists what was already written. Retry with only the rest.
- Every error carries a `hint` and usually a `request_id`. Quote both when
  you hand a failure back to the owner.
- Never print an invitation link unless the owner asks for it
  (`reveal_invitation_url: true`). Treat it like a password.

## 2. Find out where things stand

1. `payverge_instance_status`: the server is up, and you can see which
   features are on (`features.ai`, email, WhatsApp) and how the MCP server is
   authenticated.
2. `payverge_list_businesses`. No business yet? Go to step 3. Several? Ask
   which one.
3. `payverge_get_setup_status` for that business: the dashboard's first-run
   checklist. Work through the missing items in the order below and skip the
   ones that are already done.

## 3. Business profile

- New restaurant: `payverge_create_business` with `name` and
  `business_type` (`restaurant`, `cafe`, `bar`, `quick_service`,
  `food_truck`, `bakery`, `fine_dining` or `other`).
- Existing one: `payverge_get_business_profile`, then
  `payverge_update_business_profile` with only the fields that change.
  Confirm `default_currency` (ISO 4217, for example `ARS` or `EUR`) and
  `timezone` (IANA, for example `America/Argentina/Buenos_Aires`) before any
  menu work. Prices and the service day depend on them.

## 4. Opening hours

`payverge_get_opening_hours`, then `payverge_set_opening_hours`. The schedule
maps a weekday (`monday` .. `sunday`) to `{closed: true}` or to up to two
`periods` of `{open, close, kitchen_close?}` in `HH:MM`. Use `mode: "merge"`
(the default) to change only the days you pass.

## 5. Menu

1. `payverge_get_menu` to see what exists.
2. Turn the owner's material (a pasted list, a spreadsheet, a typed menu)
   into `{categories: [{name, description?, items: [{name, price,
   description?, allergens?, dietary_tags?, options?}]}]}`. Ask the owner
   about allergens. Never guess them: guests rely on them.
3. `payverge_import_menu` with `mode: "append"` (the default). Items whose
   name already exists in that category are skipped. The limit is 500 items
   per call.
4. Small edits use `payverge_add_menu_category`,
   `payverge_update_menu_category`, `payverge_add_menu_item` and
   `payverge_update_menu_item`. The category update has no `sort_order`
   field, so it resets a stored order to 0. The tool warns you when that will
   happen. Tell the owner, and leave ordering to the dashboard's menu tab.
5. For a photo or PDF of a printed menu, the in-app menu wizard extracts it
   (it needs an LLM provider, see `docs/agents/README.md`). Send the owner
   to the dashboard's menu onboarding instead of transcribing images
   yourself.

## 6. Tables and QR codes

1. `payverge_list_tables`.
2. `payverge_bulk_create_tables` with explicit `names`, or with `prefix`,
   `start` and `count` (for example "Table 1" to "Table 12"), and
   `capacity`. Existing names are skipped. The limit is 200 per call.
3. The result lists each table's `guest_url` (`<public URL>/t/<table_code>`).
   That is the link the printed QR code opens. If the links point at the
   wrong host, the MCP server needs `PAYVERGE_PUBLIC_URL`.

## 7. Staff

`payverge_list_staff`, then `payverge_invite_staff` with `email`, `name` and
`role` (`manager`, `server`, `host` or `kitchen`). Use
`payverge_change_staff_role` with a `reason` to change a role later. The
invitee gets an email. If the instance has no email provider, the email
only goes to the backend log: say so, and offer
`reveal_invitation_url: true` so the owner can pass the link on themselves.

## 8. Reservations

`payverge_get_reservation_settings`, then
`payverge_update_reservation_settings` with only the changed fields.

## 9. AI waiter

1. `payverge_get_ai_settings` shows the business settings and whether the
   server has a provider.
2. `payverge_update_ai_settings`: `enabled` (the table page),
   `business_page_enabled` (the public business page), `name`, `priority`
   (`balanced`, `upselling` or `service`) and `special_instructions`
   (house rules, up to 4000 characters). This tool needs the owner account.
   A manager account cannot use it.
3. If the server has no provider, the waiter runs in basic mode, answering
   from the menu only. Turning on full AI is a server change. Point the
   owner to the "Enable the in-app AI" section of `docs/agents/README.md`.

## 10. Payments

`payverge_payment_plugin_status` reports the installed and enabled payment
plugins, their last error and whether payments are ready. Connecting Stripe,
MercadoPago or PayPal needs the provider's credentials or an OAuth sign-in,
so it stays in the dashboard's plugins page. Tell the owner which plugin
needs attention, and never ask them for API keys in chat. Cash and
pay-at-counter work without any plugin.

## 11. Make it the home page

The instance root `/` serves the published venue. A venue is published when
it has a `custom_url` (its slug, `/b/<custom_url>`) and its public page is
enabled. Set both with `payverge_update_business_profile` (`custom_url`,
`business_page_enabled: true`) once the owner agrees the page is ready.

- One published venue: `/` serves it within a minute (the home lookup is
  cached for 60 seconds).
- Several published venues: `/` lists them. To pin one, the operator sets
  `PRIMARY_VENUE=<custom_url>` in the server's `.env` and restarts the
  backend; you cannot set it through the API.
- Nothing published: `/` keeps redirecting to `/dashboard`.

See `docs/api/home.md`.

## 12. Finish

Run `payverge_get_setup_status` again and summarise for the owner:

- what you changed, with ids;
- what is still open;
- what only they can do in the dashboard: payment credentials, logo and
  photos, printing the QR codes.
