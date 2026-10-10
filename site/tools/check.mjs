#!/usr/bin/env node
// Zero-dependency checks, launch gate and staging for the static site in site/.
//
//   node site/tools/check.mjs                         static checks; run on every change
//   node site/tools/check.mjs --launch                + every launch gate confirmed, every outbound link live
//   node site/tools/check.mjs --launch --stage <dir>  + copy exactly the deployable files into <dir>
//                                                     (--stage=<dir> works too; --stage implies --launch)
//   node site/tools/check.mjs --self-test             prove each check fails on a known-bad copy of the site
// Any other argument, a typo included, is a usage error: exit 2, nothing checked or staged.
//
// Static checks, for every HTML file under site/:
//   - every start tag parses; attributes are read whether double-quoted, single-quoted or unquoted
//   - local references (href, src, srcset, CSS url()) point at files inside site/ that exist
//   - same-page fragment references (#id, aria-*, for, url(#id), <use href>) resolve; ids are unique
//   - nothing loads from a third party: no external script, stylesheet, image, font, frame, CSS url()
//     or @import; outbound https links are allowed only on <a>/<area> and a few <link rel> values
//   - no http:// or javascript: URLs, no inline event handlers, no target="_blank"
//   - images carry alt text and intrinsic size (no layout shift)
//   - the Content-Security-Policy hashes match every inline script
//   - crawler URLs (canonical, og:url, og:image, twitter:image) are absolute https on one origin,
//     and the images they name ship in site/
//   - every top-level entry of site/ is classified as deployable or not (see DEPLOY / NOT_DEPLOYED)
//   - every data-gate="..." claim marker names a gate in tools/launch-gates.json, and every gate
//     is referenced by a marker
//   - every claim phrase a gate lists appears only inside an element carrying that gate
//   - screenshots (assets/screenshots/) load only through <img loading="lazy">
// Then the page-weight budget and the WCAG contrast of the palette tokens, light and dark.
// Screenshots load after first paint, so they sit outside the page budget and have their own:
// a per-file cap and a total cap.
//
// --launch adds the publishing gate: every gate in tools/launch-gates.json must carry a
// "confirmed" record, and every outbound link plus every gate URL must answer 2xx (429, 5xx and
// network errors are retried with backoff first). --stage implies --launch, so the deploy
// recipes in site/README.md cannot publish a page whose claims have not been confirmed against
// the public repository.

import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import {
  cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, extname, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE_SITE = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const BUDGET_BYTES = 150 * 1024; // excluding web fonts and lazy screenshots
// The screenshot gallery: lazy images only, each and all of them capped.
const SHOTS_DIR = 'assets/screenshots/';
const SHOT_MAX_BYTES = 150 * 1024;
const SHOTS_BUDGET_BYTES = 1536 * 1024;
// The only top-level entries a host should serve. --stage copies exactly these.
const DEPLOY = ['index.html', '404.html', 'robots.txt', 'assets'];
// Top-level entries that live in site/ but must never be served.
const NOT_DEPLOYED = ['README.md', 'tools', '.gitignore', 'dist'];
const GATES_FILE = 'tools/launch-gates.json';

// ------------------------------------------------------------------ helpers
const isFont = (f) => ['.woff2', '.woff', '.ttf', '.otf'].includes(extname(f));

function walk(dir, top = true) {
  return readdirSync(dir).flatMap((name) => {
    if (top && name === 'dist') return []; // a local --stage output, not source
    const p = join(dir, name);
    return statSync(p).isDirectory() ? walk(p, false) : [p];
  });
}

const ENTITIES = { amp: '&', quot: '"', apos: "'", lt: '<', gt: '>', '#39': "'", '#x27': "'", '#47': '/', '#x2f': '/' };
const decode = (s) => s.replace(/&(amp|quot|apos|lt|gt|#39|#x27|#47|#x2f);/gi, (m, e) => ENTITIES[e.toLowerCase()] ?? m);

const ATTR_PART = String.raw`[^\s"'>\/=]+(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s"'=<>\x60]+))?`;
const TAG_RE = new RegExp(String.raw`<([a-zA-Z][a-zA-Z0-9:-]*)((?:\s+${ATTR_PART})*)\s*\/?>`, 'g');
const ATTR_RE = /([^\s"'>\/=]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+)))?/g;

function parseAttrs(src) {
  const attrs = new Map();
  for (const a of src.matchAll(ATTR_RE)) {
    const name = a[1].toLowerCase();
    if (!attrs.has(name)) attrs.set(name, decode(a[2] ?? a[3] ?? a[4] ?? ''));
  }
  return attrs;
}

// Both keep newlines, so an index into the result still maps to a source line.
const keepNewlines = (s) => s.replace(/[^\n]/g, '');
const stripComments = (html) => html.replace(/<!--[\s\S]*?-->/g, keepNewlines);
// Blank raw-text bodies so markup-looking strings inside <script>/<style> are not read as tags.
const blankRawText = (html) => html.replace(/(<(script|style)\b[^>]*>)([\s\S]*?)(<\/\2\s*>)/gi, (m, open, n, body, close) => open + keepNewlines(body) + close);
const lineAt = (s, index) => s.slice(0, index).split('\n').length;

function parseTags(html) {
  const scan = blankRawText(html);
  const tags = [...scan.matchAll(TAG_RE)].map((m) => ({ name: m[1].toLowerCase(), attrs: parseAttrs(m[2]), raw: m[0], at: m.index }));
  // Anything that opens like a tag but did not parse is a hole in every check below.
  const parsedAt = new Set(tags.map((t) => t.at));
  const unparsed = [...scan.matchAll(/<[a-zA-Z]/g)].filter((m) => !parsedAt.has(m.index))
    .map((m) => scan.slice(m.index, m.index + 60).replace(/\s+/g, ' '));
  return { tags, unparsed, scan };
}

// Where an element ends in `scan`: right after its start tag for void and self-closed elements,
// otherwise at its matching end tag (same-name nesting counted).
const VOID_TAGS = new Set(['area', 'base', 'br', 'col', 'embed', 'hr', 'img', 'input', 'link', 'meta', 'source', 'track', 'wbr']);
function elementEnd(scan, tag) {
  const afterStart = tag.at + tag.raw.length;
  if (VOID_TAGS.has(tag.name) || tag.raw.endsWith('/>')) return afterStart;
  const re = new RegExp(`<(/?)${tag.name}(?=[\\s/>])[^>]*>`, 'gi');
  re.lastIndex = afterStart;
  let depth = 1;
  for (let m; (m = re.exec(scan));) {
    if (m[1]) { if (--depth === 0) return m.index; } else if (!m[0].endsWith('/>')) depth++;
  }
  return scan.length;
}
const phraseRe = (p) => new RegExp(p.trim().split(/\s+/).map((w) => w.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('\\s+'), 'gi');

const SCHEME_RE = /^[a-z][a-z0-9+.-]*:/i;
const isExternal = (u) => SCHEME_RE.test(u) || u.startsWith('//');

// Outbound https links are allowed only where the browser does not load anything.
const NAV_LINK_RELS = new Set(['canonical', 'alternate', 'license', 'me', 'author', 'help', 'next', 'prev']);
function isNavigation(tag, attr) {
  if ((tag.name === 'a' || tag.name === 'area') && attr === 'href') return true;
  if (tag.name === 'link' && attr === 'href') {
    const rels = (tag.attrs.get('rel') || '').toLowerCase().split(/\s+/).filter(Boolean);
    return rels.length > 0 && rels.every((r) => NAV_LINK_RELS.has(r));
  }
  return false;
}
const URL_ATTRS = ['href', 'xlink:href', 'src', 'srcset', 'imagesrcset', 'poster', 'action', 'formaction', 'data', 'background', 'manifest', 'ping'];
const FRAGMENT_ATTRS = ['aria-labelledby', 'aria-describedby', 'aria-controls', 'aria-owns', 'aria-activedescendant', 'aria-details', 'aria-errormessage', 'for', 'list', 'data-copy'];

const CSS_URL_RE = /url\(\s*(?:"([^"]*)"|'([^']*)'|([^)\s]*))\s*\)/gi;

function resolveLocal(site, fromFile, ref) {
  const clean = ref.split('#')[0].split('?')[0];
  if (clean === '') return null;
  let target = clean.startsWith('/') ? join(site, clean) : resolve(dirname(fromFile), clean);
  if (clean.endsWith('/') || (existsSync(target) && statSync(target).isDirectory())) target = join(target, 'index.html');
  return target;
}

// ------------------------------------------------------------------ contrast
function parseTokens(css) {
  const out = {};
  for (const m of css.matchAll(/--([a-z0-9-]+)\s*:\s*(#[0-9a-f]{3,8})\b/gi)) out[m[1]] = m[2].toLowerCase();
  return out;
}
function blockEnd(css, open) {
  let depth = 0;
  for (let i = open; i < css.length; i++) {
    if (css[i] === '{') depth++;
    else if (css[i] === '}' && --depth === 0) return i;
  }
  return css.length;
}
// Light tokens: every top-level `:root { }` outside @media. Dark tokens: light, overridden by
// every `@media (prefers-color-scheme: dark)` block, in source order.
function themes(css) {
  css = css.replace(/\/\*[\s\S]*?\*\//g, '');
  let outside = '';
  const darkBlocks = [];
  let i = 0;
  for (const m of css.matchAll(/@media[^{]*\{/g)) {
    if (m.index < i) continue; // nested inside a block already consumed
    const open = m.index + m[0].length - 1;
    const end = blockEnd(css, open);
    outside += css.slice(i, m.index);
    if (/prefers-color-scheme\s*:\s*dark/i.test(m[0])) darkBlocks.push(css.slice(open + 1, end));
    i = end + 1;
  }
  outside += css.slice(i);
  const light = {};
  for (const m of outside.matchAll(/(^|[}\s;,]):root\s*\{/g)) {
    const open = m.index + m[0].length - 1;
    Object.assign(light, parseTokens(outside.slice(open + 1, blockEnd(outside, open))));
  }
  const dark = darkBlocks.length ? Object.assign({ ...light }, ...darkBlocks.map(parseTokens)) : null;
  return { light, dark };
}
function luminance(hex) {
  let h = hex.slice(1);
  if (h.length === 3) h = [...h].map((c) => c + c).join('');
  const [r, g, b] = [0, 2, 4].map((k) => parseInt(h.slice(k, k + 2), 16) / 255)
    .map((c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}
function ratio(a, b) {
  const [l1, l2] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (l1 + 0.05) / (l2 + 0.05);
}

// [foreground, background, minimum ratio, what it is]
const TEXT = 4.5;
const UI = 3; // non-text: focus rings, icons, status dots (WCAG 1.4.11)
const PAIRS = {
  'index.html': [
    ['text', 'bg', TEXT, 'body text'], ['text', 'bg-alt', TEXT, 'text on alt sections'],
    ['text', 'surface', TEXT, 'card titles'], ['text', 'surface-2', TEXT, 'text on tinted panels'],
    ['muted', 'bg', TEXT, 'secondary text'], ['muted', 'bg-alt', TEXT, 'secondary text on alt sections'],
    ['muted', 'surface', TEXT, 'card body'], ['muted', 'surface-2', TEXT, 'meta on tinted panels'],
    ['primary', 'bg', TEXT, 'links and eyebrows'], ['primary', 'bg-alt', TEXT, 'links on alt sections'],
    ['primary', 'surface', TEXT, 'links on cards'], ['primary', 'primary-soft', TEXT, 'icons and chips on tint'],
    ['primary-hover', 'bg', TEXT, 'hovered links'],
    ['on-primary', 'primary', TEXT, 'primary button'], ['on-primary', 'primary-hover', TEXT, 'hovered primary button'],
    ['band-text', 'band-bg', TEXT, 'AI band headings'], ['band-muted', 'band-bg', TEXT, 'AI band body'],
    ['band-accent', 'band-bg', TEXT, 'AI band accent line'], ['band-chip', 'band-bg', TEXT, 'AI band pills and labels'],
    ['subtle', 'bg', TEXT, 'captions and step labels'], ['subtle', 'surface', TEXT, 'captions on cards'],
    ['subtle', 'surface-2', TEXT, 'browser frame address'], ['brand-ink', 'bg', TEXT, 'wordmark'],
    ['tile-text', 'ink-tile', TEXT, 'dark bento tile body'], ['tile-muted', 'ink-tile', TEXT, 'dark bento tile footer'],
    ['tile-live', 'ink-tile', TEXT, 'dark bento tile label'],
    ['teal-text', 'teal-tile', TEXT, 'teal bento tile body'], ['teal-text', 'teal-tile-2', TEXT, 'teal bento tile body (gradient end)'],
    ['teal-muted', 'teal-tile', TEXT, 'teal bento tile label'], ['teal-muted', 'teal-tile-2', TEXT, 'teal bento tile label (gradient end)'],
    ['footer-text', 'footer-bg', TEXT, 'footer text'], ['footer-muted', 'footer-bg', TEXT, 'footer links and notes'],
    ['live', 'surface', UI, 'live status dot'],
    ['code-text', 'code-bg', TEXT, 'terminal command'], ['code-muted', 'code-bg', TEXT, 'terminal labels'],
    ['focus', 'bg', UI, 'focus ring'], ['focus', 'bg-alt', UI, 'focus ring on alt sections'], ['focus', 'surface', UI, 'focus ring on cards'],
    ['code-text', 'code-bg', UI, 'focus ring in terminal'],
    ['accent', 'surface', UI, 'timeline node icon'], ['border-strong', 'bg', 1.4, 'ghost button outline (decorative; text carries the meaning)'],
  ],
  '404.html': [
    ['text', 'bg', TEXT, 'heading'], ['muted', 'bg', TEXT, 'body'], ['primary', 'bg', TEXT, 'status code'],
    ['on-primary', 'primary', TEXT, 'primary button'], ['on-primary', 'primary-hover', TEXT, 'hovered primary button'],
    ['focus', 'bg', UI, 'focus ring'],
  ],
};

// ------------------------------------------------------------------ launch gates
function loadGates(site, fail) {
  const file = join(site, GATES_FILE);
  if (!existsSync(file)) { fail(GATES_FILE, 'missing'); return []; }
  let gates;
  try { gates = JSON.parse(readFileSync(file, 'utf8')).gates; } catch (e) { fail(GATES_FILE, `invalid JSON: ${e.message}`); return []; }
  if (!Array.isArray(gates)) { fail(GATES_FILE, 'expected {"gates": [...]}'); return []; }
  const seen = new Set();
  for (const g of gates) {
    const where = `${GATES_FILE} gate "${g.id}"`;
    if (typeof g.id !== 'string' || !/^[a-z0-9-]+$/.test(g.id)) fail(GATES_FILE, `gate id must be kebab-case: ${JSON.stringify(g.id)}`);
    if (seen.has(g.id)) fail(GATES_FILE, `duplicate gate id "${g.id}"`);
    seen.add(g.id);
    for (const k of ['claim', 'needs', 'verify']) if (typeof g[k] !== 'string' || !g[k].trim()) fail(where, `needs a non-empty "${k}"`);
    if (g.urls !== undefined && (!Array.isArray(g.urls) || g.urls.some((u) => !/^https:\/\//.test(u)))) fail(where, '"urls" must be a list of https URLs');
    if (g.phrases !== undefined && (!Array.isArray(g.phrases) || g.phrases.some((p) => typeof p !== 'string' || !p.trim()))) fail(where, '"phrases" must be a list of non-empty strings');
    if (g.confirmed !== null) {
      const c = g.confirmed || {};
      if (!c.by || !/^\d{4}-\d{2}-\d{2}$/.test(c.on || '') || !c.ref) fail(where, '"confirmed" must be null or {"by", "on": "YYYY-MM-DD", "ref": "<public commit or tag>", "note"}');
    }
  }
  return gates;
}

// ------------------------------------------------------------------ static checks
export function checkSite(site) {
  const failures = [];
  const notes = [];
  const contrastRows = [];
  const fail = (file, msg) => failures.push(`${file}: ${msg}`);
  const rel = (p) => relative(site, p).split(sep).join('/');

  const allFiles = walk(site);
  const htmlFiles = allFiles.filter((f) => f.endsWith('.html'));
  const isDeployed = (f) => DEPLOY.includes(rel(f).split('/')[0]);
  const deployHtml = htmlFiles.filter(isDeployed);

  // Every top-level entry is classified, so --stage never silently drops or ships a file.
  for (const name of readdirSync(site)) {
    if (!DEPLOY.includes(name) && !NOT_DEPLOYED.includes(name)) fail(name, 'is not in the deploy list; add it to DEPLOY or NOT_DEPLOYED in tools/check.mjs');
  }
  for (const name of DEPLOY) if (!existsSync(join(site, name))) fail(name, 'listed in DEPLOY but missing');
  const gates = loadGates(site, fail);

  const loadsOf = new Map(); // html file -> local files a browser loads to render it
  const linked = new Set(); // local files reached by navigation
  const crawlerAssets = new Set();
  const outbound = new Set(); // https links to probe at launch
  const gateRefs = new Map(); // gate id -> [file]
  let canonicalOrigin = null;

  for (const file of htmlFiles) {
    const name = rel(file);
    const raw = readFileSync(file, 'utf8');
    const html = stripComments(raw);
    const loads = new Set();
    loadsOf.set(file, loads);
    const deployed = isDeployed(file);
    const { tags, unparsed, scan } = parseTags(html);
    for (const u of unparsed) fail(name, `could not parse tag near "${u}"`);
    const byName = (n) => tags.filter((t) => t.name === n);

    // Claim phrases: wherever a gate's phrase appears (text or attribute, scripts and comments
    // excluded), an enclosing element must carry that gate. Runs on every HTML file, including
    // tools/og-image.html, whose text ships inside assets/og-image.png.
    const gated = tags.filter((t) => t.attrs.has('data-gate'))
      .map((t) => ({ ids: new Set(t.attrs.get('data-gate').split(/\s+/)), start: t.at, end: elementEnd(scan, t) }));
    for (const g of gates) {
      for (const phrase of Array.isArray(g.phrases) ? g.phrases : []) {
        for (const m of scan.matchAll(phraseRe(phrase))) {
          if (gated.some((r) => r.ids.has(g.id) && r.start <= m.index && m.index < r.end)) continue;
          fail(name, `line ${lineAt(scan, m.index)}: "${m[0].replace(/\s+/g, ' ')}" is a claim of launch gate "${g.id}" but no enclosing element carries data-gate="${g.id}" (mark the claim or reword it)`);
        }
      }
    }

    // Basics every page needs.
    if (!/^<!doctype html>/i.test(raw.trimStart())) fail(name, 'missing <!doctype html>');
    if (!/^[a-z]{2}/i.test(byName('html')[0]?.attrs.get('lang') || '')) fail(name, '<html> has no lang attribute');
    if (!/<title>[^<]+<\/title>/i.test(html)) fail(name, 'missing <title>');
    if (!byName('meta').some((t) => t.attrs.get('name') === 'viewport')) fail(name, 'missing viewport meta');

    // Raw CSS: <style> bodies, style="" attributes, and any local stylesheet.
    const cssSources = [
      ...[...html.matchAll(/<style\b[^>]*>([\s\S]*?)<\/style\s*>/gi)].map((m) => ({ css: m[1], from: file })),
      ...tags.filter((t) => t.attrs.has('style')).map((t) => ({ css: t.attrs.get('style'), from: file })),
    ];

    // Ids and fragment references.
    const ids = tags.filter((t) => t.attrs.has('id')).map((t) => t.attrs.get('id'));
    const idSet = new Set(ids);
    for (const id of ids.filter((id, k) => ids.indexOf(id) !== k)) fail(name, `duplicate id "${id}"`);
    const fragRefs = [];

    for (const tag of tags) {
      for (const [attr, value] of tag.attrs) {
        if (/^on[a-z]+$/.test(attr)) fail(name, `inline event handler ${attr}= on <${tag.name}> (the CSP blocks it; use the hashed script)`);
        for (const m of value.matchAll(/url\(\s*['"]?#([^)'"\s]+)/g)) fragRefs.push(m[1]);
        if (FRAGMENT_ATTRS.includes(attr)) fragRefs.push(...value.split(/\s+/).filter(Boolean));
        if (attr === 'data-gate') for (const g of value.split(/\s+/).filter(Boolean)) gateRefs.set(g, [...(gateRefs.get(g) || []), name]);
      }
      if (tag.attrs.get('target') === '_blank') fail(name, 'target="_blank" used (open links in the same tab)');

      for (const attr of URL_ATTRS) {
        if (!tag.attrs.has(attr)) continue;
        const value = tag.attrs.get(attr).trim();
        const refs = /srcset$/.test(attr) ? value.split(',').map((s) => s.trim().split(/\s+/)[0]).filter(Boolean) : [value];
        for (const ref of refs) {
          const what = `<${tag.name} ${attr}="${ref}">`;
          if (ref.startsWith('#')) { fragRefs.push(ref.slice(1)); continue; }
          const nav = isNavigation(tag, attr);
          if (/^javascript:/i.test(ref)) { fail(name, `javascript: URL in ${what}`); continue; }
          if (/^http:\/\//i.test(ref)) { fail(name, `insecure http:// URL in ${what}`); continue; }
          if (/^data:/i.test(ref)) { if (nav) fail(name, `data: URL as a link in ${what}`); continue; }
          if (isExternal(ref)) {
            if (nav && /^https:\/\//i.test(ref)) { if (deployed) outbound.add(ref.split('#')[0]); continue; }
            if (nav && /^(mailto|tel):/i.test(ref)) continue;
            fail(name, `third-party or non-https resource ${what} (everything the page loads must ship in site/)`);
            continue;
          }
          const target = resolveLocal(site, file, ref);
          if (!target) continue;
          if (target !== site && !target.startsWith(site + sep)) { fail(name, `reference escapes site/: ${what}`); continue; }
          if (!existsSync(target)) { fail(name, `missing local file for ${what} (${rel(target)})`); continue; }
          if (nav) linked.add(target);
          else loads.add(target);
          if (rel(target).startsWith(SHOTS_DIR) && !(tag.name === 'img' && !nav && (tag.attrs.get('loading') || '').toLowerCase() === 'lazy')) {
            fail(name, `screenshot ${rel(target)} must load through <img loading="lazy"> (screenshots are outside the page budget): ${what}`);
          }
          const rels = (tag.attrs.get('rel') || '').toLowerCase().split(/\s+/);
          if (tag.name === 'link' && rels.includes('stylesheet')) cssSources.push({ css: readFileSync(target, 'utf8'), from: target });
        }
      }

      // Images: alt text and intrinsic size.
      if (tag.name === 'img') {
        if (!tag.attrs.has('alt')) fail(name, `<img> without alt: ${tag.raw}`);
        if (!/^\d+$/.test(tag.attrs.get('width') || '') || !/^\d+$/.test(tag.attrs.get('height') || '')) fail(name, `<img> without width/height: ${tag.raw}`);
      }
    }

    // CSS: no @import, no url() outside site/, local url()s exist.
    for (const { css, from } of cssSources) {
      const body = css.replace(/\/\*[\s\S]*?\*\//g, '');
      if (/@import\b/i.test(body)) fail(name, `CSS @import in ${rel(from)} (inline the CSS; imports block rendering and can load from third parties)`);
      for (const m of body.matchAll(CSS_URL_RE)) {
        const ref = (m[1] ?? m[2] ?? m[3] ?? '').trim();
        if (ref.startsWith('#')) { fragRefs.push(ref.slice(1)); continue; }
        if (ref === '' || /^data:/i.test(ref)) continue;
        if (isExternal(ref)) { fail(name, `CSS url(${ref}) loads from outside site/`); continue; }
        const target = resolveLocal(site, from, ref);
        if (!target) continue;
        if (!target.startsWith(site + sep)) { fail(name, `CSS url(${ref}) escapes site/`); continue; }
        if (!existsSync(target)) fail(name, `missing local file for CSS url(${ref}) (${rel(target)})`);
        else loads.add(target);
      }
    }

    for (const id of fragRefs) {
      if (id === '' || id === 'top') continue;
      if (!idSet.has(id)) fail(name, `fragment reference "#${id}" has no matching id`);
    }

    // CSP: every executable inline script must be allowed by a hash.
    const csp = byName('meta').find((t) => (t.attrs.get('http-equiv') || '').toLowerCase() === 'content-security-policy')?.attrs.get('content');
    const scripts = [...html.matchAll(/<script\b([^>]*)>([\s\S]*?)<\/script\s*>/gi)].map((m) => ({ attrs: parseAttrs(m[1]), body: m[2] }));
    const inlineScripts = scripts.filter((s) => !s.attrs.has('src') && !/^application\/(ld\+)?json$/i.test(s.attrs.get('type') || '')).map((s) => s.body);
    if (deployed && !csp) fail(name, 'missing Content-Security-Policy meta');
    if (csp) {
      if (/'unsafe-eval'/.test(csp)) fail(name, "CSP allows 'unsafe-eval'");
      const scriptSrc = (csp.match(/script-src ([^;]+)/) || [])[1] || '';
      if (/'unsafe-inline'/.test(scriptSrc)) fail(name, "CSP script-src allows 'unsafe-inline'");
      if (scripts.some((s) => s.attrs.has('src')) && !/'self'/.test(scriptSrc)) fail(name, "a <script src> is present but CSP script-src does not allow 'self'");
      const allowed = new Set([...scriptSrc.matchAll(/'sha256-([^']+)'/g)].map((m) => m[1]));
      const actual = inlineScripts.map((s) => createHash('sha256').update(s, 'utf8').digest('base64'));
      for (const h of actual) if (!allowed.has(h)) fail(name, `inline script hash sha256-${h} is not in the CSP (update script-src)`);
      for (const h of allowed) if (!actual.includes(h)) fail(name, `CSP lists sha256-${h} but no inline script matches it`);
    }

    // Crawler URLs: absolute https, one origin, and the images ship in site/.
    const crawler = [
      ...tags.filter((t) => t.name === 'link' && (t.attrs.get('rel') || '').toLowerCase() === 'canonical').map((t) => ['canonical', t.attrs.get('href') || '']),
      ...byName('meta').map((t) => [t.attrs.get('property') || t.attrs.get('name') || '', t.attrs.get('content') || ''])
        .filter(([k]) => /^(og:url|og:image|og:image:url|og:image:secure_url|twitter:image)$/.test(k)),
    ];
    const canonical = crawler.find(([k, u]) => k === 'canonical' && /^https:\/\//.test(u));
    const pageOrigin = canonical ? new URL(canonical[1]).origin : null;
    if (pageOrigin && deployed) canonicalOrigin ??= pageOrigin;
    for (const [key, url] of crawler) {
      if (!/^https:\/\//.test(url)) { fail(name, `${key} must be an absolute https URL: ${url}`); continue; }
      if (pageOrigin && new URL(url).origin !== pageOrigin) fail(name, `${key} ${url} is not on the canonical origin ${pageOrigin}`);
      if (/image/.test(key)) {
        const local = join(site, decodeURIComponent(new URL(url).pathname));
        if (!existsSync(local)) fail(name, `${key} ${url} has no matching file in site/`);
        else crawlerAssets.add(local);
      }
    }
  }
  // Links back to the site's own domain cannot be probed before it is live.
  if (canonicalOrigin) for (const u of [...outbound]) if (new URL(u).origin === canonicalOrigin) outbound.delete(u);

  // Every shipped asset is used by a deployed page (the font license excepted).
  const used = new Set([...deployHtml.flatMap((f) => [...loadsOf.get(f)]), ...linked, ...crawlerAssets]);
  for (const f of allFiles.filter((f) => rel(f).startsWith('assets/') && !rel(f).endsWith('OFL.txt'))) {
    if (!used.has(f)) fail(rel(f), 'not referenced by any deployed page (delete it or link it)');
  }

  // Weight budget.
  const size = (f) => statSync(f).size;
  const index = join(site, 'index.html');
  if (existsSync(index)) {
    const isShot = (f) => rel(f).startsWith(SHOTS_DIR);
    const pageFiles = new Set([index, ...loadsOf.get(index)]);
    const pageBytes = [...pageFiles].filter((f) => !isFont(f) && !isShot(f)).reduce((n, f) => n + size(f), 0);
    const fontBytes = [...pageFiles].filter(isFont).reduce((n, f) => n + size(f), 0);
    const deployBytes = allFiles.filter((f) => isDeployed(f) && !isFont(f) && !isShot(f) && !rel(f).endsWith('OFL.txt')).reduce((n, f) => n + size(f), 0);
    const shots = allFiles.filter(isShot);
    const shotBytes = shots.reduce((n, f) => n + size(f), 0);
    if (shots.length) notes.push(`${shots.length} lazy screenshots: ${(shotBytes / 1024).toFixed(1)} KB (budget ${SHOTS_BUDGET_BYTES / 1024} KB, ${SHOT_MAX_BYTES / 1024} KB each)`);
    for (const f of shots) if (size(f) > SHOT_MAX_BYTES) fail(rel(f), `screenshot is ${size(f)} B, over ${SHOT_MAX_BYTES} B (re-encode it smaller)`);
    if (shotBytes > SHOTS_BUDGET_BYTES) fail('site', `screenshots total ${shotBytes} B exceeds ${SHOTS_BUDGET_BYTES} B`);
    notes.push(`index.html and everything it loads: ${(pageBytes / 1024).toFixed(1)} KB excluding fonts (+${(fontBytes / 1024).toFixed(1)} KB fonts)`);
    notes.push(`all deployable files (${DEPLOY.join(', ')}): ${(deployBytes / 1024).toFixed(1)} KB excluding fonts`);
    if (pageBytes > BUDGET_BYTES) fail('index.html', `page weight ${pageBytes} B exceeds ${BUDGET_BYTES} B`);
    if (deployBytes > BUDGET_BYTES) fail('site', `deployable files total ${deployBytes} B exceeds ${BUDGET_BYTES} B`);
  }

  // Contrast.
  for (const [page, pairs] of Object.entries(PAIRS)) {
    const file = join(site, page);
    if (!existsSync(file)) continue;
    const css = [...readFileSync(file, 'utf8').matchAll(/<style\b[^>]*>([\s\S]*?)<\/style\s*>/gi)].map((m) => m[1]).join('\n');
    const { light, dark } = themes(css);
    for (const [themeName, tokens] of [['light', light], ['dark', dark]]) {
      if (!tokens) { fail(page, `no ${themeName} palette found`); continue; }
      for (const [fg, bg, min, what] of pairs) {
        if (!tokens[fg] || !tokens[bg]) { fail(page, `${themeName}: token --${!tokens[fg] ? fg : bg} not found`); continue; }
        const r = ratio(tokens[fg], tokens[bg]);
        contrastRows.push(`${page.padEnd(10)} ${themeName.padEnd(5)} ${(`--${fg}`).padEnd(16)} on ${(`--${bg}`).padEnd(14)} ${tokens[fg]} / ${tokens[bg]}  ${r.toFixed(2).padStart(5)}:1  min ${min}  ${r >= min ? 'ok' : 'FAIL'}  ${what}`);
        if (r < min) fail(page, `${themeName}: --${fg} on --${bg} is ${r.toFixed(2)}:1, needs ${min}:1 (${what})`);
      }
    }
  }

  // Launch gates: markers and gate list agree.
  const gateIds = new Set(gates.map((g) => g.id));
  for (const [id, files] of gateRefs) if (!gateIds.has(id)) fail([...new Set(files)].join(', '), `data-gate="${id}" names an unknown launch gate (add it to ${GATES_FILE})`);
  for (const g of gates) if (!gateRefs.has(g.id)) fail(GATES_FILE, `gate "${g.id}" is not referenced by any data-gate marker (mark the claim or delete the gate)`);
  const open = gates.filter((g) => g.confirmed === null);
  notes.push(`launch gates: ${gates.length - open.length} of ${gates.length} confirmed${open.length ? ` (open: ${open.map((g) => g.id).join(', ')})` : ''}`);

  return { failures, notes, contrastRows, gates, outbound: [...outbound].sort(), htmlCount: htmlFiles.length };
}

// ------------------------------------------------------------------ launch
// One HEAD (falling back to GET) per URL. A non-numeric status means the request itself failed.
async function probeOnce(url) {
  for (const method of ['HEAD', 'GET']) {
    try {
      const res = await fetch(url, { method, redirect: 'follow', signal: AbortSignal.timeout(15000), headers: { 'user-agent': 'payverge-site-check' } });
      if (method === 'GET') await res.body?.cancel();
      if (res.ok) return { ok: true, status: res.status };
      if (method === 'HEAD' && [403, 405, 501].includes(res.status)) continue;
      return { ok: false, status: res.status, retryAfter: res.headers.get('retry-after') };
    } catch (e) {
      if (method === 'GET') return { ok: false, status: e.name || 'error' };
    }
  }
  return { ok: false, status: 'unreachable' };
}

// github.com can answer 429 or 5xx to unauthenticated probes, especially from shared CI egress.
// Those answers, and network errors, are retried with backoff (Retry-After honoured, capped);
// anything else, such as a 404, fails at once. A URL that never answers 2xx still fails, so a
// flaky network delays a deploy but never lets a dead link through.
const PROBE_ATTEMPTS = 3;
const isTransient = (status) => typeof status !== 'number' || status === 408 || status === 425 || status === 429 || status >= 500;
const sleepMs = (ms) => new Promise((r) => setTimeout(r, ms));
export async function probeWithRetry(url, { once = probeOnce, sleep = sleepMs, attempts = PROBE_ATTEMPTS } = {}) {
  let res;
  for (let k = 1; ; k++) {
    res = await once(url);
    if (res.ok || !isTransient(res.status) || k >= attempts) return { ...res, attempts: k };
    // Retry-After in delta-seconds only; an absent header or an HTTP date falls back to backoff.
    const hinted = /^\d+$/.test(String(res.retryAfter ?? '').trim()) ? Number(res.retryAfter) : null;
    await sleep(hinted !== null ? Math.min(hinted, 30) * 1000 : 2000 * 2 ** (k - 1));
  }
}

// Probe a few URLs at a time rather than all at once, which is what trips rate limits.
async function mapLimit(items, limit, fn) {
  const out = new Array(items.length);
  let next = 0;
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, async () => {
    while (next < items.length) { const i = next++; out[i] = await fn(items[i]); }
  }));
  return out;
}

export async function checkLaunch(result, probe = probeWithRetry) {
  const failures = [];
  for (const g of result.gates) {
    if (g.confirmed === null) failures.push(`launch gate "${g.id}" is not confirmed. Claim: ${g.claim} Needs: ${g.needs}`);
  }
  const urls = [...new Set([...result.outbound, ...result.gates.flatMap((g) => g.urls || [])])].sort();
  const probes = await mapLimit(urls, 4, async (u) => [u, await probe(u)]);
  const rows = probes.map(([u, p]) => `${p.ok ? 'ok  ' : 'FAIL'} ${String(p.status).padEnd(4)} ${u}${p.attempts > 1 ? ` (${p.attempts} attempts)` : ''}`);
  for (const [u, p] of probes) if (!p.ok) failures.push(`outbound URL ${u} answered ${p.status}`);
  return { failures, rows };
}

// ------------------------------------------------------------------ stage
export function stage(site, outDir) {
  const out = resolve(outDir);
  const insideSite = out.startsWith(site + sep);
  if (out === site || site.startsWith(out + sep) || (insideSite && relative(site, out).split(sep)[0] !== 'dist')) {
    throw new Error(`refusing to stage into ${out} (use an empty directory outside site/, or site/dist)`);
  }
  if (existsSync(out)) {
    const stray = readdirSync(out).filter((n) => !DEPLOY.includes(n));
    if (stray.length) throw new Error(`${out} is not empty and is not a previous stage (contains ${stray.slice(0, 5).join(', ')}); pick an empty directory`);
    for (const n of DEPLOY) rmSync(join(out, n), { recursive: true, force: true });
  }
  mkdirSync(out, { recursive: true });
  for (const n of DEPLOY) cpSync(join(site, n), join(out, n), { recursive: true });
  return out;
}

// ------------------------------------------------------------------ self-test
async function selfTest() {
  const base = mkdtempSync(join(tmpdir(), 'payverge-site-selftest-'));
  let n = 0;
  const copy = () => {
    const d = join(base, `case-${++n}`);
    cpSync(HERE_SITE, d, { recursive: true, filter: (src) => relative(HERE_SITE, src).split(sep)[0] !== 'dist' });
    return d;
  };
  const edit = (dir, file, fn) => {
    const p = join(dir, file);
    const before = readFileSync(p, 'utf8');
    const after = fn(before);
    if (after === before) throw new Error(`self-test mutation did not change ${file}`);
    writeFileSync(p, after);
  };
  const beforeMainEnd = (snippet) => (s) => s.replace('</main>', `${snippet}\n</main>`);
  const beforeHeadEnd = (snippet) => (s) => s.replace('</head>', `${snippet}\n</head>`);
  // beforeMainEnd puts its snippet on the line where </main> is now.
  const mainLine = readFileSync(join(HERE_SITE, 'index.html'), 'utf8').split('\n').findIndex((l) => l.includes('</main>')) + 1;
  const confirmAll = (dir) => edit(dir, GATES_FILE, (s) => {
    const j = JSON.parse(s);
    for (const g of j.gates) g.confirmed = { by: 'self-test', on: '2026-01-01', ref: 'v0.0.0', note: 'fixture' };
    return JSON.stringify(j, null, 2);
  });

  const cases = [
    ['broken #anchor', (d) => edit(d, 'index.html', beforeMainEnd('<a href="#nowhere">x</a>')), /fragment reference "#nowhere"/],
    ['missing asset', (d) => edit(d, 'index.html', (s) => s.replace('src="assets/logo.svg"', 'src="assets/logo-missing.svg"')), /missing local file/],
    ['edited inline script', (d) => edit(d, 'index.html', (s) => s.replace('btn.hidden = false;', 'btn.hidden = false; ')), /is not in the CSP/],
    ['low-contrast light token', (d) => edit(d, 'index.html', (s) => s.replace(/--muted:\s*#[0-9a-f]+/i, '--muted: #c8c4bc')), /light: --muted on/],
    ['low-contrast token in a second dark block', (d) => edit(d, 'index.html', beforeHeadEnd('<style>@media (prefers-color-scheme: dark) { :root { --muted: #333333; } }</style>')), /dark: --muted on/],
    ['http:// link', (d) => edit(d, 'index.html', beforeMainEnd('<a href="http://example.com/">x</a>')), /insecure http/],
    ['javascript: link', (d) => edit(d, 'index.html', beforeMainEnd('<a href="javascript:alert(1)">x</a>')), /javascript: URL/],
    ['external script, double quotes', (d) => edit(d, 'index.html', beforeHeadEnd('<script src="https://cdn.example.com/a.js"></script>')), /third-party/],
    ['external script, single quotes', (d) => edit(d, 'index.html', beforeHeadEnd("<script src='https://cdn.example.com/a.js'></script>")), /third-party/],
    ['external script, unquoted', (d) => edit(d, 'index.html', beforeHeadEnd('<script src=https://cdn.example.com/a.js></script>')), /third-party/],
    ['protocol-relative script', (d) => edit(d, 'index.html', beforeHeadEnd('<script src="//cdn.example.com/a.js"></script>')), /third-party/],
    ['Google Fonts stylesheet', (d) => edit(d, 'index.html', beforeHeadEnd("<link rel=stylesheet href='https://fonts.googleapis.com/css2?family=DM+Sans'>")), /third-party/],
    ['preconnect to a third party', (d) => edit(d, 'index.html', beforeHeadEnd('<link rel="preconnect" href="https://fonts.gstatic.com">')), /third-party/],
    ['CSS @import', (d) => edit(d, 'index.html', (s) => s.replace('<style>', '<style>\n@import url(https://fonts.googleapis.com/css2?family=DM+Sans);')), /@import/],
    ['external CSS url() in @font-face', (d) => edit(d, 'index.html', (s) => s.replace('url("assets/fonts/dm-sans-latin.woff2")', 'url("https://fonts.gstatic.com/s/dmsans/x.woff2")')), /loads from outside site/],
    ['external CSS url() in style=""', (d) => edit(d, 'index.html', beforeMainEnd('<div style="background:url(https://example.com/t.png)"></div>')), /loads from outside site/],
    ['external image', (d) => edit(d, 'index.html', beforeMainEnd('<img src="https://example.com/p.png" width="1" height="1" alt="">')), /third-party/],
    ['external <use href>', (d) => edit(d, 'index.html', beforeMainEnd('<svg><use href="https://example.com/s.svg#x"/></svg>')), /third-party/],
    ['iframe', (d) => edit(d, 'index.html', beforeMainEnd('<iframe src="https://example.com/"></iframe>')), /third-party/],
    ['img without size', (d) => edit(d, 'index.html', beforeMainEnd('<img src="assets/logo.svg" alt="">')), /without width\/height/],
    ['img without alt', (d) => edit(d, 'index.html', beforeMainEnd('<img src="assets/logo.svg" width="1" height="1">')), /without alt/],
    ['inline event handler', (d) => edit(d, 'index.html', beforeMainEnd('<button type="button" onclick="x()">x</button>')), /inline event handler/],
    ['unparseable tag', (d) => edit(d, 'index.html', beforeMainEnd('<img src="assets/logo.svg alt="">')), /could not parse tag/],
    ['target=_blank', (d) => edit(d, 'index.html', beforeMainEnd("<a href='https://github.com/stdevMac' target=_blank>x</a>")), /target="_blank"/],
    ['missing og:image file', (d) => edit(d, 'index.html', (s) => s.replace('content="https://payverge.io/assets/og-image.png"', 'content="https://payverge.io/assets/missing.png"')), /no matching file/],
    ['og:image on another origin', (d) => edit(d, 'index.html', (s) => s.replace('content="https://payverge.io/assets/og-image.png"', 'content="https://example.com/assets/og-image.png"')), /not on the canonical origin/],
    ['orphan asset', (d) => writeFileSync(join(d, 'assets/stray.png'), 'x'), /not referenced by any deployed page/],
    ['heavy image outside screenshots/ still counts', (d) => { writeFileSync(join(d, 'assets/big.png'), Buffer.alloc(BUDGET_BYTES)); edit(d, 'index.html', beforeMainEnd('<img src="assets/big.png" width="1" height="1" alt="" loading="lazy">')); }, /page weight \d+ B exceeds/],
    ['eager screenshot', (d) => { mkdirSync(join(d, SHOTS_DIR), { recursive: true }); writeFileSync(join(d, SHOTS_DIR, 'x.webp'), 'x'); edit(d, 'index.html', beforeMainEnd('<img src="assets/screenshots/x.webp" width="1" height="1" alt="">')); }, /must load through <img loading="lazy">/],
    ['screenshot preloaded by a <link>', (d) => { mkdirSync(join(d, SHOTS_DIR), { recursive: true }); writeFileSync(join(d, SHOTS_DIR, 'x.webp'), 'x'); edit(d, 'index.html', beforeMainEnd('<img src="assets/screenshots/x.webp" width="1" height="1" alt="" loading="lazy"><link rel="preload" as="image" href="assets/screenshots/x.webp">')); }, /must load through <img loading="lazy">/],
    ['oversized screenshot', (d) => { mkdirSync(join(d, SHOTS_DIR), { recursive: true }); writeFileSync(join(d, SHOTS_DIR, 'x.webp'), Buffer.alloc(SHOT_MAX_BYTES + 1)); edit(d, 'index.html', beforeMainEnd('<img src="assets/screenshots/x.webp" width="1" height="1" alt="" loading="lazy">')); }, /screenshot is \d+ B, over/],
    ['screenshots over the total budget', (d) => {
      mkdirSync(join(d, SHOTS_DIR), { recursive: true });
      const k = Math.ceil(SHOTS_BUDGET_BYTES / SHOT_MAX_BYTES) + 1;
      let tags = '';
      for (let i = 0; i < k; i++) { writeFileSync(join(d, SHOTS_DIR, `s${i}.webp`), Buffer.alloc(SHOT_MAX_BYTES)); tags += `<img src="assets/screenshots/s${i}.webp" width="1" height="1" alt="" loading="lazy">`; }
      edit(d, 'index.html', beforeMainEnd(tags));
    }, /screenshots total \d+ B exceeds/],
    ['unclassified top-level file', (d) => writeFileSync(join(d, 'humans.txt'), 'Payverge'), /not in the deploy list/],
    ['unknown data-gate marker', (d) => edit(d, 'index.html', beforeMainEnd('<p data-gate="no-such-gate">claim</p>')), /unknown launch gate/],
    ['gate without a marker', (d) => edit(d, GATES_FILE, (s) => {
      const j = JSON.parse(s);
      j.gates.push({ id: 'orphan-gate', claim: 'x', needs: 'x', verify: 'x', confirmed: null });
      return JSON.stringify(j, null, 2);
    }), /"orphan-gate" is not referenced/],
    ['malformed confirmation', (d) => edit(d, GATES_FILE, (s) => {
      const j = JSON.parse(s);
      j.gates[0].confirmed = { by: 'someone' };
      return JSON.stringify(j, null, 2);
    }), /"confirmed" must be null or/],
    ['malformed claim phrases', (d) => edit(d, GATES_FILE, (s) => {
      const j = JSON.parse(s);
      j.gates[0].phrases = ['ok', ' '];
      return JSON.stringify(j, null, 2);
    }), /"phrases" must be a list/],
    // Claim phrases: gated copy cannot lose its marker, and new copy cannot make a gated claim unmarked.
    ['hero note without its gate', (d) => edit(d, 'index.html', (s) => s.replace('<p class="hero-note" data-gate="licence-files zero-accounts">', '<p class="hero-note">')), /line \d+: "sign-up" is a claim of launch gate "zero-accounts"/],
    ['hero lead without its gate', (d) => edit(d, 'index.html', (s) => s.replace('<p class="lead" data-gate="feature-claims">Payverge is', '<p class="lead">Payverge is')), /"platform fee" is a claim of launch gate "feature-claims"/],
    ['meta description without its gate', (d) => edit(d, 'index.html', (s) => s.replace('Apache-2.0." data-gate="feature-claims licence-files">', 'Apache-2.0.">')), /"Apache" is a claim of launch gate "licence-files"/],
    ['og-image source without its gate', (d) => edit(d, 'tools/og-image.html', (s) => s.replace('<div class="eyebrow" data-gate="licence-files">', '<div class="eyebrow">')), /tools\/og-image\.html: line \d+: "Apache"/],
    ['new ungated claim, reported at its line', (d) => edit(d, 'index.html', beforeMainEnd('<p>Now with no platform fees.</p>')), new RegExp(`index\\.html: line ${mainLine}: "platform fee"`)],
    ['ungated claim split across lines', (d) => edit(d, 'index.html', beforeMainEnd('<p>Deploy in one\n      command.</p>')), /"one command" is a claim of launch gate "install"/],
    ['ungated claim in an attribute', (d) => edit(d, 'index.html', beforeMainEnd('<img src="assets/logo.svg" width="1" height="1" alt="Apache licensed">')), /"Apache" is a claim of launch gate "licence-files"/],
    ['claim under the wrong gate', (d) => edit(d, 'index.html', beforeMainEnd('<p data-gate="install">Apache-2.0</p>')), /"Apache" is a claim of launch gate "licence-files"/],
    ['claim after its gated element closes', (d) => edit(d, 'index.html', beforeMainEnd('<div data-gate="install"><div>x</div></div><p>one command</p>')), /"one command" is a claim of launch gate "install"/],
  ];
  // Copy that must keep passing: nesting, multiple gates per marker, comments and scripts.
  const accepts = [
    ['a lazy screenshot within its cap', (d) => { mkdirSync(join(d, SHOTS_DIR), { recursive: true }); writeFileSync(join(d, SHOTS_DIR, 'ok.webp'), Buffer.alloc(SHOT_MAX_BYTES)); edit(d, 'index.html', beforeMainEnd('<img src="assets/screenshots/ok.webp" width="1" height="1" alt="" loading="lazy">')); }],
    ['claim nested inside a same-name gated element', (d) => edit(d, 'index.html', beforeMainEnd('<div data-gate="install"><div>x</div><div><p>one command</p></div></div>'))],
    ['one marker naming two gates', (d) => edit(d, 'index.html', beforeMainEnd('<p data-gate="install licence-files">One command, Apache-2.0.</p>'))],
    ['claim phrase inside a comment', (d) => edit(d, 'index.html', beforeMainEnd('<!-- one command, no platform fees -->'))],
  ];

  const results = [];
  const record = (label, ok, detail = '') => results.push({ label, ok, detail });

  const clean = checkSite(copy());
  record('clean copy passes', clean.failures.length === 0, clean.failures.join(' | '));

  for (const [label, mutate, expect] of cases) {
    const d = copy();
    mutate(d);
    const r = checkSite(d);
    const hit = r.failures.find((f) => expect.test(f));
    record(`rejects: ${label}`, Boolean(hit), hit || `no failure matched ${expect} (got: ${r.failures.join(' | ') || 'none'})`);
  }
  for (const [label, mutate] of accepts) {
    const d = copy();
    mutate(d);
    const r = checkSite(d);
    record(`accepts: ${label}`, r.failures.length === 0, r.failures.join(' | '));
  }

  // Arguments: anything unknown is a usage error (exit 2), never a silent static-only run.
  {
    const parses = [
      [[], { launch: false, stage: null }],
      [['--launch'], { launch: true, stage: null }],
      [['--stage', 'out'], { launch: true, stage: 'out' }],
      [['--stage=out'], { launch: true, stage: 'out' }],
      [['--launch', '--stage=out'], { launch: true, stage: 'out' }],
    ];
    for (const [argv, want] of parses) {
      let got;
      try { got = parseArgs(argv); } catch (e) { got = { error: e.message }; }
      record(`args: ${JSON.stringify(argv)} parses`, got.launch === want.launch && got.stage === want.stage, JSON.stringify(got));
    }
    const rejected = [['--lanuch'], ['--launch=yes'], ['launch'], ['--stage'], ['--stage='], ['--stage', '--launch'], ['--stage=a', '--stage', 'b'], ['--self-test', '--launch']];
    for (const argv of rejected) {
      let threw = false;
      try { parseArgs(argv); } catch { threw = true; }
      record(`args: ${JSON.stringify(argv)} is rejected`, threw);
    }
    const script = fileURLToPath(import.meta.url);
    const typo = spawnSync(process.execPath, [script, '--lanuch'], { encoding: 'utf8' });
    record('args: CLI exits 2 on a typo and runs nothing', typo.status === 2 && /unknown argument "--lanuch"/.test(typo.stderr) && !/Checked/.test(typo.stdout), `status ${typo.status}: ${typo.stderr.trim()}`);
    const stageOut = join(base, 'eq-stage');
    const eq = spawnSync(process.execPath, [script, `--stage=${stageOut}`, '--launch', '--bogus'], { encoding: 'utf8' });
    record('args: CLI rejects the whole line before staging', eq.status === 2 && !existsSync(stageOut), `status ${eq.status}`);
  }

  // Probes: transient answers are retried with backoff, permanent ones are not, and the
  // URL list is probed a few at a time.
  {
    const scripted = (answers) => {
      const seq = [...answers];
      return async () => seq.shift() ?? { ok: false, status: 'exhausted' };
    };
    const run = async (answers) => {
      const sleeps = [];
      const r = await probeWithRetry('https://example.com/', { once: scripted(answers), sleep: async (ms) => { sleeps.push(ms); } });
      return { ...r, sleeps };
    };
    const r429 = await run([{ ok: false, status: 429, retryAfter: '1' }, { ok: true, status: 200 }]);
    record('probe: 429 then 200 passes after Retry-After', r429.ok && r429.attempts === 2 && JSON.stringify(r429.sleeps) === '[1000]', JSON.stringify(r429));
    const rNoHint = await run([{ ok: false, status: 429, retryAfter: null }, { ok: true, status: 200 }]);
    record('probe: 429 without Retry-After backs off', rNoHint.ok && JSON.stringify(rNoHint.sleeps) === '[2000]', JSON.stringify(rNoHint));
    const rCap = await run([{ ok: false, status: 429, retryAfter: '600' }, { ok: true, status: 200 }]);
    record('probe: Retry-After is capped', rCap.ok && JSON.stringify(rCap.sleeps) === '[30000]', JSON.stringify(rCap));
    const rNet = await run([{ ok: false, status: 'TimeoutError' }, { ok: true, status: 200 }]);
    record('probe: a network error is retried', rNet.ok && rNet.attempts === 2, JSON.stringify(rNet));
    const r503 = await run([{ ok: false, status: 503 }, { ok: false, status: 503 }, { ok: false, status: 503 }, { ok: true, status: 200 }]);
    record('probe: persistent 5xx still fails after 3 attempts', !r503.ok && r503.attempts === 3 && JSON.stringify(r503.sleeps) === '[2000,4000]', JSON.stringify(r503));
    const r404 = await run([{ ok: false, status: 404 }, { ok: true, status: 200 }]);
    record('probe: 404 fails at once, without retrying', !r404.ok && r404.attempts === 1 && r404.sleeps.length === 0, JSON.stringify(r404));

    let inFlight = 0;
    let peak = 0;
    const slow = async () => { peak = Math.max(peak, ++inFlight); await sleepMs(5); inFlight--; return { ok: true, status: 200 }; };
    const many = { gates: [], outbound: Array.from({ length: 12 }, (_, k) => `https://example.com/${k}`) };
    const rl = await checkLaunch(many, slow);
    record('probe: at most 4 URLs in flight', peak > 1 && peak <= 4 && rl.rows.length === 12 && rl.failures.length === 0, `peak ${peak}`);
  }

  // Launch gate, with a fake network so the self-test runs offline.
  const ok200 = async () => ({ ok: true, status: 200 });
  {
    const d = copy();
    const r = await checkLaunch(checkSite(d), ok200);
    record('launch: open gates block publishing', r.failures.some((f) => /is not confirmed/.test(f)), r.failures[0] || 'no failure');
  }
  {
    const d = copy();
    confirmAll(d);
    const s = checkSite(d);
    const r = await checkLaunch(s, ok200);
    record('launch: confirmed gates + live links pass', s.failures.length === 0 && r.failures.length === 0, [...s.failures, ...r.failures].join(' | '));
    const probed = r.rows.map((row) => row.split(/\s+/).pop());
    const install = s.gates.flatMap((g) => g.urls || []);
    record('launch: probes gate URLs and outbound links', install.every((u) => probed.includes(u)) && s.outbound.every((u) => probed.includes(u)) && s.outbound.length > 0, probed.join(', '));
    const r404 = await checkLaunch(s, async (u) => (u === probed[0] ? { ok: false, status: 404 } : { ok: true, status: 200 }));
    record('launch: a dead link blocks publishing', r404.failures.some((f) => /answered 404/.test(f)), r404.failures.join(' | ') || 'no failure');
  }

  // Staging copies exactly the deploy set and refuses to clobber unrelated directories.
  {
    const out = join(base, 'stage-out');
    stage(HERE_SITE, out);
    const top = readdirSync(out).sort();
    const shipped = walk(out).map((f) => relative(out, f).split(sep).join('/'));
    record('stage: copies only index.html, 404.html, robots.txt, assets/', JSON.stringify(top) === JSON.stringify([...DEPLOY].sort()) && !shipped.some((f) => /tools\/|README\.md|launch-gates/.test(f)), top.join(', '));
    stage(HERE_SITE, out);
    record('stage: re-staging into a previous stage works', readdirSync(out).length === DEPLOY.length);
    const foreign = join(base, 'foreign');
    mkdirSync(foreign);
    writeFileSync(join(foreign, 'keep.txt'), 'x');
    let refused = false;
    try { stage(HERE_SITE, foreign); } catch { refused = true; }
    record('stage: refuses a non-empty unrelated directory', refused && existsSync(join(foreign, 'keep.txt')));
  }

  rmSync(base, { recursive: true, force: true });
  for (const r of results) console.log(`${r.ok ? 'ok  ' : 'FAIL'} ${r.label}${r.ok ? '' : `\n       ${r.detail}`}`);
  const bad = results.filter((r) => !r.ok);
  console.log(`\n${results.length - bad.length}/${results.length} self-tests passed.`);
  if (bad.length) process.exit(1);
}

// ------------------------------------------------------------------ main
const USAGE = `usage: node site/tools/check.mjs [--launch] [--stage <dir> | --stage=<dir>]
       node site/tools/check.mjs --self-test
       node site/tools/check.mjs --help`;

// Strict on purpose: a typo such as --lanuch must not quietly run only the static checks and
// exit 0, or a deploy script would publish believing the launch gates passed.
export function parseArgs(argv) {
  const opts = { help: false, selfTest: false, launch: false, stage: null };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--help' || a === '-h') opts.help = true;
    else if (a === '--self-test') opts.selfTest = true;
    else if (a === '--launch') opts.launch = true;
    else if (a === '--stage' || a.startsWith('--stage=')) {
      if (opts.stage !== null) throw new Error('--stage given more than once');
      const dir = a === '--stage' ? argv[++i] : a.slice('--stage='.length);
      if (!dir || dir.startsWith('-')) throw new Error('--stage needs a directory');
      opts.stage = dir;
    } else throw new Error(`unknown argument ${JSON.stringify(a)}`);
  }
  if (opts.selfTest && (opts.launch || opts.stage !== null)) throw new Error('--self-test runs on its own');
  if (opts.stage !== null) opts.launch = true; // --stage implies --launch
  return opts;
}

async function main(argv) {
  let opts;
  try { opts = parseArgs(argv); } catch (e) { console.error(`check.mjs: ${e.message}\n${USAGE}`); process.exit(2); }
  if (opts.help) { console.log(USAGE); return; }
  if (opts.selfTest) return selfTest();
  const { launch, stage: stageDir } = opts;

  const result = checkSite(HERE_SITE);
  console.log(`Checked ${result.htmlCount} HTML files under site/`);
  for (const n of result.notes) console.log(`  ${n}`);
  console.log(`Contrast (${result.contrastRows.length} pairs):`);
  for (const row of result.contrastRows) console.log(`  ${row}`);
  const failures = [...result.failures];

  if (launch) {
    const l = await checkLaunch(result);
    console.log(`Launch probes (${l.rows.length} URLs):`);
    for (const row of l.rows) console.log(`  ${row}`);
    failures.push(...l.failures);
  }

  if (failures.length) {
    console.error(`\n${failures.length} problem(s):`);
    for (const f of failures) console.error(`  - ${f}`);
    if (launch) console.error('\nNot ready to publish. Confirm each gate in site/tools/launch-gates.json against the public repository first.');
    process.exit(1);
  }
  if (stageDir) console.log(`\nStaged ${DEPLOY.join(', ')} into ${stage(HERE_SITE, stageDir)}`);
  if (launch) console.log('\nAll checks and launch gates passed.');
  else {
    const open = result.gates.filter((g) => g.confirmed === null).length;
    console.log(`\nAll static checks passed.${open ? ` ${open} launch gate(s) still open: do not publish yet (\`--launch\` fails until they are confirmed).` : ''}`);
  }
}

await main(process.argv.slice(2));
