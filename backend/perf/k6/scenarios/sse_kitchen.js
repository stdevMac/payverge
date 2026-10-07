// sse_kitchen.js — long-poll GET /businesses/:id/events as text/event-stream.
//
// The route is auth-gated; without a valid STAFF_TOKEN the GETs will be 401.
// k6 doesn't have first-class SSE support, so we use a long-running HTTP GET
// with a 30s timeout and parse `data:` chunks out of the body once the
// stream closes (idle timeout, server shutdown, or the 30s read budget).
//
// `sse_event_lag_ms` only populates if the server-side SSE payload includes
// a `timestamp:<unix-ms>` line. The current BusinessEvent serialization emits
// timestamp as a JSON field inside the JSON-encoded data payload; until the
// server side adjusts the wire shape (see Task 12 / staging-perf notes), the
// lag Trend may remain empty.
import http from 'k6/http';
import { check, sleep } from 'k6';
import { pickBusiness } from '../lib/fixtures.js';
import { Trend, Counter } from 'k6/metrics';
import { sseThresholds } from '../lib/thresholds.js';

const baseURL = __ENV.BASE_URL || 'http://localhost:8080';
const staffToken = __ENV.STAFF_TOKEN || '';

const sseEventLag = new Trend('sse_event_lag_ms', true);
const sseEventsReceived = new Counter('sse_events_received');

export const options = {
  thresholds: sseThresholds,
  scenarios: {
    subscribers: {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS) || 500,
      duration: __ENV.DURATION || '2m',
    },
  },
};

// Per-VU business-ID cache via menu route (same discovery pattern as
// checkout.js: SSE route uses integer business ID in the path).
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
