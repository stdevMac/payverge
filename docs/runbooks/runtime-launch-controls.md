# Runtime launch controls

The backend owns launch controls in PostgreSQL. They are evaluated before
mutation handlers, so hiding a frontend button is never the enforcement
boundary. Control changes and invite-batch creation require the authenticated
`/api/v1/admin` surface and append durable `runtime_control_audit_events` rows.

No launch control may be changed without a named owner, a non-empty reason, and
a future expiry. Expired feature controls fail closed. Migration `000181`
explicitly seeds existing features enabled through 2100 to preserve behavior;
maintenance and read-only mode seed disabled. Operators should replace those
bootstrap expiries with reviewed dates before launch.

| Control | Disabled/enabled behavior | Owner seeded by migration |
|---|---|---|
| `maintenance_mode` | When enabled, blocks mutations globally. Health, provider webhooks, and authenticated runtime-control recovery remain reachable. | platform on-call |
| `read_only_mode` | When enabled, blocks mutations globally with the same recovery exclusions. | platform on-call |
| `payments_enabled` | When disabled, blocks new payment/checkout/quote attempts. Provider webhooks remain available for reconciliation. | payments on-call |
| `fiscal_enabled` | When disabled, blocks fiscal mutations. | fiscal on-call |
| `ai_enabled` | When disabled, blocks AI mutations. | AI on-call |
| `uploads_enabled` | When disabled, blocks public and authenticated upload mutations. | platform on-call |
| `guest_orders_enabled` | When disabled, blocks guest order create, quote, and cancel mutations. | hospitality on-call |

All blocked operations return HTTP 503 with code
`RUNTIME_CONTROL_DISABLED` and the enforcing control name. A database error
while evaluating a mutation blocks the mutation rather than guessing that a
feature is enabled.

## Admin API

- `GET /api/v1/admin/runtime-controls`
- `PUT /api/v1/admin/runtime-controls/:key`
- `POST /api/v1/admin/runtime-controls/invite-batches`
- `GET /api/v1/admin/runtime-controls/audit?limit=100`

Example change body:

```json
{
  "enabled": false,
  "owner": "payments-oncall",
  "reason": "Provider incident INC-123",
  "expires_at": "2026-08-02T12:00:00Z"
}
```

The invite-batch endpoint accepts `name`, `cohort_cap`, `owner`, `reason`, and
`expires_at`. Its response includes a random plaintext `invite_code` exactly
once; only the SHA-256 digest is stored. Registration submits that value as
`invite_code`. The claim row, batch counter increment, user row, and auth row
share one transaction. The conditional counter update guarantees concurrent
registrations cannot exceed the batch cap.

## Drill and recovery procedure

1. Announce the bounded staging drill and name the control owner.
2. Record the current control document and latest audit-event ID.
3. Set a short expiry and the drill reason through the authenticated admin API.
4. Verify the intended mutation returns 503. For maintenance/read-only, also
   verify `/api/v1/health/live`, `/api/v1/health/ready`, provider reconciliation
   webhooks, and the admin runtime-control endpoint remain reachable.
5. Restore the prior value with a new reason; never edit the database row
   directly. Confirm a second audit event exists.

Repository tests prove the enforcement boundary; a drill on your own staging
instance is the operational proof that a control behaves as described.
