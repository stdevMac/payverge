import { axiosInstance } from "./tools/instance";

/**
 * Who may create an operator account on this instance (backend
 * REGISTRATION_MODE, default "invite"):
 *  - invite: signup needs an admin-minted invite code
 *  - open:   anyone can sign up
 *  - closed: no new accounts; existing users sign in only
 */
export type RegistrationMode = "invite" | "open" | "closed";

const REGISTRATION_MODES: readonly RegistrationMode[] = [
  "invite",
  "open",
  "closed",
];

export function parseRegistrationMode(value: unknown): RegistrationMode | null {
  if (typeof value !== "string") return null;
  const normalized = value.trim().toLowerCase();
  return (REGISTRATION_MODES as readonly string[]).includes(normalized)
    ? (normalized as RegistrationMode)
    : null;
}

/** True when a backend error body is the closed-signup refusal. */
export function isRegistrationClosedError(data: unknown): boolean {
  if (!data || typeof data !== "object") return false;
  const params = (data as { params?: { reason?: unknown } }).params;
  return params?.reason === "registration_closed";
}

// The mode is server configuration that only changes on a backend restart,
// so one successful read per page load is enough. A failed read is not
// cached: the next signup view retries.
let inflight: Promise<RegistrationMode | null> | null = null;
let known: RegistrationMode | null = null;

/** Last mode read from the backend in this page load, if any. */
export function peekRegistrationMode(): RegistrationMode | null {
  return known;
}

/**
 * GET /api/v1/platform/registration-mode. Resolves null (never rejects) when
 * the backend is unreachable or answers with an unknown value; the signup
 * form then stays permissive and the backend remains the enforcer.
 */
export function fetchRegistrationMode(): Promise<RegistrationMode | null> {
  if (known) return Promise.resolve(known);
  if (!inflight) {
    inflight = Promise.resolve()
      .then(() =>
        axiosInstance.get<{ registration_mode?: string }>(
          "/platform/registration-mode",
          { _skipErrorToast: true, _skipAuthRefresh: true },
        ),
      )
      .then((response) => {
        known = parseRegistrationMode(response?.data?.registration_mode);
        return known;
      })
      .catch(() => null)
      .finally(() => {
        inflight = null;
      });
  }
  return inflight;
}

export function resetRegistrationModeCacheForTests(): void {
  inflight = null;
  known = null;
}
