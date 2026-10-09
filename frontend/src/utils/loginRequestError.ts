/**
 * Login transport failures (CORS / no response / sanitized axios network
 * errors) must not collapse to "Something went wrong" (issue 363). WAF 429
 * without ACAO also lands here — show an honest reachability message rather
 * than guessing a rate limit.
 */
const SANITIZED_NETWORK_MESSAGE =
  /unable to connect|couldn't reach|could not reach|check your (internet )?connection/i;

export function isCorsOrNoResponseLoginFailure(err: unknown): boolean {
  if (err == null) return true;
  if (typeof err !== "object") return false;
  const e = err as {
    message?: string;
    code?: string;
    response?: unknown;
    status?: number;
  };
  if (e.response != null) return false;
  // A real HTTP status means we got a response. status 0 is a transport failure.
  if (typeof e.status === "number" && e.status > 0) return false;
  const message = String(e.message ?? "");
  const code = String(e.code ?? "");
  return (
    code === "ERR_NETWORK" ||
    code === "ERR_FAILED" ||
    /network error/i.test(message) ||
    /failed to fetch/i.test(message) ||
    /err_failed/i.test(message) ||
    /cors/i.test(message) ||
    SANITIZED_NETWORK_MESSAGE.test(message)
  );
}

export function mapLoginRequestError(
  err: unknown,
  ordinaryMessage: string,
  transportMessage: string,
): string {
  if (isCorsOrNoResponseLoginFailure(err)) return transportMessage;
  return ordinaryMessage;
}
