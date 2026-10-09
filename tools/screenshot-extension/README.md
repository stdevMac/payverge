# Payverge Marketing Screenshot Extension

One-click curated marketing screenshots, captured from your own logged-in
Chrome session against real data.

## Install (unpacked)

1. Open `chrome://extensions`.
2. Enable **Developer mode** (top right).
3. Click **Load unpacked** and select this folder (`tools/screenshot-extension/`).
4. Pin the extension so its icon is visible in the toolbar.

## Use

1. Log into the operator dashboard for the business you want to shoot, so the
   active tab is on `…/business/<id>/dashboard`.
2. **Size your Chrome window** to the dimensions you want for the shots — the
   extension captures the visible viewport at your screen's pixel ratio; it does
   not force a viewport.
3. Click the extension icon, tick the shots you want, click **Run**.
4. Screenshots land in `Downloads/payverge-shots/<YYYY-MM-DD>/`.

The guest-menu and storefront shots point at the **real business you're logged
into**: the extension resolves that business's `custom_url` and a live
`table_code` from the API using your session cookie (start the run from a
`…/business/<id>/dashboard` tab so the business id is known). If resolution
fails, those shots are **skipped** with a reason in the popup log — never shot
against a placeholder page. To force specific values, set `customUrl` /
`tableCode` in `shots.js` (the `defaults` export).

## Add or change shots

Edit `shots.js`. Each shot is one object: `name`, `url` (with `{businessId}` /
`{tableCode}` / `{customUrl}` tokens), optional `waitFor` selector, optional
`settleMs`, optional `action`. Reload the extension in `chrome://extensions`
after editing.

## Development

```bash
cd tools/screenshot-extension
npm install
npm test   # jest unit tests for lib/ (template, filename, readiness)
```

## Notes / limits

- **You must be logged in.** If the session expired, the app bounces
  `/business/…` to the signed-out `/dashboard` page; the run detects this,
  marks those shots `error: redirected …` and does NOT capture the login
  screen. Log in and re-run.
- The target tab is discovered automatically: the active tab if it's on a
  business dashboard, else the most recent `/business/<id>/dashboard` tab in
  any window. If none exists the run errors out immediately.
- The run **brings the target tab to the front before each capture** (it must be
  visible to be captured). Let it run — it will keep pulling its window forward.
  At the end it navigates the tab back to where it started.
- The popup closes when the run starts (focus change) — reopen it after the
  run to see the persisted log of the last run, including skip/error reasons.
- Hands-free run: open `chrome-extension://<id>/popup.html?autorun=1` in a
  normal tab; it runs all shots and the tab stays open showing live progress.
- Cookie consent is seeded as **declined (non-essential)** at run start so the
  banner never overlays guest/storefront shots. An existing choice is
  respected.
- Readiness = no `[aria-busy="true"]` gate, no *large* `animate-pulse`
  skeleton block (small pulsing live-dots are ignored), plus the shot's
  optional `waitFor`. If a screen doesn't settle within ~20s the shot is still
  captured and marked `warned` in the popup log — re-run that one if the frame
  is imperfect.
- Captures the visible viewport only (above-the-fold hero shots). No full-page
  stitching in v1.
- No toolbar icon image ships; Chrome shows a default. Drop `icons/` + an
  `action.default_icon` entry in `manifest.json` later if you want branding.
