#!/usr/bin/env node

import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import http from "node:http";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, "..");
const bin = resolve(root, "bin/payverge-admin-mcp.mjs");

const backend = await startFakeAdminApi();
const child = spawn(process.execPath, [bin], {
  cwd: root,
  env: {
    ...process.env,
    PAYVERGE_API_BASE_URL: `${backend.url}/api/v1`,
    PAYVERGE_ADMIN_MCP_TOKEN: "smoke-admin-token",
    PAYVERGE_ADMIN_TOKEN: "smoke-admin-token",
    PAYVERGE_OWNER_EMAIL: "owner@smoke.test",
    PAYVERGE_OWNER_PASSWORD: "smoke-owner-password",
    PAYVERGE_MCP_READ_ONLY: "",
    PAYVERGE_MCP_BUSINESS_IDS: "",
    PAYVERGE_OWNER_TOKEN: "",
    PAYVERGE_PUBLIC_URL: "",
  },
  stdio: ["pipe", "pipe", "pipe"],
});

const stderr = [];
child.stderr.setEncoding("utf8");
child.stderr.on("data", (chunk) => stderr.push(chunk));

try {
  const responses = collectJsonLines(child.stdout);

  child.stdin.write(`${JSON.stringify({
    jsonrpc: "2.0",
    id: 1,
    method: "initialize",
    params: {
      protocolVersion: "2025-11-25",
      capabilities: {},
      clientInfo: { name: "payverge-admin-mcp-smoke", version: "0" },
    },
  })}\n`);
  child.stdin.write(`${JSON.stringify({
    jsonrpc: "2.0",
    id: 2,
    method: "tools/call",
    params: {
      name: "payverge_diagnose_issue",
      arguments: { businessId: 42, component: "checkout" },
    },
  })}\n`);
  child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", id: 3, method: "tools/list", params: {} })}\n`);
  child.stdin.write(`${JSON.stringify({
    jsonrpc: "2.0",
    id: 4,
    method: "tools/call",
    params: { name: "payverge_instance_status", arguments: {} },
  })}\n`);
  child.stdin.write(`${JSON.stringify({
    jsonrpc: "2.0",
    id: 5,
    method: "tools/call",
    params: { name: "payverge_bulk_create_tables", arguments: { business_id: 42, count: 3 } },
  })}\n`);
  child.stdin.end();

  const initialize = await waitForResponse(responses, 1);
  assert.equal(initialize.result.serverInfo.name, "payverge-admin-mcp");

  const diagnosisResponse = await waitForResponse(responses, 2);
  const diagnosis = diagnosisResponse.result.structuredContent;
  assert.equal(diagnosis.status, "degraded");
  assert.equal(diagnosis.business.business.name, "Smoke Cafe");
  assert.ok(diagnosis.signals.some((signal) => signal.name === "failed_webhooks"));
  assert.ok(diagnosis.signals.some((signal) => signal.name === "fiscal_failed_jobs"));

  const list = await waitForResponse(responses, 3);
  const names = list.result.tools.map((tool) => tool.name);
  assert.ok(names.includes("payverge_import_menu"), "config tools are listed");
  assert.ok(names.includes("payverge_diagnose_issue"), "admin tools are listed when the admin token is set");

  const status = (await waitForResponse(responses, 4)).result.structuredContent;
  assert.equal(status.instance.product_name, "Smoke POS");
  assert.equal(status.mcp.owner_auth.mode, "email_password");
  assert.equal(status.mcp.admin_token_configured, true);

  const tables = (await waitForResponse(responses, 5)).result.structuredContent;
  assert.equal(tables.dry_run, true);
  assert.deepEqual(tables.to_create, ["Table 2", "Table 3"]);
  assert.equal(backend.logins(), 1, "the owner session logs in once");
  assert.equal(backend.writes(), 0, "a preview never writes");

  const exitCode = await waitForExit(child);
  assert.equal(exitCode, 0, stderr.join(""));
  console.log("payverge-admin-mcp smoke passed");
} finally {
  child.kill();
  await backend.close();
}

async function startFakeAdminApi() {
  // Three auth surfaces, as on a real backend: public probes, the owner
  // session (/auth/login -> /inside/*), and the admin token (/admin/*).
  const ownerToken = "smoke-owner-session";
  let logins = 0;
  let writes = 0;
  const server = http.createServer(async (req, res) => {
    const url = new URL(req.url, "http://127.0.0.1");
    if (req.method !== "GET" && url.pathname !== "/api/v1/auth/login") writes += 1;

    switch (url.pathname) {
      case "/api/v1/instance":
        return json(res, 200, { product_name: "Smoke POS", features: { ai: true } });
      case "/api/v1/health/live":
        return json(res, 200, { status: "ok" });
      case "/api/v1/health/ready":
        return json(res, 200, { status: "ready" });
      case "/api/v1/auth/login": {
        const body = JSON.parse(await readBody(req));
        if (body.email !== "owner@smoke.test" || body.password !== "smoke-owner-password") {
          return json(res, 401, { error: "Invalid email or password" });
        }
        logins += 1;
        return json(res, 200, { token: ownerToken });
      }
      default:
        break;
    }

    if (url.pathname.startsWith("/api/v1/inside/")) {
      if (req.headers.authorization !== `Bearer ${ownerToken}`) {
        return json(res, 401, { error: "unauthorized" });
      }
      if (req.method === "GET" && url.pathname === "/api/v1/inside/businesses/42/tables") {
        return json(res, 200, { tables: [{ id: 1, name: "Table 1", table_code: "smk1", qr_url: "/t/smk1" }] });
      }
      return json(res, 404, { error: `unhandled smoke path ${req.method} ${url.pathname}` });
    }

    if (req.headers.authorization !== "Bearer smoke-admin-token") {
      return json(res, 401, { error: "unauthorized" });
    }

    switch (url.pathname) {
      case "/api/v1/admin/system/health":
        return json(res, 200, {
          status: "degraded",
          queues: [
            { name: "failed_webhooks", count: 1, label: "Failed Stripe webhooks" },
            { name: "failed_fiscal_jobs", count: 1, label: "Failed fiscal jobs" },
          ],
        });
      case "/api/v1/admin/errors":
        return json(res, 200, {
          errors: [{ id: 1, component: "checkout", error: "checkout failed" }],
          total: 1,
        });
      case "/api/v1/admin/webhooks/failed":
        return json(res, 200, {
          events: [{ id: 7, provider: "plugin_paypal", event_type: "PAYMENT.CAPTURE.COMPLETED" }],
          count: 1,
        });
      case "/api/v1/admin/fiscal/summary":
        return json(res, 200, {
          failed_retryable_jobs: 1,
          failed_permanent_jobs: 0,
        });
      case "/api/v1/admin/fiscal/jobs":
        return json(res, 200, {
          jobs: [{ id: 9, status: "failed_retryable" }],
          total: 1,
        });
      case "/api/v1/admin/businesses/42/detail":
        return json(res, 200, {
          business: { id: 42, name: "Smoke Cafe", status: "active" },
        });
      default:
        return json(res, 404, { error: `unhandled smoke path ${url.pathname}` });
    }
  });

  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address();
  return {
    url: `http://127.0.0.1:${port}`,
    logins: () => logins,
    writes: () => writes,
    close() {
      return new Promise((resolve, reject) => {
        server.close((err) => (err ? reject(err) : resolve()));
      });
    },
  };
}

function collectJsonLines(stream) {
  const seen = new Map();
  const waiters = new Map();
  stream.setEncoding("utf8");
  stream.on("data", (chunk) => {
    for (const line of chunk.split(/\r?\n/)) {
      if (!line.trim()) continue;
      const message = JSON.parse(line);
      const resolveWaiter = waiters.get(message.id);
      if (resolveWaiter) {
        waiters.delete(message.id);
        resolveWaiter(message);
      } else {
        seen.set(message.id, message);
      }
    }
  });
  return { seen, waiters };
}

function waitForResponse(responses, id) {
  if (responses.seen.has(id)) {
    const message = responses.seen.get(id);
    responses.seen.delete(id);
    return Promise.resolve(message);
  }
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      responses.waiters.delete(id);
      reject(new Error(`timed out waiting for JSON-RPC response ${id}`));
    }, 5000);
    responses.waiters.set(id, (message) => {
      clearTimeout(timer);
      resolve(message);
    });
  });
}

function waitForExit(childProcess) {
  return new Promise((resolve) => {
    childProcess.on("exit", (code) => resolve(code));
  });
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    let text = "";
    req.setEncoding("utf8");
    req.on("data", (chunk) => {
      text += chunk;
    });
    req.on("end", () => resolve(text));
    req.on("error", reject);
  });
}

function json(res, status, body) {
  res.statusCode = status;
  res.setHeader("content-type", "application/json");
  res.end(JSON.stringify(body));
}
