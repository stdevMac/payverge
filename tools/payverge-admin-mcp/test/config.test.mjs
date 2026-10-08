import assert from "node:assert/strict";
import test from "node:test";

import { assertTransportSafe, createRuntime, isLoopbackHost, loadConfig, parseBusinessIds } from "../src/config.mjs";
import { CONFIG_TOOL_NAMES } from "../src/configTools.mjs";

test("PAYVERGE_ADMIN_MCP_TOKEN is preferred for backend admin auth", () => {
  const config = loadConfig({
    PAYVERGE_ADMIN_MCP_TOKEN: "mcp-token",
    PAYVERGE_ADMIN_TOKEN: "session-token",
  });

  assert.equal(config.adminToken, "mcp-token");
});

test("PAYVERGE_ADMIN_TOKEN remains a fallback for short-lived session testing", () => {
  const config = loadConfig({
    PAYVERGE_ADMIN_TOKEN: "session-token",
  });

  assert.equal(config.adminToken, "session-token");
});

test("owner, read-only and allow-list settings are parsed", () => {
  const config = loadConfig({
    PAYVERGE_API_BASE_URL: "https://pos.example.test/api/v1/",
    PAYVERGE_OWNER_EMAIL: " owner@example.test ",
    PAYVERGE_OWNER_PASSWORD: "pw",
    PAYVERGE_MCP_READ_ONLY: "Yes",
    PAYVERGE_MCP_BUSINESS_IDS: "3, 7 3",
  });
  assert.equal(config.apiBaseUrl, "https://pos.example.test/api/v1");
  assert.equal(config.publicUrl, "https://pos.example.test", "the public URL defaults to the API origin");
  assert.equal(config.ownerEmail, "owner@example.test");
  assert.equal(config.readOnly, true);
  assert.deepEqual(config.businessAllowList, [3, 7]);

  assert.equal(loadConfig({ PAYVERGE_MCP_READ_ONLY: "0" }).readOnly, false);
  assert.equal(loadConfig({}).businessAllowList, undefined);
  assert.equal(loadConfig({ PAYVERGE_PUBLIC_URL: "http://localhost:3000/" }).publicUrl, "http://localhost:3000");
});

test("parseBusinessIds rejects anything but positive integers", () => {
  assert.deepEqual(parseBusinessIds("1,2,,3"), [1, 2, 3]);
  assert.equal(parseBusinessIds("  "), undefined);
  assert.throws(() => parseBusinessIds("1,abc"), /PAYVERGE_MCP_BUSINESS_IDS/);
  assert.throws(() => parseBusinessIds("0"), /PAYVERGE_MCP_BUSINESS_IDS/);
  assert.throws(() => parseBusinessIds("-4"), /PAYVERGE_MCP_BUSINESS_IDS/);
});

test("createRuntime wires the clients each credential enables", () => {
  const fetchImpl = async () => {
    throw new Error("no network in this test");
  };
  assert.throws(
    () => createRuntime(loadConfig({ PAYVERGE_OWNER_EMAIL: "owner@example.test" }), { fetchImpl }),
    /both PAYVERGE_OWNER_EMAIL and PAYVERGE_OWNER_PASSWORD/,
  );

  const bare = createRuntime(loadConfig({}), { fetchImpl });
  assert.equal(bare.client, undefined);
  assert.equal(bare.ownerClient, undefined);
  assert.ok(bare.publicClient);
  assert.equal(bare.tools.listTools().length, CONFIG_TOOL_NAMES.length);

  const owner = createRuntime(loadConfig({ PAYVERGE_OWNER_TOKEN: "pasted-jwt" }), { fetchImpl });
  assert.equal(owner.ownerSession.mode, "static_token");

  const full = createRuntime(
    loadConfig({ PAYVERGE_ADMIN_MCP_TOKEN: "admin", PAYVERGE_OWNER_EMAIL: "o@example.test", PAYVERGE_OWNER_PASSWORD: "pw" }),
    { fetchImpl },
  );
  assert.equal(full.ownerSession.mode, "email_password");
  const names = full.tools.listTools().map((tool) => tool.name);
  assert.ok(names.includes("payverge_diagnose_issue"));
  assert.ok(names.includes("payverge_import_menu"));
  assert.equal(names.length, CONFIG_TOOL_NAMES.length + 9);
});

test("credentials are never sent over plain http to a remote host", () => {
  const fetchImpl = async () => {
    throw new Error("no network in this test");
  };
  const remote = { PAYVERGE_API_BASE_URL: "http://pos.example.test/api/v1" };
  for (const creds of [
    { PAYVERGE_ADMIN_MCP_TOKEN: "admin-token" },
    { PAYVERGE_OWNER_EMAIL: "owner@example.test", PAYVERGE_OWNER_PASSWORD: "pw" },
    { PAYVERGE_OWNER_TOKEN: "pasted-jwt" },
  ]) {
    assert.throws(() => createRuntime(loadConfig({ ...remote, ...creds }), { fetchImpl }), /plain http/);
    // Explicit opt-in for a trusted private network (e.g. http://backend:8080 inside Compose).
    assert.doesNotThrow(() =>
      createRuntime(loadConfig({ ...remote, ...creds, PAYVERGE_MCP_ALLOW_INSECURE_HTTP: "true" }), { fetchImpl }),
    );
    assert.doesNotThrow(() =>
      createRuntime(loadConfig({ PAYVERGE_API_BASE_URL: "https://pos.example.test/api/v1", ...creds }), { fetchImpl }),
    );
    assert.doesNotThrow(() =>
      createRuntime(loadConfig({ PAYVERGE_API_BASE_URL: "http://127.0.0.1:8080/api/v1", ...creds }), { fetchImpl }),
    );
  }
  // Anonymous status checks carry no secret, so plain http stays allowed.
  assert.doesNotThrow(() => createRuntime(loadConfig(remote), { fetchImpl }));
  assert.throws(() => assertTransportSafe({ apiBaseUrl: "ftp://pos.example.test/api/v1" }), /https/);
  assert.throws(() => assertTransportSafe({ apiBaseUrl: "not a url" }), /valid URL/);
});

test("isLoopbackHost recognises loopback names only", () => {
  for (const host of ["localhost", "LOCALHOST", "payverge.localhost", "127.0.0.1", "127.1.2.3", "[::1]", "::1", "localhost."]) {
    assert.equal(isLoopbackHost(host), true, host);
  }
  for (const host of ["example.com", "localhost.example.com", "127.0.0.1.nip.io", "10.0.0.1", "0.0.0.0", "backend", ""]) {
    assert.equal(isLoopbackHost(host), false, host);
  }
});
