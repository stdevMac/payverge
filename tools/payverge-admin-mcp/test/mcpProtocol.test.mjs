import assert from "node:assert/strict";
import test from "node:test";

import { handleJsonRpcMessage } from "../src/mcpProtocol.mjs";

const tools = {
  listTools() {
    return [
      {
        name: "payverge_health_snapshot",
        description: "Get health.",
        inputSchema: { type: "object", properties: {} },
      },
    ];
  },
  async callTool(name, args) {
    assert.equal(name, "payverge_health_snapshot");
    assert.deepEqual(args, { includeErrors: true });
    return {
      content: [{ type: "text", text: "ok" }],
      structuredContent: { ok: true },
    };
  },
};

test("initialize returns MCP capabilities and server metadata", async () => {
  const response = await handleJsonRpcMessage(
    { jsonrpc: "2.0", id: 1, method: "initialize", params: {} },
    { tools },
  );

  assert.equal(response.id, 1);
  assert.equal(response.result.protocolVersion, "2025-11-25");
  assert.deepEqual(response.result.capabilities, { tools: {} });
  assert.equal(response.result.serverInfo.name, "payverge-admin-mcp");
});

test("tools/list exposes registered tool schemas", async () => {
  const response = await handleJsonRpcMessage(
    { jsonrpc: "2.0", id: 2, method: "tools/list", params: {} },
    { tools },
  );

  assert.equal(response.result.tools[0].name, "payverge_health_snapshot");
});

test("tools/call delegates to the tool registry", async () => {
  const response = await handleJsonRpcMessage(
    {
      jsonrpc: "2.0",
      id: 3,
      method: "tools/call",
      params: {
        name: "payverge_health_snapshot",
        arguments: { includeErrors: true },
      },
    },
    { tools },
  );

  assert.deepEqual(response.result.structuredContent, { ok: true });
  assert.deepEqual(response.result.content, [{ type: "text", text: "ok" }]);
});

test("unknown methods return a JSON-RPC method-not-found error", async () => {
  const response = await handleJsonRpcMessage(
    { jsonrpc: "2.0", id: 4, method: "nope", params: {} },
    { tools },
  );

  assert.equal(response.error.code, -32601);
  assert.match(response.error.message, /Method not found/);
});
