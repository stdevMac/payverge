# AI providers (self-hosting)

AI is optional. Payverge runs without any LLM provider: menu, tables, orders,
bills, KDS and payments never call a model, and the guest AI waiter falls back
to its no-key mode. `GET /api/v1/instance` reports `features.ai=false` so the
UI can hide AI-only surfaces.

When you do configure a provider, every AI feature talks to it through one
OpenAI-compatible Chat Completions client. The default is OpenRouter. You can
point it at any OpenAI-compatible endpoint instead: Ollama, vLLM, LiteLLM, or
a hosted vendor.

## Choosing a provider

| Variable | Meaning |
|---|---|
| `LLM_BASE_URL` | OpenAI-compatible base URL, ending in `/v1`, e.g. `http://ollama:11434/v1`. Empty means OpenRouter (`https://openrouter.ai/api/v1`). Must be an absolute `http(s)` URL with a host and no `user:pass@`. An invalid value disables AI and logs why. |
| `LLM_API_KEY` | Bearer key for `LLM_BASE_URL`. Optional for Ollama/vLLM. With no key, no `Authorization` header is sent. |
| `OPENROUTER_API_KEY` | OpenRouter key. Used only when `LLM_API_KEY` is empty **and** the endpoint is OpenRouter. It is never sent to a non-OpenRouter `LLM_BASE_URL`. |

AI is enabled when:

- **OpenRouter** (`LLM_BASE_URL` empty or an `openrouter.ai` host): a key is set.
- **Any other endpoint**: `LLM_BASE_URL` is valid. A key is not required.

Startup sorts a non-OpenRouter endpoint into one of two classes, because
pricing and privacy rules differ between them:

- **Local**: infrastructure you run. That is a loopback, private-range or
  link-local IP; `localhost`; a single-label host such as a compose service
  name (`ollama`, `vllm`, `litellm`); or a private-network suffix
  (`host.docker.internal`, `*.local`, `*.internal`, `*.lan`, `*.home.arpa`,
  `*.svc`).
- **Hosted**: any other public host, such as `api.openai.com`, Together, or
  a LiteLLM proxy you expose on a public domain.

The resolution lives in `backend/internal/config/llm.go` (`LLMIsOpenRouter`,
`LLMBaseURLIsLocal`). Startup wiring is in `backend/cmd/app/oss_llm.go`.

### Recipes

```bash
# OpenRouter (default)
OPENROUTER_API_KEY=sk-or-...

# Ollama running as a compose service named "ollama"
LLM_BASE_URL=http://ollama:11434/v1
OPENROUTER_MODEL_CHAT=qwen2.5:14b
OPENROUTER_MODEL_MENU=qwen2.5:14b
OPENROUTER_MODEL_DIRECTOR=qwen2.5:14b
OPENROUTER_MODEL_GUARDRAIL=qwen2.5:7b

# Ollama on the Docker host (Linux also needs
#   extra_hosts: ["host.docker.internal:host-gateway"] on the backend service)
LLM_BASE_URL=http://host.docker.internal:11434/v1

# vLLM
LLM_BASE_URL=http://vllm:8000/v1
OPENROUTER_MODEL_CHAT=Qwen/Qwen2.5-14B-Instruct

# LiteLLM proxy (routes to any vendor; keeps vendor keys out of Payverge)
LLM_BASE_URL=http://litellm:4000/v1
LLM_API_KEY=sk-litellm-...

# A hosted OpenAI-compatible vendor
LLM_BASE_URL=https://api.openai.com/v1
LLM_API_KEY=sk-...
OPENROUTER_MODEL_CHAT=gpt-4o-mini
```

The model variables keep their `OPENROUTER_` prefix for compatibility, but the
values are passed verbatim to whichever endpoint `LLM_BASE_URL` names. Use the
model id that endpoint expects, e.g. `qwen2.5:14b` for Ollama.

## Models

| Variable | Used by | Default (OpenRouter id) |
|---|---|---|
| `OPENROUTER_MODEL_CHAT` | guest AI waiter, WhatsApp waiter | `google/gemini-2.5-flash` |
| `OPENROUTER_MODEL_MENU` | menu extraction, setup wizard, menu generation | `google/gemini-2.5-flash` |
| `OPENROUTER_MODEL_IMAGE` | dish image generation | `google/gemini-2.5-flash-image` |
| `OPENROUTER_MODEL_DIRECTOR` | operator director console | `google/gemini-2.5-flash` |
| `OPENROUTER_MODEL_GUARDRAIL` | guest input classifier | `google/gemini-2.5-flash-lite` |

Each model also has an optional fallback list, comma-separated:

- `OPENROUTER_CHAT_FALLBACKS`
- `OPENROUTER_MENU_FALLBACKS`
- `OPENROUTER_IMAGE_FALLBACKS`
- `OPENROUTER_DIRECTOR_FALLBACKS`
- `OPENROUTER_GUARDRAIL_FALLBACKS`

**The defaults are OpenRouter ids.** If you set `LLM_BASE_URL`, you must set
every model your features use to an id your endpoint serves.

### What a model must support

- **Tool calling.** The guest waiter and the director use OpenAI
  `tools`/`tool_calls`. Pick a model with reliable function calling, such as
  Qwen 2.5 or Llama 3.1 instruct.
- **Structured output.** The guardrail, menu extraction and wizard request
  `response_format: json_schema`. Servers that ignore it can return free text
  that fails validation. When that happens, the feature returns an error or
  uses its fallback.
- **Vision.** Menu photo extraction sends images as `image_url` content parts,
  so it needs a vision model.
- **Image generation** reads images from the OpenRouter `images` response
  field. Most self-hosted servers (Ollama, vLLM) do not return images there.
  Expect dish image generation to fail off OpenRouter unless a LiteLLM route
  provides it.

## Differences off OpenRouter (compatible mode)

When `LLM_BASE_URL` is not OpenRouter, the client speaks plain OpenAI Chat
Completions and leaves out these OpenRouter-only extensions:

| OpenRouter feature | Compatible mode |
|---|---|
| Server-side fallbacks (`models: [...]`) | Not sent. Only the primary model is called, so the `*_FALLBACKS` lists have no effect. |
| Zero-data-retention routing (`provider` block, `OPENROUTER_ZDR_MODE`, `OPENROUTER_ZDR_APPROVED_MODELS`) | Not sent and not validated. On a **local** endpoint prompts stay on your own hardware. On a **hosted** endpoint the vendor's own retention policy applies: when the ZDR mode is `enforce` (explicitly, or by the production default) startup logs a warning that it cannot be honored. Set `OPENROUTER_ZDR_MODE=audit` to acknowledge, or use OpenRouter or a local endpoint if you need zero data retention. |
| Prompt caching hints (`cache_control`) | Stripped. |
| `image_config` | Stripped. |
| Ranking headers (`HTTP-Referer`, `X-Title`) | Not sent. |

On OpenRouter, the ranking headers default to the instance identity:

- `HTTP-Referer` is `OPENROUTER_APP_URL`, else `PUBLIC_URL` when it is set explicitly.
- `X-Title` is `OPENROUTER_APP_TITLE`, else `PRODUCT_NAME`.

## Cost accounting and budgets

Spend is metered per call from a static price table, in USD per million
tokens. The defaults are in `backend/internal/llm/cost.go`. To add or override
prices, use this format:

```bash
OPENROUTER_PRICES=qwen2.5:14b=0/0,gpt-4o-mini=0.15/0.60
```

Unpriced models are handled differently by endpoint:

- **OpenRouter:** every configured primary and fallback model must be priced.
  A missing price is fatal at startup in production and a warning in
  development.
- **Local endpoint:** a missing price is a startup warning, and those calls
  are booked at **$0**. Budget caps therefore never trip on unpriced
  self-hosted models. To enforce caps, list those models in `OPENROUTER_PRICES`;
  an explicit `0/0` is a valid price.
- **Hosted endpoint:** a missing price is a startup warning, but calls are
  **not** booked at $0. While a budget cap applies (production always has
  one), calls to an unpriced model are refused, so set `OPENROUTER_PRICES`
  for every model you configure, e.g.
  `OPENROUTER_PRICES=gpt-4o-mini=0.15/0.60`. When the vendor reports a dated
  snapshot of the requested model (`gpt-4o-mini-2024-07-18`), the call is
  billed under the id you configured. Any other reported model id is treated
  as unpriced: the first such call sets a durable, cluster-wide fail-closed
  latch that refuses budget-capped AI calls. After pricing the model, clear
  it with
  `UPDATE ai_budget_controls SET unpriced_model_shutdown = false WHERE id = 1;`

Budget caps:

| Variable | Default | Scope |
|---|---|---|
| `AI_DAILY_BUDGET_USD` | `$50` in production when unset; no cap in development | per business, per day |
| `AI_WAITER_DAILY_MESSAGE_BUDGET` | `2000` | guest waiter messages per business, per day |

Leave a cap empty to use its default. Production preflight rejects a cap that
is set but is not a positive decimal, even when no provider is configured.

### Dish image limits

Dish image generation has its own fair-use limits, stored as platform
settings rather than environment variables:

| Setting | Default | Effect |
|---|---|---|
| daily limit | `500` | images one business may generate per UTC day; the next request gets `429 image_daily_limit_reached` |
| monthly alert | `2000` | per-business monthly count that raises an operator alert (Sentry event and the `payverge_ai_image_monthly_alerts_total` metric); never blocks |

Show or change them from the server binary. A change applies to the next
request; no restart is needed. Both values must be positive.

```bash
docker compose exec -T backend /app/server settings image-limits
docker compose exec -T backend /app/server settings image-limits --daily 100 --monthly-alert 1000
```

## Guardrail and timeouts

Guest messages are screened by the guardrail model
(`OPENROUTER_MODEL_GUARDRAIL`) before the waiter model answers them. The
classifier has a hard **2 second** timeout.

`GUARDRAIL_STRICT` decides what happens when the classifier is slow or down:

- `true`: fail closed. Borderline messages are redirected instead of answered.
- `false`: fail open.
- Unset: fail closed in production, fail open in development.

A slow local model plus strict mode means guests see redirects. Use a small,
fast guardrail model, or set `GUARDRAIL_STRICT=false` if you accept unscreened
input.

Per-feature request ceilings are fixed in code (`backend/internal/llm/config.go`)
and are not configurable through the environment:

| Feature | Timeout |
|---|---|
| waiter / WhatsApp waiter | 20 s |
| wizard, ops assistant, marketing | 30 s |
| director | 35 s |
| menu extraction, image | 60 s |

The HTTP client has a 60 s overall timeout. Size your hardware so the chat
model answers well inside 20 s.

## Docker Compose

Both compose files forward every AI variable on this page to the backend:
`LLM_BASE_URL`, `LLM_API_KEY`, `OPENROUTER_API_KEY`, `OPENROUTER_PRICES`,
the `OPENROUTER_MODEL_*` and `*_FALLBACKS` variables,
`OPENROUTER_ZDR_MODE`, `OPENROUTER_ZDR_APPROVED_MODELS`,
`OPENROUTER_APP_URL`, `OPENROUTER_APP_TITLE`, `GUARDRAIL_STRICT` and the
AI budget and abuse ceilings. That covers `deploy/docker-compose.yml` for an
image-based install and the root `docker-compose.yml` for a source
checkout. Set them in the `.env` next to the compose file.

`OPENROUTER_ZDR_MODE` can stay empty. It then means `enforce` in production
mode and `audit` otherwise, and it only matters on OpenRouter:

- **OpenRouter:** leave it empty or set `enforce`. The production preflight
  refuses `audit`.
- **Local endpoint:** ignored. Prompts stay on your own hardware.
- **Hosted endpoint that is not OpenRouter:** ZDR cannot be enforced (see
  the ZDR row above). Set `audit` to acknowledge that and silence the
  startup warning.

## Data retention

Payverge deletes AI conversation history on its own. Hourly janitors in the
backend remove:

| Data | Deleted after | Variable |
|------|---------------|----------|
| Ops assistant threads (the dashboard help assistant), with their messages, tool calls and request records | 30 days with no new message. A thread someone keeps writing to is never deleted. There is no archive and no per-thread delete. | `OPS_ASSISTANT_RETENTION_DAYS` (default `30`) |
| AI waiter guest transcripts | 90 days | `AI_TRANSCRIPT_RETENTION_DAYS` (default `90`) |

Tell your staff that ops assistant history disappears after a month without
use. Setting either variable to `0` turns that janitor off in every mode,
production included, and keeps the rows until the business is deleted. The
startup log then warns that the janitor is disabled. Change these only if your
own privacy policy says so; the defaults match
[ai-data-retention.md](../policies/ai-data-retention.md).

## Verifying

1. Check the startup log for one of these lines:
   - `no LLM provider configured ... AI features disabled`
   - `LLM_BASE_URL is not an absolute http(s) URL; AI features disabled`
   - `AI privacy ZDR checks skipped: LLM_BASE_URL is a local endpoint`
   - `AI privacy ZDR not applicable: LLM_BASE_URL is a hosted third-party endpoint`
     (or the `OPENROUTER_ZDR_MODE=enforce cannot be honored` warning)
2. Run `curl -s http://localhost:8080/api/v1/instance | jq .features.ai`. It
   should print `true`.
3. Open a table's guest page and ask the waiter about a dish.

## See also

- Every AI variable, with defaults: [configuration.md](configuration.md#ai).
- AI requests run up to 300 seconds; a web server in front of Caddy must allow
  that: [reverse-proxies.md](reverse-proxies.md).
