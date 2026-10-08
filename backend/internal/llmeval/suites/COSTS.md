# LLM eval suites — live-run cost estimates

Offline replay (`make eval-offline` / `go test ./internal/llmeval/... -run TestEvalOffline`)
is **free** — it replays committed fixtures with zero network.

Live runs (`go run ./cmd/llmeval -suite <name>`) and the nightly red-team CI job
call OpenRouter and cost real tokens. Token counts below are the recorded
totals from the committed fixtures (prompt + completion); every `judge_rubric`
case makes a SECOND provider call (the judge), so a suite's live calls =
candidate cases + judge_rubric assertions. Judge-rubric counts per suite:
allergen_redteam=3, injection_redteam=20, exact_names=4, director_quality=5
(locale_adherence and json_adherence have none).

| Suite | Cases | Live calls | Recorded total tokens | Est. cost/run* |
|---|---|---|---|---|
| allergen_redteam | 25 | 28 (25 candidate + 3 judge) | ~6,610 | < $0.01 |
| injection_redteam | 20 | 40 (20 candidate + 20 judge) | ~8,600 | < $0.01 |
| locale_adherence | 10 | 10 (no judge) | ~2,500 | < $0.01 |
| exact_names | 5 | 9 (5 candidate + 4 judge) | ~1,880 | < $0.01 |
| json_adherence | 6 | 6 (no judge) | ~1,680 | < $0.01 |
| director_quality | 5 | 10 (5 candidate + 5 judge) | ~2,000 | < $0.01 |

\* Cost = (prompt_tokens × input_price + completion_tokens × output_price) for
`google/gemini-2.5-flash`. Confirm current per-1M-token input/output prices at
https://openrouter.ai/models before quoting. As of model refresh (Task 9) the
full suite of six is a small-dollar run; the nightly CI job only runs the two
red-team suites (allergen + injection) to bound recurring cost.

## When to run live
- **Nightly CI** (`.github/workflows/llm-evals.yml`): allergen + injection only.
- **Before a model bump** (plan Task 9): all four threshold suites
  (allergen 100%, injection ≥95%, locale ≥95%, json ≥98%) on the candidate model.
- **After a prompt change** (Lane B/C/D): re-record fixtures + re-run the
  affected suite live, then commit the updated fixtures.
