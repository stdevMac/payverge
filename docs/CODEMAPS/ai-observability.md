# AI Observability & Eval Discipline

Token-lean reference for reading LLM telemetry, running the eval suites, and
keeping prompts honest with regression evals. Companion to `docs/CODEMAPS/backend.md`.

## 1. Reading the observer logs (per-call telemetry)

Every `llm.Provider.Generate` call emits one observer event (success or error).
The default observer is a structured `slog` line wired in `cmd/app/main.go`
(contract C1, Lane A). The event carries `llm.CallInfo`
(`backend/internal/llm/observe.go`):

| Field | Meaning |
|---|---|
| `Feature` | call site: `waiter`, `waiter_whatsapp`, `director`, `wizard`, `extraction`, `image`, `guardrail` |
| `Model` | the requested primary model |
| `ServedModel` | the model that actually answered (fallback-aware) — diverges from `Model` when a fallback fired |
| `InputTokens` / `OutputTokens` | prompt / completion tokens from the provider payload |
| `Latency` | wall-clock for the call |
| `Err` | nil on success; otherwise the classified error (`ErrRateLimited`, `ErrUpstream`, `ErrAuth`, `ErrMalformedResponse`) |
| `BusinessID` | business context (0 = unknown/aggregate) |
| `EstimatedCostUSD` | filled by `AnnotateCost` from the static price table |

### What to look for
- **Cost per feature/business**: group by `Feature` (+ business_id if the slog
  line includes it) and sum `InputTokens`+`OutputTokens`. The waiter re-sends the
  menu JSON on every message, so `waiter` dominates token spend — the prompt-cache
  work (Lane J) targets exactly this.
- **Fallback rate**: `ServedModel != Model` means the primary failed and a
  fallback answered. A rising fallback rate on `waiter`/`director` is an upstream
  health signal (and a cost signal if the fallback is pricier).
- **Abuse / wallet attacks** (P0-3): a single business with anomalous `waiter`
  call volume on the public endpoint. Cross-reference per-business budgets (Lane B).
- **Latency p95**: `director` should sit under its loop wall-clock (Lane C raised
  it 12s→35s); spikes correlate with fallbacks.

### Example queries (structured-log backend)
```
# tokens by feature, last 24h
feature=waiter OR feature=director | stats sum(InputTokens), sum(OutputTokens) by Feature
# fallback events
ServedModel != Model | stats count by Feature, ServedModel
```

## 2. The eval workflow

Six production suites live in `backend/internal/llmeval/suites/` (contract C9):

| Suite | Guards | Acceptance |
|---|---|---|
| `allergen_redteam` | P0-1 allergen safety (never guarantee, always confirm-with-staff, refuse on missing data) | 100% |
| `injection_redteam` | P0-2 prompt injection / spotlighting | ≥95% |
| `locale_adherence` | P2-7 response-language fidelity | ≥95% |
| `exact_names` | menu fidelity / no hallucinated items | tracked |
| `json_adherence` | P1-4 director + wizard strict-schema validity (incl. date survival) | ≥98% |
| `director_quality` | §4-F relevance / on-topic (judge_rubric) | tracked |

### Offline (free, hermetic — CI default)
```bash
cd backend && make eval-offline                    # all suites, fixture replay, no network
go test ./internal/llmeval/... -run TestEvalOffline # same, explicit
```
Fixtures are committed under `suites/testdata/fixtures/<suite>/<key>.json`. CI
runs this on every pull request and push to main
(`.github/workflows/ci.yml`, `ai-offline` job).

### Live (costs tokens — manual or nightly CI)
```bash
cd backend && OPENROUTER_API_KEY=... go run ./cmd/llmeval -suite allergen_redteam
OPENROUTER_API_KEY=... go run ./cmd/llmeval -suite injection_redteam -record  # re-seed fixtures
```
The nightly CI job runs `allergen_redteam` + `injection_redteam` live at 07:00
UTC, posts the report tables to the job summary, and fails on any regression.
Per-suite cost estimates: `backend/internal/llmeval/suites/COSTS.md`.

### Promptfoo (live/manual model checks)
Promptfoo coverage lives in two layers:

```bash
npm run ai:eval:promptfoo:validate     # validates root smoke config
OPENROUTER_API_KEY=... npm run ai:eval:promptfoo
OPENROUTER_API_KEY=... npm run ai:eval:promptfoo:full
```

- `promptfooconfig.yaml` is the top-level smoke suite for quick checks across
  waiter safety, injection resistance, exact menu names, locale naturalness,
  Director JSON, and menu-wizard JSON.
- `evals/promptfoo/configs/*.yaml` is the production-faithful benchmark suite:
  it loads the real backend prompt assets through `evals/promptfoo/lib/prod_prompts.js`
  and uses per-surface production params. Use this for model comparisons and
  larger release confidence.
- The scripts retain `promptfoo@0.120.19` for reproducible eval output. The
  repository runtime is `.nvmrc` Node 22.22.0; evaluate promptfoo upgrades as a
  separate dependency change with the smoke and production-faithful suites.

## 3. Regression-eval discipline (add a case when prod breaks)

When a production AI issue is found (a bad allergen answer, an injection that
worked, a wrong-language reply, a mangled date), the fix is **not done until a
failing eval case reproduces it**. Workflow:

1. **Write the case** in the matching suite YAML. Use the exact prod prompt
   wording in `system` and the offending guest message in `user`. Add the
   assertion(s) that SHOULD have caught it (e.g. `contains_none: ["guaranteed safe"]`,
   `regex_absent: "redacted-phone"`, `language_is: <locale>`).
2. **Confirm it fails** on the current prompt — record the live response that
   reproduces the bug (`go run ./cmd/llmeval -suite <name>`).
3. **Fix the prompt** (Lane B/C/D owns the prompt assets) until the live run passes.
4. **Re-record the fixture** (`-record`) so the offline replay is green, and
   commit the new case + fixture together.
5. The case now guards against the regression forever — the offline CI gate runs
   it on every prompt-touching PR.

### Authoring rules
- One case per file is NOT required (suites are single YAML files with a `cases:`
  list); give each case a unique, descriptive `id`.
- NEVER weaken an assertion to make a fixture pass. A failing safety assertion
  on a live model is a real regression, not a fixture bug.
- Safety suites (`allergen_redteam`) are zero-tolerance: a single new failure
  blocks the model/prompt change.

## 4. Where things live
- Harness: `backend/internal/llmeval/` (Lane H0) — `Case`, `Grade`, `RunSuite`,
  `RunJudge`, `NewFixtureProvider`, `langid.Detect`.
- Suites + fixtures: `backend/internal/llmeval/suites/` (Lane H1).
- Live runner: `backend/cmd/llmeval/main.go`.
- CI: `.github/workflows/ci.yml` (`ai-offline` job; fixture replay only, no live evals).
- Model defaults: `backend/internal/llm/config.go` (`OPENROUTER_MODEL_*` env overrides).
- Observer: `backend/internal/llm/observe.go` + wiring in `cmd/app/main.go`.
