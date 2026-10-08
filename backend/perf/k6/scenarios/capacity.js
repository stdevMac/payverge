import http from 'k6/http';
import sseClient from 'k6/x/sse';
import { check, sleep } from 'k6';
import exec from 'k6/execution';
import { Counter, Gauge, Trend } from 'k6/metrics';
import { setupOperator } from '../lib/auth.js';
import { seededBusinessSlugs } from '../lib/fixtures.js';
import { capacityThresholds } from '../lib/thresholds.js';

const baseURL = __ENV.BASE_URL || 'http://localhost:8080';
const duration = __ENV.DURATION || '30s';
const fixtureSlug = __ENV.PERF_BUSINESS_SLUG || 'perf-seed-001';

const sseEventLag = new Trend('sse_event_lag_ms', true);
const sseConnect = new Trend('sse_connect_ms', true);
const sseEventsReceived = new Counter('sse_events_received');
const sseErrors = new Counter('sse_errors');
const backgroundJobLag = new Trend('background_job_lag_seconds', true);
const queueDepth = new Gauge('queue_depth');
const goGoroutines = new Gauge('go_goroutines');

function rate(name, fallback) {
  return Math.max(1, Number(__ENV[name]) || fallback);
}

export const options = {
  summaryTrendStats: ['min', 'med', 'p(50)', 'avg', 'p(90)', 'p(95)', 'p(99)', 'max'],
  thresholds: capacityThresholds,
  scenarios: {
    anonymous_menu: arrival('anonymousMenu', rate('RPS_MENU', 5)),
    operator_action: arrival('operatorAction', rate('RPS_OPERATOR', 1)),
    guest_quote: arrival('guestQuote', rate('RPS_GUEST_QUOTE', 1)),
    guest_order: arrival('guestOrder', rate('RPS_GUEST_ORDER', 1)),
    callback_webhook: arrival('callbackWebhook', rate('RPS_CALLBACK', 1)),
    sse: {
      executor: 'constant-vus',
      exec: 'sse',
      vus: rate('VUS_SSE', 2),
      duration,
      gracefulStop: '35s',
    },
    telemetry: {
      executor: 'constant-vus',
      exec: 'telemetry',
      vus: 1,
      duration,
    },
  },
};

function arrival(execName, defaultRate) {
  return {
    executor: 'constant-arrival-rate',
    exec: execName,
    rate: defaultRate,
    timeUnit: '1s',
    duration,
    preAllocatedVUs: Math.max(2, defaultRate),
    maxVUs: Math.max(10, defaultRate * 5),
  };
}

function parseJSON(res, description) {
  try {
    return res.json();
  } catch (_) {
    throw new Error(`${description} returned invalid JSON (${res.status})`);
  }
}

export function setup() {
  const requestedSlugs = __ENV.PERF_BUSINESS_SLUG ? [fixtureSlug] : seededBusinessSlugs;
  const fixtures = requestedSlugs.map(slug => {
    const table = `${slug}-t01`;
    const menu = http.get(`${baseURL}/api/v1/business/${slug}/menu`, {
      tags: { flow: 'setup_fixture' },
    });
    if (menu.status !== 200) {
      throw new Error(`perf menu fixture ${slug} missing (${menu.status}); run perf-seed`);
    }
    const menuBody = parseJSON(menu, `perf menu fixture ${slug}`);
    const businessID = menuBody && menuBody.menu && menuBody.menu.business_id;
    if (!businessID) throw new Error(`perf menu fixture ${slug} did not expose business_id`);

    const bill = http.get(`${baseURL}/api/v1/guest/table/${table}/bill`, {
      tags: { flow: 'setup_fixture' },
    });
    const billBody = parseJSON(bill, `perf active bill fixture ${slug}`);
    const billID = billBody && billBody.bill && billBody.bill.id;
    if (bill.status !== 200 || !billID) {
      throw new Error(`perf active bill fixture ${slug} missing; rerun perf-seed`);
    }
    return { slug, table, businessID, billID };
  });
  const operatorFixture = fixtures.find(fixture => fixture.slug === fixtureSlug) || fixtures[0];

  return {
    fixtures,
    operatorFixture,
    token: setupOperator(baseURL, operatorFixture.businessID, operatorFixture.slug),
  };
}

function checked(res, flow, expected) {
  return check(res, { [`${flow} expected response`]: r => expected.includes(r.status) }, { flow });
}

function pickFixture(data) {
  return data.fixtures[exec.scenario.iterationInTest % data.fixtures.length];
}

export function anonymousMenu(data) {
  const flow = 'anonymous_menu';
  const fixture = pickFixture(data);
  const res = http.get(`${baseURL}/api/v1/business/${fixture.slug}/menu`, { tags: { flow } });
  checked(res, flow, [200]);
}

export function operatorAction(data) {
  const flow = 'operator_action';
  const res = http.get(`${baseURL}/api/v1/inside/businesses/${data.operatorFixture.businessID}/orders?limit=20`, {
    headers: { Authorization: `Bearer ${data.token}` },
    tags: { flow },
  });
  checked(res, flow, [200]);
}

function orderPayload(fixture) {
  return JSON.stringify({
    bill_id: fixture.billID,
    notes: 'D5 capacity fixture',
    items: [{
      menu_item_id: `${fixture.slug}-item-001`,
      menu_item_name: 'Item 1',
      quantity: 1,
      price: 5.0,
    }],
  });
}

export function guestQuote(data) {
  const flow = 'guest_quote';
  const fixture = pickFixture(data);
  const res = http.post(
    `${baseURL}/api/v1/guest/table/${fixture.table}/order/quote`,
    orderPayload(fixture),
    { headers: { 'Content-Type': 'application/json' }, tags: { flow } },
  );
  checked(res, flow, [200]);
}

export function guestOrder(data) {
  const flow = 'guest_order';
  const fixture = pickFixture(data);
  const requestID = `d5-${exec.vu.idInTest}-${exec.scenario.iterationInTest}`;
  const res = http.post(
    `${baseURL}/api/v1/guest/table/${fixture.table}/order`,
    orderPayload(fixture),
    {
      headers: { 'Content-Type': 'application/json', 'X-Request-Id': requestID },
      tags: { flow },
    },
  );
  checked(res, flow, [200, 201]);
}

// Validation-only callback traffic: this deliberately invalid signature must
// be rejected before provider/network work. Full signed-provider callback
// proof belongs to the isolated external candidate environment.
export function callbackWebhook() {
  const flow = 'callback_webhook';
  const res = http.post(
    `${baseURL}/api/v1/webhooks/stripe`,
    '{}',
    {
      headers: { 'Content-Type': 'application/json' },
      tags: { flow },
      responseCallback: http.expectedStatuses(400),
    },
  );
  checked(res, flow, [400]);
}

function recordSSEData(data) {
  if (!data) return;
  try {
    const envelope = JSON.parse(data);
    const timestampMS = Date.parse(envelope.timestamp);
    if (Number.isFinite(timestampMS)) sseEventLag.add(Date.now() - timestampMS);
    sseEventsReceived.add(1);
  } catch (_) {
    // The handler emits a legacy raw-data frame and a timestamped envelope.
    // Only the envelope contributes lag; an empty lag series fails thresholds.
  }
}

export function sse(data) {
  const flow = 'sse';
  const startedAt = Date.now();
  let opened = false;
  const res = sseClient.open(`${baseURL}/api/v1/inside/businesses/${data.operatorFixture.businessID}/events`, {
    method: 'GET',
    headers: { Authorization: `Bearer ${data.token}`, Accept: 'text/event-stream' },
    tags: { flow },
  }, client => {
    client.on('open', () => {
      opened = true;
      sseConnect.add(Date.now() - startedAt);
    });
    client.on('event', event => recordSSEData(event.data));
    client.on('error', () => {
      sseErrors.add(1);
      client.close();
    });
  });
  checked(res, flow, [200]);
  if (!opened) sseErrors.add(1);
}

function sampleMetric(body, name) {
  const matches = body.match(new RegExp(`^${name}(?:\\{[^}]*\\})?\\s+([0-9.eE+-]+)$`, 'gm')) || [];
  return matches.reduce((sum, line) => sum + Number(line.trim().split(/\s+/).pop()), 0);
}

export function telemetry() {
  const headers = __ENV.METRICS_TOKEN
    ? { Authorization: `Bearer ${__ENV.METRICS_TOKEN}` }
    : {};
  const res = http.get(`${baseURL}/metrics`, { headers, tags: { flow: 'telemetry' } });
  if (!check(res, { 'metrics scrape 200': r => r.status === 200 })) {
    throw new Error('metrics scrape unavailable; telemetry evidence would be incomplete');
  }
  const body = String(res.body || '');
  goGoroutines.add(sampleMetric(body, 'go_goroutines'));
  queueDepth.add(
    sampleMetric(body, 'payverge_plugin_notification_queue_depth') +
    sampleMetric(body, 'payverge_fiscal_jobs_queue_depth') +
    sampleMetric(body, 'payverge_fiscal_delivery_pending'),
  );
  backgroundJobLag.add(Math.max(
    sampleMetric(body, 'payverge_fiscal_delivery_oldest_age_seconds'),
    sampleMetric(body, 'payverge_payment_reconciliation_oldest_pending_seconds'),
  ));
  sleep(Number(__ENV.TELEMETRY_INTERVAL_SECONDS) || 5);
}

export function handleSummary(data) {
  const path = __ENV.SUMMARY_PATH || '/tmp/payverge-perf-results/capacity-summary.json';
  data.payverge_evidence = {
    profile: __ENV.PROFILE_NAME || 'custom',
    candidate_sha: __ENV.CANDIDATE_SHA || '',
    base_url: baseURL,
    generated_at: new Date().toISOString(),
  };
  return { [path]: JSON.stringify(data, null, 2) };
}
