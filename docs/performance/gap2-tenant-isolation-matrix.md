# GAP-2 tenant isolation matrix (Session Q)

**Generated:** 2026-08-06T05:08:09.512490Z

- Owner A: `admin@local.test` businesses [48, 49]
- Owner B: `owner-b-gap2@example.invalid` businesses [50]
- Leaks found: **0** (success if zero)

## Outcomes

| Owner | Method | Path | Status | Isolated | Leaked | Note |
|-------|--------|------|--------|----------|--------|------|
| A | GET | `/inside/businesses/50` | 403 | True | False | business detail |
| A | GET | `/inside/businesses/50/bills` | 403 | True | False | bills list |
| A | GET | `/inside/businesses/50/menu` | 403 | True | False | menu |
| A | GET | `/inside/businesses/50/staff` | 403 | True | False | staff |
| A | GET | `/inside/businesses/50/reservations` | 403 | True | False | reservations |
| A | GET | `/inside/businesses/50/analytics/overview` | 404 | True | False | analytics |
| A | GET | `/inside/businesses/50/settings` | 404 | True | False | settings |
| A | PUT | `/inside/businesses/50` | 403 | True | False | business mutate |
| B | GET | `/inside/businesses/48` | 403 | True | False | business detail |
| B | GET | `/inside/businesses/48/bills` | 403 | True | False | bills list |
| B | GET | `/inside/businesses/48/menu` | 403 | True | False | menu |
| B | GET | `/inside/businesses/48/staff` | 403 | True | False | staff |
| B | GET | `/inside/businesses/48/reservations` | 403 | True | False | reservations |
| B | GET | `/inside/businesses/48/analytics/overview` | 404 | True | False | analytics |
| B | GET | `/inside/businesses/48/settings` | 404 | True | False | settings |
| B | PUT | `/inside/businesses/48` | 403 | True | False | business mutate |

## Commands

```bash
# against a local stack with demo data
# 1) admin creates invite-batch
# 2) POST /auth/register with invite_code + @example.invalid email
# 3) POST /auth/email/verify
# 4) create business for B; run matrix as A against B ids and reverse
PLAYWRIGHT_RUN_QA_PIPELINE=1 npx playwright test --config=playwright.qa-pipeline.config.ts tenant-isolation.spec.ts --project=chromium
```
