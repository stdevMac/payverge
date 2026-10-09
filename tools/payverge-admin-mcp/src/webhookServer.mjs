import { createHash, timingSafeEqual } from "node:crypto";
import http from "node:http";

const MAX_BODY_BYTES = 1024 * 1024;

export async function startWebhookServer({
  host = "127.0.0.1",
  port = 3977,
  secret,
  diagnoseIssue,
  log = console,
} = {}) {
  if (!secret) throw new Error("PAYVERGE_MCP_WEBHOOK_SECRET is required for webhook receiver");
  if (typeof diagnoseIssue !== "function") throw new Error("diagnoseIssue callback is required");

  const server = http.createServer(async (req, res) => {
    try {
      if (req.method === "GET" && req.url === "/health") {
        return json(res, 200, { ok: true });
      }

      if (req.method !== "POST" || req.url !== "/webhooks/backend-flag") {
        return json(res, 404, { error: "not found" });
      }

      if (!authorized(req, secret)) {
        return json(res, 401, { error: "unauthorized" });
      }

      const payload = await readJsonBody(req);
      const diagnosis = await diagnoseIssue(normalizeWebhookPayload(payload));
      return json(res, 200, { ok: true, diagnosis });
    } catch (err) {
      const status = err?.statusCode ?? 500;
      if (status >= 500) {
        log.error?.(`payverge-admin-webhooks error: ${err?.stack ?? err}`);
      }
      return json(res, status, { error: err?.message ?? "internal error" });
    }
  });

  await new Promise((resolve) => server.listen(port, host, resolve));
  const address = server.address();
  const actualPort = typeof address === "object" ? address.port : port;

  return {
    url: `http://${host}:${actualPort}`,
    close() {
      return new Promise((resolve, reject) => {
        server.close((err) => (err ? reject(err) : resolve()));
      });
    },
  };
}

function normalizeWebhookPayload(payload = {}) {
  return compactObject({
    businessId: payload.businessId ?? payload.business_id,
    component: payload.component,
    source: payload.source,
    type: payload.type,
    severity: payload.severity,
    message: payload.message,
  });
}

function authorized(req, secret) {
  const headerSecret = req.headers["x-payverge-mcp-secret"];
  if (typeof headerSecret === "string" && secretsEqual(headerSecret, secret)) return true;

  const auth = req.headers.authorization;
  return typeof auth === "string" && secretsEqual(auth, `Bearer ${secret}`);
}

// Constant-time comparison. Hashing first gives both sides the same length,
// so neither the content nor the length of the secret leaks through timing.
export function secretsEqual(candidate, expected) {
  const a = createHash("sha256").update(String(candidate)).digest();
  const b = createHash("sha256").update(String(expected)).digest();
  return timingSafeEqual(a, b);
}

async function readJsonBody(req) {
  const chunks = [];
  let size = 0;
  for await (const chunk of req) {
    size += chunk.length;
    if (size > MAX_BODY_BYTES) {
      const err = new Error("payload too large");
      err.statusCode = 413;
      throw err;
    }
    chunks.push(chunk);
  }

  const raw = Buffer.concat(chunks).toString("utf8");
  if (!raw.trim()) return {};
  try {
    return JSON.parse(raw);
  } catch {
    const err = new Error("invalid JSON payload");
    err.statusCode = 400;
    throw err;
  }
}

function json(res, status, body) {
  res.statusCode = status;
  res.setHeader("content-type", "application/json");
  res.end(JSON.stringify(body));
}

function compactObject(input) {
  return Object.fromEntries(
    Object.entries(input).filter(([, value]) => value !== undefined && value !== null && value !== ""),
  );
}
