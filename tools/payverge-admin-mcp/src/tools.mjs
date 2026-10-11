import { ToolError, createConfigTools, explainError, resolveDryRun } from "./configTools.mjs";

const DEFAULT_LIMIT = 25;

const DRY_RUN_PROPERTIES = {
  dry_run: { type: "boolean", description: "Default true: describe the action without calling the backend. Set false to apply." },
  dryRun: { type: "boolean", description: "Alias of dry_run." },
};
const ADMIN_READ = { readOnlyHint: true, openWorldHint: false };
const ADMIN_WRITE = { readOnlyHint: false, destructiveHint: true, idempotentHint: false, openWorldHint: false };

const toolDefinitions = [
  {
    name: "payverge_health_snapshot",
    description: "Fetch the Payverge admin operational health snapshot, with optional recent issue queues.",
    inputSchema: {
      type: "object",
      properties: {
        includeErrors: { type: "boolean", description: "Include recent admin error logs." },
        includeFailedWebhooks: { type: "boolean", description: "Include failed payment webhooks." },
        includeFiscal: { type: "boolean", description: "Include fiscal compliance summary." },
      },
    },
  },
  {
    name: "payverge_list_error_logs",
    description: "List Payverge admin error logs with optional source/component filters.",
    inputSchema: {
      type: "object",
      properties: {
        source: { type: "string" },
        component: { type: "string" },
        limit: { type: "number", minimum: 1, maximum: 200 },
        offset: { type: "number", minimum: 0 },
      },
    },
  },
  {
    name: "payverge_list_failed_webhooks",
    description: "List failed payment webhook events known to the backend.",
    inputSchema: {
      type: "object",
      properties: {
        limit: { type: "number", minimum: 1, maximum: 200 },
      },
    },
  },
  {
    name: "payverge_ack_failed_webhook",
    description: "Acknowledge a failed payment webhook after admin review. Platform admin token only. Defaults to dry_run=true. Applying requires confirm=true and a non-empty reason.",
    inputSchema: {
      type: "object",
      required: ["id"],
      properties: {
        id: { type: "number", minimum: 1 },
        reason: { type: "string", description: "Admin review note explaining why this failure can be ignored." },
        confirm: { type: "boolean", description: "Must be true together with dry_run=false to apply." },
        ...DRY_RUN_PROPERTIES,
      },
    },
  },
  {
    name: "payverge_fiscal_summary",
    description: "Fetch platform-wide fiscal compliance summary metrics.",
    inputSchema: { type: "object", properties: {} },
  },
  {
    name: "payverge_list_fiscal_jobs",
    description: "List fiscal compliance jobs with optional status filtering.",
    inputSchema: {
      type: "object",
      properties: {
        status: { type: "string" },
        limit: { type: "number", minimum: 1, maximum: 100 },
        offset: { type: "number", minimum: 0 },
      },
    },
  },
  {
    name: "payverge_requeue_fiscal_job",
    description: "Requeue a fiscal compliance job. failed_permanent jobs need allow_permanent=true. Platform admin token only. Defaults to dry_run=true. Applying requires confirm=true.",
    inputSchema: {
      type: "object",
      required: ["id"],
      properties: {
        id: { type: "number", minimum: 1 },
        allow_permanent: { type: "boolean", description: "Set true to requeue a failed_permanent job. Retryable jobs do not need it." },
        confirm: { type: "boolean", description: "Must be true together with dry_run=false to apply." },
        ...DRY_RUN_PROPERTIES,
      },
    },
  },
  {
    name: "payverge_inspect_business",
    description: "Fetch admin detail for a business by numeric id or business id string. When PAYVERGE_MCP_BUSINESS_IDS is set, only a listed numeric id is accepted.",
    inputSchema: {
      type: "object",
      required: ["id"],
      properties: {
        id: { oneOf: [{ type: "number", minimum: 1 }, { type: "string" }] },
      },
    },
  },
  {
    name: "payverge_diagnose_issue",
    description: "Gather health, recent errors, failed webhooks, fiscal jobs, and optional business context into one diagnosis. Health, errors, webhooks and fiscal jobs are instance-wide; when PAYVERGE_MCP_BUSINESS_IDS is set, businessId must be a listed numeric id.",
    inputSchema: {
      type: "object",
      properties: {
        businessId: { oneOf: [{ type: "number", minimum: 1 }, { type: "string" }] },
        component: { type: "string", description: "Filter recent error logs by component." },
        source: { type: "string", description: "Filter recent error logs by source." },
      },
    },
  },
];

const ADMIN_MUTATING = new Set([
  "payverge_ack_failed_webhook",
  "payverge_requeue_fiscal_job",
]);

// Admin tools that read one business. PAYVERGE_MCP_BUSINESS_IDS is applied to
// them too, as defence in depth. It does NOT make the admin token tenant-scoped:
// health, error logs, failed webhooks and fiscal jobs are instance-wide.
const BUSINESS_SCOPED_ADMIN_TOOLS = new Set(["payverge_inspect_business", "payverge_diagnose_issue"]);

// Pick the business a business-scoped admin tool reads, once, so the
// allow-list check and the backend request always use the same value.
// payverge_inspect_business names it `id` and payverge_diagnose_issue names it
// `businessId`; each tool also accepts the other spelling, so two different
// values, or a blank one, are refused instead of letting one slip past the guard.
function resolveAdminBusinessRef(args) {
  const present = (value) => value !== undefined && value !== null;
  const hasId = present(args.id);
  const hasBusinessId = present(args.businessId);
  if (hasId && hasBusinessId && String(args.id).trim() !== String(args.businessId).trim()) {
    throw new ToolError("invalid_arguments", "id and businessId name different businesses", {
      hint: "Name the business once: id for payverge_inspect_business, businessId for payverge_diagnose_issue.",
    });
  }
  const ref = hasId ? args.id : hasBusinessId ? args.businessId : undefined;
  if (ref !== undefined && String(ref).trim() === "") {
    throw new ToolError("invalid_arguments", "The business id is blank", {
      hint: "Pass a numeric business id, or leave businessId out for an instance-wide diagnosis.",
    });
  }
  return ref;
}

// Returns the value to send to the backend: the canonical numeric id when an
// allow-list is set, otherwise the value unchanged.
function assertAdminBusinessAllowed(allowList, value) {
  if (!allowList || value === undefined) return value;
  const text = String(value).trim();
  const numeric = /^\d+$/.test(text) ? Number(text) : NaN;
  if (!Number.isInteger(numeric)) {
    throw new ToolError("business_not_allowed", `Business "${text}" cannot be checked against PAYVERGE_MCP_BUSINESS_IDS`, {
      hint: `Pass the numeric business id. This MCP server is limited to businesses ${allowList.join(", ")}.`,
    });
  }
  if (!allowList.includes(numeric)) {
    throw new ToolError("business_not_allowed", `Business ${numeric} is not in PAYVERGE_MCP_BUSINESS_IDS`, {
      hint: `This MCP server is limited to businesses ${allowList.join(", ")}.`,
    });
  }
  return numeric;
}

// Empty or missing allow-lists mean "not limited". A non-empty list is coerced
// to numbers so string ids from the environment still match numeric args.
function normalizeAllowList(businessAllowList) {
  return Array.isArray(businessAllowList) && businessAllowList.length > 0
    ? businessAllowList.map(Number)
    : null;
}

const adminDefinitions = toolDefinitions.map((tool) => ({
  ...tool,
  annotations: ADMIN_MUTATING.has(tool.name) ? ADMIN_WRITE : ADMIN_READ,
}));

/**
 * @param {object} deps
 * @param {object} [deps.client]       platform-admin client (/api/v1/admin/*), optional
 * @param {object} [deps.ownerClient]  restaurant-owner session client (/api/v1/inside/*), optional
 * @param {object} [deps.publicClient] anonymous client for public routes, optional
 * @param {object} [deps.options]      readOnly, businessAllowList, publicUrl, apiBaseUrl, ownerSession
 */
export function createPayvergeTools({ client, ownerClient, publicClient, options = {} } = {}) {
  if (!client && !ownerClient && !publicClient) throw new Error("client is required");

  const config = createConfigTools({
    ownerClient,
    publicClient,
    options: { ...options, adminConfigured: Boolean(client) },
  });
  const readOnly = Boolean(options.readOnly);
  const allowList = normalizeAllowList(options.businessAllowList);

  return {
    listTools() {
      return client ? [...config.definitions, ...adminDefinitions] : config.definitions;
    },

    async callTool(name, args = {}) {
      if (config.has(name)) return config.call(name, args);
      if (!toolDefinitions.some((tool) => tool.name === name)) {
        throw new Error(`Unknown tool: ${name}`);
      }
      try {
        return await callAdminTool(name, args ?? {});
      } catch (err) {
        return adminErrorResult(err);
      }
    },
  };

  async function callAdminTool(name, args) {
    if (!client) {
      throw new ToolError("admin_auth_not_configured", `${name} is a platform-admin tool and needs PAYVERGE_ADMIN_MCP_TOKEN`, {
        hint: "Set PAYVERGE_ADMIN_MCP_TOKEN in the MCP server env (and the matching PAYVERGE_ADMIN_MCP_TOKEN_SHA256 on the backend). See tools/payverge-admin-mcp/README.md#platform-admin-token.",
      });
    }
    if (readOnly && ADMIN_MUTATING.has(name) && !resolveDryRun(args)) {
      throw new ToolError("read_only_mode", "PAYVERGE_MCP_READ_ONLY is set, so changes are refused", {
        hint: "Previews (dry_run=true) still work. Unset PAYVERGE_MCP_READ_ONLY and restart the MCP server to apply changes.",
      });
    }
    if (BUSINESS_SCOPED_ADMIN_TOOLS.has(name)) {
      const business = assertAdminBusinessAllowed(allowList, resolveAdminBusinessRef(args));
      args = { ...args, id: undefined, businessId: business };
    }
    switch (name) {
      case "payverge_health_snapshot":
        return toolResult(await healthSnapshot(client, args));
      case "payverge_list_error_logs":
        return toolResult(await listErrorLogs(client, args));
      case "payverge_list_failed_webhooks":
        return toolResult(await listFailedWebhooks(client, args));
      case "payverge_ack_failed_webhook":
        return toolResult(await ackFailedWebhook(client, args));
      case "payverge_fiscal_summary":
        return toolResult(await client.get("/admin/fiscal/summary"));
      case "payverge_list_fiscal_jobs":
        return toolResult(await listFiscalJobs(client, args));
      case "payverge_requeue_fiscal_job":
        return toolResult(await requeueFiscalJob(client, args));
      case "payverge_inspect_business":
        return toolResult(await inspectBusiness(client, args));
      case "payverge_diagnose_issue":
        return toolResult(await diagnoseIssue(client, args));
      default:
        throw new Error(`Unknown tool: ${name}`);
    }
  }
}

function adminErrorResult(err) {
  const error = explainError(err);
  // explainError's 401/403 hints describe the owner session; admin tools
  // authenticate with the platform token instead.
  if (error.status === 401) {
    error.code = "admin_unauthorized";
    error.hint = "The backend rejected PAYVERGE_ADMIN_MCP_TOKEN. Check that the backend has the same token, or its SHA-256 in PAYVERGE_ADMIN_MCP_TOKEN_SHA256, and restart both.";
  } else if (error.status === 403) {
    error.code = "admin_forbidden";
    error.hint = "The backend refused the admin token from this address. Add the IP the backend sees to PAYVERGE_ADMIN_MCP_ALLOWED_IPS (default: loopback only).";
  }
  const structuredContent = { error };
  return {
    isError: true,
    content: [{ type: "text", text: JSON.stringify(structuredContent, null, 2) }],
    structuredContent,
  };
}

// Webhook payloads carry an attacker-influenced businessId. Resolve it the
// same way the MCP dispatcher does, so PAYVERGE_MCP_BUSINESS_IDS applies
// before any /admin/businesses/:id/detail request.
export async function diagnoseIssueWithinAllowList(client, args = {}, businessAllowList) {
  const allowList = normalizeAllowList(businessAllowList);
  const business = assertAdminBusinessAllowed(allowList, resolveAdminBusinessRef(args));
  return diagnoseIssue(client, { ...args, id: undefined, businessId: business });
}

export async function diagnoseIssue(client, args = {}) {
  // Callers (the MCP dispatcher and diagnoseIssueWithinAllowList) have already
  // resolved and checked the business. Blank means an instance-wide diagnosis.
  const raw = args.businessId ?? args.id;
  const businessRef = raw === undefined || raw === null || String(raw).trim() === "" ? undefined : raw;
  const [health, errors, failedWebhooks, fiscal, fiscalJobs, business] = await Promise.all([
    capture("health", () => client.get("/admin/system/health")),
    capture("recent_errors", () => listErrorLogs(client, { ...args, limit: 10 })),
    capture("failed_webhooks", () => listFailedWebhooks(client, { limit: 25 })),
    capture("fiscal_summary", () => client.get("/admin/fiscal/summary")),
    capture("fiscal_jobs", () => listFiscalJobs(client, { status: "failed_retryable", limit: 10 })),
    businessRef !== undefined
      ? capture("business", () => inspectBusiness(client, { businessId: businessRef }))
      : Promise.resolve(undefined),
  ]);

  const signals = [];
  const healthValue = valueOf(health);
  for (const queue of healthValue?.queues ?? []) {
    if (Number(queue.count) > 0) {
      signals.push({
        name: queue.name,
        count: Number(queue.count),
        label: queue.label,
        source: "admin/system/health",
      });
    }
  }

  const errorsValue = valueOf(errors);
  if (Number(errorsValue?.total ?? errorsValue?.errors?.length ?? 0) > 0) {
    signals.push({
      name: "recent_errors",
      count: Number(errorsValue.total ?? errorsValue.errors.length),
      source: "admin/errors",
    });
  }

  const webhooksValue = valueOf(failedWebhooks);
  if (
    Number(webhooksValue?.count ?? webhooksValue?.events?.length ?? 0) > 0 &&
    !signals.some((signal) => signal.name === "failed_webhooks")
  ) {
    signals.push({
      name: "failed_webhooks",
      count: Number(webhooksValue.count ?? webhooksValue.events.length),
      source: "admin/webhooks/failed",
    });
  }

  const fiscalValue = valueOf(fiscal);
  const fiscalFailures =
    Number(fiscalValue?.failed_retryable_jobs ?? 0) +
    Number(fiscalValue?.failed_permanent_jobs ?? 0);
  if (fiscalFailures > 0) {
    signals.push({
      name: "fiscal_failed_jobs",
      count: fiscalFailures,
      source: "admin/fiscal/summary",
    });
  }

  const recommendations = buildRecommendations(signals);

  return {
    status: healthValue?.status ?? (signals.length > 0 ? "degraded" : "unknown"),
    generated_at: new Date().toISOString(),
    signals,
    recommendations,
    health,
    errors,
    failed_webhooks: failedWebhooks,
    fiscal,
    fiscal_jobs: fiscalJobs,
    business,
  };
}

async function healthSnapshot(client, args) {
  const out = {
    health: await client.get("/admin/system/health"),
  };
  if (args.includeErrors) {
    out.errors = await listErrorLogs(client, { limit: 10 });
  }
  if (args.includeFailedWebhooks) {
    out.failed_webhooks = await listFailedWebhooks(client, { limit: 25 });
  }
  if (args.includeFiscal) {
    out.fiscal = await client.get("/admin/fiscal/summary");
  }
  return out;
}

async function listErrorLogs(client, args) {
  return client.get("/admin/errors", {
    source: args.source,
    component: args.component,
    limit: clampInt(args.limit, DEFAULT_LIMIT, 1, 200),
    offset: clampInt(args.offset, 0, 0, Number.MAX_SAFE_INTEGER),
  });
}

async function listFailedWebhooks(client, args) {
  const result = await client.get("/admin/webhooks/failed", {
    limit: clampInt(args.limit, DEFAULT_LIMIT, 1, 200),
  });
  const limit = clampInt(args.limit, DEFAULT_LIMIT, 1, 200);
  if (Array.isArray(result.events) && result.events.length > limit) {
    return { ...result, events: result.events.slice(0, limit), returned: limit };
  }
  return result;
}

async function ackFailedWebhook(client, args) {
  const id = positiveId(args.id, "id");
  const reason = String(args.reason ?? "").trim();
  const path = `/admin/webhooks/${id}/acknowledge`;
  if (resolveDryRun(args)) {
    return {
      dry_run: true,
      dryRun: true,
      action: `POST ${path}`,
      body: reason ? { reason } : {},
      next: "Inspect the webhook payload/error first, then call again with dry_run=false and confirm=true only if it is safe to ignore.",
    };
  }
  if (args.confirm !== true) {
    throw new ToolError("confirmation_required", "Acknowledging a failed webhook requires confirm=true when dry_run=false", {
      hint: "Call again with dry_run=false and confirm=true, and a non-empty reason, to apply.",
    });
  }
  if (reason === "") {
    throw new ToolError("invalid_arguments", "reason is required to acknowledge a failed webhook");
  }
  return {
    dry_run: false,
    dryRun: false,
    response: await client.post(path, { reason }),
  };
}

async function listFiscalJobs(client, args) {
  return client.get("/admin/fiscal/jobs", {
    status: args.status,
    limit: clampInt(args.limit, DEFAULT_LIMIT, 1, 100),
    offset: clampInt(args.offset, 0, 0, Number.MAX_SAFE_INTEGER),
  });
}

async function requeueFiscalJob(client, args) {
  const id = positiveId(args.id, "id");
  const path = `/admin/fiscal/jobs/${id}/requeue`;
  const body = args.allow_permanent === true ? { allow_permanent: true } : {};
  if (resolveDryRun(args)) {
    return {
      dry_run: true,
      dryRun: true,
      action: `POST ${path}`,
      body,
      next: "Call again with dry_run=false and confirm=true to requeue this fiscal job. failed_permanent jobs also need allow_permanent=true.",
    };
  }
  if (args.confirm !== true) {
    throw new ToolError("confirmation_required", "Requeuing a fiscal job requires confirm=true when dry_run=false", {
      hint: "Call again with dry_run=false and confirm=true to apply. failed_permanent jobs also need allow_permanent=true.",
    });
  }
  return {
    dry_run: false,
    dryRun: false,
    response: await client.post(path, body),
  };
}

async function inspectBusiness(client, args) {
  const id = businessId(args.id ?? args.businessId);
  return client.get(`/admin/businesses/${encodeURIComponent(id)}/detail`);
}

async function capture(name, fn) {
  try {
    return await fn();
  } catch (err) {
    return {
      capture: name,
      error: err?.message ?? String(err),
      status: err?.status,
      body: err?.body,
    };
  }
}

function valueOf(result) {
  if (!result || typeof result !== "object") return result;
  if ("capture" in result && "error" in result) return undefined;
  return result;
}

function buildRecommendations(signals) {
  const names = new Set(signals.map((signal) => signal.name));
  const recommendations = [];

  if (names.has("failed_webhooks")) {
    recommendations.push("Inspect failed payment webhook rows with payverge_list_failed_webhooks. Acknowledge reviewed stale/no-op rows with payverge_ack_failed_webhook using dry_run=false and confirm=true, with a reason.");
  }
  if (names.has("failed_fiscal_jobs") || names.has("fiscal_failed_jobs")) {
    recommendations.push("Inspect retryable fiscal jobs with payverge_list_fiscal_jobs status=failed_retryable, then requeue a safe job with payverge_requeue_fiscal_job using dry_run=false and confirm=true. failed_permanent jobs also need allow_permanent=true.");
  }
  if (names.has("errors_1h") || names.has("recent_errors")) {
    recommendations.push("Group recent errors by component with payverge_list_error_logs and correlate request_id values with backend logs.");
  }
  if (recommendations.length === 0) {
    recommendations.push("No obvious remediation queue is non-empty. Check recent deploys, external provider status, and targeted business context.");
  }

  return recommendations;
}

function toolResult(structuredContent) {
  return {
    content: [
      {
        type: "text",
        text: JSON.stringify(structuredContent, null, 2),
      },
    ],
    structuredContent,
  };
}

function clampInt(value, fallback, min, max) {
  if (value === undefined || value === null || value === "") return fallback;
  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.min(Math.max(parsed, min), max);
}

function positiveId(value, name) {
  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    throw new Error(`${name} must be a positive integer`);
  }
  return parsed;
}

function businessId(value) {
  if (typeof value === "number") return String(positiveId(value, "id"));
  const trimmed = String(value ?? "").trim();
  if (!trimmed) throw new Error("id is required");
  return trimmed;
}
