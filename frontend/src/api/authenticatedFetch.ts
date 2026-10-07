import { API_BASE_URL } from "@/api/tools/baseUrl";
import { isNonSessionAuthFailure } from "@/utils/apiError";
import { refreshAuthSession } from "@/utils/refreshAuth";

async function peekResponseCode(response: Response): Promise<string | undefined> {
  try {
    const data = (await response.clone().json()) as { code?: unknown };
    return typeof data.code === "string" ? data.code : undefined;
  } catch {
    return undefined;
  }
}

const refreshBaseFor = (input: string | URL): string => {
  const url = String(input);
  const insideIndex = url.indexOf("/inside/");
  return insideIndex >= 0 ? url.slice(0, insideIndex) : API_BASE_URL;
};

/**
 * Fetch for owner/staff dashboard APIs that mirrors the axios client's
 * refresh-once behavior without making SSR callers depend on axios.
 */
export async function authenticatedFetch(
  input: string | URL,
  init: RequestInit = {},
): Promise<Response> {
  const firstResponse = await globalThis.fetch(input, init);
  if (firstResponse.status !== 401) return firstResponse;

  const responseCode = await peekResponseCode(firstResponse);
  if (isNonSessionAuthFailure(responseCode)) {
    return firstResponse;
  }

  const refreshResult = await refreshAuthSession(refreshBaseFor(input));
  if (!refreshResult.ok) {
    if (refreshResult.sessionDead && typeof window !== "undefined") {
      window.dispatchEvent(new CustomEvent("auth:session-expired"));
    }
    return firstResponse;
  }

  // A single retry is deliberate: a second 401 is returned to the caller and
  // can never recurse into another refresh attempt.
  return globalThis.fetch(input, init);
}
