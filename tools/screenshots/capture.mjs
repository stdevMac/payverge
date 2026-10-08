#!/usr/bin/env node
// Captures the README / site screenshots from a local `--demo` install.
//
//   ./deploy/install.sh --build --domain localhost --admin-email you@example.com --demo
//   cd frontend && npm ci                      # provides playwright and sharp
//   PAYVERGE_URL=https://localhost:18443 \
//   PAYVERGE_EMAIL=you@example.com PAYVERGE_PASSWORD=... \
//     node tools/screenshots/capture.mjs
//
// It plays one real dinner service against the demo venue: a guest scans a
// table, orders from the menu, staff approve the ticket in the kitchen, the
// guest opens the bill, asks to pay at the counter, and staff record the cash
// payment, which lands the guest on the receipt. Every screen is the live app.
// Nothing is mocked. The only addition is a label on the AI screens saying no
// model is connected.
//
// No model key is configured on a demo install, so the AI shots show the
// scripted greeting and fallback reply. The guest helper already says it is
// not an AI; the staff AI screens are stamped with a banner that says so. They
// must never be replaced by pasted or invented model output.
//
// Writes PNGs to docs/assets/screenshots/ and a lighter WebP subset to
// site/assets/screenshots/. An install made with --demo serves the venue
// storefront on "/" (PRIMARY_VENUE=parrilla-quebracho-azul).
//
// Env: PAYVERGE_URL (default https://localhost:18443), PAYVERGE_EMAIL,
// PAYVERGE_PASSWORD (owner/admin of the demo), PAYVERGE_VENUE (slug, default
// parrilla-quebracho-azul), PAYVERGE_TABLE (table code; default: the first
// table of that venue with no open bill), ONLY (comma list of shot names).

import { createRequire } from 'node:module';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, '..', '..');
const require = createRequire(join(repo, 'frontend', 'package.json'));
const { chromium, request } = require('playwright');
const sharp = require('sharp');

const BASE = (process.env.PAYVERGE_URL || 'https://localhost:18443').replace(/\/$/, '');
const EMAIL = process.env.PAYVERGE_EMAIL;
const PASSWORD = process.env.PAYVERGE_PASSWORD;
const VENUE = process.env.PAYVERGE_VENUE || 'parrilla-quebracho-azul';
const ONLY = (process.env.ONLY || '').split(',').filter(Boolean);
if (!EMAIL || !PASSWORD) {
  console.error('Set PAYVERGE_EMAIL and PAYVERGE_PASSWORD (the demo owner login).');
  process.exit(2);
}

const DOCS_DIR = join(repo, 'docs', 'assets', 'screenshots');
const SITE_DIR = join(repo, 'site', 'assets', 'screenshots');
const DOCS_MAX = 300 * 1024; // per PNG in docs/
const SITE_MAX = 150 * 1024; // per WebP on the site (site/tools/check.mjs)

// Shots that also ship on the site, with the width the WebP is scaled to.
const SITE_SHOTS = {
  'storefront-desktop': 1280,
  'guest-menu': 520,
  'guest-bill-split': 520,
  'guest-receipt': 520,
  'kitchen': 1280,
  'tables': 1280,
  'menu-editor': 1280,
  'ai-waiter-guest': 520,
};

const DESKTOP = { viewport: { width: 1440, height: 900 }, deviceScaleFactor: 1 };
const MOBILE = { viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true };

const raw = mkdtempSync(join(tmpdir(), 'payverge-shots-'));
const taken = [];

const consent = () => {
  try {
    localStorage.setItem(
      'payverge_cookie_consent',
      JSON.stringify({ version: 1, analytics: false, marketing: false, decidedAt: '2026-01-01T00:00:00Z' }),
    );
  } catch {}
};

async function launch() {
  try {
    return await chromium.launch();
  } catch {
    return chromium.launch({ channel: 'chrome' });
  }
}

async function newContext(browser, device) {
  const ctx = await browser.newContext({ ...device, ignoreHTTPSErrors: true, reducedMotion: 'reduce', locale: 'en-US' });
  await ctx.addInitScript(consent);
  return ctx;
}

async function settle(page, ms = 2500) {
  await page.waitForLoadState('load').catch(() => {});
  await page.waitForTimeout(ms);
}

async function shot(page, name, { fullPage = false } = {}) {
  if (ONLY.length && !ONLY.includes(name)) return;
  const file = join(raw, `${name}.png`);
  await page.screenshot({ path: file, fullPage });
  taken.push(name);
  console.log(`captured ${name}`);
}

// A plain, visible stamp for the AI screens: no model is configured, so what
// the guest sees is scripted copy, and the picture has to say that itself.
async function stampNoModel(page) {
  await page.evaluate(() => {
    const el = document.createElement('div');
    el.textContent = 'Demo install, no model key: scripted replies, not AI output';
    el.setAttribute(
      'style',
      'position:fixed;left:50%;transform:translateX(-50%);z-index:2147483647;' +
        'bottom:12px;' +
        'background:#1c1917;color:#faf9f6;font:600 13px/1.3 system-ui,sans-serif;' +
        'padding:8px 14px;border-radius:999px;box-shadow:0 2px 10px rgba(0,0,0,.25);' +
        'max-width:calc(100vw - 24px);text-align:center',
    );
    document.body.appendChild(el);
  });
}

// ---------- staff API ----------
const api = await request.newContext({
  baseURL: `${BASE}/api/v1/`,
  ignoreHTTPSErrors: true,
  extraHTTPHeaders: { Origin: BASE },
});
async function call(method, path, data) {
  const res = await api.fetch(path, { method, data });
  if (!res.ok()) throw new Error(`${method} ${path} -> ${res.status()} ${(await res.text()).slice(0, 200)}`);
  return res.json();
}
const login = await api.post('auth/login', { data: { email: EMAIL, password: PASSWORD } });
if (!login.ok()) throw new Error(`login failed: ${login.status()}`);

// The AI shots are labelled as "no model, scripted replies". That label is only
// true when the backend has no model provider. Compose forwards
// OPENROUTER_API_KEY / LLM_BASE_URL from the shell, so check rather than trust.
const instance = await call('GET', 'instance');
if (instance?.features?.ai !== false) {
  console.error(
    'This install has an AI model configured (features.ai is not false). The AI\n' +
      'shots would be stamped "no model key" over real model output. Restart the\n' +
      'stack without OPENROUTER_API_KEY / LLM_BASE_URL in the environment.',
  );
  process.exit(3);
}

const businesses = await call('GET', 'inside/businesses');
const venue = businesses.find((b) => b.custom_url === VENUE) || businesses[0];
const BIZ = venue.id;
const { tables } = await call('GET', `inside/businesses/${BIZ}/tables`);
async function guestBill(code) {
  const res = await api.get(`guest/table/${code}/bill`);
  return res.ok() ? (await res.json()).bill : null;
}
let TABLE = process.env.PAYVERGE_TABLE;
if (!TABLE) {
  for (const t of tables.filter((t) => t.is_active)) {
    if (!(await guestBill(t.table_code))) {
      TABLE = t.table_code;
      break;
    }
  }
}
if (!TABLE) throw new Error('No free table found; pass PAYVERGE_TABLE.');
console.log(`venue ${venue.name} (#${BIZ}), table ${TABLE}`);

// Staff open the cash register before service, as they would on a real shift.
// With no register open and no card processor, the guest bill offers no way to
// pay and the cashier step below has nothing to click. On a demo venue reading
// the register already opens the house drawer; the POST covers any other venue.
const register = await call('GET', `inside/businesses/${BIZ}/cash-register/current`);
if (register.session?.status !== 'open') {
  await call('POST', `inside/businesses/${BIZ}/cash-register/sessions`, {
    opening_float: register.suggested_opening_float ?? 0,
    opening_note: 'Screenshot run',
  });
  console.log('opened the cash register');
}

const browser = await launch();
try {
  // ---------- public pages, desktop ----------
  const pub = await newContext(browser, DESKTOP);
  const pubPage = await pub.newPage();
  await pubPage.goto(`${BASE}/`);
  await settle(pubPage, 3500);
  await shot(pubPage, 'storefront-desktop');
  await pubPage.goto(`${BASE}/t/${TABLE}/menu`);
  await settle(pubPage, 3500);
  await shot(pubPage, 'guest-menu-desktop');
  await pub.close();

  // ---------- guest orders, mobile ----------
  const guest = await newContext(browser, MOBILE);
  const g = await guest.newPage();
  await g.goto(`${BASE}/`);
  await settle(g, 3500);
  await shot(g, 'storefront-mobile');

  await g.goto(`${BASE}/t/${TABLE}/menu`);
  await settle(g, 3500);
  const adds = g.locator('button[aria-label^="Add "][aria-label$=" to cart"]:visible');
  const n = Math.min(await adds.count(), 3);
  if (!n) throw new Error('No quick-add buttons on the guest menu.');
  for (let i = 0; i < n; i++) {
    await adds.nth(i).click();
    await g.waitForTimeout(900);
  }
  await g.waitForTimeout(3500); // let the "Added" toasts clear
  await shot(g, 'guest-menu');
  await g.getByRole('button', { name: /^View cart with/ }).click();
  await settle(g, 1500);
  await shot(g, 'guest-cart');
  await g.getByRole('button', { name: 'Place Order' }).click();
  await g.getByText(/Order Submitted/i).first().waitFor({ timeout: 15000 });
  await g.waitForTimeout(1200);
  await shot(g, 'guest-order-placed');

  // ---------- staff, desktop ----------
  const staff = await newContext(browser, DESKTOP);
  const sl = await staff.request.post(`${BASE}/api/v1/auth/login`, { data: { email: EMAIL, password: PASSWORD } });
  if (!sl.ok()) throw new Error(`browser login failed: ${sl.status()}`);
  const s = await staff.newPage();
  const tab = async (key, name, ms = 4000) => {
    await s.goto(`${BASE}/business/${BIZ}/dashboard?tab=${key}`, { waitUntil: 'load' });
    await s.waitForTimeout(ms);
    await shot(s, name);
  };
  await tab('overview', 'dashboard-overview', 5000);
  await tab('kitchen', 'kitchen'); // the guest's ticket waits under "Needs approval"

  // Approve the guest's ticket so the bill can be paid.
  const bill = await guestBill(TABLE);
  if (!bill) throw new Error('The guest order did not open a bill.');
  const { orders } = await call('GET', `inside/businesses/${BIZ}/orders`);
  for (const o of orders.filter((o) => o.bill_id === bill.id && o.status === 'pending')) {
    await call('PUT', `inside/businesses/${BIZ}/orders/${o.id}/status`, { status: 'approved' });
  }

  await tab('tables', 'tables');
  await tab('menu', 'menu-editor');
  await tab('reservations', 'reservations');
  await tab('delivery', 'delivery');

  // ---------- guest bill, split, payment, receipt ----------
  await g.goto(`${BASE}/t/${TABLE}/bill`);
  await settle(g, 3500);
  await shot(g, 'guest-bill');
  await g.getByRole('button', { name: /^Split Bill/ }).first().click();
  await g.waitForTimeout(1500);
  await g.getByText(/Split equally/i).first().scrollIntoViewIfNeeded();
  await g.evaluate(() => window.scrollBy(0, -140));
  await g.waitForTimeout(800);
  await shot(g, 'guest-bill-split');
  await g.getByRole('button', { name: /^Split Bill/ }).first().click();
  await g.waitForTimeout(800);
  await g.getByText(/^Payment Options$/i).first().scrollIntoViewIfNeeded();
  await g.evaluate(() => window.scrollBy(0, -120));
  await g.waitForTimeout(800);
  await shot(g, 'guest-payment');

  await g.getByRole('button', { name: /^Pay with Cashier/ }).first().click();
  await g.waitForTimeout(2500);
  const { pending_payments: pendingList = [] } = await call('GET', `inside/bills/${bill.id}/pending-alternative-payments`);
  const latest = (await guestBill(TABLE)) || bill;
  await call('POST', `inside/bills/${bill.id}/alternative-payment`, {
    ...(pendingList[0]?.id ? { request_id: pendingList[0].id } : {}),
    amount: Number(latest.total_amount).toFixed(2),
    payment_method: 'cash',
    business_confirmation: true,
  });
  await g.getByText(/Total paid/i).first().waitFor({ timeout: 30000 });
  await g.evaluate(() => window.scrollTo(0, 0));
  await g.waitForTimeout(1500);
  await shot(g, 'guest-receipt');

  // ---------- AI waiter, no model ----------
  const ai = await newContext(browser, MOBILE);
  const a = await ai.newPage();
  await a.goto(`${BASE}/t/${TABLE}/menu`);
  await settle(a, 3500);
  // With no model the guest widget is labelled "menu helper", not "AI assistant".
  await a.getByRole('button', { name: /^Open (AI assistant|menu helper)$/ }).click({ force: true });
  await a.waitForTimeout(2500);
  const box = a.locator('textarea:visible').last();
  await box.fill('What do you recommend for two people?');
  await a.getByRole('button', { name: 'Send message' }).click({ force: true });
  await a.waitForTimeout(5000);
  // Show the conversation from the top: the header, the disclosure and the
  // greeting are what tell the guest whether a model is answering.
  await a.getByRole('log').last().evaluate((el) => el.scrollTo(0, 0));
  await a.waitForTimeout(800);
  await shot(a, 'ai-waiter-guest');
  await ai.close();

  await s.goto(`${BASE}/business/${BIZ}/dashboard?tab=ai-waiter`, { waitUntil: 'load' });
  await s.waitForTimeout(4000);
  await stampNoModel(s);
  await shot(s, 'ai-waiter-dashboard');
  await s.goto(`${BASE}/business/${BIZ}/dashboard?tab=director-console`, { waitUntil: 'load' });
  await s.waitForTimeout(4000);
  await stampNoModel(s);
  await shot(s, 'director-console');
  await s.goto(`${BASE}/admin`, { waitUntil: 'load' });
  await s.waitForTimeout(4000);
  // The health panel refreshes itself; a shot taken mid-refresh has shown
  // grey bands over its cards. Wait until the refresh button is idle and the
  // page has gone a moment without repainting.
  await s
    .locator('button[aria-label="Refresh system health"]:not([data-loading="true"])')
    .waitFor({ timeout: 15000 })
    .catch(() => {});
  await s.waitForTimeout(1500);
  await shot(s, 'admin');
  await staff.close();
  await guest.close();
} finally {
  await browser.close();
  await api.dispose();
}

// ---------- encode ----------
async function underCap(encode, cap) {
  for (const q of [90, 80, 70, 60, 50, 40]) {
    const buf = await encode(q);
    if (buf.length <= cap) return buf;
  }
  return null;
}

mkdirSync(DOCS_DIR, { recursive: true });
mkdirSync(SITE_DIR, { recursive: true });
let docsTotal = 0;
let siteTotal = 0;
for (const name of taken) {
  const src = readFileSync(join(raw, `${name}.png`));
  const meta = await sharp(src).metadata();
  const docsWidth = meta.width;
  let png = await underCap(
    (q) => sharp(src).resize({ width: docsWidth }).png({ palette: true, quality: q, effort: 10, compressionLevel: 9 }).toBuffer(),
    DOCS_MAX,
  );
  if (!png) {
    png = await sharp(src).resize({ width: Math.round(docsWidth * 0.75) }).png({ palette: true, quality: 60, effort: 10 }).toBuffer();
  }
  if (png.length > DOCS_MAX) throw new Error(`${name}.png is ${png.length} bytes, over the docs cap`);
  writeFileSync(join(DOCS_DIR, `${name}.png`), png);
  docsTotal += png.length;

  const siteWidth = SITE_SHOTS[name];
  if (siteWidth) {
    const webp = await underCap(
      (q) => sharp(src).resize({ width: siteWidth }).webp({ quality: q, effort: 6 }).toBuffer(),
      SITE_MAX,
    );
    if (!webp) throw new Error(`${name}.webp cannot fit the site cap`);
    writeFileSync(join(SITE_DIR, `${name}.webp`), webp);
    siteTotal += webp.length;
    const { width, height } = await sharp(webp).metadata();
    console.log(`  site ${name}.webp ${width}x${height} ${(webp.length / 1024).toFixed(0)} KB`);
  }
  console.log(`  docs ${name}.png ${(png.length / 1024).toFixed(0)} KB`);
}
rmSync(raw, { recursive: true, force: true });
console.log(`docs total ${(docsTotal / 1024).toFixed(0)} KB, site total ${(siteTotal / 1024).toFixed(0)} KB`);
