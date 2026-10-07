# Providers

Every AI feature reaches a model through one Go interface and one client.
This page explains that seam for contributors. To configure a provider on a
server, with recipes for OpenRouter, Ollama, vLLM, LiteLLM and hosted
vendors, read [self-hosting/ai.md](../self-hosting/ai.md).

## The seam

[`llm.Provider`](../../backend/internal/llm/types.go) has one method:

```go
type Provider interface {
    Generate(ctx context.Context, req GenerateRequest) (*Response, error)
}
```

A `GenerateRequest` carries the messages, tools, an optional JSON response
schema, `MaxTokens`, the model and its fallbacks, and the **feature** label
(`waiter`, `director`, `guardrail` and so on). The feature decides the
timeout, the privacy class and the budget scope. A `Response` carries text,
tool calls, generated images, the model that actually answered and the token
usage.

Services above the seam (the AI waiter, the director, menu AI, the agent
loop used by the ops assistant) depend only on this
interface. Tests replace it with fixture providers. See [evals.md](evals.md).

## The one implementation

[`openrouter.Provider`](../../backend/internal/llm/openrouter/provider.go)
speaks OpenAI Chat Completions. Despite its name it serves two modes:

| | OpenRouter (default) | OpenAI-compatible (`LLM_BASE_URL` set to another host) |
|---|---|---|
| Endpoint | `https://openrouter.ai/api/v1` | `LLM_BASE_URL`, e.g. `http://ollama:11434/v1` |
| Key | `LLM_API_KEY`, else `OPENROUTER_API_KEY` | `LLM_API_KEY` only; `OPENROUTER_API_KEY` is never sent. With no key, no `Authorization` header is sent. |
| Model fallbacks | Sent as `models: [...]`; OpenRouter fails over | Not sent; only the primary model is called |
| Zero-data-retention routing | Sent and validated at startup | Not sent. A local endpoint skips the check; a hosted one logs a warning that `OPENROUTER_ZDR_MODE=enforce` cannot be honored. |
| Prompt-cache hints, `image_config`, ranking headers | Sent | Stripped |
| Unpriced model | Startup error in production | Warning at startup. Booked at $0 on a local endpoint; refused while a budget cap applies on a hosted one. See [Unpriced models](#unpriced-models). |

The endpoint resolution is in
[config/llm.go](../../backend/internal/config/llm.go). The provider config and
the startup checks are in
[oss_llm.go](../../backend/cmd/app/oss_llm.go). The provider is built once in
[main.go](../../backend/cmd/app/main.go) (search `openrouter.New`) and
shared by every surface.

### Endpoint classes

Startup sorts the configured endpoint into one of three classes, because
pricing and privacy rules differ between them
([`config.LLMBaseURLIsLocal`](../../backend/internal/config/llm.go)):

| Class | When | Unpriced model |
|---|---|---|
| OpenRouter | `LLM_BASE_URL` unset, or a host under `openrouter.ai` | A missing price for any configured primary or fallback model stops startup in production and is a warning in development |
| Local | `LLM_BASE_URL` host is a loopback, private-range or link-local IP, `localhost`, a single-label name such as `ollama` or `litellm`, or ends in `.localhost`, `.local`, `.internal`, `.lan`, `.home.arpa` or `.svc` | Startup warning; the call is booked at **$0**, so budget caps cannot trip on it |
| Hosted | Any other public host, such as `https://api.openai.com/v1` or a hosted LiteLLM | Startup warning; the call is **not** free. While a budget cap applies (production always has one) the call is refused |

### Unpriced models

Give every model you configure a price in `OPENROUTER_PRICES`
(`model=in/out`, dollars per million tokens; `0/0` is a valid price) unless
you accept $0 accounting on a local endpoint. Under Docker Compose, also
check that the variable reaches the backend; see
[Passing variables through Compose](#passing-variables-through-compose).

On a hosted endpoint, a vendor often reports a dated snapshot of the model
you asked for, such as `gpt-4o-mini-2024-07-18` for `gpt-4o-mini`. That call
is billed under the id you configured. Any other reported model id counts as
unpriced: the first such call sets a durable, cluster-wide shutdown latch, and
every later budget-capped call refuses with `ErrUnpricedModel`. After you add
the price, clear the latch:

```sql
UPDATE ai_budget_controls SET unpriced_model_shutdown = false WHERE id = 1;
```

The same latch guards OpenRouter, where it trips when a fallback serves a
model missing from the table. [cost-and-budgets.md](cost-and-budgets.md)
covers the ledger.

### Passing variables through Compose

The backend reads `OPENROUTER_PRICES`, the model ids and the budget
variables from its own process environment. Compose hands a variable from
`.env` to a container only when the compose file names it for that service.
A value the compose file does not list never reaches the backend, and
nothing reports that it was dropped: an unpriced hosted model then stays
refused even though `.env` prices it.

From the directory that holds `docker-compose.yml`, list what the backend
will actually receive:

```sh
docker compose config backend | grep -E 'OPENROUTER_PRICES|OPENROUTER_MODEL|AI_BUDGET|AI_DAILY_BUDGET'
```

If a variable you set is missing, add it in a `docker-compose.override.yml`
next to `docker-compose.yml`. Compose merges that file automatically, and an
upgrade of the [deploy stack](../../deploy/README.md) does not overwrite it:

```yaml
services:
  backend:
    environment:
      - OPENROUTER_PRICES=${OPENROUTER_PRICES:-}
      - AI_BUDGET_PUBLIC_USD_DAY=${AI_BUDGET_PUBLIC_USD_DAY:-}
```

Then recreate the backend with `docker compose up -d backend`.

### What `Generate` does

In order:

1. **Privacy class.** `ApplyFeaturePrivacy` looks up the feature in
   [privacy.go](../../backend/internal/llm/privacy.go). An unknown feature
   is refused before any spend or network call. On OpenRouter, sensitive
   features (guest chat, owner data) also require zero-data-retention routing.
2. **Budget reservation** through `WithBudget`. See
   [cost-and-budgets.md](cost-and-budgets.md#how-a-call-is-charged).
3. **The request**, with up to 2 retries for rate limits and `5xx` errors,
   with full-jitter backoff from 250 ms up to 2 s
   ([retry.go](../../backend/internal/llm/openrouter/retry.go)). Client errors
   are not retried.
4. **Settle** the reservation and report the call to the observer, which
   feeds metrics.

## Models

[`llm.LoadModelConfig`](../../backend/internal/llm/config.go) reads one model
per role. The variables keep their `OPENROUTER_` prefix for compatibility,
but the value is passed as-is to whichever endpoint is configured.

| Variable | Default | Used by |
|---|---|---|
| `OPENROUTER_MODEL_CHAT` | `google/gemini-2.5-flash` | AI waiter (web and WhatsApp) |
| `OPENROUTER_MODEL_MENU` | `google/gemini-2.5-flash` | Menu extraction, setup wizard |
| `OPENROUTER_MODEL_IMAGE` | `google/gemini-2.5-flash-image` | Dish and marketing images |
| `OPENROUTER_MODEL_DIRECTOR` | `google/gemini-2.5-flash` | Director, ops assistant |
| `OPENROUTER_MODEL_GUARDRAIL` | `google/gemini-2.5-flash-lite` | Input classifier |

Each has a comma-separated `OPENROUTER_<ROLE>_FALLBACKS` list, which only
OpenRouter uses.

A model needs **tool calling** for the waiter, director and ops assistant;
**JSON-schema output** for the guardrail, extraction and wizard;
**vision** for menu extraction; and an OpenRouter-style `images` response
field for image generation.

## No provider

When neither an OpenRouter key nor a valid `LLM_BASE_URL` is set, startup logs
why, does not build the provider and leaves the AI services unset. Each
surface then degrades as described in
[README.md § What happens with no model configured](README.md#what-happens-with-no-model-configured).
`GET /api/v1/instance` reports `features.ai: false`. The frontend does not
read that flag yet, so AI entry points stay visible and degrade as that table
describes when used. Hiding them is planned work; see
[README.md § Known gaps](README.md#known-gaps-honest-list).

## Adding another provider

Implement `llm.Provider` and construct it in place of `openrouter.New`. Most
self-hosters do not need to: an OpenAI-compatible endpoint, or a LiteLLM
proxy in front of any vendor, already works through compatible mode.

A new implementation must also carry the duties `openrouter.Provider`
performs inside `Generate`: the privacy-class check, the budget
reserve/finalize/release, and the observer callback. They are not a separate
wrapper today, so a provider that skips them runs **without** dollar ceilings.

## Known limitations

- **Budget enforcement lives inside the client.** See above. Moving it into a
  decorator around any `Provider` would make the seam safer to extend.
- **Image generation assumes OpenRouter's response shape.** Most self-hosted
  servers do not return images in that field.
- **Compatible mode has no fallbacks.** If the one configured model is down,
  the call fails and the surface uses its deterministic fallback.
