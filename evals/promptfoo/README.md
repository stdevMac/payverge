# Payverge Model Benchmark

Promptfoo suite that benchmarks candidate LLMs (`google/gemini-2.5-flash` — the
production default — vs `openai/gpt-4o-mini`) across Payverge's AI surfaces, using
the **real production prompts** over OpenRouter.

## What makes this faithful

Each surface runs through `lib/prod_prompts.js`, a prompt-function module that:

1. **Loads the actual backend prompt asset** (`backend/internal/services/prompts/...`,
   `backend/internal/guardrails/prompts/classifier.md`) — not a hand-maintained
   copy that can drift.
2. **Reproduces the production message shape**: a system prompt **plus the user's
   real message as a separate `user` turn**. (Earlier versions of this suite sent
   the system prompt only and never delivered the question, so every non-guardrails
   surface was graded on an empty input — pass rates were noise.)
3. **Mirrors per-surface production behavior**: `data_block` spotlighting for the
   waiter, the review-pending → English-body fallback for unreviewed locales
   (`ja`, `ar`), the director's owner-question-plus-context-JSON user turn with a
   trailing language anchor, and the guardrails `surface / language_hint / message`
   user turn.
4. **Uses each surface's production model params** (temperature / max_tokens /
   JSON response format), which is why surfaces are **separate config files** — a
   single unified config cannot carry guardrails' `temp 0 / max_tokens 80 / JSON`
   alongside the waiter's `temp 0.4 / max_tokens 1024`.

### Deliberate, documented deviations

- **No tools.** The waiter's `add_to_cart` tool and the director's analytics
  tool-loop are omitted. promptfoo grades conversational **text**; a tool-call
  response has empty text and would corrupt the language/safety/accuracy rubrics
  that are the scorecard. The system prompts (which drive those behaviors) are
  reproduced verbatim.
- **Director is single-shot** over the provided context JSON (the first loop
  iteration), since promptfoo cannot drive the multi-iteration tool loop. That is
  exactly the request we compare models on: JSON validity + language fidelity +
  grounded analysis.
- The `data_block` spotlighting marker is a fixed constant (production randomizes
  it; the value never changes model behavior).

## Not run in CI

This suite is manual. It calls live models through OpenRouter, so it needs
`OPENROUTER_API_KEY`, costs money on every run and its scores vary between
runs. No workflow runs it, and the root `promptfooconfig.yaml` is a manual
entry point too. The hermetic AI gate in CI is the recorded backend `llmeval`
suites (the "AI suites (offline)" job in `.github/workflows/ci.yml`).

## Quick start

```bash
# Run every surface and print the model × surface scoreboard (~3-5 min, ~$0.40)
./evals/promptfoo/runner.sh

# Run one surface
./evals/promptfoo/runner.sh waiter-ordering
./evals/promptfoo/runner.sh director
./evals/promptfoo/runner.sh guardrails

# Re-print the scoreboard from the last run (no API calls)
./evals/promptfoo/runner.sh scoreboard

# Open the per-test web dashboard (read actual outputs, filter by assertion)
./evals/promptfoo/runner.sh view
```

`OPENROUTER_API_KEY` must be set in your environment.

## Surfaces

| Surface (config)            | Prompt asset                                   | Model vs gpt-4o-mini      | Params                         |
|-----------------------------|------------------------------------------------|---------------------------|--------------------------------|
| `waiter-ordering`           | `ai_waiter/ordering_<lang>.md`                 | gemini-2.5-flash          | temp 0.4, 1024                 |
| `waiter-concierge`          | `ai_waiter/concierge_<lang>.md`                | gemini-2.5-flash          | temp 0.4, 1024                 |
| `director`                  | `director_console/<lang>.md`                   | gemini-2.5-flash          | temp 0.2, 4096, JSON           |
| `wizard`                    | `menu_wizard/<lang>.md`                         | gemini-2.5-flash          | temp 0.6, 4096, JSON           |
| `guardrails`                | `guardrails/prompts/classifier.md`             | gemini-2.5-flash-**lite** | temp 0, 80, JSON               |

Languages: waiter surfaces cover en/es/es-AR/ja/ar; director and wizard ship
en/es/es-AR prompts; guardrails is language-agnostic (tested with multilingual
inputs).

> **Not covered:** the menu-extraction surface (its prompt is built inline in Go,
> with no `.md` asset to load) and the WhatsApp waiter variant. Add them later if
> the model pick needs to account for them.

## Scoring

The scoreboard reports pass rate per surface for each model and an overall total
plus average latency. The per-test dashboard (`runner.sh view`) is where you read
the actual model outputs and see which assertion each failure tripped.

| Criterion          | How it's measured                                                   |
|--------------------|---------------------------------------------------------------------|
| Language fidelity  | `llm-rubric` "entirely in <language>" across en/es/es-AR/ja/ar       |
| Schema adherence   | `javascript` JSON-shape asserts (director, wizard) + `json_object`   |
| Safety             | guardrails category asserts; waiter allergen `not-contains nut-free` |
| Content accuracy   | rubrics grounded in the fixture data (real item names, numbers)      |
| Steering           | off-topic / injection rubrics + `not-contains data_block`            |

## Tier-2 sweep

After picking the winning model on the 5-language benchmark, confirm it across
every supported guest locale:

```bash
npx --yes promptfoo@0.120.19 eval -c evals/promptfoo/configs/waiter-ordering-full.yaml
```

Use the release `runner.sh` pins; the gotchas below are specific to 0.120.x.

## Gotchas (promptfoo 0.120.x)

- JavaScript assertions must use the `|` multiline block form, never a quoted
  single line.
- `llm-rubric` assertions go **last** in an assertion list and need a grader
  provider (`defaultTest.options.provider`).
- Avoid mixing `contains-json` with `javascript` — `contains-json` mutates the
  output for later assertions.
- `file://` prompt-function paths are resolved relative to the **config file**'s
  directory (hence `file://../lib/prod_prompts.js:fn`).
- Editing a `backend/.../prompts/*.md` file changes the benchmark automatically —
  that is the point. Keep production prompts and the benchmark in lockstep.
