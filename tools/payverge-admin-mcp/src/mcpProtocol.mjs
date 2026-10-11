const MCP_PROTOCOL_VERSION = "2025-11-25";
export const MCP_SERVER_VERSION = "1.0.1"; // x-release-please-version

export async function handleJsonRpcMessage(message, { tools }) {
  if (!message || message.jsonrpc !== "2.0") {
    return errorResponse(message?.id ?? null, -32600, "Invalid JSON-RPC request");
  }

  const id = Object.hasOwn(message, "id") ? message.id : undefined;

  try {
    switch (message.method) {
      case "initialize":
        return response(id, {
          protocolVersion: MCP_PROTOCOL_VERSION,
          capabilities: { tools: {} },
          serverInfo: {
            name: "payverge-admin-mcp",
            version: MCP_SERVER_VERSION,
          },
        });
      case "notifications/initialized":
        return undefined;
      case "ping":
        return response(id, {});
      case "tools/list":
        return response(id, { tools: tools.listTools() });
      case "tools/call": {
        const params = message.params ?? {};
        const result = await tools.callTool(params.name, params.arguments ?? {});
        return response(id, result);
      }
      default:
        return errorResponse(id, -32601, `Method not found: ${message.method}`);
    }
  } catch (err) {
    return errorResponse(id, -32000, err?.message ?? String(err), {
      name: err?.name,
      status: err?.status,
      body: err?.body,
    });
  }
}

function response(id, result) {
  if (id === undefined) return undefined;
  return { jsonrpc: "2.0", id, result };
}

function errorResponse(id, code, message, data) {
  if (id === undefined) return undefined;
  return {
    jsonrpc: "2.0",
    id,
    error: {
      code,
      message,
      ...(data ? { data } : {}),
    },
  };
}
