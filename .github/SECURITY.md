# Security Policy

Payverge processes payments and stores restaurant and guest data. We take
vulnerability reports seriously and appreciate the work that goes into them.

## Reporting a vulnerability

**Do not open a public issue, pull request or Discussion for a security
problem.**

Report it privately through GitHub's private vulnerability reporting:

**[Report a vulnerability](https://github.com/stdevMac/payverge/security/advisories/new)**
(repository **Security** tab, then **Report a vulnerability**).

This opens a private advisory that only you and the maintainers can see. We can
discuss the issue there, work on a fix in a private fork, and credit you when it
is published.

If you cannot use GitHub's reporting form, open a public issue that says only
that you need a private security contact. Include no details. A maintainer will
reply with a private channel.

### What to include

- The affected component (backend API, frontend, payment plugin, AI feature,
  deployment files, admin MCP tool) and the version or commit SHA.
- Step-by-step reproduction against a **local or self-hosted instance you
  control**, including requests and responses where relevant.
- The impact as you understand it. Who can exploit it, and what can they read,
  change or spend?
- Any proof-of-concept code, kept to the minimum needed to show the issue.
- Whether you plan to publish the findings, and when.

## Supported versions

Security fixes go into the latest release line. Older versions do not receive
backports; upgrade to get the fix.

| Version | Supported |
| --- | --- |
| Latest `1.x` minor release | Yes |
| Older `1.x` minor releases | No. Upgrade to the latest `1.x` |
| `main` branch | Best effort. Fixes land here first |
| Anything before `1.0.0` | No |

## Response targets

Payverge is community-maintained and has no paid security team. These are
targets we work to, not contractual guarantees.

| Stage | Target |
| --- | --- |
| Acknowledge your report | 5 business days |
| Initial assessment (accepted or not, and severity) | 10 business days |
| Fix released, Critical or High | 30 days from acceptance |
| Fix released, Medium or Low | 90 days, or the next scheduled release |
| Public advisory | When the fix is released, coordinated with you |

We follow coordinated disclosure. Please give us 90 days from your report, or
until a fix is released (whichever comes first), before you publish. If we need
longer we will tell you why and agree a date with you.

Fixes ship as patch releases. Each one comes with a GitHub Security Advisory and,
where it qualifies, a CVE requested through GitHub.

## Scope

**In scope:** the code and release artefacts in this repository.

- The backend API (`backend/`), including authentication, role-based access
  control, tenant isolation between businesses, and the payment, refund and
  webhook paths.
- The frontend (`frontend/`), including guest QR flows and the owner and staff
  console.
- Payment integrations (Stripe, PayPal, MercadoPago) and on-chain USDC payment
  verification.
- AI features: the AI waiter, operations assistant and director console.
  Prompt injection is in scope when it crosses a security boundary:
  reading another tenant's or guest's data, running a tool the caller is not
  allowed to run, or spending beyond the configured budget.
- Deployment files shipped here (Compose files and reverse-proxy
  configuration) and the container images published from this repository.
- `tools/payverge-admin-mcp`.

Especially valuable: cross-tenant data access, authentication or permission
bypass, settling a bill more than once or for the wrong amount, webhook forgery,
leaks of secrets or personal data, SSRF, injection, and remote code execution.

**Out of scope:**

- Deployments run by other people. Report those to their operator. If the root
  cause is in this code, report it here as well.
- Testing against any hosted instance you do not operate, including the
  project's own website. Run your own instance instead (see the README).
- Findings that need an already compromised server, administrator account or
  leaked secret.
- Volumetric denial of service, spam, and social engineering of maintainers or
  users.
- Missing security headers, cookie flags or version disclosure with no
  demonstrated impact; self-XSS; clickjacking on pages with no sensitive
  actions.
- Vulnerabilities in third-party services (payment providers, LLM providers,
  blockchain RPC endpoints, email providers). Report them to the vendor.
- Known-vulnerable dependencies with no reachable path in Payverge. Dependabot
  tracks these, and a regular pull request is welcome.
- Builds with optional, non-default build tags (for example `-tags whatsapp`)
  when the issue is in the upstream library.
- AI answer quality problems that do not cross a security boundary. Open a
  regular issue for those.

## Bounty

There is no bug bounty programme and no monetary reward. With your permission,
we credit you in the published advisory and the release notes.

## Safe harbour

We will not pursue or support legal action against anyone who researches and
reports in good faith under this policy. That means you test only against
instances you control, do not access, change or keep other people's data, do
not degrade service for others, and give us reasonable time to fix the issue
before you disclose it.

## Known limitations for self-hosters

These are design constraints, not open vulnerabilities. Take them into account
when you deploy.

- **Rate limits and abuse quotas are kept in memory per backend process.**
  This covers request rate limits and the daily per-network and per-device
  caps on guest AI messages. With more than one backend replica, each replica
  enforces its own limits. Run a single backend, or add rate limiting at your
  reverse proxy or edge. The daily AI spending caps are stored in the database
  and shared by every replica.
- **Per-IP limits depend on your proxy settings.** The backend trusts client-IP
  headers only from the proxies you declare (`TRUSTED_PROXIES`, `EDGE`). Set
  them to match your deployment, or every guest appears to come from the
  proxy's address.
- **The admin MCP IP allowlist uses the client IP the backend sees.** That
  allowlist (`PAYVERGE_ADMIN_MCP_ALLOWED_IPS`, loopback by default) is only as
  reliable as your `TRUSTED_PROXIES` setting. Loopback is accepted only for
  direct local connections that carry no forwarding headers.
- **You own the deployment's security.** That includes TLS, strong unique
  secrets (`JWT_SECRET_KEY`, `PLUGIN_SECRET_KEY` and the rest), database
  backups, host patching and image updates. Subscribe to this repository's
  releases to hear about security fixes.
- **AI features send prompts to the LLM provider you configure.** Those prompts
  include business data and text typed by guests. Choose a provider whose data
  handling you accept. Daily spending caps are on by default and cannot be
  switched off; tune them with `AI_DAILY_BUDGET_USD`,
  `AI_BUDGET_GLOBAL_USD_DAY` and `AI_BUDGET_GUEST_POOL_USD_DAY`.
- **On-chain payment verification trusts your RPC endpoint.** Without
  `RPC_URL`, the backend uses the public Base RPC. Use an RPC provider you
  trust, and the mainnet chain in production.
- **`npm audit` reports a high advisory in `braces` (GHSA-vfj7-8cjw-p6xm).**
  It reaches the frontend through Tailwind CSS 3, which only runs while the
  frontend is built and only reads the project's own source files. The running
  server never loads it. No fixed `braces` release exists; the advisory goes
  away with the planned move to Tailwind 4. CI accepts it through a reviewed
  allowlist (`frontend/scripts/npm-audit-allowlist.json`) that expires, and any
  other high advisory still fails the build.

<!--
  Maintainers: if a residual finding is formally accepted instead of fixed,
  add it to "Known limitations" in plain language, without exploit details or
  file:line references. Unfixed findings stay in private advisories until a
  fix ships.
  Forks: replace stdevMac/payverge in the links above with your repository.
-->
