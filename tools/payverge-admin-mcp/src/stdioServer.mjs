import { handleJsonRpcMessage } from "./mcpProtocol.mjs";

export function runStdioServer({ tools, input = process.stdin, output = process.stdout, log = process.stderr } = {}) {
  if (!tools) throw new Error("tools are required");

  let buffer = "";
  input.setEncoding("utf8");

  input.on("data", async (chunk) => {
    buffer += chunk;
    const lines = buffer.split(/\r?\n/);
    buffer = lines.pop() ?? "";

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed) continue;
      await handleLine(trimmed, { tools, output, log });
    }
  });

  input.on("end", async () => {
    const trimmed = buffer.trim();
    if (trimmed) {
      await handleLine(trimmed, { tools, output, log });
    }
  });
}

async function handleLine(line, { tools, output, log }) {
  let message;
  try {
    message = JSON.parse(line);
  } catch (err) {
    writeMessage(output, {
      jsonrpc: "2.0",
      id: null,
      error: {
        code: -32700,
        message: `Parse error: ${err.message}`,
      },
    });
    return;
  }

  try {
    const response = await handleJsonRpcMessage(message, { tools });
    if (response) writeMessage(output, response);
  } catch (err) {
    log.write(`payverge-admin-mcp internal error: ${err?.stack ?? err}\n`);
  }
}

function writeMessage(output, message) {
  output.write(`${JSON.stringify(message)}\n`);
}
