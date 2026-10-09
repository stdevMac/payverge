# Releasing Payverge

This document is for maintainers. It covers how versions are numbered, how a
release is cut, and what each release ships.

Releases are automated by [`.github/workflows/release.yml`](../../.github/workflows/release.yml).
The first release, `v1.0.0`, is tagged by hand; see
[First release (v1.0.0)](#first-release-v100).

## Versioning

Payverge uses [Semantic Versioning 2.0.0](https://semver.org/). One version
tag covers the whole repository: the backend, the frontend, the container
images and `tools/payverge-admin-mcp` are released together from the same
commit, and the `vX.Y.Z` tag names that release.

release-please keeps the version fields in step with the tag: the root
manifest (`.github/.release-please-manifest.json`), `frontend/package.json`
and its lockfile, `tools/payverge-admin-mcp/package.json`, and the default
image tag in `deploy/docker-compose.yml`, `deploy/.env.example` and the
Coolify compose file (the `x-release-please` markers). The frontend image
reports its version through `NEXT_PUBLIC_VERSION`; the backend binary does
not report one, so identify a backend by its image tag.

Version numbers describe what a **self-hoster** experiences on upgrade.

| Bump | When | Examples |
| --- | --- | --- |
| **MAJOR** (`2.0.0`) | An upgrade needs manual action, or something is removed | Renaming or removing an environment variable; removing an `/api/v1` endpoint or response field; a migration that needs manual steps; dropping a supported upgrade path, a locale or a payment integration |
| **MINOR** (`1.4.0`) | New features that upgrade automatically | New features; new optional environment variables with safe defaults; new migrations the backend applies on startup; new locales |
| **PATCH** (`1.4.1`) | Fixes only | Bug fixes, security fixes, translation fixes, dependency updates with no behaviour change (see [Dependency updates](#dependency-updates)) |

Rules that follow from this:

- The upgrade path is forward-only. Any `1.x` release can upgrade to any later
  `1.x` release by deploying the new images. The backend applies pending
  migrations from `backend/migrations/` on startup.
- Rolling back means restoring the database backup taken before the upgrade.
  Running down migrations is not a supported rollback path: many of them,
  including the genesis baseline, are irreversible placeholders, and there is
  no tested procedure for reverting a whole release with them. Release notes
  therefore always say whether a release contains migrations.
- Pre-releases use SemVer suffixes: `v1.5.0-rc.1`, `v2.0.0-beta.2`. They are
  marked as pre-releases on GitHub and never receive the `latest` image tag.

## Tags and branches

- Releases are cut from `main`. There are no long-lived release branches.
- Tags are annotated and named `vMAJOR.MINOR.PATCH` (for example `v1.0.0`).
  They are created only by the release workflow or by a maintainer following
  this document, and are never moved or deleted once pushed.
- A tag ruleset on `v*` lets only maintainers create tags and nobody update or
  delete them (set it up as described under
  [Repository settings](#repository-settings-the-workflow-needs)).
- If a security fix has to reach an older minor while `main` holds unreleased
  breaking work, cut a short-lived `release/vX.Y` branch from the last `vX.Y.Z`
  tag, release `vX.Y.(Z+1)` from it, and delete the branch afterwards. This is
  the only exception.

## What a release ships

| Artefact | Where |
| --- | --- |
| Git tag and GitHub Release with release notes | `https://github.com/stdevMac/payverge/releases` |
| `ghcr.io/stdevmac/payverge-backend` and `ghcr.io/stdevmac/payverge-frontend`, multi-arch (`linux/amd64`, `linux/arm64`) | GitHub Container Registry, tagged `X.Y.Z`, `vX.Y.Z`, `X.Y`, `X` and, for stable releases only, `latest`. Pre-releases get only `X.Y.Z-pre` and `vX.Y.Z-pre` |
| SBOM and SLSA provenance attestations per platform, and a keyless cosign signature on each multi-arch index | Attached to the images in the registry |
| `install.sh`, `docker-compose.yml`, `Caddyfile`, `env.example` and `payverge-deploy-X.Y.Z.tar.gz` (every tracked file under `deploy/`) | Release assets, staged by `scripts/ci/release-assets.sh` |
| `THIRD_PARTY_LICENSES.md` and `SHA256SUMS` | Release assets. The installer checks the deploy files against `SHA256SUMS` |

Release images are built without optional build tags. In particular, **never
publish an image built with `-tags whatsapp`**: that build links GPL-3.0 code,
and the default images must stay Apache-2.0-compatible.

## Release notes and the changelog

GitHub Releases are the canonical changelog. Release notes are generated from
[Conventional Commit](../../.github/CONTRIBUTING.md#commit-messages) subjects
and then edited by a maintainer. Every release note has these sections, in this
order (omit empty ones):

1. **Upgrade notes.** Breaking changes, new or renamed environment variables,
   whether the release has migrations, and any manual steps. This section
   comes first because self-hosters need it before they upgrade.
2. **Security.** Fixed vulnerabilities, with links to their advisories.
3. **Features** (`feat`).
4. **Fixes** (`fix`, `perf`).
5. **Other changes**, such as dependencies and documentation, summarized
   rather than listed commit by commit.
6. **Contributors**, thanking everyone who landed a change, including first-time
   contributors.

[`docs/CHANGELOG.md`](../CHANGELOG.md) mirrors the release notes, one section
per version, newest first. It lives under `docs/` because the repository root
is a closed set of files; release-please writes there through the
`changelog-path` setting in `.github/release-please-config.json`. The
release-please sections map to the list above: `feat` to Features, `fix`,
`perf` and `revert` to Fixes, `docs` to Other changes. Add the Upgrade notes,
Security and Contributors sections by hand in the release PR.

Notes written ahead of a release go under an `## [Unreleased]` heading at the
top of `docs/CHANGELOG.md`. release-please does not read that section: when
the release PR opens, move those notes into the new version's section in the
PR branch, delete the `[Unreleased]` heading, and edit the PR description to
match, because the description becomes the GitHub Release notes.

## Release flow

Every push to `main` runs the release workflow:

1. **release-please** keeps a **release PR** open that bumps the version
   fields listed under [Versioning](#versioning) and adds a section to
   `docs/CHANGELOG.md` from the Conventional Commit subjects since the last
   release. Configuration: `.github/release-please-config.json` and
   `.github/.release-please-manifest.json`.
2. When it is time to release, a maintainer edits the release PR's changelog
   section (the upgrade notes in particular), checks that it passes the
   [release-notes check](#release-notes-check), and merges it once `ci-ok`
   is green.
3. The merge creates the `vX.Y.Z` tag and the GitHub Release. The same
   workflow run then:
   - waits for a green `ci.yml` run on the release commit
     (`scripts/ci/wait-for-ci.sh`) and fails if the commit still names the
     upstream domain (`scripts/check-hardcoded-domain.sh --release`);
   - builds both images natively per platform, with no optional build tags,
     and fails if the backend binary links `go.mau.fi` (the GPL `whatsapp`
     build);
   - merges the per-platform digests into one index per image, tags it and
     signs it with cosign (keyless, the workflow's OIDC identity);
   - uploads the release assets.

   This all happens in one workflow because a tag created with the default
   `GITHUB_TOKEN` does not trigger other workflows.

   release-please publishes the GitHub Release when the PR merges, before the
   images and assets exist. For the length of the workflow run, which builds
   both images on two platforms, `releases/latest/download/install.sh` returns 404 and
   `PAYVERGE_VERSION=X.Y.Z` cannot be pulled. A draft release would avoid
   that window, but release-please creates no tag for a draft, and the
   publish jobs need the tag, so the configuration does not use drafts. Do
   not announce a release until the workflow is green.
4. Run the [post-release checks](#post-release-checks).

If a run fails after the tag exists (CI was red, a build broke, the registry
was down), fix the cause on `main` if needed, make CI green on the tagged
commit (re-run its `push` run of CI; dispatched CI runs do not count), then
run **Actions > Release > Run workflow** from `main` with the existing tag
(for example `v1.2.3`). A dispatch only publishes; it never creates a tag.

### Repository settings the workflow needs

None of these exist in a fresh repository. Set the first four up before the
first release, and the rulesets right after it:

- **`release` environment** (Settings > Environments > New environment):
  deployment branches limited to `main`. The release-please, publish and
  assets jobs run in it, and the workflow refuses to run from any other ref
  or for a tag not reachable from `main`.
- **`RELEASE_PLEASE_TOKEN`**, a secret of the `release` environment (not a
  repository secret): a fine-grained token with contents and pull-request
  write access on this repository only. Without it, release-please falls
  back to `GITHUB_TOKEN`, and a release PR opened with `GITHUB_TOKEN` does
  not trigger `ci.yml`. Then `ci-ok` never reports on the release PR; close
  and reopen the PR to get a run.
- **Actions permissions** (Settings > Actions > General): workflows have
  read and write permissions and may create pull requests, and GitHub
  Packages accepts pushes from this repository's workflows (the first push
  creates both packages).
- **Labels** for Dependabot: `dependencies`, `go`, `javascript`, `docker`
  and `github-actions` (Issues > Labels). Dependabot skips a label that does
  not exist, and logs an error on the PR.
- **Packages visibility**: after the first release, make both packages
  public (each package's settings > Danger zone > Change visibility), or
  anonymous `docker compose pull` fails.
- **Rulesets** (Settings > Rules > Rulesets), once the release exists:
  `main` requires `ci-ok` and CodeQL and blocks force pushes and deletion;
  a tag ruleset on `v*` lets only maintainers and the release workflow
  create tags, and nobody move or delete them.

## First release (v1.0.0)

The public repository starts as a single root commit whose
`docs/CHANGELOG.md` already holds the complete 1.0.0 notes and whose
manifest already says `1.0.0`. There is no history before it and no release
PR for it, and v1.0.0 is the first and only release on that commit. Tag it
by hand, then let the workflow publish:

1. Set up the [repository settings](#repository-settings-the-workflow-needs)
   above, except the rulesets and package visibility.
2. Wait for `ci-ok` to be green on the root commit's `push` run of `ci.yml`
   (re-run failed jobs if needed; a dispatched run does not count). Then run
   the release-notes check on that commit:
   ```bash
   RELEASE_VERSION=1.0.0 node --test scripts/ci/release_notes_contract.test.mjs
   ```
   Any change, including the date in the `## [1.0.0]` heading, means a new
   commit, and the repository is meant to hold exactly one when 1.0.0 is
   tagged. Fix such issues before the public push, not after.
3. Create and push an annotated, signed tag on that commit:
   ```bash
   git switch main && git pull --ff-only
   git tag -s v1.0.0 -m "Payverge v1.0.0"
   git push origin v1.0.0
   ```
4. Run **Actions > Release > Run workflow** from `main` with
   `tag` = `v1.0.0`. The workflow waits for the green `ci.yml` run, builds
   and signs the images, and attaches the assets to a **draft** release it
   creates, because the tag has no release yet.
5. When it is green, check the images and their signatures:
   ```bash
   docker buildx imagetools inspect ghcr.io/stdevmac/payverge-backend:1.0.0
   cosign verify ghcr.io/stdevmac/payverge-backend:1.0.0 \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com \
     --certificate-identity https://github.com/stdevMac/payverge/.github/workflows/release.yml@refs/heads/main
   ```
   Repeat for `payverge-frontend`.
6. Make both packages public, then check that an anonymous client can pull
   them:
   ```bash
   docker logout ghcr.io
   docker pull ghcr.io/stdevmac/payverge-backend:1.0.0
   docker pull ghcr.io/stdevmac/payverge-frontend:1.0.0
   ```
7. Open the draft release, paste the 1.0.0 section of `docs/CHANGELOG.md`
   into its notes, and publish it as the latest release. Until it is
   published, `releases/latest/download/install.sh` returns 404, and
   release-please has no release to count from.
8. Check that the installer resolves:
   ```bash
   curl -fsSLI https://github.com/stdevMac/payverge/releases/latest/download/install.sh
   ```
   Then add the rulesets and run the
   [post-release checks](#post-release-checks).

Push nothing to `main` between the root commit and publishing v1.0.0, so the
release sits on the only commit and release-please's first release PR
starts from 1.0.0. Further commits come after the release is public.

## Dependency updates

Dependabot (`.github/dependabot.yml`) opens weekly PRs labelled
`dependencies` plus the ecosystem (`go`, `javascript`, `docker`,
`github-actions`). Their commit prefixes decide whether a merge leads to a
release:

- Runtime dependencies of what ships (backend Go modules, frontend and
  admin MCP npm packages, the Dockerfiles' base images and the deploy
  compose images) use `fix(deps)`, so release-please proposes a patch
  release with them.
- Development dependencies use `chore(deps-dev)`, and GitHub Actions and
  the repository's internal tooling use `ci(deps)` or `chore(deps)`. These
  types are hidden in the changelog and never start a release on their own.
- PostgreSQL major versions are ignored in `dependabot.yml`: moving to a new
  major moves the data directory (`deploy/upgrade-postgres.sh`) and the
  genesis `PostgresMajor` check refuses it, so it is a MAJOR release prepared
  by hand with a `!` or `BREAKING CHANGE:` commit, never a `fix(deps)` patch.
  If such a Dependabot PR appears anyway, close it.

Every PR that changes a Go module or an npm lockfile fails the `licenses`
job (and with it `ci-ok`), because
`docs/licensing/THIRD_PARTY_LICENSES.md` is generated from the dependency
trees and Dependabot cannot regenerate it. Regenerate it on the PR branch
before merging:

```bash
gh pr checkout <number>
bash scripts/licenses/generate-third-party.sh
git add docs/licensing/THIRD_PARTY_LICENSES.md
git commit -s -m "chore(deps): regenerate third-party licenses"
git push
```

Doing this automatically needs a token that can push to Dependabot branches
and retrigger CI (a push made with `GITHUB_TOKEN` does not start a new
`ci.yml` run), stored as a Dependabot secret. Until such a token exists, it
stays a maintainer step.

## Release-notes check

Release notes must not promise more than the tagged code does. Before a tag is
created, run:

```bash
RELEASE_VERSION=X.Y.Z node --test scripts/ci/release_notes_contract.test.mjs
```

Without `RELEASE_VERSION`, the script runs only the always-on checks. These
fail when the newest section of `docs/CHANGELOG.md`, `CONTRIBUTING.md`,
`SECURITY.md`, `SUPPORT.md` or the bug-report form claims optional object
storage or email, zero third-party accounts, or an installer while the code
still says otherwise. With `RELEASE_VERSION` set, the script also checks that
the version has a dated changelog section, that no integrator notes or
release-date placeholders remain, that every repository path the newest
changelog section names exists, and that every relative link in the
changelog, the community files and the governance documents resolves.

The checks match key phrases, so they catch over-claims but cannot prove that
a release note is accurate. A maintainer still reads the notes against the
code. When a capability changes how it is switched on, update the matching
check in the script in the same PR.

## Post-release checks

- Install the released version on a clean machine with the released
  `install.sh` (see [deploy/README.md](../../deploy/README.md#install)), and
  complete the first-run flow: sign in with the admin email and the one-time
  password the installer prints, create a business, place a QR order, and
  close the bill.
- Upgrade an instance running the previous release, and confirm migrations
  apply and data survives.
- Check that the image tags point at the digests in the workflow summary,
  and verify the signatures with the `cosign verify` command it prints.
- Announce the release in Discussions (Announcements), linking the notes.

## Security releases

1. The fix is developed in the private fork attached to the GitHub Security
   Advisory, not in a public branch.
2. Release it as a patch on the latest minor (see
   [SECURITY.md](../../.github/SECURITY.md#supported-versions)). Older minors
   are not backported.
3. Publish the advisory at the same time as the release, crediting the reporter
   if they agree. The release notes link to it under **Security**. Exploit
   details stay in the advisory.
