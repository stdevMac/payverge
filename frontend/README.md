# Payverge frontend

This directory is the Next.js 15 App Router frontend of Payverge. It serves
the operator console and the guest QR ordering and payment flow.

The UI is TypeScript, styled with Tailwind CSS and NextUI. Client state is
Zustand. Server state is React Query (`@tanstack/react-query`). Wallet reads
use Wagmi and Viem.

The API lives in [`../backend`](../backend). The product overview is
[`../README.md`](../README.md).

## Requirements

Node.js 22 and npm.

The repository, CI, and release builds use Node `22.22.0`, recorded in the
repo root [`.nvmrc`](../.nvmrc). This package sets
`"packageManager": "npm@10.9.4"` in `package.json`. npm is the only supported
package manager (no yarn, pnpm, or bun). Dependencies are locked in
`package-lock.json`.

From this directory:

```bash
npm ci
```

## Configuration

`NEXT_PUBLIC_*` values are inlined at build time.

For standalone `npm run dev`, create `frontend/.env.local` with the frontend
variables listed in the repo root [`../.env.example`](../.env.example). Which
names belong in that file is described in
[`frontend/.env.example`](.env.example). Never commit `.env.local`.

## Run

```bash
npm run dev
```

The development server listens on http://localhost:3000. The backend is
expected on port 8080. See [`../backend`](../backend).

## Scripts

Names below are the `scripts` entries in `package.json`. Run them from this
directory as `npm run <script>`.

| Script | What it runs |
| --- | --- |
| `dev` | `next dev` (development server) |
| `build` | Production `next build` |
| `start` | `next start` (serve a production build) |
| `typecheck` | Clears `.next/types`, then `tsc --noEmit` |
| `lint` | `eslint src/` |
| `test` | Jest |
| `format` | Prettier write of `src/**/*.{ts,tsx}` |
| `i18n:check` | Checks generated locale files against `locales/registry.json` |
| `i18n:validate` | Locale parity and structural validation |
| `test:e2e:staff` | Playwright: `tests/staff-roles.spec.ts` |

`dev` and `build` also run `predev` and `prebuild`
(`node scripts/copy-pdf-worker.mjs`), which copy the PDF.js worker before
Next.js starts.

## Layout

| Path | Role |
| --- | --- |
| `src/app/` | App Router routes |
| `src/api/` | Typed API client, one file per domain |
| `src/components/` | React components |
| `src/store/` | Zustand stores |
| `src/i18n/` | Custom translation system |

Guest copy is one JSON file per locale under `src/i18n/guest-messages/`
(21 locales). Operator copy for `en` and `es` is under
`src/i18n/messages/<locale>/`. `src/i18n/messages/es-ar/` is a regional
override layer merged on top of `es`. It holds only the keys whose
Rioplatense wording differs, and it may not add keys that `es` lacks.

[`locales/registry.json`](../locales/registry.json) at the repository root is
the source of truth for which locales exist. Generated locale files are
produced from it. Do not edit those generated files by hand.

## Deploy

Self-hosting uses the published images and the Compose files in
[`../deploy/`](../deploy/). See [`../deploy/README.md`](../deploy/README.md).
[`frontend/Dockerfile`](Dockerfile) builds the frontend image.

## Contributing

See [`../.github/CONTRIBUTING.md`](../.github/CONTRIBUTING.md).
