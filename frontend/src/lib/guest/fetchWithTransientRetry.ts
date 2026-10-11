const TRANSIENT_STATUSES = new Set([408, 425, 429, 500, 502, 503, 504]);

const wait = (ms: number) =>
  new Promise<void>((resolve) => {
    setTimeout(resolve, ms);
  });

export async function fetchWithTransientRetry(
  input: string,
  init: RequestInit = {},
  attempts = 3,
): Promise<Response> {
  let lastError: unknown;
  let retried404 = false;
  for (let i = 0; i < attempts; i += 1) {
    try {
      const res = await fetch(input, init);
      if (res.ok) return res;
      // A live table/storefront can 404 on the first edge hop and resolve on
      // reload. Retry one 404; a second 404 is a confirmed miss.
      if (res.status === 404) {
        if (retried404 || i === attempts - 1) return res;
        retried404 = true;
        lastError = res;
        continue;
      }
      if (!TRANSIENT_STATUSES.has(res.status)) {
        return res;
      }
      lastError = res;
    } catch (error) {
      lastError = error;
    }
    if (i < attempts - 1) {
      await wait(50 * 2 ** i);
    }
  }
  if (lastError instanceof Response) return lastError;
  throw lastError instanceof Error
    ? lastError
    : new Error("fetch failed");
}
