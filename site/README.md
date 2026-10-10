# payverge.io landing page

`site/` is the static page served at payverge.io. It describes the open-source project and links to the GitHub repository.

It has no build step, no framework and no JavaScript dependencies. There is one small inline script, and all it does is copy the install command. Any static host can serve it.

> **Launch status.** The public repository and the v1.0.0 release are live, and every gate in `tools/launch-gates.json` is confirmed except `live-demo`. That gate waits on v1.0.1 (it contains the staff sign-in fix) being released and deployed to the demo host; its `needs` lists the remaining steps. Until it is confirmed, `node site/tools/check.mjs --launch` fails, and so does every deploy recipe below.
>
> Deploy payverge.io only through the gated recipes in this file: `node site/tools/check.mjs --launch --stage <dir>` followed by the upload, or the Cloudflare Pages Git build command, which runs the same check. Do not upload `site/` by hand. On 2026-10-09 the page was deployed before the gates were confirmed; that must not happen again.

```
site/
├── index.html          the landing page; critical CSS is inline
├── 404.html            not-found page, served by the host for unknown paths
├── robots.txt          allows all crawlers
├── .gitignore          ignores dist/, the local staging output
├── assets/
│   ├── logo.svg, favicon.ico, apple-touch-icon.png
│   ├── og-image.png    1200x630 social preview
│   ├── screenshots/    lazy-loaded tour images (webp), made by tools/screenshots/capture.mjs
│   └── fonts/          DM Sans and DM Serif Display (latin subsets, woff2) + OFL.txt
└── tools/
    ├── check.mjs           zero-dependency checks, launch gate and staging (Node 20+)
    ├── launch-gates.json   claims that must be confirmed before the page goes live
    └── og-image.html       source for assets/og-image.png
```

Only `index.html`, `404.html`, `robots.txt` and `assets/` are served. `README.md`, `tools/` and `.gitignore` stay in the repository; `check.mjs --stage` copies exactly the deployable set, and it fails if a new top-level file is not classified in its `DEPLOY` / `NOT_DEPLOYED` lists.

The page makes no third-party requests: fonts are self-hosted, it sets no cookies and it runs no analytics. Keep it that way. The Content-Security-Policy in `index.html` blocks anything that is not in this directory.

## Preview locally

```bash
python3 -m http.server --directory site 8000   # then open http://localhost:8000/
node site/tools/check.mjs                        # static checks; run after every edit
node site/tools/check.mjs --self-test            # proves each check fails on a known-bad copy
node site/tools/check.mjs --launch               # static checks + launch gates + live link probes
```

`check.mjs` accepts only `--launch`, `--stage <dir>` (or `--stage=<dir>`), `--self-test` and `--help`. Any other argument, including a typo such as `--lanuch`, prints the usage and exits with status 2 without checking or staging anything, so a script never mistakes a static-only run for a passed launch gate.

`check.mjs` exits with a non-zero status if any of these fail:

- a tag does not parse (attributes are read whether double-quoted, single-quoted or unquoted);
- a local link or asset is missing, or a reference leaves `site/`;
- an `#anchor`, `aria-*`, `for` or `url(#id)` reference points nowhere, or an id repeats;
- anything the page loads comes from a third party: a script, stylesheet, image, frame, `<use>`, preconnect, CSS `url()` or any CSS `@import`;
- a URL is `http://` or `javascript:`, an element has an inline `on*=` handler, or a link uses `target="_blank"`;
- an `<img>` lacks `alt` or `width`/`height`;
- an inline script does not match the CSP hash;
- the canonical, `og:*` or `twitter:image` URLs are not absolute https on one origin, or the image they name is missing;
- an asset is not used by a deployed page, or a top-level file is not classified as deployable or not;
- a `data-gate` marker and `tools/launch-gates.json` disagree, or one of a gate's claim `phrases` appears outside an element carrying that gate;
- the page, not counting fonts and screenshots, is over 150 KB;
- a screenshot under `assets/screenshots/` loads any way other than `<img loading="lazy">`, is over 150 KB, or the screenshots together are over 1.5 MB;
- a palette pair in light or dark mode falls below WCAG AA contrast (every `prefers-color-scheme: dark` block is read).

`--launch` also fails while any launch gate is unconfirmed, and when any outbound link or gate URL does not answer 2xx. `--stage <dir>` implies `--launch`, then copies the deployable files into `<dir>`.

## Deploy

All three options publish a staged copy, never `site/` itself, so `README.md`, `tools/` and the gate file are not served:

```bash
node site/tools/check.mjs --launch --stage site/dist   # refuses while a gate is open or a link is dead
```

`site/dist/` is gitignored. `--stage` only writes into an empty directory, a previous stage, or `site/dist`.

`index.html` uses relative asset paths, so it also works under a sub-path such as `https://<user>.github.io/payverge/`. `404.html` uses root-absolute paths (`/assets/...`), because hosts serve it at any depth. It only renders fully when the site is at a domain root.

### Option 1: GitHub Pages (from `/site` through a workflow)

GitHub Pages cannot serve a folder named `site/` directly; the "deploy from a branch" mode only supports `/` and `/docs`. Use the Actions source instead:

1. In the repository, go to **Settings → Pages → Build and deployment → Source** and choose **GitHub Actions**.
2. Add a workflow. For example, `.github/workflows/pages.yml`:

   ```yaml
   name: Landing page

   on:
     push:
       branches: [main]
       paths: ['site/**']
     workflow_dispatch:

   permissions:
     contents: read
     pages: write
     id-token: write

   concurrency:
     group: pages
     cancel-in-progress: false

   jobs:
     deploy:
       runs-on: ubuntu-latest
       environment:
         name: github-pages
         url: ${{ steps.deployment.outputs.page_url }}
       steps:
         - uses: actions/checkout@v4
         - uses: actions/setup-node@v4
           with:
             node-version: 22
         - run: node site/tools/check.mjs --self-test
         - run: node site/tools/check.mjs --launch --stage "$RUNNER_TEMP/payverge-site"
         - uses: actions/configure-pages@v5
         - uses: actions/upload-pages-artifact@v3
           with:
             path: ${{ runner.temp }}/payverge-site
         - id: deployment
           uses: actions/deploy-pages@v4
   ```

3. To use a custom domain, go to **Settings → Pages → Custom domain** and enter `payverge.io`. Then point DNS at GitHub Pages as described in GitHub's "Managing a custom domain for your GitHub Pages site" guide, and tick **Enforce HTTPS**.

This workflow is an example only. It is not committed to `.github/workflows/`.

### Option 2: Caddy `file_server`

Stage the page and copy it to the server, for example to `/srv/payverge-site`:

```bash
node site/tools/check.mjs --launch --stage site/dist && rsync -a --delete site/dist/ host:/srv/payverge-site/
```

Then serve it:

```caddyfile
payverge.io, www.payverge.io {
	root * /srv/payverge-site
	encode zstd gzip

	header {
		Strict-Transport-Security "max-age=31536000; includeSubDomains"
		X-Content-Type-Options "nosniff"
		Referrer-Policy "strict-origin-when-cross-origin"
		# frame-ancestors only works as a header; the rest of the CSP is in the page's <meta>.
		Content-Security-Policy "frame-ancestors 'none'"
		Permissions-Policy "camera=(), microphone=(), geolocation=()"
	}
	@static path /assets/*
	header @static Cache-Control "public, max-age=604800"

	file_server

	# Requires Caddy 2.8 or newer.
	handle_errors 404 {
		rewrite * /404.html
		file_server
	}
}
```

Old app URLs on payverge.io, such as `/login` or `/t/...`, fall through to `404.html`. That page explains that Payverge is now open source and links back. To send one path somewhere specific instead, add a `redir` before `file_server`, for example `redir /docs https://github.com/stdevMac/payverge/tree/main/docs 302`.

If the same Caddy instance also runs a self-hosted Payverge, give the app its own host name (for example `pos.example.com`, as in `docs/self-hosting/storage.md`) and keep a single `payverge.io` site block: Caddy rejects a config that defines the same site address twice.

### Option 3: Cloudflare Pages

**From Git.** In **Workers & Pages → Create → Pages → Connect to Git**, pick the repository and use these settings:

| Setting | Value |
|---|---|
| Framework preset | None |
| Build command | `node site/tools/check.mjs --launch --stage site/dist` |
| Build output directory | `site/dist` |
| Root directory | *(repository root, the default)* |
| Environment variable | `NODE_VERSION` = `22` |

The build fails, and nothing is published, while a launch gate is open. Cloudflare serves `404.html` for unknown paths automatically. Add `payverge.io` under the project's **Custom domains** tab.

**Direct upload, without connecting Git:**

```bash
node site/tools/check.mjs --launch --stage site/dist && npx wrangler pages deploy site/dist --project-name payverge-site
```

To add response headers on Cloudflare, put a `site/_headers` file next to `index.html`, add `_headers` to the `DEPLOY` list at the top of `tools/check.mjs` so it is staged, and use the same headers as the Caddy example.

## Editing the page

- **Copy.** Every feature claim on the page maps to code in this repository. Before adding one, check that the code actually does it, and say "beta" when it is not fully shipped yet. ARCA e-invoicing is an example. A claim that depends on work not yet on the public main branch gets a `data-gate="<id>"` marker and a gate in `tools/launch-gates.json`. That includes the `<meta>` descriptions and the text baked into the social image (`tools/og-image.html`).
- **Inline script.** The CSP in `index.html` allows the copy-button script by its SHA-256 hash. After editing the script, run `node site/tools/check.mjs`. It prints the new hash; paste that into the `script-src 'sha256-...'` value of the CSP `<meta>`. JSON-LD blocks are data, not scripts, so the CSP does not cover them.
- **Another domain.** Absolute URLs are only used where crawlers need them. To serve the page somewhere other than `https://payverge.io/`, update all of these in `index.html`:
  - the canonical link;
  - `og:url`;
  - `og:image`;
  - `twitter:image`;
  - the JSON-LD `url`.
- **Install command.** The "Deploy in one command" block runs `install.sh` from the latest GitHub release (`releases/latest/download/install.sh`), the release asset the release workflow publishes, never a file on a moving branch. The read-first link points at `releases/latest`, and the deploy links point at `docs/self-hosting`. Keep them in sync with the release workflow and the docs. The `install` gate lists the URL, so `--launch` probes it.
- **Colors.** The palette is a set of CSS custom properties on `:root`, with a dark palette under `prefers-color-scheme: dark`. `check.mjs` reads these tokens straight from the page. When you add a new text and background pairing, add it to the `PAIRS` table in `check.mjs`.
- **Fonts.** Use only the self-hosted woff2 files. The `DM Sans Fallback` and `DM Serif Display Fallback` faces size Arial and Georgia to match them, so swapping fonts causes no layout shift. If you change the fonts, recompute those overrides.

## Launch gates

`tools/launch-gates.json` lists every claim on the page that depends on work not yet merged to the public repository: the installer, "no third-party accounts", the AI spend controls, the licence files, the footer provenance, and a final pass over the feature cards. Each element that makes such a claim carries `data-gate="<id>"`, and `check.mjs` fails if a marker names an unknown gate or a gate has no marker. One element can carry several gates (`data-gate="licence-files zero-accounts"`), and a marker covers everything inside the element.

A gate can also list `phrases`: words only gated copy may use, such as `one command` for `install`, `Apache` for `licence-files` and `platform fee` for `feature-claims`. Matching ignores case and spacing. Wherever a phrase appears in a page, in text or in an attribute such as a `<meta>` description or `alt`, an enclosing element must carry that gate. Otherwise the plain static check fails and prints the file and line. A new sentence such as "no platform fees" in the hero therefore cannot skip the gate review. When you add a gate, give it the phrases that only its claim would use.

To publish:

1. Wait until the work in a gate's `needs` is on the public main branch or in a release.
2. Do what its `verify` says, against that branch or release.
3. Replace `"confirmed": null` with `{"by": "<name>", "on": "YYYY-MM-DD", "ref": "<commit or tag>", "note": "<what you checked>"}`. If the claim turned out false, change the copy instead.
4. Run `node site/tools/check.mjs --launch`. It also checks that every GitHub link on the page and every gate URL answers 2xx.
5. When all gates pass, delete the "Not ready to publish yet" note at the top of this file.

Leave the markers and the gate file in place after launch. The next time a claim depends on unmerged work, set that gate back to `null`.

### Network access during a deploy

`--stage` implies `--launch`, so every deploy, including a routine redeploy after launch, probes each outbound GitHub link and gate URL from the machine that runs it: your laptop, the Actions runner or the Cloudflare build. This is deliberate: a renamed doc or a missing release asset blocks the deploy instead of shipping a dead link.

GitHub sometimes answers unauthenticated requests from shared CI networks with 429 or 5xx. `check.mjs` therefore probes four URLs at a time. It retries 408, 425, 429, 5xx and network errors up to three attempts, honouring a `Retry-After` of up to 30 seconds and otherwise waiting 2 s, then 4 s. Any other status, such as a 404, fails at once. If GitHub keeps refusing, the deploy fails and nothing is published. There is no flag to skip the probes. Retry the build later, or run the direct-upload recipe (Option 3) from a machine that can reach GitHub.

### Regenerating the social image

`tools/og-image.html` is the source for `assets/og-image.png`.

1. Open it in Chrome.
2. Set the viewport to 1200x630 at device pixel ratio 1. In DevTools, use the device toolbar and a responsive size.
3. Run **Capture screenshot** from the command menu.
4. Quantize the PNG to about 128 colors to keep it near 40 KB.

The illustration is the sketch that used to be the hero of `index.html`. The hero now shows the storefront and QR-menu screenshots, so `og-image.html` keeps the only copy of that sketch. Edit it there.

## Licenses

The page is part of Payverge and is licensed under Apache-2.0, like the rest of the repository. The Payverge name and logo are covered by the trademark policy, not the code license.

DM Sans and DM Serif Display are licensed under the SIL Open Font License 1.1. See `assets/fonts/OFL.txt`.
