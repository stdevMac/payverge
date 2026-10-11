# AI in Payverge

Payverge has five AI surfaces. Each one keeps working, in a reduced form or
with an explicit "not configured" answer, when no language model is
configured. Every claim below links to the code that implements it. If code
and docs disagree, the code wins. Please file the drift as a bug.

> **Scope of this page.** It explains how the AI features are built, for
> contributors and reviewers. To turn AI on for a server, read
> [self-hosting/ai.md](../self-hosting/ai.md).

## The surfaces

| Surface | Who uses it | Entry point | Model needed? | Page |
|---|---|---|---|---|
| **AI waiter** | Restaurant guests, from the table QR page | `POST /api/v1/ai-waiter/:businessId` in [main.go](../../backend/cmd/app/main.go), handled by [ai_waiter_handler.go](../../backend/internal/server/ai_waiter_handler.go) | No. Most turns are answered from a server-side menu snapshot. The model is called only for ordering turns. | [ai-waiter.md](ai-waiter.md) |
| **Director console** | Owners | `/api/v1/inside/businesses/:id/ai/director/*`, served by [director_console_service.go](../../backend/internal/services/director_console_service.go) | For free-form chat, yes. Briefings, insights, threads and the propose → apply → undo rail work without one. | [director-console.md](director-console.md) |
| **Ops assistant** | Owners and staff | `/api/v1/inside/businesses/:id/assistant/*`, served by [ops_service.go](../../backend/internal/agents/ops_service.go) | Yes. Without a provider it answers `503 ai_not_configured`. | [ops-assistant.md](ops-assistant.md) |
| **Menu AI** (extraction, setup wizard, dish images) | Owners during onboarding | `/api/v1/inside/businesses/:id/ai/*`, served by [menu_ai_service.go](../../backend/internal/services/menu_ai_service.go) | Yes | [menu-ai.md](menu-ai.md) |

Two more LLM call sites are not described as separate surfaces:

- the **input guardrail classifier**, covered in [guardrails.md](guardrails.md);
- the **WhatsApp waiter**, which is compiled only with the `whatsapp` build tag.
  See [ADR 0008](../adr/0008-apache-2-with-whatsapp-behind-gpl-build-tag.md) and
  [self-hosting/whatsapp.md](../self-hosting/whatsapp.md).

## Cross-cutting pages

- [guardrails.md](guardrails.md): input classification, PII redaction,
  allergen and alcohol handling, prompt spotlighting and the prompt-leakage
  invariant.
- [cost-and-budgets.md](cost-and-budgets.md): the micro-USD spend ledger, the
  reservation-based cost gate, daily budgets and guest quotas.
- [providers.md](providers.md): the `llm.Provider` seam, the OpenRouter client,
  and any OpenAI-compatible endpoint through `LLM_BASE_URL`, Ollama included.
- [evals.md](evals.md): hermetic contract tests (`aicontract`), fixture-replay
  evals (`llmeval`) and live promptfoo benchmarks.

## The one design rule

Model output is **untrusted**. Across every surface, the model may phrase an
answer. It may not decide what is true, what is executable, or what changes
in the database.

- **Facts come from the server.** Waiter answers are assembled by
  [`FinalizeWaiterV2`](../../backend/internal/server/ai_waiter_v2.go) from a
  server-built menu snapshot. Its doc comment states the rule: *"Raw model
  prose never becomes a menu fact, entity, source, action, or transaction
  claim."* The ops assistant follows the same rule in `FinalizeOpsV2`
  ([ops_finalizer.go](../../backend/internal/agents/ops_finalizer.go)): model
  actions and URLs are ignored.
- **Actions are validated against server state.** A waiter `add_to_cart` call
  is rebuilt from the menu snapshot by
  [`validateCartToolCalls`](../../backend/internal/server/ai_waiter_tool_validation.go)
  before it can reach the guest or the database.
- **Writes are proposals.** The director can only stage a proposal. A human
  applies it, and the apply is guarded by optimistic locking on the menu
  version. See [director-console.md](director-console.md).
- **The shared response contract** for all assistants is in
  [`internal/assistantcontract`](../../backend/internal/assistantcontract/). It
  defines the response envelope, actions, sources and status, and every
  surface validates against it before replying.

## What happens with no model configured

A server with no LLM provider (see [providers.md](providers.md)) still runs
every surface that can work without one. Routes that cannot work without a
model answer `503` with `code: "ai_not_configured"` and a message telling
the operator to configure a provider. There is no plan gate: every business
gets every AI surface once a provider is set.

| Surface | No provider configured |
|---|---|
| AI waiter | Answers from the menu snapshot through the same deterministic finalizers. Guest business projections report `ai_waiter_mode: "basic"` (`"llm"` when a provider is wired). |
| WhatsApp waiter | Replies with a short localized "AI unavailable" notice. Unlike the web waiter it has no basic mode; see [ai-waiter.md § WhatsApp](ai-waiter.md#whatsapp). |
| Director console | Data endpoints (briefing, insights, threads, actions, `propose-price-change`) keep working. Chat (`ask`, `ask/stream`) returns `503 ai_not_configured`. |
| Ops assistant | `503 ai_not_configured`. The service is constructed only when a provider exists. |
| Menu AI | `503 ai_not_configured` on extraction, wizard, dish-image and marketing-image routes. The marketing caption route answers `200` with `ai_available: false`. |

The `503` helper and the `ai_waiter_mode` projection live in
[ai_entitlements.go](../../backend/internal/server/ai_entitlements.go).
`GET /api/v1/instance` reports `features.ai`. No frontend screen reads that
flag yet, so AI entry points stay visible and the routes above answer as the
table describes. See [Known gaps](#known-gaps-honest-list). The services are
constructed in
[main.go](../../backend/cmd/app/main.go) only when a provider is
configured: search for `NewMenuAIService`.

## Known gaps (honest list)

- **The UI only partly hides AI when no model is configured.** The dashboard
  sidebar drops its AI tabs, but other entry points remain: the overview still
  shows a "Today's briefings" card that names Sage, and guests still see the
  Sage chat bubble, labelled as an AI assistant, which answers from the basic
  waiter. An operator who reaches an AI endpoint gets `503 ai_not_configured`.
  Gating the remaining surfaces on `features.ai` is planned work.
- **Most locales never reach the model.** Waiter cart intent is detected only
  in en, es and es-AR. See [ai-waiter.md § Locales](ai-waiter.md#locales).
- **18 of 21 locales have unreviewed waiter prompts.** Their machine-translated
  prompt files carry `review_pending: true`. Until a native speaker clears
  that flag,
  [`resolveWaiterPrompt`](../../backend/internal/services/waiter_prompts.go)
  serves the English prompt body and tells the model to answer in the guest's
  language.
- **The WhatsApp waiter is less guarded than the web one.** Its model reply
  is sent as prose with only an allergen note added: no deterministic
  answers, no menu-checked prices and no basic mode without a model. See
  [ai-waiter.md § WhatsApp](ai-waiter.md#whatsapp).
- **The SSE hub is in-process.** AI-waiter live updates do not fan out across
  backend replicas. See [architecture/realtime.md](../architecture/realtime.md).
