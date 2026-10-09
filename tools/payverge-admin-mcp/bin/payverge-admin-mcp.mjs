#!/usr/bin/env node

import { createRuntime, loadConfig } from "../src/config.mjs";
import { runStdioServer } from "../src/stdioServer.mjs";

try {
  const runtime = createRuntime(loadConfig());
  runStdioServer({ tools: runtime.tools });
} catch (err) {
  process.stderr.write(`payverge-admin-mcp failed to start: ${err?.message ?? err}\n`);
  process.exitCode = 1;
}
