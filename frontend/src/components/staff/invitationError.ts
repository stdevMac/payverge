export type InvitationErrorKind = "permanent" | "transient";

// Stable backend codes (backend/internal/server/staff_handlers.go). Codes
// decouple classification from English message text — es/es-AR invitees used
// to be misclassified because their localized messages matched nothing (P3).
const PERMANENT_CODES = new Set([
  "INVITE_EXPIRED",
  "INVITE_INVALID",
  "INVITE_CANNOT_RESEND",
  "STAFF_ACCOUNT_EXISTS",
  "STAFF_EMAIL_LINKED",
  "STAFF_EMAIL_EXISTS",
  "INVITE_ALREADY_PENDING",
]);
const TRANSIENT_CODES = new Set(["INVITE_VALIDATION_FAILED", "RATE_LIMITED"]);

// Keywords that mean the invitation itself is dead and retrying won't help.
// Backend invitation states are pending|accepted|expired|revoked, so its
// error envelopes carry these tokens. Everything else (network/server blips)
// is transient and worth a retry.
const PERMANENT_PATTERNS = [
  "expired",
  "already", // covers "already used" / "already accepted"
  "has been used", // specific, unlike a bare "used" that matches "could not be used"
  "revoked",
  "no longer",
  "not found",
  "invalid invitation token",
  "accepted",
];

/**
 * Classifies a staff-invitation error. Prefers the stable backend `code`
 * (language-independent); falls back to English-substring matching for
 * uncoded/legacy responses. Unknown/empty → transient (safe default: retry).
 */
export function classifyInvitationError(
  message: string,
  code?: string,
): InvitationErrorKind {
  if (code) {
    if (PERMANENT_CODES.has(code)) return "permanent";
    if (TRANSIENT_CODES.has(code)) return "transient";
  }
  const lower = (message || "").toLowerCase();
  return PERMANENT_PATTERNS.some((p) => lower.includes(p))
    ? "permanent"
    : "transient";
}
