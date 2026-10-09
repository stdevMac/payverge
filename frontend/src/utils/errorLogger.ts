import { getPublicConfig } from "@/config/publicConfig";
import { captureClientError } from "@/lib/sentry/reporting";

interface ErrorLogPayload {
  timestamp: string;
  error: string;
  component: string;
  function: string;
  requestId?: string;
  additionalInfo?: Record<string, any>;
}

// Per-signature debounce: skip duplicate errors fired within 1 second.
// Keyed on (component, function, error message) so two unrelated errors in
// the same second BOTH get sent to the server — only literal repeats are
// dropped. Previously this was a single global timestamp, which silently
// suppressed every error that followed any other error by <1s.
const lastLogTimePerKey = new Map<string, number>();
const DEBOUNCE_MS = 1000;
// Cap the map so a long-running app doesn't grow it unbounded if every error
// has a unique payload. Once we hit the cap, drop the oldest entry.
const DEBOUNCE_MAX_ENTRIES = 200;

export const logError = async (
  error: Error | string,
  component: string,
  functionName: string,
  additionalInfo?: Record<string, any>,
  requestId?: string
) => {
  const message = error instanceof Error ? error.message : error;

  // ONE shared per-signature debounce gates BOTH the Sentry capture and the
  // backend POST. Previously the Sentry mirror ran on every logError call,
  // which meant an error loop (same signature firing hundreds of times a
  // second) burned unbounded Sentry quota even though the backend POST was
  // already debounced. Keyed on (component, function, message) so distinct
  // errors in the same window each get through; only literal repeats drop.
  const debounceKey = `${component}::${functionName}::${message}`;
  const now = Date.now();
  const lastFiredAt = lastLogTimePerKey.get(debounceKey) ?? 0;
  if (now - lastFiredAt < DEBOUNCE_MS) {
    console.warn('[logError debounced]', { component, function: functionName, error: message });
    return;
  }
  lastLogTimePerKey.set(debounceKey, now);
  if (lastLogTimePerKey.size > DEBOUNCE_MAX_ENTRIES) {
    const oldestKey = lastLogTimePerKey.keys().next().value;
    if (oldestKey !== undefined) lastLogTimePerKey.delete(oldestKey);
  }

  // Mirror to Sentry via the scrubbing reporting layer. Gated by the debounce
  // above, but kept INDEPENDENT of the API-URL guard below so it still fires
  // when the API URL is unset (Sentry does its own server-side dedup).
  captureClientError({
    error,
    component,
    functionName,
    additionalInfo,
    requestId,
  });

  // Guard: skip HTTP call if API URL is not configured
  const apiUrl = getPublicConfig().apiUrl;
  if (!apiUrl) {
    console.error('Error (no API URL configured):', { component, function: functionName, error });
    return;
  }

  const errorPayload: ErrorLogPayload = {
    timestamp: new Date().toISOString(),
    error: typeof message === "string" ? message : String(message),
    component,
    function: functionName,
    requestId,
    additionalInfo,
  };

  try {
    await fetch(`${apiUrl}/logs/error`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(errorPayload),
      signal: AbortSignal.timeout(5000),
    });
  } catch (loggingError) {
    // Fallback to console if logging fails — don't cascade
    console.error('Failed to log error:', errorPayload);
  }
};
