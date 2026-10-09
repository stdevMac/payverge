import * as Sentry from "@sentry/nextjs";

// Next loads the server instrumentation hook only from the directory that
// holds app/, which is src/ in this project. A frontend/instrumentation.ts
// is silently ignored (instrumentation-client.ts is resolved separately and
// may stay at the root). src/instrumentation.test.ts pins the location.

export async function register() {
  if (process.env.NEXT_RUNTIME === "nodejs") {
    // Surface runtime public-config mistakes (PUBLIC_URL missing/http, bad
    // API_URL, ...) once at boot. Issues name the variable and the rule only,
    // never the value. See docs/self-hosting/frontend-config.md.
    const { validatePublicEnv } = await import("./config/publicConfig");
    // Record each request's socket peer for the same-origin proxy, which
    // cannot see the socket (src/lib/proxy/peerStamp.ts).
    const { installPeerStamp } = await import("./lib/proxy/installPeerStamp");
    const { trustedProxyIssues } = await import("./lib/proxy/trustedProxies");
    installPeerStamp();
    for (const issue of [
      ...validatePublicEnv(process.env),
      ...trustedProxyIssues(process.env),
    ]) {
      console.warn(`[payverge] frontend config: ${issue}`);
    }
    await import("../sentry.server.config");
  }

  if (process.env.NEXT_RUNTIME === "edge") {
    await import("../sentry.edge.config");
  }
}

export const onRequestError = Sentry.captureRequestError;
