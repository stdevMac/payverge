import assert from "node:assert/strict";
import http from "node:http";
import test from "node:test";

import { PayvergeAdminClient, PayvergeAdminClientError } from "../src/backendClient.mjs";

async function withServer(handler, fn) {
  const server = http.createServer(handler);
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address();
  try {
    return await fn(`http://127.0.0.1:${port}/api/v1`);
  } finally {
    await new Promise((resolve, reject) => {
      server.close((err) => (err ? reject(err) : resolve()));
    });
  }
}

test("GET requests use the configured API base URL, bearer token, and query params", async () => {
  await withServer((req, res) => {
    assert.equal(req.method, "GET");
    assert.equal(req.url, "/api/v1/admin/errors?limit=5&component=checkout");
    assert.equal(req.headers.authorization, "Bearer admin-token");
    assert.match(req.headers["user-agent"], /payverge-admin-mcp/);

    res.setHeader("content-type", "application/json");
    res.end(JSON.stringify({ errors: [{ id: 1, component: "checkout" }], total: 1 }));
  }, async (baseUrl) => {
    const client = new PayvergeAdminClient({ baseUrl, token: "admin-token" });
    const body = await client.get("/admin/errors", {
      limit: 5,
      component: "checkout",
      ignored: undefined,
    });

    assert.deepEqual(body, { errors: [{ id: 1, component: "checkout" }], total: 1 });
  });
});

test("POST requests serialize JSON bodies and surface sanitized backend errors", async () => {
  await withServer(async (req, res) => {
    assert.equal(req.method, "POST");
    assert.equal(req.url, "/api/v1/admin/fiscal/jobs/9/requeue");
    assert.equal(req.headers["content-type"], "application/json");

    let raw = "";
    for await (const chunk of req) raw += chunk;
    assert.deepEqual(JSON.parse(raw), { reason: "ops retry" });

    res.statusCode = 409;
    res.setHeader("content-type", "application/json");
    res.end(JSON.stringify({ error: "job cannot be requeued in its current state" }));
  }, async (baseUrl) => {
    const client = new PayvergeAdminClient({ baseUrl, token: "sensitive-token" });

    await assert.rejects(
      () => client.post("/admin/fiscal/jobs/9/requeue", { reason: "ops retry" }),
      (err) => {
        assert.ok(err instanceof PayvergeAdminClientError);
        assert.equal(err.status, 409);
        assert.equal(err.message, "Payverge admin API POST /admin/fiscal/jobs/9/requeue failed with 409");
        assert.deepEqual(err.body, { error: "job cannot be requeued in its current state" });
        assert.equal(JSON.stringify(err).includes("sensitive-token"), false);
        return true;
      },
    );
  });
});
