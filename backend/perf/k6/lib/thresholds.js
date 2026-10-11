// Launch SLOs. Keep these values synchronized with ../capacity-contract.json;
// perf/contracts enforces coverage and percentile shape. HTTP metrics are
// flow-tagged so a fast menu read can never hide a slow mutation.
const flowBudgets = {
  anonymous_menu:  { p50: 100, p95: 250, p99: 500,  errors: 0.001 },
  operator_action: { p50: 150, p95: 500, p99: 1000, errors: 0.005 },
  guest_quote:     { p50: 100, p95: 300, p99: 750,  errors: 0.002 },
  guest_order:     { p50: 250, p95: 800, p99: 2000, errors: 0.005 },
  callback_webhook:{ p50: 100, p95: 300, p99: 750,  errors: 0.005 },
  sse:             { p50: 100, p95: 500, p99: 1000, errors: 0.001 },
};

function flowThresholds(flow) {
  const b = flowBudgets[flow];
  return {
    [`http_req_failed{flow:${flow}}`]: [`rate<${b.errors}`],
    [`http_req_duration{flow:${flow}}`]: [
      `p(50)<${b.p50}`,
      `p(95)<${b.p95}`,
      `p(99)<${b.p99}`,
    ],
    [`checks{flow:${flow}}`]: [`rate>${1 - b.errors}`],
  };
}

export const menuBrowseThresholds = flowThresholds('anonymous_menu');
export const checkoutThresholds = flowThresholds('guest_order');
export const sseThresholds = {
  'checks{flow:sse}': ['rate>0.999'],
  sse_connect_ms: ['p(50)<100', 'p(95)<500', 'p(99)<1000'],
  sse_errors: ['count<1'],
  sse_events_received: ['count>0'],
  sse_event_lag_ms: ['p(50)<250', 'p(95)<500', 'p(99)<1000'],
};

export const capacityThresholds = {
  ...flowThresholds('anonymous_menu'),
  ...flowThresholds('operator_action'),
  ...flowThresholds('guest_quote'),
  ...flowThresholds('guest_order'),
  ...flowThresholds('callback_webhook'),
  ...sseThresholds,
  dropped_iterations: ['count<1'],
  background_job_lag_seconds: ['p(95)<30', 'p(99)<60'],
  queue_depth: ['value<100'],
  go_goroutines: ['value<1500'],
};
