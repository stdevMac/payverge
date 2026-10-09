# Guest orders API

Public endpoints that let a guest order from a table's QR code. No login is
needed. The table code in the URL identifies both the table and the venue.

There is no OpenAPI spec for the Payverge API yet. This page is the reference
for these endpoints.

## Endpoints

| Method | Path | Notes |
|---|---|---|
| `POST` | `/api/v1/guest/table/{code}/order/quote` | Prices a cart without creating anything. |
| `POST` | `/api/v1/guest/table/{code}/order` | Creates the order. Requires `X-Request-Id`. |
| `POST` | `/api/v1/guest/table/{code}/orders/{orderId}/cancel` | Cancels a guest order while it is still pending. |

All three share a per-IP limit of 120 requests per minute. Operators can change
it with `GUEST_ORDER_RATE_LIMIT_REQUESTS_PER_MINUTE`.

## Create an order

```http
POST /api/v1/guest/table/T7-4KQ9/order
Content-Type: application/json
X-Request-Id: 3f1c9a4e-7b2d-4c8e-9f10-2a6b5d4e3c21

{
  "bill_id": 812,
  "items": [
    { "menu_item_id": "item1", "menu_item_name": "Coffee", "quantity": 2, "price": 3.5,
      "special_requests": "oat milk" }
  ],
  "notes": "",
  "promo_code": ""
}
```

- `bill_id` is optional. Leave it out, or send `0`, to open a new bill for the
  table. If you send it, the bill must be open and belong to this table.
- `items[].price` is informational. The server re-prices every line from the
  menu.
- `notes` and each `special_requests` can be at most 500 characters.

### X-Request-Id is the idempotency key

`X-Request-Id` is **required**, because it is what makes ordering safe to
retry. Guests on restaurant Wi-Fi often time out after the server has already
saved the order. With the same id, a retry returns that original order instead
of creating a second one.

The rules:

- Generate a **new** id for each order the guest places. A UUID is
  recommended. The web client uses `crypto.randomUUID()`.
- **Reuse** the same id when retrying that order. This applies to network
  errors, timeouts and 5xx responses.
- The id can be at most **64 bytes**, after leading and trailing whitespace is
  trimmed. Any characters are accepted.
- Ids are unique per venue. An id that was already used at another table of the
  same venue is rejected with `409 idempotency_conflict`.
- A replay returns the original order **as it was first created**, even if the
  retried body is different. To change the cart, use a new id.

The response always carries an `X-Request-Id` header. It echoes your id when
the id is 1–64 characters of `[A-Za-z0-9-]`. Otherwise the header holds a
server-generated trace id. This is for log correlation only: the idempotency
key is always the header you sent.

### Responses

| Status | Meaning |
|---|---|
| `201 Created` | The order was created. |
| `200 OK` | Replay: an order already exists for this `X-Request-Id`, and it is returned unchanged. |

```json
{
  "bill": { "...": "public bill projection" },
  "order": { "...": "the order" },
  "quote": { "...": "cent-exact price breakdown" },
  "replay": false,
  "duplicate": false
}
```

`duplicate` is a legacy alias of `replay`.

### Errors

Errors are JSON with `error` and, on every guest-facing failure, a stable
`code`. The guest frontend localizes errors by `code`. The X-Request-Id errors
also carry an English `message` for API clients.

| Status | `code` | When |
|---|---|---|
| 400 | `request_id_required` | The `X-Request-Id` header is missing or blank. `error` is `missing_request_id`. |
| 400 | `request_id_invalid` | `X-Request-Id` is longer than 64 bytes. `error` is `invalid_request_id`. |
| 400 | `text_too_long` | `notes` or `special_requests` is over 500 characters. |
| 400 | `VALIDATION_INVALID_INPUT` | The body is malformed or an item is invalid. |
| 403 | `business_unavailable` | A server administrator suspended or closed the venue. |
| 404 | (none) | Unknown table code. |
| 409 | `ordering_disabled` | The venue has turned off guest ordering. |
| 409 | `bill_not_open` | `bill_id` refers to a closed bill. |
| 409 | `item_not_orderable` | One or more items are sold out or hidden. `details.items` lists them. |
| 409 | `idempotency_conflict` | The `X-Request-Id` was already used at another table of the same venue. |
| 429 | (none) | Rate limited. |

The two `X-Request-Id` errors look like this:

```json
{
  "error": "missing_request_id",
  "code": "request_id_required",
  "message": "The X-Request-Id header is required. Send a new unique id (a UUID is recommended) for each order and reuse it when retrying that order, so the order is created at most once."
}
```

```json
{
  "error": "invalid_request_id",
  "code": "request_id_invalid",
  "message": "The X-Request-Id header must be at most 64 bytes (a UUID is 36). Send a shorter unique id for each order."
}
```

Both are checked before any order is written, so retrying them is safe.

## Source

- Handler: `backend/internal/server/guest_handlers.go` (`CreateGuestOrder`).
- Request-id validation: `backend/internal/server/guest_request_id.go`.
- Checkout and replay: `backend/internal/services/guest_checkout.go`.
- Tests: `backend/internal/server/guest_request_id_test.go`.
