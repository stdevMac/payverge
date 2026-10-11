import { extractContext, fillUrl } from "./lib/template.js";
import { buildFilename } from "./lib/filename.js";
import { deriveApiBase } from "./lib/api.js";
import {
  BOOT_GATE_SELECTOR,
  SKELETON_PULSE_SELECTOR,
  MIN_SKELETON_AREA_PX,
} from "./lib/readiness.js";
import { landedOnRequestedPage } from "./lib/navigation.js";
import { chooseTargetTab } from "./lib/targetTab.js";
import { shots as ALL_SHOTS, defaults } from "./shots.js";

const LOG = "[payverge-shots]";

// Service-worker consoles are invisible outside DevTools, so surface crashes
// and the last run's outcome as tiny files under payverge-shots/debug/. This
// is how skip/error reasons stay diagnosable after the popup has closed.
function debugBeacon(name, text) {
  try {
    chrome.downloads
      .download({
        url: "data:text/plain;charset=utf-8," + encodeURIComponent(text || name),
        filename: `payverge-shots/debug/${name}.txt`,
        saveAs: false,
        conflictAction: "overwrite",
      })
      .catch(() => {});
  } catch {
    // downloads unavailable — nothing else we can do from a SW
  }
}
self.addEventListener("error", (e) =>
  debugBeacon("sw-error", String((e && (e.message || e.error)) || e)),
);
self.addEventListener("unhandledrejection", (e) =>
  debugBeacon("sw-rejection", String((e.reason && (e.reason.stack || e.reason.message)) || e.reason)),
);

function today() {
  const d = new Date();
  const p = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

// Navigate the tab, keep it the ACTIVE tab, and resolve once it finishes
// loading. The listener is registered before navigation so a fast load can't
// be missed, and "complete" only counts after we've seen this navigation's
// "loading" phase — otherwise a still-finishing previous page could satisfy
// the wait and we'd capture shot N-1 under shot N's name. A timeout
// guarantees the run never hangs.
function navigateAndWait(tabId, url, timeoutMs = 15000) {
  return new Promise((resolve) => {
    let settled = false;
    let sawLoading = false;
    const finish = () => {
      if (settled) return;
      settled = true;
      chrome.tabs.onUpdated.removeListener(listener);
      clearTimeout(timer);
      resolve();
    };
    const listener = (id, info) => {
      if (id !== tabId) return;
      if (info.status === "loading") sawLoading = true;
      if (info.status === "complete" && sawLoading) finish();
    };
    chrome.tabs.onUpdated.addListener(listener);
    const timer = setTimeout(finish, timeoutMs);
    chrome.tabs.update(tabId, { url, active: true });
  });
}

// Injected into the page to decide when it's settled enough to capture. Must
// be fully self-contained (serialized for injection) — it mirrors
// lib/readiness.js#evaluateReadiness; selectors/threshold arrive via opts so
// there is one source of truth. Returns a promise so executeScript awaits it.
// Also reports location.href so the caller can detect auth redirects.
function pageReadyProbe(opts) {
  const {
    waitFor,
    settleMs,
    timeoutMs,
    bootGateSelector,
    skeletonPulseSelector,
    minSkeletonArea,
    action,
  } = opts;
  if (action && action.type === "click") {
    document.querySelector(action.selector)?.click();
  }
  const isReady = () => {
    if (document.querySelector(bootGateSelector)) return false;
    for (const el of document.querySelectorAll(skeletonPulseSelector)) {
      const rect = el.getBoundingClientRect();
      if (rect.width * rect.height >= minSkeletonArea) return false;
    }
    if (waitFor && !document.querySelector(waitFor)) return false;
    return true;
  };
  const start = Date.now();
  return new Promise((resolve) => {
    const done = (status) =>
      setTimeout(() => resolve({ status, href: location.href }), settleMs);
    const tick = () => {
      if (isReady()) return done("READY");
      if (Date.now() - start > timeoutMs) return done("WARN");
      setTimeout(tick, 150);
    };
    tick();
  });
}

async function waitForReady(tabId, shot) {
  try {
    const [res] = await chrome.scripting.executeScript({
      target: { tabId },
      func: pageReadyProbe,
      args: [
        {
          waitFor: shot.waitFor ?? null,
          settleMs: shot.settleMs ?? 800,
          timeoutMs: shot.timeoutMs ?? 20000,
          bootGateSelector: BOOT_GATE_SELECTOR,
          skeletonPulseSelector: SKELETON_PULSE_SELECTOR,
          minSkeletonArea: MIN_SKELETON_AREA_PX,
          action: shot.action ?? null,
        },
      ],
    });
    return res?.result ?? { status: "WARN", href: null };
  } catch (e) {
    console.error(`${LOG} readiness probe failed`, e);
    return { status: "WARN", href: null };
  }
}

// Injected once at run start. Seeds the DECLINED (non-essential) cookie
// consent so the banner never mounts over guest/storefront shots. Respects an
// existing choice — it only writes when the visitor has not decided yet.
// Shape mirrors frontend/src/lib/analytics/consentGate.ts.
function seedConsentDecline() {
  try {
    const KEY = "payverge_cookie_consent";
    if (!localStorage.getItem(KEY)) {
      localStorage.setItem(
        KEY,
        JSON.stringify({
          version: 1,
          analytics: false,
          marketing: false,
          decidedAt: new Date().toISOString(),
        }),
      );
    }
    return true;
  } catch {
    return false;
  }
}

// Injected into the page to resolve the business's storefront slug + a live
// table code. Runs in the PAGE context (payverge.io) so the request is
// same-site to api.payverge.io and the SameSite=Lax session cookie is sent — a
// background fetch is cross-site and would be unauthenticated. Self-contained.
async function resolveInPage(apiBase, businessId, needCustomUrl, needTable) {
  const out = {};
  // The access token expires quickly; the app silently refreshes on 401 via
  // its axios interceptor, but this raw fetch must do the same dance itself:
  // POST /auth/refresh (HttpOnly refresh_token cookie rides along same-site)
  // and retry once.
  const get = async (path) => {
    let r = await fetch(`${apiBase}${path}`, { credentials: "include" });
    if (r.status === 401) {
      const refreshed = await fetch(`${apiBase}/api/v1/auth/refresh`, {
        method: "POST",
        credentials: "include",
      });
      if (refreshed.ok) {
        r = await fetch(`${apiBase}${path}`, { credentials: "include" });
      }
    }
    if (!r.ok) throw new Error(`${r.status} ${path}`);
    return r.json();
  };
  if (needCustomUrl) {
    try {
      const b = await get(`/api/v1/inside/businesses/${businessId}`);
      out.customUrl = b.custom_url || (b.business && b.business.custom_url) || null;
    } catch (e) {
      out.customUrlError = String(e.message || e);
    }
  }
  if (needTable) {
    try {
      const d = await get(`/api/v1/inside/businesses/${businessId}/tables`);
      const list = Array.isArray(d) ? d : d.tables || [];
      const t = list.find((x) => x.is_active) || list[0];
      out.tableCode = (t && t.table_code) || null;
    } catch (e) {
      out.tableCodeError = String(e.message || e);
    }
  }
  return out;
}

// Resolve the REAL business's storefront slug + table code so logged-out shots
// point at the business you're logged into. Best-effort: on failure the value
// stays unset and the dependent shot is skipped (not faked).
async function resolveContext(ctx, origin, tabId, needCustomUrl, needTable) {
  if (!ctx.businessId) return ctx;
  if ((!needCustomUrl || ctx.customUrl) && (!needTable || ctx.tableCode)) {
    return ctx;
  }
  const apiBase = defaults.apiBase || deriveApiBase(origin);
  try {
    const [res] = await chrome.scripting.executeScript({
      target: { tabId },
      func: resolveInPage,
      args: [apiBase, ctx.businessId, needCustomUrl && !ctx.customUrl, needTable && !ctx.tableCode],
    });
    const out = res?.result ?? {};
    if (out.customUrl) ctx.customUrl = out.customUrl;
    if (out.tableCode) ctx.tableCode = out.tableCode;
    console.log(`${LOG} resolved`, {
      customUrl: ctx.customUrl ?? "(none)",
      tableCode: ctx.tableCode ?? "(none)",
      customUrlError: out.customUrlError,
      tableCodeError: out.tableCodeError,
    });
    // Resolution failures otherwise surface only as bare "missing context"
    // skips later — put the actual API error in the run log.
    for (const err of [out.customUrlError, out.tableCodeError]) {
      if (err) {
        report({ type: "PROGRESS", name: "resolve", state: "warned", message: err });
      }
    }
  } catch (e) {
    console.error(`${LOG} resolution injection failed:`, e);
  }
  return ctx;
}

// The popup closes as soon as the run focuses the target window, so live
// PROGRESS messages usually have no listener. Persist every line — the popup
// re-reads the last run's log on open, which is how skip/error reasons
// actually reach the operator.
let runLog = [];

function report(msg) {
  chrome.runtime.sendMessage(msg).catch(() => {});
  if (msg.type === "PROGRESS") {
    runLog.push(`${msg.name}: ${msg.state}${msg.message ? ` — ${msg.message}` : ""}`);
  } else if (msg.type === "DONE") {
    runLog.push("Done.");
  }
  chrome.storage.local
    .set({ lastRunLog: runLog.join("\n"), lastRunAt: Date.now() })
    .catch(() => {});
}

// Re-entrancy guard: two popup pages firing RUN (e.g. a re-opened autorun
// tab) must not interleave navigations on the same target tab.
let runInFlight = false;

async function runShots(shotIds) {
  if (runInFlight) {
    console.warn(`${LOG} run already in flight — ignoring duplicate RUN`);
    return;
  }
  runInFlight = true;
  try {
    await runShotsInner(shotIds);
  } finally {
    runInFlight = false;
  }
}

async function runShotsInner(shotIds) {
  runLog = [];
  const tab = chooseTargetTab(await chrome.tabs.query({}));
  if (!tab) {
    console.warn(`${LOG} no business dashboard tab found`);
    report({
      type: "PROGRESS",
      name: "run",
      state: "error",
      message: "no /business/<id>/dashboard tab open — log in and open the dashboard first",
    });
    report({ type: "DONE" });
    return;
  }
  const originalUrl = tab.url;
  const ctx = { ...defaults, ...extractContext(tab.url) };
  const origin = ctx.origin ?? new URL(tab.url).origin;
  const dateStr = today();
  const selected = ALL_SHOTS.filter((s) => shotIds.includes(s.name));

  try {
    await chrome.scripting.executeScript({ target: { tabId: tab.id }, func: seedConsentDecline });
  } catch (e) {
    console.warn(`${LOG} consent seed failed (non-fatal)`, e);
  }

  const needCustomUrl = selected.some((s) => s.url.includes("{customUrl}"));
  const needTable = selected.some((s) => s.url.includes("{tableCode}"));
  await resolveContext(ctx, origin, tab.id, needCustomUrl, needTable);
  console.log(`${LOG} run start`, { origin, businessId: ctx.businessId, shots: selected.length });

  for (const shot of selected) {
    // Missing {customUrl}/{tableCode} => couldn't resolve a real value. Skip
    // with a reason; never navigate to a placeholder page.
    let path;
    try {
      path = fillUrl(shot.url, ctx);
    } catch (err) {
      const message = String(err.message || err);
      console.warn(`${LOG} skip ${shot.name}: ${message}`);
      report({ type: "PROGRESS", name: shot.name, state: "skipped", message });
      continue;
    }

    report({ type: "PROGRESS", name: shot.name, state: "navigating" });
    console.log(`${LOG} navigate ${shot.name} -> ${origin + path}`);
    await navigateAndWait(tab.id, origin + path);

    const res = await waitForReady(tab.id, shot);
    console.log(`${LOG} ${shot.name} readiness: ${res.status} @ ${res.href}`);

    // Auth guard: if the app redirected us off the requested page (expired
    // session bounces /business/... to the signed-out /dashboard), capturing
    // would silently save a login screen. Report and move on instead.
    if (res.href && !landedOnRequestedPage(path, res.href)) {
      let landedPath = res.href;
      try {
        landedPath = new URL(res.href).pathname;
      } catch {}
      const message = `redirected to ${landedPath} — session expired / not logged in? Log in and re-run.`;
      console.warn(`${LOG} ${shot.name} ${message}`);
      report({ type: "PROGRESS", name: shot.name, state: "error", message });
      continue;
    }

    // Pin the capture to OUR tab: make it the active tab and bring its window
    // forward, then let it paint. captureVisibleTab grabs whatever tab is
    // visible in the window, so without this it can capture an unrelated tab.
    try {
      await chrome.tabs.update(tab.id, { active: true });
      await chrome.windows.update(tab.windowId, { focused: true });
    } catch (e) {
      console.warn(`${LOG} could not focus target tab`, e);
    }
    await new Promise((r) => setTimeout(r, 300));

    let dataUrl;
    try {
      dataUrl = await chrome.tabs.captureVisibleTab(tab.windowId, { format: "png" });
    } catch (err) {
      const message = `capture failed: ${err.message || err}`;
      console.error(`${LOG} ${shot.name} ${message}`);
      report({ type: "PROGRESS", name: shot.name, state: "error", message });
      continue;
    }

    try {
      await chrome.downloads.download({
        url: dataUrl,
        filename: buildFilename(shot.name, dateStr),
        saveAs: false,
        conflictAction: "overwrite",
      });
    } catch (err) {
      const message = `download failed: ${err.message || err}`;
      console.error(`${LOG} ${shot.name} ${message}`);
      report({ type: "PROGRESS", name: shot.name, state: "error", message });
      continue;
    }

    const state = res.status === "WARN" ? "warned" : "saved";
    console.log(`${LOG} ${shot.name}: ${state}`);
    report({ type: "PROGRESS", name: shot.name, state });
  }

  // Put the operator back where they started instead of leaving the tab on
  // the last shot's page.
  if (originalUrl) {
    await navigateAndWait(tab.id, originalUrl);
  }
  console.log(`${LOG} run done`);
  report({ type: "DONE" });
  debugBeacon("last-run-log", runLog.join("\n"));
}

chrome.runtime.onMessage.addListener((msg) => {
  if (msg.type === "RUN") {
    runShots(msg.shotIds).catch((e) =>
      debugBeacon("sw-run-crash", String((e && (e.stack || e.message)) || e)),
    );
  }
});
