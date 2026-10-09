# oss-export

This tool builds the public `stdevMac/payverge` repository from a ref of the private repository. The result is a fresh repository with a single commit. It never pushes; the push is a manual step only the owner runs (see the end of this file).

> **Maintainer tool.** It is in the public tree only so the release process can be read and audited. It cannot run from a clone of the public repository: it refuses to export without `forbidden.txt`, and that file and the other lists are private by design (they name the strings that must not ship; see [Configuration (private)](#configuration-private)). Their in-repository copy lives under `docs/superpowers/oss-export/`, which the export itself removes. Contributors do not need this tool.

```
tools/oss-export/export.sh [<ref>] <outdir> --author-email <email> [options]
```

- `<ref>` is any commit-ish. The default is `oss/main`.
- `<outdir>` must be a new or empty directory **outside** the repository.
- `--author-email` is required and has no default. It sets the author and committer email of the public commit. The name defaults to `Marcos Maceo`.
- The tool is plain bash (it also runs on macOS `/bin/bash` 3.2) plus Node 20 or newer. It needs no npm packages.

## What it does

1. **Archive.** It runs `git archive <ref> | tar -x` into `<outdir>`. Only the ref's tracked content is used. The working tree, including untracked files such as `frontend/.env`, is never read. Your global and system git config and attributes are ignored here too (no `core.autocrlf`, no `core.eol`, no global `export-ignore` or filter), so they cannot change which bytes are exported. The private repository's own `.gitattributes`, `.git/config` and `.git/info/attributes` still apply.
2. **Drop.** It removes every `docs/superpowers/` directory, at any depth (always), and every path in `drop.txt`. Directories left empty are removed too. Entries that match nothing only print a warning.
3. **Scrub.** It applies the rules in `scrub.rules` to every UTF-8 text file. Binary files are skipped.
4. **Gates.** It runs every gate listed below. If any gate fails, the script exits with status 1. The tree is left in place for inspection, and no repository is created. If the script stops before the drop and scrub steps have finished (for example on a configuration error), it deletes the raw archive from `<outdir>`, so an unsanitized tree is never left behind.
5. **Stats.** It prints the file count, the total size, the size of each top-level entry and the largest files.
6. **Commit.** It runs `git init` with branch `main`, stages exactly the files in the tree and makes one commit, `Initial public release`.
   - The committer date is the current time.
   - Your global and system git config, hooks, signing and global excludes are all ignored, as in the archive step.
   - After the commit it checks three things: there is exactly one commit, there is no remote, and the working tree is clean.

Reports are written to `<outdir>.export-report/`, or to the directory given by `--report-dir`, which must be outside both the repository and the outdir:

| File | Contents |
|---|---|
| `drop.json` | What the drop step removed |
| `scrub.json` | What the scrub step changed |
| `gates.json` | Every gate's result. Hits give `file:line` and a rule name, never the matched value |
| `stats.json` | The tree statistics |
| `summary.txt` | The overall result |
| `secrets-allow.candidates.txt` | Unreviewed scanner findings, already formatted as allowlist lines |

### Exit codes

| Code | Meaning |
|---|---|
| 0 | Exported. With `--no-commit`, all gates passed |
| 1 | A gate failed. Nothing was committed |
| 2 | Usage or configuration error |
| 3 | Internal error |

## Gates (all fatal)

| | Gate | Fails when |
|---|---|---|
| a | `gitleaks` | gitleaks (`dir`, or `detect --no-git` on older versions) finds anything not listed in `secrets-allow.txt`. The gate pins its own config (the default rules) and ignores any `.gitleaks.toml` or `gitleaks:allow` comment inside the export, so the exported tree cannot weaken its own scan. gitleaks 8.x always honours a `.gitleaksignore` at the scanned root, so a root `.gitleaksignore` in the export fails the gate (`scanner-suppression-file`). |
| b | `trufflehog` | `trufflehog filesystem --no-only-verified` reports anything not allowlisted. A finding that verifies as live is always fatal. Verification is off by default; `--trufflehog-verify` turns it on, which sends candidate strings to third-party APIs. |
| c | `forbidden-strings` | Any rule in `forbidden.txt` matches a path, a file, a symlink target, the commit author string or the commit message; or a file is an archive the gate cannot read (`opaque-archive`). See "What gate (c) reads" below. |
| d | `file-size` | A file is larger than `--max-file-mb` (default 10). Paths in `--size-allow` are exempt; the default allowlist is `frontend/public/fonts/print/noto-*-cjk-*`. |
| e | `symlinks` | A symlink has an absolute target, lands outside the tree, or is part of a loop, or the tree contains a special file. Targets are resolved one component at a time, following every link on the way, as the kernel would, so a chain such as `sub/up -> ..` plus `sub/esc -> up/../..` fails even though each target looks harmless on its own; the kernel's own `realpath` is checked as well. A dangling link that stays inside the tree only produces a note. |
| f | `env-files` | A file named `.env`, `.env` followed by `.`, `-` or `_` and anything (`.env.local`, `.env-production`, `.env_local`), `*.env`, `.envrc` or `.envrc.*` is present, or any file sits under a `.envs/` directory (`.envs/.production/.django`). Names are compared case-insensitively. Only names ending in `.example` are exempt. `--allow-env-file <path>` exempts one file, and needs the owner's sign-off. |
| g | `gitignore-clean` | `git check-ignore --no-index` reports a file as ignored by the exported `.gitignore` files. Your global excludes are not consulted. |
| h | `license` | `LICENSE` is missing, is not a regular file, or is not the Apache-2.0 text. The whole file is compared with the canonical text in `lib/apache-2.0.txt`, with whitespace collapsed: the terms must match word for word (`not-apache-2.0`), and nothing may follow them except the stock appendix (`text-after-apache-terms`), so the Apache text with an added restriction such as a Commons Clause fails. The appendix's copyright line may keep its placeholder, or name years and exactly the `--copyright-holder` (default: the author name, `Marcos Maceo`), optionally with `(c)` or `©` and a final period. Anything else on that line fails as `copyright-holder-mismatch`, so a restriction cannot ride in on it (`Copyright 2026 Marcos Maceo. Commercial use is not permitted ...`). A missing `NOTICE` only produces a note. |
| i | `private-paths` | A `docs/superpowers` directory exists at any depth (`frontend/docs/superpowers/` counts, and so does any letter case), or any path still matches a drop entry. |
| j | `doc-links` | A relative link (inline, image or reference definition, outside code) in `README.md`, `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md` or `docs/agents/README.md` points at a path that is not in the exported tree (`link-target-missing`), or climbs above the tree root (`link-leaves-tree`). Fragments and queries are ignored; a leading `/` resolves from the tree root. This catches a drop entry that removes something the entry-point docs promise, such as `.claude/skills/`. |

### What gate (c) reads

Each file is expanded into several views, and every text rule runs over each one. A hit's line says which view matched: a number for the file itself, `binary` for its raw bytes, or a label such as `utf-16le:3`, `nul-stripped`, `gzip:12`, `gzip@512:2`, `png-zTXt`, `unescaped:7` or `base64:4/gzip`. Decoded views report the file's own line number.

- UTF-8 text as is; any other file as raw bytes (latin1).
- UTF-16 text, LE or BE, with or without a byte-order mark, decoded with line numbers.
- Any other binary file that holds NUL bytes, with every NUL removed. This exposes UTF-16 and UTF-32 strings stored inside a binary format, such as a font's name table.
- gzip (`.gz`, `.tgz` and any file with the gzip magic number), inflated and expanded again, up to 3 levels deep and 64 MB per level.
- In any binary file, a gzip member that starts past offset 0 (`gzip@<offset>`): one appended to an image, or a compressed member of a tar. It is inflated and expanded the same way, at most 64 members per file. A tar's uncompressed members are already in its raw bytes.
- The compressed text chunks of a PNG (`zTXt`, `iTXt`). Uncompressed metadata (PNG `tEXt`, EXIF and XMP in JPEG and WebP) is already in the raw bytes.
- In a text file (and the commit message), each line decoded once from JS/JSON escapes (`\u0040`, `\u{40}`, `\x40`, `\-`; view `unescaped`), percent-encoding (`%40`; `percent-decoded`), HTML character references (`&#64;`, `&#x40;`, `&commat;`; `entity-decoded`) and base64, standard or URL-safe, in runs of 16 or more characters that decode to UTF-8 or UTF-16 text (`base64-decoded`). A base64 run that decodes to a gzip, a PNG or an archive is expanded like a file (`base64:<line>/...`). A decoded line that shows the same hit as the line it came from is reported once.

Formats that need a real archive reader fail the gate as `opaque-archive`: zip and everything built on it (`.jar`, `.docx`, `.xlsx`, `.pptx`, `.odt`, `.epub` and so on), bzip2, xz, 7z, zstd, rar, lzip, lz4, brotli (`.br`) and `compress` (`.Z`). That holds wherever the archive starts: at offset 0, inside a binary file past offset 0 (`embedded-zip@1536`: a tar member, or a zip appended to an image), or base64-encoded in a text file (`base64:4/zip`). Brotli and `compress` have no magic number and are only recognised by extension. A corrupt gzip, a gzip file followed by anything but zero padding (such as an appended zip), or one that inflates past the bound, fails the same way. Drop such files, or unpack them upstream. `--allow-opaque <glob>` lets one ship unscanned, and needs the owner's sign-off; it never hides a hit in content the gate did read.

What gate (c) does **not** read:

- Text that exists only as pixels, such as a screenshot of a wallet address. Pin such a file with a `sha256:` rule and by path.
- Compressed data inside other binary formats: WOFF and WOFF2 font tables, PDF streams, PNG `iCCP` profiles and image data, and the like.
- Text in other encodings (UTF-32 is only seen through the NUL-stripped view, Shift-JIS and the like not at all).
- Encodings applied twice or chained (base64 of percent-encoded text, base64 written with JSON escapes), base64 split across lines or glued to other base64-alphabet characters, hex dumps, quoted-printable, and anything a program assembles at run time (string concatenation, `String.fromCharCode`, a reversed or XOR-ed literal). A rule only sees what one decoding step leaves on one line.

**Missing scanners.** If `gitleaks` or `trufflehog` is not installed, gate (a) or (b) fails. Passing `--allow-missing-scanner` turns that into a loud WARN for dry runs. Never publish an export produced this way.

The scanners are looked up in `$GITLEAKS_BIN` and `$TRUFFLEHOG_BIN`, then on `PATH`. Tested versions are gitleaks 8.21 and trufflehog 3.82.

> **trufflehog flag gotcha.** In trufflehog 3.82, `--only-verified=false` is parsed as `--only-verified`, which silently hides every unverified result. On the same file, `--only-verified=false` returned 0 findings and `--no-only-verified` returned 2. The gate uses `--no-only-verified`. Do not "simplify" it.

## Configuration (private)

The lists name the very strings that must not ship, so none of them may be in the public tree. The script looks for each file in this order:

1. The explicit flag: `--forbidden`, `--drop`, `--scrub` or `--secrets-allow`.
2. `--config-dir DIR`. When this is given, the script reads all four files from `DIR` only.
3. `<parent of the main checkout>/oss-private/<name>`. For `/Users/x/payverge` that is `/Users/x/oss-private/`.
4. `<ref>:docs/superpowers/oss-export/<name>`. The script copies the file out of the archive before the drop step, which then removes it.

`forbidden.txt` is mandatory: the script refuses to export without it. The other three files are optional. A config file that resolves to a path inside the outdir is rejected.

### Line grammar shared by all files

- Blank lines and lines starting with `#` are ignored.
- Options follow the body after ` ;; `, as `key=value` pairs separated by spaces. Commas separate the values of a list.
- Paths and globs are relative to the tree root and anchored there. A path that names a directory covers everything below it.
- Globs support `*`, `**`, `?` and `{a,b}`.

### `forbidden.txt`

| Line form | Meaning |
|---|---|
| `text` | Literal, case-insensitive |
| `lit:Text` | Literal, case-sensitive |
| `re:regex` | JavaScript regex, case-sensitive |
| `rei:regex` | JavaScript regex, case-insensitive |
| `sha256:<64 lowercase hex>` | Any file whose bytes hash to this value, under any name or path. The hit is reported as `file:blob` |

Text rules see the whole path, or the whole file, as one string without the `m` flag. A `re:` rule anchored with `^...$` therefore matches paths only: no file body is a single bare path. Use this to forbid a file by name without also catching code that references it.

`sha256:` rules are for files whose content no text rule can see, such as screenshots that show personal data in their pixels. Several `sha256:` lines may share one name. They take no `allow=` or `only=`, and the hash of the empty file is rejected. Re-encoding a file changes its hash, so pin such a file by path as well until it has been replaced.

Each rule accepts these options:

- `name=<label>`: the name shown in reports. Hits print only `file:line [name]`, so a name must not repeat the value.
- `allow=<globs>`: paths where the rule does not apply.
- `only=<globs>`: the only paths where the rule applies.

### `drop.txt`

One path or glob per line.

### `scrub.rules`

```
s<d>PATTERN<d>REPLACEMENT<d>FLAGS   ;; name=<label> only=<globs> exclude=<globs>
```

- `<d>` is the delimiter. It can be any character except a letter, a digit, whitespace or a backslash.
- `PATTERN` is a JavaScript regex applied to the whole file. Use the `m` flag if it needs `^` or `$` to match at line boundaries.
- `REPLACEMENT` treats `\x` as a literal `x`, and expands `$1`, `${1}`, `$&` and `$<name>`.
- `FLAGS` can be any of `gimsu`.
- Rules run from top to bottom.

Keep scrubs mechanical: paths, org renames and fixture addresses. Content fixes belong upstream, so the public repo matches what actually runs.

### `secrets-allow.txt`

```
<gitleaks|trufflehog> <rule-id> <path> sha256:<hex of the matched value>
```

Each line is a reviewed synthetic fixture, pinned by the hash of its value, so changing the value makes the gate fire again. After a run, copy lines from `secrets-allow.candidates.txt`, but only the ones you have confirmed are fake.

## Procedure

```bash
# 0. The licence branch must be merged so LICENSE exists (gate h).
#    oss/main must contain the upstream Phase A fixes.

# 1. Dry run. Nothing is committed and the gates report what still leaks.
tools/oss-export/export.sh oss/main /tmp/payverge-public \
  --author-email stdevMac@users.noreply.github.com --no-commit

# 2. Fix upstream (preferred) or extend the private config, then rerun into
#    a fresh directory. The outdir must be empty.
rm -rf /tmp/payverge-public /tmp/payverge-public.export-report
tools/oss-export/export.sh oss/main /tmp/payverge-public \
  --author-email stdevMac@users.noreply.github.com
```

The author string `Name <email>` and the `--message` text are also checked against `forbidden.txt`, which keeps a personal address out of the public history. Use the GitHub noreply address. The scanners of gates (a) and (b) read only the tree, not the message, so do not paste anything secret into it.

If you want a signed commit, re-sign it yourself after the export:

```bash
git -C /tmp/payverge-public commit --amend --no-edit -S
```

## Post-export checks (optional, recommended before the first push)

```bash
cd /tmp/payverge-public
(cd backend && go build ./... && go vet ./... && go test -short ./...)
(cd frontend && npm ci && npm run typecheck && npm run build)
```

These checks are not gates, because they take minutes and need network access for `npm ci`. Run them before every first publish and after any large drop-list change.

## Owner-only push step (never run by tools or agents)

`export.sh` has no push code and rejects `--push`. After you have reviewed the tree and the reports, run this yourself:

```bash
cd /tmp/payverge-public
git remote add origin git@github.com:stdevMac/payverge.git && git push -u origin main
```

Create the GitHub repository empty, with no README, licence or .gitignore, so `main` is accepted as is.

## Tests

```bash
tools/oss-export/test.sh
```

The tests build a tiny fixture repository in a temporary directory, with stub scanners, and check that each gate fails when it should and passes on a clean tree. Set `OSS_EXPORT_REAL_GITLEAKS=/path/to/gitleaks` to also run one check against real gitleaks, and `OSS_EXPORT_BASH=/bin/bash` to run the end-to-end cases under macOS bash 3.2.
