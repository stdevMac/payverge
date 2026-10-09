import http from 'k6/http';

// Acquires a real staff JWT from deterministic perf-seed data. The login code
// is one-use by design; rerun `make perf-seed` before each capacity run to
// reactivate it. STAFF_TOKEN remains available for an externally minted token.
export function setupOperator(baseURL, businessID, businessSlug = 'perf-seed-001') {
  if (__ENV.STAFF_TOKEN) return __ENV.STAFF_TOKEN;

  const businessIndex = businessSlug.split('-').pop();
  const email = __ENV.PERF_STAFF_EMAIL || `perf-seed-staff-${businessIndex}-1@example.test`;
  const code = __ENV.PERF_STAFF_CODE || `86${String(Number(businessIndex)).padStart(4, '0')}`;
  const res = http.post(
    `${baseURL}/api/v1/staff/verify-login-code`,
    JSON.stringify({ email, code, business_id: businessID }),
    { headers: { 'Content-Type': 'application/json' }, tags: { flow: 'setup_auth' } },
  );
  if (res.status !== 200) {
    throw new Error(`perf staff login failed (${res.status}); rerun perf-seed or provide STAFF_TOKEN`);
  }
  const body = res.json();
  if (!body || !body.token) {
    throw new Error('perf staff login returned no token');
  }
  return body.token;
}
