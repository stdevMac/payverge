# Frontend route-bundle — L3-1 after `dynamic()` rail split

**Date:** 2026-08-06  
**Branch:** `audit/r3-perf`  
**Baseline:** `docs/performance/frontend-route-bundle-baseline.md` (pre-L3-1)

## Commands (exact; three clean builds)

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

for i in 1 2 3; do
  npm run build
  npm run check:route-budget
  node -e 'const {measureAllRoutes}=require("./scripts/check-route-bundle-budget"); console.log(JSON.stringify(measureAllRoutes().dashboard))'
done
```

## Before / after (gate metric: app-build-manifest + statSync)

| Metric | Baseline (pre L3-1) | After L3-1 (×3 builds) | Delta |
|--------|---------------------|------------------------|-------|
| Dashboard total KiB | **5920.7** | **3394.3** | **−2526.4 KiB (−42.7%)** |
| Dashboard chunks | **46** | **39** | **−7** |
| Median page KiB | 2046.2 | 2047.9 | ~flat |
| Gate | OK under 6513/51 | OK under 6513/51 | pass |

Next.js reporter (cross-check, not the gate metric):

| | Baseline | After |
|--|----------|-------|
| Route Size | 630 kB | **83.8 kB** |
| First Load JS | 1.72 MB | **1.06 MB** |

## What changed

All ~20 statically imported dashboard rails converted to `dynamic(..., { ssr: false, loading: PremiumLazyTabSkeleton })`, matching the existing AI rail pattern. Shell (`DashboardLayout`, banners, print agent) stays static. Import block only in `page.tsx` (polling seam unchanged in this commit).

## Notes

- Shared `lazyRail` options object is **rejected** by Next (`dynamic options must be an object literal`) — options inlined per call.
- Budgets were intentionally **not** reseeded tighter yet so residual headroom remains for adjacent sessions; coordinator may reseed after merge.
