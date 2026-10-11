// mixed_peak.js — composite scenario layering three executors:
//   - menu  (constant-arrival-rate, default 700 RPS)
//   - checkout (constant-arrival-rate, default 250 RPS)
//   - sse   (constant-vus, default 500 VUs)
//
// Default values match lunch_rush; dinner_rush roughly doubles them via env.
//
// Implementation note: rather than importing default() from menu_browse.js,
// checkout.js, and sse_kitchen.js, we inline the request logic here. k6
// evaluates the top-level statements of every imported file at module init
// time (including their `options` blocks); only the entrypoint's `options`
// is honored, but the children's top-level reads of `__ENV.RPS` etc. are
// still evaluated, which can produce confusing duplicate-scenario errors.
// Inlining is slightly DRY-violating but predictable.
import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';
import { pickBusiness } from '../lib/fixtures.js';
import {
  menuBrowseThresholds,
  checkoutThresholds,
  sseThresholds,
} from '../lib/thresholds.js';

const baseURL = __ENV.BASE_URL || 'http://localhost:8080';
const staffToken = __ENV.STAFF_TOKEN || '';
const billID = Number(__ENV.OPEN_BILL_ID) || 0;

const sseEventLag = new Trend('sse_event_lag_ms', true);
const sseEventsReceived = new Counter('sse_events_received');

export const options = {
  thresholds: {
    ...menuBrowseThresholds,
    ...checkoutThresholds,
    ...sseThresholds,
  },
  scenarios: {
    menu: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RPS_MENU) || 700,
      timeUnit: '1s',
      duration: __ENV.DURATION || '15m',
      preAllocatedVUs: 200,
      maxVUs: 1000,
      exec: 'menu',
    },
    checkout: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RPS_CHECKOUT) || 250,
      timeUnit: '1s',
      duration: __ENV.DURATION || '15m',
      preAllocatedVUs: 100,
      maxVUs: 800,
      exec: 'checkout',
    },
    sse: {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS_SSE) || 500,
      duration: __ENV.DURATION || '15m',
      exec: 'sse',
    },
  },
};

// Per-VU business slug → numeric ID cache. Shared across the three executors
// because each VU runs one of menu / checkout / sse, never overlapping.
const businessIDCache = {};
function resolveBusinessID(slug) {
  if (businessIDCache[slug]) return businessIDCache[slug];
  const res = http.get(`${baseURL}/api/v1/business/${slug}/menu`);
  if (res.status === 200) {
    let body;
    try {
      body = res.json();
    } catch (_) {
      return null;
    }
    const id = body && body.menu ? body.menu.business_id : null;
    if (id) {
      businessIDCache[slug] = id;
      return id;
    }
  }
  return null;
}

export function menu() {
  const slug = pickBusiness(__VU, __ITER);
  const res = http.get(`${baseURL}/api/v1/business/${slug}/menu`);
  check(res, { 'menu 200': r => r.status === 200 });
  sleep(Math.random() * 0.5);
}

export function checkout() {
  const slug = pickBusiness(__VU, __ITER);
  const bizID = resolveBusinessID(slug);
  if (!bizID) {
    check(null, { 'business resolved': () => false });
    return;
  }
  const headers = {
    Authorization: `Bearer ${staffToken}`,
    'Content-Type': 'application/json',
  };
  group('create order', function () {
    if (billID === 0) {
      check(null, { 'OPEN_BILL_ID set': () => false });
      return;
    }
    const payload = JSON.stringify({
      bill_id: billID,
      notes: 'k6 bench',
      items: [
        {
          menu_item_id: `${slug}-item-001`,
          menu_item_name: 'Item 1',
          quantity: 1,
          price: 5.00,
        },
      ],
    });
    const res = http.post(`${baseURL}/api/v1/businesses/${bizID}/orders`, payload, { headers });
    check(res, { 'order 2xx': r => r.status >= 200 && r.status < 300 });
  });
  sleep(Math.random() * 0.3);
}

export function sse() {
  const slug = pickBusiness(__VU, __ITER);
  const bizID = resolveBusinessID(slug);
  if (!bizID) {
    check(null, { 'business resolved': () => false });
    return;
  }
  const res = http.get(`${baseURL}/api/v1/businesses/${bizID}/events`, {
    headers: {
      Authorization: `Bearer ${staffToken}`,
      Accept: 'text/event-stream',
    },
    timeout: '30s',
    responseType: 'text',
  });
  check(res, { 'sse 200 or stream-ended': r => r.status === 200 });
  if (res.body) {
    const events = res.body.split('\n\n').filter(e => e.startsWith('data:'));
    sseEventsReceived.add(events.length);
    for (const ev of events) {
      const m = ev.match(/timestamp:(\d+)/);
      if (m) {
        const lag = Date.now() - Number(m[1]);
        sseEventLag.add(lag);
      }
    }
  }
  sleep(1);
}
