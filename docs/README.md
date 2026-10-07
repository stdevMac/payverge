# Payverge documentation

Pick a starting point by what you came to do.

| You want to | Start with |
|---|---|
| Run Payverge for your restaurant or group | [deploy/README.md](../deploy/README.md) |
| Judge the code in twenty minutes | [code-tour.md](code-tour.md) |
| Understand how it fits together | [architecture/overview.md](architecture/overview.md) |
| Understand the AI features and their limits | [ai/README.md](ai/README.md) |
| Contribute a change | [../.github/CONTRIBUTING.md](../.github/CONTRIBUTING.md), then [ONBOARDING.md](ONBOARDING.md) |
| Report a vulnerability | [../.github/SECURITY.md](../.github/SECURITY.md) |

## Self-hosting

Start with [deploy/README.md](../deploy/README.md). It installs the stack with
Docker Compose and covers configuration, admin access, upgrades, backups and
restore, Cloudflare, metrics and troubleshooting. These pages go deeper on one
subject each:

| Page | Covers |
|---|---|
| [storage.md](self-hosting/storage.md) | Local disk or S3-compatible storage for uploads |
| [email.md](self-hosting/email.md) | Log-only, SMTP, Resend or Postmark, and email verification |
| [ai.md](self-hosting/ai.md) | Choosing a model provider (OpenRouter, Ollama, vLLM), or running with none |
| [admin.md](self-hosting/admin.md) | The first platform admin, password resets, who may sign up, and demo restaurants |
| [frontend-config.md](self-hosting/frontend-config.md) | How the web app reads its settings at runtime |
| [whatsapp.md](self-hosting/whatsapp.md) | The optional WhatsApp channel, its separate build, and its licence |

Other operator topics, and where they are covered:

| Topic | Where |
|---|---|
| Every setting, with defaults | [deploy/.env.example](../deploy/.env.example) for a deployment; the root [.env.example](../.env.example) for development |
| Payment providers and their webhooks | [architecture/plugins.md](architecture/plugins.md#per-business-configuration), [architecture/money-flow.md](architecture/money-flow.md) |
| A reverse proxy other than the bundled Caddy | [architecture/overview.md § One origin](architecture/overview.md#one-origin), with [deploy/Caddyfile](../deploy/Caddyfile) as the reference |
| Request IDs and rate limits | [architecture/request-flow.md](architecture/request-flow.md) |

## How it is built

- [code-tour.md](code-tour.md): eight stops through the code that matters
  most, about twenty minutes.
- Architecture:
  [overview](architecture/overview.md),
  [request flow](architecture/request-flow.md),
  [money flow](architecture/money-flow.md),
  [realtime](architecture/realtime.md),
  [plugins](architecture/plugins.md),
  [data and migrations](architecture/data-and-migrations.md).
- [adr/](adr/README.md): architecture decision records, the reasons behind
  the shape of the system.
- [CODEMAPS/](CODEMAPS/): compact maps of the backend, frontend, data model
  and dependencies, for finding your way in the tree.
- [ONBOARDING.md](ONBOARDING.md): a task-to-file map for new contributors.

## AI

[ai/README.md](ai/README.md) explains the one design rule (the model never
decides a fact, a price or an action) and what happens with no model
configured. Then, per feature:
[AI waiter](ai/ai-waiter.md),
[director console](ai/director-console.md),
[ops assistant](ai/ops-assistant.md),
[menu AI](ai/menu-ai.md).
Cross-cutting:
[providers](ai/providers.md),
[cost and budgets](ai/cost-and-budgets.md),
[guardrails](ai/guardrails.md),
[evals](ai/evals.md).
The AI data policy is [policies/ai-data-retention.md](policies/ai-data-retention.md).

## Project

- Governance: [GOVERNANCE.md](governance/GOVERNANCE.md),
  [RELEASING.md](governance/RELEASING.md), [CHANGELOG.md](CHANGELOG.md).
- Community: [contributing](../.github/CONTRIBUTING.md),
  [code of conduct](../.github/CODE_OF_CONDUCT.md),
  [security policy](../.github/SECURITY.md),
  [support](../.github/SUPPORT.md).
- Licensing: [LICENSE](../LICENSE) (Apache-2.0), [NOTICE](../NOTICE),
  [TRADEMARKS.md](../TRADEMARKS.md),
  [third-party licences](licensing/THIRD_PARTY_LICENSES.md),
  [credits](licensing/CREDITS.md).
  [ADR 0008](adr/0008-apache-2-with-whatsapp-behind-gpl-build-tag.md)
  explains why the WhatsApp channel is a separate build.

## Reference notes

Written while the features were built. They go deeper than the pages above,
and some describe detail that has since moved; where a note and the code
disagree, the code is right.

| Topic | Notes |
|---|---|
| Authentication | [AUTH_SYSTEM.md](AUTH_SYSTEM.md) |
| Floor plan | [spaces-and-tables.md](spaces-and-tables.md) |
| Fiscal receipts | [fiscal/fiscal-architecture.md](fiscal/fiscal-architecture.md), [fiscal/argentina-afip.md](fiscal/argentina-afip.md) |
| Languages | [language-support-playbook.md](language-support-playbook.md), [i18n/](i18n/) |
| Telegram | [telegram-plugin-runbook.md](telegram-plugin-runbook.md) |
| Operations | [runbooks/](runbooks/), [performance-runbook.md](performance-runbook.md) |
| Performance evidence | [performance/](performance/) |
| Design | [design/](design/) |
| Product | [product/](product/) |
| Improvement backlog | [BACKLOG.md](BACKLOG.md) |

## Directory map

| Directory | Contents |
|---|---|
| [adr/](adr/README.md) | Architecture decision records |
| [agents/](agents/README.md) | How the in-app AI, the admin MCP tools and the agent skills fit together |
| [ai/](ai/README.md) | AI features, providers, budgets, guardrails and evals |
| [api/](api/) | Notes on selected HTTP APIs (guest orders, instance info) |
| [architecture/](architecture/overview.md) | System overview, request, money and realtime flows, plugins, data |
| [CODEMAPS/](CODEMAPS/) | Compact maps of the backend, frontend, data model and dependencies |
| [design/](design/) | UI and design standards |
| [fiscal/](fiscal/) | ARCA/AFIP fiscal receipts |
| [governance/](governance/GOVERNANCE.md) | Governance and release process |
| [i18n/](i18n/) | Translation and locale notes |
| [licensing/](licensing/THIRD_PARTY_LICENSES.md) | Third-party licences and credits |
| [performance/](performance/) | Benchmark evidence for scoped performance work |
| [policies/](policies/) | Data policies, such as AI data retention |
| [product/](product/) | Product and feature notes |
| [runbooks/](runbooks/) | Operational procedures |
| [self-hosting/](self-hosting/) | Per-topic self-hosting guides |
