# Home API

`GET /api/v1/home` tells the frontend what the instance root (`/`) serves. The
endpoint is public, rate limited like the other guest reads, and cached for 60
seconds (`Cache-Control: public, max-age=60`). The frontend server also keeps
its last good answer for 30 seconds (`HOME_TTL_MS` in
`frontend/src/lib/instance/serverHome.ts`), so publishing a venue or changing
`PRIMARY_VENUE` can take up to about 90 seconds to show at `/`.

The handler is `backend/internal/server/home_handler.go`. Venues come from
`backend/internal/database/home_venues.go`, which uses the same publish rule as
`/b/<custom_url>`.

## How `/` is chosen

1. `PRIMARY_VENUE` set to a published venue's numeric id, storefront slug
   (`custom_url`) or `business_id` slug, tried in that order (the storefront
   slug ignores case; `business_id` matches exactly): `/` serves that venue.
2. Otherwise, exactly one published venue: `/` serves it.
3. Several published venues: `/` shows a directory that links each one at
   `/b/<custom_url>`.
4. No published venue: `/` redirects to `/dashboard`, keeping the query
   string, so a fresh install lands on the operator sign-in.

A `PRIMARY_VENUE` that matches no published storefront logs one warning and
falls back to rules 2 to 4.

`PRIMARY_VENUE` only decides what the public `/` serves. It does not change
where an operator lands after sign-in: an owner or staff member with more than
one venue still gets the venue picker at `/dashboard`, and one with a single
venue goes straight to that venue's dashboard, whatever `PRIMARY_VENUE` names.
The same holds for a `--demo` instance.

Links that carry `invite_code` or `invite` on `/` are always redirected to
`/dashboard` by the frontend middleware, whatever `/` serves.

## Response

```json
{
  "mode": "venue",
  "primary": { "id": 7, "name": "Casa Lola", "logo": "", "custom_url": "casa-lola", "city": "Buenos Aires" },
  "venues": [
    { "id": 7, "name": "Casa Lola", "logo": "", "custom_url": "casa-lola", "city": "Buenos Aires" }
  ]
}
```

| Field | Meaning |
|---|---|
| `mode` | `venue`, `directory` or `empty`. |
| `primary` | The venue served at `/` in `venue` mode, otherwise `null`. |
| `venues` | Published venues, at most 200. Empty in `empty` mode. |

A database error answers `500` with `{"error": "Failed to resolve home"}`.
