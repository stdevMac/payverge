# Responsive owner rails: findings

**Date:** 2026-08-06  
**Stack:** local docker compose stack with demo data · `admin@local.test`  
**Harness:** `frontend/tests/qa-pipeline/responsive-owner-tabs.spec.ts`

## Command

```bash
cd frontend
PLAYWRIGHT_RUN_QA_PIPELINE=1 \
  PLAYWRIGHT_BASE_URL=http://localhost:3000 \
  PLAYWRIGHT_API_BASE=http://localhost:8080/api/v1 \
  PLAYWRIGHT_FORCE_IPV4_LOCALHOST=1 \
  npx playwright test --config=playwright.qa-pipeline.config.ts \
    responsive-owner-tabs.spec.ts --project=mobile-390 --project=tablet-834
```

## Result (2026-08-06)

| Project | Viewport | Tabs probed | Overflow failures | Console fatals |
|---------|----------|-------------|-------------------|----------------|
| mobile-390 | 390×844 | all `OWNER_TABS` (priority: kitchen, counter, cash-register, bills first) | **0** | **0** |
| tablet-834 | 834×1112 | same | **0** | **0** |

Playwright: **2 passed (49.8s)**.

Raw JSON: `frontend/test-results/qa-pipeline/gap1-responsive-findings.json` (local, gitignored).

## Findings list

**No horizontal overflow or fatal console errors** observed on any owner rail at either viewport for the demo business under the local admin session.

This is a successful coverage probe, not a product fix. Future layout regressions should re-run the command above.

## Config note

Projects intentionally use **Chromium** with viewport overrides (not WebKit iPhone/iPad device profiles) so the qa-pipeline harness does not require extra browser downloads.
