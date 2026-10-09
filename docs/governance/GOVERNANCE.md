# Governance

This document describes who makes decisions about Payverge, how they are made,
and how contributors take on more responsibility. It is deliberately small: the
project has one lead maintainer, and the process should match its size.

## Model: project lead (BDFL)

Payverge follows a **benevolent dictator for life (BDFL)** model.

- **Project lead:** Marcos Maceo ([@stdevMac](https://github.com/stdevMac)),
  the original author and copyright holder.
- The project lead sets direction, has the final say on any decision, and owns
  the repository settings, release keys and the security advisory process.
- "For life" means "until the lead steps down or hands over". See
  [Continuity](#continuity).

The model is simple on purpose. If a community of regular maintainers grows, this
document will change to share decisions more widely, through the process in
[Changing this document](#changing-this-document).

## Roles

| Role | Who | Can |
| --- | --- | --- |
| **User** | Anyone running or evaluating Payverge | Ask questions, report bugs, propose features |
| **Contributor** | Anyone whose pull request, review, translation or documentation has been merged | Everything a user can, plus be credited in release notes |
| **Triager** | Contributors invited to help manage issues | Label, deduplicate, transfer to Discussions, and close issues and Discussions |
| **Maintainer** | Contributors granted write access | Review and merge pull requests, handle security reports, cut releases when delegated |
| **Project lead** | Marcos Maceo | Everything above, plus final decisions, repository administration and governance changes |

Current maintainers are listed in [`.github/CODEOWNERS`](../../.github/CODEOWNERS).

## How decisions are made

Most decisions are made in the open, in the issue, pull request or Discussion
where the work happens.

1. **Lazy consensus for routine work.** Bug fixes, small features,
   documentation, translations and dependency updates are merged once a
   maintainer approves and CI is green. Nobody needs to vote.
2. **Discuss first for significant changes.** Open an issue or a Discussion,
   and agree the approach before the implementation PR. "Significant" means
   any of the following:
   - database schema or data-model changes;
   - breaking changes to the HTTP API, configuration or environment variables;
   - a new required external service, or a new dependency with a large surface
     or a license other than permissive;
   - changes to payments, money handling, authentication, permissions or
     tenant isolation;
   - changes to how the AI features use tools, guardrails or budgets;
   - removing a feature or a supported locale.

   Significant architectural decisions will be recorded as short architecture
   decision records (ADRs) under `docs/adr/` once that directory is started.
   Until then, the reasoning lives in the issue, Discussion or PR where the
   decision was made.
3. **The project lead decides when consensus does not emerge.** The lead
   explains the reasoning in public, on the issue or PR. The explanation can
   be brief, but it is always given.
4. **Security decisions are made in private**, in the GitHub Security Advisory
   for the report, and are explained publicly when the advisory is published.
   See [SECURITY.md](../../.github/SECURITY.md).

### Principles behind decisions

When trade-offs come up, these break ties.

- **Self-hostable with zero third-party accounts.** The goal is a core system
  that runs with only Docker and a server, where payment providers, LLM
  providers, email and object storage are optional integrations that degrade
  gracefully. [docs/CHANGELOG.md](../CHANGELOG.md) says which of these are
  optional in each release. Changes may move the project toward this goal,
  never away from it.
- **Correctness of money and data first.** Payments, refunds, bill totals and
  tenant isolation outrank features, speed of delivery and convenience.
- **Measured, not guessed.** Performance claims come with benchmarks, and
  behaviour changes come with tests.
- **Guests are served in their own language.** The 21 guest locales and the
  operator locales stay at parity.
- **Agent-friendly by design.** The repository stays easy for coding agents to
  work in correctly (`AGENTS.md`, `CLAUDE.md`, explicit contracts and tests).

## Becoming a triager or maintainer

There is no application form. Responsibility follows a track record.

**Triager.** Help in issues and Discussions for a while: reproduce bugs, point
people to existing answers, flag duplicates. A maintainer will invite you.

**Maintainer.** The project lead invites contributors who have shown, over
roughly three months or more:

- several merged, non-trivial pull requests, with tests, that needed little
  rework;
- useful reviews of other people's pull requests;
- sound judgement in at least one sensitive area (payments, authentication and
  permissions, AI tool scoping, or migrations);
- respect for the [Code of Conduct](../../.github/CODE_OF_CONDUCT.md) in every
  interaction.

You may also ask the project lead directly whether you are on that path. You
will get an honest answer about what is missing.

Maintainers must:

- keep two-factor authentication enabled on their GitHub account;
- follow the security policy, and never discuss an undisclosed vulnerability in
  public;
- not merge their own significant changes without another maintainer's review,
  once more than one maintainer exists.

**Stepping back.** Maintainers who are inactive for six months, or who ask to
step back, move to *emeritus* status and lose write access. They are thanked
in the release notes, and they can return by asking.

**Removal.** The project lead may remove triage or write access for a Code of
Conduct violation, or for misuse of access. That decision is final.

## Licensing, contributions and the name

- The code is licensed under the [Apache License 2.0](../../LICENSE). There is
  no CLA. Contributors certify their right to contribute with the
  [Developer Certificate of Origin](../../.github/CONTRIBUTING.md#developer-certificate-of-origin-dco)
  (`git commit -s`), and keep the copyright to their contributions.
- The "Payverge" name and logo are not licensed under Apache-2.0. See
  [TRADEMARKS.md](../../TRADEMARKS.md). A fork offered to other people as a
  hosted service or as a distributed product uses its own name and logo.

## Continuity

If the project lead can no longer maintain Payverge, they may name a successor
lead from the maintainers. If the project goes quiet with no successor, the
license lets anyone continue it as a fork, under a different name as
TRADEMARKS.md requires.

## Changing this document

Changes to governance are proposed as a pull request to this file, left open for
at least 14 days for comment, and approved by the project lead.

Related: [RELEASING.md](RELEASING.md),
[CONTRIBUTING.md](../../.github/CONTRIBUTING.md),
[SECURITY.md](../../.github/SECURITY.md),
[SUPPORT.md](../../.github/SUPPORT.md).
