import assert from "node:assert/strict";
import test from "node:test";

import { PayvergeAdminClientError } from "../src/backendClient.mjs";
import { createPayvergeTools, diagnoseIssueWithinAllowList } from "../src/tools.mjs";

function fakeClient(responses) {
  const calls = [];
  return {
    calls,
    async get(path, query) {
      calls.push({ method: "GET", path, query });
      const key = `GET ${path}`;
      if (!(key in responses)) throw new Error(`missing fake response for ${key}`);
      return responses[key];
    },
    async post(path, body) {
      calls.push({ method: "POST", path, body });
      const key = `POST ${path}`;
      if (!(key in responses)) throw new Error(`missing fake response for ${key}`);
      return responses[key];
    },
  };
}

test("health snapshot gathers core health plus optional issue queues", async () => {
  const client = fakeClient({
    "GET /admin/system/health": {
      status: "degraded",
      queues: [{ name: "failed_webhooks", count: 2 }],
    },
    "GET /admin/errors": { errors: [{ id: 7, component: "checkout" }], total: 1 },
    "GET /admin/webhooks/failed": { events: [{ id: 4, provider: "plugin_paypal" }], count: 1 },
    "GET /admin/fiscal/summary": { failed_retryable_jobs: 3 },
  });
  const tools = createPayvergeTools({ client });

  const result = await tools.callTool("payverge_health_snapshot", {
    includeErrors: true,
    includeFailedWebhooks: true,
    includeFiscal: true,
  });

  assert.equal(result.structuredContent.health.status, "degraded");
  assert.equal(result.structuredContent.errors.total, 1);
  assert.equal(result.structuredContent.failed_webhooks.count, 1);
  assert.equal(result.structuredContent.fiscal.failed_retryable_jobs, 3);
  assert.deepEqual(client.calls.map((c) => c.path), [
    "/admin/system/health",
    "/admin/errors",
    "/admin/webhooks/failed",
    "/admin/fiscal/summary",
  ]);
});

test("mutation tools default to dry-run and only POST when dryRun is false", async () => {
  const client = fakeClient({
    "POST /admin/fiscal/jobs/12/requeue": { job: { id: 12, status: "pending" } },
    "POST /admin/webhooks/44/acknowledge": { event: { id: 44, status: "ignored" } },
  });
  const tools = createPayvergeTools({ client });

  const dryRun = await tools.callTool("payverge_requeue_fiscal_job", { id: 12 });
  assert.equal(dryRun.structuredContent.dryRun, true);
  assert.equal(client.calls.length, 0);

  const executed = await tools.callTool("payverge_requeue_fiscal_job", {
    id: 12,
    dryRun: false,
    confirm: true,
  });
  assert.equal(executed.structuredContent.dryRun, false);
  assert.deepEqual(client.calls, [
    { method: "POST", path: "/admin/fiscal/jobs/12/requeue", body: {} },
  ]);

  const ackDryRun = await tools.callTool("payverge_ack_failed_webhook", {
    id: 44,
    reason: "reviewed stale no-op",
  });
  assert.equal(ackDryRun.structuredContent.dryRun, true);
  assert.equal(client.calls.length, 1);

  const ackExecuted = await tools.callTool("payverge_ack_failed_webhook", {
    id: 44,
    reason: "reviewed stale no-op",
    dryRun: false,
    confirm: true,
  });
  assert.equal(ackExecuted.structuredContent.dryRun, false);
  assert.deepEqual(ackExecuted.structuredContent.response.event, { id: 44, status: "ignored" });
  assert.deepEqual(client.calls[1], {
    method: "POST",
    path: "/admin/webhooks/44/acknowledge",
    body: { reason: "reviewed stale no-op" },
  });
});

test("diagnose issue combines health, errors, webhooks, fiscal, and business context", async () => {
  const client = fakeClient({
    "GET /admin/system/health": {
      status: "degraded",
      queues: [
        { name: "failed_webhooks", count: 2 },
        { name: "failed_fiscal_jobs", count: 1 },
        { name: "errors_1h", count: 5 },
      ],
    },
    "GET /admin/errors": {
      errors: [
        { id: 1, component: "checkout", error: "stripe webhook failed" },
        { id: 2, component: "fiscal", error: "ARCA rejected receipt" },
      ],
      total: 2,
    },
    "GET /admin/webhooks/failed": {
      events: [{ id: 44, provider: "plugin_paypal", event_type: "PAYMENT.CAPTURE.COMPLETED" }],
      count: 1,
    },
    "GET /admin/fiscal/summary": {
      failed_retryable_jobs: 1,
      failed_permanent_jobs: 0,
    },
    "GET /admin/fiscal/jobs": {
      jobs: [{ id: 9, status: "failed_retryable" }],
      total: 1,
    },
    "GET /admin/businesses/42/detail": {
      business: { id: 42, name: "Cafe Verde", status: "active" },
    },
  });
  const tools = createPayvergeTools({ client });

  const result = await tools.callTool("payverge_diagnose_issue", {
    businessId: 42,
    component: "checkout",
  });

  assert.equal(result.structuredContent.status, "degraded");
  assert.deepEqual(result.structuredContent.signals.map((s) => s.name), [
    "failed_webhooks",
    "failed_fiscal_jobs",
    "errors_1h",
    "recent_errors",
    "fiscal_failed_jobs",
  ]);
  assert.ok(result.structuredContent.recommendations.some((r) => r.includes("payverge_ack_failed_webhook")));
  assert.ok(result.structuredContent.recommendations.some((r) => r.includes("payverge_requeue_fiscal_job")));
  assert.equal(result.structuredContent.business.business.name, "Cafe Verde");
});

test("admin tools explain a missing or rejected admin token as structured errors", async () => {
  const ownerOnly = createPayvergeTools({ ownerClient: fakeClient({}) });
  const missing = await ownerOnly.callTool("payverge_health_snapshot", {});
  assert.equal(missing.isError, true);
  assert.equal(missing.structuredContent.error.code, "admin_auth_not_configured");
  assert.match(missing.structuredContent.error.hint, /PAYVERGE_ADMIN_MCP_TOKEN/);

  const rejecting = (status) => ({
    async get(path) {
      throw new PayvergeAdminClientError(`GET ${path} failed with ${status}`, {
        status,
        body: { error: "nope" },
        method: "GET",
        path,
        requestId: "req-admin",
      });
    },
  });
  const unauthorized = await createPayvergeTools({ client: rejecting(401) }).callTool("payverge_fiscal_summary", {});
  assert.equal(unauthorized.structuredContent.error.code, "admin_unauthorized");
  assert.equal(unauthorized.structuredContent.error.request_id, "req-admin");
  const forbidden = await createPayvergeTools({ client: rejecting(403) }).callTool("payverge_fiscal_summary", {});
  assert.equal(forbidden.structuredContent.error.code, "admin_forbidden");
  assert.match(forbidden.structuredContent.error.hint, /PAYVERGE_ADMIN_MCP_ALLOWED_IPS/);

  await assert.rejects(ownerOnly.callTool("payverge_not_a_tool", {}), /Unknown tool/);
});

test("admin writes are destructive and refuse an apply that is not confirmed", async () => {
  const client = fakeClient({
    "POST /admin/fiscal/jobs/12/requeue": { job: { id: 12, status: "pending" } },
    "POST /admin/webhooks/44/acknowledge": { event: { id: 44, status: "ignored" } },
  });
  const tools = createPayvergeTools({ client });

  for (const name of ["payverge_ack_failed_webhook", "payverge_requeue_fiscal_job"]) {
    const def = tools.listTools().find((tool) => tool.name === name);
    assert.equal(def.annotations.destructiveHint, true, name);
  }

  const requeuePreview = await tools.callTool("payverge_requeue_fiscal_job", { id: 12 });
  assert.equal(requeuePreview.structuredContent.dry_run, true);
  const ackPreview = await tools.callTool("payverge_ack_failed_webhook", { id: 44 });
  assert.equal(ackPreview.structuredContent.dry_run, true);
  assert.equal(client.calls.length, 0);

  const requeueRefused = await tools.callTool("payverge_requeue_fiscal_job", { id: 12, dry_run: false });
  assert.equal(requeueRefused.structuredContent.error.code, "confirmation_required");
  assert.ok(requeueRefused.structuredContent.error.hint);
  const ackRefused = await tools.callTool("payverge_ack_failed_webhook", { id: 44, dry_run: false, reason: "reviewed" });
  assert.equal(ackRefused.structuredContent.error.code, "confirmation_required");
  assert.equal(client.calls.length, 0, "an unconfirmed apply must not call the backend");

  const ackNoReason = await tools.callTool("payverge_ack_failed_webhook", {
    id: 44,
    dry_run: false,
    confirm: true,
    reason: "   ",
  });
  assert.equal(ackNoReason.structuredContent.error.code, "invalid_arguments");
  assert.equal(ackNoReason.structuredContent.error.message, "reason is required to acknowledge a failed webhook");
  assert.equal(client.calls.length, 0);

  const requeued = await tools.callTool("payverge_requeue_fiscal_job", {
    id: 12,
    dry_run: false,
    confirm: true,
    allow_permanent: true,
  });
  assert.equal(requeued.isError, undefined);
  assert.deepEqual(client.calls, [
    { method: "POST", path: "/admin/fiscal/jobs/12/requeue", body: { allow_permanent: true } },
  ]);
});

test("read-only mode refuses admin mutations but keeps their previews", async () => {
  const client = fakeClient({});
  const tools = createPayvergeTools({ client, options: { readOnly: true } });
  const preview = await tools.callTool("payverge_requeue_fiscal_job", { id: 3 });
  assert.equal(preview.structuredContent.dry_run, true);
  const refused = await tools.callTool("payverge_requeue_fiscal_job", { id: 3, dry_run: false });
  assert.equal(refused.structuredContent.error.code, "read_only_mode");
  assert.equal(client.calls.length, 0);
});

test("PAYVERGE_MCP_BUSINESS_IDS also limits the admin tools that read one business", async () => {
  const client = fakeClient({
    "GET /admin/businesses/12/detail": { business: { id: 12 } },
    "GET /admin/system/health": { status: "ok", queues: [] },
    "GET /admin/errors": { errors: [], total: 0 },
    "GET /admin/webhooks/failed": { events: [], count: 0 },
    "GET /admin/fiscal/summary": {},
    "GET /admin/fiscal/jobs": { jobs: [] },
  });
  const tools = createPayvergeTools({ client, options: { businessAllowList: [12, 14] } });

  const allowed = await tools.callTool("payverge_inspect_business", { id: "12" });
  assert.equal(allowed.isError, undefined);
  assert.deepEqual(allowed.structuredContent, { business: { id: 12 } });

  const outside = await tools.callTool("payverge_inspect_business", { id: 13 });
  assert.equal(outside.isError, true);
  assert.equal(outside.structuredContent.error.code, "business_not_allowed");

  const byString = await tools.callTool("payverge_inspect_business", { id: "other-restaurant" });
  assert.equal(byString.structuredContent.error.code, "business_not_allowed");
  assert.match(byString.structuredContent.error.hint, /numeric business id/);

  const diagnose = await tools.callTool("payverge_diagnose_issue", { businessId: 99 });
  assert.equal(diagnose.structuredContent.error.code, "business_not_allowed");

  assert.deepEqual(
    client.calls.map((call) => call.path),
    ["/admin/businesses/12/detail"],
    "refused calls never reach the backend",
  );

  // Instance-wide admin reads are not tenant-scoped and stay available.
  const unscoped = await tools.callTool("payverge_diagnose_issue", {});
  assert.equal(unscoped.isError, undefined);
});

test("the admin business allow-list checks the same id the request uses", async () => {
  const client = fakeClient({
    "GET /admin/businesses/12/detail": { business: { id: 12 } },
    "GET /admin/system/health": { status: "ok", queues: [] },
    "GET /admin/errors": { errors: [], total: 0 },
    "GET /admin/webhooks/failed": { events: [], count: 0 },
    "GET /admin/fiscal/summary": {},
    "GET /admin/fiscal/jobs": { jobs: [] },
  });
  const tools = createPayvergeTools({ client, options: { businessAllowList: [12] } });

  // An allowed id in the other spelling must not wave a different business through.
  for (const args of [
    { id: 13, businessId: 12 },
    { id: "13", businessId: "12" },
    { id: 12, businessId: 13 },
  ]) {
    for (const name of ["payverge_inspect_business", "payverge_diagnose_issue"]) {
      const result = await tools.callTool(name, args);
      assert.equal(result.isError, true, `${name} ${JSON.stringify(args)}`);
      assert.equal(result.structuredContent.error.code, "invalid_arguments");
    }
  }

  // A blank id is refused rather than skipping the check.
  for (const args of [{ businessId: "", id: 13 }, { id: "  " }, { businessId: "" }]) {
    for (const name of ["payverge_inspect_business", "payverge_diagnose_issue"]) {
      const result = await tools.callTool(name, args);
      assert.equal(result.isError, true, `${name} ${JSON.stringify(args)}`);
      assert.equal(result.structuredContent.error.code, "invalid_arguments");
    }
  }

  // The other spelling alone still goes through the guard.
  const aliasOutside = await tools.callTool("payverge_inspect_business", { businessId: 13 });
  assert.equal(aliasOutside.structuredContent.error.code, "business_not_allowed");
  const aliasDiagnose = await tools.callTool("payverge_diagnose_issue", { id: 13 });
  assert.equal(aliasDiagnose.structuredContent.error.code, "business_not_allowed");
  assert.deepEqual(client.calls, [], "no refused call reaches the backend");

  // Matching spellings are fine, and the canonical numeric id is what is fetched.
  const same = await tools.callTool("payverge_inspect_business", { id: " 012 ", businessId: " 012" });
  assert.equal(same.isError, undefined);
  const diagnose = await tools.callTool("payverge_diagnose_issue", { businessId: "12" });
  assert.equal(diagnose.isError, undefined);
  assert.deepEqual(diagnose.structuredContent.business, { business: { id: 12 } });
  assert.deepEqual(
    client.calls.map((call) => call.path).filter((path) => path.startsWith("/admin/businesses/")),
    ["/admin/businesses/12/detail", "/admin/businesses/12/detail"],
  );
});

test("diagnoseIssueWithinAllowList refuses a business outside PAYVERGE_MCP_BUSINESS_IDS", async () => {
  const client = fakeClient({
    "GET /admin/businesses/12/detail": { business: { id: 12 } },
    "GET /admin/system/health": { status: "ok", queues: [] },
    "GET /admin/errors": { errors: [], total: 0 },
    "GET /admin/webhooks/failed": { events: [], count: 0 },
    "GET /admin/fiscal/summary": {},
    "GET /admin/fiscal/jobs": { jobs: [] },
  });

  await assert.rejects(
    () => diagnoseIssueWithinAllowList(client, { businessId: 13 }, [12]),
    (err) => {
      assert.equal(err.code, "business_not_allowed");
      return true;
    },
  );
  assert.deepEqual(client.calls, [], "a refused business makes no request");
  assert.equal(
    client.calls.some((call) => call.path === "/admin/businesses/13/detail"),
    false,
  );

  const allowed = await diagnoseIssueWithinAllowList(client, { businessId: 12 }, [12]);
  assert.deepEqual(allowed.business, { business: { id: 12 } });
  assert.ok(client.calls.some((call) => call.path === "/admin/businesses/12/detail"));
  assert.equal(
    client.calls.some((call) => call.path === "/admin/businesses/13/detail"),
    false,
  );

  client.calls.length = 0;
  const instanceWide = await diagnoseIssueWithinAllowList(client, {}, [12]);
  assert.equal(instanceWide.business, undefined);
  assert.equal(instanceWide.status, "ok");
  assert.ok(client.calls.some((call) => call.path === "/admin/system/health"));
  assert.equal(
    client.calls.some((call) => String(call.path).startsWith("/admin/businesses/")),
    false,
    "omitting businessId stays instance-wide",
  );
});

test("without an allow-list the admin business tools still take either spelling", async () => {
  const client = fakeClient({
    "GET /admin/businesses/demo-bistro/detail": { business: { business_id: "demo-bistro" } },
  });
  const tools = createPayvergeTools({ client });
  const result = await tools.callTool("payverge_inspect_business", { businessId: "demo-bistro" });
  assert.equal(result.isError, undefined);
  assert.deepEqual(client.calls.map((call) => call.path), ["/admin/businesses/demo-bistro/detail"]);
});

