#!/usr/bin/env node

import { createRuntime, loadConfig } from "../src/config.mjs";
import { ToolError } from "../src/configTools.mjs";
import { diagnoseIssueWithinAllowList } from "../src/tools.mjs";
import { startWebhookServer } from "../src/webhookServer.mjs";

// The webhook server maps err.statusCode onto the HTTP status. ToolError has
// a code, not a status, so a refused business would otherwise become a 500.
function rethrowWithHttpStatus(err) {
  if (err instanceof ToolError && err.code === "business_not_allowed") {
    throw Object.assign(new Error(err.message), { statusCode: 403 });
  }
  if (err instanceof ToolError && err.code === "invalid_arguments") {
    throw Object.assign(new Error(err.message), { statusCode: 400 });
  }
  throw err;
}

try {
  const runtime = createRuntime(loadConfig());
  if (!runtime.client) {
    throw new Error("the webhook diagnoser reads /api/v1/admin/* and needs PAYVERGE_ADMIN_MCP_TOKEN");
  }
  const server = await startWebhookServer({
    host: runtime.config.webhookHost,
    port: runtime.config.webhookPort,
    secret: runtime.config.webhookSecret,
    diagnoseIssue: async (payload) => {
      try {
        return await diagnoseIssueWithinAllowList(runtime.client, payload, runtime.config.businessAllowList);
      } catch (err) {
        rethrowWithHttpStatus(err);
      }
    },
  });

  process.stderr.write(`payverge-admin-webhooks listening on ${server.url}\n`);
} catch (err) {
  process.stderr.write(`payverge-admin-webhooks failed to start: ${err?.message ?? err}\n`);
  process.exitCode = 1;
}
