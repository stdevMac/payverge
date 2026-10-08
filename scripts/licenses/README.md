# Licence tooling

Payverge is Apache-2.0 (`LICENSE`), with required attributions in `NOTICE`.
The scripts here check that what we ship stays compatible with that, and they
regenerate the dependency list in `docs/licensing/THIRD_PARTY_LICENSES.md`.

| File | Purpose |
| --- | --- |
| `check.sh` | The gate: Go binaries, bundled fonts and frontend production deps. Exits non-zero on a policy violation. |
| `generate-third-party.sh` | Rewrites `docs/licensing/THIRD_PARTY_LICENSES.md`. With `--check`, fails when the committed copy is stale. |
| `npm-licenses.mjs` | Frontend half of both scripts (`check` / `report`). |
| `npm-licenses.test.mjs`, `lib.test.mjs`, `image-licenses.test.mjs`, `allergen-icons.test.mjs` | `node --test scripts/licenses/*.test.mjs` |
| `go-overrides.tsv` | Go modules whose detected licence is wrong or incomplete, with reasons. |
| `npm-overrides.tsv` | npm licence fill-ins and reviewed exceptions, with reasons. |
| `lib.sh` | Shared shell helpers and the per-override guards. |

## Running

```bash
scripts/licenses/check.sh                  # everything
scripts/licenses/check.sh --skip-go        # fonts + npm only (no Go toolchain)
scripts/licenses/generate-third-party.sh   # rewrite THIRD_PARTY_LICENSES.md, then commit it
scripts/licenses/generate-third-party.sh --check
```

Requirements:

- `go`. The scripts adopt the toolchain `backend/go.mod` asks for, then pin it
  with `GOTOOLCHAIN=local` and `GOROOT=$(go env GOROOT)`. go-licenses
  type-checks against GOROOT, and a mismatch fails with "compile: version X
  does not match go tool version Y".
- go-licenses v2. The scripts use `$GO_LICENSES`, then `go-licenses` on PATH,
  then `go install github.com/google/go-licenses/v2@v2.0.1` into
  `~/.cache/payverge/go-licenses/`. The install needs network once. Set
  `GO_LICENSES_VERSION` to try another release.
- `node` and an installed `frontend/node_modules` (`npm ci`). The lockfile
  leaves out the licence of about 150 packages; those licences are read from
  the installed `package.json`.

## Go policy

`go-licenses check --disallowed_types=forbidden,restricted,unknown` runs
over `./cmd/app ./cmd/email-smoke ./cmd/healthcheck`, the three binaries in
the backend image. The run matches the image build: `GOOS=linux`,
`CGO_ENABLED=0`, and no build tags. Caller `GOFLAGS` are dropped. A package
with no licence file, or with a licence go-licenses cannot classify, fails
too. Reciprocal licences such as MPL-2.0 pass: they are file-level, and the
full source ships with the project.

- **WhatsApp is out of scope.** `whatsmeow` pulls in `go.mau.fi/libsignal`
  (GPL-3.0), so that channel sits behind the opt-in `whatsapp` build tag. Run
  `LICENSES_GO_TAGS=whatsapp scripts/licenses/check.sh --skip-npm` to confirm
  that the tagged build fails the gate.
- **Payverge's own module is skipped.** It is passed as an `--ignore` prefix.
  The prefix comes from `go list -m`, so renaming the module needs no edit
  here.
- **Overrides** (`go-overrides.tsv`). go-licenses has no override feature,
  only `--ignore`. Each override module is therefore removed from the
  go-licenses run. A guard in `lib.sh` then checks the facts the override
  relies on. Packages the ignored module imports are still checked. check.sh
  fails when an override row has no guard, or when its module has left the
  dependency graph.

### github.com/ethereum/go-ethereum → LGPL-3.0-or-later

- go-licenses finds the repository-root `COPYING` (GPL-3.0) and reports the
  whole module as GPL-3.0, which is restricted.
- Upstream licenses go-ethereum in two parts. Everything outside `cmd/` is
  LGPL-3.0, per `COPYING.LESSER` and the README "License" section. Only the
  `cmd/` executables are GPL-3.0.
- The backend imports library packages only: `ethclient`, `accounts/abi`,
  `crypto`, `common` and similar.
- Guard (`guard_go_ethereum`): `COPYING.LESSER` is still the LGPL, the README
  still states the library/cmd split, and `go list -deps` of the three
  binaries contains no `github.com/ethereum/go-ethereum/cmd/...` package.
- LGPL obligations are met in `NOTICE`, which carries the attribution and
  explains how to relink: the full source is this repository plus `go.sum`.
  The GPL text is never triggered.

### github.com/golang/freetype → FTL

- Freetype-Go's `LICENSE` offers "your choice of exactly one of" the FreeType
  License or GPL-2.0-or-later. go-licenses cannot classify that wrapper text,
  so `raster` and `truetype` come back as Unknown.
- We elect the FreeType License. It is a BSD-style licence with a credit
  clause: binary distributions must state that the software "is based in
  part on the work of the FreeType Team". `NOTICE` carries that statement.
- Guard (`guard_freetype`): `LICENSE` still offers the FreeType License, and
  `licenses/ftl.txt` exists.
- The module reaches the backend through `github.com/fogleman/gg` and
  `internal/services/breakdown_overlay.go`. Replacing it with
  `golang.org/x/image/font/opentype` would remove the override.

### Dependency NOTICE files

Apache-2.0 section 4(d) requires redistributors to reproduce a dependency's
NOTICE. check.sh fails when a linked Go module or an installed npm production
package ships a `NOTICE` file that the root `NOTICE` does not name. To fix
it, copy the notice text into `NOTICE`, with the module path or package name
written out in full. The name must stand on its own: a longer name that
contains it does not count, so `gopkg.in/yaml.v3` does not credit
`gopkg.in/yaml`, and `sharp-libvips` does not credit `sharp`.

### Licences in module subdirectories

Some linked Go modules carry code under its own licence in a subdirectory,
such as `go-ethereum/crypto/keccak` (BSD, The Go Authors) and
`go-ethereum/metrics` (BSD, Richard Crowley). go-licenses reports only the
module licence. check.sh therefore walks from each linked package up to its
module root and collects every `LICENSE*`, `LICENCE*` or `COPYING*` file
below the root. Each one must meet two conditions:

- `NOTICE` names it as `module/subdir`, under "Code under its own licence
  inside linked modules".
- Its text appears verbatim in `LICENSES/go-subpackages.txt`. Whitespace
  layout may differ.

When a dependency bump adds such a directory, copy its licence into
`LICENSES/go-subpackages.txt` and add a `NOTICE` entry. Then copy both files
to `backend/` and `frontend/`, because the image copies must stay
byte-identical.

## Frontend policy

The dependency set is every non-dev entry in `frontend/package-lock.json`.
That includes the per-platform optional binaries, because the alpine image
installs the `linuxmusl` builds and not the ones on your laptop. Each
package's licence comes from the lockfile, then from the installed
`package.json` (same version only), then from a `license` row in
`npm-overrides.tsv`. A package with no licence fails the check.

The SPDX expression is evaluated with `OR` taking the best branch and `AND`
taking the worst:

| Class | Licences | Result |
| --- | --- | --- |
| permissive | MIT, ISC, BSD-2/3-Clause, Apache-2.0, 0BSD, BlueOak-1.0.0, CC0-1.0, Unlicense, Zlib, ... | allowed |
| review | LGPL, MPL, EPL, CDDL, CC-BY, FSL, `LicenseRef-*`, and any identifier not listed | allowed only with an `allow` row matching the package and the exact expression |
| forbidden | GPL, AGPL, SSPL, BUSL, CC `*-NC*` / `*-ND*`, Commons-Clause, UNLICENSED | always fails, whatever the overrides say |

Current `allow` rows:

- sharp-libvips (LGPL-3.0-or-later): loaded dynamically and unmodified.
- caniuse-lite (CC-BY-4.0): attribution in `NOTICE`.
- @sentry/cli (FSL-1.1-MIT): build-time only.

Elections recorded in `NOTICE`:

- dompurify, `(MPL-2.0 OR Apache-2.0)`: we use Apache-2.0.
- rgbcolor, `MIT OR SEE LICENSE IN FEEL-FREE.md`: we use MIT.

## When something fails

- **New Go dependency with a restricted or unknown licence.** Replace it if
  you can. If its real licence is permissive and only detection is wrong, add
  a `go-overrides.tsv` row, a guard in `lib.sh`, and a case in
  `lic_run_guard`.
- **npm package "needs review".** Read its licence. If shipping it is
  acceptable, add an `allow` row with the reason and, where the licence asks
  for it, an attribution in `NOTICE`.
- **npm package "no licence declared".** Check the registry with
  `npm view <pkg>@<version> license`, then add a `license` row.
- **"is stale".** Run `scripts/licenses/generate-third-party.sh` and commit
  `docs/licensing/THIRD_PARTY_LICENSES.md`.
