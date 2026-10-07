---
name: rebrand
description: Give a Payverge instance or fork its own product name, logo, colour, contact addresses and domain, first through the backend's branding environment variables and then through the web app's built-in assets, metadata and message files. Use when someone wants to white-label, rename or rebrand Payverge, or must rebrand before offering a fork as a hosted service.
---

# Rebrand a Payverge instance

"Payverge" and its logo and icons are trademarks. They are not licensed
under Apache-2.0 beyond nominative use. Running your own copy for yourself is
fine. **Offering a fork as a hosted service or a distributed product needs
your own name and logo.** Read `TRADEMARKS.md` first and tell the operator
which case they are in.

Keep `LICENSE`, `NOTICE` and the attributions in them. Rebranding changes the
name, not the authorship.

There are two layers:

| Layer | What it reaches | Needs a rebuild? |
|---|---|---|
| 1. Backend env vars | Transactional email, `GET /api/v1/instance`, links, CORS and redirects | No, only a backend restart |
| 2. Web app assets and copy | Favicons, app icons, social cards, page metadata, UI strings, colour palette | Yes, a frontend image you build yourself |

Layer 1 is enough for a private instance. A public fork needs both.

## Layer 1: environment variables

The backend reads these settings in `backend/internal/config/instance.go`.
Ask the operator for each value, then set them in the `.env` the compose
project reads:

| Variable | Default | Meaning |
|---|---|---|
| `PUBLIC_URL` | none (required in production) | Canonical `https://` origin. QR links, email links, CORS and redirects all follow it. |
| `PRODUCT_NAME` | `Payverge` | Product name |
| `COMPANY_NAME` | `PRODUCT_NAME` | Who operates the service |
| `LEGAL_ENTITY` | `COMPANY_NAME`, if set | Legal name in footers and terms |
| `COMPANY_ADDRESS` | empty | Postal address in marketing-email footers |
| `SUPPORT_EMAIL` | empty (nothing shown) | Support contact |
| `SECURITY_EMAIL` | `SUPPORT_EMAIL` | Security contact |
| `LOGO_URL` | the bundled logo | An `https://` URL or a root-relative path |
| `BRAND_COLOR` | `#1a6b6a` | `#rgb` or `#rrggbb` |

Apply and check:

```bash
docker compose up -d backend
curl -s https://pos.example.com/api/v1/instance | jq '{product_name, company_name, logo_url, brand_color, public_url, support_email}'
```

Then send yourself a test email (sign up, or reset a password) and check the
sender name, logo and footer.

## Layer 2: the web app

These are code changes in a fork. Work on a branch, and stage files by
explicit path. Never use `git add -A`.

1. **Inventory first.** List every place that still says Payverge:

   ```bash
   git grep -n -i "payverge" -- frontend/src frontend/public ':!*.test.*' ':!**/__tests__/**' | wc -l
   git grep -l -i "payverge" -- frontend/src frontend/public ':!*.test.*' ':!**/__tests__/**'
   ```

   Show the operator the counts by folder and agree on the scope before
   editing.

2. **Images.** Replace these files with the operator's own, keeping each
   file name and pixel size:
   - `frontend/public/images/PayvergeLogo.*` and `frontend/public/images/logo.*`
   - `frontend/public/favicon*`, `frontend/public/apple-touch-icon.png`,
     `frontend/public/android-chrome-*` and `frontend/public/maskable-icon-*`

   The share card (`/share-card.png`) is generated per request from the
   instance name, `LOGO_URL` and `BRAND_COLOR`; there is no image file to
   replace.

   Do not replace `frontend/public/images/plugins/`. Those are third-party
   marks that identify payment providers.

3. **Web manifests.** Update `name`, `short_name` and `description` in
   `frontend/public/site.webmanifest`, `frontend/public/site.es.webmanifest`
   and `frontend/public/site.es-ar.webmanifest`.

4. **Page metadata.** Start with the root `metadata` in
   `frontend/src/app/layout.tsx` (title, authors, creator, publisher, Open
   Graph). Then work through the nested `layout.tsx` and `page.tsx` files
   from the inventory.

5. **UI copy.** Brand strings live in the message files:
   - `frontend/src/i18n/messages/<locale>/*.json` (operator dashboard: en,
     es and the es-ar overrides)
   - `frontend/src/i18n/guest-messages/<code>.json` (21 guest locales)

   Change the same key in every locale so the parity hooks stay green. Do
   not machine-translate unrelated strings while you are there.

6. **Colour (optional).** `BRAND_COLOR` reaches only email and
   `/api/v1/instance`. The web palette is in `frontend/tailwind.config.ts`
   (the `#1a6b6a` scale). `frontend/eslint.config.mjs` enforces palette
   usage, so change the token values rather than adding raw hex codes in
   components.

7. **Tests that pin copy.** Some tests assert metadata and public copy, for
   example the `*.metadata.test.ts` files next to the pages. Find them with
   `git grep -l -i payverge -- 'frontend/src/**/*.test.*'`. Update their
   expectations in the same commit, and say so in the commit message.

8. **Verify.** Run these from `frontend/`:

   ```bash
   npm run typecheck
   npm run lint
   npm test
   npm run i18n:validate
   npm run build
   ```

9. **Ship.** Build and publish your own frontend image from
   `frontend/Dockerfile`. Then point the `frontend` service in your compose
   file at that image instead of the upstream `ghcr.io/stdevmac/payverge-*`
   frontend image.
   Rebuild the backend image too if you changed anything under `backend/`.

## Domain

- Set `PUBLIC_URL` (layer 1) and the Caddy site address (`DOMAIN` in
  `deploy/.env.example`) to the new host.
- Existing printed QR codes encode the old host. Tell the operator to
  reprint them, or to keep a redirect from the old host.
- Upstream docs and tests keep `payverge.io` on purpose. Do not rewrite them
  as part of a rebrand.

## Do not

- Remove `LICENSE`, `NOTICE` or copyright lines.
- Imply that the Payverge authors endorse or operate the fork.
- Name the fork, its domain or its app-store listing "Payverge" or anything
  confusingly similar.
