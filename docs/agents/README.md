# Agents and AI configuration

Payverge can be set up and run with help from AI at three levels. None of
them needs a code change to turn on.

1. **In-app AI.** Language-model features inside the product: the guest AI
   waiter, the ops assistant, the director console and the menu wizard.
   Restaurant staff use them in the dashboard. You enable them with server
   environment variables.
2. **MCP configuration tools.** [`tools/payverge-admin-mcp`](../../tools/payverge-admin-mcp/README.md)
   is a dependency-free MCP server. It gives an AI agent (Claude Code, Claude
   Desktop, Codex, Cursor or any MCP client) 34 tools. The tools set up a
   restaurant and diagnose a platform through HTTP endpoints the backend
   already serves.
3. **Coding-agent skills.** Seven Markdown playbooks in
   [`.claude/skills/`](../../.claude/skills/) walk a coding agent with a
   shell through installing, configuring, rebranding, translating, extending,
   upgrading and troubleshooting a Payverge instance.

To learn how the in-app AI works inside, read [docs/ai/README.md](../ai/README.md).
For repository conventions, read [AGENTS.md](../../AGENTS.md).

## How the layers fit together

```text
 you (operator or self-hoster)
  |
  |-- coding agent: Claude Code, Codex, Cursor, ...
  |     reads .claude/skills/<name>/SKILL.md, then uses
  |     |-- a shell     docker compose, git, npm, go       -> the server and the repository
  |     `-- MCP tools   tools/payverge-admin-mcp           -> /api/v1 (owner session or admin token)
  |
  `-- browser: the Payverge dashboard
        `-- in-app AI   waiter, ops assistant, director,   -> the LLM provider in the server env
                        menu wizard
```

- The **skills** decide what to do and in what order. Installing,
  upgrading, rebranding and code changes go through the shell. Restaurant
  data (profile, menu, tables, staff, AI settings) goes through the MCP tools.
- The **MCP tools** act as a restaurant owner (`/api/v1/inside/*`) or as the
  platform admin (`/api/v1/admin/*`). Every write previews first
  (`dry_run: true`) and applies only when the agent calls again with
  `dry_run: false`. `PAYVERGE_MCP_BUSINESS_IDS` limits the owner tools and
  the two admin tools that read one business. The platform admin token is
  instance-wide: its health, error-log, webhook and fiscal tools see every
  tenant, so set it only for diagnostics.
- The **in-app AI** runs inside the product for staff and guests. The skills
  and the MCP tools can switch it on per business, but it needs a provider
  in the server environment first. See the next section.

| You want to | Use |
|---|---|
| Install a new server | [setup-and-deploy](../../.claude/skills/setup-and-deploy/SKILL.md) |
| Set up a restaurant: profile, hours, menu, tables, staff, reservations | [configure-restaurant](../../.claude/skills/configure-restaurant/SKILL.md) and the MCP tools |
| Put your own name, logo and colour on the instance | [rebrand](../../.claude/skills/rebrand/SKILL.md) |
| Offer guests another language | [add-locale](../../.claude/skills/add-locale/SKILL.md) |
| Accept payments through a new provider | [add-payment-integration](../../.claude/skills/add-payment-integration/SKILL.md) |
| Move to a newer release | [upgrade](../../.claude/skills/upgrade/SKILL.md) |
| Find out why something is broken | [troubleshoot](../../.claude/skills/troubleshoot/SKILL.md), with the MCP operator tools |
| Let staff ask about the day, or stage price changes they can undo | the in-app AI (below) |

## Enable the in-app AI

AI is optional. Menus, tables, orders, bills, the kitchen display and
payments never call a model. The steps below change only the server
environment.

### 1. Choose a provider

Set one of these in the `.env` file that your compose project reads:

```bash
# OpenRouter (the default endpoint)
OPENROUTER_API_KEY=sk-or-...

# Or any OpenAI-compatible endpoint: Ollama, vLLM, LiteLLM, a hosted vendor
LLM_BASE_URL=http://ollama:11434/v1
LLM_API_KEY=            # optional for Ollama and vLLM
```

Model names come from `OPENROUTER_MODEL_CHAT`, `OPENROUTER_MODEL_MENU`,
`OPENROUTER_MODEL_IMAGE`, `OPENROUTER_MODEL_DIRECTOR` and
`OPENROUTER_MODEL_GUARDRAIL`. The values go verbatim to whichever endpoint
you configured, so for Ollama use ids like `qwen2.5:14b`. The models must
support tool calling. Recipes, fallbacks and model requirements are in
[docs/self-hosting/ai.md](../self-hosting/ai.md).

Every AI call sends restaurant data to that provider: the menu, guest
messages and, for the director, sales figures. Pick a provider whose data
terms you accept, or run a local model.

### 2. Cap the spend

| Variable | Default | Scope |
|---|---|---|
| `AI_DAILY_BUDGET_USD` | `$5` | per business, per UTC day, counted separately for the owner and staff tools and for guest-facing AI (the waiter) |
| `AI_BUDGET_GLOBAL_USD_DAY` | `$20` | the whole instance, every business and lane, per UTC day |
| `AI_BUDGET_GUEST_POOL_USD_DAY` | half the global cap | guest-facing AI at every business together, per UTC day; the rest of the global cap stays reserved for owners and staff |

The caps apply in every mode as soon as a provider is configured, and they
cannot be switched off. Leave a variable unset to get its default. If you
set one, it must be a positive decimal: production startup refuses anything
else (`ai_budget.daily.invalid`, `ai_budget.global.invalid`,
`ai_budget.guest_pool.invalid`), and other
modes log a warning and use the default. Cost accounting is described in
[docs/self-hosting/ai.md](../self-hosting/ai.md).

### 3. Restart the backend

```bash
docker compose up -d backend     # recreates the container with the new env
```

`docker compose restart` does not reread `.env`. With the repository's root
`docker-compose.yml`, add `--env-file .env`.

### 4. Check that it took

```bash
curl -s https://pos.example.com/api/v1/instance | jq .features.ai   # true
```

From an agent, `payverge_instance_status` (MCP) reports the same thing. If it
is still `false`, the backend log says why at startup, for example
`no LLM provider configured ... AI features disabled` or
`LLM_BASE_URL is not an absolute http(s) URL; AI features disabled`.

### 5. Turn surfaces on per business

| Surface | Where staff find it | With no provider | With a provider |
|---|---|---|---|
| Guest AI waiter | The table page (`/t/<table_code>`). Toggle `ai_enabled` (on by default) in the dashboard's AI Waiter tab or with `payverge_update_ai_settings`. `business_page_ai_enabled` puts it on the public business page too. | Basic mode (`ai_waiter_mode: "basic"`): answers come from the menu snapshot. Guests order from the menu, not by chat. | `ai_waiter_mode: "llm"`: cart requests such as "add two empanadas" go to the model, and the server validates them before anything reaches the cart. |
| Ops assistant | The assistant widget in the dashboard ([OpsAssistantWidget.tsx](../../frontend/src/components/opsAssistant/OpsAssistantWidget.tsx)) | `503` | Answers questions about the business |
| Director console | The Director console tab ([DirectorConsole/](../../frontend/src/components/business/DirectorConsole/)) | Briefings, insights, threads and the propose, apply and undo rail work. Chat returns `503 ai_not_configured`. | Free-form chat as well |
| Menu wizard and extraction | Menu builder onboarding ([AIMenuOnboarding/](../../frontend/src/components/business/MenuBuilder/AIMenuOnboarding/)) | `503 ai_not_configured` | Extracts a menu from photos or a PDF, builds one in a guided chat, and generates dish images |

How each surface is built: [docs/ai/README.md](../ai/README.md),
[docs/ai/ai-waiter.md](../ai/ai-waiter.md) and
[docs/ai/director-console.md](../ai/director-console.md).

### 6. Who gets AI

Every business on the server gets AI once a provider is set. There are no
plans. While no provider is set, AI routes answer
`503 {"code":"ai_not_configured"}` (`respondAINotConfigured` in
[ai_entitlements.go](../../backend/internal/server/ai_entitlements.go)). A
business the server administrator suspended or closed gets
`403 business_suspended` or `403 business_closed` instead.

The guest AI waiter never answers `503 ai_not_configured`. With no provider
it stays in basic mode.

## The skills

| Skill | What it does | Works through |
|---|---|---|
| [setup-and-deploy](../../.claude/skills/setup-and-deploy/SKILL.md) | Installs a server with Docker Compose and Caddy, creates the first admin, checks health | shell |
| [configure-restaurant](../../.claude/skills/configure-restaurant/SKILL.md) | Profile, hours, menu, tables and QR links, staff, reservations, AI waiter, payment status | MCP tools |
| [rebrand](../../.claude/skills/rebrand/SKILL.md) | Product name, logo, colour and contacts through env vars, then the built-in assets | shell, then code |
| [add-locale](../../.claude/skills/add-locale/SKILL.md) | Adds a guest storefront language, or promotes one to the dashboard | code |
| [add-payment-integration](../../.claude/skills/add-payment-integration/SKILL.md) | Writes a payment plugin by following the plugin checklist | code |
| [upgrade](../../.claude/skills/upgrade/SKILL.md) | Backs up, pulls a release, lets migrations run, verifies, and rolls back by restoring | shell |
| [troubleshoot](../../.claude/skills/troubleshoot/SKILL.md) | Turns a symptom into a cause with health checks, logs, request ids and the operator tools | shell and MCP tools |

The skills share four rules:

- **Preview before writing.** MCP writes run as `dry_run: true` first.
  Destructive ones (a menu `replace` import) also need `confirm: true`.
- **Secrets stay out of chat and out of git.** Passwords and keys are read
  from the environment or typed by you into a prompt. `.env` is never
  committed or printed.
- **Back up before anything that touches the schema.** Migrations only go
  forward, so a rollback means restoring a backup.
- **Ask before acting on a live server.** A skill stops and asks you before
  it restarts, upgrades or restores production.

## Using the skills with other agents

A skill is a folder holding one `SKILL.md`: YAML frontmatter with `name` and
`description`, then plain Markdown. The bodies use no Claude-specific syntax.
Steps are shell commands, file paths in this repository, or MCP tool names
from [tools/payverge-admin-mcp](../../tools/payverge-admin-mcp/README.md).

- **Claude Code** loads `.claude/skills/` automatically when it runs in a
  clone. Describe the task ("upgrade my Payverge server"), or type the
  skill's name as a slash command, for example `/upgrade`.
- **Codex and other agents that read `AGENTS.md`.** `AGENTS.md` lists the
  skills under "Operator playbooks". Point the agent at one explicitly:
  "Read `.claude/skills/upgrade/SKILL.md` and follow it."
- **Tools with their own skills folder** (Cursor rules, Gemini, OpenCode and
  others). Copy the skill folder into the tool's local directory. Those
  directories are gitignored in this repository. Copy the folder rather than
  symlinking it: a committed symlink into an ignored directory is broken in
  every other clone.
- **MCP.** Any agent that speaks MCP can use the configuration tools.
  Registration snippets for Claude Code, Claude Desktop, Codex and generic
  clients are in the
  [client configuration](../../tools/payverge-admin-mcp/README.md#client-configuration)
  section of the MCP README.

To add a skill, create `.claude/skills/<name>/SKILL.md` with the same
frontmatter. Keep the whole skill in that one file: `.gitignore` shares only
`SKILL.md`, so scripts or notes saved beside it stay local. Ground every
step in a path or command that exists, add it to the table above and to
`AGENTS.md`, and keep secrets out of examples.

## Related

- [docs/ai/README.md](../ai/README.md): how the AI surfaces are built and
  what they do with no model.
- [docs/self-hosting/ai.md](../self-hosting/ai.md): providers, models,
  budgets and verification.
- [deploy/README.md](../../deploy/README.md#configuration): installing,
  configuring, upgrading and backing up a server; every optional variable is
  a commented example in `deploy/.env.example`.
- [docs/self-hosting/admin.md](../self-hosting/admin.md): the first admin,
  password resets, sign-up mode and demo data.
- [tools/payverge-admin-mcp/README.md](../../tools/payverge-admin-mcp/README.md):
  every MCP tool, its endpoint and the permission it needs.
- [AGENTS.md](../../AGENTS.md): repository map and conventions for coding
  agents.
