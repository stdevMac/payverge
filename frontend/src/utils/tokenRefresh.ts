// Proactive access-token refresh. The backend issues a 15-min `session_token`
// cookie and a 7-day `refresh_token` cookie. We re-hit /auth/refresh every
// ~12 min so that a long-lived dashboard (KDS, dispatch console, admin
// reports) stays alive without ever showing the user a "Stay logged in?"
// prompt. The user only sees a modal when the refresh actually fails — i.e.
// the refresh_token cookie is dead and we cannot recover the session.
const REFRESH_INTERVAL_MS = 12 * 60 * 1000;
// After a *transient* failure (5xx during a deploy, a 429 rate-limit, a network
// blip at the tick) we retry sooner than a full cycle so the 15-min access
// cookie is re-minted before it expires — instead of waiting another 12 min.
// INVARIANT (P1-9): this 60s retry must stay strictly inside the backend's
// 90s refresh-replay grace (session.RefreshReuseGracePeriod). If the earlier
// POST committed server-side and only the response was lost, the retried
// stale token gets a benign AUTH_REFRESH_ROTATED — never family revocation.
const RETRY_INTERVAL_MS = 60 * 1000;
// Consecutive failed refreshes before we treat the session as genuinely dead
// and surface the expiry. Below this we keep retrying transparently so a single
// transient failure can never permanently kill proactive refresh (which used to
// leave the access cookie to silently expire, 401'ing the SSE reconnect).
const MAX_CONSECUTIVE_FAILURES = 3;

export interface TokenRefreshOptions {
  refreshFn: () => Promise<
    | boolean
    | {
        ok: boolean;
        sessionDead?: boolean;
      }
  >;
  onRefreshSuccess: () => void;
  onSessionExpired: () => void;
}

export function startTokenRefreshTimer(options: TokenRefreshOptions): () => void {
  let timer: ReturnType<typeof setTimeout> | null = null;
  let cancelled = false;
  let consecutiveFailures = 0;

  function schedule(delay: number) {
    timer = setTimeout(runCycle, delay);
  }

  function handleFailure(sessionDead?: boolean) {
    if (cancelled) return;
    if (sessionDead === false) {
      // A classified non-dead failure (for example unrecovered token rotation)
      // stays on the short retry cadence without progressing toward expiry.
      consecutiveFailures = 0;
      schedule(RETRY_INTERVAL_MS);
      return;
    }
    consecutiveFailures += 1;
    if (consecutiveFailures >= MAX_CONSECUTIVE_FAILURES) {
      // Sustained failure — the refresh_token is genuinely dead/revoked.
      options.onSessionExpired();
      return;
    }
    // Transient failure: retry on the shorter cadence rather than giving up.
    schedule(RETRY_INTERVAL_MS);
  }

  function runCycle() {
    if (cancelled) return;
    options
      .refreshFn()
      .then((result) => {
        if (cancelled) return;
        const success = typeof result === "boolean" ? result : result.ok;
        if (success) {
          consecutiveFailures = 0;
          options.onRefreshSuccess();
          schedule(REFRESH_INTERVAL_MS);
        } else {
          handleFailure(
            typeof result === "boolean" ? undefined : result.sessionDead,
          );
        }
      })
      .catch(() => {
        handleFailure();
      });
  }

  schedule(REFRESH_INTERVAL_MS);

  return () => {
    cancelled = true;
    if (timer) clearTimeout(timer);
  };
}
