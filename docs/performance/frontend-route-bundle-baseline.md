# Frontend route-bundle performance gate: baseline

**Date:** 2026-08-06  
**Metric owner:** `frontend/scripts/check-route-bundle-budget.js`  
**Budgets file:** `frontend/route-bundle-budget.json`

## Why this exists

`CLAUDE.md` defines a **Backend** Performance Gate only (`testing.B`, `-benchmem`,
`-count=3`, results in `summary.md`). Frontend changes that only move bundle
weight (dashboard code splitting, polling changes) had no gate to report against. This document is the
FE sibling: **per-route raw on-disk KiB** from `.next/app-build-manifest.json` +
`statSync` on each listed chunk. FE numbers live here under `docs/performance/`;
do **not** append them to `summary.md`.

Sibling (different metric): `frontend/scripts/check-bundle-budget.js` measures
**gzipped** shared-root + total client JS. Both gates are useful; the dashboard split uses the
route graph gate because the dashboard's static-import wall shows up as a fat
page-route sum.

## Commands (exact)

```bash
cd frontend

export NEXT_PUBLIC_API_URL=http://localhost:8080/api/v1 \
  NEXT_PUBLIC_BASE_URL=http://localhost:3000 \
  NEXT_PUBLIC_APP_URL=http://localhost:3000 \
  NEXT_PUBLIC_FRONTEND_URL=http://localhost:3000 \
  NEXT_PUBLIC_RPC_URL=https://mainnet.base.org \
  NEXT_PUBLIC_NETWORK=baseSepolia \
  NEXT_PUBLIC_SUPPORT_EMAIL=support@your-domain.example \
  NEXT_PUBLIC_ALLOW_INSECURE_URLS=true

npm run build
npm run check:route-budget          # assert against route-bundle-budget.json
# or: node scripts/check-route-bundle-budget.js --json
# reseed (+10% headroom): node scripts/check-route-bundle-budget.js --write
```

Unit tests (no build required):

```bash
npx jest --watchman=false --runInBand scripts/__tests__/check-route-bundle-budget.test.js
```

## Baseline measured 2026-08-06 (before the dashboard split)

| Metric | Value |
|--------|-------|
| Dashboard route | `/(shop)/business/[businessId]/dashboard/page` |
| Dashboard total (raw) | **5920.7 KiB** (~5.78 MiB) |
| Dashboard chunk count | **46** |
| Median page route | **2046.2 KiB** |
| Page route count | 81 |

Next.js own reporter (for cross-check only; **not** the gate metric):

| | Size | First Load JS |
|--|------|---------------|
| `/business/[businessId]/dashboard` | 630 kB | 1.72 MB |

Gate budgets seeded with +10% headroom:

| Budget key | Value |
|------------|-------|
| `dashboardTotalKiB` | 6513 |
| `dashboardChunkCountMax` | 51 |
| `medianPageKiB` | 2251 |

## Gate behaviour verified

- Pass at baseline: `check-route-bundle-budget: OK — all route budgets satisfied`
- Intentional breach (`dashboardTotalKiB: 1`) exits 1 with:
  `dashboard total: 5920.7 KiB > budget 1 KiB`

## Notes

- Raw sums over-count shared chunks that appear in multiple routes. This is a
  regression signal for the dashboard graph, not true network cost.
- For polling or request-count changes use **network call-count** Jest gates
  (fake timers), not this bundle harness.
- After the dashboard `dynamic()` split, the same commands were re-run; the
  after numbers are in `docs/performance/frontend-route-bundle-dashboard-split.md`.
