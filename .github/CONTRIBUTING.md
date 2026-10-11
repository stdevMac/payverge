# Contributing to Payverge

Thanks for taking the time to contribute. Payverge is a self-hostable restaurant
system: QR ordering, kitchen display, bills, staff tools, an AI waiter and
operations assistant, and card, wallet and USDC payments. It handles other
people's money and their guests' data, so the bar for changes is high. This guide
explains how to clear it without guesswork.

By taking part you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Where things go

| You want to… | Go to |
| --- | --- |
| Ask a question or get help self-hosting | [GitHub Discussions](https://github.com/stdevMac/payverge/discussions) (see [SUPPORT.md](SUPPORT.md)) |
| Report a bug | [Open an issue](https://github.com/stdevMac/payverge/issues/new/choose) with the bug form |
| Propose a feature | Start a Discussion for anything large; use the feature form for a concrete, scoped proposal |
| Report a security vulnerability | **Never in a public issue.** Follow [SECURITY.md](SECURITY.md) |
| Send a fix or feature | A pull request, following this guide |

For anything bigger than a small fix, open an issue or Discussion first and agree
on the approach before you write the code. It saves everyone a rewrite.

## Development setup

### Prerequisites

- **Go**: the version in `backend/go.mod` (`toolchain` line). Go downloads it
  for you when `GOTOOLCHAIN=auto`, which is the default.
- **Node.js**: the version in `.nvmrc` (`nvm use` picks it up).
- **Docker** with Compose v2, for PostgreSQL 18 and the full stack.
- **Git**, with your name and email configured (they appear in your sign-off).

### First run

```bash
git clone https://github.com/stdevMac/payverge.git
cd payverge
make install-hooks          # repo-tracked git hooks: secret scan + i18n parity
```

Then bring up the stack with the [Development](../README.md#development)
section of the README. If it disagrees with this file, the README wins, and a
PR fixing this file is welcome.

### Working on one side at a time

```bash
# Backend (Go/Gin, needs a reachable PostgreSQL)
cd backend
go mod download
make run            # API on :8080
make quick-test     # -short suite, no race detector (fast loop)
make test           # -short suite with -race, as CI runs it (no Docker)
make test-docker    # every test, including Testcontainers suites (needs Docker)

# Frontend (Next.js 15)
cd frontend
npm ci
npm run dev         # :3000
npm run lint && npm run typecheck && npm run test
```

### Cold verification (what CI runs)

```bash
make verify:backend    # env-scrubbed build, vet, race tests, integration tests
make verify:frontend   # clean install, lint, typecheck, Jest, i18n checks, build
```

`make verify:frontend` includes a full Next.js build, which takes a while. Run it
before you ask for a final review, not on every save.

### Orientation

- [`AGENTS.md`](../AGENTS.md): the deep guide to architecture, conventions and
  gotchas. It is written for coding agents, and works just as well for humans.
- [`docs/ONBOARDING.md`](../docs/ONBOARDING.md): a two-minute map from task to
  file.
- [`docs/CODEMAPS/`](../docs/CODEMAPS/): compact references for the backend,
  frontend, data model and dependencies.

## Making a change

1. Fork the repository and create a branch from `main`, for example
   `fix/split-bill-rounding` or `feat/kds-course-timer`.
2. Keep the pull request to **one logical change**. Refactors go in their own PR,
   separate from behaviour changes.
3. Write or update tests (see [Tests are required](#tests-are-required)).
4. Use [Conventional Commits](#commit-messages) and **sign off every commit**
   ([DCO](#developer-certificate-of-origin-dco)).
5. Open the PR and fill in the template. CI must be green before review starts
   in earnest. CI runs on a first-time contributor's PR wait until a maintainer
   approves them. The PR workflows use no repository secrets, so CI on a fork
   PR runs the same checks as on a maintainer's branch.

### Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).
The release tooling reads them to build release notes, so the type matters.

```
<type>(<optional scope>): <imperative summary, lower case, no trailing period>

<optional body: what and why, wrapped at ~72 columns>

Signed-off-by: Your Name <you@example.com>
```

| Type | Use for |
| --- | --- |
| `feat` | A user-visible feature |
| `fix` | A bug fix |
| `perf` | A performance improvement with benchmark evidence |
| `refactor` | A code change with no behaviour change |
| `test` | Adding or fixing tests only |
| `docs` | Documentation only |
| `build`, `ci` | Build system, Dockerfiles, workflows |
| `chore` | Maintenance, such as dependency bumps |

Scopes name the area: `payments`, `kds`, `ai-waiter`, `i18n`, `frontend`,
`backend`, `deploy`, and so on. Mark breaking changes with `!` after the
type or scope (`feat(config)!: …`) and explain the upgrade path in a
`BREAKING CHANGE:` footer.

Examples:

```
fix(payments): one on-chain USDC transfer settles at most one bill
fix(ai-waiter): ask which dish when two menu rows share one name
test(frontend): reconcile merged seams across metadata and guest locales
```

## Tests are required

Every behaviour change ships with tests. A pull request without them will be
asked to add them.

- **Bug fixes start with a failing test.** Write the regression test, watch it
  fail, then fix the code. Say in the PR that you did this.
- **Backend**: table-driven Go tests next to the code. Run the packages you
  touched with the race detector:
  `go test -race ./internal/<package>/... -run TestName`.
  HTTP handlers need tests for tenant isolation (one business must never read or
  change another's data) and for permission checks.
- **Frontend**: Jest tests next to the component or API client
  (`*.test.ts`/`*.test.tsx`). Mock `fetch` with real backend response shapes.
- **AI features**: changes to prompts, tools or guardrails need the offline eval
  suites to pass (`cd backend && make eval-offline`). Add a fixture when you
  change behaviour.
- **End to end**: Playwright specs live in `frontend/tests/`. They are not
  required for every PR, but a maintainer may ask for one on a critical flow
  (ordering, paying, the kitchen display).

### Backend rules that tests enforce

- **Schema changes are numbered migrations.** Add a zero-padded
  `NNNNNN_description.up.sql` and `.down.sql` pair in `backend/migrations/`.
  Do not add new GORM `AutoMigrate` calls or startup DDL; tests reject them.
- **Money is integer cents in storage and dollars on the wire.** The backend
  stores `int64` cents and its JSON marshalers emit `float64` dollars. Keep both
  ends consistent, and never re-divide amounts that are already dollars.
- **Payment plugins register through the deferred initializer** in
  `backend/internal/plugins/`. Webhooks must verify signatures and fail closed.

### Backend Performance Gate

Changes that can affect latency, allocations, query count or fan-out are not
done until they are measured. This covers guest and public routes, dashboard
polling, payment and webhook paths, AI session reads, list endpoints, shared
middleware, queues and schedulers, and any change to query shape, indexes,
caches or serializers.

1. **Add a regression test first.** When the risk is data access, assert that the
   dangerous query shape is gone: `SELECT *`, unused `Preload`, N+1 queries,
   duplicate counts.
2. **Capture a baseline** before changing production code: a `testing.B`
   benchmark run with `-benchmem`. Use SQLite microbenchmarks for deterministic
   access-shape work, and the Testcontainers/Postgres benchmarks when realism
   matters.
3. **Fix the implementation, not the benchmark.** Use narrow projections, joins
   instead of per-row reloads, batched reads and writes, SQL aggregates, indexes
   added through migrations, and bounded result sets.
4. **Re-run with `-count=3`** and put the before/after latency, bytes/op and
   allocs/op in the PR description, with the exact command.
5. **Run a compile gate for neighbouring packages**, for example
   `go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1`.

`cd backend && make bench` runs the shared benchmark set. PRs that touch these
paths without numbers will be asked for them.

## Internationalization parity

Payverge has two translation tiers. Both are checked by the pre-commit hook
(`make install-hooks`) and in CI.

| Tier | Where | Locales | Rule |
| --- | --- | --- | --- |
| **Operator** (owner and staff console) | `frontend/src/i18n/messages/<locale>/` | `en`, `es`, plus the `es-ar` override layer | `en` is the source of truth. Every `en` key must exist in `es`, with no extra keys. `es-ar` is a regional override layer merged on top of `es` and holds only the keys whose Rioplatense wording differs, and it may not add keys that `es` lacks. |
| **Guest** (QR menu, ordering, bill, payment) | `frontend/src/i18n/guest-messages/<locale>.json` | 21 locales | Every key must exist in all 21 files, with placeholders like `{name}` preserved exactly. |

`locales/registry.json` is the single source of truth for which locales exist.
Generated files in the frontend and backend are produced from it; never edit
them by hand.

Run the validators before you push:

```bash
# Operator tier
node frontend/scripts/check-operator-locales.js            # add --report for the full list

# Guest tier
node frontend/scripts/check-guest-locales.js --strict      # parity + untranslated-copy ratchet
node frontend/scripts/check-guest-locales.js --keys        # every t() key exists in en.json
node frontend/scripts/check-guest-locales.js --critical    # launch-critical keys and error mappings
node frontend/scripts/check-guest-locales.js --cookies     # cookie-banner and view-mode copy

# Registry, codegen and catalog checks
cd frontend
npm run i18n:check       # generated files match locales/registry.json
npm run i18n:generate    # regenerate them after editing the registry
npm run i18n:validate
npx tsx scripts/i18n/check-hardcoded-strings.ts   # no hard-coded UI strings
```

Practical rules:

- New UI copy goes through the translation helpers. No hard-coded strings in
  components.
- **Do not paste English into other locales.** The `--strict` guest check counts
  untranslated values and fails if that number grows. Machine-assisted drafts
  are fine; say so in the PR so a native speaker can review them.
- If you cannot translate a language, say so in the PR. A maintainer will help
  rather than reject the change.

## Developer Certificate of Origin (DCO)

Payverge uses the [Developer Certificate of Origin 1.1](https://developercertificate.org/)
instead of a Contributor License Agreement (CLA).

**What it is.** The DCO is a short statement that you wrote the change, or
otherwise have the right to submit it under the project's open-source license.
You agree to it by adding a `Signed-off-by` line to each commit:

```
Signed-off-by: Your Name <you@example.com>
```

Git adds the line for you:

```bash
git commit -s -m "fix(kds): keep bumped tickets in course order"
```

**Why a DCO and not a CLA.**

- There is nothing to sign up front and no legal paperwork, and you keep the
  copyright to your work.
- Contributions come in under the same license they go out under:
  [Apache-2.0](../LICENSE), whose section 5 already covers submitted
  contributions. The project needs no extra rights beyond that.
- The sign-off is recorded in git history next to the change it covers.

**Rules.**

- Sign off every commit in a PR with `git commit -s`. Maintainers check the
  sign-off when they review, and ask you to add it if it is missing.
- Sign off with a name you are known by and an email that reaches you.
  Anonymous contributions cannot be accepted.
- Signing off asserts that *you* can submit the change. That includes code
  written with AI assistance (see below).

**Forgot to sign off?**

```bash
git commit --amend --signoff --no-edit      # the last commit
git rebase --signoff main                   # every commit on your branch
git push --force-with-lease
```

## AI-assisted contributions

This repository is set up for agentic development: `AGENTS.md` and `CLAUDE.md`
exist so coding agents work within the project's rules. AI-assisted pull
requests are welcome, with three conditions:

- You understand and can explain every line you submit, and you have run the
  tests yourself.
- You do not submit generated code you lack the rights to contribute. Your
  sign-off covers it.
- You do not paste secrets, production data or other people's private
  information into prompts or into the repository.

## Secrets, data and security

- Never commit `.env` files, credentials, API keys, database dumps or real
  customer or guest data. The pre-commit hook runs a secret scan; do not bypass
  it with `--no-verify`.
- Test fixtures use obviously fake data (`example.com` emails, `+1 555` phone
  numbers, test-mode payment keys).
- If you find a vulnerability while working on something else, stop and follow
  [SECURITY.md](SECURITY.md). Do not describe it in a public issue, PR or
  commit message.

## Review and merging

- A maintainer reviews every PR. Expect questions on payments, tenant isolation,
  authentication and AI tool scoping; those areas get the strictest review.
- The required checks must pass. Rebase onto `main` when asked; history is kept
  linear.
- Maintainers squash or rebase-merge. The PR title becomes the commit subject,
  so it must also follow Conventional Commits.
- How decisions are made, and how to become a maintainer, is described in
  [docs/governance/GOVERNANCE.md](../docs/governance/GOVERNANCE.md).

## License

By contributing, you agree that your contributions are licensed under the
[Apache License 2.0](../LICENSE). The Payverge name and logo are not covered by
that license; see [TRADEMARKS.md](../TRADEMARKS.md).
