# Frontend route-bundle performance gate — baseline (Session Q / Root A)

**Date:** 2026-08-06  
**Branch:** `audit/r3-perf`  
**Metric owner:** `frontend/scripts/check-route-bundle-budget.js`  
**Budgets file:** `frontend/route-bundle-budget.json`

## Why this exists

`CLAUDE.md` defines a **Backend** Performance Gate only (`testing.B`, `-benchmem`,
`-count=3`, results in `summary.md`). Ledger rows L3-1 / L3-12 / L6-8 were parked
at "PENDING (perf gate)" behind a gate that did not exist. This document is the
FE sibling: **per-route raw on-disk KiB** from `.next/app-build-manifest.json` +
`statSync` on each listed chunk. FE numbers live here under `docs/performance/`;
do **not** append them to `summary.md`.

Sibling (different metric): `frontend/scripts/check-bundle-budget.js` measures
**gzipped** shared-root + total client JS. Both gates are useful; L3-1 uses the
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

## Baseline measured 2026-08-06 (pre L3-1)

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

- Raw sums over-count shared chunks that appear in multiple routes — same as the
  scout metric. Regression signal for the dashboard graph, not true network cost.
- For L3-12 / L6-8 use **network call-count** Jest gates (fake timers), not this
  bundle harness.
- After L3-1 (`dynamic()` splits), re-run the same commands and record after
  numbers in `docs/performance/frontend-route-bundle-l3-1.md`.
