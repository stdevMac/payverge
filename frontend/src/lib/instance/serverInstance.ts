/**
 * Server-side read of GET /api/v1/instance for middleware, the root layout
 * and server components. Uses fetch() (axios is unsafe on the server) against
 * the private backend URL from getServerApiUrl().
 *
 * The payload only changes when the backend restarts, so a good answer is
 * reused for INSTANCE_TTL_MS and kept as the fallback when a later refresh
 * fails. With no answer at all the result is null and callers use their
 * defaults.
 */

import { getServerApiUrl } from "@/lib/serverApiUrl";
import { parseInstanceInfo, type InstanceInfo } from "./instanceInfo";

const INSTANCE_TTL_MS = 30_000;
const INSTANCE_FETCH_TIMEOUT_MS = 1500;
export const INSTANCE_FAILURE_TTL_MS = 5_000;

let lastGood: InstanceInfo | null = null;
let expiresAt = 0;
let inflight: Promise<InstanceInfo | null> | null = null;

/** Test-only: forget the cached payload. */
export function resetServerInstanceCacheForTests(): void {
  lastGood = null;
  expiresAt = 0;
  inflight = null;
}

async function fetchInstance(): Promise<InstanceInfo | null> {
  const apiUrl = getServerApiUrl();
  if (!apiUrl) return null;
  try {
    const res = await fetch(`${apiUrl}/instance`, {
      cache: "no-store",
      headers: { accept: "application/json" },
      signal: AbortSignal.timeout(INSTANCE_FETCH_TIMEOUT_MS),
    });
    if (!res.ok) return null;
    return parseInstanceInfo(await res.json());
  } catch {
    return null;
  }
}

/** The instance payload, cached; null when the backend never answered. */
export async function getServerInstanceInfo(
  now: number = Date.now(),
): Promise<InstanceInfo | null> {
  if (now < expiresAt) return lastGood;
  if (!inflight) {
    inflight = fetchInstance()
      .then((info) => {
        if (info) {
          lastGood = info;
          expiresAt = Date.now() + INSTANCE_TTL_MS;
        } else {
          // Negative cache: while the backend is down, do not make every
          // request wait INSTANCE_FETCH_TIMEOUT_MS again.
          expiresAt = Date.now() + INSTANCE_FAILURE_TTL_MS;
        }
        return info ?? lastGood;
      })
      .finally(() => {
        inflight = null;
      });
  }
  return inflight;
}
