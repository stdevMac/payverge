// checkout.js — authenticated POST /businesses/:id/orders against a pre-staged
// open bill. The operator must provide:
//
//   STAFF_TOKEN   — pre-issued JWT with orders:create permission on the seeded
//                   businesses. Pre-minting is currently a manual step (see
//                   README "v1 limitations").
//   OPEN_BILL_ID  — numeric ID of an existing open bill in the perf DB. The
//                   perf-seed CLI seeds historical paid bills only; the
//                   operator stages one open bill manually before this run.
//
// Without those two env vars the scenario still parses and executes, but POSTs
// will be 401 / blocked-by-check. This is intentional for v1: it lets us
// validate scenario syntax and routing without requiring a fully wired auth
// pipeline.
import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { pickBusiness } from '../lib/fixtures.js';
import { checkoutThresholds } from '../lib/thresholds.js';

const baseURL = __ENV.BASE_URL || 'http://localhost:8080';
const staffToken = __ENV.STAFF_TOKEN || '';
const billID = Number(__ENV.OPEN_BILL_ID) || 0;

export const options = {
  thresholds: checkoutThresholds,
  scenarios: {
    checkout: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RPS) || 50,
      timeUnit: '1s',
      duration: __ENV.DURATION || '1m',
      preAllocatedVUs: 50,
      maxVUs: 500,
    },
  },
};

// Per-VU cache of business slug → numeric DB ID (discovered via menu route).
// The orders route requires the integer business ID in the URL, not the slug,
// so we look it up once per slug per VU using the public menu endpoint.
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

export default function () {
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
      // Operator hasn't pre-staged an open bill — bench is degraded.
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
