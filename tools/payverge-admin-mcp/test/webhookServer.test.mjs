import assert from "node:assert/strict";
import test from "node:test";

import { secretsEqual, startWebhookServer } from "../src/webhookServer.mjs";

test("webhook receiver requires the configured shared secret", async () => {
  const server = await startWebhookServer({
    host: "127.0.0.1",
    port: 0,
    secret: "hook-secret",
    diagnoseIssue: async () => ({ unreachable: true }),
  });

  try {
    const response = await fetch(`${server.url}/webhooks/backend-flag`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ type: "backend.flagged" }),
    });

    assert.equal(response.status, 401);
  } finally {
    await server.close();
  }
});

test("webhook receiver runs diagnosis for authorized backend flag payloads", async () => {
  const received = [];
  const server = await startWebhookServer({
    host: "127.0.0.1",
    port: 0,
    secret: "hook-secret",
    diagnoseIssue: async (payload) => {
      received.push(payload);
      return { status: "degraded", signals: [{ name: "failed_webhooks" }] };
    },
  });

  try {
    const response = await fetch(`${server.url}/webhooks/backend-flag`, {
      method: "POST",
      headers: {
        "content-type": "application/json",
        "x-payverge-mcp-secret": "hook-secret",
      },
      body: JSON.stringify({
        type: "backend.flagged",
        component: "checkout",
        businessId: 42,
      }),
    });

    assert.equal(response.status, 200);
    assert.deepEqual(await response.json(), {
      ok: true,
      diagnosis: { status: "degraded", signals: [{ name: "failed_webhooks" }] },
    });
    assert.deepEqual(received, [
      { type: "backend.flagged", component: "checkout", businessId: 42 },
    ]);
  } finally {
    await server.close();
  }
});

test("webhook receiver exposes a health check", async () => {
  const server = await startWebhookServer({
    host: "127.0.0.1",
    port: 0,
    secret: "hook-secret",
    diagnoseIssue: async () => ({}),
  });

  try {
    const response = await fetch(`${server.url}/health`);
    assert.equal(response.status, 200);
    assert.deepEqual(await response.json(), { ok: true });
  } finally {
    await server.close();
  }
});

test("webhook receiver uses statusCode from a refused diagnosis", async () => {
  const server = await startWebhookServer({
    host: "127.0.0.1",
    port: 0,
    secret: "hook-secret",
    diagnoseIssue: async () => {
      throw Object.assign(new Error("x"), { statusCode: 403 });
    },
  });

  try {
    const response = await fetch(`${server.url}/webhooks/backend-flag`, {
      method: "POST",
      headers: {
        "content-type": "application/json",
        "x-payverge-mcp-secret": "hook-secret",
      },
      body: JSON.stringify({ type: "backend.flagged", businessId: 13 }),
    });
    assert.equal(response.status, 403);
    assert.deepEqual(await response.json(), { error: "x" });
  } finally {
    await server.close();
  }
});

test("webhook secret comparison is exact and accepts the bearer form", async () => {
  assert.equal(secretsEqual("hook-secret", "hook-secret"), true);
  assert.equal(secretsEqual("hook-secreT", "hook-secret"), false);
  assert.equal(secretsEqual("hook-secret-longer", "hook-secret"), false);
  assert.equal(secretsEqual("", "hook-secret"), false);

  const server = await startWebhookServer({
    host: "127.0.0.1",
    port: 0,
    secret: "hook-secret",
    diagnoseIssue: async () => ({ ok: true }),
  });
  try {
    const post = (headers) =>
      fetch(`${server.url}/webhooks/backend-flag`, {
        method: "POST",
        headers: { "content-type": "application/json", ...headers },
        body: JSON.stringify({ type: "backend.flagged" }),
      });
    assert.equal((await post({ authorization: "Bearer hook-secret" })).status, 200);
    assert.equal((await post({ authorization: "Bearer hook-secre" })).status, 401);
    assert.equal((await post({ "x-payverge-mcp-secret": "hook-secret-x" })).status, 401);
  } finally {
    await server.close();
  }
});
