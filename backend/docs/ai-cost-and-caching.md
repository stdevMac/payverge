# AI cost & prompt caching (Lane J)

## Decision record: implicit vs explicit caching (verified 2026-06-06)
- Gemini **2.5 Flash/Pro** on OpenRouter support **implicit (automatic) caching** — no `cache_control` required. Minimum **1024 tokens** (2.5 Flash) to be cache-eligible; cached input billed at **0.25x**; TTL ~3–5 min. Maximize hits by keeping the **front** of the message array identical across turns (the per-business system prompt + menu) and pushing variation to the end. Lane B's static-first prompt ordering does exactly this; `TestWaiterStaticPrefixExceedsCacheMinimum` guards the 1024-token floor and `TestWaiterStaticContentPrecedesVariableContent` guards that per-request variable content (bill context) follows the stable menu region so the cacheable prefix is identical across turns.
- Gemini **2.0 Flash** (current `OPENROUTER_MODEL_CHAT` default) is **not** an implicit-cache model. Real savings land once Lane H1 eval-gates the bump to `google/gemini-2.5-flash`.
- We also ship an **explicit** `cache_control: {type:"ephemeral"}` breakpoint on the waiter system message (`GenerateRequest.CacheControl=true`). It is a no-op on models that ignore it and marks the static prefix cacheable on models that honor explicit breakpoints — belt-and-suspenders so the win is captured the moment the model flips.

## Cache usage reporting
- OpenRouter returns cache reads in `usage.prompt_tokens_details.cached_tokens`. The provider maps it to `llm.Usage.CachedTokens`; the observer annotates `EstimatedCostUSD` using the 0.25x cache-read factor.

## Cost price table (env-overridable)
- Static defaults live in `internal/llm/cost.go` (USD per 1,000,000 tokens). Override without a deploy:
  `OPENROUTER_PRICES="google/gemini-2.5-flash=0.30/2.50,google/gemini-2.0-flash-001=0.10/0.40"` (model=in/out).
- The observability estimate reports unknown models as 0 rather than inventing a price. Budget enforcement is deliberately stricter: an unknown served model consumes the full conservative reservation and activates the durable shutdown described below. Numbers for known models remain directional — confirm against OpenRouter's model pages before relying on them for billing.

## Durable budget enforcement and unpriced-model shutdown
- Every restaurant-owned request carries its numeric `BusinessID`; anonymous/global calls remain unattributed, while a restaurant-only feature with a missing ID fails closed with `ErrMissingBusinessID` before provider spend.
- PostgreSQL reservations are created before the provider call. After provider success, actual usage is first persisted as `status=consumed`, then transferred to the daily ledger. If that transfer fails or a replica crashes, the hourly reconciler finalizes the recorded actual; an ambiguous stale `reserved` call is charged at its conservative maximum instead of being released.
- If OpenRouter serves a model absent from the checked-in/override price table, the reservation is charged at its full conservative amount and `ai_budget_controls.unpriced_model_shutdown` is set atomically. Every replica checks that durable latch before new capped calls, so a restart cannot bypass the shutdown.
- Recovery is intentionally manual: verify and add the served model's current price, deploy the updated pricing configuration, then clear the singleton latch with `UPDATE ai_budget_controls SET unpriced_model_shutdown = false, shutdown_reason = '', updated_at = now() WHERE id = 1;`. Never clear it before the model is priced.

## Per-call observability
- The default slog observer (the closure in `cmd/app/main.go`, authored by Lane A and extended by Lane J) logs one `llm_call` line per Generate call: `feature, model, served_model, input_tokens, output_tokens, latency_ms, error_class, business_id, estimated_cost_usd`.
- `estimated_cost_usd` is populated by `llm.AnnotateCost(&ci, resp.Usage.CachedTokens)`, called inside `openrouter/retry.go`'s `Generate` immediately before the observer fires — the only live `CallInfo` build site. `business_id` is copied there from `GenerateRequest.BusinessID` (set by the waiter caller; threaded from the Lane-B handler's `businessID`).
- Daily per-business roll-up: `llm.DailyCostRollup` accumulates `Add(businessID, feature, costUSD)` (fed from the same observer closure) and `Flush()` emits sorted `CostLine{BusinessID, Calls, TotalUSD, ByFeature}`. `main.go` runs a 24h scheduler tick that `Flush()`es and logs one `llm_cost_daily` line per business (`business_id, calls, total_usd, by_feature`). Lane H1 owns surfacing these on a dashboard.

## Live before/after measurement (manual)
Run a scripted 10-turn conversation against a fixture business and read `cached_tokens` per turn:
```bash
# 1. Point chat at an implicit-cache model:
export OPENROUTER_API_KEY=sk-...           # real key
export OPENROUTER_MODEL_CHAT=google/gemini-2.5-flash

# 2. Send 10 turns to the same session against a seeded fixture business and a
#    large menu; the static prefix (system prompt + menu) must exceed 1024 tokens.
#    Use the public waiter endpoints (session + chat) from C4 (Lane B):
#      POST /api/v1/ai-waiter/:businessId/session
#      POST /api/v1/ai-waiter/:businessId/chat   (10x, same session_token)
#    For each response, log resp.Usage: PromptTokens vs CachedTokens.

# 3. Expected pattern (gemini-2.5-flash): turn 1 cached_tokens=0 (cache warm);
#    turns 2..10 cached_tokens ~= the static prefix token count (>=1024), so
#    billed input on later turns drops to ~uncached_tail + 0.25*prefix.
```
Record per-turn `prompt_tokens` and `cached_tokens` in the PR/Evidence section. Repeat 3x and average (the perf-gate `-count=3` spirit) since live cache state varies (3–5 min TTL).

## Offline proof (CI-safe, no network)
- `go test ./internal/services/... -run TestTenTurnInputTokenModel -v` models the input-token totals across 10 turns and asserts cacheable-prefix reuse >= 80%.
- `go test ./internal/services/... -bench BenchmarkWaiterBuildRequest -benchmem -count=3` proves the cache flag adds no measurable allocation.
- `go test ./internal/llm/openrouter/... -run TestCachedTokensParsed` proves `cached_tokens` parsing.
- `go test ./internal/llm/openrouter/... -run TestObserverReceivesEstimatedCostAndBusinessID` proves the live observer receives a non-zero, cache-discounted `EstimatedCostUSD` and the propagated `BusinessID` — i.e. cost instrumentation is wired end-to-end, not just unit-tested.
